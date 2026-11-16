package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JamesWillian/loobby-api/api-go/internal/dbtest"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
)

// mailbox guarda o que seria enviado; com fail, recusa como um Resend fora do ar.
type mailbox struct {
	mu   sync.Mutex
	sent []string // "destinatario|assunto|html"
	fail bool
}

func (m *mailbox) Send(_ context.Context, to, subject, html string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return errors.New("falha ao enviar email: Resend respondeu 503")
	}
	m.sent = append(m.sent, to+"|"+subject+"|"+html)
	return nil
}

func (m *mailbox) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

func testService(t *testing.T, opts Options) (*Service, *pgxpool.Pool, *mailbox) {
	t.Helper()
	pool := dbtest.Pool(t)
	mail := &mailbox{}
	if opts.BaseURL == "" {
		opts.BaseURL = "https://api.loobby.app"
	}
	return NewService(pool, NewTokens(testSecret), mail, nil, opts), pool, mail
}

// registered cria uma conta com email e senha, pelo mesmo caminho do app.
func registered(t *testing.T, s *Service, pool *pgxpool.Pool) (userID, email string) {
	t.Helper()
	userID = dbtest.User(t, pool, 0)
	email = "teste-" + ids.New()[:8] + "@teste.local"
	if _, err := s.Register(context.Background(), userID, email, "senha-de-teste"); err != nil {
		t.Fatal(err)
	}
	return userID, email
}

func status(err error) (int, string) {
	var he *httpx.Error
	if !errors.As(err, &he) {
		return 0, ""
	}
	if he.Message == nil {
		return he.Status, "<null>"
	}
	return he.Status, *he.Message
}

// forged: um "refresh token" de quem so sabe o id do usuario -- assinado com um
// segredo que nao e o do servidor.
func forged(userID string, expiresIn time.Duration) string {
	tok := NewTokens("segredo-de-um-atacante-com-mais-de-32-bytes")
	tok.now = func() time.Time { return time.Now().Add(expiresIn - RefreshTokenValidity) }
	return tok.Refresh(userID)
}

func expiredRefresh(userID string) string {
	tok := NewTokens(testSecret)
	tok.now = func() time.Time { return time.Now().Add(-RefreshTokenValidity - time.Hour) }
	return tok.Refresh(userID)
}

// O UNICO ponto em que o Go difere do Kotlin de proposito (ACHADOS §22).
//
// No Kotlin, um refresh recusado ainda renova a sessao se o usuario for
// anonimo -- lendo o token SEM conferir a assinatura. Quem sabe o id de um
// anonimo (qualquer membro do mesmo grupo ve) fabrica o token e entra na conta.
// Aqui a excecao so vale para token assinado por este servidor.
func TestRefreshForjadoDeAnonimoERecusado(t *testing.T) {
	s, pool, _ := testService(t, Options{})
	anon := dbtest.User(t, pool, 0)
	for name, token := range map[string]string{
		"forjado e vencido":      forged(anon, -time.Hour),
		"forjado ainda no prazo": forged(anon, time.Hour),
	} {
		_, err := s.Refresh(context.Background(), token)
		if code, msg := status(err); code != 401 || msg != "Refresh token expired or invalid" {
			t.Errorf("%s: esperava 401, veio %v", name, err)
		}
	}
}

// O caso legitimo continua funcionando: anonimo que ficou mais de 15 dias sem
// abrir o app nao tem senha para logar de novo, e o refresh vencido -- mas
// emitido por este servidor -- e renovado.
func TestRefreshVencidoDeAnonimoERenovado(t *testing.T) {
	s, pool, _ := testService(t, Options{})
	anon := dbtest.User(t, pool, 0)
	resp, err := s.Refresh(context.Background(), expiredRefresh(anon))
	if err != nil {
		t.Fatalf("refresh vencido de anonimo deveria ser renovado: %v", err)
	}
	if resp.UserID != anon || len(resp.Roles) != 1 || resp.Roles[0] != "ANON" || resp.ExpiresIn != 900 {
		t.Errorf("resposta: %+v", resp)
	}
	if _, terr := s.tokens.Verify(resp.RefreshToken); terr != nil {
		t.Errorf("o refresh novo deveria ser valido: %v", terr)
	}

	// para conta com senha, vencido e vencido: ela loga de novo
	user, _ := registered(t, s, pool)
	_, err = s.Refresh(context.Background(), expiredRefresh(user))
	if code, _ := status(err); code != 401 {
		t.Errorf("refresh vencido de conta com senha: esperava 401, veio %v", err)
	}
}

