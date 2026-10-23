// Package auth valida os JWT emitidos pelo lado Kotlin.
//
// Nao ha migracao de sessao: o Kotlin assina HS256 com JWT_SECRET, e o Go valida
// com o MESMO segredo. Token emitido por um vale no outro, nos dois sentidos.
// E o que permite o strangler rota a rota sem deslogar ninguem.
package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
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

// MustPrincipal so deve ser chamado dentro de handler protegido por Middleware.
func MustPrincipal(ctx context.Context) Principal {
	p, _ := FromContext(ctx)
	return p
}

type Middleware struct {
	secret               []byte
	acceptRefreshAsToken bool
}

func NewMiddleware(secret string, acceptRefreshAsAccess bool) *Middleware {
	return &Middleware{secret: []byte(secret), acceptRefreshAsToken: acceptRefreshAsAccess}
}

// Require exige JWT valido. Falha responde 401 com o mesmo envelope do Kotlin.
//
// O Spring Security responde 401 com corpo VAZIO e o header WWW-Authenticate
// quando falta ou falha o Bearer -- nao passa pelo ApiExceptionHandler. Isso foi
// medido na baseline (golden erro-401-sem-token) e esta replicado aqui.
func (m *Middleware) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := bearer(r)
		if err != nil {
			unauthorized(w, "")
			return
		}
		p, err := m.parse(raw)
		if err != nil {
			unauthorized(w, describe(err))
			return
		}
		ctx := context.WithValue(r.Context(), principalKey, p)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *Middleware) parse(raw string) (Principal, error) {
	var claims jwt.MapClaims
	_, err := jwt.ParseWithClaims(raw, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("algoritmo inesperado")
		}
		return m.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return Principal{}, err
	}

	p := Principal{
		UserID:   str(claims["sub"]),
		Username: str(claims["username"]),
		Type:     str(claims["type"]),
	}
	p.Roles = roles(claims["roles"])

	if p.UserID == "" {
		return Principal{}, errors.New("token sem sub")
	}
	if !m.acceptRefreshAsToken && p.Type == "refresh" {
		return Principal{}, errors.New("refresh token nao vale como access token")
	}
	return p, nil
}

// roles aceita as duas formas que o JwtAuthConverter do Kotlin aceita: lista, ou
// string separada por virgula/espaco.
func roles(raw any) []string {
	switch v := raw.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, r := range v {
			if s := strings.TrimSpace(str(r)); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		var out []string
		for _, r := range strings.FieldsFunc(v, func(c rune) bool { return c == ',' || c == ' ' }) {
			if s := strings.TrimSpace(r); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func bearer(r *http.Request) (string, error) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", errors.New("sem Authorization")
	}
	if len(h) < 7 || !strings.EqualFold(h[:7], "bearer ") {
		return "", errors.New("Authorization nao e Bearer")
	}
	return strings.TrimSpace(h[7:]), nil
}

func describe(err error) string {
	switch {
	case errors.Is(err, jwt.ErrTokenExpired):
		return `Bearer error="invalid_token", error_description="Jwt expired"`
	default:
		return `Bearer error="invalid_token"`
	}
}

func unauthorized(w http.ResponseWriter, challenge string) {
	if challenge == "" {
		challenge = "Bearer"
	}
	w.Header().Set("WWW-Authenticate", challenge)
	w.WriteHeader(http.StatusUnauthorized)
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
