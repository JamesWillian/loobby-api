package groups

import (
	"context"
	"testing"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/dbtest"
	"github.com/JamesWillian/loobby-api/api-go/internal/files"
	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
)

// Sair do grupo, ou ser removido, tira a pessoa das gameplays ativas do grupo
// (participacao e voto aberto). O resto do comportamento da v1 nao muda -- o
// contrato (cmd/api) cobre as respostas.
func TestSairDoGrupoTiraDasGameplaysAtivas(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Pool(t)
	storage, err := files.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(pool, storage)
	owner, leaver, removed := dbtest.User(t, pool, 1), dbtest.User(t, pool, 1), dbtest.User(t, pool, 1)
	g, err := svc.Create(ctx, owner, "grupo do teste de saida", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{leaver, removed} {
		if err := svc.Join(ctx, u, g.ID); err != nil {
			t.Fatal(err)
		}
	}
	gameplay, game := ids.New(), ids.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO gameplays (id, group_id, creator_id, name, mode, platforms, scheduled_at, finalists, multi_vote, voting_round)
	      VALUES ($1, $2, $3, 'Sexta', 'VOTING', '["PC"]', $4, 1, false, 1)`, gameplay, g.ID, owner, time.Now().Add(time.Hour))
	exec(`INSERT INTO gameplay_games (id, gameplay_id, name) VALUES ($1, $2, 'Valorant')`, game, gameplay)
	for _, u := range []string{leaver, removed} {
		exec(`INSERT INTO gameplay_participants (gameplay_id, user_id, status) VALUES ($1, $2, 'GOING')`, gameplay, u)
		exec(`INSERT INTO gameplay_votes (gameplay_game_id, voting_round, user_id, gameplay_id) VALUES ($1, 1, $2, $3)`, game, u, gameplay)
	}
	left := func(user string) bool {
		var n int
		err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM gameplay_participants WHERE gameplay_id = $1 AND user_id = $2)
		                                + (SELECT count(*) FROM gameplay_votes WHERE gameplay_id = $1 AND user_id = $2)`,
			gameplay, user).Scan(&n)
		if err != nil {
			t.Fatal(err)
		}
		return n == 0
	}

	if err := svc.Leave(ctx, leaver, g.ID); err != nil {
		t.Fatal(err)
	}
	if !left(leaver) || left(removed) {
		t.Errorf("quem sai perde participacao e voto; os outros ficam (saiu: %v, ficou: %v)", left(leaver), !left(removed))
	}
	if err := svc.RemoveMember(ctx, owner, g.ID, removed); err != nil {
		t.Fatal(err)
	}
	if !left(removed) {
		t.Error("quem e removido tambem perde participacao e voto")
	}
	if err := svc.Leave(ctx, owner, g.ID); err == nil {
		t.Error("o dono continua sem poder sair, como na v1")
	}
}
