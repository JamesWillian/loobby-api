package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// O UUID.fromString do Java e bem mais tolerante do que o formato canonico --
// medido no Kotlin com o `sub` do token e com os {id} do caminho.
func TestParseJavaUUID(t *testing.T) {
	aceitos := map[string]string{
		"54f29b67-cb8b-4435-8b72-43c17f2f4b8f": "54f29b67-cb8b-4435-8b72-43c17f2f4b8f",
		"54F29B67-CB8B-4435-8B72-43C17F2F4B8F": "54f29b67-cb8b-4435-8b72-43c17f2f4b8f",
		"1-1-1-1-1":                            "00000001-0001-0001-0001-000000000001",
		"0-0-0-0-0":                            "00000000-0000-0000-0000-000000000000",
		"123456789-1-1-1-1":                    "23456789-0001-0001-0001-000000000001", // grupo longo perde a esquerda
		"1-12345-1-1-1":                        "00000001-2345-0001-0001-000000000001",
		"+1-1-1-1-1":                           "00000001-0001-0001-0001-000000000001",
	}
	for in, want := range aceitos {
		got, ok := ParseJavaUUID(in)
		if !ok || got != want {
			t.Errorf("%q: veio %q (ok=%v), esperava %q", in, got, ok, want)
		}
	}
	for _, in := range []string{
		"", "xyz", "1-1-1-1", "1-1-1-1-1-1", "g-1-1-1-1", "1--1-1-1", "++1-1-1-1-1",
		"nao-e-um-uuid-valido", strings.Repeat("1", 37),
		"00000000-0000-0000-0000-00000000000000", // 38 caracteres
	} {
		if got, ok := ParseJavaUUID(in); ok {
			t.Errorf("%q deveria ser recusado, virou %q", in, got)
		}
	}
}

// {id} que nao e UUID: MethodArgumentTypeMismatchException no Kotlin, que cai
// no catch-all -- 500, nao 400 (medido com GET /groups/xyz).
func TestPathIDInvalidoResponde500(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /groups/{id}", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := PathID(w, r, "id"); ok {
			_, _ = w.Write([]byte(id))
		}
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/groups/xyz", nil))
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), `"message":"Unexpected error"`) {
		t.Errorf("esperava 500 Unexpected error, veio %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/groups/%201-1-1-1-1%20", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "00000001-0001-0001-0001-000000000001" {
		t.Errorf("id tolerado pelo Java deveria ser aceito e normalizado, veio %d %s", rec.Code, rec.Body)
	}
}
