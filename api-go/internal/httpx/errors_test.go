package httpx

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestEnvelopeMatchesGolden le os golden gravados da API Kotlin em producao e
// exige que o tipo Go, ao re-serializar o mesmo conteudo, produza JSON IDENTICO
// byte a byte. E o que prova ordem de campos, null vs chave ausente e o formato
// do timestamp -- sem depender do que eu acho que o Jackson faz.
func TestEnvelopeMatchesGolden(t *testing.T) {
	paths, err := filepath.Glob("../../../contract/golden/*.json")
	if err != nil || len(paths) == 0 {
		t.Skipf("baseline de contrato nao encontrada (%v golden); rode contract/capture.py", len(paths))
	}

	checked := 0
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		var g struct {
			Response struct {
				Status  int    `json:"status"`
				BodyRaw string `json:"body_raw"`
			} `json:"response"`
		}
		if err := json.Unmarshal(raw, &g); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if g.Response.Status < 400 || g.Response.BodyRaw == "" {
			continue
		}
		// So os envelopes de erro; /legal e as paginas HTML de auth ficam de fora.
		if !bytes.HasPrefix([]byte(g.Response.BodyRaw), []byte(`{"timestamp"`)) {
			continue
		}

		var got ErrorResponse
		if err := json.Unmarshal([]byte(g.Response.BodyRaw), &got); err != nil {
			t.Errorf("%s: nao desserializou no ErrorResponse: %v", filepath.Base(p), err)
			continue
		}
		out, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if string(out) != g.Response.BodyRaw {
			t.Errorf("%s: round-trip divergiu\n  kotlin: %s\n  go    : %s",
				filepath.Base(p), g.Response.BodyRaw, out)
			continue
		}
		checked++
	}

	if checked == 0 {
		t.Fatal("nenhum envelope de erro conferido -- o teste nao provou nada")
	}
	t.Logf("%d envelopes de erro identicos ao Kotlin", checked)
}

// TestInstantGrouping trava o agrupamento 0/3/6/9 do ISO_INSTANT. O default do
// Go (RFC3339Nano) falharia nos casos com zero a direita.
func TestInstantGrouping(t *testing.T) {
	base := time.Date(2026, 9, 8, 16, 14, 36, 0, time.UTC)
	cases := []struct {
		ns   int
		want string
	}{
		{0, "2026-09-08T16:14:36Z"},
		{500_000_000, "2026-09-08T16:14:36.500Z"},
		{211_055_000, "2026-09-08T16:14:36.211055Z"},
		{699_860_759, "2026-09-08T16:14:36.699860759Z"},
		// o caso que o RFC3339Nano quebraria: .211050 viraria .21105 (5 digitos)
		{211_050_000, "2026-09-08T16:14:36.211050Z"},
	}
	for _, c := range cases {
		got := Instant(base.Add(time.Duration(c.ns))).Format()
		if got != c.want {
			t.Errorf("ns=%d: got %s, want %s", c.ns, got, c.want)
		}
		if std := base.Add(time.Duration(c.ns)).Format(time.RFC3339Nano); std != c.want {
			t.Logf("  (RFC3339Nano daria %s -- por isso o tipo proprio)", std)
		}
	}
}
