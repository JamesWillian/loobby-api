package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JamesWillian/loobby-api/api-go/internal/config"
	"github.com/JamesWillian/loobby-api/api-go/internal/contracttest"
	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/games"
)

const secret = "segredo-de-teste"

func gamesPool(t *testing.T, ids ...string) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL nao definida")
	}
	pool, err := db.Connect(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	clean := func() { _, _ = pool.Exec(context.Background(), `DELETE FROM games WHERE id = ANY($1)`, ids) }
	clean()
	t.Cleanup(func() { clean(); pool.Close() })
	return pool
}

// rawgFalso guarda o que recebeu, para os testes de parametro.
type rawgFalso struct {
	mu     sync.Mutex
	search string
	games  map[string]string
	down   bool
	q      string
	page   int
}

func (r *rawgFalso) Search(_ context.Context, q string, page int) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.q, r.page = q, page
	if r.down {
		return nil, games.ErrUnavailable
	}
	return []byte(r.search), nil
}

func (r *rawgFalso) Game(_ context.Context, id string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.down {
		return nil, games.ErrUnavailable
	}
	if b, ok := r.games[id]; ok {
		return []byte(b), nil
	}
	return nil, nil
}

func gamesHandler(pool *pgxpool.Pool, rawg games.Rawg) http.Handler {
	return Handler(Deps{
		Config: config.Config{JWTSecret: secret, AcceptRefreshTokenAsAccess: true},
		DB:     fakeDB{},
		Games:  games.NewHandler(games.NewService(rawg, games.NewStore(pool))),
	})
}

