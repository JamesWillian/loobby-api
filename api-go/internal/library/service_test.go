package library

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JamesWillian/loobby-api/api-go/internal/dbtest"
	"github.com/JamesWillian/loobby-api/api-go/internal/games"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
	"github.com/JamesWillian/loobby-api/api-go/internal/platform"
)

// Os testes usam o banco de TEST_DATABASE_URL, dividido com os outros pacotes:
// cada um cria os proprios usuarios, grupos e jogos, e apaga so o que criou.

type fixture struct {
	t       *testing.T
	pool    *pgxpool.Pool
	svc     *Service
	catalog *fakeCatalog
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := dbtest.Pool(t)
	f := &fixture{t: t, pool: pool}
	f.catalog = &fakeCatalog{f: f, known: map[string]string{}}
	f.svc = NewService(pool, f.catalog)
	return f
}

func (f *fixture) ctx() context.Context { return context.Background() }

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.ctx(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

// rawgID sorteia um id do RAWG que nenhum outro teste usa.
func rawgID() string { return fmt.Sprintf("8%011d", rand.Int64N(1e11)) }

// game grava um jogo no cache do catalogo e o apaga no fim do teste (as
// bibliotecas vao junto, pelo CASCADE). rawg e o games.platforms.
func (f *fixture) game(name, rawg string) string {
	f.t.Helper()
	id := rawgID()
	f.cache(id, name, rawg)
	return id
}

func (f *fixture) cache(id, name, rawg string) {
	f.t.Helper()
	var platforms *string
	if rawg != "" {
		platforms = &rawg
	}
	f.exec(`INSERT INTO games (id, name, platforms) VALUES ($1, $2, ($3::text)::jsonb)`, id, name, platforms)
	f.t.Cleanup(func() { _, _ = f.pool.Exec(context.Background(), `DELETE FROM games WHERE id = $1`, id) })
}

func (f *fixture) user() string      { return dbtest.User(f.t, f.pool, 1) }
func (f *fixture) anonymous() string { return dbtest.User(f.t, f.pool, 0) }

// group cria um grupo de owner (o gatilho do banco o inclui como membro) com
// os outros membros.
func (f *fixture) group(owner string, members ...string) string {
	f.t.Helper()
	id := ids.New()
	f.exec(`INSERT INTO groups (id, name, invite_code, owner_id) VALUES ($1, $2, $3, $4)`,
		id, "Turma "+id[:6], "$-"+ids.Code(6), owner)
	for _, m := range members {
		f.exec(`INSERT INTO group_members (id, group_id, user_id) VALUES (gen_random_uuid(), $1, $2)`, id, m)
	}
	f.t.Cleanup(func() { _, _ = f.pool.Exec(context.Background(), `DELETE FROM groups WHERE id = $1`, id) })
	return id
}

// fakeCatalog faz o papel do RAWG: conhece os jogos de known e, quando pedem um
// deles, grava no cache, como o games.Service.Details.
type fakeCatalog struct {
	f     *fixture
	known map[string]string // id -> nome
	calls []string
}

func (c *fakeCatalog) Details(_ context.Context, id string) (games.Details, error) {
	c.calls = append(c.calls, id)
	name, ok := c.known[id]
	if !ok {
		return games.Details{}, games.ErrNotFound
	}
	c.f.cache(id, name, `[{"platform":{"slug":"nintendo-switch"}}]`)
	return games.Details{ID: id, Name: name}, nil
}

func wantErr(t *testing.T, err error, want *httpx.Error) {
	t.Helper()
	var got *httpx.Error
	if !errors.As(err, &got) || got.Status != want.Status || got.Error() != want.Error() {
		t.Fatalf("queria %d %q, veio %v", want.Status, want.Error(), err)
	}
}

func gameIDs(list []item) []string {
	var out []string
	for _, it := range list {
		out = append(out, it.GameID)
	}
	return out
}

func TestBibliotecaDaPessoa(t *testing.T) {
	f := newFixture(t)
	me, anon := f.user(), f.anonymous()
	gta := f.game("Grand Theft Auto V", `[{"platform":{"slug":"playstation5"}},{"platform":{"slug":"pc"}}]`)
	cs := f.game("counter-Strike 2", "")

	it, err := f.svc.Put(f.ctx(), me, gta, []string{})
	if err != nil {
		t.Fatal(err)
	}
	if it.Name != "Grand Theft Auto V" || len(it.OwnedPlatforms) != 0 ||
		!slices.Equal(it.AvailablePlatforms, []string{platform.PC, platform.PlayStation}) {
		t.Errorf("jogo novo, so na biblioteca: %+v", it)
	}
	if it, err = f.svc.Put(f.ctx(), me, cs, []string{platform.PC}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(it.OwnedPlatforms, []string{platform.PC}) || len(it.AvailablePlatforms) != 0 {
		t.Errorf("o RAWG sem plataformas da lista vazia: %+v", it)
	}

	// o PUT de novo troca o "tenho o jogo"
	it, err = f.svc.Put(f.ctx(), me, gta, []string{platform.PlayStation})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(it.OwnedPlatforms, []string{platform.PlayStation}) {
		t.Errorf("tenho o jogo: %v", it.OwnedPlatforms)
	}

	// pelo nome, sem diferenciar maiusculas
	mine, err := f.svc.Mine(f.ctx(), me)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(gameIDs(mine), []string{cs, gta}) {
		t.Errorf("ordem: %v", gameIDs(mine))
	}

	// o anonimo tem a dele, e uma nao ve a outra
	if _, err := f.svc.Put(f.ctx(), anon, gta, []string{platform.Switch}); err != nil {
		t.Fatalf("o anonimo tambem tem biblioteca: %v", err)
	}
	theirs, err := f.svc.Mine(f.ctx(), anon)
	if err != nil || !slices.Equal(gameIDs(theirs), []string{gta}) {
		t.Errorf("biblioteca do anonimo: %v (%v)", gameIDs(theirs), err)
	}

	// tirar, e tirar de novo
	if err := f.svc.Remove(f.ctx(), me, gta); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Remove(f.ctx(), me, gta); err != nil {
		t.Errorf("tirar o que nao esta la nao e erro: %v", err)
	}
	if mine, _ = f.svc.Mine(f.ctx(), me); !slices.Equal(gameIDs(mine), []string{cs}) {
		t.Errorf("depois de tirar: %v", gameIDs(mine))
	}
}

// O jogo que nao esta no cache e buscado no catalogo; o que o catalogo nao
// conhece da 404.
func TestBibliotecaBuscaOJogoNoCatalogo(t *testing.T) {
	f := newFixture(t)
	me := f.user()
	cached := f.game("Hades", "")
	fetched := rawgID()
	f.catalog.known[fetched] = "Celeste"

	if _, err := f.svc.Put(f.ctx(), me, cached, []string{}); err != nil {
		t.Fatal(err)
	}
	it, err := f.svc.Put(f.ctx(), me, fetched, []string{platform.Switch})
	if err != nil {
		t.Fatal(err)
	}
	if it.Name != "Celeste" || !slices.Equal(it.AvailablePlatforms, []string{platform.Switch}) {
		t.Errorf("jogo trazido do catalogo: %+v", it)
	}
	if !slices.Equal(f.catalog.calls, []string{fetched}) {
		t.Errorf("so o jogo fora do cache vai ao catalogo: %v", f.catalog.calls)
	}

	_, err = f.svc.Put(f.ctx(), me, rawgID(), []string{})
	wantErr(t, err, errGameNotFound)
	_, err = f.svc.PutGroup(f.ctx(), me, f.group(me), rawgID())
	wantErr(t, err, errGameNotFound)
}

func TestBibliotecaTemLimite(t *testing.T) {
	f := newFixture(t)
	f.svc.maxUser, f.svc.maxGroup = 2, 1
	me := f.user()
	a, b, c := f.game("A", ""), f.game("B", ""), f.game("C", "")
	for _, id := range []string{a, b} {
		if _, err := f.svc.Put(f.ctx(), me, id, []string{}); err != nil {
			t.Fatal(err)
		}
	}
	_, err := f.svc.Put(f.ctx(), me, c, []string{})
	wantErr(t, err, errUserFull)
	if _, err := f.svc.Put(f.ctx(), me, a, []string{platform.PC}); err != nil {
		t.Errorf("na biblioteca cheia, o que ja esta la ainda muda: %v", err)
	}

	g := f.group(me)
	if _, err := f.svc.PutGroup(f.ctx(), me, g, a); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.PutGroup(f.ctx(), me, g, b)
	wantErr(t, err, errGroupFull)
	if _, err := f.svc.PutGroup(f.ctx(), me, g, a); err != nil {
		t.Errorf("repetir na biblioteca cheia nao e erro: %v", err)
	}
}

func TestBibliotecaDoGrupo(t *testing.T) {
	f := newFixture(t)
	owner, member, anon, outsider := f.user(), f.user(), f.anonymous(), f.user()
	g := f.group(owner, member, anon)
	gta := f.game("GTA V", `[{"platform":{"slug":"xbox-one"}}]`)
	cs := f.game("CS2", "")

	it, err := f.svc.PutGroup(f.ctx(), member, g, gta)
	if err != nil {
		t.Fatal(err)
	}
	if it.AddedBy == nil || it.AddedBy.ID != member || !slices.Equal(it.AvailablePlatforms, []string{platform.Xbox}) {
		t.Errorf("jogo novo no grupo: %+v", it)
	}
	// repetir nao muda quem adicionou
	if it, err = f.svc.PutGroup(f.ctx(), owner, g, gta); err != nil || it.AddedBy == nil || it.AddedBy.ID != member {
		t.Errorf("repetido: %+v (%v)", it, err)
	}
	if _, err := f.svc.PutGroup(f.ctx(), owner, g, cs); err != nil {
		t.Fatal(err)
	}

	// o anonimo ve, mas nao mexe
	list, err := f.svc.Group(f.ctx(), anon, g)
	if err != nil || len(list) != 2 || list[0].GameID != cs || list[1].GameID != gta {
		t.Fatalf("lista para o anonimo: %+v (%v)", list, err)
	}
	if list[1].AddedBy == nil || list[1].AddedBy.Username == "" {
		t.Errorf("quem adicionou sai com o usuario inteiro: %+v", list[1].AddedBy)
	}
	_, err = f.svc.PutGroup(f.ctx(), anon, g, gta)
	wantErr(t, err, errAnonymous)
	wantErr(t, f.svc.RemoveGroup(f.ctx(), anon, g, gta), errAnonymous)

	// quem nao e membro nao ve nem mexe; grupo que nao existe, 404
	_, err = f.svc.Group(f.ctx(), outsider, g)
	wantErr(t, err, errNotMember)
	_, err = f.svc.PutGroup(f.ctx(), outsider, g, gta)
	wantErr(t, err, errNotMember)
	_, err = f.svc.Group(f.ctx(), owner, ids.New())
	wantErr(t, err, errGroupNotFound)

	// qualquer membro tira, inclusive o que outro adicionou
	if err := f.svc.RemoveGroup(f.ctx(), owner, g, gta); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.RemoveGroup(f.ctx(), owner, g, gta); err != nil {
		t.Errorf("tirar o que nao esta la nao e erro: %v", err)
	}
	if list, _ = f.svc.Group(f.ctx(), member, g); len(list) != 1 || list[0].GameID != cs {
		t.Errorf("depois de tirar: %+v", list)
	}
}

// A exclusao de conta (users.DeleteAccount) tira a biblioteca da pessoa e o
// nome dela dos jogos que pos na biblioteca do grupo.
func TestDeleteUserData(t *testing.T) {
	f := newFixture(t)
	gone, other := f.user(), f.user()
	g := f.group(other, gone)
	gta := f.game("GTA V", "")
	for _, u := range []string{gone, other} {
		if _, err := f.svc.Put(f.ctx(), u, gta, []string{platform.PC}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.svc.PutGroup(f.ctx(), gone, g, gta); err != nil {
		t.Fatal(err)
	}

	if err := DeleteUserData(f.ctx(), f.svc.q, gone); err != nil {
		t.Fatal(err)
	}
	if mine, _ := f.svc.Mine(f.ctx(), gone); len(mine) != 0 {
		t.Errorf("a biblioteca de quem saiu: %v", gameIDs(mine))
	}
	if theirs, _ := f.svc.Mine(f.ctx(), other); len(theirs) != 1 {
		t.Errorf("a dos outros fica: %v", gameIDs(theirs))
	}
	list, err := f.svc.Group(f.ctx(), other, g)
	if err != nil || len(list) != 1 || list[0].AddedBy != nil {
		t.Errorf("o jogo fica com o grupo, sem quem adicionou: %+v (%v)", list, err)
	}
}
