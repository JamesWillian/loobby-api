package gameplays

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/JamesWillian/loobby-api/api-go/internal/platform"
)

// catalogGame grava um jogo no cache do catalogo (games) e o apaga no fim do
// teste; a biblioteca vai junto, pelo CASCADE.
func (f *fixture) catalogGame(name string) string {
	f.t.Helper()
	id := fmt.Sprintf("7%011d", rand.Int64N(1e11))
	f.exec(`INSERT INTO games (id, name) VALUES ($1, $2)`, id, name)
	f.t.Cleanup(func() { _, _ = f.pool.Exec(context.Background(), `DELETE FROM games WHERE id = $1`, id) })
	return id
}

// owns poe o jogo na biblioteca de userID, com o "tenho o jogo" em platforms.
func (f *fixture) owns(userID, gameID string, platforms ...string) {
	f.t.Helper()
	f.exec(`INSERT INTO user_library (user_id, game_id, owned_platforms) VALUES ($1, $2, ($3::text)::jsonb)`,
		userID, gameID, platform.Encode(platforms))
}

func ownerIDs(list []userRef) []string {
	out := []string{}
	for _, u := range list {
		out = append(out, u.ID)
	}
	return out
}

func votingOwners(t *testing.T, g gameplayResponse, name string) []string {
	t.Helper()
	for _, gm := range g.Voting.Games {
		if gm.Name == name {
			return ownerIDs(gm.Owners)
		}
	}
	t.Fatalf("o jogo %q nao esta na votacao", name)
	return nil
}

// O match: em cada jogo do catalogo, quem vai e o tem numa plataforma da
// gameplay, na ordem em que respondeu.
func TestMatchNaVotacao(t *testing.T) {
	f := newFixture(t)
	owner, ana, bia, caio := f.user(), f.user(), f.user(), f.user()
	group := f.group(owner, ana, bia, caio)
	cs, valorant := f.catalogGame("CS2"), f.catalogGame("Valorant")
	f.owns(owner, cs, platform.PC)
	f.owns(ana, cs, platform.PC, platform.Xbox)
	f.owns(ana, valorant, platform.PC)
	f.owns(bia, cs, platform.Xbox) // a gameplay e de PC
	f.owns(caio, cs)               // so na biblioteca, sem o "tenho o jogo"

	in := votingIn("Digitado")
	in.Games = append(in.Games, gameInput{GameID: &cs, Name: "CS2"}, gameInput{GameID: &valorant, Name: "Valorant"})
	g := f.create(owner, &group, in)
	for _, u := range []string{ana, bia, caio} {
		g = f.respond(g.ID, u, StatusGoing)
	}
	if got := votingOwners(t, g, "CS2"); !slices.Equal(got, []string{owner, ana}) {
		t.Errorf("CS2: %v", got)
	}
	if got := votingOwners(t, g, "Valorant"); !slices.Equal(got, []string{ana}) {
		t.Errorf("Valorant: %v", got)
	}
	if got := votingOwners(t, g, "Digitado"); got == nil || len(got) != 0 {
		t.Errorf("o jogo digitado nao tem match: %#v", got)
	}

	// quem deixa de ir sai do match
	g = f.respond(g.ID, ana, StatusNotGoing)
	if got := votingOwners(t, g, "CS2"); !slices.Equal(got, []string{owner}) {
		t.Errorf("CS2 sem a Ana: %v", got)
	}
}

func TestMatchNoJogoDefinido(t *testing.T) {
	f := newFixture(t)
	owner, ana, bia := f.user(), f.user(), f.user()
	group := f.group(owner, ana, bia)
	cs := f.catalogGame("CS2")
	f.owns(owner, cs, platform.PC)
	f.owns(ana, cs, platform.PC, platform.Xbox)
	f.owns(bia, cs, platform.Xbox)

	g := f.create(owner, &group, createInput{
		Name: "CS no Xbox", Mode: ModeFixed, Platforms: []string{platform.Xbox}, ScheduledAt: soon(),
		Game: &gameInput{GameID: &cs, Name: "CS2"},
	})
	if g.Game == nil || len(g.Game.Owners) != 0 {
		t.Fatalf("quem criou so tem no PC: %+v", g.Game)
	}
	f.respond(g.ID, bia, StatusGoing)
	g = f.respond(g.ID, ana, StatusGoing)
	if got := ownerIDs(g.Game.Owners); !slices.Equal(got, []string{bia, ana}) {
		t.Errorf("donos no Xbox, na ordem das respostas: %v", got)
	}
	if g.Game.Owners[0].Username == "" {
		t.Errorf("o dono sai com o usuario inteiro: %+v", g.Game.Owners[0])
	}

	typed := f.create(owner, &group, createInput{
		Name: "Free Fire", Mode: ModeFixed, Platforms: []string{platform.Mobile}, ScheduledAt: soon(),
		Game: &gameInput{Name: "Free Fire"},
	})
	if typed.Game == nil || typed.Game.Owners == nil || len(typed.Game.Owners) != 0 {
		t.Errorf("o jogo digitado sai com a lista vazia: %#v", typed.Game)
	}
}

// As listas dizem se ha empate, e enquanto o desempate nao tem voto mostram o
// empate que o abriu.
func TestEmpateNasListas(t *testing.T) {
	f := newFixture(t)
	creator, ana, bia, caio := f.user(), f.user(), f.user(), f.user()
	group := f.group(creator, ana, bia, caio)
	g := f.create(creator, &group, votingIn("A", "B", "C"))
	a, b := gameID(t, g, "A"), gameID(t, g, "B")

	summary := func() gameplaySummary {
		t.Helper()
		list, err := f.svc.ListGroup(f.ctx(), creator, group, false, nil, defaultPage)
		if err != nil || len(list) != 1 {
			t.Fatalf("lista do grupo: %+v (%v)", list, err)
		}
		return list[0]
	}
	names := func(s gameplaySummary) []string {
		var out []string
		for _, l := range s.Leading {
			out = append(out, l.Name)
		}
		return out
	}

	if s := summary(); s.Tie || len(s.Leading) != 0 {
		t.Errorf("sem voto: %+v", s)
	}
	f.vote(g.ID, ana, a)
	f.vote(g.ID, bia, b)
	if s := summary(); !s.Tie || !slices.Equal(names(s), []string{"A", "B"}) {
		t.Errorf("empate: tie %v, %v", s.Tie, names(s))
	}
	if _, err := f.svc.Tiebreak(f.ctx(), g.ID, creator); err != nil {
		t.Fatal(err)
	}
	if s := summary(); !s.Tie || !slices.Equal(names(s), []string{"A", "B"}) {
		t.Errorf("desempate sem voto mostra o empate: tie %v, %v", s.Tie, names(s))
	}
	f.vote(g.ID, caio, a)
	if s := summary(); s.Tie || !slices.Equal(names(s), []string{"A"}) {
		t.Errorf("desempatado: tie %v, %v", s.Tie, names(s))
	}

	// com 2 finalistas, dois na frente nao e empate
	in := votingIn("A", "B", "C")
	in.Finalists = 2
	two := f.create(creator, &group, in)
	f.vote(two.ID, ana, gameID(t, two, "A"))
	f.vote(two.ID, bia, gameID(t, two, "B"))
	list, err := f.svc.Lobby(f.ctx(), creator)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range list {
		if s.ID == two.ID && (s.Tie || len(s.Leading) != 2) {
			t.Errorf("dois finalistas: %+v", s)
		}
	}
}
