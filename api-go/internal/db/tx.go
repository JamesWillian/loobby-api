package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
)

// InTx roda fn numa transacao: confirma se fn devolver nil, desfaz caso
// contrario. E o @Transactional do Kotlin -- inclusive no detalhe de que um
// erro de regra de negocio (httpx.Error) tambem desfaz tudo o que ja foi escrito.
func InTx(ctx context.Context, pool *pgxpool.Pool, fn func(q *sqlc.Queries) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() // sem efeito depois do Commit
	if err := fn(sqlc.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// NotFound: a consulta :one nao achou linha. E o Optional vazio do JPA.
func NotFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
