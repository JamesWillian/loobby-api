package gameplays

import (
	"context"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
)

// DeleteUserData e a parte da 2.0 na exclusao de conta. Roda dentro da
// transacao do users.DeleteAccount, depois que os grupos do usuario ja
// passaram para outro membro (ou sumiram, levando as gameplays junto).
//
// A exclusao so anonimiza a linha de users, entao o que e da pessoa sai aqui:
// votos (de todas as rodadas), comentarios (as denuncias deles vao junto),
// denuncias que ela fez e participacoes. Quem herda o que ela criou:
//
//   - gameplay do grupo: o dono do grupo;
//   - unica: o participante mais antigo que nao e anonimo; sem nenhum, a unica
//     e apagada, como o evento instantaneo da v1.
func DeleteUserData(ctx context.Context, q *sqlc.Queries, userID string) error {
	created, err := q.ListGameplaysByCreator(ctx, userID)
	if err != nil {
		return err
	}
	for _, g := range created {
		var heir string
		if g.GroupID != nil {
			group, err := q.FindGroup(ctx, *g.GroupID)
			if err != nil {
				return err
			}
			heir = group.OwnerID
		} else {
			heir, err = q.FindHeirParticipant(ctx, sqlc.FindHeirParticipantParams{GameplayID: g.ID, UserID: userID})
			if db.NotFound(err) {
				if err := q.DeleteGameplay(ctx, g.ID); err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return err
			}
		}
		if heir == userID {
			continue // dono do grupo ainda e ele: o DeleteAccount ja trata o grupo antes
		}
		if err := q.SetGameplayCreator(ctx, sqlc.SetGameplayCreatorParams{ID: g.ID, CreatorID: heir}); err != nil {
			return err
		}
	}
	for _, del := range []func(context.Context, string) error{
		q.DeleteVotesByUser, q.DeleteCommentsByUser, q.DeleteReportsByUser,
		q.DeleteParticipationsByUser, q.ClearGameplayGamesAddedBy,
	} {
		if err := del(ctx, userID); err != nil {
			return err
		}
	}
	return nil
}

// MemberLeft: quem sai ou e removido de um grupo perde a participacao e os
// votos da rodada aberta nas gameplays ativas daquele grupo. As encerradas
// ficam como estavam (sao historico).
func MemberLeft(ctx context.Context, q *sqlc.Queries, groupID, userID string, now time.Time) error {
	active, err := q.ListActiveGroupGameplays(ctx, sqlc.ListActiveGroupGameplaysParams{
		GroupID: groupID, Cutoff: now.Add(-AutoEndAfter),
	})
	if err != nil || len(active) == 0 {
		return err
	}
	ids := make([]string, 0, len(active))
	for _, g := range active {
		ids = append(ids, g.ID)
	}
	err = q.DeleteOpenRoundVotesInGameplays(ctx, sqlc.DeleteOpenRoundVotesInGameplaysParams{UserID: userID, GameplayIds: ids})
	if err != nil {
		return err
	}
	return q.DeleteParticipationInGameplays(ctx, sqlc.DeleteParticipationInGameplaysParams{UserID: userID, GameplayIds: ids})
}
