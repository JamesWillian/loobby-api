package notifications

import (
	"net/http"

	"github.com/JamesWillian/loobby-api/api-go/internal/auth"
	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

// Handler: DeviceTokenController -- o app registra e remove o token de push do aparelho.
type Handler struct{ q *sqlc.Queries }

func NewHandler(dbtx sqlc.DBTX) *Handler { return &Handler{q: sqlc.New(dbtx)} }

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /devices/register", h.register)
	mux.HandleFunc("DELETE /devices/{token}", h.unregister)
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.ReadObject(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	token := body.String("token")
	httpx.Req(body, token)
	platform := httpx.Req(body, body.Enum("platform", "ANDROID", "IOS"))
	if err := body.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v := httpx.NewViolations(r)
	v.NotBlank("token", token)
	if v.Any() {
		v.Write(w, r)
		return
	}
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Um token e de um aparelho: registrar de novo passa o aparelho para o
	// usuario que esta logado nele agora.
	err = h.q.UpsertDeviceToken(r.Context(), sqlc.UpsertDeviceTokenParams{
		Token: *token, UserID: userID, Platform: platform,
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// unregister apaga o token, exista ou nao, e nao confere de quem ele e --
// igual ao Kotlin (contract/ACHADOS.md).
func (h *Handler) unregister(w http.ResponseWriter, r *http.Request) {
	if err := h.q.DeleteDeviceToken(r.Context(), r.PathValue("token")); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
