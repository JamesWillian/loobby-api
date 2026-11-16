// Package teams e a porta do EventTeamController e do EventTeamService do
// Kotlin: os times de um evento, seus jogadores e o sorteio automatico.
//
// Nenhuma rota confere permissao alem do token: qualquer usuario autenticado
// mexe nos times de qualquer evento. E assim no Kotlin (contract/ACHADOS.md).
package teams

import (
	"context"
	"math"
	"math/rand/v2"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
	"github.com/JamesWillian/loobby-api/api-go/internal/kotlin"
)

// teamResponse: EventTeamResponse.
type teamResponse struct {
	ID      string           `json:"id"`
	EventID string           `json:"eventId"`
	Order   int32            `json:"order"`
	Name    string           `json:"name"`
	Color   *string          `json:"color"`
	Players []playerResponse `json:"players"`
}

// playerResponse: TeamPlayerResponse.
type playerResponse struct {
	UserID      string  `json:"userId"`
	Role        *string `json:"role"`
	Username    string  `json:"username"`
	Displayname *string `json:"displayname"`
	AvatarURL   *string `json:"avatarUrl"`
}

// playerRequest: TeamPlayerRequest / AddPlayerToTeamRequest.
type playerRequest struct {
	UserID string
	Role   *string
}

type createRequest struct {
	Name    string
	Color   *string
	Order   *int32
	Players []playerRequest
}

type updateRequest struct {
	Name  *string
	Color *string
	Order *int32
}

type autoGenerateRequest struct {
	TeamSize        *int32
	TeamCount       *int32
	IncludeReserves bool
}

var teamColors = []string{
	"#F44336", // vermelho
	"#2196F3", // azul
	"#4CAF50", // verde
	"#FF9800", // laranja
	"#9C27B0", // roxo
	"#009688", // teal
	"#3F51B5", // indigo
}

type Service struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: sqlc.New(pool)}
}

var (
	errEventNotFound = httpx.BadRequest("Event not found")
	errTeamNotFound  = httpx.BadRequest("Team not found")
	errWrongEvent    = httpx.BadRequest("Team does not belong to this event")
)

// teamOf le o time e confere que ele e do evento do caminho.
func teamOf(ctx context.Context, q *sqlc.Queries, eventID, teamID string) (sqlc.EventTeam, error) {
	t, err := q.FindTeam(ctx, teamID)
	if db.NotFound(err) {
		return sqlc.EventTeam{}, errTeamNotFound
	}
	if err != nil {
		return sqlc.EventTeam{}, err
	}
	if t.EventID != eventID {
		return sqlc.EventTeam{}, errWrongEvent
	}
	return t, nil
}

