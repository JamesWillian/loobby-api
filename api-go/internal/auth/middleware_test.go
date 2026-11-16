package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

const testSecret = "segredo-so-de-teste-nao-e-o-de-producao-0123456789"

const testUser = "54f29b67-cb8b-4435-8b72-43c17f2f4b8f"

// Formatos dos tokens REAIS emitidos pela API Kotlin em producao. O header vem
// sem `typ`, e o refresh nao carrega roles nem username:
//
//	access : {sub, exp, type:"access", iat, roles:["USER"], username:"user_demo"}
//	refresh: {sub, exp, type:"refresh", iat}
func newTestTokens(at time.Time) *Tokens {
	t := NewTokens(testSecret)
	t.now = func() time.Time { return at }
	return t
}

func isPublicOnlyHealth(method, path string) bool {
	return method == http.MethodGet && path == "/actuator/health"
}

// serve passa uma requisicao pelo Filter e devolve a resposta e o que o handler
// protegido viu (principal), se chegou a ser chamado.
func serve(f *Filter, method, path, authHeader string) (*httptest.ResponseRecorder, *Principal) {
	var seen *Principal
	h := f.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p, ok := FromContext(r.Context()); ok {
			seen = &p
		}
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(method, path, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, seen
}

func newFilter(acceptRefresh bool) (*Filter, *Tokens) {
	tok := NewTokens(testSecret)
	return NewFilter(tok, acceptRefresh, isPublicOnlyHealth), tok
}

func TestAccessTokenAceito(t *testing.T) {
	f, tok := newFilter(true)
	rec, p := serve(f, "GET", "/users/me", "Bearer "+tok.Access(testUser, "user_demo", []string{"USER"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, veio %d", rec.Code)
	}
	if p == nil || p.UserID != testUser || p.Username != "user_demo" || p.Type != "access" ||
		len(p.Roles) != 1 || p.Roles[0] != "USER" {
		t.Errorf("principal errado: %+v", p)
	}
}

// Paridade com o Kotlin: o resource server nao confere o claim `type`, entao um
// refresh token e aceito onde deveria ir um access (medido em producao).
func TestRefreshTokenAceitoEmParidade(t *testing.T) {
	f, tok := newFilter(true)
	rec, p := serve(f, "GET", "/users/me", "Bearer "+tok.Refresh(testUser))
	if rec.Code != http.StatusOK {
		t.Fatalf("paridade com o Kotlin exige 200, veio %d", rec.Code)
	}
	if p == nil || p.Type != "refresh" {
		t.Errorf("tipo do token deveria chegar no principal: %+v", p)
	}
}

func TestRefreshTokenRecusadoQuandoEndurecido(t *testing.T) {
	f, tok := newFilter(false)
	rec, _ := serve(f, "GET", "/users/me", "Bearer "+tok.Refresh(testUser))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("com ACCEPT_REFRESH_AS_ACCESS=false esperava 401, veio %d", rec.Code)
	}
}

// O Spring Security responde 401 com corpo VAZIO e WWW-Authenticate, sem passar
// pelo ApiExceptionHandler -- nem content-type sai.
func TestSemAuthorizationRespondeComoOSpring(t *testing.T) {
	f, _ := newFilter(true)
	rec, _ := serve(f, "GET", "/users/me", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperava 401, veio %d", rec.Code)
	}
	if rec.Body.String() != "" {
		t.Errorf("corpo deveria ser vazio, veio %q", rec.Body.String())
	}
	if rec.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Errorf("WWW-Authenticate: %q", rec.Header().Get("WWW-Authenticate"))
	}
	if ct := rec.Header().Get("Content-Type"); ct != "" {
		t.Errorf("nao deveria ter content-type, veio %q", ct)
	}
}

func TestRotaPublicaPassaSemTokenMasNaoOutroMetodo(t *testing.T) {
	f, _ := newFilter(true)
	if rec, _ := serve(f, "GET", "/actuator/health", ""); rec.Code != http.StatusOK {
		t.Errorf("GET publico sem token: %d", rec.Code)
	}
	// HEAD numa rota publica de GET exige token (medido).
	if rec, _ := serve(f, "HEAD", "/actuator/health", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("HEAD sem token: %d", rec.Code)
	}
}

// O token e validado em TODA rota, ate nas publicas.
func TestTokenRuimEmRotaPublicaTambemDa401(t *testing.T) {
	f, _ := newFilter(true)
	rec, _ := serve(f, "GET", "/actuator/health", "Bearer abc.def.ghi")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperava 401, veio %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("WWW-Authenticate"), `error="invalid_token"`) {
		t.Errorf("challenge: %q", rec.Header().Get("WWW-Authenticate"))
	}
}

