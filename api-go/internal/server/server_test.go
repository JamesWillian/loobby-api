package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JamesWillian/loobby-api/api-go/internal/config"
	"github.com/JamesWillian/loobby-api/api-go/internal/contracttest"
)

type fakeDB struct{ err error }

func (f fakeDB) Ping(context.Context) error { return f.err }

func handler(db fakeDB) http.Handler {
	return Handler(Deps{
		Config: config.Config{JWTSecret: "segredo-de-teste", AcceptRefreshTokenAsAccess: true},
		DB:     db,
	})
}

func do(h http.Handler, method, path string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// Primeiro modulo do strangler. Corpo, status e headers de contrato precisam
// ser identicos aos gravados do Kotlin em producao.
func TestLegalIdenticoAoKotlin(t *testing.T) {
	for _, name := range []string{"legal-privacy", "legal-account-deletion", "legal-child-safety"} {
		t.Run(name, func(t *testing.T) {
			g := contracttest.Load(t, name)
			contracttest.AssertMatches(t, g, do(handler(fakeDB{}), g.Request.Method, g.Request.Path, nil))
		})
	}
}

func TestHealthIdenticoAoKotlin(t *testing.T) {
	g := contracttest.Load(t, "actuator-health")
	contracttest.AssertMatches(t, g, do(handler(fakeDB{}), "GET", "/actuator/health", nil))
}

// O 401 sai do filtro do Spring Security antes de qualquer controller, entao a
// forma da resposta -- status, headers, corpo vazio -- e a mesma em qualquer rota
// protegida. O golden foi gravado em /users/me, que o Go ainda nao serve; por
// isso a comparacao e feita na rota protegida que existe.
func Test401SemTokenIdenticoAoKotlin(t *testing.T) {
	g := contracttest.Load(t, "erro-401-sem-token")
	contracttest.AssertMatches(t, g, do(handler(fakeDB{}), "GET", "/internal/whoami", nil))
}

func TestPreflightCORSIdenticoAoKotlin(t *testing.T) {
	g := contracttest.Load(t, "cors-preflight-recusado")
	contracttest.AssertMatches(t, g, do(handler(fakeDB{}), "OPTIONS", "/legal/privacy", map[string]string{
		"Origin": "https://exemplo.com", "Access-Control-Request-Method": "GET",
	}))
}

func TestHealthDownQuandoOBancoCai(t *testing.T) {
	rec := do(handler(fakeDB{err: errors.New("conexao recusada")}), "GET", "/actuator/health", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("esperava 503, veio %d", rec.Code)
	}
	if got := rec.Body.String(); got != `{"status":"DOWN"}` {
		t.Errorf("corpo: %s", got)
	}
}

// O padrao "GET" do ServeMux tambem atende HEAD, como o Spring: mesmos headers,
// sem corpo.
func TestHeadEmLegal(t *testing.T) {
	rec := do(handler(fakeDB{}), "HEAD", "/legal/privacy", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD esperava 200, veio %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html;charset=UTF-8" {
		t.Errorf("content-type no HEAD: %q", ct)
	}
}
