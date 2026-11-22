package notifications

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
)

// Tipos de notificacao. O nome e gravado em notification_dispatch_log como
// chave de idempotencia diaria -- nao renomear sem migrar os dados.
const (
	TypeRsvpConfirmedByPeer      = "RSVP_CONFIRMED_BY_PEER"
	TypeEventTodayPendingRsvp    = "EVENT_TODAY_PENDING_RSVP"
	TypeEventReminderHoursBefore = "EVENT_REMINDER_HOURS_BEFORE"
	TypeNewGroupEvent            = "NEW_GROUP_EVENT"
)

// Valor de loobby.notifications.rsvp-peer-debounce-minutes no application.yaml.
const peerDebounce = 5 * time.Minute

// Service: NotificationService. Decide quem recebe o que; a entrega e do Sender.
type Service struct {
	q         *sqlc.Queries
	sender    Sender
	debouncer *debouncer
}

func NewService(dbtx sqlc.DBTX, sender Sender) *Service {
	return &Service{q: sqlc.New(dbtx), sender: sender, debouncer: newDebouncer(peerDebounce)}
}

// background roda um gancho de notificacao fora da requisicao que o originou.
// No Kotlin esses ganchos rodam dentro da requisicao e engolem qualquer erro;
// o efeito para o cliente e o mesmo, sem fazer a resposta esperar pelo Firebase.
func background(name string, fn func(ctx context.Context)) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("notificacao: panico", "gancho", name, "erro", r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		fn(ctx)
	}()
}

func eventData(e sqlc.Event) map[string]string {
	data := map[string]string{"eventId": e.ID}
	if e.GroupID != nil {
		data["groupId"] = *e.GroupID
	}
	return data
}

// OnRsvpConfirmed: alguem confirmou presenca; avisa quem ja tinha confirmado
// no mesmo evento, agrupando por destinatario (ver debouncer).
func (s *Service) OnRsvpConfirmed(event sqlc.Event, actorID string) {
	background("rsvp confirmado", func(ctx context.Context) {
		rsvps, err := s.q.FindRsvpsByEventsAndStatus(ctx, sqlc.FindRsvpsByEventsAndStatusParams{
			EventIds: []string{event.ID}, Status: "YES",
		})
		if err != nil {
			slog.Error("notificacao RSVP_CONFIRMED_BY_PEER falhou", "evento", event.ID, "err", err)
			return
		}
		peers := map[string]bool{}
		for _, r := range rsvps {
			if r.UserID != actorID {
				peers[r.UserID] = true
			}
		}
		if len(peers) == 0 {
			return
		}
		actor, err := s.q.FindUser(ctx, actorID)
		if err != nil {
			if !db.NotFound(err) {
				slog.Error("notificacao RSVP_CONFIRMED_BY_PEER falhou", "evento", event.ID, "err", err)
			}
			return
		}
		for recipient := range peers {
			s.debouncer.schedule(event.ID, recipient, actorID, actor.Displayname, func(names []string, extras int) {
				data := eventData(event)
				data["actorId"] = actorID
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				s.sender.SendToUsers(ctx, []string{recipient}, Payload{
					Title: event.Name, Body: peerRsvpMessage(names, extras),
					Type: TypeRsvpConfirmedByPeer, Data: data,
				})
			})
		}
	})
}

func peerRsvpMessage(names []string, extras int) string {
	switch {
	case len(names) == 1 && extras == 0:
		return names[0] + " confirmou presença"
	case len(names) == 2 && extras == 0:
		return names[0] + " e " + names[1] + " confirmaram presença"
	case len(names) >= 2 && extras > 0:
		return names[0] + ", " + names[1] + " e mais " + strconv.Itoa(extras) + " confirmaram presença"
	case len(names) == 1 && extras > 0:
		return names[0] + " e mais " + strconv.Itoa(extras) + " confirmaram presença"
	default:
		return "Novas confirmações no evento"
	}
}

// OnGroupEventCreated: evento novo num grupo; avisa os membros, menos quem criou.
func (s *Service) OnGroupEventCreated(event sqlc.Event, creatorID string) {
	if event.GroupID == nil {
		return
	}
	groupID := *event.GroupID
	background("evento de grupo criado", func(ctx context.Context) {
		group, err := s.q.FindGroup(ctx, groupID)
		if err != nil {
			if !db.NotFound(err) {
				slog.Error("notificacao NEW_GROUP_EVENT falhou", "evento", event.ID, "err", err)
			}
			return
		}
		members, err := s.q.FindMemberUserIDs(ctx, groupID)
		if err != nil {
			slog.Error("notificacao NEW_GROUP_EVENT falhou", "evento", event.ID, "err", err)
			return
		}
		var recipients []string
		for _, id := range members {
			if id != creatorID {
				recipients = append(recipients, id)
			}
		}
		if len(recipients) == 0 {
			return
		}
		s.sender.SendToUsers(ctx, recipients, Payload{
			Title: group.Name, Body: "Novo evento: " + event.Name, Type: TypeNewGroupEvent,
			Data: map[string]string{"eventId": event.ID, "groupId": groupID},
		})
	})
}

func (s *Service) sendEventTodayPendingRsvp(ctx context.Context, userID string, e sqlc.Event) {
	s.sender.SendToUsers(ctx, []string{userID}, Payload{
		Title: "Evento hoje", Body: e.Name + " é hoje — você ainda não confirmou presença",
		Type: TypeEventTodayPendingRsvp, Data: eventData(e),
	})
}

func (s *Service) sendEventReminder(ctx context.Context, userID string, e sqlc.Event, hoursBefore int) {
	s.sender.SendToUsers(ctx, []string{userID}, Payload{
		Title: e.Name, Body: "Começa em " + strconv.Itoa(hoursBefore) + " horas — se prepare!",
		Type: TypeEventReminderHoursBefore, Data: eventData(e),
	})
}
