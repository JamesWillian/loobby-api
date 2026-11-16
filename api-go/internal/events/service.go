package events

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
	"github.com/JamesWillian/loobby-api/api-go/internal/jackson"
	"github.com/JamesWillian/loobby-api/api-go/internal/kotlin"
)

type Service struct {
	pool     *pgxpool.Pool
	q        *sqlc.Queries
	notifier Notifier
	games    GameCache
}

func NewService(pool *pgxpool.Pool, notifier Notifier, games GameCache) *Service {
	return &Service{pool: pool, q: sqlc.New(pool), notifier: notifier, games: games}
}

var errEventNotFound = httpx.BadRequest("Event not found")

func findEvent(ctx context.Context, q *sqlc.Queries, id string) (sqlc.Event, error) {
	e, err := q.FindEvent(ctx, id)
	if db.NotFound(err) {
		return sqlc.Event{}, errEventNotFound
	}
	return e, err
}

// Create: EventService.createGroupEvent (groupID != nil) e createInstantEvent.
//
// Diferenca deliberada em relacao ao Kotlin, invisivel na resposta: la o evento
// e gravado ANTES de validar os detalhes do tipo e fora de transacao, entao um
// pedido recusado ("durationMinutes must be > 0", ou a restricao do banco para
// maxPlayers = 0) deixa para tras um evento orfao, sem os detalhes. Aqui a
// validacao vem antes e evento + detalhes sao gravados juntos: a resposta e a
// mesma, e nao sobra lixo (contract/ACHADOS.md).
func (s *Service) Create(ctx context.Context, ownerID string, groupID *string, req createRequest) (eventResponse, error) {
	if groupID != nil {
		if _, err := s.q.FindGroup(ctx, *groupID); db.NotFound(err) {
			return eventResponse{}, httpx.BadRequest("Group not found")
		} else if err != nil {
			return eventResponse{}, err
		}
	}
	switch req.EventType {
	case "GAMEPLAY":
		if req.Gameplay == nil {
			return eventResponse{}, httpx.BadRequest("Gameplay details are required for GAMEPLAY events")
		}
		if kotlin.IsBlank(req.Gameplay.GameName) {
			return eventResponse{}, httpx.BadRequest("Gameplay event must have gameName")
		}
	case "SPORT":
		if req.Sport == nil {
			return eventResponse{}, httpx.BadRequest("Sport details are required for SPORT events")
		}
		if req.Sport.DurationMinutes <= 0 {
			return eventResponse{}, httpx.BadRequest("durationMinutes must be > 0")
		}
	}

	code, err := s.inviteCode(ctx)
	if err != nil {
		return eventResponse{}, err
	}
	event := sqlc.Event{
		ID: ids.New(), EventType: req.EventType, GroupID: groupID, IsInstant: groupID == nil,
		OwnerID: ownerID, ScheduledDatetime: req.ScheduledDatetime, Name: req.Name,
		Description: req.Description, InviteCode: code,
	}
	err = db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		err := q.InsertEvent(ctx, sqlc.InsertEventParams{
			ID: event.ID, EventType: event.EventType, GroupID: event.GroupID, IsInstant: event.IsInstant,
			OwnerID: event.OwnerID, Name: event.Name, Description: event.Description, InviteCode: event.InviteCode,
			// O driver do Kotlin arredonda para microssegundos; o pgx truncaria.
			ScheduledDatetime: event.ScheduledDatetime.Round(time.Microsecond),
		})
		if err != nil {
			return err
		}
		switch req.EventType {
		case "GAMEPLAY":
			return q.InsertGameplay(ctx, sqlc.InsertGameplayParams{
				EventID: event.ID, GameID: req.Gameplay.GameID, GameName: req.Gameplay.GameName,
			})
		case "SPORT":
			price := "0"
			if req.Sport.PricePerPlayer != nil {
				price = *req.Sport.PricePerPlayer
			}
			return q.InsertSport(ctx, sqlc.InsertSportParams{
				EventID: event.ID, DurationMinutes: req.Sport.DurationMinutes, Arena: req.Sport.Arena,
				PricePerPlayer: price, MaxPlayers: req.Sport.MaxPlayers,
				AcceptReserve: req.Sport.AcceptReserve != nil && *req.Sport.AcceptReserve,
			})
		}
		return nil
	})
	if err != nil {
		return eventResponse{}, err
	}

	if req.EventType == "GAMEPLAY" && req.Gameplay.GameID != nil {
		s.games.EnsureCached(*req.Gameplay.GameID)
	}
	if groupID != nil {
		s.notifier.OnGroupEventCreated(event, ownerID)
	}
	// A resposta sai da entidade recem-salva: createdAt null, e a data com a
	// precisao que o cliente mandou (nao a que o banco guardou).
	return s.response(ctx, s.q, event, view{newlyCreated: true})
}

