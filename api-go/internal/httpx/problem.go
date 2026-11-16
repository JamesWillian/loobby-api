package httpx

import (
	"errors"
	"log/slog"
	"net/http"
)

// Error e um erro de regra de negocio com o status HTTP que o Kotlin devolve
// para ele. O ApiExceptionHandler de la mapeia por tipo de excecao:
//
//	IllegalArgumentException / IllegalStateException -> 400   (BadRequest)
//	AccessDeniedException                            -> 403   (Forbidden)
//	ResponseStatusException                          -> o status da excecao (Status)
//	qualquer outra                                   -> 500 "Unexpected error"
//
// Do lado Go a regra e a mesma, invertida: um *Error vira o envelope com o seu
// status; qualquer outro erro (banco, rede, bug) vira o 500 generico -- ver Fail.
type Error struct {
	Status  int
	Message *string // nil sai como "message":null, como ex.message nulo no Kotlin
}

func (e *Error) Error() string {
	if e.Message == nil {
		return http.StatusText(e.Status)
	}
	return *e.Message
}

func BadRequest(message string) *Error { return &Error{http.StatusBadRequest, &message} }
func Forbidden(message string) *Error  { return &Error{http.StatusForbidden, &message} }

// Status: o ResponseStatusException(status, reason) do Kotlin.
func Status(status int, message string) *Error { return &Error{status, &message} }

// Fail escreve a resposta de erro de um handler. E o ApiExceptionHandler do Go.
func Fail(w http.ResponseWriter, r *http.Request, err error) {
	var e *Error
	if errors.As(err, &e) {
		WriteErrorResponse(w, e.Status, NewError(e.Status, e.Message, RequestPath(r)))
		return
	}
	// O Kotlin nao loga o catch-all; aqui o log e o unico rastro do motivo real.
	slog.Error("erro inesperado", "metodo", r.Method, "rota", RequestPath(r), "err", err)
	Unexpected(w, r)
}

// Unexpected e a resposta do catch-all do ApiExceptionHandler.
func Unexpected(w http.ResponseWriter, r *http.Request) {
	WriteError(w, r, http.StatusInternalServerError, "Unexpected error")
}
