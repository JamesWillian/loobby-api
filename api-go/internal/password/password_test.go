package password

import (
	"errors"
	"strings"
	"testing"
)

// Hash gerado pelo BCryptPasswordEncoder do Kotlin para "senha-local-123" (o do
// seed do ambiente local). O Go precisa aceitar o que o Kotlin gravou.
const hashDoKotlin = "$2a$10$sl3ZCrphczJTtp1Yf3MLJeC/y/Lc3EGgTjsxwEHD1ByCJZAvPNs36"

func TestAceitaHashGravadoPeloKotlin(t *testing.T) {
	if !Matches("senha-local-123", hashDoKotlin) {
		t.Fatal("o hash do Kotlin deveria conferir")
	}
	if Matches("senha-local-124", hashDoKotlin) {
		t.Fatal("senha errada conferiu")
	}
}

func TestHashTemOFormatoQueOKotlinAceita(t *testing.T) {
	h, err := Hash("uma senha qualquer")
	if err != nil {
		t.Fatal(err)
	}
	// $2a$, custo 10: o padrao do BCryptPasswordEncoder
	if !strings.HasPrefix(h, "$2a$10$") || len(h) != 60 {
		t.Errorf("formato inesperado: %q", h)
	}
	if !Matches("uma senha qualquer", h) || Matches("outra", h) {
		t.Errorf("o hash gerado nao confere com a propria senha")
	}
	if h2, _ := Hash("uma senha qualquer"); h2 == h {
		t.Errorf("dois hashes da mesma senha sairam iguais: sem sal")
	}
}

// O limite de 72 bytes do BCrypt: ao GERAR, o Spring recusa (400 com esta
// mensagem, medido); ao CONFERIR, usa so os 72 primeiros bytes.
func TestLimiteDe72Bytes(t *testing.T) {
	if _, err := Hash(strings.Repeat("x", 72)); err != nil {
		t.Errorf("72 bytes deveria passar: %v", err)
	}
	_, err := Hash(strings.Repeat("x", 73))
	if !errors.Is(err, ErrTooLong) || ErrTooLong.Status != 400 || *ErrTooLong.Message != "password cannot be more than 72 bytes" {
		t.Errorf("73 bytes deveria dar ErrTooLong, veio %v", err)
	}
	// o limite e em BYTES: 37 letras acentuadas sao 74
	if _, err := Hash(strings.Repeat("ã", 37)); !errors.Is(err, ErrTooLong) {
		t.Errorf("74 bytes em 37 caracteres deveria dar ErrTooLong, veio %v", err)
	}
	h, _ := Hash(strings.Repeat("x", 72))
	if !Matches(strings.Repeat("x", 80), h) {
		t.Errorf("na conferencia, o que passa de 72 bytes e ignorado")
	}
}

// Hash que nao tem cara de BCrypt: o Spring avisa no log e responde "nao
// confere" -- nunca um erro.
func TestHashMalformadoNaoConfere(t *testing.T) {
	for _, h := range []string{"", "texto-puro", "$2a$10$curto", "$1$" + strings.Repeat("a", 53)} {
		if Matches("qualquer", h) {
			t.Errorf("hash %q nao deveria conferir", h)
		}
	}
}