// inviteCode: "$-XXXX-XXXX". (No Kotlin a checagem de existencia procura o
// codigo sem o prefixo "$-" e por isso nunca acha; aqui usa o codigo completo.)
func (s *Service) inviteCode(ctx context.Context) (string, error) {
	for {
		code := "$-" + ids.Code(4) + "-" + ids.Code(4)
		exists, err := s.q.EventInviteCodeExists(ctx, code)
		if err != nil {
			return "", err
		}
		if !exists {
			return code, nil
		}
	}
}

// view sao os dados da resposta que nao estao na linha do evento.
type view struct {
	newlyCreated     bool // createdAt sai null
	rsvpStatus       *string
	confirmedCount   int
	confirmedAvatars []*string
	sportPrice       *string // texto do pricePerPlayer quando ele veio do pedido
}

// response: EventService.buildResponse. Os detalhes do tipo sao lidos do banco.
func (s *Service) response(ctx context.Context, q *sqlc.Queries, e sqlc.Event, v view) (eventResponse, error) {
	resp := eventResponse{
		ID: e.ID, EventType: e.EventType, GroupID: e.GroupID, IsInstant: e.IsInstant, OwnerID: e.OwnerID,
		ScheduledDatetime: httpx.Instant(e.ScheduledDatetime), Name: e.Name, Description: e.Description,
		InviteCode: e.InviteCode, RsvpStatus: v.rsvpStatus, ConfirmedCount: v.confirmedCount,
		ConfirmedAvatars: v.confirmedAvatars,
	}
	if !v.newlyCreated {
		t := httpx.Instant(e.CreatedAt)
		resp.CreatedAt = &t
	}
	switch e.EventType {
	case "GAMEPLAY":
		g, err := q.FindGameplay(ctx, e.ID)
		if err == nil {
			resp.Gameplay = &gameplayResponse{GameID: g.GameID, GameName: g.GameName}
		} else if !db.NotFound(err) {
			return eventResponse{}, err
		}
	case "SPORT":
		sp, err := q.FindSport(ctx, e.ID)
		if err == nil {
			price := sp.PricePerPlayer
			if v.sportPrice != nil {
				price = *v.sportPrice
			}
			resp.Sport = &sportResponse{
				DurationMinutes: sp.DurationMinutes, Arena: sp.Arena, PricePerPlayer: httpx.Number(price),
				MaxPlayers: sp.MaxPlayers, AcceptReserve: sp.AcceptReserve,
			}
		} else if !db.NotFound(err) {
			return eventResponse{}, err
		}
	}
	return resp, nil
}

// confirmed monta, para um conjunto de eventos, a contagem de confirmados e os
// avatares dos cinco primeiros. "Primeiros" na ordem em que o banco devolve --
// a consulta do Kotlin nao tem ORDER BY, e aqui tambem nao.
func (s *Service) confirmed(ctx context.Context, eventIDs []string) (map[string][]sqlc.EventRsvp, map[string]sqlc.User, error) {
	yes, err := s.q.FindRsvpsByEventsAndStatus(ctx, sqlc.FindRsvpsByEventsAndStatusParams{
		EventIds: eventIDs, Status: "YES",
	})
	if err != nil {
		return nil, nil, err
	}
	byEvent := map[string][]sqlc.EventRsvp{}
	for _, r := range yes {
		byEvent[r.EventID] = append(byEvent[r.EventID], r)
	}
	var top []string
	seen := map[string]bool{}
	for _, list := range byEvent {
		for i, r := range list {
			if i == 5 {
				break
			}
			if !seen[r.UserID] {
				seen[r.UserID] = true
				top = append(top, r.UserID)
			}
		}
	}
	users := map[string]sqlc.User{}
	if len(top) > 0 {
		rows, err := s.q.FindUsersByIDs(ctx, top)
		if err != nil {
			return nil, nil, err
		}
		for _, u := range rows {
			users[u.ID] = u
		}
	}
	return byEvent, users, nil
}

