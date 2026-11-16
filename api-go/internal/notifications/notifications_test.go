package notifications

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/dbtest"
	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
)

func TestMensagemDeConfirmacoesAgrupadas(t *testing.T) {
	cases := []struct {
		names  []string
		extras int
		want   string
	}{
		{[]string{"Ana"}, 0, "Ana confirmou presença"},
		{[]string{"Ana", "Bia"}, 0, "Ana e Bia confirmaram presença"},
		{[]string{"Ana", "Bia"}, 3, "Ana, Bia e mais 3 confirmaram presença"},
		{[]string{"Ana"}, 2, "Ana e mais 2 confirmaram presença"},
		{nil, 0, "Novas confirmações no evento"},
	}
	for _, c := range cases {
		if got := peerRsvpMessage(c.names, c.extras); got != c.want {
			t.Errorf("%v +%d: %q, esperava %q", c.names, c.extras, got, c.want)
		}
	}
}

// O debouncer junta as confirmacoes de um evento por destinatario: um push so,
// com os dois primeiros nomes e a contagem do resto.
func TestDebouncerAgrupaPorEventoEDestinatario(t *testing.T) {
	d := newDebouncer(5 * time.Minute)
	var timers []func()
	d.after = func(delay time.Duration, f func()) {
		if delay != 5*time.Minute {
			t.Errorf("espera de %s, esperava 5 minutos", delay)
		}
		timers = append(timers, f)
	}
	var flushed []string
	flush := func(names []string, extras int) { flushed = append(flushed, peerRsvpMessage(names, extras)) }

	d.schedule("evento", "dono", "u1", "Ana", flush)
	d.schedule("evento", "dono", "u2", "Bia", flush)
	d.schedule("evento", "dono", "u2", "Bia", flush) // confirmou de novo: conta uma vez
	d.schedule("evento", "dono", "u3", "Caio", flush)
	d.schedule("evento", "dono", "u4", "Duda", flush)
	d.schedule("evento", "outro-destinatario", "u1", "Ana", flush)
	d.schedule("outro-evento", "dono", "u1", "Ana", flush)
	if len(timers) != 3 {
		t.Fatalf("uma janela por (evento, destinatario): %d", len(timers))
	}
	if len(flushed) != 0 {
		t.Fatalf("nada sai antes do prazo")
	}
	for _, fire := range timers {
		fire()
	}
	sort.Strings(flushed)
	want := []string{"Ana confirmou presença", "Ana confirmou presença", "Ana, Bia e mais 2 confirmaram presença"}
	if strings.Join(flushed, "|") != strings.Join(want, "|") {
		t.Errorf("veio %v", flushed)
	}
	// janela fechada: a proxima confirmacao abre outra
	d.schedule("evento", "dono", "u5", "Eva", flush)
	if len(timers) != 4 {
		t.Errorf("depois do disparo, uma confirmacao nova deveria abrir outra janela")
	}
}

// fakeFirebase faz o papel do servidor de token do Google e do FCM.
type fakeFirebase struct {
	mu        sync.Mutex
	tokenReqs int
	assertion string
	sent      []fcmMessage
	auth      []string
	dead      map[string]string // token do aparelho -> errorCode do FCM
}

func (f *fakeFirebase) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		f.tokenReqs++
		f.assertion = r.PostForm.Get("assertion")
		f.mu.Unlock()
		if r.PostForm.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"token-de-acesso","expires_in":3600,"token_type":"Bearer"}`))
	})
	mux.HandleFunc("POST /v1/projects/loobby-teste/messages:send", func(w http.ResponseWriter, r *http.Request) {
		var msg fcmMessage
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &msg)
		f.mu.Lock()
		f.sent = append(f.sent, msg)
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		code := f.dead[msg.Message.Token]
		f.mu.Unlock()
		if code != "" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404,"status":"NOT_FOUND","details":[{"@type":"x","errorCode":"` + code + `"}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"name":"projects/loobby-teste/messages/1"}`))
	})
	return mux
}

