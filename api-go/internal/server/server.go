// Package server monta a cadeia HTTP completa. Fica fora do main para que os
// testes de contrato exercitem exatamente a mesma fiacao que vai para producao --
// inclusive a ordem das camadas, que importa:
//
//	SpringHeaders -> auth.Filter -> precedencia de rotas -> mux -> handler
//
// SpringHeaders e a mais externa porque o Kotlin escreve os headers de
// seguranca ate no 401 do filtro; o filtro vem antes do roteamento porque o
// Spring Security barra a requisicao antes de saber se a rota existe.
package server

import (
	"net/http"
	"strings"

	"github.com/JamesWillian/loobby-api/api-go/internal/appconfig"
	"github.com/JamesWillian/loobby-api/api-go/internal/auth"
	"github.com/JamesWillian/loobby-api/api-go/internal/config"
	"github.com/JamesWillian/loobby-api/api-go/internal/events"
	"github.com/JamesWillian/loobby-api/api-go/internal/files"
	"github.com/JamesWillian/loobby-api/api-go/internal/gameplays"
	"github.com/JamesWillian/loobby-api/api-go/internal/games"
	"github.com/JamesWillian/loobby-api/api-go/internal/groups"
	"github.com/JamesWillian/loobby-api/api-go/internal/health"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
	"github.com/JamesWillian/loobby-api/api-go/internal/legal"
	"github.com/JamesWillian/loobby-api/api-go/internal/notifications"
	"github.com/JamesWillian/loobby-api/api-go/internal/teams"
	"github.com/JamesWillian/loobby-api/api-go/internal/users"
)

// Deps sao os modulos ja construidos. Um modulo nil nao tem as rotas
// registradas -- os testes montam so o que exercitam.
type Deps struct {
	Config config.Config
	DB     health.Pinger
	Tokens *auth.Tokens // nil: criado a partir de Config.JWTSecret

	Auth    *auth.Handler
	Users   *users.Handler
	Groups  *groups.Handler
	Events  *events.Handler
	Teams   *teams.Handler
	Games   *games.Handler
	Devices *notifications.Handler
	Files   *files.Storage

	Gameplays *gameplays.Handler // a 2.0, em /v2
}

func Handler(d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /actuator/health", health.Handler(d.DB))
	legal.Register(mux)
	appconfig.Register(mux)
	if d.Auth != nil {
		d.Auth.Register(mux)
	}
	if d.Users != nil {
		d.Users.Register(mux)
	}
	if d.Groups != nil {
		d.Groups.Register(mux)
	}
	if d.Events != nil {
		d.Events.Register(mux)
	}
	if d.Teams != nil {
		d.Teams.Register(mux)
	}
	if d.Games != nil {
		d.Games.Register(mux)
	}
	if d.Devices != nil {
		d.Devices.Register(mux)
	}
	if d.Files != nil {
		d.Files.Register(mux)
	}
	if d.Gameplays != nil {
		d.Gameplays.Register(mux)
	}

	tokens := d.Tokens
	if tokens == nil {
		tokens = auth.NewTokens(d.Config.JWTSecret)
	}
	filter := auth.NewFilter(tokens, d.Config.AcceptRefreshTokenAsAccess, isPublic)
	return httpx.SpringHeaders(filter.Wrap(route(mux, d)))
}

// isPublic e a lista de permitAll() do SecurityConfig do Kotlin, mais o
// /app/config, que o app consulta antes de ter sessao. A comparacao e por
// metodo exato e caminho exato: HEAD numa rota de GET nao e publica, e uma
// barra a mais no fim tambem nao (as duas coisas medidas no Kotlin).
func isPublic(method, path string) bool {
	switch method {
	case http.MethodPost:
		switch path {
		case "/auth/anonymous", "/auth/refresh", "/auth/login", "/auth/google",
			"/auth/forgot-password", "/auth/reset-password":
			return true
		}
	case http.MethodGet:
		switch path {
		case "/auth/verify-email", "/auth/reset-password", "/actuator/health", "/files", "/legal",
			"/app/config":
			return true
		}
		return strings.HasPrefix(path, "/files/") || strings.HasPrefix(path, "/legal/")
	}
	return false
}

// route entrega a requisicao ao mux, com dois ajustes para o roteamento ficar
// igual ao do Spring MVC.
//
// 1. Rota que nao existe (ou metodo que a rota nao aceita). O ServeMux
// responderia 404 ou 405; no Kotlin isso e uma excecao sem handler proprio e
// cai no catch-all: 500 "Unexpected error" (medido com GET /nada e PUT /groups).
//
// 2. Os convites. /groups/invite/{code} e /events/invite/{code} se sobrepoem a
// rotas como /groups/{groupId}/members: o caminho /groups/invite/members casa
// com as duas. O ServeMux recusa registrar padroes assim; o Spring registra e
// desempata pelo padrao mais longo. As rotas de convite ficam entao fora do
// mux, e o desempate do Spring e aplicado aqui:
//
//	GET /events/invite/X    sempre o convite (vence .../rsvps e .../teams)
//	GET /groups/invite/X    o convite, exceto X = "members", que e a lista de
//	                        membros do "grupo" invite (e da 500: nao e UUID)
func route(mux *http.ServeMux, d Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			if seg := strings.Split(r.URL.Path, "/"); len(seg) == 4 && seg[2] == "invite" && seg[3] != "" {
				switch {
				case seg[1] == "events" && d.Events != nil:
					d.Events.ServeInvite(w, r, seg[3])
					return
				case seg[1] == "groups" && d.Groups != nil && seg[3] != "members":
					d.Groups.ServeInvite(w, r, seg[3])
					return
				}
			}
		}
		if _, pattern := mux.Handler(r); pattern == "" {
			httpx.Unexpected(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
