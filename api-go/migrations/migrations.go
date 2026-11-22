// Package migrations guarda as migracoes do banco e as aplica, com o goose.
//
// Os arquivos NNNNN_nome.sql desta pasta sao a fonte de verdade do schema: o
// sqlc gera o codigo a partir deles (sqlc.yaml), os testes montam o banco com
// Up, e a producao recebe cada um pelo `go run ./cmd/migrate up`.
//
// A 00001 e a base: o schema da producao de antes das migracoes. Num banco
// vazio ela cria tudo; na producao, que ja tinha esse schema, ela e marcada
// como aplicada uma unica vez (Baseline), sem rodar.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"
)

//go:embed *.sql
var files embed.FS

const (
	// Schema e Table: o historico do goose fica num schema proprio. No public,
	// os privilegios padrao do Supabase dariam ao anon e ao authenticated
	// acesso total a tabela.
	Schema = "goose"
	Table  = Schema + ".goose_db_version"

	// BaselineVersion e a 00001, o schema de antes das migracoes.
	BaselineVersion = 1

	// transactionPoolerPort e o pooler do Supabase em modo transacao, o do
	// DB_URL da API. Migracao vai pela conexao de sessao (porta 5432).
	transactionPoolerPort = 6543
)

var (
	// ErrNeedsBaseline: o banco ja tem o schema, mas o goose nao sabe disso.
	// E a producao antes do primeiro Baseline -- rodar a 00001 la falharia na
	// primeira tabela, e e melhor nem tentar.
	ErrNeedsBaseline = errors.New("o banco ja tem o schema, mas a migracao de base nao esta marcada como aplicada: " +
		"se este e o banco de producao, rode `baseline` uma vez antes de `up`")

	// ErrEmptyDatabase: Baseline num banco que nao tem o schema.
	ErrEmptyDatabase = errors.New("o banco nao tem o schema de producao: use `up`, que cria tudo")
)

// Open conecta pelo pgx e recusa o pooler em modo transacao.
func Open(url string) (*sql.DB, error) {
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("URL do banco invalida: %w", err)
	}
	if cfg.Port == transactionPoolerPort {
		return nil, fmt.Errorf("a porta %d e o pooler do Supabase em modo transacao; "+
			"migracao precisa da conexao de sessao (porta 5432)", transactionPoolerPort)
	}
	db := stdlib.OpenDB(*cfg)
	db.SetMaxOpenConns(2)
	return db, nil
}

// Up aplica as migracoes que faltam e devolve o que rodou (vazio se nada).
func Up(ctx context.Context, db *sql.DB) ([]*goose.MigrationResult, error) {
	if err := requireBaseline(ctx, db); err != nil {
		return nil, err
	}
	p, err := prepare(ctx, db)
	if err != nil {
		return nil, err
	}
	return p.Up(ctx)
}

// Baseline marca a 00001 como aplicada, sem roda-la. E para a producao, uma
// vez. Devolve false se ela ja estava marcada.
func Baseline(ctx context.Context, db *sql.DB) (bool, error) {
	schema, err := hasSchema(ctx, db)
	if err != nil {
		return false, err
	}
	if !schema {
		return false, ErrEmptyDatabase
	}
	done, err := applied(ctx, db, BaselineVersion)
	if err != nil || done {
		return false, err
	}
	p, err := prepare(ctx, db)
	if err != nil {
		return false, err
	}
	// Cria a tabela de historico como o goose cria (com a versao 0), e so
	// entao registra a base.
	if _, err := p.GetDBVersion(ctx); err != nil {
		return false, err
	}
	store, err := database.NewStore(database.DialectPostgres, Table)
	if err != nil {
		return false, err
	}
	if err := store.Insert(ctx, db, database.InsertRequest{Version: BaselineVersion}); err != nil {
		return false, fmt.Errorf("marcar a base: %w", err)
	}
	return true, nil
}

// Status lista cada migracao e se ja foi aplicada. So le: num banco em que o
// goose nunca rodou, tudo aparece pendente e nada e criado.
func Status(ctx context.Context, db *sql.DB) ([]*goose.MigrationStatus, error) {
	p, err := provider(db)
	if err != nil {
		return nil, err
	}
	tracked, err := historyExists(ctx, db)
	if err != nil {
		return nil, err
	}
	if !tracked {
		var all []*goose.MigrationStatus
		for _, s := range p.ListSources() {
			all = append(all, &goose.MigrationStatus{Source: s, State: goose.StatePending})
		}
		return all, nil
	}
	return p.Status(ctx)
}

// requireBaseline: num banco que ja tem o schema, a base precisa estar marcada.
func requireBaseline(ctx context.Context, db *sql.DB) error {
	schema, err := hasSchema(ctx, db)
	if err != nil || !schema {
		return err
	}
	done, err := applied(ctx, db, BaselineVersion)
	if err != nil {
		return err
	}
	if !done {
		return ErrNeedsBaseline
	}
	return nil
}

func provider(db *sql.DB) (*goose.Provider, error) {
	return goose.NewProvider(goose.DialectPostgres, db, files,
		goose.WithTableName(Table),
		goose.WithDisableGlobalRegistry(true))
}

// prepare cria o schema do historico, que o goose espera existir.
func prepare(ctx context.Context, db *sql.DB) (*goose.Provider, error) {
	if _, err := db.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS "+Schema); err != nil {
		return nil, fmt.Errorf("criar o schema %s: %w", Schema, err)
	}
	return provider(db)
}

// hasSchema: o banco ja tem as tabelas da base?
func hasSchema(ctx context.Context, db *sql.DB) (bool, error) {
	var ok bool
	err := db.QueryRowContext(ctx, "SELECT to_regclass('public.users') IS NOT NULL").Scan(&ok)
	return ok, err
}

func historyExists(ctx context.Context, db *sql.DB) (bool, error) {
	var ok bool
	err := db.QueryRowContext(ctx, "SELECT to_regclass($1) IS NOT NULL", Table).Scan(&ok)
	return ok, err
}

func applied(ctx context.Context, db *sql.DB, version int64) (bool, error) {
	tracked, err := historyExists(ctx, db)
	if err != nil || !tracked {
		return false, err
	}
	var ok bool
	err = db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM "+Table+" WHERE version_id = $1 AND is_applied)", version).Scan(&ok)
	return ok, err
}
