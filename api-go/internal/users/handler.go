package users

import (
	"net/http"

	"github.com/JamesWillian/loobby-api/api-go/internal/auth"
	"github.com/JamesWillian/loobby-api/api-go/internal/files"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

type Handler struct {
	svc     *Service
	storage *files.Storage
}

func NewHandler(svc *Service, storage *files.Storage) *Handler {
	return &Handler{svc: svc, storage: storage}
}

// Register: todas as rotas exigem token (o auth.Filter cuida disso).
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /users/me", h.me)
	mux.HandleFunc("PATCH /users/me", h.updateMe)
	mux.HandleFunc("POST /users/me/avatar", h.uploadAvatar)
	mux.HandleFunc("GET /users/feed", h.feed)
	mux.HandleFunc("PUT /users/me/password", h.changePassword)
	mux.HandleFunc("DELETE /users/me", h.deleteAccount)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.Me(r.Context(), userID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) updateMe(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.ReadObject(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	req := profileUpdate{
		Username:    body.String("username"),
		Displayname: body.String("displayname"),
		AvatarURL:   body.String("avatarUrl"),
	}
	if err := body.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v := httpx.NewViolations(r)
	v.SizeMsg("username", req.Username, 3, 30, "username must be between 3 and 30 characters")
	v.SizeMsg("displayname", req.Displayname, 0, 50, "displayname must be at most 50 characters")
	v.SizeMsg("avatarUrl", req.AvatarURL, 0, 255, "avatarUrl must be at most 255 characters")
	if v.Any() {
		v.Write(w, r)
		return
	}
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.UpdateProfile(r.Context(), userID, req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) uploadAvatar(w http.ResponseWriter, r *http.Request) {
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
	// O arquivo e gravado antes de conferir se o usuario existe, como no Kotlin.
	url, err := h.storage.Store(upload, "avatars", userID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.UpdateAvatar(r.Context(), userID, url)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) feed(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.Feed(r.Context(), userID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.ReadObject(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Os tres campos sao String nao-nula sem default: faltou um, 500.
	current := httpx.Req(body, body.String("currentPassword"))
	newPass := httpx.Req(body, body.String("newPassword"))
	confirm := httpx.Req(body, body.String("confirmPassword"))
	if err := body.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.ChangePassword(r.Context(), userID, current, newPass, confirm); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, message{"Senha alterada com sucesso."})
}

func (h *Handler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.ReadObject(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	pass := httpx.Req(body, body.String("password"))
	if err := body.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.DeleteAccount(r.Context(), userID, pass); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, message{"Conta excluída com sucesso."})
}