func accessToken(t *testing.T) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "54f29b67-cb8b-4435-8b72-43c17f2f4b8f", "type": "access", "username": "user_demo",
		"roles": []string{"USER"}, "iat": time.Now().Unix(), "exp": time.Now().Add(15 * time.Minute).Unix(),
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func get(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.RequestURI = target // como chega do servidor de verdade
	req.Header.Set("Authorization", "Bearer "+accessToken(t))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// rawgDaResposta reconstroi o que o RAWG teria mandado a partir de uma resposta
// da API: nomes de campo do RAWG e id numerico. Os valores seguem como BYTES
// BRUTOS: decodificar num map do Go perderia a ordem das chaves dentro de
// genres/platforms (e o json.Marshal reescreveria em ordem alfabetica), e o
// literal do rating ("0.0", "3.31") tambem precisa chegar intacto.
func rawgDaResposta(t *testing.T, item map[string]json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	out := map[string]json.RawMessage{}
	rename := map[string]string{"backgroundImage": "background_image", "descriptionRaw": "description_raw"}
	for k, v := range item {
		if n, ok := rename[k]; ok {
			k = n
		}
		out[k] = v
	}
	var id string
	if err := json.Unmarshal(out["id"], &id); err == nil {
		out["id"] = json.RawMessage(id) // no RAWG o id e numero
	}
	return out
}

func decodeRaw(t *testing.T, raw string) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func decode(t *testing.T, raw string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestGamesBuscaIdenticaAoGolden(t *testing.T) {
	golden := contracttest.Load(t, "games-search")
	var resp struct {
		Count   json.RawMessage
		Results []map[string]json.RawMessage
	}
	if err := json.Unmarshal([]byte(*golden.Response.BodyRaw), &resp); err != nil {
		t.Fatal(err)
	}
	var ids []string
	var items []any
	for _, m := range resp.Results {
		var id string
		_ = json.Unmarshal(m["id"], &id)
		ids = append(ids, id)
		items = append(items, rawgDaResposta(t, m))
	}
	search, _ := json.Marshal(map[string]any{"count": resp.Count, "results": items})
	rawg := &rawgFalso{search: string(search)}
	h := gamesHandler(gamesPool(t, ids...), rawg)

	contracttest.AssertMatches(t, golden, get(t, h, "/games/search?q=zelda&page=1"))
	contracttest.AssertMatches(t, contracttest.Load(t, "games-search-cache-hit"),
		get(t, h, "/games/search?q=zelda&page=1"))
	if rawg.q != "zelda" || rawg.page != 1 {
		t.Errorf("RAWG recebeu q=%q page=%d", rawg.q, rawg.page)
	}
}

// Golden gravado da producao quando o jogo ja estava no cache: "rating":0.00.
// O teste semeia o banco com os mesmos dados e exige a mesma resposta.
func TestGamesDetalheDoCacheIdenticoAoGolden(t *testing.T) {
	golden := contracttest.Load(t, "games-details")
	d := decode(t, *golden.Response.BodyRaw)
	pool := gamesPool(t, d["id"].(string))
	genres, _ := json.Marshal(d["genres"])
	platforms, _ := json.Marshal(d["platforms"])
	_, err := pool.Exec(context.Background(), `
		INSERT INTO games (id, slug, name, background_image, released, rating, metacritic,
		                   description_raw, genres, platforms, refreshed_at)
		VALUES ($1, $2, $3, $4, $5::date, $6::numeric, $7, $8, $9::jsonb, $10::jsonb, now())`,
		d["id"], d["slug"], d["name"], d["backgroundImage"], d["released"], d["rating"].(json.Number).String(),
		d["metacritic"], d["descriptionRaw"], string(genres), string(platforms))
	if err != nil {
		t.Fatal(err)
	}
	contracttest.AssertMatches(t, golden, get(t, gamesHandler(pool, &rawgFalso{}), golden.Request.Path))
}

// Medido em producao em 23/10: a 1a chamada vem do RAWG ("rating":0.0), a 2a
// do cache ("rating":0.00). O Go precisa reproduzir as DUAS, nessa ordem.
func TestGamesFrescoECacheIdenticosAProducao(t *testing.T) {
	for _, id := range []string{"257", "21482"} {
		t.Run(id, func(t *testing.T) {
			read := func(kind string) string {
				b, err := os.ReadFile(filepath.Join("testdata", "games", id+"-"+kind+".json"))
				if err != nil {
					t.Fatal(err)
				}
				return string(b)
			}
			fresco, cacheado := read("fresco"), read("cache")
			body, _ := json.Marshal(rawgDaResposta(t, decodeRaw(t, fresco)))
			h := gamesHandler(gamesPool(t, id), &rawgFalso{games: map[string]string{id: string(body)}})

			for i, want := range []string{fresco, cacheado} {
				rec := get(t, h, "/games/"+id)
				if got := rec.Body.String(); rec.Code != http.StatusOK || got != want {
					at := 0
					for at < len(got) && at < len(want) && got[at] == want[at] {
						at++
					}
					from := max(0, at-60)
					t.Errorf("chamada %d (%s), status %d, primeira diferenca no byte %d:\n  producao: ...%q\n  go      : ...%q",
						i+1, map[int]string{0: "RAWG", 1: "cache"}[i], rec.Code, at,
						want[from:min(len(want), at+60)], got[from:min(len(got), at+60)])
				}
			}
		})
	}
}

// O golden e de ANTES do PR #1 (500). O Go ja segue o Kotlin corrigido: 404.
// Os headers continuam os do golden.
func TestGamesDetalheInexistente(t *testing.T) {
	golden := contracttest.Load(t, "games-details-inexistente")
	h := gamesHandler(gamesPool(t, "id-que-nao-existe-9999999"), &rawgFalso{})
	rec := get(t, h, golden.Request.Path)
	var env map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if rec.Code != 404 || env["error"] != "Not Found" ||
		env["message"] != "Jogo id-que-nao-existe-9999999 não encontrado" ||
		env["path"] != "/games/id-que-nao-existe-9999999" {
		t.Errorf("404 esperado: %d %s", rec.Code, rec.Body.String())
	}
	for _, name := range contracttest.ContractHeaders {
		if name == "content-type" || golden.Response.Headers[name] == "" {
			continue
		}
		if got := rec.Header().Get(name); got == "" {
			t.Errorf("header %s ausente", name)
		}
	}
}

// Cada linha foi medida em producao em 23/10 (ver contract/ACHADOS.md).
func TestGamesParametrosComoOSpring(t *testing.T) {
	h := gamesHandler(gamesPool(t), &rawgFalso{search: `{"count":0,"results":[]}`})
	cases := []struct {
		query  string
		status int
		q      string
		page   int
	}{
		{"", 500, "", 0},          // q ausente
		{"q=", 500, "", 0},        // q vazio
		{"q=%20", 500, "", 0},     // so espaco
		{"q=%C2%A0", 200, " ", 1}, // espaco nao-separavel: o trim do Java nao remove
		{"q=mario&q=kart", 200, "mario,kart", 1},
		{"q=mario&page=abc", 500, "", 0},
		{"q=mario&page=", 200, "mario", 1},    // vazio = default
		{"q=mario&page=0x2", 200, "mario", 2}, // hexadecimal
		{"q=mario&page=%202%20", 200, "mario", 2},
		{"q=mario&page=2&page=3", 200, "mario", 2},
		{"q=mario&page=&page=2", 500, "", 0},
	}
	for _, c := range cases {
		rawg := &rawgFalso{search: `{"count":0,"results":[]}`}
		h = gamesHandler(gamesPool(t), rawg)
		rec := get(t, h, "/games/search?"+c.query)
		if rec.Code != c.status {
			t.Errorf("?%s: status %d, esperava %d", c.query, rec.Code, c.status)
			continue
		}
		if c.status == 500 {
			var env map[string]any
			_ = json.Unmarshal(rec.Body.Bytes(), &env)
			if env["message"] != "Unexpected error" {
				t.Errorf("?%s: envelope %s", c.query, rec.Body.String())
			}
			continue
		}
		if rawg.q != c.q || rawg.page != c.page {
			t.Errorf("?%s: RAWG recebeu q=%q page=%d, esperava q=%q page=%d", c.query, rawg.q, rawg.page, c.q, c.page)
		}
	}
}

func TestGamesRawgForaDa503(t *testing.T) {
	h := gamesHandler(gamesPool(t), &rawgFalso{down: true})
	rec := get(t, h, "/games/search?q=zelda")
	var env map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if rec.Code != 503 || env["message"] != "Catálogo de jogos temporariamente indisponível." {
		t.Errorf("RAWG fora: %d %s", rec.Code, rec.Body.String())
	}
}
