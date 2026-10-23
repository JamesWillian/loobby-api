package httpx

import (
	"net/http"
	"time"
)

// ErrorResponse e a porta do ApiErrorResponse do Kotlin. O formato foi congelado
// na baseline de contrato (contract/golden), e a ordem dos campos aqui e a mesma
// que o Jackson emite -- encoding/json respeita a ordem de declaracao.
//
// NENHUM campo leva `omitempty`, de proposito. O Jackson emite `null`; omitempty
// sumiria com a chave, e cliente que faz json["message"] quebra. O caso extremo
// esta em POST /auth/google com token malformado, onde a resposta inteira e
// {"...","message":null,...} -- ali o null e o contrato.
//
// FieldErrors e um slice nil por padrao, que o encoding/json serializa como
// `null` (e nao `[]`), batendo com o Kotlin.
type ErrorResponse struct {
	Timestamp   Instant      `json:"timestamp"`
	Status      int          `json:"status"`
	Error       string       `json:"error"`
	Message     *string      `json:"message"`
	Path        *string      `json:"path"`
	FieldErrors []FieldError `json:"fieldErrors"`
}

type FieldError struct {
	Field   string  `json:"field"`
	Message *string `json:"message"`
}

// NewError monta a resposta com a reason phrase que o Kotlin usa. O
// HttpStatus.reasonPhrase do Spring e o http.StatusText do Go coincidem nos
// status em uso ("Bad Request", "Not Found", "Internal Server Error", ...).
func NewError(status int, message *string, path string) ErrorResponse {
	return ErrorResponse{
		Timestamp: Instant(time.Now()),
		Status:    status,
		Error:     http.StatusText(status),
		Message:   message,
		Path:      &path,
	}
}

func WriteError(w http.ResponseWriter, r *http.Request, status int, message string) {
	var msg *string
	if message != "" {
		msg = &message
	}
	WriteErrorResponse(w, status, NewError(status, msg, r.URL.Path))
}

func WriteErrorResponse(w http.ResponseWriter, status int, body ErrorResponse) {
	WriteJSONAs(w, status, "application/json", body)
}

func WriteJSON(w http.ResponseWriter, status int, body any) {
	WriteJSONAs(w, status, "application/json", body)
}

// WriteJSONAs existe porque nem todo JSON do Kotlin e application/json: o
// /actuator/health sai como application/vnd.spring-boot.actuator.v3+json.
func WriteJSONAs(w http.ResponseWriter, status int, contentType string, body any) {
	if body == nil {
		w.WriteHeader(status)
		return
	}
	b, err := Marshal(body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(b)
}
