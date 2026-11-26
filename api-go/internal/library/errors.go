package library

import (
	"net/http"

	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

// Os erros da v2, com as mensagens das gameplays onde a situacao e a mesma.
var (
	errGameNotFound  = httpx.Status(http.StatusNotFound, "Jogo não encontrado")
	errGroupNotFound = httpx.Status(http.StatusNotFound, "Grupo não encontrado")
	errUserNotFound  = httpx.Status(http.StatusNotFound, "Usuário não encontrado")

	errAnonymous = httpx.Status(http.StatusForbidden, "Conta anônima só pode ver. Crie uma conta para participar.")
	errNotMember = httpx.Status(http.StatusForbidden, "Você não é membro desse grupo")

	errUserFull  = httpx.Status(http.StatusConflict, "Sua biblioteca já tem 500 jogos")
	errGroupFull = httpx.Status(http.StatusConflict, "A biblioteca do grupo já tem 100 jogos")

	errBadBody = httpx.BadRequest("Corpo da requisição inválido")
)

// fieldError responde como a validacao das gameplays: 400 "Dados inválidos",
// com a falha em fieldErrors.
func fieldError(w http.ResponseWriter, r *http.Request, field, msg string) {
	title := "Dados inválidos"
	body := httpx.NewError(http.StatusBadRequest, &title, httpx.RequestPath(r))
	body.FieldErrors = []httpx.FieldError{{Field: field, Message: &msg}}
	httpx.WriteErrorResponse(w, http.StatusBadRequest, body)
}
