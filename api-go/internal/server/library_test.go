package server

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JamesWillian/loobby-api/api-go/internal/auth"
	"github.com/JamesWillian/loobby-api/api-go/internal/config"
	"github.com/JamesWillian/loobby-api/api-go/internal/dbtest"
	"github.com/JamesWillian/loobby-api/api-go/internal/games"
	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
	"github.com/JamesWillian/loobby-api/api-go/internal/library"
)

// semCatalogo: o RAWG que nao conhece nenhum jogo. Os do teste ja estao no cache.
type semCatalogo struct{}

func (semCatalogo) Details(context.Context, string) (games.Details, error) {
	return games.Details{}, games.ErrNotFound
}

// As bibliotecas pela cadeia inteira: filtro, roteamento, corpo, servico e o
// JSON que o app 2.0 vai ler.
func TestBibliotecasPelaAPI(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := t.Context()
	owner, anon := dbtest.User(t, pool, 1), dbtest.User(t, pool, 0)
	group := ids.New()
	if _, err := pool.Exec(ctx, `INSERT INTO groups (id, name, invite_code, owner_id) VALUES ($1, 'Turma', $2, $3)`,
		group, "$-"+ids.Code(6), owner); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM groups WHERE id = $1`, group) })
	if _, err := pool.Exec(ctx, `INSERT INTO group_members (id, group_id, user_id) VALUES (gen_random_uuid(), $1, $2)`, group, anon); err != nil {
		t.Fatal(err)
	}
	game := fmt.Sprintf("6%011d", rand.Int64N(1e11))
	if _, err := pool.Exec(ctx, `INSERT INTO games (id, name, background_image, platforms) VALUES ($1, 'Hades', 'https://media.rawg.io/h.jpg',
	                             '[{"platform":{"id":7,"name":"Nintendo Switch","slug":"nintendo-switch"}},{"platform":{"id":4,"name":"PC","slug":"pc"}}]')`,
		game); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM games WHERE id = $1`, game) })

	h := Handler(Deps{
		Config:  config.Config{JWTSecret: secret, AcceptRefreshTokenAsAccess: true},
		DB:      fakeDB{},
		Library: library.NewHandler(library.NewService(pool, semCatalogo{})),
	})
	tokens := auth.NewTokens(secret)
	as := func(user string, roles ...string) map[string]string {
		if len(roles) == 0 {
			roles = []string{"USER"}
		}
		return map[string]string{"Authorization": "Bearer " + tokens.Access(user, "u", roles)}
	}
	call := func(method, path string, hdr map[string]string, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	expect := func(rec *httptest.ResponseRecorder, status int, contains ...string) {
		t.Helper()
		if rec.Code != status {
			t.Fatalf("status %d, esperava %d: %s", rec.Code, status, rec.Body)
		}
		for _, c := range contains {
			if !strings.Contains(rec.Body.String(), c) {
				t.Errorf("a resposta deveria conter %s: %s", c, rec.Body)
			}
		}
	}
	mine := "/v2/me/library/" + game

	expect(call("GET", "/v2/me/library", nil, ""), http.StatusUnauthorized)
	expect(call("GET", "/v2/me/library", as(owner), ""), http.StatusOK, `[]`)

	// a minha biblioteca: o PUT adiciona e troca o "tenho o jogo"
	expect(call("PUT", mine, as(owner), `{"ownedPlatforms":[]}`), http.StatusOK,
		`"gameId":"`+game+`","name":"Hades","coverUrl":"https://media.rawg.io/h.jpg"`,
		`"availablePlatforms":["PC","SWITCH"]`, `"ownedPlatforms":[]`, `"addedAt":`, `"updatedAt":`)
	expect(call("PUT", mine, as(owner), `{"ownedPlatforms":["SWITCH","PC"]}`), http.StatusOK, `"ownedPlatforms":["PC","SWITCH"]`)
	expect(call("GET", "/v2/me/library", as(owner), ""), http.StatusOK, `"ownedPlatforms":["PC","SWITCH"]`)
	expect(call("PUT", mine, as(anon, "ANON"), `{"ownedPlatforms":["PC"]}`), http.StatusOK) // o anonimo tambem tem

	// os erros: corpo, plataformas, jogo
	expect(call("PUT", mine, as(owner), `{"ownedPlatforms":`), http.StatusBadRequest, `"message":"Corpo da requisição inválido"`)
	expect(call("PUT", mine, as(owner), ``), http.StatusBadRequest, `"message":"Corpo da requisição inválido"`)
	expect(call("PUT", mine, as(owner), `{}`), http.StatusBadRequest, `"message":"Dados inválidos"`, `"field":"ownedPlatforms"`)
	expect(call("PUT", mine, as(owner), `{"ownedPlatforms":["PC","PC"]}`), http.StatusBadRequest, `Plataforma repetida`)
	expect(call("PUT", mine, as(owner), `{"ownedPlatforms":["NES"]}`), http.StatusBadRequest, `Plataforma inválida`)
	expect(call("PUT", "/v2/me/library/hades", as(owner), `{"ownedPlatforms":[]}`), http.StatusNotFound, `"message":"Jogo não encontrado"`)
	expect(call("PUT", "/v2/me/library/123456789012345678901", as(owner), `{"ownedPlatforms":[]}`), http.StatusNotFound)
	expect(call("PUT", "/v2/me/library/999999999999", as(owner), `{"ownedPlatforms":[]}`), http.StatusNotFound, `"message":"Jogo não encontrado"`)

	expect(call("DELETE", mine, as(owner), ""), http.StatusNoContent)
	expect(call("DELETE", mine, as(owner), ""), http.StatusNoContent)
	expect(call("GET", "/v2/me/library", as(owner), ""), http.StatusOK, `[]`)

	// a do grupo: o membro que nao e anonimo mexe, o anonimo so ve
	shared := "/v2/groups/" + group + "/library"
	expect(call("PUT", shared+"/"+game, as(owner), ""), http.StatusOK, `"gameId":"`+game+`"`,
		`"addedBy":{"id":"`+owner+`"`, `"availablePlatforms":["PC","SWITCH"]`)
	expect(call("PUT", shared+"/"+game, as(anon, "ANON"), ""), http.StatusForbidden,
		`"message":"Conta anônima só pode ver. Crie uma conta para participar."`)
	expect(call("GET", shared, as(anon, "ANON"), ""), http.StatusOK, `"name":"Hades"`)
	expect(call("GET", shared, as(dbtest.User(t, pool, 1)), ""), http.StatusForbidden, `"message":"Você não é membro desse grupo"`)
	expect(call("GET", "/v2/groups/nao-e-uuid/library", as(owner), ""), http.StatusNotFound, `"message":"Grupo não encontrado"`)
	expect(call("GET", "/v2/groups/"+ids.New()+"/library", as(owner), ""), http.StatusNotFound)
	expect(call("DELETE", shared+"/"+game, as(owner), ""), http.StatusNoContent)
	expect(call("GET", shared, as(owner), ""), http.StatusOK, `[]`)
}
