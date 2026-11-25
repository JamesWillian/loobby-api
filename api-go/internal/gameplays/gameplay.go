package gameplays

import (
	"context"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
)

// Create cria a gameplay do grupo (groupID != nil) ou a unica, que ja nasce
// com o convite. Quem cria entra como Vou. Na do grupo, os outros membros
// recebem o push de gameplay nova.
func (s *Service) Create(ctx context.Context, userID string, groupID *string, in createInput) (gameplayResponse, error) {
	var (
		resp      gameplayResponse
		created   sqlc.Gameplay
		groupName string
	)
	err := db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		u, err := q.FindUser(ctx, userID)
		if db.NotFound(err) {
			return errUserNotFound
		}
		if err != nil {
			return err
		}
		if u.AuthProvider == 0 {
			return errAnonymous
		}
		if groupID != nil {
			group, err := q.FindGroup(ctx, *groupID)
			if db.NotFound(err) {
				return errGroupNotFound
			}
			if err != nil {
				return err
			}
			member, err := q.MemberExists(ctx, sqlc.MemberExistsParams{GroupID: group.ID, UserID: userID})
			if err != nil {
				return err
			}
			if !member {
				return errNotMember
			}
			groupName = group.Name
		}

		p := sqlc.CreateGameplayParams{
			ID: ids.New(), GroupID: groupID, CreatorID: userID, Name: in.Name, Description: in.Description,
			Mode: in.Mode, Platforms: encodePlatforms(in.Platforms),
			ScheduledAt: in.ScheduledAt.Round(time.Microsecond), // a precisao do Postgres
		}
		if groupID == nil {
			code, err := inviteCode(ctx, q)
			if err != nil {
				return err
			}
			p.InviteCode = &code
		}
		switch in.Mode {
		case ModeFixed:
			if in.Game == nil {
				return errNotFixed
			}
			p.GameID, p.GameName = in.Game.GameID, &in.Game.Name
		case ModeVoting:
			p.Finalists, p.MultiVote, p.VotingRound = int16p(in.Finalists), &in.MultiVote, int16p(1)
		}
		if err := q.CreateGameplay(ctx, p); err != nil {
			return err
		}
		err = q.UpsertParticipant(ctx, sqlc.UpsertParticipantParams{GameplayID: p.ID, UserID: userID, Status: StatusGoing})
		if err != nil {
			return err
		}
		for _, gm := range in.Games {
			err := q.AddGameplayGame(ctx, sqlc.AddGameplayGameParams{
				ID: ids.New(), GameplayID: p.ID, GameID: gm.GameID, Name: gm.Name, AddedBy: &userID,
			})
			if isUniqueViolation(err) {
				return errRepeated
			}
			if err != nil {
				return err
			}
		}
		st, err := s.load(ctx, q, p.ID, userID, false)
		if err != nil {
			return err
		}
		created = st.g
		resp, err = s.detail(ctx, q, st)
		return err
	})
	if err != nil {
		return gameplayResponse{}, err
	}
	s.cache(in.Game)
	for i := range in.Games {
		s.cache(&in.Games[i])
	}
	if groupID != nil {
		s.notifyNewInGroup(created, groupName)
	}
	return resp, nil
}

// cache deixa a capa do jogo do catalogo no cache local, em segundo plano.
func (s *Service) cache(g *gameInput) {
	if g != nil && g.GameID != nil {
		s.games.EnsureCached(*g.GameID)
	}
}

func (s *Service) Get(ctx context.Context, id, userID string) (gameplayResponse, error) {
	st, err := s.load(ctx, s.q, id, userID, false)
	if err != nil {
		return gameplayResponse{}, err
	}
	return s.detail(ctx, s.q, st)
}

// mutate e o molde das escritas: trava a gameplay, deixa fn mudar o que for, e
// responde com a gameplay relida dentro da mesma transacao.
func (s *Service) mutate(ctx context.Context, id, userID string, fn func(q *sqlc.Queries, st state) error) (gameplayResponse, error) {
	var resp gameplayResponse
	err := db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		st, err := s.load(ctx, q, id, userID, true)
		if err != nil {
			return err
		}
		if err := fn(q, st); err != nil {
			return err
		}
		if st, err = s.load(ctx, q, id, userID, false); err != nil {
			return err
		}
		resp, err = s.detail(ctx, q, st)
		return err
	})
	return resp, err
}

