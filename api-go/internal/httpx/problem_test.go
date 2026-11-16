package httpx

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

var timestampJSON = regexp.MustCompile(`"timestamp":"[^"]+"`)

func envelope(rec *httptest.ResponseRecorder) string {
	return timestampJSON.ReplaceAllString(rec.Body.String(), `"timestamp":"T"`)
}

func TestFailEscreveOEnvelopeDoKotlin(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		body   string
	}{
		{"erro de negocio", BadRequest("Group not found"), 400,
			`{"timestamp":"T","status":400,"error":"Bad Request","message":"Group not found","path":"/groups/x","fieldErrors":null}`},
		{"sem permissao", Forbidden("Only the owner can do this"), 403,
			`{"timestamp":"T","status":403,"error":"Forbidden","message":"Only the owner can do this","path":"/groups/x","fieldErrors":null}`},
		{"espera", Status(http.StatusTooManyRequests, "Aguarde"), 429,
			`{"timestamp":"T","status":429,"error":"Too Many Requests","message":"Aguarde","path":"/groups/x","fieldErrors":null}`},
		// message null e contrato: o Google devolve uma excecao sem mensagem (ACHADOS §15)
		{"mensagem nula", &Error{Status: 400}, 400,
			`{"timestamp":"T","status":400,"error":"Bad Request","message":null,"path":"/groups/x","fieldErrors":null}`},
		{"erro embrulhado", fmt.Errorf("contexto: %w", BadRequest("x")), 400,
			`{"timestamp":"T","status":400,"error":"Bad Request","message":"x","path":"/groups/x","fieldErrors":null}`},
		// qualquer outra coisa e o catch-all: o detalhe vai para o log, nunca para o cliente
		{"erro interno", errors.New("senha do banco: hunter2"), 500,
			`{"timestamp":"T","status":500,"error":"Internal Server Error","message":"Unexpected error","path":"/groups/x","fieldErrors":null}`},
		{"corpo ilegivel", ErrBody, 500,
			`{"timestamp":"T","status":500,"error":"Internal Server Error","message":"Unexpected error","path":"/groups/x","fieldErrors":null}`},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		Fail(rec, httptest.NewRequest("GET", "/groups/x", nil), c.err)
		if rec.Code != c.status || envelope(rec) != c.body {
			t.Errorf("%s:\n  veio     %d %s\n  esperava %d %s", c.name, rec.Code, envelope(rec), c.status, c.body)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s: content-type %q", c.name, ct)
		}
	}
}
