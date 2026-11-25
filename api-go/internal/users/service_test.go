package users

import (
	"context"
	"testing"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/dbtest"
	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
	"github.com/JamesWillian/loobby-api/api-go/internal/password"
)

// A exclusao de conta limpa a 2.0 na mesma transacao: votos, comentarios e
// participacoes saem, e o que a pessoa criou passa para outra.
//
// Num banco so deste teste: a exclusao grava username "", que e unico, e no
// banco compartilhado uma segunda exclusao ao mesmo tempo daria 500
// (contract/ACHADOS.md §23).
func TestExclusaoDeContaLimpaAsGameplays(t *testing.T) {
	ctx := context.Background()
	pool, err := db.Connect(ctx, dbtest.MigratedDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	count := func(sql string, args ...any) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	gone, owner := ids.New(), ids.New()
	for _, id := range []string{gone, owner} {
		exec(`INSERT INTO users (id, username, displayname, auth_provider) VALUES ($1, $2, $2, 1)`, id, "u_"+id[:8])
	}
	hash, err := password.Hash("senha-123")
	if err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO user_credentials (user_id, email, password_hash) VALUES ($1, 'sai@loobby.app', $2)`, gone, hash)
	group := ids.New()
	exec(`INSERT INTO groups (id, name, invite_code, owner_id) VALUES ($1, 'Turma', '$-ABC123', $2)`, group, owner)
	exec(`INSERT INTO group_members (id, group_id, user_id) VALUES (gen_random_uuid(), $1, $2)`, group, gone)

	inGroup, single, game := ids.New(), ids.New(), ids.New()
	when := time.Now().Add(2 * time.Hour)
	exec(`INSERT INTO gameplays (id, group_id, creator_id, name, mode, platforms, scheduled_at, finalists, multi_vote, voting_round)
	      VALUES ($1, $2, $3, 'Sexta', 'VOTING', '["PC"]', $4, 1, false, 1)`, inGroup, group, gone, when)
	exec(`INSERT INTO gameplays (id, creator_id, name, mode, platforms, scheduled_at, invite_code)
	      VALUES ($1, $2, 'Sozinho', 'FREE', '["PC"]', $3, '$-SOZINHO1')`, single, gone, when)
	exec(`INSERT INTO gameplay_participants (gameplay_id, user_id, status) VALUES ($1, $3, 'GOING'), ($2, $3, 'GOING')`, inGroup, single, gone)
	exec(`INSERT INTO gameplay_games (id, gameplay_id, name, added_by) VALUES ($1, $2, 'Valorant', $3)`, game, inGroup, gone)
	exec(`INSERT INTO gameplay_votes (gameplay_game_id, voting_round, user_id, gameplay_id) VALUES ($1, 1, $2, $3)`, game, gone, inGroup)
	exec(`INSERT INTO gameplay_comments (id, gameplay_id, user_id, body) VALUES (gen_random_uuid(), $1, $2, 'gg')`, inGroup, gone)

	if err := NewService(pool).DeleteAccount(ctx, gone, "senha-123"); err != nil {
		t.Fatal(err)
	}

	for name, sql := range map[string]string{
		"votos":         `SELECT count(*) FROM gameplay_votes WHERE user_id = $1`,
		"comentarios":   `SELECT count(*) FROM gameplay_comments WHERE user_id = $1`,
		"participacoes": `SELECT count(*) FROM gameplay_participants WHERE user_id = $1`,
		"criadas":       `SELECT count(*) FROM gameplays WHERE creator_id = $1`,
	} {
		if n := count(sql, gone); n != 0 {
			t.Errorf("%s de quem excluiu a conta: %d", name, n)
		}
	}
	if n := count(`SELECT count(*) FROM gameplays WHERE id = $1 AND creator_id = $2`, inGroup, owner); n != 1 {
		t.Error("a gameplay do grupo passa para o dono do grupo")
	}
	if n := count(`SELECT count(*) FROM gameplays WHERE id = $1`, single); n != 0 {
		t.Error("a unica sem mais ninguem e apagada")
	}
	if n := count(`SELECT count(*) FROM users WHERE id = $1 AND username = '' AND displayname = '[deleted]'`, gone); n != 1 {
		t.Error("o usuario continua sendo anonimizado, como na v1")
	}
}