// avatars: os avatares dos cinco primeiros confirmados; null quando ninguem
// confirmou. Quem nao tem avatar entra como null na lista.
func avatars(yes []sqlc.EventRsvp, users map[string]sqlc.User) []*string {
	if len(yes) == 0 {
		return nil
	}
	n := min(len(yes), 5)
	out := make([]*string, 0, n)
	for _, r := range yes[:n] {
		out = append(out, users[r.UserID].AvatarUrl) // usuario ausente: valor zero, avatar nil
	}
	return out
}

// ListByGroup: EventService.listGroupEvents. Grupo inexistente devolve lista
// vazia -- o Kotlin nao confere o grupo aqui.
func (s *Service) ListByGroup(ctx context.Context, groupID, userID string) ([]eventResponse, error) {
	events, err := s.q.FindEventsByGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	out := make([]eventResponse, 0, len(events))
	if len(events) == 0 {
		return out, nil
	}
	eventIDs := make([]string, 0, len(events))
	for _, e := range events {
		eventIDs = append(eventIDs, e.ID)
	}
	mine, err := s.q.FindRsvpsByEventsAndUser(ctx, sqlc.FindRsvpsByEventsAndUserParams{EventIds: eventIDs, UserID: userID})
	if err != nil {
		return nil, err
	}
	myStatus := map[string]*string{}
	for _, r := range mine {
		myStatus[r.EventID] = &r.Status
	}
	yes, users, err := s.confirmed(ctx, eventIDs)
	if err != nil {
		return nil, err
	}
	for _, e := range events {
		resp, err := s.response(ctx, s.q, e, view{
			rsvpStatus: myStatus[e.ID], confirmedCount: len(yes[e.ID]),
			confirmedAvatars: avatars(yes[e.ID], users),
		})
		if err != nil {
			return nil, err
		}
		out = append(out, resp)
	}
	return out, nil
}

// detail monta a resposta de um evento para um usuario: o RSVP dele, a
// contagem de confirmados e os avatares.
func (s *Service) detail(ctx context.Context, e sqlc.Event, userID string) (eventResponse, error) {
	v := view{}
	rsvp, err := s.q.FindRsvp(ctx, sqlc.FindRsvpParams{EventID: e.ID, UserID: userID})
	if err == nil {
		v.rsvpStatus = &rsvp.Status
	} else if !db.NotFound(err) {
		return eventResponse{}, err
	}
	yes, users, err := s.confirmed(ctx, []string{e.ID})
	if err != nil {
		return eventResponse{}, err
	}
	v.confirmedCount = len(yes[e.ID])
	v.confirmedAvatars = avatars(yes[e.ID], users)
	return s.response(ctx, s.q, e, v)
}

// Get: EventService.getEventById.
func (s *Service) Get(ctx context.Context, eventID, userID string) (eventResponse, error) {
	e, err := findEvent(ctx, s.q, eventID)
	if err != nil {
		return eventResponse{}, err
	}
	return s.detail(ctx, e, userID)
}

// GetByInviteCode: EventService.getByInviteCode.
func (s *Service) GetByInviteCode(ctx context.Context, code, userID string) (eventResponse, error) {
	e, err := s.q.FindEventByInviteCode(ctx, code)
	if db.NotFound(err) {
		return eventResponse{}, httpx.BadRequest("Invalid Invite Code")
	}
	if err != nil {
		return eventResponse{}, err
	}
	return s.detail(ctx, e, userID)
}

// assertCanManage: so o criador do evento ou o dono do grupo.
func assertCanManage(ctx context.Context, q *sqlc.Queries, userID string, e sqlc.Event) error {
	if e.OwnerID == userID {
		return nil
	}
	if e.GroupID != nil {
		g, err := q.FindGroup(ctx, *e.GroupID)
		if err == nil && g.OwnerID == userID {
			return nil
		}
		if err != nil && !db.NotFound(err) {
			return err
		}
	}
	return httpx.Forbidden("Only the event creator or the group owner can manage this event")
}

