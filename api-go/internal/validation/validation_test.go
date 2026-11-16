package validation

import (
	"encoding/json"
	"os"
	"testing"
)

type oracleFile struct {
	Messages map[string]map[string]string `json:"messages"`
	Emails   map[string]bool              `json:"emails"`
}

func loadOracle(t *testing.T) oracleFile {
	t.Helper()
	raw, err := os.ReadFile("testdata/oracle.json")
	if err != nil {
		t.Fatal(err)
	}
	var o oracleFile
	if err := json.Unmarshal(raw, &o); err != nil {
		t.Fatal(err)
	}
	return o
}

// Cada veredito veio do EmailValidator real do Hibernate Validator 8.0.3.
func TestEmailIgualAoHibernateValidator(t *testing.T) {
	o := loadOracle(t)
	if len(o.Emails) < 60 {
		t.Fatalf("gabarito incompleto: %d emails", len(o.Emails))
	}
	for email, want := range o.Emails {
		if got := IsEmail(email); got != want {
			t.Errorf("IsEmail(%q): hibernate=%v go=%v", email, want, got)
		}
	}
}

func TestMensagensIguaisAoGabarito(t *testing.T) {
	o := loadOracle(t)
	for bundle, msgs := range o.Messages {
		for k, want := range msgs {
			if got := Message(bundle, k); got != want {
				t.Errorf("[%s] %s: %q != %q", bundle, k, got, want)
			}
		}
	}
}

// Medido no Kotlin local com a JVM em en_US, como a imagem de producao.
func TestIdiomaPeloAcceptLanguage(t *testing.T) {
	for header, want := range map[string]string{
		"":                                    "must not be blank",
		"pt-BR":                               "não deve estar em branco",
		"pt":                                  "não deve estar em branco",
		"pt-PT":                               "não deve estar em branco",
		"pt-BR,pt;q=0.9,en-US;q=0.8,en;q=0.7": "não deve estar em branco",
		"en-US,pt-BR;q=0.5":                   "must not be blank",
		"es":                                  "no debe estar vacío",
		"xx-YY":                               "must not be blank",
	} {
		if got := Message(Bundle(header), "NotBlank"); got != want {
			t.Errorf("Accept-Language %q: %q, esperava %q", header, got, want)
		}
	}
	if got := SizeMessage(Bundle("pt-BR"), 6, 100); got != "tamanho deve ser entre 6 e 100" {
		t.Errorf("Size pt-BR: %q", got)
	}
}

func TestNotBlankELen(t *testing.T) {
	if NotBlank(" \t\n") || !NotBlank(" a ") || !NotBlank(string(rune(0xA0))) {
		t.Error("NotBlank deve seguir o trim() do Java (so <= U+0020)")
	}
	if Len("abc") != 3 || Len(string(rune(0x1F600))) != 2 {
		t.Error("Len deve contar unidades UTF-16")
	}
}