// ALLOW_UNSIGNED_ANON_REFRESH=true devolve o comportamento do Kotlin. So para
// quem trocou o JWT_SECRET e quer preservar os anonimos com token antigo.
func TestRefreshForjadoAceitoSoComAOpcaoLigada(t *testing.T) {
	s, pool, _ := testService(t, Options{AllowUnsignedAnonRefresh: true})
	anon := dbtest.User(t, pool, 0)
	if _, err := s.Refresh(context.Background(), forged(anon, -time.Hour)); err != nil {
		t.Errorf("com a opcao ligada, o comportamento do Kotlin volta: %v", err)
	}
	// mesmo com a opcao, conta com senha nunca entra por token forjado
	user, _ := registered(t, s, pool)
	if _, err := s.Refresh(context.Background(), forged(user, -time.Hour)); err == nil {
		t.Errorf("conta com senha nao pode ser renovada por token forjado")
	}
}

func TestRefreshValido(t *testing.T) {
	s, pool, _ := testService(t, Options{})
	user, _ := registered(t, s, pool)
	resp, err := s.Refresh(context.Background(), s.tokens.Refresh(user))
	if err != nil || resp.UserID != user || resp.Roles[0] != "USER" {
		t.Fatalf("refresh valido: %+v %v", resp, err)
	}
	cases := map[string]struct {
		token string
		code  int
		msg   string
	}{
		"access token no lugar do refresh": {s.tokens.Access(user, "u", []string{"USER"}), 400, "Invalid token type"},
		"usuario que nao existe":           {s.tokens.Refresh(ids.New()), 400, "User not found"},
		"sub que nao e uuid":               {s.tokens.Refresh("nao-e-uuid"), 400, "Invalid UUID string: nao-e-uuid"},
		"lixo":                             {"token.que.nao.existe", 401, "Refresh token expired or invalid"},
	}
	for name, c := range cases {
		_, err := s.Refresh(context.Background(), c.token)
		if code, msg := status(err); code != c.code || msg != c.msg {
			t.Errorf("%s: esperava %d %q, veio %d %q", name, c.code, c.msg, code, msg)
		}
	}
}

// Se o Resend falhar, o token novo nao fica gravado: tudo ou nada, como no
// @Transactional do Kotlin. O token anterior continua valendo.
func TestFalhaNoEmailNaoGravaToken(t *testing.T) {
	s, pool, mail := testService(t, Options{})
	user, email := registered(t, s, pool)
	ctx := context.Background()
	if err := s.SendVerificationEmail(ctx, user); err != nil {
		t.Fatal(err)
	}
	var before string
	_ = pool.QueryRow(ctx, `SELECT email_verification_token FROM user_credentials WHERE user_id = $1`, user).Scan(&before)
	if before == "" || mail.count() != 1 || !strings.Contains(mail.sent[0], email+"|Confirme seu email no Loobby|") ||
		!strings.Contains(mail.sent[0], "https://api.loobby.app/auth/verify-email?token="+before) {
		t.Fatalf("o email deveria levar o link com o token gravado (%q): %v", before, mail.sent)
	}

	mail.fail = true
	if err := s.SendVerificationEmail(ctx, user); err == nil {
		t.Fatal("falha do Resend deveria subir como erro (500 para o cliente)")
	}
	var after string
	_ = pool.QueryRow(ctx, `SELECT email_verification_token FROM user_credentials WHERE user_id = $1`, user).Scan(&after)
	if after != before {
		t.Errorf("o token mudou (%q -> %q) apesar de o email nao ter saido", before, after)
	}
	if err := s.VerifyEmail(ctx, before); err != nil {
		t.Errorf("o token que chegou por email deveria continuar valendo: %v", err)
	}
}

