package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf16"

	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
)

// GoogleCertsURL: as chaves publicas com que o Google assina os ID tokens.
const GoogleCertsURL = "https://www.googleapis.com/oauth2/v3/certs"

// Folga de relogio padrao do GoogleIdTokenVerifier do Kotlin.
const googleClockSkew = 300 * time.Second

// GoogleVerifier confere um ID token do "Entrar com Google": assinatura RS256
// com as chaves do Google, emissor, audiencia (os client ids do app) e validade.
// Faz o papel do GoogleIdTokenVerifier da biblioteca do Google no Kotlin.
type GoogleVerifier struct {
	clientIDs map[string]bool
	certsURL  string
	client    *http.Client
	now       func() time.Time

	mu     sync.Mutex
	keys   map[string]*rsa.PublicKey
	expiry time.Time
}

func NewGoogleVerifier(clientIDs []string, certsURL string) *GoogleVerifier {
	ids := map[string]bool{}
	for _, id := range clientIDs {
		if id = strings.TrimSpace(id); id != "" {
			ids[id] = true
		}
	}
	return &GoogleVerifier{
		clientIDs: ids, certsURL: certsURL,
		client: &http.Client{Timeout: 10 * time.Second}, now: time.Now,
	}
}

// googleIdentity e o que o login usa do token.
type googleIdentity struct {
	Subject string
	Email   *string
	Name    *string
	Picture *string
}

var errGoogleInvalid = httpx.Status(http.StatusUnauthorized, "Token do Google inválido")

// Verify devolve a identidade de um ID token valido.
//
// Os erros reproduzem o Kotlin, onde a leitura do token e feita pela
// biblioteca do Google e as falhas dela escapam como excecoes diferentes
// (medido caso a caso):
//
//	sem os dois pontos, ou sem "alg"      400 com message null
//	trecho que nao e base64               400 com a mensagem da biblioteca
//	cabecalho ou payload que nao e JSON   500
//	token legivel mas nao aprovado        401 "Token do Google inválido"
func (g *GoogleVerifier) Verify(ctx context.Context, idToken string) (*googleIdentity, error) {
	null := &httpx.Error{Status: http.StatusBadRequest}
	first := strings.IndexByte(idToken, '.')
	if first < 0 {
		return nil, null
	}
	headerBytes, err := guavaBase64(idToken[:first])
	if err != nil {
		return nil, err
	}
	second := strings.IndexByte(idToken[first+1:], '.')
	if second < 0 {
		return nil, null
	}
	second += first + 1
	if strings.IndexByte(idToken[second+1:], '.') >= 0 {
		return nil, null
	}
	payloadBytes, err := guavaBase64(idToken[first+1 : second])
	if err != nil {
		return nil, err
	}
	signature, err := guavaBase64(idToken[second+1:])
	if err != nil {
		return nil, err
	}

	var header struct {
		Alg *string `json:"alg"`
		Kid string  `json:"kid"`
	}
	if err := decodeGoogleJSON(headerBytes, &header); err != nil {
		return nil, err
	}
	if header.Alg == nil {
		return nil, null
	}
	var payload struct {
		Iss     string          `json:"iss"`
		Aud     json.RawMessage `json:"aud"`
		Sub     string          `json:"sub"`
		Exp     *json.Number    `json:"exp"`
		Iat     *json.Number    `json:"iat"`
		Email   *string         `json:"email"`
		Name    json.RawMessage `json:"name"`
		Picture json.RawMessage `json:"picture"`
	}
	if err := decodeGoogleJSON(payloadBytes, &payload); err != nil {
		return nil, err
	}

	// A partir daqui, qualquer reprovacao e o mesmo 401.
	if payload.Iss != "accounts.google.com" && payload.Iss != "https://accounts.google.com" {
		return nil, errGoogleInvalid
	}
	if !g.trustedAudience(payload.Aud) {
		return nil, errGoogleInvalid
	}
	now := g.now()
	exp, okExp := seconds(payload.Exp)
	iat, okIat := seconds(payload.Iat)
	if !okExp || !okIat || now.After(exp.Add(googleClockSkew)) || now.Before(iat.Add(-googleClockSkew)) {
		return nil, errGoogleInvalid
	}
	if *header.Alg != "RS256" {
		return nil, errGoogleInvalid
	}
	keys, err := g.publicKeys(ctx)
	if err != nil {
		return nil, err // sem as chaves do Google nao ha como decidir: 500, como no Kotlin
	}
	digest := sha256.Sum256([]byte(idToken[:second]))
	verified := false
	if key, ok := keys[header.Kid]; ok {
		verified = rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature) == nil
	} else {
		for _, key := range keys {
			if rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature) == nil {
				verified = true
				break
			}
		}
	}
	if !verified {
		return nil, errGoogleInvalid
	}
	return &googleIdentity{
		Subject: payload.Sub, Email: payload.Email,
		Name: jsonString(payload.Name), Picture: jsonString(payload.Picture),
	}, nil
}

// jsonString: o `payload["name"] as? String` do Kotlin -- so vale se for texto.
func jsonString(raw json.RawMessage) *string {
	var s string
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &s) != nil {
		return nil
	}
	return &s
}

func seconds(n *json.Number) (time.Time, bool) {
	if n == nil {
		return time.Time{}, false
	}
	i, err := n.Int64()
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(i, 0), true
}

// trustedAudience: `aud` pode ser um texto ou uma lista; todos precisam ser
// client ids do app, e precisa haver pelo menos um.
func (g *GoogleVerifier) trustedAudience(raw json.RawMessage) bool {
	var list []string
	var one string
	switch {
	case json.Unmarshal(raw, &one) == nil:
		list = []string{one}
	case json.Unmarshal(raw, &list) != nil:
		return false
	}
	if len(list) == 0 {
		return false
	}
	for _, aud := range list {
		if !g.clientIDs[aud] {
			return false
		}
	}
	return true
}

