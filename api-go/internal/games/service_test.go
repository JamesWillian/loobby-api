package games

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JamesWillian/loobby-api/api-go/internal/db"
)

// Integracao com Postgres: TEST_DATABASE_URL com contract/schema.sql aplicado.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL nao definida")
	}
	pool, err := db.Connect(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	clean := func() { _, _ = pool.Exec(context.Background(), `DELETE FROM games WHERE id LIKE 't-%'`) }
	clean()
	t.Cleanup(func() { clean(); pool.Close() })
	return pool
}

type fakeRawg struct {
	mu          sync.Mutex
	games       map[string]string // id -> corpo; ausente = 404
	search      string
	down        bool
	gameCalls   int
	searchCalls int
}

func (f *fakeRawg) Search(_ context.Context, q string, page int) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.searchCalls++
	if f.down {
		return nil, ErrUnavailable
	}
	return []byte(f.search), nil
}

func (f *fakeRawg) Game(_ context.Context, id string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gameCalls++
	if f.down {
		return nil, ErrUnavailable
	}
	body, ok := f.games[id]
	if !ok {
		return nil, nil
	}
	return []byte(body), nil
}

func (f *fakeRawg) calls() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gameCalls, f.searchCalls
}

func marshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const jogo = `{"id":"t-1","name":"Jogo","slug":"jogo","rating":0.0,"metacritic":71,` +
	`"released":"2017-03-22","description_raw":"desc","genres":[{"slug":"acao","id":4}],"platforms":null}`

// A mesma rota, mesmo jogo: "0.0" na primeira chamada (do RAWG), "0.00" na
// segunda (do cache). Medido em producao; aqui com Postgres de verdade.
func TestRatingFrescoDepoisDoCache(t *testing.T) {
	ctx := context.Background()
	rawg := &fakeRawg{games: map[string]string{"t-1": jogo}}
	s := NewService(rawg, NewStore(testPool(t)))

	first, err := s.Details(ctx, "t-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Details(ctx, "t-1")
	if err != nil {
		t.Fatal(err)
	}
	if first.Rating != "0.0" || second.Rating != "0.00" {
		t.Errorf("rating fresco=%s cache=%s; esperava 0.0 e 0.00", first.Rating, second.Rating)
	}
	if g, _ := rawg.calls(); g != 1 {
		t.Errorf("a 2a leitura deveria vir do cache, mas o RAWG recebeu %d chamadas", g)
	}
}

// Deduzido do Kotlin, nao observado: do RAWG, genres sai na ordem do RAWG; do
// cache, na ordem do jsonb (o Postgres ordena as chaves pelo tamanho). Nos
// jogos medidos as duas ordens coincidiam, entao a producao nao distinguiu.
func TestOrdemDasChavesFrescoVsCache(t *testing.T) {
	ctx := context.Background()
	s := NewService(&fakeRawg{games: map[string]string{"t-1": jogo}}, NewStore(testPool(t)))
	first, _ := s.Details(ctx, "t-1")
	second, _ := s.Details(ctx, "t-1")
	if string(first.Genres) != `[{"slug":"acao","id":4}]` {
		t.Errorf("fresco: %s", first.Genres)
	}
	if string(second.Genres) != `[{"id":4,"slug":"acao"}]` {
		t.Errorf("cache: %s", second.Genres)
	}
	if first.Platforms != nil || second.Platforms != nil {
		t.Errorf("platforms null deveria continuar null: %s / %s", first.Platforms, second.Platforms)
	}
}

// As quatro decisoes do getDetails do Kotlin, na ordem.
func TestTTLDoDetalhe(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	rawg := &fakeRawg{games: map[string]string{"t-1": jogo}}
	s := NewService(rawg, NewStore(pool))
	base := time.Now()
	if _, err := s.Details(ctx, "t-1"); err != nil { // grava no cache
		t.Fatal(err)
	}

	cases := []struct {
		name      string
		age       time.Duration
		sync, bg  bool
		wantRatng Number
	}{
		{"ate 7 dias: so cache", 6 * 24 * time.Hour, false, false, "0.00"},
		{"7 a 30 dias: cache + refresh em segundo plano", 10 * 24 * time.Hour, false, true, "0.00"},
		{"mais de 30 dias: refresh antes de responder", 31 * 24 * time.Hour, true, false, "0.0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _ = pool.Exec(ctx, `UPDATE games SET refreshed_at = $1 WHERE id = 't-1'`, base)
			s.now = func() time.Time { return base.Add(c.age) }
			before, _ := rawg.calls()
			d, err := s.Details(ctx, "t-1")
			if err != nil {
				t.Fatal(err)
			}
			s.wg.Wait()
			after, _ := rawg.calls()
			if got := after - before; got != map[bool]int{true: 1, false: 0}[c.sync || c.bg] {
				t.Errorf("chamadas ao RAWG: %d", got)
			}
			if d.Rating != c.wantRatng {
				t.Errorf("rating %s, esperava %s (de onde veio a resposta)", d.Rating, c.wantRatng)
			}
		})
	}
	s.now = time.Now

	t.Run("sem descricao: busca o completo mesmo com cache novo", func(t *testing.T) {
		_, _ = pool.Exec(ctx, `UPDATE games SET description_raw = NULL, refreshed_at = now() WHERE id = 't-1'`)
		before, _ := rawg.calls()
		d, _ := s.Details(ctx, "t-1")
		after, _ := rawg.calls()
		if after-before != 1 || d.DescriptionRaw == nil {
			t.Errorf("deveria ter buscado no RAWG: chamadas=%d desc=%v", after-before, d.DescriptionRaw)
		}
	})
}

