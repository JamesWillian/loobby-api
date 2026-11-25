package gameplays

import (
	"context"
	"log/slog"

	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
	"github.com/JamesWillian/loobby-api/api-go/internal/notifications"
)

// Os pushes da 2.0. O tipo vai em data.type, ao lado de gameplayId e, na do
// grupo, groupId -- e o que o app usa para abrir a tela certa. Os dois
// agendados ficam tambem em notification_dispatch_log, com reference_id = id
// da gameplay: nao renomear sem migrar os registros.
const (
	TypeNewInGroup     = "GAMEPLAY_NEW_IN_GROUP"    // gameplay nova no grupo
	TypeTiebreakOpened = "GAMEPLAY_TIEBREAK_OPENED" // o criador abriu o desempate
	TypeReminder       = "GAMEPLAY_REMINDER"        // 1 h antes do inicio
	TypeTodayPending   = "GAMEPLAY_TODAY_PENDING"   // as 8h, para quem nao respondeu
)

func pushData(g sqlc.Gameplay) map[string]string {
	data := map[string]string{"gameplayId": g.ID}
	if g.GroupID != nil {
		data["groupId"] = *g.GroupID
	}
	return data
}

// notifyNewInGroup avisa os membros do grupo, menos quem criou.
func (s *Service) notifyNewInGroup(g sqlc.Gameplay, groupName string) {
	if g.GroupID == nil {
		return
	}
	s.async("gameplay nova no grupo", func(ctx context.Context) {
		members, err := s.q.FindMemberUserIDs(ctx, *g.GroupID)
		if err != nil {
			slog.Error("push "+TypeNewInGroup+" falhou", "gameplay", g.ID, "err", err)
			return
		}
		var to []string
		for _, id := range members {
			if id != g.CreatorID {
				to = append(to, id)
			}
		}
		if len(to) == 0 {
			return
		}
		s.sender.SendToUsers(ctx, to, notifications.Payload{
			Title: groupName, Body: "Nova gameplay: " + g.Name, Type: TypeNewInGroup, Data: pushData(g),
		})
	})
}

// notifyTiebreak avisa quem marcou Vou, menos quem abriu o desempate.
func (s *Service) notifyTiebreak(g sqlc.Gameplay) {
	s.async("desempate aberto", func(ctx context.Context) {
		rows, err := s.q.ListParticipantsOfGameplays(ctx, []string{g.ID})
		if err != nil {
			slog.Error("push "+TypeTiebreakOpened+" falhou", "gameplay", g.ID, "err", err)
			return
		}
		var to []string
		for _, r := range rows {
			if r.Status == StatusGoing && r.UserID != g.CreatorID {
				to = append(to, r.UserID)
			}
		}
		if len(to) == 0 {
			return
		}
		s.sender.SendToUsers(ctx, to, notifications.Payload{
			Title: g.Name, Body: "Deu empate! Vote no desempate", Type: TypeTiebreakOpened, Data: pushData(g),
		})
	})
}
