package gameplays

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JamesWillian/loobby-api/api-go/internal/dbtest"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
	"github.com/JamesWillian/loobby-api/api-go/internal/notifications"
)

// Os testes usam o banco de TEST_DATABASE_URL, dividido com os outros pacotes:
// cada um cria os proprios usuarios, grupos e gameplays.

type push struct {
	to []string
	p  notifications.Payload
}

// recorder faz o papel do Firebase: guarda o que seria enviado.
type recorder struct {
	mu     sync.Mutex
	pushes []push
}

func (r *recorder) SendToUsers(_ context.Context, userIDs []string, p notifications.Payload) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pushes = append(r.pushes, push{to: append([]string(nil), userIDs...), p: p})
}

// to: quem recebeu pushes do tipo kind, na ordem, sem repetir.
func (r *recorder) to(kind string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, p := range r.pushes {
		if p.p.Type != kind {
			continue
		}
		for _, id := range p.to {
			if !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
	}
	return out
}

type cacheSpy struct {
	mu  sync.Mutex
	ids []string
}

func (c *cacheSpy) EnsureCached(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ids = append(c.ids, id)
}

type fixture struct {
	t     *testing.T
	pool  *pgxpool.Pool
	svc   *Service
	sent  *recorder
	cache *cacheSpy
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := dbtest.Pool(t)
	sent, cache := &recorder{}, &cacheSpy{}
	svc := NewService(pool, sent, cache)
	svc.async = func(_ string, fn func(ctx context.Context)) { fn(context.Background()) } // sem goroutine: o teste ve o push na hora
	return &fixture{t: t, pool: pool, svc: svc, sent: sent, cache: cache}
}

func (f *fixture) ctx() context.Context { return context.Background() }

func (f *fixture) user() string      { return dbtest.User(f.t, f.pool, 1) }
func (f *fixture) anonymous() string { return dbtest.User(f.t, f.pool, 0) }

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.ctx(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

// group cria um grupo de owner (o gatilho do banco o inclui como membro) com
// os outros membros.
func (f *fixture) group(owner string, members ...string) string {
	f.t.Helper()
	id := ids.New()
	f.exec(`INSERT INTO groups (id, name, invite_code, owner_id) VALUES ($1, $2, $3, $4)`,
		id, "Turma "+id[:6], "$-"+ids.Code(6), owner)
	for _, m := range members {
		f.join(id, m)
	}
	f.t.Cleanup(func() { _, _ = f.pool.Exec(context.Background(), `DELETE FROM groups WHERE id = $1`, id) })
	return id
}

func (f *fixture) join(groupID, userID string) {
	f.exec(`INSERT INTO group_members (id, group_id, user_id) VALUES (gen_random_uuid(), $1, $2)`, groupID, userID)
}

func soon() time.Time { return time.Now().Add(2 * time.Hour).Round(time.Second) }

func votingIn(names ...string) createInput {
	in := createInput{Name: "Sexta do CS", Mode: ModeVoting, Platforms: []string{"PC"}, ScheduledAt: soon(), Finalists: 1}
	for _, n := range names {
		in.Games = append(in.Games, gameInput{Name: n})
	}
	return in
}

func (f *fixture) create(userID string, groupID *string, in createInput) gameplayResponse {
	f.t.Helper()
	resp, err := f.svc.Create(f.ctx(), userID, groupID, in)
	if err != nil {
		f.t.Fatalf("criar gameplay: %v", err)
	}
	return resp
}

func (f *fixture) get(id, userID string) gameplayResponse {
	f.t.Helper()
	resp, err := f.svc.Get(f.ctx(), id, userID)
	if err != nil {
		f.t.Fatalf("ler gameplay: %v", err)
	}
	return resp
}

// gameID: o id, na votacao, do jogo de nome name.
func gameID(t *testing.T, g gameplayResponse, name string) string {
	t.Helper()
	for _, gm := range g.Voting.Games {
		if gm.Name == name {
			return gm.ID
		}
	}
	t.Fatalf("o jogo %q nao esta na votacao", name)
	return ""
}

func (f *fixture) vote(id, userID string, choice ...string) gameplayResponse {
	f.t.Helper()
	resp, err := f.svc.Vote(f.ctx(), id, userID, choice)
	if err != nil {
		f.t.Fatalf("votar: %v", err)
	}
	return resp
}

func (f *fixture) respond(id, userID, status string) gameplayResponse {
	f.t.Helper()
	resp, err := f.svc.Respond(f.ctx(), id, userID, status)
	if err != nil {
		f.t.Fatalf("responder: %v", err)
	}
	return resp
}

// insertGameplay grava direto no banco, sem as regras do servico -- para a
// gameplay que comecou ontem, que a API nao deixaria criar.
func (f *fixture) insertGameplay(creator string, groupID *string, mode string, scheduled time.Time) string {
	f.t.Helper()
	id := ids.New()
	var code *string
	if groupID == nil {
		c := "$-" + ids.Code(inviteCodeLen)
		code = &c
	}
	var finalists *int16
	var multi *bool
	var round *int16
	var gameName *string
	switch mode {
	case ModeVoting:
		finalists, round = int16p(1), int16p(1)
		no := false
		multi = &no
	case ModeFixed:
		name := "Jogo"
		gameName = &name
	}
	f.exec(`INSERT INTO gameplays (id, group_id, creator_id, name, mode, platforms, scheduled_at, invite_code,
	                               game_name, finalists, multi_vote, voting_round)
	        VALUES ($1, $2, $3, $4, $5, '["PC"]', $6, $7, $8, $9, $10, $11)`,
		id, groupID, creator, "Gameplay "+id[:6], mode, scheduled, code, gameName, finalists, multi, round)
	f.exec(`INSERT INTO gameplay_participants (gameplay_id, user_id, status) VALUES ($1, $2, 'GOING')`, id, creator)
	return id
}

// wantErr confere que err e o erro esperado da v2 (mesmo status e mensagem).
func wantErr(t *testing.T, err error, want *httpx.Error) {
	t.Helper()
	var got *httpx.Error
	if !errors.As(err, &got) || got.Status != want.Status || *got.Message != *want.Message {
		t.Fatalf("queria %d %q, veio %v", want.Status, *want.Message, err)
	}
}

func participantStatus(g gameplayResponse, userID string) string {
	for _, p := range g.Participants {
		if p.User.ID == userID {
			return p.Status
		}
	}
	return ""
}