func TestBearerMalformado(t *testing.T) {
	f, _ := newFilter(true)
	for _, h := range []string{"Bearer ", "Bearer a b", "BearerXYZ", "Bearer a.b.c!"} {
		rec, _ := serve(f, "GET", "/users/me", h)
		if rec.Code != http.StatusUnauthorized ||
			!strings.Contains(rec.Header().Get("WWW-Authenticate"), "Bearer token is malformed") {
			t.Errorf("%q: %d %q", h, rec.Code, rec.Header().Get("WWW-Authenticate"))
		}
	}
}

func TestBearerEmMinusculaAceito(t *testing.T) {
	f, tok := newFilter(true)
	rec, _ := serve(f, "GET", "/users/me", "bearer "+tok.Access(testUser, "u", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, veio %d", rec.Code)
	}
}

// 60 s de folga: medido contra o Kotlin (59 s passa, 61 s nao).
func TestFolgaDeRelogio(t *testing.T) {
	f, _ := newFilter(true)
	agora := time.Now()
	emitido := newTestTokens(agora.Add(-AccessTokenValidity - 59*time.Second))
	if rec, _ := serve(f, "GET", "/users/me", "Bearer "+emitido.Access(testUser, "u", nil)); rec.Code != http.StatusOK {
		t.Errorf("expirado ha 59 s deveria passar: %d", rec.Code)
	}
	velho := newTestTokens(agora.Add(-AccessTokenValidity - 2*time.Minute))
	rec, _ := serve(f, "GET", "/users/me", "Bearer "+velho.Access(testUser, "u", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expirado ha 2 min deveria cair: %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("WWW-Authenticate"), "Jwt expired at ") {
		t.Errorf("mensagem: %q", rec.Header().Get("WWW-Authenticate"))
	}
}

func TestAssinaturaDeOutroSegredoRecusada(t *testing.T) {
	f, _ := newFilter(true)
	outro := NewTokens("outro-segredo-qualquer-com-mais-de-32-bytes-xx")
	rec, _ := serve(f, "GET", "/users/me", "Bearer "+outro.Access(testUser, "u", nil))
	if rec.Code != http.StatusUnauthorized ||
		!strings.Contains(rec.Header().Get("WWW-Authenticate"), "Signed JWT rejected: Invalid signature") {
		t.Fatalf("%d %q", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}
}

func TestAlgNoneRecusado(t *testing.T) {
	f, _ := newFilter(true)
	// {"alg":"none"}.{"sub":"x"}.
	tok := "eyJhbGciOiJub25lIn0.eyJzdWIiOiJ4In0."
	rec, _ := serve(f, "GET", "/users/me", "Bearer "+tok)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("alg=none precisa ser recusado, veio %d", rec.Code)
	}
}

// O StrictHttpFirewall recusa estes caminhos com 401 `Bearer`, mesmo com token valido.
func TestFirewallRecusaCaminhosEstranhos(t *testing.T) {
	f, tok := newFilter(true)
	h := "Bearer " + tok.Access(testUser, "u", nil)
	for _, path := range []string{"/users//me", "/users/./me", "/users/../me", "/users/me;x=1", "/users/%2e/me", "/users/%2Fme"} {
		req := httptest.NewRequest("GET", "/", nil)
		req.URL.Path = strings.ReplaceAll(path, "%2e", ".")
		req.RequestURI = path
		req.Header.Set("Authorization", h)
		rec := httptest.NewRecorder()
		f.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })).ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") != "Bearer" {
			t.Errorf("%s: %d %q", path, rec.Code, rec.Header().Get("WWW-Authenticate"))
		}
	}
}

// O JwtAuthConverter do Kotlin aceita roles como lista OU como string.
func TestRolesEmListaOuString(t *testing.T) {
	for _, raw := range []any{[]any{"USER", "ADMIN"}, "USER,ADMIN", "USER ADMIN"} {
		got := roles(raw)
		if len(got) != 2 || got[0] != "USER" || got[1] != "ADMIN" {
			t.Errorf("%v -> %v", raw, got)
		}
	}
}

func TestUserIDComSubInvalido(t *testing.T) {
	f, tok := newFilter(true)
	var err error
	h := f.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, err = UserID(r) }))
	req := httptest.NewRequest("GET", "/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+tok.Access("nao-e-uuid", "u", nil))
	h.ServeHTTP(httptest.NewRecorder(), req)
	var he *httpx.Error
	if !errors.As(err, &he) || he.Status != 400 || he.Message == nil || *he.Message != "Invalid UUID string: nao-e-uuid" {
		t.Fatalf("esperava 400 \"Invalid UUID string: nao-e-uuid\", veio %#v", err)
	}
}
