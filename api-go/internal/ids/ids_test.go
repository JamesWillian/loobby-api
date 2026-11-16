package ids

import (
	"regexp"
	"testing"
)

func TestNewEUUIDv4NoFormatoDoJava(t *testing.T) {
	v4 := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	seen := map[string]bool{}
	for range 1000 {
		id := New()
		if !v4.MatchString(id) {
			t.Fatalf("%q nao e um UUID v4 em minusculas", id)
		}
		if seen[id] {
			t.Fatalf("%q repetido", id)
		}
		seen[id] = true
	}
}

// O formato do codigo de convite e contrato: $-XXXXXX para grupo, $-XXXX-XXXX
// para evento, sempre A-Z e 0-9.
func TestCode(t *testing.T) {
	only := regexp.MustCompile(`^[A-Z0-9]{6}$`)
	distinct := map[string]bool{}
	for range 200 {
		c := Code(6)
		if !only.MatchString(c) {
			t.Fatalf("%q fora do alfabeto ou do tamanho", c)
		}
		distinct[c] = true
	}
	if len(distinct) < 190 {
		t.Errorf("so %d codigos distintos em 200: o sorteio esta viciado", len(distinct))
	}
	if Code(0) != "" {
		t.Errorf("Code(0) deveria ser vazio")
	}
}