func credentialsFile(t *testing.T, tokenURI string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	raw, _ := json.Marshal(map[string]string{
		"type": "service_account", "project_id": "loobby-teste",
		"client_email": "push@loobby-teste.iam.gserviceaccount.com",
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"token_uri":    tokenURI,
	})
	path := filepath.Join(t.TempDir(), "fcm.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFCMEnviaParaCadaAparelhoEApagaOsMortos(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	fake := &fakeFirebase{dead: map[string]string{}}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	fcm, err := NewFCM(pool, credentialsFile(t, srv.URL+"/token"))
	if err != nil {
		t.Fatal(err)
	}
	fcm.endpoint = srv.URL

	ana, bia, semAparelho := dbtest.User(t, pool, 1), dbtest.User(t, pool, 1), dbtest.User(t, pool, 1)
	suffix := ids.New()[:8]
	vivo1, vivo2, morto, invalido := "vivo1-"+suffix, "vivo2-"+suffix, "morto-"+suffix, "invalido-"+suffix
	for token, user := range map[string]string{vivo1: ana, morto: ana, vivo2: bia, invalido: bia} {
		if _, err := pool.Exec(ctx, `INSERT INTO device_tokens (token, user_id, platform) VALUES ($1, $2, 'ANDROID')`, token, user); err != nil {
			t.Fatal(err)
		}
	}
	fake.dead[morto], fake.dead[invalido] = "UNREGISTERED", "INVALID_ARGUMENT"

	fcm.SendToUsers(ctx, []string{ana, bia, semAparelho}, Payload{
		Title: "Futebol de quinta", Body: "Ana confirmou presença", Type: TypeRsvpConfirmedByPeer,
		Data: map[string]string{"eventId": "e1", "actorId": "a1"},
	})

	if len(fake.sent) != 4 {
		t.Fatalf("um envio por aparelho: %d", len(fake.sent))
	}
	for i, msg := range fake.sent {
		if fake.auth[i] != "Bearer token-de-acesso" {
			t.Errorf("Authorization: %q", fake.auth[i])
		}
		// title e body vao tambem em data: o app le de la em primeiro plano
		want := map[string]string{"type": "RSVP_CONFIRMED_BY_PEER", "title": "Futebol de quinta",
			"body": "Ana confirmou presença", "eventId": "e1", "actorId": "a1"}
		if msg.Message.Notification["title"] != "Futebol de quinta" || msg.Message.Notification["body"] != "Ana confirmou presença" {
			t.Errorf("notification: %v", msg.Message.Notification)
		}
		for k, v := range want {
			if msg.Message.Data[k] != v {
				t.Errorf("data[%s] = %q, esperava %q", k, msg.Message.Data[k], v)
			}
		}
	}

	// o que o Firebase diz que nao existe mais e apagado; o resto fica
	var left []string
	rows, _ := pool.Query(ctx, `SELECT token FROM device_tokens WHERE user_id = ANY($1) ORDER BY token`, []string{ana, bia})
	for rows.Next() {
		var tok string
		_ = rows.Scan(&tok)
		left = append(left, tok)
	}
	if strings.Join(left, ",") != vivo1+","+vivo2 {
		t.Errorf("tokens restantes: %v", left)
	}

	// o token de acesso e reaproveitado; e a assercao e um JWT RS256 da conta de servico
	fcm.SendToUsers(ctx, []string{ana}, Payload{Title: "t", Body: "b", Type: TypeNewGroupEvent})
	if fake.tokenReqs != 1 {
		t.Errorf("o token de acesso deveria ser pedido uma vez; foram %d", fake.tokenReqs)
	}
	if parts := strings.Split(fake.assertion, "."); len(parts) != 3 {
		t.Errorf("assercao nao e um JWT: %q", fake.assertion)
	}
	// sem aparelho nenhum, nada sai
	before := len(fake.sent)
	fcm.SendToUsers(ctx, []string{semAparelho}, Payload{Title: "t", Body: "b", Type: TypeNewGroupEvent})
	fcm.SendToUsers(ctx, nil, Payload{Title: "t", Body: "b", Type: TypeNewGroupEvent})
	if len(fake.sent) != before {
		t.Errorf("usuario sem aparelho nao deveria gerar envio")
	}
}

// Credenciais ruins impedem a partida, como no Kotlin -- melhor do que subir e
// descobrir no primeiro push.
func TestFCMCredenciaisInvalidasImpedemAPartida(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		_ = os.WriteFile(p, []byte(content), 0o600)
		return p
	}
	for name, path := range map[string]string{
		"arquivo ausente":     filepath.Join(dir, "nao-existe.json"),
		"nao e json":          write("a.json", "isto nao e json"),
		"faltam campos":       write("b.json", `{"project_id":"p"}`),
		"chave que nao e PEM": write("c.json", `{"project_id":"p","client_email":"e","private_key":"abc"}`),
	} {
		if _, err := NewFCM(nil, path); err == nil {
			t.Errorf("%s: deveria falhar", name)
		}
	}
}

// O registro de envios e a trava contra push repetido -- inclusive entre o
// Kotlin e o Go rodando juntos: o segundo que tentar registrar perde.
func TestTryDispatchEnviaUmaVezPorDia(t *testing.T) {
	pool := dbtest.Pool(t)
	user := dbtest.User(t, pool, 1)
	s := &Scheduler{q: NewService(pool, DisabledSender{}).q}
	day := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	ref := ids.New()
	sent := 0
	for range 3 {
		s.tryDispatch(context.Background(), user, TypePaymentDueDaily, ref, day, func() { sent++ })
	}
	if sent != 1 {
		t.Errorf("mesmo usuario, tipo, referencia e dia: 1 envio, vieram %d", sent)
	}
	s.tryDispatch(context.Background(), user, TypePaymentDueDaily, ref, day.AddDate(0, 0, 1), func() { sent++ })
	s.tryDispatch(context.Background(), user, TypeEventReminderHoursBefore, ref, day, func() { sent++ })
	if sent != 3 {
		t.Errorf("outro dia e outro tipo sao envios novos: vieram %d", sent)
	}
}

func TestProximosHorariosDosJobs(t *testing.T) {
	s, err := NewScheduler(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	sp, _ := time.LoadLocation("America/Sao_Paulo")
	// job diario: 8h de Sao Paulo
	if got := s.nextDaily(time.Date(2026, 11, 16, 7, 59, 0, 0, sp)); !got.Equal(time.Date(2026, 11, 16, 8, 0, 0, 0, sp)) {
		t.Errorf("antes das 8h: %s", got)
	}
	if got := s.nextDaily(time.Date(2026, 11, 16, 8, 0, 0, 0, sp)); !got.Equal(time.Date(2026, 11, 17, 8, 0, 0, 0, sp)) {
		t.Errorf("as 8h em ponto, o proximo e amanha: %s", got)
	}
	// lembretes: a cada 5 minutos cheios
	if got := s.nextReminder(time.Date(2026, 11, 16, 10, 3, 20, 0, time.UTC)); !got.Equal(time.Date(2026, 11, 16, 10, 5, 0, 0, time.UTC)) {
		t.Errorf("proximo multiplo de 5 minutos: %s", got)
	}
}
