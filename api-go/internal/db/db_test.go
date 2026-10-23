package db

import (
	"context"
	"os"
	"testing"
)

// Integracao: precisa de um Postgres (ver api-go/README.md). Pula sem um.
func testURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL nao definida")
	}
	return url
}

// O pgx, pelo protocolo binario, le numeric 0.00 como "0". O Kotlin responde
// "0.00". Este teste trava que, pelo pool do Connect, a escala sobrevive --
// inclusive a do zero, que e a nota de boa parte dos jogos.
func TestNumericPreservaAEscala(t *testing.T) {
	ctx := context.Background()
	pool, err := Connect(ctx, testURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	for lit, want := range map[string]string{
		"0.00::numeric(4,2)":    "0.00",
		"0::numeric(12,2)":      "0.00",
		"25.50::numeric(12,2)":  "25.50",
		"100::numeric(12,2)":    "100.00",
		"4.38::numeric(4,2)":    "4.38",
		"(-0.00)::numeric(4,2)": "0.00",
	} {
		var got *string
		if err := pool.QueryRow(ctx, "SELECT "+lit).Scan(&got); err != nil {
			t.Fatalf("%s: %v", lit, err)
		}
		if got == nil || *got != want {
			t.Errorf("%s: esperava %q, veio %v", lit, want, got)
		}
	}

	var null *string
	if err := pool.QueryRow(ctx, "SELECT NULL::numeric").Scan(&null); err != nil || null != nil {
		t.Errorf("NULL numeric: esperava nil, veio %v (err %v)", null, err)
	}

	// e no sentido da gravacao: parametro string vira numeric com a escala da coluna
	rating := "0.0"
	var back string
	if err := pool.QueryRow(ctx, "SELECT ($1::numeric(4,2))", &rating).Scan(&back); err != nil {
		t.Fatal(err)
	}
	if back != "0.00" {
		t.Errorf("gravar \"0.0\" em numeric(4,2): voltou %q", back)
	}
}
