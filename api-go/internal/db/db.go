package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect abre o pool contra o mesmo Postgres que o Kotlin usa (Supabase).
//
// O pool e pequeno de proposito. O banco e externo e gerenciado: cada conexao
// custa dos dois lados, e o alvo de deploy e uma VM de 1 OCPU. O Kotlin roda com
// Hikari em 5 conexoes; nao ha motivo para o Go pedir mais enquanto os dois
// processos convivem durante o strangler -- somados, precisam caber no limite
// do plano.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("DB_URL invalida: %w", err)
	}

	cfg.MaxConns = 5
	cfg.MinConns = 1
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("nao foi possivel criar o pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("banco inacessivel: %w", err)
	}
	return pool, nil
}
