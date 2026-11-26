package gameplays

import (
	"context"
	"strings"

	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
	"github.com/JamesWillian/loobby-api/api-go/internal/platform"
)

// Respond e o Vou / Nao vou / Pendente. Passar para Pendente ou Nao vou apaga
// os votos da pessoa na rodada aberta; as rodadas que ja congelaram ficam como
// estavam.
func (s *Service) Respond(ctx context.Context, id, userID, status string) (gameplayResponse, error) {
	return s.mutate(ctx, id, userID, func(q *sqlc.Queries, st state) error {
		if err := st.requireAct(); err != nil {
			return err
		}
		err := q.UpsertParticipant(ctx, sqlc.UpsertParticipantParams{GameplayID: id, UserID: userID, Status: status})
		if err != nil {
			return err
		}
		if status != StatusGoing && st.g.Mode == ModeVoting {
			return clearMyVotes(ctx, q, st)
		}
		return nil
	})
}

func clearMyVotes(ctx context.Context, q *sqlc.Queries, st state) error {
	return q.DeleteMyRoundVotes(ctx, sqlc.DeleteMyRoundVotesParams{
		GameplayID: st.g.ID, VotingRound: int16(st.round()), UserID: st.me,
	})
}

func normalizeCode(code string) string { return strings.ToUpper(strings.TrimSpace(code)) }

// Preview: o que o convite da unica mostra antes de a pessoa entrar.
func (s *Service) Preview(ctx context.Context, code, userID string) (invitePreview, error) {
	g, err := s.q.GetGameplayByInviteCode(ctx, normalizeCode(code))
	if db.NotFound(err) {
		return invitePreview{}, errInviteNotFound
	}
	if err != nil {
		return invitePreview{}, err
	}
	rows, err := s.q.ListParticipantsOfGameplays(ctx, []string{g.ID})
	if err != nil {
		return invitePreview{}, err
	}
	users, err := people(ctx, s.q, []string{g.CreatorID})
	if err != nil {
		return invitePreview{}, err
	}
	now := s.now()
	p := invitePreview{
		ID: g.ID, Name: g.Name, Mode: g.Mode, Platforms: platform.Decode(g.Platforms),
		ScheduledAt: instant(g.ScheduledAt), AutoEndAt: instant(autoEndAt(g)), Status: statusOf(g, now),
		EndedAt: instantPtr(endedAt(g, now)), Creator: refOf(users, g.CreatorID),
	}
	for _, r := range rows {
		if r.Status == StatusGoing {
			p.GoingCount++
		}
		if r.UserID == userID {
			p.Joined = true
		}
	}
	return p, nil
}

// Join: entrar na unica pelo convite, como Pendente. Vale para anonimo, que
// entra e ve tudo. Quem ja estava continua como estava.
func (s *Service) Join(ctx context.Context, code, userID string) (gameplayResponse, error) {
	var resp gameplayResponse
	err := db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		g, err := q.GetGameplayByInviteCode(ctx, normalizeCode(code))
		if db.NotFound(err) {
			return errInviteNotFound
		}
		if err != nil {
			return err
		}
		if g, err = q.LockGameplay(ctx, g.ID); err != nil {
			return err
		}
		_, err = q.GetParticipant(ctx, sqlc.GetParticipantParams{GameplayID: g.ID, UserID: userID})
		switch {
		case db.NotFound(err):
			if endedAt(g, s.now()) != nil {
				return errEnded
			}
			if _, err := q.FindUser(ctx, userID); db.NotFound(err) {
				return errUserNotFound
			} else if err != nil {
				return err
			}
			if err := q.JoinGameplay(ctx, sqlc.JoinGameplayParams{GameplayID: g.ID, UserID: userID}); err != nil {
				return err
			}
		case err != nil:
			return err
		}
		st, err := s.load(ctx, q, g.ID, userID, false)
		if err != nil {
			return err
		}
		resp, err = s.detail(ctx, q, st)
		return err
	})
	return resp, err
}

// Leave: sair da unica. Quem criou nao sai (encerra). Da gameplay do grupo
// ninguem sai: sai-se do grupo.
func (s *Service) Leave(ctx context.Context, id, userID string) error {
	return db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		st, err := s.load(ctx, q, id, userID, true)
		if err != nil {
			return err
		}
		switch {
		case !st.single():
			return errGroupLeave
		case st.creator():
			return errCreatorLeaves
		case !st.active():
			return errEnded
		}
		if st.g.Mode == ModeVoting {
			if err := clearMyVotes(ctx, q, st); err != nil {
				return err
			}
		}
		return q.DeleteParticipant(ctx, sqlc.DeleteParticipantParams{GameplayID: id, UserID: userID})
	})
}
