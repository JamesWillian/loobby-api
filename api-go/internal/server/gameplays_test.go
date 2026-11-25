package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/auth"
	"github.com/JamesWillian/loobby-api/api-go/internal/config"
	"github.com/JamesWillian/loobby-api/api-go/internal/dbtest"
	"github.com/JamesWillian/loobby-api/api-go/internal/gameplays"
	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
	"github.com/JamesWillian/loobby-api/api-go/internal/notifications"
)

// As rotas /v2 pela cadeia inteira: filtro de autenticacao, roteamento,
// leitura do corpo, servico, banco e o JSON que o app 2.0 vai ler.
func TestGameplaysPelaAPI(t *testing.T) {
	pool := dbtest.Pool(t)
	owner, member, anon := dbtest.User(t, pool, 1), dbtest.User(t, pool, 1), dbtest.User(t, pool, 0)
	ctx := t.Context()
	group := ids.New()
	if _, err := pool.Exec(ctx, `INSERT INTO groups (id, name, invite_code, owner_id) VALUES ($1, 'Turma v2', $2, $3)`,
		group, "$-"+ids.Code(6), owner); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(t.Context(), `DELETE FROM groups WHERE id = $1`, group) })
	for _, u := range []string{member, anon} {
		if _, err := pool.Exec(ctx, `INSERT INTO group_members (id, group_id, user_id) VALUES (gen_random_uuid(), $1, $2)`, group, u); err != nil {
			t.Fatal(err)
		}
	}

	h := Handler(Deps{
		Config:    config.Config{JWTSecret: secret, AcceptRefreshTokenAsAccess: true},
		DB:        fakeDB{},
		Gameplays: gameplays.NewHandler(gameplays.NewService(pool, notifications.DisabledSender{}, semJogos{})),
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

	when := time.Now().Add(3 * time.Hour).UTC().Format(time.RFC3339)
	create := `{"name":"Sexta do CS","mode":"VOTING","platforms":["PC"],"scheduledAt":"` + when + `",
	            "voting":{"finalists":1},"games":[{"gameId":"4291","name":"Counter-Strike 2"},{"name":"Valorant"}]}`

	// sem token, o filtro responde antes de tudo
	expect(call("GET", "/v2/me/gameplays", nil, ""), http.StatusUnauthorized)

	rec := call("POST", "/v2/groups/"+group+"/gameplays", as(owner), create)
	expect(rec, http.StatusCreated, `"mode":"VOTING"`, `"platforms":["PC"]`, `"groupName":"Turma v2"`,
		`"inviteCode":null`, `"description":null`, `"status":"ACTIVE"`, `"myStatus":"GOING"`)
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("headers: %v", rec.Header())
	}
	var g struct {
		ID     string `json:"id"`
		Voting struct {
			Games []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"games"`
		} `json:"voting"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &g); err != nil || len(g.Voting.Games) != 2 {
		t.Fatalf("corpo da criacao: %s (%v)", rec.Body, err)
	}

	// os erros da v2: corpo ilegivel 400, validacao com fieldErrors, anonimo 403, 404 e 409
	expect(call("POST", "/v2/gameplays", as(owner), `{"name":`), http.StatusBadRequest, `"message":"Corpo da requisição inválido"`)
	expect(call("POST", "/v2/gameplays", as(owner), `{"name":"x","mode":2}`), http.StatusBadRequest)
	expect(call("POST", "/v2/gameplays", as(owner), `{"name":" ","mode":"VOTING","platforms":["PC","XBOX"],"scheduledAt":"2020-01-01T00:00:00Z"}`),
		http.StatusBadRequest, `"message":"Dados inválidos"`, `"field":"name"`, `"field":"platforms"`, `"field":"scheduledAt"`,
		`Por enquanto, só uma plataforma por gameplay`)
	expect(call("POST", "/v2/groups/"+group+"/gameplays", as(anon, "ANON"), create), http.StatusForbidden,
		`"message":"Conta anônima só pode ver. Crie uma conta para participar."`)
	expect(call("GET", "/v2/gameplays/"+ids.New(), as(owner), ""), http.StatusNotFound, `"message":"Gameplay não encontrada"`)
	expect(call("GET", "/v2/gameplays/nao-e-uuid", as(owner), ""), http.StatusNotFound, `"path":"/v2/gameplays/nao-e-uuid"`)

	// o membro vota pela API: Pendente vira Vou
	valorant := g.Voting.Games[1].ID
	expect(call("PUT", "/v2/gameplays/"+g.ID+"/votes/me", as(member), `{"votes":["`+valorant+`"]}`), http.StatusOK,
		`"myStatus":"GOING"`, `"finalistIds":["`+valorant+`"]`)
	expect(call("PUT", "/v2/gameplays/"+g.ID+"/votes/me", as(member), `{}`), http.StatusBadRequest, `"field":"votes"`)
	expect(call("PUT", "/v2/gameplays/"+g.ID+"/participants/me", as(member), `{"status":"MAYBE"}`), http.StatusBadRequest, `"field":"status"`)
	expect(call("PATCH", "/v2/gameplays/"+g.ID, as(owner), `{"voting":{"multiVote":true}}`), http.StatusConflict,
		`A votação já tem voto`)

	// comentario, lista, denuncia sem corpo, apagar
	rec = call("POST", "/v2/gameplays/"+g.ID+"/comments", as(member), `{"body":"  bora  "}`)
	expect(rec, http.StatusCreated, `"body":"bora"`, `"canDelete":true`, `"reportedByMe":false`)
	var c struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &c)
	expect(call("POST", "/v2/gameplays/"+g.ID+"/comments/"+c.ID+"/report", as(owner), ""), http.StatusNoContent)
	expect(call("GET", "/v2/gameplays/"+g.ID+"/comments?limit=10", as(owner), ""), http.StatusOK, `"reportedByMe":true`)
	expect(call("GET", "/v2/gameplays/"+g.ID+"/comments?limit=x", as(owner), ""), http.StatusBadRequest)
	expect(call("DELETE", "/v2/gameplays/"+g.ID+"/comments/"+c.ID, as(owner), ""), http.StatusNoContent)

	// listas
	expect(call("GET", "/v2/me/gameplays", as(member), ""), http.StatusOK, `"id":"`+g.ID+`"`, `"leading":[{"gameId":null,"name":"Valorant"`)
	expect(call("GET", "/v2/groups/"+group+"/gameplays", as(member), ""), http.StatusOK, `"id":"`+g.ID+`"`)
	expect(call("GET", "/v2/groups/"+group+"/gameplays?status=ended", as(member), ""), http.StatusOK)
	expect(call("GET", "/v2/groups/"+group+"/gameplays?status=talvez", as(member), ""), http.StatusBadRequest)

	// a unica pelo convite, ate para o anonimo
	rec = call("POST", "/v2/gameplays", as(owner), `{"name":"Unica","mode":"FIXED","platforms":["MOBILE"],"scheduledAt":"`+when+`","game":{"name":"Free Fire"}}`)
	expect(rec, http.StatusCreated, `"game":{"gameId":null,"name":"Free Fire","coverUrl":null}`, `"voting":null`)
	var single struct {
		ID         string `json:"id"`
		InviteCode string `json:"inviteCode"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &single)
	expect(call("GET", "/v2/invites/"+single.InviteCode, as(anon, "ANON"), ""), http.StatusOK, `"joined":false`)
	expect(call("POST", "/v2/invites/"+single.InviteCode, as(anon, "ANON"), ""), http.StatusOK, `"myStatus":"PENDING"`, `"isAnonymous":true`)
	expect(call("GET", "/v2/invites/$-NAOEXISTE", as(anon, "ANON"), ""), http.StatusNotFound, `"message":"Convite inválido"`)

	// encerrar
	expect(call("POST", "/v2/gameplays/"+g.ID+"/end", as(member), ""), http.StatusForbidden)
	expect(call("POST", "/v2/gameplays/"+g.ID+"/end", as(owner), ""), http.StatusOK, `"status":"ENDED"`)
	expect(call("POST", "/v2/gameplays/"+g.ID+"/end", as(owner), ""), http.StatusConflict, `"message":"Essa gameplay já foi encerrada"`)
}
