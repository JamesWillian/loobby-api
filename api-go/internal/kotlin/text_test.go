package kotlin

import "testing"

// Onde o trim() do Kotlin e o TrimSpace do Go discordam: os controles
// U+001C..U+001F sao espaco para o Kotlin e nao para o Go; U+0085 (NEL) e
// espaco para o Go e nao para o Kotlin.
func TestTrim(t *testing.T) {
	cases := map[string]string{
		"  nome  ":               "nome",
		"\t\n nome \r\n":         "nome",
		"\u00a0nome\u00a0":       "nome",
		"\u2003nome\u2028":       "nome",
		"\x1cnome\x1f":           "nome",
		"\u0085nome\u0085":       "\u0085nome\u0085",
		"no meio fica":           "no meio fica",
		"\u200bzero-width\u200b": "\u200bzero-width\u200b",
	}
	for in, want := range cases {
		if got := Trim(in); got != want {
			t.Errorf("Trim(%q) = %q, esperava %q", in, got, want)
		}
	}
}

func TestBlank(t *testing.T) {
	for _, s := range []string{"", " ", "\t\n", "\u00a0", "\x1c"} {
		if !IsBlank(s) || IfBlankNil(s) != nil {
			t.Errorf("%q deveria ser branco", s)
		}
	}
	if IsBlank("\u0085") {
		t.Errorf("U+0085 nao e espaco para o Kotlin")
	}
	// ifBlank devolve o texto original, sem aparar
	if got := IfBlankNil(" a "); got == nil || *got != " a " {
		t.Errorf("IfBlankNil nao deve aparar: %v", got)
	}
}
