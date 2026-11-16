package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

const googleClient = "web.apps.googleusercontent.com"

type fakeGoogle struct {
	key     *rsa.PrivateKey
	other   *rsa.PrivateKey // chave que o "Google" nao publica
	server  *httptest.Server
	fetches atomic.Int32
	down    atomic.Bool
}

func newFakeGoogle(t *testing.T) *fakeGoogle {
	t.Helper()
	g := &fakeGoogle{}
	var err error
	if g.key, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
		t.Fatal(err)
	}
	if g.other, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
		t.Fatal(err)
	}
	g.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.fetches.Add(1)
		if g.down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		b64 := base64.RawURLEncoding.EncodeToString
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": "chave-1", "kty": "RSA", "alg": "RS256",
			"n": b64(g.key.N.Bytes()), "e": b64(big.NewInt(int64(g.key.E)).Bytes()),
		}}})
	}))
	t.Cleanup(g.server.Close)
	return g
}

func (g *fakeGoogle) verifier() *GoogleVerifier {
	return NewGoogleVerifier([]string{googleClient, " ios.apps.googleusercontent.com ", ""}, g.server.URL)
}

func (g *fakeGoogle) token(key *rsa.PrivateKey, header string, claims map[string]any) string {
	payload, _ := json.Marshal(claims)
	signing := segment(header) + "." + segment(string(payload))
	digest := sha256.Sum256([]byte(signing))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func googleClaims(overrides map[string]any) map[string]any {
	now := time.Now().Unix()
	c := map[string]any{
		"iss": "https://accounts.google.com", "aud": googleClient, "sub": "1234567890",
		"email": "fulana@gmail.com", "name": "Fulana", "picture": "https://exemplo.com/f.png",
		"iat": now - 10, "exp": now + 3600,
	}
	for k, v := range overrides {
		if v == nil {
			delete(c, k)
		} else {
			c[k] = v
		}
	}
	return c
}

const rs256 = `{"alg":"RS256","kid":"chave-1","typ":"JWT"}`

func TestGoogleTokenValido(t *testing.T) {
	g := newFakeGoogle(t)
	v := g.verifier()
	id, err := v.Verify(context.Background(), g.token(g.key, rs256, googleClaims(nil)))
	if err != nil {
		t.Fatal(err)
	}
	if id.Subject != "1234567890" || *id.Email != "fulana@gmail.com" || *id.Name != "Fulana" || *id.Picture != "https://exemplo.com/f.png" {
		t.Errorf("identidade: %+v", id)
	}
	aceitos := map[string]map[string]any{
		"emissor sem https":           {"iss": "accounts.google.com"},
		"outro client id do app":      {"aud": "ios.apps.googleusercontent.com"},
		"audiencia em lista":          {"aud": []string{googleClient}},
		"vencido ha 4 minutos":        {"exp": time.Now().Unix() - 240, "iat": time.Now().Unix() - 4000},
		"emitido 4 minutos no futuro": {"iat": time.Now().Unix() + 240},
		"sem nome e sem foto":         {"name": nil, "picture": nil},
		"nome que nao e texto":        {"name": 42},
	}
	for name, o := range aceitos {
		if _, err := v.Verify(context.Background(), g.token(g.key, rs256, googleClaims(o))); err != nil {
			t.Errorf("%s: deveria ser aceito, veio %v", name, err)
		}
	}
	// chave sem "kid" conhecido: tenta todas, como a biblioteca do Google
	if _, err := v.Verify(context.Background(), g.token(g.key, `{"alg":"RS256","kid":"desconhecida"}`, googleClaims(nil))); err != nil {
		t.Errorf("kid desconhecido deveria tentar as chaves publicadas: %v", err)
	}
	if n := g.fetches.Load(); n != 1 {
		t.Errorf("as chaves do Google deveriam ser buscadas uma vez e guardadas; foram %d", n)
	}
	// `name` que nao e texto e ignorado (o `as? String` do Kotlin)
	id, _ = v.Verify(context.Background(), g.token(g.key, rs256, googleClaims(map[string]any{"name": 42})))
	if id == nil || id.Name != nil {
		t.Errorf("name numerico deveria virar nil: %+v", id)
	}
}

// Token legivel mas reprovado: sempre o mesmo 401, sem dizer o motivo.
func TestGoogleTokenReprovado(t *testing.T) {
	g := newFakeGoogle(t)
	v := g.verifier()
	now := time.Now().Unix()
	cases := map[string]string{
		"de outro app":         g.token(g.key, rs256, googleClaims(map[string]any{"aud": "outro.apps.googleusercontent.com"})),
		"lista com outro app":  g.token(g.key, rs256, googleClaims(map[string]any{"aud": []string{googleClient, "outro"}})),
		"sem audiencia":        g.token(g.key, rs256, googleClaims(map[string]any{"aud": nil})),
		"emissor errado":       g.token(g.key, rs256, googleClaims(map[string]any{"iss": "https://accounts.exemplo.com"})),
		"vencido ha 6 minutos": g.token(g.key, rs256, googleClaims(map[string]any{"exp": now - 360, "iat": now - 4000})),
		"emitido no futuro":    g.token(g.key, rs256, googleClaims(map[string]any{"iat": now + 360})),
		"sem exp":              g.token(g.key, rs256, googleClaims(map[string]any{"exp": nil})),
		"assinado por outra":   g.token(g.other, rs256, googleClaims(nil)),
		"algoritmo trocado":    g.token(g.key, `{"alg":"HS256","kid":"chave-1"}`, googleClaims(nil)),
	}
	for name, token := range cases {
		_, err := v.Verify(context.Background(), token)
		var he *httpx.Error
		if !errors.As(err, &he) || he.Status != 401 || *he.Message != "Token do Google inválido" {
			t.Errorf("%s: esperava 401 \"Token do Google inválido\", veio %v", name, err)
		}
	}
}

// Token ILEGIVEL: no Kotlin a biblioteca do Google estoura de formas
// diferentes, e cada uma vira uma resposta diferente (medido uma a uma).
func TestGoogleTokenIlegivel(t *testing.T) {
	g := newFakeGoogle(t)
	v := g.verifier()
	guava := "com.google.common.io.BaseEncoding$DecodingException: "
	cases := []struct {
		name, token string
		status      int
		message     *string // nil = message null no envelope
	}{
		{"sem pontos", "nao-e-um-token", 400, nil},
		{"um ponto so", segment(rs256) + ".abc", 400, nil},
		{"pontos demais", "nao.e.um.id.token", 400, nil},
		{"cabecalho sem alg", segment(`{"kid":"x"}`) + "." + segment(`{}`) + ".AAAA", 400, nil},
		{"caractere fora do base64", "a!b.cd.ef", 400, strPtr(guava + "Unrecognized character: !")},
		{"tamanho impossivel", "abcde.cd.ef", 400, strPtr(guava + "Invalid input length 5")},
		{"cabecalho que e JSON mas nao objeto", segment(`"texto"`) + "." + segment(`{}`) + ".AAAA", 400, strPtr("")},
	}
	for _, c := range cases {
		_, err := v.Verify(context.Background(), c.token)
		var he *httpx.Error
		if !errors.As(err, &he) || he.Status != c.status {
			t.Errorf("%s: esperava %d, veio %v", c.name, c.status, err)
			continue
		}
		if (he.Message == nil) != (c.message == nil) || (he.Message != nil && *he.Message != *c.message) {
			t.Errorf("%s: mensagem %v, esperava %v", c.name, deref(he.Message), deref(c.message))
		}
	}
	// trecho que nao e JSON: erro de leitura no Kotlin, 500 -- aqui, um erro comum
	_, err := v.Verify(context.Background(), segment(`isto nao e json`)+"."+segment(`{}`)+".AAAA")
	var he *httpx.Error
	if err == nil || errors.As(err, &he) {
		t.Errorf("cabecalho que nao e JSON deveria ser erro interno (500), veio %v", err)
	}
}

func strPtr(s string) *string { return &s }

func deref(s *string) string {
	if s == nil {
		return "<null>"
	}
	return *s
}

// Sem as chaves do Google nao ha como decidir: e erro interno (500), nunca um
// "token invalido" -- o usuario nao fez nada errado.
func TestGoogleForaDoAr(t *testing.T) {
	g := newFakeGoogle(t)
	g.down.Store(true)
	_, err := g.verifier().Verify(context.Background(), g.token(g.key, rs256, googleClaims(nil)))
	var he *httpx.Error
	if err == nil || errors.As(err, &he) {
		t.Errorf("esperava erro interno, veio %v", err)
	}
}

// generateUsername do Kotlin: minusculas, espaco vira "_", so letras, digitos e
// "_", 12 caracteres, e um sufixo de 4 hexadecimais.
func TestGoogleUsername(t *testing.T) {
	cases := map[string]string{
		"Fulana da Silva Sauro": "fulana_da_si",
		"fluxo.sem.nome":        "fluxosemnome",
		"José Ação":             "josé_ação",
		"A":                     "a",
		"!!!":                   "",
		"Jogador 😀 Um":          "jogador__um",
	}
	for name, base := range cases {
		got := googleUsername(name)
		if !regexp.MustCompile(`^` + regexp.QuoteMeta(base) + `_[0-9a-f]{4}$`).MatchString(got) {
			t.Errorf("googleUsername(%q) = %q, esperava %q + _xxxx", name, got, base)
		}
	}
}
