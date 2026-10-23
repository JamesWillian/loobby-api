// Package server monta a cadeia HTTP completa. Fica fora do main para que os
// testes de contrato exercitem exatamente a mesma fiacao que vai para producao --
// inclusive a ordem dos middlewares, que importa: SpringHeaders precisa envolver
// o JWT, porque o Kotlin escreve os headers de seguranca ate no 401 do filtro.
package server

import (
	"net/http"

	"github.com/JamesWillian/loobby-api/api-go/internal/auth"
	"github.com/JamesWillian/loobby-api/api-go/internal/config"
	"github.com/JamesWillian/loobby-api/api-go/internal/games"
	"github.com/JamesWillian/loobby-api/api-go/internal/health"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
	"github.com/JamesWillian/loobby-api/api-go/internal/legal"
)

type Deps struct {
	Config config.Config
	DB     health.Pinger
	Games  *games.Handler // nil: rotas de jogos nao sao registradas
}

func Handler(d Deps) http.Handler {
	mux := http.NewServeMux()
	jwt := auth.NewMiddleware(d.Config.JWTSecret, d.Config.AcceptRefreshTokenAsAccess)

	mux.Handle("GET /actuator/health", health.Handler(d.DB))
	legal.Register(mux)
	if d.Games != nil {
		d.Games.Register(mux, jwt.Require)
	}

	// Rota temporaria: prova o middleware ponta a ponta contra um token real do
	// Kotlin. Nao e contrato e o nginx nao a expoe; some quando /users/me migrar.
	mux.Handle("GET /internal/whoami", jwt.Require(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			p := auth.MustPrincipal(r.Context())
			httpx.WriteJSON(w, http.StatusOK, map[string]any{
				"userId": p.UserID, "username": p.Username,
				"roles": p.Roles, "tokenType": p.Type,
			})
		})))

	return httpx.SpringHeaders(mux)
}