func TestRawgForaCaiNoCacheOuDa404(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	rawg := &fakeRawg{games: map[string]string{"t-1": jogo}}
	s := NewService(rawg, NewStore(pool))
	_, _ = s.Details(ctx, "t-1")
	_, _ = pool.Exec(ctx, `UPDATE games SET refreshed_at = now() - interval '40 days' WHERE id = 't-1'`)

	rawg.down = true
	d, err := s.Details(ctx, "t-1")
	if err != nil || d.Rating != "0.00" {
		t.Errorf("RAWG fora com cache vencido: deveria servir o cache, veio %v / %v", d.Rating, err)
	}
	if _, err := s.Details(ctx, "t-nunca-cacheado"); !errors.Is(err, ErrNotFound) {
		t.Errorf("RAWG fora sem cache: o Kotlin responde 404, veio %v", err)
	}
}

func TestDetalheSemDescriptionRawUsaDescription(t *testing.T) {
	ctx := context.Background()
	body := `{"id":"t-2","name":"X","description":"<p>html</p>"}`
	s := NewService(&fakeRawg{games: map[string]string{"t-2": body}}, NewStore(testPool(t)))
	d, err := s.Details(ctx, "t-2")
	if err != nil || d.DescriptionRaw == nil || *d.DescriptionRaw != "<p>html</p>" {
		t.Errorf("descricao: %v (%v)", d.DescriptionRaw, err)
	}
}

// Busca sem nome no item: o registro mantem o nome que ja tinha.
func TestBuscaPreservaNomeQuandoORawgNaoManda(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	_, _ = pool.Exec(ctx, `INSERT INTO games (id, name) VALUES ('t-3', 'Nome Antigo')`)
	rawg := &fakeRawg{search: `{"count":1,"results":[{"id":"t-3","rating":4.5}]}`}
	resp, err := NewService(rawg, NewStore(pool)).Search(ctx, "x", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 1 || resp.Results[0].Name != "Nome Antigo" || resp.Results[0].Rating != "4.5" {
		t.Errorf("resultado: %s", marshal(t, resp))
	}
}

// Chave do cache: trim + minusculas. "Zelda" e " zelda " dividem a entrada.
func TestBuscaUsaOCacheDeDezMinutos(t *testing.T) {
	ctx := context.Background()
	rawg := &fakeRawg{search: `{"count":0,"results":[]}`}
	s := NewService(rawg, NewStore(testPool(t)))
	a, _ := s.Search(ctx, "Zelda", 1)
	b, _ := s.Search(ctx, " zelda ", 1)
	if _, n := rawg.calls(); n != 1 {
		t.Errorf("a 2a busca deveria vir do cache: %d chamadas", n)
	}
	if got := marshal(t, a); got != marshal(t, b) || !strings.Contains(got, `"results":[]`) {
		t.Errorf("busca vazia deveria sair com results:[] -> %s", got)
	}
	s.now = func() time.Time { return time.Now().Add(11 * time.Minute) }
	_, _ = s.Search(ctx, "zelda", 1)
	if _, n := rawg.calls(); n != 2 {
		t.Errorf("depois de 10 min deveria ir ao RAWG de novo: %d chamadas", n)
	}
}

func TestBuscaComRawgForaDaErroDeIndisponivel(t *testing.T) {
	s := NewService(&fakeRawg{down: true}, NewStore(testPool(t)))
	if _, err := s.Search(context.Background(), "x", 1); !errors.Is(err, ErrSearchUnavailable) {
		t.Errorf("esperava ErrSearchUnavailable, veio %v", err)
	}
}
