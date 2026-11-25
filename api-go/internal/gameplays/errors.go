package gameplays

import (
	"net/http"

	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

// Os erros da v2. O envelope e o da v1 (httpx.Fail); o status e a mensagem,
// nao: 404 para o que nao existe ou nao e visivel, 409 para conflito de estado,
// e textos em portugues que o app pode mostrar como estao.

func notFound(msg string) *httpx.Error { return httpx.Status(http.StatusNotFound, msg) }
func conflict(msg string) *httpx.Error { return httpx.Status(http.StatusConflict, msg) }
func forbidden(msg string) *httpx.Error {
	return httpx.Status(http.StatusForbidden, msg)
}

var (
	errNotFound        = notFound("Gameplay não encontrada")
	errGroupNotFound   = notFound("Grupo não encontrado")
	errInviteNotFound  = notFound("Convite inválido")
	errGameNotFound    = notFound("Jogo não encontrado na votação")
	errCommentNotFound = notFound("Comentário não encontrado")
	errUserNotFound    = notFound("Usuário não encontrado")

	errAnonymous   = forbidden("Conta anônima só pode ver. Crie uma conta para participar.")
	errNotMember   = forbidden("Você não é membro desse grupo")
	errNotManager  = forbidden("Só quem criou a gameplay ou o dono do grupo pode fazer isso")
	errNotCreator  = forbidden("Só quem criou a gameplay pode fazer isso")
	errSingleGames = forbidden("Na gameplay única, só quem criou mexe na lista de jogos")
	errNotYours    = forbidden("Você não pode apagar esse comentário")

	errEnded         = conflict("Essa gameplay já foi encerrada")
	errLocked        = conflict("A votação já tem voto: finalistas e voto múltiplo não mudam mais")
	errFrozen        = conflict("O desempate está aberto: a lista de jogos não muda mais")
	errFull          = conflict("A votação já tem 20 jogos")
	errRepeated      = conflict("Esse jogo já está na votação")
	errNotGoingVote  = conflict("Quem marcou Não vou não vota")
	errNoTie         = conflict("Não há empate para desempatar")
	errCreatorLeaves = conflict("Quem criou não sai da gameplay: encerre-a")
	errGroupLeave    = conflict("Para sair de uma gameplay do grupo, saia do grupo")

	errNoVoting    = httpx.BadRequest("Essa gameplay não tem votação")
	errNotFixed    = httpx.BadRequest("Só a gameplay de jogo definido tem jogo")
	errOwnComment  = httpx.BadRequest("Não dá para denunciar o próprio comentário")
	errBadBody     = httpx.BadRequest("Corpo da requisição inválido")
	errBadCursor   = httpx.BadRequest("O parâmetro before não é uma gameplay dessa lista")
	errBadComment  = httpx.BadRequest("O parâmetro before não é um comentário dessa gameplay")
	errBadStatus   = httpx.BadRequest("O parâmetro status aceita active ou ended")
	errBadLimit    = httpx.BadRequest("O parâmetro limit precisa ser um número")
	errSingleVote  = httpx.BadRequest("Vote em um jogo só")
	errVoteTwice   = httpx.BadRequest("Voto repetido")
	errNotOnBallot = httpx.BadRequest("Esse jogo não está na votação")
)

// fieldErrors acumula as falhas de validacao do corpo e responde como o
// "Validation failed" da v1: 400 com a lista em fieldErrors.
type fieldErrors struct{ list []httpx.FieldError }

func (f *fieldErrors) add(field, msg string) {
	f.list = append(f.list, httpx.FieldError{Field: field, Message: &msg})
}

func (f *fieldErrors) any() bool { return len(f.list) > 0 }

func (f *fieldErrors) write(w http.ResponseWriter, r *http.Request) {
	msg := "Dados inválidos"
	body := httpx.NewError(http.StatusBadRequest, &msg, httpx.RequestPath(r))
	body.FieldErrors = f.list
	httpx.WriteErrorResponse(w, http.StatusBadRequest, body)
}
