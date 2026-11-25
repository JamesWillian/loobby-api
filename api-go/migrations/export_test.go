package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

// UpTo aplica as migracoes ate a versao dada. So para os testes: monta um
// banco como o da producao antes de uma migracao nova.
func UpTo(ctx context.Context, db *sql.DB, version int64) ([]*goose.MigrationResult, error) {
	p, err := prepare(ctx, db)
	if err != nil {
		return nil, err
	}
	return p.UpTo(ctx, version)
}