// O relogio do servico decide a espera de 3 minutos e a validade dos links.
func TestEsperaEValidadePeloRelogio(t *testing.T) {
	s, pool, mail := testService(t, Options{})
	user, email := registered(t, s, pool)
	ctx := context.Background()
	start := time.Now()
	s.now = func() time.Time { return start }
	if err := s.SendVerificationEmail(ctx, user); err != nil {
		t.Fatal(err)
	}

	s.now = func() time.Time { return start.Add(2*time.Minute + 59*time.Second) }
	if code, _ := status(s.ResendVerificationEmail(ctx, user)); code != 429 {
		t.Errorf("reenvio antes de 3 minutos deveria ser 429")
	}
	s.now = func() time.Time { return start.Add(3*time.Minute + time.Second) }
	if err := s.ResendVerificationEmail(ctx, user); err != nil || mail.count() != 2 {
		t.Errorf("reenvio depois de 3 minutos deveria sair: %v (%d emails)", err, mail.count())
	}

	var token string
	_ = pool.QueryRow(ctx, `SELECT email_verification_token FROM user_credentials WHERE user_id = $1`, user).Scan(&token)
	s.now = func() time.Time { return start.Add(25 * time.Hour) }
	if code, msg := status(s.VerifyEmail(ctx, token)); code != 400 || !strings.HasPrefix(msg, "Token expirado") {
		t.Errorf("token de 24 h usado depois de 25 h: veio %d %q", code, msg)
	}

	// reset: pedido repetido em menos de 3 minutos nao manda outro email; o link vale 1 hora
	s.now = func() time.Time { return start }
	_ = s.RequestPasswordReset(ctx, email)
	_ = s.RequestPasswordReset(ctx, email)
	if mail.count() != 3 {
		t.Errorf("segundo pedido de reset dentro da espera nao deveria mandar email (%d)", mail.count())
	}
	_ = pool.QueryRow(ctx, `SELECT password_reset_token FROM user_credentials WHERE user_id = $1`, user).Scan(&token)
	s.now = func() time.Time { return start.Add(61 * time.Minute) }
	if code, msg := status(s.ResetPassword(ctx, token, "senha-nova-1")); code != 400 || !strings.HasPrefix(msg, "Link expirado") {
		t.Errorf("link de 1 h usado depois de 61 min: veio %d %q", code, msg)
	}
	s.now = func() time.Time { return start.Add(59 * time.Minute) }
	if err := s.ResetPassword(ctx, token, "senha-nova-1"); err != nil {
		t.Errorf("link dentro do prazo deveria trocar a senha: %v", err)
	}
	if _, err := s.Login(ctx, email, "senha-nova-1"); err != nil {
		t.Errorf("login com a senha nova: %v", err)
	}
}

// Email desconhecido: nenhuma pista. Sem erro, sem email.
func TestResetNaoRevelaSeOEmailExiste(t *testing.T) {
	s, _, mail := testService(t, Options{})
	if err := s.RequestPasswordReset(context.Background(), "ninguem-"+ids.New()[:8]+"@teste.local"); err != nil || mail.count() != 0 {
		t.Errorf("email desconhecido: err=%v, emails=%d", err, mail.count())
	}
}

func TestAnonimoNasceComoOKotlinCria(t *testing.T) {
	s, pool, _ := testService(t, Options{})
	resp, err := s.CreateAnonymous(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dbtest.Delete(pool, resp.UserID) })
	if !strings.HasPrefix(resp.Username, "user_") || len(resp.Username) != 11 || resp.Roles[0] != "ANON" {
		t.Errorf("anonimo: %+v", resp)
	}
	var displayname string
	var provider int
	_ = pool.QueryRow(context.Background(), `SELECT displayname, auth_provider FROM users WHERE id = $1`, resp.UserID).
		Scan(&displayname, &provider)
	if displayname != resp.Username || provider != 0 {
		t.Errorf("displayname %q, provider %d", displayname, provider)
	}
}
