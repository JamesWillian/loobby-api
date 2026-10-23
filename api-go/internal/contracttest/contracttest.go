// Package contracttest compara respostas do Go com os golden gravados da API
// Kotlin em producao (contract/golden). E o criterio de aceite de cada modulo
// migrado: se o teste passa, o cliente nao consegue distinguir os dois lados.
package contracttest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ContractHeaders espelha KEPT_HEADERS de contract/capture.py. Os dois precisam
// andar juntos: um header gravado la e ignorado aqui seria divergencia invisivel.
var ContractHeaders = []string{
	"content-type", "location", "www-authenticate", "allow",
	"cache-control", "pragma", "expires",
	"x-content-type-options", "x-xss-protection", "x-frame-options",
	"strict-transport-security",
	"vary",
}

type Golden struct {
	Name     string
	Request  struct{ Method, Path string } `json:"request"`
	Response struct {
		Status     int               `json:"status"`
		Headers    map[string]string `json:"headers"`
		BodyRaw    *string           `json:"body_raw"`
		BodyEmpty  bool              `json:"body_empty"`
		BodyBinary *struct {
			Bytes  int    `json:"bytes"`
			SHA256 string `json:"sha256"`
		} `json:"body_binary"`
	} `json:"response"`
}

// Load le contract/golden/<name>.json. Pula o teste se a baseline nao existir --
// o modulo Go pode ser compilado fora do repo, mas sem golden o teste nao prova
// nada e nao deve passar fingindo que provou.
func Load(t *testing.T, name string) Golden {
	t.Helper()
	dir := goldenDir(t)
	raw, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if err != nil {
		t.Fatalf("golden %q ausente em %s: %v (rode contract/capture.py)", name, dir, err)
	}
	var g Golden
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatalf("golden %q ilegivel: %v", name, err)
	}
	g.Name = name
	return g
}

// AssertMatches exige status, headers de contrato e corpo identicos ao golden.
// Os headers sao conferidos nos DOIS sentidos: faltar um que o Kotlin manda e
// mandar um que o Kotlin nao manda sao igualmente divergencia.
func AssertMatches(t *testing.T, g Golden, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != g.Response.Status {
		t.Errorf("%s: status kotlin=%d go=%d", g.Name, g.Response.Status, rec.Code)
	}
	for _, name := range ContractHeaders {
		want := g.Response.Headers[name]
		got := strings.Join(rec.Header().Values(name), ", ")
		if want != got {
			t.Errorf("%s: header %s\n  kotlin: %q\n  go    : %q", g.Name, name, want, got)
		}
	}

	body := rec.Body.Bytes()
	switch {
	case g.Response.BodyBinary != nil:
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != g.Response.BodyBinary.SHA256 {
			t.Errorf("%s: corpo binario divergiu (%d bytes no go, %d no kotlin)",
				g.Name, len(body), g.Response.BodyBinary.Bytes)
		}
	case g.Response.BodyEmpty:
		if len(body) != 0 {
			t.Errorf("%s: kotlin responde corpo vazio, go mandou %d bytes", g.Name, len(body))
		}
	case g.Response.BodyRaw != nil:
		if string(body) != *g.Response.BodyRaw {
			t.Errorf("%s: corpo divergiu\n  kotlin: %s\n  go    : %s",
				g.Name, clip(*g.Response.BodyRaw), clip(string(body)))
		}
	}
}

func goldenDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "contract", "golden")
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate
		}
		if filepath.Dir(dir) == dir {
			t.Skip("contract/golden nao encontrado acima de " + wd)
		}
	}
}

func clip(s string) string {
	if len(s) > 240 {
		return s[:240] + "…"
	}
	return s
}
