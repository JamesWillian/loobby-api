package server

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/JamesWillian/loobby-api/api-go/internal/auth"
	"github.com/JamesWillian/loobby-api/api-go/internal/config"
	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
	"github.com/JamesWillian/loobby-api/api-go/internal/dbtest"
	"github.com/JamesWillian/loobby-api/api-go/internal/events"
	"github.com/JamesWillian/loobby-api/api-go/internal/files"
	"github.com/JamesWillian/loobby-api/api-go/internal/groups"
)

// A lista de rotas publicas e a do SecurityConfig do Kotlin, comparada por
// metodo exato e caminho exato (medido: HEAD e barra no fim nao sao publicos).
func TestRotasPublicas(t *testing.T) {
	publicas := [][2]string{
		{"POST", "/auth/anonymous"}, {"POST", "/auth/login"}, {"POST", "/auth/google"}, {"POST", "/auth/refresh"},
		{"POST", "/auth/forgot-password"}, {"POST", "/auth/reset-password"},
		{"GET", "/auth/verify-email"}, {"GET", "/auth/reset-password"}, {"GET", "/actuator/health"},
		{"GET", "/legal/privacy"}, {"GET", "/legal/qualquer/coisa"}, {"GET", "/files/avatars/x/1.png"},
	}
	for _, r := range publicas {
		if !isPublic(r[0], r[1]) {
			t.Errorf("%s %s deveria ser publica", r[0], r[1])
		}
	}
	protegidas := [][2]string{
		{"POST", "/auth/register"}, {"POST", "/auth/resend-verification"}, // exigem o token do anonimo
		{"HEAD", "/legal/privacy"}, {"HEAD", "/actuator/health"}, {"GET", "/auth/login"},
		{"POST", "/auth/verify-email"}, {"GET", "/auth/anonymous/"}, {"POST", "/auth/login/"},
		{"POST", "/files/x"}, {"GET", "/filesx"}, {"GET", "/users/me"}, {"GET", "/games/search"},
		{"OPTIONS", "/legal/privacy"}, {"GET", "/"},
	}
	for _, r := range protegidas {
		if isPublic(r[0], r[1]) {
			t.Errorf("%s %s NAO deveria ser publica", r[0], r[1])
		}
	}
}

type semNotificacao struct{}

func (semNotificacao) OnRsvpConfirmed(sqlc.Event, string)     {}
func (semNotificacao) OnGroupEventCreated(sqlc.Event, string) {}

type semJogos struct{}

func (semJogos) EnsureCached(string) {}

// O que o Spring MVC faz e o ServeMux do Go nao faria sozinho:
//   - rota que nao existe e metodo errado sao 500 (o catch-all do Kotlin), nao 404/405;
//   - /groups/invite/{code} e /events/invite/{code} disputam com as rotas de
//     {id}; o Spring desempata pelo padrao mais especifico.
func TestRoteamentoComoOSpring(t *testing.T) {
	pool := dbtest.Pool(t)
	storage, err := files.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	owner := dbtest.User(t, pool, 1)
	groupSvc := groups.NewService(pool, storage)
	group, err := groupSvc.Create(context.Background(), owner, "grupo do teste de rotas", nil)
	if err != nil {
		t.Fatal(err)
	}
	h := Handler(Deps{
		Config: config.Config{JWTSecret: secret, AcceptRefreshTokenAsAccess: true},
		DB:     fakeDB{},
		Groups: groups.NewHandler(groupSvc),
		Events: events.NewHandler(events.NewService(pool, semNotificacao{}, semJogos{})),
		Files:  storage,
	})
	bearer := map[string]string{"Authorization": "Bearer " + auth.NewTokens(secret).Access(owner, "u", []string{"USER"})}

	cases := []struct {
		name, method, path string
		hdr                map[string]string
		status             int
		contains           string
	}{
		{"rota inexistente", "GET", "/nada", bearer, 500, `"message":"Unexpected error"`},
		{"metodo errado", "PUT", "/groups", bearer, 500, `"message":"Unexpected error"`},
		{"rota inexistente sem token: o filtro responde antes", "GET", "/nada", nil, 401, ""},
		{"arquivo inexistente e publico: 500, nao 401", "GET", "/files/avatars/x/1.png", nil, 500, `"path":"/files/avatars/x/1.png"`},

		{"convite de grupo", "GET", "/groups/invite/" + group.InviteCode, bearer, 200, `"name":"grupo do teste de rotas"`},
		{"convite em minusculas", "GET", "/groups/invite/" + strings.ToLower(group.InviteCode), bearer, 200, `"id":"` + group.ID + `"`},
		{"convite que nao existe", "GET", "/groups/invite/NAOEXISTE", bearer, 400, `"message":"Invalid Invite Code"`},
		// "members" casa tambem com /groups/{groupId}/members, que e mais especifico: grupo "invite" nao e UUID
		{"/groups/invite/members", "GET", "/groups/invite/members", bearer, 500, `"message":"Unexpected error"`},
		// em eventos o convite sempre vence /events/{eventId}/rsvps e /teams
		{"/events/invite/rsvps", "GET", "/events/invite/rsvps", bearer, 400, `"message":"Invalid Invite Code"`},
		{"/events/invite/teams", "GET", "/events/invite/teams", bearer, 400, `"message":"Invalid Invite Code"`},
		{"evento com id que nao e uuid", "GET", "/events/xyz/rsvps", bearer, 500, `"message":"Unexpected error"`},
	}
	for _, c := range cases {
		rec := do(h, c.method, c.path, c.hdr)
		if rec.Code != c.status || !strings.Contains(rec.Body.String(), c.contains) {
			t.Errorf("%s: %s %s -> %d %s (esperava %d contendo %s)", c.name, c.method, c.path, rec.Code, rec.Body, c.status, c.contains)
		}
		// os headers do Spring Security saem em toda resposta, inclusive nos erros
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("X-Frame-Options") != "DENY" {
			t.Errorf("%s: faltam os headers de seguranca", c.name)
		}
	}
	// HEAD segue o mesmo desempate do GET (o corpo quem corta e o servidor HTTP)
	if rec := do(h, http.MethodHead, "/groups/invite/"+group.InviteCode, bearer); rec.Code != 200 {
		t.Errorf("HEAD no convite: %d", rec.Code)
	}
}
