package gameplays

import (
	"context"
	"log/slog"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
	"github.com/JamesWillian/loobby-api/api-go/internal/notifications"
)

const (
	jobZone        = "America/Sao_Paulo"
	jobInterval    = 5 * time.Minute
	reminderBefore = time.Hour
	reminderWindow = 15 * time.Minute // o job de 5 em 5 min passa umas 3 vezes; o registro de envio barra a repeticao
	pendingHour    = 8
)

// Jobs sao os tres trabalhos agendados da 2.0:
//
//   - de 5 em 5 minutos, o encerramento automatico: carimba o ended_at das que
//     passaram das 24 h. Roda sempre, mesmo com o push desligado. As consultas
//     ja tratam como encerrada a que passou do prazo, entao o job so deixa o
//     banco em dia (historico, relatorios);
//   - de 5 em 5 minutos, o lembrete para quem marcou Vou, 1 h antes do inicio;
//   - todo dia as 8h (America/Sao_Paulo), o aviso de gameplay hoje para quem
//     ainda nao respondeu (anonimo, que nao pode responder, fica de fora).
//
// Os dois pushes seguem as flags de hoje (NOTIFICATIONS_ENABLED e
// NOTIFICATIONS_SCHEDULER_ENABLED) e usam o notification_dispatch_log, como os
// da v1: com dois processos rodando, so um envia.
type Jobs struct {
	q      *sqlc.Queries
	sender notifications.Sender
	loc    *time.Location
	now    func() time.Time
}

func NewJobs(dbtx sqlc.DBTX, sender notifications.Sender) (*Jobs, error) {
	loc, err := time.LoadLocation(jobZone)
	if err != nil {
		return nil, err
	}
	return &Jobs{q: sqlc.New(dbtx), sender: sender, loc: loc, now: time.Now}, nil
}

// RunAutoEnd bloqueia ate o contexto ser cancelado.
func (j *Jobs) RunAutoEnd(ctx context.Context) {
	j.loop(ctx, j.nextTick, func(ctx context.Context) { _, _ = j.autoEnd(ctx) })
}

// RunNotifications bloqueia ate o contexto ser cancelado.
func (j *Jobs) RunNotifications(ctx context.Context) {
	go j.loop(ctx, j.nextMorning, j.todayPending)
	j.loop(ctx, j.nextTick, j.remind)
}

func (j *Jobs) loop(ctx context.Context, next func(time.Time) time.Time, job func(context.Context)) {
	for {
		wait := time.NewTimer(time.Until(next(j.now())))
		select {
		case <-ctx.Done():
			wait.Stop()
			return
		case <-wait.C:
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("job das gameplays: panico", "erro", r)
				}
			}()
			jobCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			defer cancel()
			job(jobCtx)
		}()
	}
}

func (j *Jobs) nextTick(now time.Time) time.Time {
	return now.Truncate(jobInterval).Add(jobInterval)
}

func (j *Jobs) nextMorning(now time.Time) time.Time {
	local := now.In(j.loc)
	next := time.Date(local.Year(), local.Month(), local.Day(), pendingHour, 0, 0, 0, j.loc)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// day: o dia local como parametro date do Postgres.
func day(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func (j *Jobs) autoEnd(ctx context.Context) (int64, error) {
	n, err := j.q.AutoEndGameplays(ctx, sqlc.AutoEndGameplaysParams{
		AfterSeconds: autoEndSeconds, Cutoff: j.now().Add(-AutoEndAfter),
	})
	if err != nil {
		slog.Error("encerramento automatico das gameplays falhou", "err", err)
		return 0, err
	}
	if n > 0 {
		slog.Info("gameplays encerradas pelo prazo de 24 h", "quantas", n)
	}
	return n, nil
}

// remind: as que comecam entre daqui a 1 h e daqui a 1 h 15.
func (j *Jobs) remind(ctx context.Context) {
	start := j.now().Add(reminderBefore)
	list, err := j.q.ListGameplaysStartingBetween(ctx, sqlc.ListGameplaysStartingBetweenParams{
		StartAt: start, EndAt: start.Add(reminderWindow),
	})
	if err != nil {
		slog.Error(TypeReminder+" falhou", "err", err)
		return
	}
	for _, g := range list {
		rows, err := j.q.ListParticipantsOfGameplays(ctx, []string{g.ID})
		if err != nil {
			slog.Error(TypeReminder+" falhou", "gameplay", g.ID, "err", err)
			continue
		}
		when := day(g.ScheduledAt.In(j.loc))
		for _, r := range rows {
			if r.Status != StatusGoing {
				continue
			}
			userID := r.UserID
			notifications.DispatchOnce(ctx, j.q, userID, TypeReminder, g.ID, when, func() {
				j.sender.SendToUsers(ctx, []string{userID}, notifications.Payload{
					Title: g.Name, Body: "Começa em 1 hora — se prepare!", Type: TypeReminder, Data: pushData(g),
				})
			})
		}
	}
}

// todayPending: as que ainda vao comecar hoje. Quem recebe: no grupo, os
// membros que nao responderam Vou nem Nao vou; na unica, quem esta Pendente.
func (j *Jobs) todayPending(ctx context.Context) {
	now := j.now().In(j.loc)
	end := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999_999_999, j.loc)
	list, err := j.q.ListGameplaysStartingBetween(ctx, sqlc.ListGameplaysStartingBetweenParams{StartAt: now, EndAt: end})
	if err != nil {
		slog.Error(TypeTodayPending+" falhou", "err", err)
		return
	}
	for _, g := range list {
		to, err := j.pending(ctx, g)
		if err != nil {
			slog.Error(TypeTodayPending+" falhou", "gameplay", g.ID, "err", err)
			continue
		}
		for _, userID := range to {
			notifications.DispatchOnce(ctx, j.q, userID, TypeTodayPending, g.ID, day(now), func() {
				j.sender.SendToUsers(ctx, []string{userID}, notifications.Payload{
					Title: "Gameplay hoje", Body: g.Name + " é hoje — você ainda não respondeu",
					Type: TypeTodayPending, Data: pushData(g),
				})
			})
		}
	}
}

func (j *Jobs) pending(ctx context.Context, g sqlc.Gameplay) ([]string, error) {
	rows, err := j.q.ListParticipantsOfGameplays(ctx, []string{g.ID})
	if err != nil {
		return nil, err
	}
	status := map[string]string{}
	for _, r := range rows {
		status[r.UserID] = r.Status
	}
	var candidates []string
	if g.GroupID != nil {
		if candidates, err = j.q.FindMemberUserIDs(ctx, *g.GroupID); err != nil {
			return nil, err
		}
	} else {
		for _, r := range rows {
			candidates = append(candidates, r.UserID)
		}
	}
	var undecided []string
	for _, id := range candidates {
		if st := status[id]; st != StatusGoing && st != StatusNotGoing {
			undecided = append(undecided, id)
		}
	}
	users, err := people(ctx, j.q, undecided)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, id := range undecided {
		if u, ok := users[id]; ok && u.AuthProvider != 0 {
			out = append(out, id)
		}
	}
	return out, nil
}
