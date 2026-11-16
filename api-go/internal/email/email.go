// Package email envia os emails transacionais pelo Resend. Porta do
// EmailService do Kotlin.
package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Sender envia um email HTML.
type Sender interface {
	Send(ctx context.Context, to, subject, html string) error
}

// Resend fala com a API do Resend (POST /emails).
type Resend struct {
	apiKey  string
	from    string
	baseURL string
	client  *http.Client
}

// NewResend: baseURL e https://api.resend.com em producao; trocavel para teste.
func NewResend(apiKey, from, baseURL string) *Resend {
	return &Resend{apiKey: apiKey, from: from, baseURL: baseURL, client: &http.Client{Timeout: 15 * time.Second}}
}

// Send devolve erro quando o Resend nao responde 2xx. Quem chama deixa o erro
// subir: no Kotlin isso e uma RuntimeException, e a resposta ao cliente e 500.
//
// O corpo e montado com um codificador JSON de verdade. O Kotlin monta por
// concatenacao de texto e so escapa o HTML -- um destinatario ou assunto com
// aspas quebraria o JSON de la (contract/ACHADOS.md). Para os valores que o
// Loobby realmente envia, os dois produzem o mesmo conteudo.
func (r *Resend) Send(ctx context.Context, to, subject, html string) error {
	body, err := json.Marshal(struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		HTML    string   `json:"html"`
	}{r.from, []string{to}, subject, html})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/emails", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("falha ao enviar email: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("falha ao enviar email: Resend respondeu %d: %s", resp.StatusCode, detail)
	}
	return nil
}