// responses: buildTeamResponses. Jogador cujo usuario sumiu, ou cuja conta foi
// excluida (username em branco), nao aparece. Saida ordenada por "order".
func responses(ctx context.Context, q *sqlc.Queries, teams []sqlc.EventTeam) ([]teamResponse, error) {
	teamIDs := make([]string, 0, len(teams))
	for _, t := range teams {
		teamIDs = append(teamIDs, t.ID)
	}
	players, err := q.FindPlayersByTeams(ctx, teamIDs)
	if err != nil {
		return nil, err
	}
	users := map[string]sqlc.User{}
	if len(players) > 0 {
		userIDs := make([]string, 0, len(players))
		for _, p := range players {
			userIDs = append(userIDs, p.UserID)
		}
		rows, err := q.FindUsersByIDs(ctx, userIDs)
		if err != nil {
			return nil, err
		}
		for _, u := range rows {
			users[u.ID] = u
		}
	}
	byTeam := map[string][]playerResponse{}
	for _, p := range players {
		u, ok := users[p.UserID]
		if !ok || kotlin.IsBlank(u.Username) {
			continue
		}
		byTeam[p.TeamID] = append(byTeam[p.TeamID], playerResponse{
			UserID: u.ID, Role: p.Role, Username: u.Username, Displayname: &u.Displayname, AvatarURL: u.AvatarUrl,
		})
	}
	out := make([]teamResponse, 0, len(teams))
	for _, t := range teams {
		list := byTeam[t.ID]
		if list == nil {
			list = []playerResponse{} // [] no JSON, nunca null
		}
		out = append(out, teamResponse{ID: t.ID, EventID: t.EventID, Order: t.Order, Name: t.Name, Color: t.Color, Players: list})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out, nil
}

func response(ctx context.Context, q *sqlc.Queries, t sqlc.EventTeam) (teamResponse, error) {
	list, err := responses(ctx, q, []sqlc.EventTeam{t})
	if err != nil {
		return teamResponse{}, err
	}
	return list[0], nil
}

// Create: EventTeamService.createTeam. Sem "order", o time entra depois do
// ultimo. Jogador cujo usuario nao existe e ignorado em silencio.
func (s *Service) Create(ctx context.Context, eventID string, req createRequest) (teamResponse, error) {
	var resp teamResponse
	err := db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		if exists, err := q.EventExists(ctx, eventID); err != nil {
			return err
		} else if !exists {
			return errEventNotFound
		}
		order := int32(1)
		if req.Order != nil {
			order = *req.Order
		} else {
			existing, err := q.FindTeamsByEvent(ctx, eventID) // ordenado por "order"
			if err != nil {
				return err
			}
			if n := len(existing); n > 0 {
				order = existing[n-1].Order + 1
			}
		}
		team := sqlc.EventTeam{ID: ids.New(), EventID: eventID, Order: order, Name: req.Name, Color: req.Color}
		if err := insertTeam(ctx, q, team); err != nil {
			return err
		}
		if len(req.Players) > 0 {
			userIDs := make([]string, 0, len(req.Players))
			for _, p := range req.Players {
				userIDs = append(userIDs, p.UserID)
			}
			rows, err := q.FindUsersByIDs(ctx, userIDs)
			if err != nil {
				return err
			}
			known := map[string]bool{}
			for _, u := range rows {
				known[u.ID] = true
			}
			for _, p := range req.Players {
				if !known[p.UserID] {
					continue
				}
				err := q.UpsertTeamPlayer(ctx, sqlc.UpsertTeamPlayerParams{TeamID: team.ID, UserID: p.UserID, Role: p.Role})
				if err != nil {
					return err
				}
			}
		}
		var err error
		resp, err = response(ctx, q, team)
		return err
	})
	return resp, err
}

func insertTeam(ctx context.Context, q *sqlc.Queries, t sqlc.EventTeam) error {
	return q.InsertTeam(ctx, sqlc.InsertTeamParams{ID: t.ID, EventID: t.EventID, OrderIndex: t.Order, Name: t.Name, Color: t.Color})
}

// List: EventTeamService.listTeams.
func (s *Service) List(ctx context.Context, eventID string) ([]teamResponse, error) {
	if exists, err := s.q.EventExists(ctx, eventID); err != nil {
		return nil, err
	} else if !exists {
		return nil, errEventNotFound
	}
	teams, err := s.q.FindTeamsByEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if len(teams) == 0 {
		return []teamResponse{}, nil
	}
	return responses(ctx, s.q, teams)
}

// Update: EventTeamService.updateTeam.
func (s *Service) Update(ctx context.Context, eventID, teamID string, req updateRequest) (teamResponse, error) {
	var resp teamResponse
	err := db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		t, err := teamOf(ctx, q, eventID, teamID)
		if err != nil {
			return err
		}
		if req.Name != nil {
			t.Name = *req.Name
		}
		if req.Color != nil {
			t.Color = req.Color
		}
		if req.Order != nil {
			t.Order = *req.Order
		}
		err = q.UpdateTeam(ctx, sqlc.UpdateTeamParams{ID: t.ID, Name: t.Name, Color: t.Color, OrderIndex: t.Order})
		if err != nil {
			return err
		}
		resp, err = response(ctx, q, t)
		return err
	})
	return resp, err
}

