package auth

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

// Validades do application.yaml do Kotlin (security.jwt.*).
const (
	AccessTokenValidity  = 15 * time.Minute
	RefreshTokenValidity = 21600 * time.Minute // 15 dias

	// clockSkew: a folga padrao do JwtTimestampValidator do Spring. Medido: um
	// token expirado ha 59 s ainda e aceito; ha 61 s, nao.
	clockSkew = 60 * time.Second
)

// Tokens emite e valida os JWT do Loobby: HS256 com o JWT_SECRET, no mesmo
// formato do TokenService do Kotlin. Token de um lado vale no outro.
type Tokens struct {
	secret []byte
	now    func() time.Time
}

func NewTokens(secret string) *Tokens {
	return &Tokens{secret: []byte(secret), now: time.Now}
}

// Claims e o conteudo de um token ja validado.
type Claims struct {
	Subject  string
	Username string
	Roles    []string
	Type     string // "" quando o claim nao veio
	HasType  bool
}

// O cabecalho que o NimbusJwtEncoder emite: so o algoritmo, sem "typ".
var headerSegment = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`))

// Os payloads repetem a ordem de chaves que o Kotlin emite (medida num token
// real). Nao e contrato -- o token e opaco para o cliente --, mas mantem os
// dois lados indistinguiveis.
type accessPayload struct {
	Sub      string   `json:"sub"`
	Exp      int64    `json:"exp"`
	Type     string   `json:"type"`
	Iat      int64    `json:"iat"`
	Roles    []string `json:"roles"`
	Username string   `json:"username"`
}

type refreshPayload struct {
	Sub  string `json:"sub"`
	Exp  int64  `json:"exp"`
	Type string `json:"type"`
	Iat  int64  `json:"iat"`
}

func (t *Tokens) Access(userID, username string, roles []string) string {
	now := t.now()
	return t.sign(accessPayload{
		Sub: userID, Exp: now.Add(AccessTokenValidity).Unix(), Type: "access",
		Iat: now.Unix(), Roles: roles, Username: username,
	})
}

func (t *Tokens) Refresh(userID string) string {
	now := t.now()
	return t.sign(refreshPayload{
		Sub: userID, Exp: now.Add(RefreshTokenValidity).Unix(), Type: "refresh", Iat: now.Unix(),
	})
}

func (t *Tokens) sign(payload any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload) // structs de strings e inteiros: nao falha
	signing := headerSegment + "." +
		base64.RawURLEncoding.EncodeToString(bytes.TrimSuffix(buf.Bytes(), []byte("\n")))
	return signing + "." + base64.RawURLEncoding.EncodeToString(t.mac(signing))
}

func (t *Tokens) mac(signing string) []byte {
	h := hmac.New(sha256.New, t.secret)
	h.Write([]byte(signing))
	return h.Sum(nil)
}

// TokenError e a recusa de um token, com a descricao que o Spring Security
// escreve no error_description do WWW-Authenticate.
type TokenError struct{ Description string }

func (e *TokenError) Error() string { return e.Description }

const decodePrefix = "An error occurred while attempting to decode the Jwt: "

func decodeErr(detail string) *TokenError { return &TokenError{decodePrefix + detail} }

// Verify valida um token como o NimbusJwtDecoder do Spring Security, na mesma
// ordem e com as mesmas mensagens (todas medidas contra o Kotlin):
//
//	formato / cabecalho ilegivel      Malformed token
//	alg "none"                        Unsupported algorithm of none   (sem o prefixo)
//	payload que nao e objeto JSON     Malformed payload
//	alg diferente de HS256            Signed JWT rejected: Another algorithm expected, ...
//	assinatura errada                 Signed JWT rejected: Invalid signature
//	exp <= iat                        expiresAt must be after issuedAt
//	expirado (com 60 s de folga)      Jwt expired at 2026-11-15T22:43:12Z
//	nbf no futuro (com 60 s de folga) Jwt used before ...
func (t *Tokens) Verify(raw string) (Claims, *TokenError) {
	claims, terr := t.verifySignature(raw)
	if terr != nil {
		return Claims{}, terr
	}
	exp, hasExp := numericDate(claims["exp"])
	iat, hasIat := numericDate(claims["iat"])
	nbf, hasNbf := numericDate(claims["nbf"])
	if hasExp && hasIat && !exp.After(iat) {
		return Claims{}, decodeErr("expiresAt must be after issuedAt")
	}
	now := t.now()
	if hasExp && now.Add(-clockSkew).After(exp) {
		return Claims{}, decodeErr("Jwt expired at " + httpx.Instant(exp).Format())
	}
	if hasNbf && now.Add(clockSkew).Before(nbf) {
		return Claims{}, decodeErr("Jwt used before " + httpx.Instant(nbf).Format())
	}
	return claimsOf(claims), nil
}

// VerifyIgnoringTime confere formato e ASSINATURA, mas nao a validade. E o que
// o /auth/refresh usa para renovar a sessao de um anonimo cujo refresh token
// venceu: o token precisa ter sido emitido por este servidor, so nao precisa
// estar no prazo.
func (t *Tokens) VerifyIgnoringTime(raw string) (Claims, bool) {
	claims, terr := t.verifySignature(raw)
	if terr != nil {
		return Claims{}, false
	}
	return claimsOf(claims), true
}

// UnverifiedClaims le o conteudo de um token SEM conferir assinatura nem
// validade -- qualquer um pode forjar o que isto devolve. So existe para
// reproduzir, sob opcao explicita, o comportamento do Kotlin no /auth/refresh
// (ver Service.Refresh).
func UnverifiedClaims(raw string) (Claims, bool) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return Claims{}, false
	}
	var header, claims map[string]any
	if !decodeSegment(parts[0], &header) || header == nil {
		return Claims{}, false
	}
	if alg, ok := header["alg"].(string); !ok || alg == "none" {
		return Claims{}, false
	}
	if !decodeSegment(parts[1], &claims) || claims == nil {
		return Claims{}, false
	}
	return claimsOf(claims), true
}

func claimsOf(claims map[string]any) Claims {
	c := Claims{Subject: str(claims["sub"]), Username: str(claims["username"]), Roles: roles(claims["roles"])}
	c.Type, c.HasType = claims["type"].(string)
	return c
}

func (t *Tokens) verifySignature(raw string) (map[string]any, *TokenError) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, decodeErr("Malformed token")
	}
	var header map[string]any
	if !decodeSegment(parts[0], &header) || header == nil {
		return nil, decodeErr("Malformed token")
	}
	alg, ok := header["alg"].(string)
	if !ok {
		return nil, decodeErr("Malformed token")
	}
	if alg == "none" {
		if parts[2] != "" {
			return nil, decodeErr("Malformed token")
		}
		return nil, &TokenError{"Unsupported algorithm of none"}
	}
	if typ, present := header["typ"]; present && typ != nil {
		if s, _ := typ.(string); !strings.EqualFold(s, "JWT") {
			return nil, decodeErr("JOSE header typ (type) " + s + " not allowed")
		}
	}
	var claims map[string]any
	if !decodeSegment(parts[1], &claims) || claims == nil {
		return nil, decodeErr("Malformed payload")
	}
	if alg != "HS256" {
		return nil, decodeErr("Signed JWT rejected: Another algorithm expected, or no matching key(s) found")
	}
	sig, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[2], "="))
	if err != nil || !hmac.Equal(sig, t.mac(parts[0]+"."+parts[1])) {
		return nil, decodeErr("Signed JWT rejected: Invalid signature")
	}
	return claims, nil
}

func decodeSegment(seg string, into *map[string]any) bool {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(seg, "="))
	if err != nil {
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	return dec.Decode(into) == nil
}

func numericDate(v any) (time.Time, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return time.Time{}, false
	}
	if i, err := n.Int64(); err == nil {
		return time.Unix(i, 0), true
	}
	f, err := n.Float64()
	if err != nil {
		return time.Time{}, false
	}
	return time.UnixMilli(int64(f * 1000)), true
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

func str(v any) string {
	s, _ := v.(string)
	return s
}
