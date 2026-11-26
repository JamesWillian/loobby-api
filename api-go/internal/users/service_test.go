package users

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/dbtest"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
	"github.com/JamesWillian/loobby-api/api-go/internal/password"
)

// banco: um banco so do teste, com todas as migracoes. O compartilhado de
// TEST_DATABASE_URL pode estar sem a mais nova, e estes testes contam linhas
// por usuario.
type banco struct {
	t    *testing.T
	ctx  context.Context
	pool *pgxpool.Pool
}

func novoBanco(t *testing.T) *banco {
	t.Helper()
	ctx := context.Background()
	pool, err := db.Connect(ctx, dbtest.MigratedDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return &banco{t: t, ctx: ctx, pool: pool}
}

func (b *banco) exec(sql string, args ...any) {
	b.t.Helper()
	if _, err := b.pool.Exec(b.ctx, sql, args...); err != nil {
		b.t.Fatal(err)
	}
}

func (b *banco) count(sql string, args ...any) int {
	b.t.Helper()
	var n int
	if err := b.pool.QueryRow(b.ctx, sql, args...).Scan(&n); err != nil {
		b.t.Fatal(err)
	}
	return n
}

// conta cria um usuario (provider: 0 anonimo, 1 email, 2 Google). Com senha,
// ganha credenciais de senha; com google, uma linha em user_google_credentials.
func (b *banco) conta(provider int, senha string, google bool) string {
	b.t.Helper()
	id := ids.New()
	b.exec(`INSERT INTO users (id, username, displayname, auth_provider) VALUES ($1, $2, $2, $3)`, id, "u_"+id[:8], provider)
	if senha != "" {
		hash, err := password.Hash(senha)
		if err != nil {
			b.t.Fatal(err)
		}
		b.exec(`INSERT INTO user_credentials (user_id, email, password_hash) VALUES ($1, $2, $3)`, id, id[:8]+"@loobby.app", hash)
	}
	if google {
		b.exec(`INSERT INTO user_google_credentials (user_id, google_id, email) VALUES ($1, $2, $3)`,
			id, "google-"+id[:8], id[:8]+"@gmail.com")
	}
	return id
}

func wantErr(t *testing.T, err error, want *httpx.Error) {
	t.Helper()
	var got *httpx.Error
	if !errors.As(err, &got) || got.Status != want.Status || got.Error() != want.Error() {
		t.Errorf("queria %d %q, veio %v", want.Status, want.Error(), err)
	}
}

// A exclusao de conta limpa a 2.0 na mesma transacao: votos, comentarios e
// participacoes saem, e o que a pessoa criou passa para outra.
func TestExclusaoDeContaLimpaAsGameplays(t *testing.T) {
	b := novoBanco(t)
	gone := b.conta(1, "senha-123", false)
	owner := b.conta(1, "", false)
	group := ids.New()
	b.exec(`INSERT INTO groups (id, name, invite_code, owner_id) VALUES ($1, 'Turma', '$-ABC123', $2)`, group, owner)
	b.exec(`INSERT INTO group_members (id, group_id, user_id) VALUES (gen_random_uuid(), $1, $2)`, group, gone)

	inGroup, single, game := ids.New(), ids.New(), ids.New()
	when := time.Now().Add(2 * time.Hour)
	b.exec(`INSERT INTO gameplays (id, group_id, creator_id, name, mode, platforms, scheduled_at, finalists, multi_vote, voting_round)
	      VALUES ($1, $2, $3, 'Sexta', 'VOTING', '["PC"]', $4, 1, false, 1)`, inGroup, group, gone, when)
	b.exec(`INSERT INTO gameplays (id, creator_id, name, mode, platforms, scheduled_at, invite_code)
	      VALUES ($1, $2, 'Sozinho', 'FREE', '["PC"]', $3, '$-SOZINHO1')`, single, gone, when)
	b.exec(`INSERT INTO gameplay_participants (gameplay_id, user_id, status) VALUES ($1, $3, 'GOING'), ($2, $3, 'GOING')`, inGroup, single, gone)
	b.exec(`INSERT INTO gameplay_games (id, gameplay_id, name, added_by) VALUES ($1, $2, 'Valorant', $3)`, game, inGroup, gone)
	b.exec(`INSERT INTO gameplay_votes (gameplay_game_id, voting_round, user_id, gameplay_id) VALUES ($1, 1, $2, $3)`, game, gone, inGroup)
	b.exec(`INSERT INTO gameplay_comments (id, gameplay_id, user_id, body) VALUES (gen_random_uuid(), $1, $2, 'gg')`, inGroup, gone)

	if err := NewService(b.pool).DeleteAccount(b.ctx, gone, "senha-123"); err != nil {
		t.Fatal(err)
	}

	for name, sql := range map[string]string{
		"votos":         `SELECT count(*) FROM gameplay_votes WHERE user_id = $1`,
		"comentarios":   `SELECT count(*) FROM gameplay_comments WHERE user_id = $1`,
		"participacoes": `SELECT count(*) FROM gameplay_participants WHERE user_id = $1`,
		"criadas":       `SELECT count(*) FROM gameplays WHERE creator_id = $1`,
	} {
		if n := b.count(sql, gone); n != 0 {
			t.Errorf("%s de quem excluiu a conta: %d", name, n)
		}
	}
	if n := b.count(`SELECT count(*) FROM gameplays WHERE id = $1 AND creator_id = $2`, inGroup, owner); n != 1 {
		t.Error("a gameplay do grupo passa para o dono do grupo")
	}
	if n := b.count(`SELECT count(*) FROM gameplays WHERE id = $1`, single); n != 0 {
		t.Error("a unica sem mais ninguem e apagada")
	}
	if n := b.count(`SELECT count(*) FROM users WHERE id = $1 AND username = '' AND displayname = '[deleted]'`, gone); n != 1 {
		t.Error("o usuario continua sendo anonimizado, como na v1")
	}
}

// Contas excluidas uma depois da outra: todas ficam com o username vazio. Antes
// da migracao 00003, a segunda dava 500 e continuava existindo
// (contract/ACHADOS.md §23).
func TestExclusaoDeVariasContas(t *testing.T) {
	b := novoBanco(t)
	svc := NewService(b.pool)
	var gone []string
	for i := range 3 {
		id := b.conta(1, "senha-123", false)
		if err := svc.DeleteAccount(b.ctx, id, "senha-123"); err != nil {
			t.Fatalf("exclusao %d: %v", i+1, err)
		}
		gone = append(gone, id)
	}
	if n := b.count(`SELECT count(*) FROM users WHERE id = ANY($1) AND username = '' AND displayname = '[deleted]'`, gone); n != 3 {
		t.Errorf("contas anonimizadas: %d de 3", n)
	}
	if n := b.count(`SELECT count(*) FROM user_credentials WHERE user_id = ANY($1)`, gone); n != 0 {
		t.Errorf("credenciais de senha que sobraram: %d", n)
	}
}

// Conta so com Google nao tem senha: o token basta, e o password que os apps
// mandam e ignorado (contract/ACHADOS.md §24). As credenciais do Google saem
// junto; sem isso, entrar de novo com o Google cairia na conta excluida.
func TestExclusaoDeContaGoogle(t *testing.T) {
	b := novoBanco(t)
	svc := NewService(b.pool)
	id := b.conta(2, "", true)
	b.exec(`UPDATE users SET avatar_url = 'https://exemplo.com/foto.png' WHERE id = $1`, id)

	if err := svc.DeleteAccount(b.ctx, id, "qualquer coisa"); err != nil {
		t.Fatal(err)
	}
	if n := b.count(`SELECT count(*) FROM user_google_credentials WHERE user_id = $1`, id); n != 0 {
		t.Error("as credenciais do Google continuam la")
	}
	if n := b.count(`SELECT count(*) FROM users
	                 WHERE id = $1 AND username = '' AND displayname = '[deleted]' AND avatar_url IS NULL`, id); n != 1 {
		t.Error("a conta Google deveria ser anonimizada, como a de senha")
	}
	// Ja excluida, nao sobra o que confirme: 404, como a de senha excluida de novo.
	wantErr(t, svc.DeleteAccount(b.ctx, id, "qualquer coisa"), errSemConta)
}

// Conta com senha e com Google (conta Google que ganhou senha pelo
// /auth/register): vale a senha, e as duas credenciais saem.
func TestExclusaoDeContaComSenhaEGoogle(t *testing.T) {
	b := novoBanco(t)
	svc := NewService(b.pool)
	id := b.conta(1, "senha-123", true)

	wantErr(t, svc.DeleteAccount(b.ctx, id, "senha-errada"), httpx.Status(http.StatusUnauthorized, "Senha incorreta."))
	if n := b.count(`SELECT count(*) FROM users WHERE id = $1 AND username <> ''`, id); n != 1 {
		t.Fatal("com a senha errada, a conta nao pode ser excluida")
	}

	if err := svc.DeleteAccount(b.ctx, id, "senha-123"); err != nil {
		t.Fatal(err)
	}
	for name, sql := range map[string]string{
		"de senha":  `SELECT count(*) FROM user_credentials WHERE user_id = $1`,
		"do Google": `SELECT count(*) FROM user_google_credentials WHERE user_id = $1`,
	} {
		if n := b.count(sql, id); n != 0 {
			t.Errorf("as credenciais %s continuam la", name)
		}
	}
}

// Conta anonima nao tem senha nem Google: 404, como no Kotlin.
func TestExclusaoDeContaAnonima(t *testing.T) {
	b := novoBanco(t)
	id := b.conta(0, "", false)
	wantErr(t, NewService(b.pool).DeleteAccount(b.ctx, id, "qualquer"), errSemConta)
	if n := b.count(`SELECT count(*) FROM users WHERE id = $1 AND username <> ''`, id); n != 1 {
		t.Error("a conta anonima nao pode ser esvaziada")
	}
}

// Troca de senha de conta sem senha: a do Google recebe 400 com uma mensagem
// que o app mostra, e a anonima segue com o 404 do Kotlin (contract/ACHADOS.md §24).
func TestTrocaDeSenhaDeContaSemSenha(t *testing.T) {
	b := novoBanco(t)
	svc := NewService(b.pool)
	google, anon := b.conta(2, "", true), b.conta(0, "", false)

	wantErr(t, svc.ChangePassword(b.ctx, google, "qualquer", "senha-nova", "senha-nova"),
		httpx.Status(http.StatusBadRequest, "Sua conta entra com o Google e não tem senha."))
	wantErr(t, svc.ChangePassword(b.ctx, anon, "qualquer", "senha-nova", "senha-nova"), errSemConta)
}