// Delete: EventTeamService.deleteTeam.
func (s *Service) Delete(ctx context.Context, eventID, teamID string) error {
	if _, err := teamOf(ctx, s.q, eventID, teamID); err != nil {
		return err
	}
	return s.q.DeleteTeam(ctx, teamID)
}

// AddPlayer: EventTeamService.addPlayerToTeam. Se o jogador ja esta no time, o
// papel e sobrescrito (inclusive por null).
func (s *Service) AddPlayer(ctx context.Context, eventID, teamID string, req playerRequest) (teamResponse, error) {
	var resp teamResponse
	err := db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		t, err := teamOf(ctx, q, eventID, teamID)
		if err != nil {
			return err
		}
		if _, err := q.FindUser(ctx, req.UserID); db.NotFound(err) {
			return httpx.BadRequest("User not found")
		} else if err != nil {
			return err
		}
		err = q.UpsertTeamPlayer(ctx, sqlc.UpsertTeamPlayerParams{TeamID: t.ID, UserID: req.UserID, Role: req.Role})
		if err != nil {
			return err
		}
		resp, err = response(ctx, q, t)
		return err
	})
	return resp, err
}

// UpdatePlayer: EventTeamService.updateTeamPlayer. Com newTeamId, move o
// jogador (mantendo o papel se nao vier outro) e devolve o time de DESTINO;
// sem, so troca o papel.
func (s *Service) UpdatePlayer(ctx context.Context, eventID, teamID, userID string, newTeamID, role *string) (teamResponse, error) {
	var resp teamResponse
	err := db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		t, err := teamOf(ctx, q, eventID, teamID)
		if err != nil {
			return err
		}
		player, err := q.FindTeamPlayer(ctx, sqlc.FindTeamPlayerParams{TeamID: teamID, UserID: userID})
		if db.NotFound(err) {
			return httpx.BadRequest("Player not found in this team")
		}
		if err != nil {
			return err
		}
		if role != nil {
			player.Role = role
		}
		if newTeamID != nil && *newTeamID != teamID {
			target, err := q.FindTeam(ctx, *newTeamID)
			if db.NotFound(err) {
				return httpx.BadRequest("Target team not found")
			}
			if err != nil {
				return err
			}
			if target.EventID != eventID {
				return httpx.BadRequest("Target team does not belong to this event")
			}
			if err := q.DeleteTeamPlayer(ctx, sqlc.DeleteTeamPlayerParams{TeamID: teamID, UserID: userID}); err != nil {
				return err
			}
			t = target
		}
		err = q.UpsertTeamPlayer(ctx, sqlc.UpsertTeamPlayerParams{TeamID: t.ID, UserID: userID, Role: player.Role})
		if err != nil {
			return err
		}
		resp, err = response(ctx, q, t)
		return err
	})
	return resp, err
}

// RemovePlayer: EventTeamService.removePlayerFromTeam. Idempotente.
func (s *Service) RemovePlayer(ctx context.Context, eventID, teamID, userID string) (teamResponse, error) {
	var resp teamResponse
	err := db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		t, err := teamOf(ctx, q, eventID, teamID)
		if err != nil {
			return err
		}
		if err := q.DeleteTeamPlayer(ctx, sqlc.DeleteTeamPlayerParams{TeamID: teamID, UserID: userID}); err != nil {
			return err
		}
		resp, err = response(ctx, q, t)
		return err
	})
	return resp, err
}

