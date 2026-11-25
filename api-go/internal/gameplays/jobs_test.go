package gameplays

import (
	"testing"
	"time"
)

// Os jobs leem todas as gameplays do banco. Para nao esbarrar nas dos outros
// testes, estes usam um "agora" em 2031 e so conferem os proprios usuarios.

func newJobs(t *testing.T, f *fixture, now time.Time) *Jobs {
	t.Helper()
	j, err := NewJobs(f.pool, f.sent)
	if err != nil {
		t.Fatal(err)
	}
	j.now = func() time.Time { return now }
	return j
}

func TestLembreteUmaHoraAntes(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2031, 3, 10, 15, 0, 0, 0, time.UTC)
	owner, ana, bia, caio := f.user(), f.user(), f.user(), f.user()
	group := f.group(owner, ana, bia, caio)

	in := votingIn()
	in.ScheduledAt = now.Add(reminderBefore + 5*time.Minute)
	g := f.create(owner, &group, in)
	f.respond(g.ID, ana, StatusGoing)
	f.respond(g.ID, caio, StatusNotGoing)

	far := votingIn()
	far.ScheduledAt = now.Add(3 * time.Hour) // fora da janela
	farG := f.create(owner, &group, far)
	f.respond(farG.ID, ana, StatusGoing)

	ended := votingIn()
	ended.ScheduledAt = now.Add(reminderBefore + 10*time.Minute)
	endedG := f.create(owner, &group, ended)
	f.respond(endedG.ID, bia, StatusGoing)
	if _, err := f.svc.End(f.ctx(), endedG.ID, owner); err != nil {
		t.Fatal(err)
	}

	j := newJobs(t, f, now)
	j.remind(f.ctx())
	j.remind(f.ctx()) // o job de 5 em 5 minutos passa de novo: nada sai duas vezes

	var mine []push
	for _, p := range f.sent.pushes {
		if p.p.Type == TypeReminder && p.p.Data["gameplayId"] == g.ID {
			mine = append(mine, p)
		}
	}
	if got := f.sent.to(TypeReminder); !sameSet(got, []string{owner, ana}) || len(mine) != 2 {
		t.Fatalf("lembrete so para quem marcou Vou, uma vez: %v (%d envios)", got, len(mine))
	}
	if mine[0].p.Title != g.Name || mine[0].p.Body != "Começa em 1 hora — se prepare!" || mine[0].p.Data["groupId"] != group {
		t.Errorf("conteudo do lembrete: %+v", mine[0].p)
	}
}

func TestPendentesDoDia(t *testing.T) {
	f := newFixture(t)
	sp, _ := time.LoadLocation(jobZone)
	morning := time.Date(2031, 3, 11, pendingHour, 0, 0, 0, sp)
	owner, ana, bia, anon := f.user(), f.user(), f.user(), f.anonymous()
	group := f.group(owner, ana, bia, anon)

	at := func(hour int, days int) time.Time { return time.Date(2031, 3, 11+days, hour, 0, 0, 0, sp) }
	in := votingIn()
	in.ScheduledAt = at(20, 0)
	tonight := f.create(owner, &group, in)
	f.respond(tonight.ID, ana, StatusGoing)

	early := votingIn()
	early.ScheduledAt = at(7, 0) // hoje, mas ja passou
	f.create(owner, &group, early)
	tomorrow := votingIn()
	tomorrow.ScheduledAt = at(10, 1)
	f.create(owner, &group, tomorrow)

	host, guest, anonGuest := f.user(), f.user(), f.anonymous()
	single := createInput{Name: "Unica", Mode: ModeFree, Platforms: []string{"PC"}, ScheduledAt: at(21, 0)}
	s := f.create(host, nil, single)
	for _, u := range []string{guest, anonGuest} {
		if _, err := f.svc.Join(f.ctx(), *s.InviteCode, u); err != nil {
			t.Fatal(err)
		}
	}

	j := newJobs(t, f, morning)
	j.todayPending(f.ctx())
	j.todayPending(f.ctx())

	// no grupo: quem nao respondeu (bia), sem o anonimo; na unica: quem esta Pendente
	got := f.sent.to(TypeTodayPending)
	if !sameSet(got, []string{bia, guest}) {
		t.Fatalf("pendentes do dia: %v, queria bia e o convidado", got)
	}
	count := 0
	for _, p := range f.sent.pushes {
		if p.p.Type == TypeTodayPending {
			count++
			if p.p.Title != "Gameplay hoje" {
				t.Errorf("titulo: %q", p.p.Title)
			}
		}
	}
	if count != 2 {
		t.Errorf("um aviso por pessoa e gameplay no dia: %d", count)
	}
}

func TestProximosHorariosDosJobsDaGameplay(t *testing.T) {
	f := newFixture(t)
	j := newJobs(t, f, time.Now())
	sp := j.loc
	if got := j.nextMorning(time.Date(2031, 3, 11, 7, 59, 0, 0, sp)); !got.Equal(time.Date(2031, 3, 11, 8, 0, 0, 0, sp)) {
		t.Errorf("antes das 8h: %s", got)
	}
	if got := j.nextMorning(time.Date(2031, 3, 11, 8, 0, 0, 0, sp)); !got.Equal(time.Date(2031, 3, 12, 8, 0, 0, 0, sp)) {
		t.Errorf("as 8h em ponto, o proximo e amanha: %s", got)
	}
	if got := j.nextTick(time.Date(2031, 3, 11, 10, 3, 20, 0, time.UTC)); !got.Equal(time.Date(2031, 3, 11, 10, 5, 0, 0, time.UTC)) {
		t.Errorf("de 5 em 5 minutos cheios: %s", got)
	}
}
