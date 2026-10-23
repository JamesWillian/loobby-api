package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "segredo-so-de-teste-nao-e-o-de-producao"

// Formatos copiados de tokens REAIS emitidos pela API Kotlin em producao
// (claims inspecionadas em 08/09/2026). O header vem sem `typ`, e o refresh nao
// carrega roles nem username:
//
//	access : {exp, iat, roles:["USER"], sub, type:"access", username:"user_demo"}
//	refresh: {exp, iat, sub, type:"refresh"}
func sign(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func accessClaims() jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"sub": "54f29b67-cb8b-4435-8b72-43c17f2f4b8f", "iat": now.Unix(),
		"exp": now.Add(15 * time.Minute).Unix(), "type": "access",
		"username": "user_demo", "roles": []string{"USER"},
	}
}

func refreshClaims() jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"sub": "54f29b67-cb8b-4435-8b72-43c17f2f4b8f", "iat": now.Unix(),
		"exp": now.Add(15 * 24 * time.Hour).Unix(), "type": "refresh",
	}
}

func serve(m *Middleware, authHeader string) *httptest.ResponseRecorder {
	var seen Principal
	h := m.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = MustPrincipal(r.Context())
		w.Header().Set("X-User", seen.UserID)
		w.Header().Set("X-Type", seen.Type)
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/users/me", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAccessTokenAceito(t *testing.T) {
	m := NewMiddleware(testSecret, true)
	rec := serve(m, "Bearer "+sign(t, accessClaims()))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, veio %d", rec.Code)
	}
	if got := rec.Header().Get("X-User"); got != "54f29b67-cb8b-4435-8b72-43c17f2f4b8f" {
		t.Errorf("sub nao chegou no principal: %q", got)
	}
}

// Paridade com o Kotlin: o resource server nao checa o claim `type`, entao um
// refresh token e aceito onde deveria ir um access. Medido em producao:
// GET /users/me com o refresh no Bearer devolve 200.
func TestRefreshTokenAceitoQuandoEmParidade(t *testing.T) {
	m := NewMiddleware(testSecret, true)
	rec := serve(m, "Bearer "+sign(t, refreshClaims()))
	if rec.Code != http.StatusOK {
		t.Fatalf("paridade com o Kotlin exige 200, veio %d", rec.Code)
	}
	if got := rec.Header().Get("X-Type"); got != "refresh" {
		t.Errorf("tipo do token deveria chegar no principal, veio %q", got)
	}
}

func TestRefreshTokenRecusadoQuandoEndurecido(t *testing.T) {
	m := NewMiddleware(testSecret, false)
	rec := serve(m, "Bearer "+sign(t, refreshClaims()))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("com ACCEPT_REFRESH_AS_ACCESS=false esperava 401, veio %d", rec.Code)
	}
}

// O Spring Security responde 401 com corpo VAZIO e WWW-Authenticate, sem passar
// pelo ApiExceptionHandler -- nem content-type sai. Medido no golden
// erro-401-sem-token.
func TestSemAuthorizationRespondeComoOSpring(t *testing.T) {
	m := NewMiddleware(testSecret, true)
	rec := serve(m, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperava 401, veio %d", rec.Code)
	}
	if body := rec.Body.String(); body != "" {
		t.Errorf("corpo deveria ser vazio, veio %q", body)
	}
	if rec.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Errorf("WWW-Authenticate ausente: %q", rec.Header().Get("WWW-Authenticate"))
	}
	if ct := rec.Header().Get("Content-Type"); ct != "" {
		t.Errorf("nao deveria ter content-type, veio %q", ct)
	}
}

func TestTokenExpirado(t *testing.T) {
	c := accessClaims()
	c["exp"] = time.Now().Add(-time.Minute).Unix()
	rec := serve(NewMiddleware(testSecret, true), "Bearer "+sign(t, c))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperava 401, veio %d", rec.Code)
	}
}

func TestAssinaturaDeOutroSegredoRecusada(t *testing.T) {
	tok := sign(t, accessClaims())
	rec := serve(NewMiddleware("outro-segredo-qualquer", true), "Bearer "+tok)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperava 401, veio %d", rec.Code)
	}
}

func TestAlgNoneRecusado(t *testing.T) {
	tok, err := jwt.NewWithClaims(jwt.SigningMethodNone, accessClaims()).
		SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	rec := serve(NewMiddleware(testSecret, true), "Bearer "+tok)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("alg=none precisa ser recusado, veio %d", rec.Code)
	}
}

// O JwtAuthConverter do Kotlin aceita roles como lista OU como string separada
// por virgula/espaco. O Go precisa aceitar as duas.
func TestRolesEmListaOuString(t *testing.T) {
	for _, raw := range []any{[]string{"USER", "ADMIN"}, "USER,ADMIN", "USER ADMIN"} {
		c := accessClaims()
		c["roles"] = raw
		m := NewMiddleware(testSecret, true)
		p, err := m.parse(sign(t, c))
		if err != nil {
			t.Fatalf("%v: %v", raw, err)
		}
		if len(p.Roles) != 2 || p.Roles[0] != "USER" || p.Roles[1] != "ADMIN" {
			t.Errorf("%v -> %v", raw, p.Roles)
		}
	}
}
