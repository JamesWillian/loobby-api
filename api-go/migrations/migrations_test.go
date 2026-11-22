package migrations_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

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

// A producao na primeira vez: o schema ja existe e o goose nunca rodou. up
// recusa sem criar nada, status so le, baseline marca a base, e depois o up nao
// roda a 00001.
func TestProducaoPrecisaDoBaselineAntesDoUp(t *testing.T) {
	ctx := context.Background()
	db := open(t, dbtest.EmptyDatabase(t))
	if _, err := migrations.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	// Fica como a producao: o schema, sem o historico.
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
	if results, err := migrations.Up(ctx, db); err != nil || len(results) != 0 {
		t.Errorf("up depois do baseline: a base nao pode rodar, veio %v (erro %v)", results, err)
	}
	status, err = migrations.Status(ctx, db)
	if err != nil || status[0].State != goose.StateApplied {
		t.Errorf("status depois do baseline: queria a base aplicada, veio %v (erro %v)", status, err)
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