// Patch edita a gameplay. Nome, descricao, horario, plataforma e o jogo do
// FIXED: quem criou ou o dono do grupo (na unica, so quem criou). Finalistas e
// voto multiplo: so quem criou, e so ate o primeiro voto. O modo nao muda.
func (s *Service) Patch(ctx context.Context, id, userID string, in patchInput) (gameplayResponse, error) {
	general := in.Name != nil || in.Description != nil || in.ClearDescription || in.ScheduledAt != nil ||
		in.Platforms != nil || in.Game != nil
	votingCfg := in.Finalists != nil || in.MultiVote != nil
	resp, err := s.mutate(ctx, id, userID, func(q *sqlc.Queries, st state) error {
		if !general && !votingCfg {
			return nil
		}
		g := st.g
		if !st.active() {
			return errEnded
		}
		if general && !st.manager() {
			return errNotManager
		}
		if in.Game != nil && g.Mode != ModeFixed {
			return errNotFixed
		}
		if votingCfg {
			if g.Mode != ModeVoting {
				return errNoVoting
			}
			if !st.creator() {
				return errNotCreator
			}
			votes, err := q.ListGameplayVotes(ctx, []string{g.ID})
			if err != nil {
				return err
			}
			if st.round() > 1 || len(votes) > 0 {
				return errLocked
			}
		}

		p := sqlc.SetGameplayDetailsParams{
			ID: g.ID, Name: g.Name, Description: g.Description, ScheduledAt: g.ScheduledAt,
			Platforms: g.Platforms, GameID: g.GameID, GameName: g.GameName,
			Finalists: g.Finalists, MultiVote: g.MultiVote,
		}
		if in.Name != nil {
			p.Name = *in.Name
		}
		if in.ClearDescription {
			p.Description = nil
		} else if in.Description != nil {
			p.Description = in.Description
		}
		if in.ScheduledAt != nil {
			p.ScheduledAt = in.ScheduledAt.Round(time.Microsecond)
		}
		if in.Platforms != nil {
			p.Platforms = encodePlatforms(in.Platforms)
		}
		if in.Game != nil {
			p.GameID, p.GameName = in.Game.GameID, &in.Game.Name
		}
		if in.Finalists != nil {
			p.Finalists = int16p(*in.Finalists)
		}
		if in.MultiVote != nil {
			p.MultiVote = in.MultiVote
		}
		return q.SetGameplayDetails(ctx, p)
	})
	if err != nil {
		return gameplayResponse{}, err
	}
	s.cache(in.Game)
	return resp, nil
}

// End e o Encerrar: quem criou ou o dono do grupo (na unica, so quem criou).
// Encerrada, a gameplay fica so para leitura.
func (s *Service) End(ctx context.Context, id, userID string) (gameplayResponse, error) {
	return s.mutate(ctx, id, userID, func(q *sqlc.Queries, st state) error {
		if err := st.requireManager(); err != nil {
			return err
		}
		now := st.now
		return q.EndGameplay(ctx, sqlc.EndGameplayParams{ID: st.g.ID, EndedAt: &now, EndedBy: &userID})
	})
}

// ListGroup: as ativas do grupo, da mais proxima para a mais distante; ou as
// encerradas, da que encerrou por ultimo, em paginas (before = a ultima
// gameplay da pagina anterior).
func (s *Service) ListGroup(ctx context.Context, userID, groupID string, ended bool, before *string, limit int) ([]gameplaySummary, error) {
	group, err := s.q.FindGroup(ctx, groupID)
	if db.NotFound(err) {
		return nil, errGroupNotFound
	}
	if err != nil {
		return nil, err
	}
	member, err := s.q.MemberExists(ctx, sqlc.MemberExistsParams{GroupID: group.ID, UserID: userID})
	if err != nil {
		return nil, err
	}
	if !member {
		return nil, errNotMember
	}
	now := s.now()
	cutoff := now.Add(-AutoEndAfter)
	var list []sqlc.Gameplay
	switch {
	case !ended:
		list, err = s.q.ListActiveGroupGameplays(ctx, sqlc.ListActiveGroupGameplaysParams{GroupID: groupID, Cutoff: cutoff})
	case before == nil:
		list, err = s.q.ListEndedGroupGameplays(ctx, sqlc.ListEndedGroupGameplaysParams{
			GroupID: groupID, Cutoff: cutoff, AfterSeconds: autoEndSeconds, MaxRows: int32(limit),
		})
	default:
		cursor, err := s.q.GetGameplay(ctx, *before)
		if db.NotFound(err) {
			return nil, errBadCursor
		}
		if err != nil {
			return nil, err
		}
		end := endedAt(cursor, now)
		if cursor.GroupID == nil || *cursor.GroupID != groupID || end == nil {
			return nil, errBadCursor
		}
		list, err = s.q.ListEndedGroupGameplaysBefore(ctx, sqlc.ListEndedGroupGameplaysBeforeParams{
			GroupID: groupID, Cutoff: cutoff, AfterSeconds: autoEndSeconds,
			BeforeAt: *end, BeforeID: cursor.ID, MaxRows: int32(limit),
		})
		if err != nil {
			return nil, err
		}
	}
	if err != nil {
		return nil, err
	}
	return s.summaries(ctx, s.q, list, userID, now)
}

// Lobby: as gameplays ativas do usuario -- as dos grupos dele e as unicas em
// que entrou --, da mais proxima para a mais distante.
func (s *Service) Lobby(ctx context.Context, userID string) ([]gameplaySummary, error) {
	now := s.now()
	list, err := s.q.ListLobbyGameplays(ctx, sqlc.ListLobbyGameplaysParams{Cutoff: now.Add(-AutoEndAfter), UserID: userID})
	if err != nil {
		return nil, err
	}
	return s.summaries(ctx, s.q, list, userID, now)
}

// autoEndSeconds: AutoEndAfter como o SQL recebe.
const autoEndSeconds = int32(AutoEndAfter / time.Second)
