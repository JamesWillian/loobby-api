package gameplays

import (
	"strings"
	"testing"
	"time"
)

func TestConviteDaUnica(t *testing.T) {
	f := newFixture(t)
	creator, guest, anon := f.user(), f.user(), f.anonymous()
	g := f.create(creator, nil, votingIn("A", "B"))
	code := *g.InviteCode

	p, err := f.svc.Preview(f.ctx(), code, guest)
	if err != nil || p.ID != g.ID || p.Joined || p.GoingCount != 1 || p.Creator.ID != creator || p.Status != "ACTIVE" {
		t.Fatalf("previa: %+v %v", p, err)
	}
	_, err = f.svc.Preview(f.ctx(), "$-NAOEXISTE", guest)
	wantErr(t, err, errInviteNotFound)

	// entra como Pendente; o codigo vale em minusculas e com espacos
	joined, err := f.svc.Join(f.ctx(), "  "+strings.ToLower(code)+" ", guest)
	if err != nil || joined.MyStatus != StatusPending || len(joined.Participants) != 2 || !joined.Permissions.Leave {
		t.Fatalf("entrar: %+v %v", joined, err)
	}
	if p, _ := f.svc.Preview(f.ctx(), code, guest); !p.Joined {
		t.Error("depois de entrar, a previa diz joined")
	}
	f.respond(g.ID, guest, StatusGoing)
	again, err := f.svc.Join(f.ctx(), code, guest)
	if err != nil || again.MyStatus != StatusGoing {
		t.Errorf("entrar de novo nao muda o status: %s %v", again.MyStatus, err)
	}

	// o anonimo entra e ve tudo, mas nao responde
	asAnon, err := f.svc.Join(f.ctx(), code, anon)
	if err != nil || asAnon.Permissions.Respond || asAnon.Voting == nil {
		t.Fatalf("anonimo entra e so ve: %+v %v", asAnon.Permissions, err)
	}
	for _, p := range asAnon.Participants {
		if p.User.ID == anon && !p.IsAnonymous {
			t.Error("o participante anonimo vem marcado")
		}
	}

	// sair: quem criou nao sai; quem sai perde o acesso e o voto aberto
	f.vote(g.ID, guest, gameID(t, g, "A"))
	err = f.svc.Leave(f.ctx(), g.ID, creator)
	wantErr(t, err, errCreatorLeaves)
	if err := f.svc.Leave(f.ctx(), g.ID, guest); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.Get(f.ctx(), g.ID, guest)
	wantErr(t, err, errNotFound)
	if after := f.get(g.ID, creator); votesOf(after, gameID(t, after, "A")) != 0 {
		t.Error("o voto de quem saiu sai junto")
	}
	err = f.svc.Leave(f.ctx(), g.ID, guest)
	wantErr(t, err, errNotFound)

	// encerrada: ninguem novo entra
	if _, err := f.svc.End(f.ctx(), g.ID, creator); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.Join(f.ctx(), code, guest)
	wantErr(t, err, errEnded)
	if back, err := f.svc.Join(f.ctx(), code, anon); err != nil || back.Status != "ENDED" {
		t.Errorf("quem ja estava continua vendo a encerrada: %v", err)
	}
}

func TestDaGameplayDoGrupoSaiSeDoGrupo(t *testing.T) {
	f := newFixture(t)
	owner, member := f.user(), f.user()
	group := f.group(owner, member)
	g := f.create(owner, &group, votingIn())
	err := f.svc.Leave(f.ctx(), g.ID, member)
	wantErr(t, err, errGroupLeave)
	if f.get(g.ID, member).Permissions.Leave {
		t.Error("na gameplay do grupo nao ha o botao de sair")
	}
}

func TestParticipantesEmOrdem(t *testing.T) {
	f := newFixture(t)
	owner, ana, bia, caio := f.user(), f.user(), f.user(), f.user()
	group := f.group(owner, ana, bia, caio)
	g := f.create(owner, &group, votingIn())
	f.respond(g.ID, bia, StatusNotGoing)
	time.Sleep(5 * time.Millisecond) // updated_at diferentes
	g = f.respond(g.ID, ana, StatusGoing)

	var order []string
	for _, p := range g.Participants {
		order = append(order, p.User.ID+":"+p.Status)
	}
	// Vou (na ordem em que responderam), depois Pendente, depois Nao vou
	want := []string{owner + ":GOING", ana + ":GOING", caio + ":PENDING", bia + ":NOT_GOING"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("ordem: %v\nqueria %v", order, want)
	}
	if g.Counts != (counts{Going: 2, NotGoing: 1, Pending: 1}) {
		t.Errorf("contagem: %+v", g.Counts)
	}
}
