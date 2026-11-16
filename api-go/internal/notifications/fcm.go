// Package notifications e a porta do modulo de notificacoes do Kotlin: registro
// de tokens de aparelho, envio de push pelo Firebase Cloud Messaging, o
// agrupamento de "fulano confirmou presenca" e os jobs agendados.
package notifications

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
)

// Payload: NotificationPayload. Data sao os extras que o app usa para abrir a
// tela certa: eventId, groupId, actorId.
type Payload struct {
	Title string
	Body  string
	Type  string
	Data  map[string]string
}

// Sender entrega um push aos aparelhos de um conjunto de usuarios.
type Sender interface {
	SendToUsers(ctx context.Context, userIDs []string, p Payload)
}

// DisabledSender e o FcmSender com loobby.notifications.enabled=false: so
// registra o que enviaria.
type DisabledSender struct{}

func (DisabledSender) SendToUsers(_ context.Context, userIDs []string, p Payload) {
	if len(userIDs) == 0 {
		return
	}
	slog.Info("notificacoes desligadas: push nao enviado",
		"tipo", p.Type, "titulo", p.Title, "corpo", p.Body, "usuarios", userIDs)
}

// serviceAccount: os campos usados do JSON de conta de servico do Firebase.
type serviceAccount struct {
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// FCM envia pela API HTTP v1 do Firebase Cloud Messaging, sem o SDK do
// Firebase: um POST por aparelho, autenticado com um token OAuth obtido com a
// chave da conta de servico. E o que o sendEachForMulticast do SDK faz por baixo.
type FCM struct {
	q        *sqlc.Queries
	client   *http.Client
	account  serviceAccount
	key      *rsa.PrivateKey
	endpoint string // https://fcm.googleapis.com, trocavel em teste

	mu     sync.Mutex
	token  string
	expiry time.Time
}

const fcmScope = "https://www.googleapis.com/auth/firebase.messaging"

// NewFCM le o JSON da conta de servico. Erro aqui deve impedir a partida, como
// no Kotlin (FirebaseConfig relanca a falha de inicializacao).
func NewFCM(db sqlc.DBTX, credentialsPath string) (*FCM, error) {
	path := strings.TrimSpace(credentialsPath)
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = home + path[1:]
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("credenciais do Firebase: %w", err)
	}
	var acc serviceAccount
	if err := json.Unmarshal(raw, &acc); err != nil {
		return nil, fmt.Errorf("credenciais do Firebase: JSON invalido: %w", err)
	}
	if acc.ProjectID == "" || acc.ClientEmail == "" || acc.PrivateKey == "" {
		return nil, errors.New("credenciais do Firebase: faltam project_id, client_email ou private_key")
	}
	if acc.TokenURI == "" {
		acc.TokenURI = "https://oauth2.googleapis.com/token"
	}
	block, _ := pem.Decode([]byte(acc.PrivateKey))
	if block == nil {
		return nil, errors.New("credenciais do Firebase: private_key nao e PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("credenciais do Firebase: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("credenciais do Firebase: private_key nao e RSA")
	}
	return &FCM{
		q: sqlc.New(db), client: &http.Client{Timeout: 15 * time.Second},
		account: acc, key: key, endpoint: "https://fcm.googleapis.com",
	}, nil
}

// SendToUsers: FcmSender.sendToUsers. Nunca devolve erro -- push e melhor
// esforco e nao pode derrubar a requisicao que o originou.
func (f *FCM) SendToUsers(ctx context.Context, userIDs []string, p Payload) {
	if len(userIDs) == 0 {
		return
	}
	tokens, err := f.q.FindDeviceTokensByUsers(ctx, userIDs)
	if err != nil {
		slog.Error("push: nao foi possivel ler os tokens", "tipo", p.Type, "err", err)
		return
	}
	if len(tokens) == 0 {
		return
	}
	access, err := f.accessToken(ctx)
	if err != nil {
		slog.Error("push: sem token de acesso do Google", "tipo", p.Type, "err", err)
		return
	}

	// title e body vao tambem em data, como no Kotlin: o app os le de la quando
	// trata a notificacao em primeiro plano.
	data := map[string]string{"type": p.Type, "title": p.Title, "body": p.Body}
	for k, v := range p.Data {
		data[k] = v
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		invalid []string
		ok, bad int
		slots   = make(chan struct{}, 8)
	)
	for _, token := range tokens {
		wg.Add(1)
		slots <- struct{}{}
		go func(token string) {
			defer wg.Done()
			defer func() { <-slots }()
			code, err := f.sendOne(ctx, access, token, p, data)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				ok++
				return
			}
			bad++
			if code == "UNREGISTERED" || code == "INVALID_ARGUMENT" {
				invalid = append(invalid, token)
			}
		}(token)
	}
	wg.Wait()
	slog.Info("push enviado", "tipo", p.Type, "sucesso", ok, "falha", bad)

	// Token que o Firebase diz nao existir mais e apagado, para nao ser tentado de novo.
	if len(invalid) > 0 {
		if err := f.q.DeleteDeviceTokens(ctx, invalid); err != nil {
			slog.Error("push: nao foi possivel apagar tokens invalidos", "err", err)
		}
	}
}

type fcmMessage struct {
	Message struct {
		Token        string            `json:"token"`
		Notification map[string]string `json:"notification"`
		Data         map[string]string `json:"data"`
	} `json:"message"`
}

// sendOne devolve o codigo de erro do FCM ("UNREGISTERED", ...) quando falha.
func (f *FCM) sendOne(ctx context.Context, access, token string, p Payload, data map[string]string) (string, error) {
	var msg fcmMessage
	msg.Message.Token = token
	msg.Message.Notification = map[string]string{"title": p.Title, "body": p.Body}
	msg.Message.Data = data
	body, _ := json.Marshal(msg)

	u := f.endpoint + "/v1/projects/" + url.PathEscape(f.account.ProjectID) + "/messages:send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode/100 == 2 {
		return "", nil
	}
	var fail struct {
		Error struct {
			Status  string `json:"status"`
			Details []struct {
				ErrorCode string `json:"errorCode"`
			} `json:"details"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &fail)
	code := ""
	for _, d := range fail.Error.Details {
		if d.ErrorCode != "" {
			code = d.ErrorCode
		}
	}
	return code, fmt.Errorf("FCM respondeu %d (%s)", resp.StatusCode, fail.Error.Status)
}

// accessToken troca uma assercao JWT assinada com a chave da conta de servico
// por um token de acesso (fluxo JWT bearer do OAuth 2.0), e o reaproveita ate
// perto de vencer.
func (f *FCM) accessToken(ctx context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.token != "" && time.Now().Before(f.expiry) {
		return f.token, nil
	}
	now := time.Now()
	b64 := base64.RawURLEncoding.EncodeToString
	header := b64([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{
		"iss": f.account.ClientEmail, "scope": fcmScope, "aud": f.account.TokenURI,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	})
	signing := header + "." + b64(claims)
	digest := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {signing + "." + b64(sig)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.account.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK || json.Unmarshal(raw, &out) != nil || out.AccessToken == "" {
		return "", fmt.Errorf("servidor de token respondeu %d", resp.StatusCode)
	}
	f.token = out.AccessToken
	f.expiry = now.Add(time.Duration(out.ExpiresIn)*time.Second - time.Minute)
	return f.token, nil
}
