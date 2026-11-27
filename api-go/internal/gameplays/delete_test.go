package gameplays

import (
	"slices"
	"testing"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
)

// O Excluir vale para os mesmos do Encerrar: quem criou ou o dono do grupo. O
// banco leva tudo junto, e quem marcou Vou na ativa recebe o aviso.
func TestExcluirGameplayDoGrupo(t *testing.T) {
	f := newFixture(t)
	owner, creator, going, notGoing, pending := f.user(), f.user(), f.user(), f.user(), f.user()
	anon, outsider := f.anonymous(), f.user()
	group := f.group(owner, creator, going, notGoing, pending, anon)
	g := f.create(creator, &group, votingIn("A", "B"))
	f.vote(g.ID, going, gameID(t, g, "A")) // Pendente vira Vou
	f.respond(g.ID, notGoing, StatusNotGoing)
	f.respond(g.ID, owner, StatusGoing) // quem exclui nao recebe o proprio aviso
	c, err := f.svc.AddComment(f.ctx(), g.ID, going, "bora")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Report(f.ctx(), g.ID, notGoing, c.ID, nil); err != nil {
		t.Fatal(err)
	}

	for _, u := range []string{creator, owner} {
		if !f.get(g.ID, u).Permissions.Delete {
			t.Errorf("quem criou e o dono do grupo excluem: %s", u)
		}
	}
	for _, u := range []string{going, anon} {
		if f.get(g.ID, u).Permissions.Delete {
			t.Errorf("membro e anonimo nao excluem: %s", u)
		}
	}
	for _, u := range []string{going, notGoing, pending, anon} {
		wantErr(t, f.svc.Delete(f.ctx(), g.ID, u), errNotManager)
	}
	wantErr(t, f.svc.Delete(f.ctx(), g.ID, outsider), errNotFound) // quem nao ve, nao sabe que existe
	wantErr(t, f.svc.Delete(f.ctx(), ids.New(), owner), errNotFound)
	if len(f.sent.to(TypeDeleted)) != 0 {
		t.Fatalf("a recusa nao avisa ninguem: %v", f.sent.to(TypeDeleted))
	}

	before := f.leftovers(g.ID, c.ID)
	for table, n := range before {
		if n == 0 {
			t.Fatalf("o teste precisa de linhas em %s antes de excluir: %v", table, before)
		}
	}

	// o dono do grupo exclui a gameplay que outra pessoa criou
	if err := f.svc.Delete(f.ctx(), g.ID, owner); err != nil {
		t.Fatal(err)
	}
	for table, n := range f.leftovers(g.ID, c.ID) {
		if n != 0 {
			t.Errorf("o CASCADE deveria levar %s: sobraram %d", table, n)
		}
	}
	_, err = f.svc.Get(f.ctx(), g.ID, creator)
	wantErr(t, err, errNotFound)
	wantErr(t, f.svc.Delete(f.ctx(), g.ID, owner), errNotFound) // repetir: ela nao existe mais
	if active, err := f.svc.ListGroup(f.ctx(), going, group, false, nil, defaultPage); err != nil || len(active) != 0 {
		t.Errorf("as ativas do grupo ficam sem ela: %+v %v", active, err)
	}

	// o aviso: quem marcou Vou, menos quem excluiu; sem o gameplayId, que nao existe mais
	if got := f.sent.to(TypeDeleted); !sameSet(got, []string{creator, going}) {
		t.Errorf("aviso de exclusao para %v, queria quem criou e quem votou", got)
	}
	p := f.sent.pushes[len(f.sent.pushes)-1].p
	if p.Type != TypeDeleted || p.Title != g.Name || p.Body != "Essa gameplay foi cancelada" {
		t.Errorf("push: %+v", p)
	}
	if _, ok := p.Data["gameplayId"]; ok || p.Data["groupId"] != group {
		t.Errorf("data do push: %v", p.Data)
	}
}

