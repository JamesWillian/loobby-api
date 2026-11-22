// Comando migrate: confere e aplica as migracoes do banco (api-go/migrations).
//
//	go run ./cmd/migrate status     so le: o que ja foi aplicado e o que falta
//	go run ./cmd/migrate up         aplica o que falta
//	go run ./cmd/migrate baseline   so na producao, uma vez: marca a base como aplicada
//
// O banco vem de MIGRATE_DATABASE_URL, no formato do pgx (postgres://...), e
// nao de um argumento: assim a senha nao fica na lista de processos. Na
// producao e a conexao de SESSAO do Supabase (porta 5432). O pooler em modo
// transacao (6543), o do DB_URL da API, e recusado.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pressly/goose/v3"

	"github.com/JamesWillian/loobby-api/api-go/migrations"
)

func main() {
	if len(os.Args) != 2 {
		usage()
	}
	command := os.Args[1]
	if command != "status" && command != "up" && command != "baseline" {
		usage()
	}
	url := os.Getenv("MIGRATE_DATABASE_URL")
	if url == "" {
		fail(errors.New("defina MIGRATE_DATABASE_URL com a URL do banco"))
	}
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		fail(fmt.Errorf("URL do banco invalida: %w", err))
	}
	db, err := migrations.Open(url)
	if err != nil {
		fail(err)
	}
	defer db.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	fmt.Printf("banco: %s@%s:%d/%s\n", cfg.User, cfg.Host, cfg.Port, cfg.Database)
	switch command {
	case "status":
		list, err := migrations.Status(ctx, db)
		if err != nil {
			fail(err)
		}
		for _, m := range list {
			when := ""
			if m.State == goose.StateApplied {
				when = m.AppliedAt.Format("2006-01-02 15:04")
			}
			fmt.Printf("%-9s %-16s %s\n", m.State, when, filepath.Base(m.Source.Path))
		}
	case "up":
		results, err := migrations.Up(ctx, db)
		for _, r := range results {
			fmt.Println(r)
		}
		if err != nil {
			fail(err)
		}
		if len(results) == 0 {
			fmt.Println("nada a aplicar")
		}
	case "baseline":
		marked, err := migrations.Baseline(ctx, db)
		if err != nil {
			fail(err)
		}
		if marked {
			fmt.Printf("migracao %05d marcada como aplicada, sem rodar\n", migrations.BaselineVersion)
		} else {
			fmt.Println("a base ja estava marcada: nada a fazer")
		}
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "uso: MIGRATE_DATABASE_URL=postgres://... go run ./cmd/migrate status|up|baseline")
	os.Exit(2)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "erro:", err)
	os.Exit(1)
}