// Update: EventService.updateEvent. Tipo, grupo e "instantaneo" nao mudam; os
// detalhes so sao aplicados se forem do tipo do evento.
//
// A resposta nao traz RSVP nem confirmados (rsvpStatus null, confirmedCount 0),
// como no Kotlin. E um pricePerPlayer enviado no pedido volta como foi escrito
// ("30", nao "30.00"): o Kotlin responde com o valor em memoria, antes de o
// banco aplicar a escala da coluna.
func (s *Service) Update(ctx context.Context, userID, eventID string, req updateRequest) (eventResponse, error) {
	var resp eventResponse
	var cacheGame *string
	err := db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		e, err := findEvent(ctx, q, eventID)
		if err != nil {
			return err
		}
		if err := assertCanManage(ctx, q, userID, e); err != nil {
			return err
		}
		if req.Name != nil {
			e.Name = *req.Name
		}
		if req.ClearDescription {
			e.Description = nil
		} else if req.Description != nil {
			e.Description = req.Description
		}
		if req.ScheduledDatetime != nil {
			e.ScheduledDatetime = *req.ScheduledDatetime
		}
		err = q.UpdateEvent(ctx, sqlc.UpdateEventParams{
			ID: e.ID, Name: e.Name, Description: e.Description,
			ScheduledDatetime: e.ScheduledDatetime.Round(time.Microsecond),
		})
		if err != nil {
			return err
		}

		v := view{}
		switch {
		case e.EventType == "GAMEPLAY" && req.Gameplay != nil:
			g, err := q.FindGameplay(ctx, e.ID)
			if db.NotFound(err) {
				return httpx.BadRequest("Gameplay details not found for event " + e.ID)
			}
			if err != nil {
				return err
			}
			if !kotlin.IsBlank(req.Gameplay.GameName) {
				g.GameName = req.Gameplay.GameName
			}
			if req.Gameplay.GameID != nil {
				g.GameID = req.Gameplay.GameID
				cacheGame = req.Gameplay.GameID
			}
			err = q.UpdateGameplay(ctx, sqlc.UpdateGameplayParams{EventID: e.ID, GameID: g.GameID, GameName: g.GameName})
			if err != nil {
				return err
			}
		case e.EventType == "SPORT" && req.Sport != nil:
			sp, err := q.FindSport(ctx, e.ID)
			if db.NotFound(err) {
				return httpx.BadRequest("Sport details not found for event " + e.ID)
			}
			if err != nil {
				return err
			}
			if req.Sport.DurationMinutes > 0 {
				sp.DurationMinutes = req.Sport.DurationMinutes
			}
			if req.Sport.Arena != nil {
				sp.Arena = req.Sport.Arena
			}
			if req.Sport.PricePerPlayer != nil {
				sp.PricePerPlayer = *req.Sport.PricePerPlayer
				text := jackson.BigDecimalString(*req.Sport.PricePerPlayer)
				v.sportPrice = &text
			}
			if req.Sport.MaxPlayers != nil {
				sp.MaxPlayers = req.Sport.MaxPlayers
			}
			if req.Sport.AcceptReserve != nil {
				sp.AcceptReserve = *req.Sport.AcceptReserve
			}
			err = q.UpdateSport(ctx, sqlc.UpdateSportParams{
				EventID: e.ID, DurationMinutes: sp.DurationMinutes, Arena: sp.Arena,
				PricePerPlayer: sp.PricePerPlayer, MaxPlayers: sp.MaxPlayers, AcceptReserve: sp.AcceptReserve,
			})
			if err != nil {
				return err
			}
		}
		resp, err = s.response(ctx, q, e, v)
		return err
	})
	if err != nil {
		return eventResponse{}, err
	}
	if cacheGame != nil {
		s.games.EnsureCached(*cacheGame)
	}
	return resp, nil
}

// Delete: EventService.deleteEvent.
func (s *Service) Delete(ctx context.Context, userID, eventID string) error {
	e, err := findEvent(ctx, s.q, eventID)
	if err != nil {
		return err
	}
	if err := assertCanManage(ctx, s.q, userID, e); err != nil {
		return err
	}
	return s.q.DeleteEvent(ctx, eventID)
}
