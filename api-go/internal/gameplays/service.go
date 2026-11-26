package gameplays

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
	"github.com/JamesWillian/loobby-api/api-go/internal/ids"
	"github.com/JamesWillian/loobby-api/api-go/internal/notifications"
)

type Service struct {
	pool   *pgxpool.Pool
	q      *sqlc.Queries
	sender notifications.Sender
	games  GameCache
	now    func() time.Time
	async  runner
}

func NewService(pool *pgxpool.Pool, sender notifications.Sender, games GameCache) *Service {
	return &Service{
		pool: pool, q: sqlc.New(pool), sender: sender, games: games,
		now: time.Now, async: background,
	}
}

// background roda um push fora da requisicao: a resposta nao espera o
// Firebase, e uma falha dele nao derruba a acao (como na v1).
func background(name string, fn func(ctx context.Context)) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("push da gameplay: panico", "gancho", name, "erro", r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		fn(ctx)
	}()
}

// state e uma gameplay vista por um usuario: o que e preciso para decidir o
// que ele pode fazer.
type state struct {
	g         sqlc.Gameplay
	group     *sqlc.Group // nil na unica
	me        string
	anonymous bool
	mine      *sqlc.GameplayParticipant // a linha de quem le, se houver
	now       time.Time
}

// load le a gameplay para userID, e so a entrega a quem tem acesso: na do
// grupo, os membros (anonimos inclusive); na unica, quem entrou pelo convite.
// Sem acesso, 404 -- igual a quando ela nao existe. Com lock, a linha fica
// travada ate o fim da transacao.
func (s *Service) load(ctx context.Context, q *sqlc.Queries, id, userID string, lock bool) (state, error) {
	var (
		g   sqlc.Gameplay
		err error
	)
	if lock {
		g, err = q.LockGameplay(ctx, id)
	} else {
		g, err = q.GetGameplay(ctx, id)
	}
	if db.NotFound(err) {
		return state{}, errNotFound
	}
	if err != nil {
		return state{}, err
	}
	u, err := q.FindUser(ctx, userID)
	if db.NotFound(err) {
		return state{}, errUserNotFound
	}
	if err != nil {
		return state{}, err
	}
	st := state{g: g, me: userID, anonymous: u.AuthProvider == 0, now: s.now()}

	row, err := q.GetParticipant(ctx, sqlc.GetParticipantParams{GameplayID: id, UserID: userID})
	switch {
	case err == nil:
		st.mine = &row
	case !db.NotFound(err):
		return state{}, err
	}

	if g.GroupID == nil {
		if st.mine == nil {
			return state{}, errNotFound
		}
		return st, nil
	}
	group, err := q.FindGroup(ctx, *g.GroupID)
	if err != nil {
		return state{}, err // a FK com CASCADE garante o grupo
	}
	st.group = &group
	member, err := q.MemberExists(ctx, sqlc.MemberExistsParams{GroupID: group.ID, UserID: userID})
	if err != nil {
		return state{}, err
	}
	if !member {
		return state{}, errNotFound
	}
	return st, nil
}

// autoEndAt: o horario do encerramento automatico.
func autoEndAt(g sqlc.Gameplay) time.Time { return g.ScheduledAt.Add(AutoEndAfter) }

// endedAt: quando encerrou, ou nil se esta ativa. A que passou das 24 h conta
// como encerrada mesmo antes de o job carimbar o ended_at.
func endedAt(g sqlc.Gameplay, now time.Time) *time.Time {
	if g.EndedAt != nil {
		return g.EndedAt
	}
	if end := autoEndAt(g); !now.Before(end) {
		return &end
	}
	return nil
}

func (st state) active() bool    { return endedAt(st.g, st.now) == nil }
func (st state) creator() bool   { return st.g.CreatorID == st.me }
func (st state) owner() bool     { return st.group != nil && st.group.OwnerID == st.me }
func (st state) single() bool    { return st.group == nil }
func (st state) manager() bool   { return st.creator() || st.owner() } // na unica nao ha dono: so o criador
func (st state) moderator() bool { return st.owner() || (st.single() && st.creator()) }

func (st state) status() string {
	if st.mine != nil {
		return st.mine.Status
	}
	return StatusPending
}

func (st state) round() int {
	if st.g.VotingRound == nil {
		return 0
	}
	return int(*st.g.VotingRound)
}

// requireAct: as acoes de quem participa (responder, votar, comentar, mexer
// nos jogos) pedem a gameplay ativa e uma conta que nao seja anonima.
func (st state) requireAct() error {
	if !st.active() {
		return errEnded
	}
	if st.anonymous {
		return errAnonymous
	}
	return nil
}

func (st state) requireManager() error {
	if !st.active() {
		return errEnded
	}
	if !st.manager() {
		return errNotManager
	}
	return nil
}

// canEditGames: na do grupo, qualquer membro que nao seja anonimo; na unica, so
// o criador. E so na rodada principal: o desempate congela a lista.
func (st state) canEditGames() bool {
	return st.active() && !st.anonymous && st.g.Mode == ModeVoting && st.round() == 1 &&
		(!st.single() || st.creator())
}

// permissions: os botoes de quem le. locked e tie vem da votacao (ver tally).
func (st state) permissions(locked, tie bool) permissions {
	act := st.active() && !st.anonymous
	voting := st.g.Mode == ModeVoting
	return permissions{
		Respond:         act,
		Vote:            act && voting && st.status() != StatusNotGoing,
		EditGames:       st.canEditGames(),
		Comment:         act,
		Manage:          st.active() && st.manager(),
		ConfigureVoting: st.active() && voting && st.creator() && !locked,
		Tiebreak:        st.active() && voting && st.creator() && tie,
		Leave:           st.active() && st.single() && !st.creator(),
		Moderate:        st.moderator(),
	}
}

// inviteCode: "$-" + 8 caracteres de A-Z e 0-9, repetindo enquanto ja existir.
func inviteCode(ctx context.Context, q *sqlc.Queries) (string, error) {
	for {
		code := "$-" + ids.Code(inviteCodeLen)
		exists, err := q.GameplayInviteCodeExists(ctx, code)
		if err != nil {
			return "", err
		}
		if !exists {
			return code, nil
		}
	}
}

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

func int16p(v int) *int16 {
	n := int16(v)
	return &n
}
