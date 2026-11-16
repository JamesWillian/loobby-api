package notifications

import (
	"context"
	"log/slog"
	"math/big"
	"time"
	_ "time/tzdata" // America/Sao_Paulo mesmo em maquina sem o banco de fusos

	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
)

// Valores do bloco loobby.notifications do application.yaml do Kotlin.
const (
	schedulerZone       = "America/Sao_Paulo"
	dailyJobHour        = 8               // daily-job-cron: "0 0 8 * * *"
	reminderJobInterval = 5 * time.Minute // reminder-job-cron: "0 */5 * * * *"
	reminderHoursBefore = 4
	reminderWindow      = 15 * time.Minute
	paymentDueLookahead = 30 * 24 * time.Hour
)

// Scheduler: NotificationScheduler. Dois jobs:
//
//   - todo dia as 8h (America/Sao_Paulo): "evento hoje, voce nao confirmou" e
//     "pagamento pendente";
//   - a cada 5 minutos: lembrete de evento confirmado que comeca em ~4 horas.
//
// Nenhum push sai duas vezes: cada envio e registrado antes em
// notification_dispatch_log, que tem restricao unica por (usuario, tipo,
// evento, dia). Isso vale tambem ENTRE processos -- com Kotlin e Go rodando os
// jobs ao mesmo tempo durante o strangler, so um dos dois envia.
type Scheduler struct {
	q   *sqlc.Queries
	svc *Service
	loc *time.Location
	now func() time.Time
}

func NewScheduler(dbtx sqlc.DBTX, svc *Service) (*Scheduler, error) {
	loc, err := time.LoadLocation(schedulerZone)
	if err != nil {
		return nil, err
	}
	return &Scheduler{q: sqlc.New(dbtx), svc: svc, loc: loc, now: time.Now}, nil
}

// Run bloqueia ate o contexto ser cancelado.
func (s *Scheduler) Run(ctx context.Context) {
	go s.loop(ctx, s.nextDaily, s.dailyMorningJob)
	s.loop(ctx, s.nextReminder, s.reminderJob)
}

func (s *Scheduler) loop(ctx context.Context, next func(time.Time) time.Time, job func(context.Context)) {
	for {
		wait := time.NewTimer(time.Until(next(s.now())))
		select {
		case <-ctx.Done():
			wait.Stop()
			return
		case <-wait.C:
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("job de notificacao: panico", "erro", r)
				}
			}()
			jobCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			defer cancel()
			job(jobCtx)
		}()
	}
}

