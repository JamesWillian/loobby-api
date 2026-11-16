// Package auth e o Spring Security do lado Go: valida e emite os JWT do Loobby
// e decide, por rota, quem precisa estar autenticado.
//
// Nao ha migracao de sessao: o Kotlin assina HS256 com JWT_SECRET, e o Go usa o
// MESMO segredo. Token emitido por um vale no outro, nos dois sentidos. E o que
// permite o strangler rota a rota sem deslogar ninguem.
package auth

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

type ctxKey int

const principalKey ctxKey = iota

// Principal e o que o Kotlin expoe via @AuthenticationPrincipal Jwt.
type Principal struct {
	UserID   string // claim `sub`
	Username string
	Roles    []string
	Type     string // "access" ou "refresh"
}

func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey).(Principal)
	return p, ok
}

// MustPrincipal so deve ser chamado em rota protegida pelo Filter.
func MustPrincipal(ctx context.Context) Principal {
	p, _ := FromContext(ctx)
	return p
}

// UserID e o `UUID.fromString(jwt.subject)` que abre quase todo controller do
// Kotlin. Quando o `sub` nao e um UUID, la isso lanca IllegalArgumentException
// e a resposta e 400 "Invalid UUID string: <sub>" (medido) -- so alcancavel com
// um token assinado com o segredo certo, entao e mais formalidade que caso real.
func UserID(r *http.Request) (string, error) {
	sub := MustPrincipal(r.Context()).UserID
	id, ok := httpx.ParseJavaUUID(sub)
	if !ok {
		return "", httpx.BadRequest("Invalid UUID string: " + sub)
	}
	return id, nil
}

// Filter reproduz a cadeia do Spring Security do Kotlin, e por isso envolve
// TODAS as rotas -- inclusive as que nao existem. As regras, todas medidas:
//
//  1. Caminho ou metodo que o StrictHttpFirewall recusa ("//", "..", ";",
//     "%2F"...): 401 `Bearer`, mesmo com token valido.
//  2. Authorization comecando com "bearer": o token e validado em QUALQUER
//     rota, ate nas publicas. Token ruim em /legal/privacy tambem da 401.
//  3. Sem token: so passam as rotas publicas (isPublic), comparadas por metodo
//     exato. HEAD numa rota publica de GET exige token.
//
// O 401 sai com corpo vazio e WWW-Authenticate, sem passar pelo envelope de
// erro da API -- no Kotlin quem responde e o filtro, nao um controller.
type Filter struct {
	tokens                *Tokens
	acceptRefreshAsAccess bool
	isPublic              func(method, path string) bool
}

func NewFilter(tokens *Tokens, acceptRefreshAsAccess bool, isPublic func(method, path string) bool) *Filter {
	return &Filter{tokens: tokens, acceptRefreshAsAccess: acceptRefreshAsAccess, isPublic: isPublic}
}

// Formato do DefaultBearerTokenResolver. Sem diferenciar maiusculas: "bearer x"
// e aceito (medido).
var bearerPattern = regexp.MustCompile(`(?i)^Bearer ([a-zA-Z0-9\-._~+/]+=*)$`)

const (
	challengeBare = "Bearer"
	errorURI      = `error_uri="https://tools.ietf.org/html/rfc6750#section-3.1"`
)

func (f *Filter) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if firewallRejects(r) {
			unauthorized(w, challengeBare)
			return
		}

		authenticated := false
		if h := r.Header.Get("Authorization"); len(h) >= 6 && strings.EqualFold(h[:6], "bearer") {
			m := bearerPattern.FindStringSubmatch(h)
			if m == nil {
				unauthorized(w, invalidToken("Bearer token is malformed"))
				return
			}
			claims, terr := f.tokens.Verify(m[1])
			if terr != nil {
				unauthorized(w, invalidToken(terr.Description))
				return
			}
			// O Kotlin NAO confere o claim `type` (ver config). Com a opcao
			// desligada, o Go passa a recusar refresh token usado como access.
			if !f.acceptRefreshAsAccess && claims.Type == "refresh" {
				unauthorized(w, invalidToken("Refresh token is not an access token"))
				return
			}
			authenticated = true
			r = r.WithContext(context.WithValue(r.Context(), principalKey, Principal{
				UserID: claims.Subject, Username: claims.Username, Roles: claims.Roles, Type: claims.Type,
			}))
		}

		if !authenticated && !f.isPublic(r.Method, r.URL.Path) {
			unauthorized(w, challengeBare)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func invalidToken(description string) string {
	return `Bearer error="invalid_token", error_description="` + description + `", ` + errorURI
}

func unauthorized(w http.ResponseWriter, challenge string) {
	w.Header().Set("WWW-Authenticate", challenge)
	w.WriteHeader(http.StatusUnauthorized)
}

// firewallRejects reproduz o StrictHttpFirewall. No Kotlin, uma requisicao
// recusada vira 400 interno, que e redespachado para /error, que exige
// autenticacao e nao a tem (o filtro do Bearer nao roda de novo): o cliente ve
// 401 `Bearer`. Medido com "/users//me" e token valido.
func firewallRejects(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions:
	default:
		return true
	}
	raw, _, _ := strings.Cut(r.RequestURI, "?")
	if raw == "" { // requisicao montada em teste
		raw = r.URL.EscapedPath()
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < 0x20 || raw[i] > 0x7e {
			return true
		}
	}
	lower := strings.ToLower(raw)
	for _, bad := range []string{"//", ";", `\`, "%2f", "%25", "%2e", "%5c", "%3b", "%00", "%0a", "%0d"} {
		if strings.Contains(lower, bad) {
			return true
		}
	}
	for _, seg := range strings.Split(raw, "/") {
		if seg == "." || seg == ".." {
			return true
		}
	}
	return false
}