// decodeGoogleJSON: texto que nao e JSON e erro de leitura no Kotlin (500); um
// JSON que nao e objeto e IllegalArgumentException de mensagem vazia (400 "").
func decodeGoogleJSON(raw []byte, into any) error {
	if !json.Valid(raw) {
		return errors.New("ID token do Google: trecho nao e JSON")
	}
	if trimmed := strings.TrimSpace(string(raw)); !strings.HasPrefix(trimmed, "{") {
		return httpx.BadRequest("")
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("ID token do Google: %w", err)
	}
	return nil
}

// guavaBase64 decodifica como a biblioteca do Google: base64 padrao ou
// "url-safe", com ou sem "=" no fim. Os erros levam a mensagem exata do Guava,
// porque o Kotlin a devolve ao cliente.
func guavaBase64(s string) ([]byte, error) {
	fail := func(detail string) error {
		return httpx.BadRequest("com.google.common.io.BaseEncoding$DecodingException: " + detail)
	}
	s = strings.TrimRight(strings.TrimSpace(s), "=")
	if len(s)%4 == 1 {
		return nil, fail("Invalid input length " + strconv.Itoa(len(s)))
	}
	for _, c := range s {
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '+', c == '/':
		case c > 0x7f || c <= 0x20 || c == 0x7f:
			return nil, fail("Unrecognized character: 0x" + strconv.FormatInt(int64(c), 16))
		default:
			return nil, fail("Unrecognized character: " + string(c))
		}
	}
	s = strings.NewReplacer("+", "-", "/", "_").Replace(s)
	out, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fail("Invalid input length " + strconv.Itoa(len(s)))
	}
	return out, nil
}

// publicKeys busca as chaves do Google e as guarda por uma hora.
func (g *GoogleVerifier) publicKeys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.keys != nil && g.now().Before(g.expiry) {
		return g.keys, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.certsURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chaves do Google: %w", err)
	}
	defer resp.Body.Close()
	var set struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&set) != nil {
		return nil, fmt.Errorf("chaves do Google: resposta %d ilegivel", resp.StatusCode)
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		n, errN := base64.RawURLEncoding.DecodeString(k.N)
		e, errE := base64.RawURLEncoding.DecodeString(k.E)
		if k.Kty != "RSA" || errN != nil || errE != nil {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	if len(keys) == 0 {
		return nil, errors.New("chaves do Google: nenhuma chave RSA na resposta")
	}
	g.keys, g.expiry = keys, g.now().Add(time.Hour)
	return keys, nil
}

// LoginWithGoogle: GoogleAuthService.loginOrRegister. Conta Google ja
// conhecida entra; desconhecida e criada (auth_provider 2) com nome e foto do
// Google.
func (s *Service) LoginWithGoogle(ctx context.Context, idToken string) (authResponse, error) {
	identity, err := s.google.Verify(ctx, idToken)
	if err != nil {
		return authResponse{}, err
	}
	if identity.Email == nil {
		return authResponse{}, httpx.Status(http.StatusBadRequest, "Email não disponível")
	}

	var user sqlc.User
	err = db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		existing, err := q.FindGoogleCredentialsByGoogleID(ctx, identity.Subject)
		if err == nil {
			user, err = q.FindUser(ctx, existing.UserID)
			if db.NotFound(err) {
				return httpx.Status(http.StatusNotFound, "User not found")
			}
			if err != nil {
				return err
			}
			// aproveita a foto do Google so se o usuario ainda nao tem avatar
			if identity.Picture != nil && (user.AvatarUrl == nil || strings.TrimSpace(*user.AvatarUrl) == "") {
				user.AvatarUrl = identity.Picture
				return q.UpdateUserAvatar(ctx, sqlc.UpdateUserAvatarParams{ID: user.ID, AvatarUrl: identity.Picture})
			}
			return nil
		}
		if !db.NotFound(err) {
			return err
		}

		displayname := *identity.Email
		if at := strings.IndexByte(displayname, '@'); at >= 0 {
			displayname = displayname[:at]
		}
		if identity.Name != nil {
			displayname = *identity.Name
		}
		user = sqlc.User{
			ID: ids.New(), Username: googleUsername(displayname), Displayname: displayname,
			AvatarUrl: identity.Picture, AuthProvider: 2,
		}
		err = q.InsertUser(ctx, sqlc.InsertUserParams{
			ID: user.ID, Username: user.Username, Displayname: user.Displayname,
			AvatarUrl: user.AvatarUrl, AuthProvider: user.AuthProvider,
		})
		if err != nil {
			return err
		}
		return q.InsertGoogleCredentials(ctx, sqlc.InsertGoogleCredentialsParams{
			UserID: user.ID, GoogleID: identity.Subject, Email: *identity.Email,
		})
	})
	if err != nil {
		return authResponse{}, err
	}
	return s.issue(user), nil
}

// googleUsername: GoogleAuthService.generateUsername -- o nome em minusculas,
// espacos virando "_", so letras, digitos e "_", cortado em 12 caracteres, mais
// "_" e 4 hexadecimais.
func googleUsername(name string) string {
	var b strings.Builder
	units := 0
	for _, r := range strings.ReplaceAll(strings.ToLower(name), " ", "_") {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_') {
			continue
		}
		units += utf16.RuneLen(r) // o take(12) do Kotlin conta em unidades UTF-16
		if units > 12 {
			break
		}
		b.WriteRune(r)
	}
	return b.String() + "_" + ids.New()[:4]
}
