package games

import (
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

// Handler: GET /games/search e GET /games/{id}, ambos com JWT -- porta do
// GameController do Kotlin.
type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	mux.Handle("GET /games/search", requireAuth(http.HandlerFunc(h.search)))
	mux.Handle("GET /games/{id}", requireAuth(http.HandlerFunc(h.details)))
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Medido em producao: q ausente, vazio ou so com espacos -> 500
	// "Unexpected error". O Kotlin nao trata MissingServletRequestParameter nem a
	// violacao do @NotBlank, e o catch-all responde 500 (contract/ACHADOS.md
	// secao 18). Mantido identico ate o Kotlin ser corrigido.
	values, ok := query["q"]
	if !ok {
		unexpected(w, r)
		return
	}
	// varios q viram um so, separados por virgula (medido: q=mario&q=kart
	// busca "mario,kart")
	q := strings.Join(values, ",")
	if javaTrim(q) == "" {
		unexpected(w, r)
		return
	}

	page := 1
	if pv, ok := query["page"]; ok && !(len(pv) == 1 && pv[0] == "") {
		p, err := springInt(pv[0]) // varios page: o Spring usa o primeiro
		if err != nil {
			unexpected(w, r)
			return
		}
		page = p
	}

	resp, err := h.svc.Search(r.Context(), q, page)
	switch {
	case errors.Is(err, ErrSearchUnavailable):
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "Catálogo de jogos temporariamente indisponível.")
	case err != nil:
		slog.Error("busca de jogos", "err", err)
		unexpected(w, r)
	default:
		httpx.WriteJSON(w, http.StatusOK, resp)
	}
}

func (h *Handler) details(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id") // ja decodificado, como o @PathVariable do Spring
	d, err := h.svc.Details(r.Context(), id)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "Jogo "+id+" não encontrado")
	case err != nil:
		slog.Error("detalhe de jogo", "id", id, "err", err)
		unexpected(w, r)
	default:
		httpx.WriteJSON(w, http.StatusOK, d)
	}
}

// unexpected e a resposta do catch-all do ApiExceptionHandler.
func unexpected(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, r, http.StatusInternalServerError, "Unexpected error")
}

// javaTrim: String.trim() do Java, que o @NotBlank usa -- remove so os
// caracteres <= U+0020, nao o espaco Unicode em geral.
func javaTrim(s string) string {
	return strings.TrimFunc(s, func(r rune) bool { return r <= ' ' })
}

// springInt reproduz a conversao de @RequestParam Int do Spring
// (NumberUtils.parseNumber): remove TODO espaco em branco, inclusive no meio;
// aceita sinal; e "0x", "0X" ou "#" fazem ler em hexadecimal. Medido em
// producao: page=0x2 e page=%202%20 chegam como 2.
func springInt(raw string) (int, error) {
	s := strings.Map(func(r rune) rune {
		if javaIsWhitespace(r) {
			return -1
		}
		return r
	}, raw)

	body, neg := s, false
	if strings.HasPrefix(body, "-") {
		body, neg = body[1:], true
	}
	for _, prefix := range []string{"0x", "0X", "#"} {
		if strings.HasPrefix(body, prefix) { // Integer.decode
			digits := body[len(prefix):]
			if digits == "" || digits[0] == '-' || digits[0] == '+' {
				return 0, strconv.ErrSyntax
			}
			v, err := strconv.ParseInt(digits, 16, 64)
			if err != nil {
				return 0, err
			}
			if neg {
				v = -v
			}
			if v < math.MinInt32 || v > math.MaxInt32 {
				return 0, strconv.ErrRange
			}
			return int(v), nil
		}
	}
	v, err := strconv.ParseInt(s, 10, 32) // Integer.valueOf: aceita sinal
	return int(v), err
}

// javaIsWhitespace: Character.isWhitespace -- espacos Unicode, exceto os
// "nao separaveis" (U+00A0, U+2007, U+202F), mais os controles de separacao.
func javaIsWhitespace(r rune) bool {
	switch {
	case r == '\t', r == '\n', r == '\v', r == '\f', r == '\r', r >= 0x1C && r <= 0x1F:
		return true
	case r == 0xA0, r == 0x2007, r == 0x202F:
		return false
	}
	return unicode.In(r, unicode.Zs, unicode.Zl, unicode.Zp)
}
