package auth

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func segment(json string) string { return base64.RawURLEncoding.EncodeToString([]byte(json)) }

// signed monta um token com o cabecalho e o payload dados, assinado de verdade.
func signed(tok *Tokens, header, payload string) string {
	signing := segment(header) + "." + segment(payload)
	return signing + "." + base64.RawURLEncoding.EncodeToString(tok.mac(signing))
}

// O formato dos tokens emitidos e o do Kotlin, chave por chave (medido num
// token real de producao): sem "typ" no cabecalho, e o refresh sem roles nem
// username.
func TestFormatoDosTokensEmitidos(t *testing.T) {
	at := time.Unix(1789488875, 0)
	tok := newTestTokens(at)

	parts := strings.Split(tok.Access(testUser, "user_demo", []string{"USER"}), ".")
	header, _ := base64.RawURLEncoding.DecodeString(parts[0])
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	if string(header) != `{"alg":"HS256"}` {
		t.Errorf("cabecalho: %s", header)
	}
	want := `{"sub":"` + testUser + `","exp":1789489775,"type":"access","iat":1789488875,"roles":["USER"],"username":"user_demo"}`
	if string(payload) != want {
		t.Errorf("access:\n  veio     %s\n  esperava %s", payload, want)
	}

	parts = strings.Split(tok.Refresh(testUser), ".")
	payload, _ = base64.RawURLEncoding.DecodeString(parts[1])
	want = `{"sub":"` + testUser + `","exp":1790784875,"type":"refresh","iat":1789488875}` // 15 dias
	if string(payload) != want {
		t.Errorf("refresh:\n  veio     %s\n  esperava %s", payload, want)
	}
}

// As mensagens vao para o cliente, no WWW-Authenticate. Sao as do Spring
// Security, medidas uma a uma contra o Kotlin.
func TestMensagensDeTokenRecusado(t *testing.T) {
	now := time.Unix(1789488875, 0)
	tok := newTestTokens(now)
	const prefix = "An error occurred while attempting to decode the Jwt: "
	ok := `{"sub":"x","iat":1789488000,"exp":1789489000}`
	cases := []struct{ name, token, want string }{
		{"sem pontos", "isto-nao-e-um-token", prefix + "Malformed token"},
		{"duas partes", "a.b", prefix + "Malformed token"},
		{"cabecalho ilegivel", "!!!." + segment(ok) + ".x", prefix + "Malformed token"},
		{"cabecalho sem alg", signed(tok, `{"typ":"JWT"}`, ok), prefix + "Malformed token"},
		{"alg none", segment(`{"alg":"none"}`) + "." + segment(ok) + ".", "Unsupported algorithm of none"},
		{"payload que nao e JSON", signed(tok, `{"alg":"HS256"}`, `nao e json`), prefix + "Malformed payload"},
		{"outro algoritmo", signed(tok, `{"alg":"HS512"}`, ok),
			prefix + "Signed JWT rejected: Another algorithm expected, or no matching key(s) found"},
		{"assinatura de outro segredo", signed(NewTokens("outro-segredo-qualquer-com-mais-de-32-bytes"), `{"alg":"HS256"}`, ok),
			prefix + "Signed JWT rejected: Invalid signature"},
		{"typ estranho", signed(tok, `{"alg":"HS256","typ":"at+jwt"}`, ok), prefix + "JOSE header typ (type) at+jwt not allowed"},
		{"exp antes do iat", signed(tok, `{"alg":"HS256"}`, `{"sub":"x","iat":1789489000,"exp":1789488000}`),
			prefix + "expiresAt must be after issuedAt"},
		{"vencido", signed(tok, `{"alg":"HS256"}`, `{"sub":"x","iat":1789480000,"exp":1789488000}`),
			prefix + "Jwt expired at 2026-09-15T16:00:00Z"},
		{"ainda nao vale", signed(tok, `{"alg":"HS256"}`, `{"sub":"x","nbf":1789489000,"exp":1789499000}`),
			prefix + "Jwt used before 2026-09-15T16:16:40Z"},
	}
	for _, c := range cases {
		_, terr := tok.Verify(c.token)
		if terr == nil || terr.Description != c.want {
			t.Errorf("%s:\n  veio     %v\n  esperava %s", c.name, terr, c.want)
		}
	}
	// typ "JWT" e aceito, com qualquer caixa
	if _, terr := tok.Verify(signed(tok, `{"alg":"HS256","typ":"jwt"}`, ok)); terr != nil {
		t.Errorf("typ jwt deveria ser aceito: %v", terr)
	}
}

// VerifyIgnoringTime: o que o /auth/refresh usa para o anonimo com refresh
// vencido. A validade e relevada; a assinatura, nunca.
func TestVerifyIgnoringTime(t *testing.T) {
	tok := newTestTokens(time.Unix(1789488875, 0))
	vencido := signed(tok, `{"alg":"HS256"}`, `{"sub":"`+testUser+`","type":"refresh","iat":1700000000,"exp":1700000900}`)
	if _, terr := tok.Verify(vencido); terr == nil {
		t.Fatal("o token esta vencido: Verify deveria recusar")
	}
	claims, ok := tok.VerifyIgnoringTime(vencido)
	if !ok || claims.Subject != testUser || claims.Type != "refresh" {
		t.Errorf("vencido mas assinado deveria ser lido: %+v ok=%v", claims, ok)
	}
	forjado := signed(NewTokens("segredo-de-quem-nao-e-o-servidor-0123456789"), `{"alg":"HS256"}`,
		`{"sub":"`+testUser+`","type":"refresh","iat":1700000000,"exp":1700000900}`)
	if _, ok := tok.VerifyIgnoringTime(forjado); ok {
		t.Errorf("token forjado nao pode passar em VerifyIgnoringTime")
	}
	// UnverifiedClaims le qualquer coisa -- por isso so entra sob opcao explicita
	if claims, ok := UnverifiedClaims(forjado); !ok || claims.Subject != testUser {
		t.Errorf("UnverifiedClaims deveria ler o token forjado: %+v ok=%v", claims, ok)
	}
}
