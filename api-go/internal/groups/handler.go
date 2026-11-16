package groups

import (
	"net/http"
	"strings"

	"github.com/JamesWillian/loobby-api/api-go/internal/auth"
	"github.com/JamesWillian/loobby-api/api-go/internal/files"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /groups", h.create)
	mux.HandleFunc("GET /groups", h.list)
	mux.HandleFunc("GET /groups/{id}", h.get)
	mux.HandleFunc("PATCH /groups/{groupId}", h.rename)
	mux.HandleFunc("POST /groups/{groupId}/image", h.uploadImage)
	mux.HandleFunc("DELETE /groups/{groupId}", h.delete)
	mux.HandleFunc("POST /groups/{groupId}/members", h.join)
	mux.HandleFunc("DELETE /groups/{groupId}/members/me", h.leave)
	mux.HandleFunc("GET /groups/{groupId}/members", h.members)
	mux.HandleFunc("DELETE /groups/{groupId}/members/{memberId}", h.removeMember)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.ReadObject(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name := body.String("name")
	imageURL := body.String("imageUrl")
	httpx.Req(body, name) // String nao-nula: ausente e 500, nao erro de validacao
	if err := body.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v := httpx.NewViolations(r)
	v.NotBlank("name", name)
	if v.Any() {
		v.Write(w, r)
		return
	}
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.Create(r.Context(), userID, *name, imageURL)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.ListForUser(r.Context(), userID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r, "id")
	if !ok {
		return
	}
	resp, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// ServeInvite atende GET /groups/invite/{code}. Nao e registrada no mux: ela
// se sobrepoe a /groups/{groupId}/members e /groups/{groupId}/events, e quem
// decide a precedencia (igual a do Spring) e o server.
func (h *Handler) ServeInvite(w http.ResponseWriter, r *http.Request, code string) {
	resp, err := h.svc.GetByCode(r.Context(), strings.ToUpper(code))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) rename(w http.ResponseWriter, r *http.Request) {
	groupID, ok := httpx.PathID(w, r, "groupId")
	if !ok {
		return
	}
	body, err := httpx.ReadObject(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name := body.String("name")
	httpx.Req(body, name)
	if err := body.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v := httpx.NewViolations(r)
	v.NotBlank("name", name)
	v.Size("name", name, 1, 100)
	if v.Any() {
		v.Write(w, r)
		return
	}
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.Rename(r.Context(), userID, groupID, *name)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) uploadImage(w http.ResponseWriter, r *http.Request) {
	groupID, ok := httpx.PathID(w, r, "groupId")
	if !ok {
		return
	}
	upload, err := files.ReadUpload(w, r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.UpdateImage(r.Context(), userID, groupID, upload)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	groupID, ok := httpx.PathID(w, r, "groupId")
	if !ok {
		return
	}
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.Delete(r.Context(), userID, groupID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// join e leave respondem 200 com corpo vazio: os metodos do controller do
// Kotlin nao devolvem nada e nao tem @ResponseStatus (golden membros-entrar-*).
func (h *Handler) join(w http.ResponseWriter, r *http.Request) {
	groupID, ok := httpx.PathID(w, r, "groupId")
	if !ok {
		return
	}
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.Join(r.Context(), userID, groupID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) leave(w http.ResponseWriter, r *http.Request) {
	groupID, ok := httpx.PathID(w, r, "groupId")
	if !ok {
		return
	}
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.Leave(r.Context(), userID, groupID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) members(w http.ResponseWriter, r *http.Request) {
	groupID, ok := httpx.PathID(w, r, "groupId")
	if !ok {
		return
	}
	resp, err := h.svc.Members(r.Context(), groupID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	groupID, ok := httpx.PathID(w, r, "groupId")
	if !ok {
		return
	}
	memberID, ok := httpx.PathID(w, r, "memberId")
	if !ok {
		return
	}
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.RemoveMember(r.Context(), userID, groupID, memberID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