// AutoGenerate: EventTeamService.autoGenerateTeams. Apaga os times do evento e
// sorteia novos com quem confirmou presenca; quem esta de reserva vai para um
// time "Reserva" a parte. Contas excluidas (username em branco) ficam de fora.
func (s *Service) AutoGenerate(ctx context.Context, eventID string, req autoGenerateRequest) ([]teamResponse, error) {
	out := []teamResponse{}
	err := db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		if exists, err := q.EventExists(ctx, eventID); err != nil {
			return err
		} else if !exists {
			return errEventNotFound
		}
		rsvps, err := q.FindRsvpsByEvent(ctx, eventID)
		if err != nil {
			return err
		}
		active := map[string]bool{}
		if len(rsvps) > 0 {
			userIDs := make([]string, 0, len(rsvps))
			for _, r := range rsvps {
				userIDs = append(userIDs, r.UserID)
			}
			users, err := q.FindUsersByIDs(ctx, userIDs)
			if err != nil {
				return err
			}
			for _, u := range users {
				if !kotlin.IsBlank(u.Username) {
					active[u.ID] = true
				}
			}
		}
		var confirmed, reserves []string
		for _, r := range rsvps {
			switch {
			case !active[r.UserID]:
			case r.Status == "YES":
				confirmed = append(confirmed, r.UserID)
			case r.Status == "RESERVE" && req.IncludeReserves:
				reserves = append(reserves, r.UserID)
			}
		}

		if err := q.DeleteTeamsByEvent(ctx, eventID); err != nil {
			return err
		}
		if len(confirmed) == 0 && len(reserves) == 0 {
			return nil
		}

		rand.Shuffle(len(confirmed), func(i, j int) { confirmed[i], confirmed[j] = confirmed[j], confirmed[i] })

		var created []sqlc.EventTeam
		if n := len(confirmed); n > 0 {
			size, err := chunkSize(n, req)
			if err != nil {
				return err
			}
			for index := 0; index*size < n; index++ {
				players := confirmed[index*size : min((index+1)*size, n)]
				color := teamColors[index%len(teamColors)]
				team := sqlc.EventTeam{
					ID: ids.New(), EventID: eventID, Order: int32(index + 1),
					Name: "Time " + strconv.Itoa(index+1), Color: &color,
				}
				if err := insertTeamWith(ctx, q, team, players); err != nil {
					return err
				}
				created = append(created, team)
			}
		}
		if len(reserves) > 0 {
			color := "#9E9E9E"
			team := sqlc.EventTeam{
				ID: ids.New(), EventID: eventID, Order: int32(len(created) + 1), Name: "Reserva", Color: &color,
			}
			if err := insertTeamWith(ctx, q, team, reserves); err != nil {
				return err
			}
			created = append(created, team)
		}
		out, err = responses(ctx, q, created)
		return err
	})
	return out, err
}

func insertTeamWith(ctx context.Context, q *sqlc.Queries, team sqlc.EventTeam, players []string) error {
	if err := insertTeam(ctx, q, team); err != nil {
		return err
	}
	for _, userID := range players {
		if err := q.UpsertTeamPlayer(ctx, sqlc.UpsertTeamPlayerParams{TeamID: team.ID, UserID: userID}); err != nil {
			return err
		}
	}
	return nil
}

// chunkSize calcula quantos jogadores vao em cada time, pela mesma conta do
// Kotlin -- inclusive nos valores absurdos:
//
//	teamCount ausente: ceil(n / teamSize), com teamSize padrao 5 (e teamSize <= 0 = 1 time)
//	teamCount = 0:     divisao por zero em double da infinito, que vira o maior
//	                   inteiro: um time so, com todos
//	teamCount < 0:     tamanho <= 0, que o chunked() do Kotlin recusa com
//	                   IllegalArgumentException -> 400 com a mensagem dele
func chunkSize(n int, req autoGenerateRequest) (int, error) {
	var teamCount float64
	if req.TeamCount != nil {
		teamCount = float64(*req.TeamCount)
	} else {
		target := 5
		if req.TeamSize != nil {
			target = int(*req.TeamSize)
		}
		teamCount = 1
		if target > 0 {
			teamCount = math.Max(1, math.Ceil(float64(n)/float64(target)))
		}
	}
	raw := math.Ceil(float64(n) / teamCount)
	if math.IsInf(raw, 1) || raw > math.MaxInt32 {
		return math.MaxInt32, nil
	}
	size := int(raw)
	if size <= 0 {
		return 0, httpx.BadRequest("size " + strconv.Itoa(size) + " must be greater than zero.")
	}
	return size, nil
}
