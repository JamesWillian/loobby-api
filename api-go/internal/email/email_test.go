package email

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendFalaComOResendComoOKotlin(t *testing.T) {
	var got struct {
		method, path, auth, contentType string
		body                            map[string]any
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path = r.Method, r.URL.Path
		got.auth, got.contentType = r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got.body)
		_, _ = w.Write([]byte(`{"id":"abc"}`))
	}))
	defer srv.Close()

	html := "<a href=\"https://api.loobby.app/x?token=1\">\n  Confirmar \\ email\r\n</a>"
	err := NewResend("re_chave", "Loobby <noreply@mail.loobby.app>", srv.URL).
		Send(context.Background(), "fulano@loobby.app", "Redefinir senha — Loobby", html)
	if err != nil {
		t.Fatal(err)
	}
	if got.method != "POST" || got.path != "/emails" || got.auth != "Bearer re_chave" || got.contentType != "application/json" {
		t.Errorf("requisicao: %+v", got)
	}
	to, _ := got.body["to"].([]any)
	if got.body["from"] != "Loobby <noreply@mail.loobby.app>" || len(to) != 1 || to[0] != "fulano@loobby.app" ||
		got.body["subject"] != "Redefinir senha — Loobby" || got.body["html"] != html {
		t.Errorf("corpo: %v", got.body)
	}
}

// O Kotlin monta o JSON por concatenacao e quebra com aspas no destinatario ou
// no assunto. Aqui e um codificador de verdade.
func TestSendEscapaOQueOKotlinNaoEscapa(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
		}
	}))
	defer srv.Close()
	err := NewResend("k", "Loobby", srv.URL).Send(context.Background(), `"aspas"@loobby.app`, `assunto "com aspas"`, "<p>x</p>")
	if err != nil || body["subject"] != `assunto "com aspas"` {
		t.Errorf("err=%v corpo=%v", err, body)
	}
}

// Resposta que nao e 2xx vira erro -- e, para quem chamou a API, 500.
func TestSendFalha(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"dominio nao verificado"}`))
	}))
	err := NewResend("k", "Loobby", srv.URL).Send(context.Background(), "a@b.c", "s", "h")
	if err == nil || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "dominio nao verificado") {
		t.Errorf("esperava erro com o status e o motivo, veio %v", err)
	}
	srv.Close()
	if err := NewResend("k", "Loobby", srv.URL).Send(context.Background(), "a@b.c", "s", "h"); err == nil {
		t.Errorf("servidor fora do ar deveria dar erro")
	}
}
