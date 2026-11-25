package gameplays

import (
	"context"
	"testing"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
)

func (f *fixture) countRows(sql string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.pool.QueryRow(f.ctx(), sql, args...).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f *fixture) creatorOf(id string) string {
	f.t.Helper()
	var creator string
	if err := f.pool.QueryRow(f.ctx(), `SELECT creator_id FROM gameplays WHERE id = $1`, id).Scan(&creator); err != nil {
		f.t.Fatal(err)
	}
	return creator
}

func TestExclusaoDeContaLimpaAs2_0(t *testing.T) {
	f := newFixture(t)
	gone, owner, ana, anon, bia := f.user(), f.user(), f.user(), f.anonymous(), f.user()
	group := f.group(owner, gone, ana)

	inGroup := f.create(gone, &group, votingIn("A", "B")) // passa para o dono do grupo
	f.vote(inGroup.ID, gone, gameID(t, inGroup, "A"))
	f.vote(inGroup.ID, ana, gameID(t, inGroup, "B"))
	mine, _ := f.svc.AddComment(f.ctx(), inGroup.ID, gone, "meu")
	theirs, _ := f.svc.AddComment(f.ctx(), inGroup.ID, ana, "da ana")
	if err := f.svc.Report(f.ctx(), inGroup.ID, gone, theirs.ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Report(f.ctx(), inGroup.ID, ana, mine.ID, nil); err != nil {
		t.Fatal(err)
	}
	ownedByOther := f.create(owner, &group, votingIn("C"))
	f.respond(ownedByOther.ID, gone, StatusGoing)

	// a unica passa para o participante mais antigo que nao e anonimo
	single := f.create(gone, nil, votingIn())
	for _, u := range []string{anon, bia} {
		if _, err := f.svc.Join(f.ctx(), *single.InviteCode, u); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	alone := f.create(gone, nil, votingIn()) // sem mais ninguem: some

	err := db.InTx(f.ctx(), f.pool, func(q *sqlc.Queries) error { return DeleteUserData(context.Background(), q, gone) })
	if err != nil {
		t.Fatal(err)
	}

	if c := f.creatorOf(inGroup.ID); c != owner {
		t.Errorf("gameplay do grupo passa para o dono do grupo: %s", c)
	}
	if c := f.creatorOf(single.ID); c != bia {
		t.Errorf("a unica passa para o participante nao anonimo mais antigo: %s", c)
	}
	if n := f.countRows(`SELECT count(*) FROM gameplays WHERE id = $1`, alone.ID); n != 0 {
		t.Error("a unica sem mais ninguem e apagada")
	}
	for name, sql := range map[string]string{
		"votos":          `SELECT count(*) FROM gameplay_votes WHERE user_id = $1`,
		"comentarios":    `SELECT count(*) FROM gameplay_comments WHERE user_id = $1`,
		"denuncias":      `SELECT count(*) FROM gameplay_comment_reports WHERE reporter_id = $1`,
		"participacoes":  `SELECT count(*) FROM gameplay_participants WHERE user_id = $1`,
		"jogos com nome": `SELECT count(*) FROM gameplay_games WHERE added_by = $1`,
	} {
		if n := f.countRows(sql, gone); n != 0 {
			t.Errorf("%s de quem excluiu a conta: %d", name, n)
		}
	}
	// o que e dos outros fica
	if n := f.countRows(`SELECT count(*) FROM gameplay_votes WHERE user_id = $1`, ana); n != 1 {
		t.Errorf("o voto da ana fica: %d", n)
	}
	if n := f.countRows(`SELECT count(*) FROM gameplay_comments WHERE id = $1`, theirs.ID); n != 1 {
		t.Error("o comentario da ana fica")
	}
	if n := f.countRows(`SELECT count(*) FROM gameplay_games WHERE gameplay_id = $1`, inGroup.ID); n != 2 {
		t.Errorf("os jogos que ele pos ficam, sem autor: %d", n)
	}
}

func TestQuemSaiDoGrupoPerdeAParticipacaoNasAtivas(t *testing.T) {
	f := newFixture(t)
	owner, member := f.user(), f.user()
	group := f.group(owner, member)
	active := f.create(owner, &group, votingIn("A"))
	f.vote(active.ID, member, gameID(t, active, "A"))
	ended := f.insertGameplay(owner, &group, ModeFree, time.Now().Add(-30*time.Hour))
	f.exec(`INSERT INTO gameplay_participants (gameplay_id, user_id, status) VALUES ($1, $2, 'GOING')`, ended, member)

	f.exec(`DELETE FROM group_members WHERE group_id = $1 AND user_id = $2`, group, member)
	err := db.InTx(f.ctx(), f.pool, func(q *sqlc.Queries) error {
		return MemberLeft(context.Background(), q, group, member, time.Now())
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := f.countRows(`SELECT count(*) FROM gameplay_participants WHERE gameplay_id = $1 AND user_id = $2`, active.ID, member); n != 0 {
		t.Error("na ativa, a participacao sai")
	}
	if n := f.countRows(`SELECT count(*) FROM gameplay_votes WHERE gameplay_id = $1 AND user_id = $2`, active.ID, member); n != 0 {
		t.Error("na ativa, o voto aberto sai")
	}
	if n := f.countRows(`SELECT count(*) FROM gameplay_participants WHERE gameplay_id = $1 AND user_id = $2`, ended, member); n != 1 {
		t.Error("na encerrada, fica (e historico)")
	}
}
