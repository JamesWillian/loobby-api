package httpx

import (
	"net/http"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/validation"
)

// Violations acumula as falhas de um @Valid e escreve a resposta que o
// ApiExceptionHandler do Kotlin escreve para MethodArgumentNotValidException:
// 400 "Validation failed" com a lista em fieldErrors.
//
// As mensagens padrao vem do Hibernate Validator e sao traduzidas pelo
// Accept-Language da requisicao (medido: pt-BR -> "não deve estar em branco").
// Mensagens escritas no proprio DTO (message = "...") saem como estao.
//
// Ordem: o Kotlin devolve as falhas em ordem ALEATORIA (elas saem de um
// HashSet; duas chamadas iguais deram ordens diferentes na medicao). Aqui saem
// na ordem em que o handler as verifica, que e a ordem de declaracao do DTO.
type Violations struct {
	bundle string
	errs   []FieldError
}

func NewViolations(r *http.Request) *Violations {
	return &Violations{bundle: validation.Bundle(r.Header.Get("Accept-Language"))}
}

func (v *Violations) add(field, message string) {
	v.errs = append(v.errs, FieldError{Field: field, Message: &message})
}

// NotBlank: null tambem falha, como no @NotBlank.
func (v *Violations) NotBlank(field string, value *string) {
	if value == nil || !validation.NotBlank(*value) {
		v.add(field, validation.Message(v.bundle, "NotBlank"))
	}
}

// Size com a mensagem padrao. null e valido. O tamanho e contado em unidades
// UTF-16, como o String.length() do Java.
func (v *Violations) Size(field string, value *string, min, max int) {
	v.SizeMsg(field, value, min, max, validation.SizeMessage(v.bundle, min, max))
}

// SizeMsg: @Size com message proprio.
func (v *Violations) SizeMsg(field string, value *string, min, max int, message string) {
	if value == nil {
		return
	}
	if n := validation.Len(*value); n < min || n > max {
		v.add(field, message)
	}
}

// Email: null e "" sao validos (quem barra o vazio e o @NotBlank).
func (v *Violations) Email(field string, value *string) {
	if value != nil && !validation.IsEmail(*value) {
		v.add(field, validation.Message(v.bundle, "Email"))
	}
}

// FutureOrPresent: null e valido.
func (v *Violations) FutureOrPresent(field string, t *time.Time) {
	if t != nil && t.Before(time.Now()) {
		v.add(field, validation.Message(v.bundle, "FutureOrPresent"))
	}
}

func (v *Violations) Any() bool { return len(v.errs) > 0 }

func (v *Violations) Write(w http.ResponseWriter, r *http.Request) {
	msg := "Validation failed"
	body := NewError(http.StatusBadRequest, &msg, RequestPath(r))
	body.FieldErrors = v.errs
	WriteErrorResponse(w, http.StatusBadRequest, body)
}
