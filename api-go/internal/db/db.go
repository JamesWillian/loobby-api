package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
	cfg.AfterConnect = RegisterTypes

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

// RegisterTypes faz o numeric chegar como o TEXTO que o Postgres escreve.
//
// Pelo protocolo binario, o pgx perde a escala justamente do zero: numeric(4,2)
// 0.00 chega como Int=0 Exp=0, e vira "0" -- enquanto 25.50 chega com Exp=-2 e
// sobrevive (medido). O Kotlin responde "0.00" (rating do cache) e "25.50"
// (pricePerPlayer); o Go precisa da escala exata, e so o texto do Postgres a
// preserva em todos os casos.
//
// Com o TextCodec, o pgx pede numeric em formato texto e entrega em string; o
// sqlc.yaml mapeia numeric para string. Parametros string tambem seguem como
// texto, e o Postgres converte para numeric.
//
// Toda conexao precisa disto: um pool aberto sem passar por aqui leria 0.00
// como "0" sem erro nenhum. Por isso db_test.go trava o comportamento.
func RegisterTypes(_ context.Context, conn *pgx.Conn) error {
	conn.TypeMap().RegisterType(&pgtype.Type{
		Name:  "numeric",
		OID:   pgtype.NumericOID,
		Codec: pgtype.TextCodec{},
	})
	return nil
}