// A encerrada tambem sai -- pelo botao ou pelo prazo --, das Finalizadas e do
// Historico, e sem aviso: ela ja passou.
func TestExcluirGameplayEncerrada(t *testing.T) {
	f := newFixture(t)
	owner, creator, member := f.user(), f.user(), f.user()
	group := f.group(owner, creator, member)
	g := f.create(creator, &group, votingIn("A"))
	f.respond(g.ID, member, StatusGoing)
	ended, err := f.svc.End(f.ctx(), g.ID, creator)
	if err != nil {
		t.Fatal(err)
	}
	if p := ended.Permissions; p.Manage || !p.Delete {
		t.Errorf("na encerrada, quem criou so tem o Excluir: %+v", p)
	}
	if f.get(g.ID, member).Permissions.Delete {
		t.Error("o membro nao exclui a encerrada")
	}
	wantErr(t, f.svc.Delete(f.ctx(), g.ID, member), errNotManager)

	old := f.insertGameplay(owner, &group, ModeFree, time.Now().Add(-30*time.Hour)) // encerrada pelo prazo
	f.exec(`INSERT INTO gameplay_participants (gameplay_id, user_id, status) VALUES ($1, $2, 'GOING')`, old, member)
	if !slices.Contains(f.history(member), g.ID) || !slices.Contains(f.history(member), old) {
		t.Fatalf("as duas deveriam estar no Historico: %v", f.history(member))
	}

	if err := f.svc.Delete(f.ctx(), g.ID, creator); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Delete(f.ctx(), old, owner); err != nil {
		t.Fatal(err)
	}
	if h := f.history(member); slices.Contains(h, g.ID) || slices.Contains(h, old) {
		t.Errorf("o Historico fica sem elas: %v", h)
	}
	if list, err := f.svc.ListGroup(f.ctx(), member, group, true, nil, defaultPage); err != nil || len(list) != 0 {
		t.Errorf("as Finalizadas do grupo ficam sem elas: %+v %v", list, err)
	}
	if len(f.sent.to(TypeDeleted)) != 0 {
		t.Errorf("a encerrada nao avisa ninguem: %v", f.sent.to(TypeDeleted))
	}
}

// Na unica, so quem criou exclui; quem entrou pelo convite e marcou Vou recebe
// o aviso, e o convite deixa de valer.
func TestExcluirUnica(t *testing.T) {
	f := newFixture(t)
	creator, friend, anon := f.user(), f.user(), f.anonymous()
	g := f.create(creator, nil, createInput{Name: "Jogatina", Mode: ModeFree, Platforms: []string{"PC"}, ScheduledAt: soon()})
	for _, u := range []string{friend, anon} {
		if _, err := f.svc.Join(f.ctx(), *g.InviteCode, u); err != nil {
			t.Fatal(err)
		}
	}
	f.respond(g.ID, friend, StatusGoing)

	if !f.get(g.ID, creator).Permissions.Delete || f.get(g.ID, friend).Permissions.Delete {
		t.Error("na unica, so quem criou exclui")
	}
	wantErr(t, f.svc.Delete(f.ctx(), g.ID, friend), errNotManager)
	wantErr(t, f.svc.Delete(f.ctx(), g.ID, anon), errNotManager)
	if err := f.svc.Delete(f.ctx(), g.ID, creator); err != nil {
		t.Fatal(err)
	}

	if got := f.sent.to(TypeDeleted); !slices.Equal(got, []string{friend}) {
		t.Errorf("aviso de exclusao: %v", got)
	}
	if p := f.sent.pushes[len(f.sent.pushes)-1].p; len(p.Data) != 0 {
		t.Errorf("a unica nao tem grupo nem gameplay para abrir: %v", p.Data)
	}
	lobby, err := f.svc.Lobby(f.ctx(), friend)
	if err != nil || len(lobby) != 0 {
		t.Errorf("o Lobby fica sem ela: %+v %v", lobby, err)
	}
	_, err = f.svc.Preview(f.ctx(), *g.InviteCode, friend)
	wantErr(t, err, errInviteNotFound)
}

// leftovers conta o que ainda resta de uma gameplay em cada tabela da 00002.
func (f *fixture) leftovers(gameplayID, commentID string) map[string]int {
	f.t.Helper()
	return map[string]int{
		"gameplays":                f.countRows(`SELECT count(*) FROM gameplays WHERE id = $1`, gameplayID),
		"gameplay_participants":    f.countRows(`SELECT count(*) FROM gameplay_participants WHERE gameplay_id = $1`, gameplayID),
		"gameplay_games":           f.countRows(`SELECT count(*) FROM gameplay_games WHERE gameplay_id = $1`, gameplayID),
		"gameplay_votes":           f.countRows(`SELECT count(*) FROM gameplay_votes WHERE gameplay_id = $1`, gameplayID),
		"gameplay_comments":        f.countRows(`SELECT count(*) FROM gameplay_comments WHERE gameplay_id = $1`, gameplayID),
		"gameplay_comment_reports": f.countRows(`SELECT count(*) FROM gameplay_comment_reports WHERE comment_id = $1`, commentID),
	}
}

// history: os ids da primeira pagina do Historico de userID.
func (f *fixture) history(userID string) []string {
	f.t.Helper()
	list, err := f.svc.History(f.ctx(), userID, nil, defaultPage)
	if err != nil {
		f.t.Fatal(err)
	}
	return summaryIDs(list)
}
