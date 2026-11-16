package httpx

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestViolacoes(t *testing.T) {
	str := func(v string) *string { return &v }
	req := httptest.NewRequest("POST", "/auth/register", nil)

	v := NewViolations(req)
	v.Email("email", str("fulano@loobby.app"))
	v.Email("email", str("")) // quem barra o vazio e o NotBlank
	v.Email("email", nil)
	v.NotBlank("nome", str("ok"))
	v.Size("senha", str("123456"), 6, 100)
	v.Size("senha", nil, 6, 100) // null e valido para o Size
	future := time.Now().Add(time.Hour)
	v.FutureOrPresent("quando", &future)
	v.FutureOrPresent("quando", nil)
	if v.Any() {
		t.Fatalf("nada disto e violacao")
	}

	past := time.Now().Add(-time.Hour)
	v.Email("email", str("nao-e-email"))
	v.NotBlank("email", str("   "))
	v.NotBlank("password", nil)
	v.Size("password", str("abc"), 6, 100)
	v.SizeMsg("username", str("ab"), 3, 30, "username must be between 3 and 30 characters")
	v.FutureOrPresent("scheduledDatetime", &past)
	rec := httptest.NewRecorder()
	v.Write(rec, req)

	want := `{"timestamp":"T","status":400,"error":"Bad Request","message":"Validation failed","path":"/auth/register",` +
		`"fieldErrors":[` +
		`{"field":"email","message":"must be a well-formed email address"},` +
		`{"field":"email","message":"must not be blank"},` +
		`{"field":"password","message":"must not be blank"},` +
		`{"field":"password","message":"size must be between 6 and 100"},` +
		`{"field":"username","message":"username must be between 3 and 30 characters"},` +
		`{"field":"scheduledDatetime","message":"must be a date in the present or in the future"}]}`
	if rec.Code != 400 || envelope(rec) != want {
		t.Errorf("veio     %d %s\nesperava 400 %s", rec.Code, envelope(rec), want)
	}
}

// As mensagens padrao do Hibernate Validator seguem o Accept-Language (medido);
// as escritas no DTO, nao.
func TestViolacoesTraduzidas(t *testing.T) {
	req := httptest.NewRequest("POST", "/groups", nil)
	req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9")
	v := NewViolations(req)
	empty := ""
	v.NotBlank("name", &empty)
	v.SizeMsg("username", &empty, 3, 30, "username must be between 3 and 30 characters")
	rec := httptest.NewRecorder()
	v.Write(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, `{"field":"name","message":"não deve estar em branco"}`) ||
		!strings.Contains(body, `{"field":"username","message":"username must be between 3 and 30 characters"}`) {
		t.Errorf("mensagens: %s", body)
	}
}
