package migrations_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pressly/goose/v3"

	"github.com/JamesWillian/loobby-api/api-go/internal/dbtest"
	"github.com/JamesWillian/loobby-api/api-go/migrations"
)

func open(t *testing.T, url string) *sql.DB {
	t.Helper()
	db, err := migrations.Open(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func exists(t *testing.T, db *sql.DB, query string) bool {
	t.Helper()
	var ok bool
	if err := db.QueryRowContext(context.Background(), query).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

// Banco vazio (testes, ambiente local): up cria tudo, e o segundo up nao tem
// o que fazer.
func TestUpNumBancoVazioCriaOSchema(t *testing.T) {
	ctx := context.Background()
	db := open(t, dbtest.EmptyDatabase(t))

	results, err := migrations.Up(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 || results[0].Source.Version != migrations.BaselineVersion {
		t.Fatalf("a primeira migracao aplicada deveria ser a base, vieram %v", results)
	}
	if !exists(t, db, "SELECT to_regclass('public.users') IS NOT NULL") ||
		!exists(t, db, "SELECT to_regclass('public.games_name_trgm_idx') IS NOT NULL") {
		t.Error("a base deveria criar as tabelas e o indice do pg_trgm")
	}
	// O historico fica fora do public.
	if !exists(t, db, "SELECT to_regclass('goose.goose_db_version') IS NOT NULL") ||
		exists(t, db, "SELECT to_regclass('public.goose_db_version') IS NOT NULL") {
		t.Error("o historico do goose deveria estar em goose.goose_db_version, e so la")
	}

	again, err := migrations.Up(ctx, db)
	if err != nil || len(again) != 0 {
		t.Errorf("segundo up: queria nada a aplicar, veio %v (erro %v)", again, err)
	}
}

// A producao na primeira vez: o schema da base ja existe e o goose nunca rodou.
// up recusa sem criar nada, status so le, baseline marca a base, e depois o up
// roda as migracoes seguintes, mas nunca a 00001.
func TestProducaoPrecisaDoBaselineAntesDoUp(t *testing.T) {
	ctx := context.Background()
	db := open(t, dbtest.EmptyDatabase(t))
	if _, err := migrations.UpTo(ctx, db, migrations.BaselineVersion); err != nil {
		t.Fatal(err)
	}
	// Fica como a producao: o schema da base, sem o historico.
	if _, err := db.ExecContext(ctx, "DROP SCHEMA goose CASCADE"); err != nil {
		t.Fatal(err)
	}
	noHistory := "SELECT to_regnamespace('goose') IS NULL"

	if _, err := migrations.Up(ctx, db); !errors.Is(err, migrations.ErrNeedsBaseline) {
		t.Fatalf("up sem a base marcada: queria ErrNeedsBaseline, veio %v", err)
	}
	if !exists(t, db, noHistory) {
		t.Error("o up recusado nao deveria ter criado nada")
	}
	status, err := migrations.Status(ctx, db)
	if err != nil || len(status) == 0 || status[0].State != goose.StatePending {
		t.Fatalf("status antes do baseline: queria a base pendente, veio %v (erro %v)", status, err)
	}
	if !exists(t, db, noHistory) {
		t.Error("o status deveria so ler")
	}

	marked, err := migrations.Baseline(ctx, db)
	if err != nil || !marked {
		t.Fatalf("baseline: queria marcar, veio %v (erro %v)", marked, err)
	}
	if marked, err := migrations.Baseline(ctx, db); err != nil || marked {
		t.Errorf("segundo baseline: queria nada a fazer, veio %v (erro %v)", marked, err)
	}
	results, err := migrations.Up(ctx, db)
	if err != nil || len(results) == 0 {
		t.Fatalf("up depois do baseline: queria as migracoes seguintes, veio %v (erro %v)", results, err)
	}
	for _, r := range results {
		if r.Source.Version <= migrations.BaselineVersion {
			t.Errorf("up depois do baseline: a base nao pode rodar, rodou a %d", r.Source.Version)
		}
	}
	if !exists(t, db, "SELECT to_regclass('public.gameplays') IS NOT NULL") {
		t.Error("o up depois do baseline deveria criar as tabelas da 2.0")
	}
	status, err = migrations.Status(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range status {
		if s.State != goose.StateApplied {
			t.Errorf("status depois do up: %s ainda %s", s.Source.Path, s.State)
		}
	}
	if again, err := migrations.Up(ctx, db); err != nil || len(again) != 0 {
		t.Errorf("segundo up: queria nada a aplicar, veio %v (erro %v)", again, err)
	}
}

func TestBaselineRecusaBancoVazio(t *testing.T) {
	db := open(t, dbtest.EmptyDatabase(t))
	if _, err := migrations.Baseline(context.Background(), db); !errors.Is(err, migrations.ErrEmptyDatabase) {
		t.Fatalf("queria ErrEmptyDatabase, veio %v", err)
	}
}

// A porta 6543 e o pooler do Supabase em modo transacao, o do DB_URL da API.
func TestOpenRecusaOPoolerDeTransacao(t *testing.T) {
	if _, err := migrations.Open("postgres://u:x@aws-1-sa-east-1.pooler.supabase.com:6543/postgres"); err == nil {
		t.Error("a porta 6543 deveria ser recusada")
	}
	db, err := migrations.Open("postgres://u:x@aws-1-sa-east-1.pooler.supabase.com:5432/postgres")
	if err != nil {
		t.Fatalf("a conexao de sessao deveria ser aceita: %v", err)
	}
	_ = db.Close()
}

// Toda tabela do public precisa de RLS: sem ele, os privilegios padrao do
// Supabase dao ao anon e ao authenticated acesso total pela API de dados.
// Uma migracao que esquecer o ENABLE ROW LEVEL SECURITY falha aqui.
func TestTodaTabelaDoPublicTemRLS(t *testing.T) {
	db := open(t, dbtest.MigratedDatabase(t))
	rows, err := db.QueryContext(context.Background(), `
		SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p') AND NOT c.relrowsecurity
		ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var missing []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		missing = append(missing, name)
	}
	if len(missing) > 0 {
		t.Errorf("tabelas do public sem RLS: %s", strings.Join(missing, ", "))
	}
}

// O papel loobby_backend so existe no Supabase, e as migracoes so dao o GRANT
// a ele se ele existir. Aqui ele e criado no servidor de teste (sem login,
// como o da producao) para conferir o GRANT das tabelas da 2.0.
func TestGrantAoLoobbyBackend(t *testing.T) {
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, adminURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	var exists bool
	if err := admin.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'loobby_backend')`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		if _, err := admin.Exec(ctx, `CREATE ROLE loobby_backend NOLOGIN`); err != nil {
			t.Fatal(err)
		}
		// Registrado antes do banco: roda depois de ele ser apagado. Se outro
		// banco de teste tiver recebido o GRANT nesse meio-tempo, o papel fica
		// -- sem login, nao atrapalha.
		t.Cleanup(func() {
			conn, err := pgx.Connect(context.Background(), adminURL(t))
			if err == nil {
				_, _ = conn.Exec(context.Background(), `DROP ROLE IF EXISTS loobby_backend`)
				_ = conn.Close(context.Background())
			}
		})
	}
	db := open(t, dbtest.MigratedDatabase(t))
	for _, table := range []string{"gameplays", "gameplay_participants", "gameplay_games",
		"gameplay_votes", "gameplay_comments", "gameplay_comment_reports"} {
		var ok bool
		err := db.QueryRowContext(ctx, `SELECT has_table_privilege('loobby_backend', $1, 'SELECT, INSERT, UPDATE, DELETE')`,
			"public."+table).Scan(&ok)
		if err != nil || !ok {
			t.Errorf("%s: o loobby_backend deveria ter SELECT, INSERT, UPDATE e DELETE (erro %v)", table, err)
		}
	}
}

// A 00003: o username vazio, o da conta excluida, pode repetir; qualquer outro
// continua unico, com o mesmo nome de restricao no erro (contract/ACHADOS.md §23).
func TestUsernameVazioPodeRepetir(t *testing.T) {
	ctx := context.Background()
	db := open(t, dbtest.MigratedDatabase(t))
	insert := func(username string) error {
		_, err := db.ExecContext(ctx,
			`INSERT INTO users (id, username, displayname) VALUES (gen_random_uuid(), $1, '[deleted]')`, username)
		return err
	}
	for i := range 3 {
		if err := insert(""); err != nil {
			t.Fatalf("conta excluida %d: %v", i+1, err)
		}
	}
	if err := insert("repetido"); err != nil {
		t.Fatal(err)
	}
	var pg *pgconn.PgError
	if err := insert("repetido"); !errors.As(err, &pg) || pg.Code != "23505" || pg.ConstraintName != "users_username_key" {
		t.Errorf("username repetido: queria 23505 em users_username_key, veio %v", err)
	}
}

func adminURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL nao definida")
	}
	return url
}
