// Package dbtest da aos testes um Postgres de verdade: o de TEST_DATABASE_URL,
// com o schema de producao (ver api-go/README.md). Sem a variavel, o teste e
// pulado.
//
// O banco e dividido entre os pacotes, que o `go test` roda ao mesmo tempo.
// Por isso cada teste cria os proprios usuarios, com ids e nomes sorteados, e
// apaga so o que criou -- nunca limpa uma tabela inteira.
package dbtest

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
)

// Pool abre o pool pelo db.Connect -- o mesmo caminho de producao, com os
// tipos registrados.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL nao definida")
	}
	pool, err := db.Connect(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// User cria um usuario (authProvider: 0 anonimo, 1 email, 2 Google) e o apaga
// no fim do teste, junto com tudo que aponta para ele.
func User(t *testing.T, pool *pgxpool.Pool, authProvider int) string {
	t.Helper()
	id := ids.New()
	name := "teste_" + ids.New()[:8]
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, username, displayname, auth_provider) VALUES ($1, $2, $2, $3)`, id, name, authProvider)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { Delete(pool, id) })
	return id
}

// Delete apaga um usuario e o que depende dele. Grupos e eventos nao tem
// ON DELETE CASCADE para o dono, e as credenciais do Google tambem nao.
func Delete(pool *pgxpool.Pool, userID string) {
	ctx := context.Background()
	for _, q := range []string{
		`DELETE FROM events WHERE owner_id = $1`,
		`DELETE FROM groups WHERE owner_id = $1`,
		`DELETE FROM user_google_credentials WHERE user_id = $1`,
		`DELETE FROM users WHERE id = $1`,
	} {
		_, _ = pool.Exec(ctx, q, userID)
	}
}