func (s *Scheduler) nextDaily(now time.Time) time.Time {
	local := now.In(s.loc)
	next := time.Date(local.Year(), local.Month(), local.Day(), dailyJobHour, 0, 0, 0, s.loc)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func (s *Scheduler) nextReminder(now time.Time) time.Time {
	return now.Truncate(reminderJobInterval).Add(reminderJobInterval)
}

// date: o dia local como parametro `date` do Postgres.
func date(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func (s *Scheduler) dailyMorningJob(ctx context.Context) {
	today := s.now().In(s.loc)
	slog.Info("job diario de notificacoes", "dia", today.Format("2006-01-02"), "fuso", schedulerZone)
	if err := s.dispatchEventTodayPendingRsvp(ctx, today); err != nil {
		slog.Error("EVENT_TODAY_PENDING_RSVP falhou", "err", err)
	}
	if err := s.dispatchPaymentDueDaily(ctx, today); err != nil {
		slog.Error("PAYMENT_DUE_DAILY falhou", "err", err)
	}
}

// Evento de grupo que acontece hoje: avisa os membros que nao responderam nem
// YES nem NO.
func (s *Scheduler) dispatchEventTodayPendingRsvp(ctx context.Context, today time.Time) error {
	start := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, s.loc)
	end := time.Date(today.Year(), today.Month(), today.Day(), 23, 59, 59, 999_999_999, s.loc)
	events, err := s.q.FindEventsBetween(ctx, sqlc.FindEventsBetweenParams{StartAt: start, EndAt: end})
	if err != nil {
		return err
	}
	for _, e := range events {
		if e.GroupID == nil {
			continue // evento instantaneo nao tem membros a avisar
		}
		members, err := s.q.FindMemberUserIDs(ctx, *e.GroupID)
		if err != nil {
			return err
		}
		rsvps, err := s.q.FindRsvpsByEvent(ctx, e.ID)
		if err != nil {
			return err
		}
		answered := map[string]bool{}
		for _, r := range rsvps {
			if r.Status == "YES" || r.Status == "NO" {
				answered[r.UserID] = true
			}
		}
		seen := map[string]bool{}
		for _, userID := range members {
			if answered[userID] || seen[userID] {
				continue
			}
			seen[userID] = true
			s.tryDispatch(ctx, userID, TypeEventTodayPendingRsvp, e.ID, date(today), func() {
				s.svc.sendEventTodayPendingRsvp(ctx, userID, e)
			})
		}
	}
	return nil
}

// Evento pago nos proximos 30 dias: avisa, uma vez por dia, quem confirmou e
// ainda nao pagou.
func (s *Scheduler) dispatchPaymentDueDaily(ctx context.Context, today time.Time) error {
	now := s.now()
	events, err := s.q.FindEventsBetween(ctx, sqlc.FindEventsBetweenParams{StartAt: now, EndAt: now.Add(paymentDueLookahead)})
	if err != nil || len(events) == 0 {
		return err
	}
	byID := make(map[string]sqlc.Event, len(events))
	ids := make([]string, 0, len(events))
	for _, e := range events {
		byID[e.ID] = e
		ids = append(ids, e.ID)
	}
	sports, err := s.q.FindSportsByEvents(ctx, ids)
	if err != nil {
		return err
	}
	var paid []string
	for _, sp := range sports {
		if price, ok := new(big.Rat).SetString(sp.PricePerPlayer); ok && price.Sign() > 0 {
			paid = append(paid, sp.EventID)
		}
	}
	if len(paid) == 0 {
		return nil
	}
	pending, err := s.q.FindUnpaidRsvps(ctx, sqlc.FindUnpaidRsvpsParams{EventIds: paid, Status: "YES"})
	if err != nil {
		return err
	}
	for _, r := range pending {
		e := byID[r.EventID]
		s.tryDispatch(ctx, r.UserID, TypePaymentDueDaily, e.ID, date(today), func() {
			s.svc.sendPaymentDue(ctx, r.UserID, e)
		})
	}
	return nil
}

// reminderJob: eventos que comecam entre daqui a 4h e daqui a 4h15.
func (s *Scheduler) reminderJob(ctx context.Context) {
	start := s.now().Add(reminderHoursBefore * time.Hour)
	events, err := s.q.FindEventsBetween(ctx, sqlc.FindEventsBetweenParams{StartAt: start, EndAt: start.Add(reminderWindow)})
	if err != nil {
		slog.Error("EVENT_REMINDER_HOURS_BEFORE falhou", "err", err)
		return
	}
	for _, e := range events {
		confirmed, err := s.q.FindRsvpsByEventsAndStatus(ctx, sqlc.FindRsvpsByEventsAndStatusParams{
			EventIds: []string{e.ID}, Status: "YES",
		})
		if err != nil {
			slog.Error("EVENT_REMINDER_HOURS_BEFORE falhou", "evento", e.ID, "err", err)
			continue
		}
		eventDay := date(e.ScheduledDatetime.In(s.loc))
		for _, r := range confirmed {
			s.tryDispatch(ctx, r.UserID, TypeEventReminderHoursBefore, e.ID, eventDay, func() {
				s.svc.sendEventReminder(ctx, r.UserID, e, reminderHoursBefore)
			})
		}
	}
}

// tryDispatch registra o envio e so entao envia. Se o registro ja existe (ou
// outro processo o criou neste instante), nao envia.
func (s *Scheduler) tryDispatch(ctx context.Context, userID, kind, referenceID string, day time.Time, send func()) {
	key := sqlc.DispatchLogExistsParams{
		UserID: userID, NotificationType: kind, ReferenceID: referenceID, DispatchedDate: day,
	}
	already, err := s.q.DispatchLogExists(ctx, key)
	if err != nil || already {
		if err != nil {
			slog.Error("registro de envio: consulta falhou", "tipo", kind, "err", err)
		}
		return
	}
	inserted, err := s.q.InsertDispatchLog(ctx, sqlc.InsertDispatchLogParams(key))
	if err != nil {
		slog.Error("registro de envio: gravacao falhou", "tipo", kind, "err", err)
		return
	}
	if inserted == 0 {
		return
	}
	send()
}
