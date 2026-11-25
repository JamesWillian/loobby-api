package gameplays

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/JamesWillian/loobby-api/api-go/internal/auth"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register: todas as rotas exigem token (o auth.Filter cuida disso). Nenhuma
// se sobrepoe a outra no ServeMux: o convite fica em /v2/invites, e nao em
// /v2/gameplays/invite, que disputaria com /v2/gameplays/{id}/...
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v2/gameplays", h.createSingle)
	mux.HandleFunc("POST /v2/groups/{groupId}/gameplays", h.createInGroup)
	mux.HandleFunc("GET /v2/groups/{groupId}/gameplays", h.listGroup)
	mux.HandleFunc("GET /v2/me/gameplays", h.lobby)
	mux.HandleFunc("GET /v2/gameplays/{id}", h.get)
	mux.HandleFunc("PATCH /v2/gameplays/{id}", h.patch)
	mux.HandleFunc("POST /v2/gameplays/{id}/end", h.end)
	mux.HandleFunc("GET /v2/invites/{code}", h.preview)
	mux.HandleFunc("POST /v2/invites/{code}", h.join)
	mux.HandleFunc("PUT /v2/gameplays/{id}/participants/me", h.respond)
	mux.HandleFunc("DELETE /v2/gameplays/{id}/participants/me", h.leave)
	mux.HandleFunc("POST /v2/gameplays/{id}/games", h.addGame)
	mux.HandleFunc("DELETE /v2/gameplays/{id}/games/{gameId}", h.removeGame)
	mux.HandleFunc("PUT /v2/gameplays/{id}/votes/me", h.vote)
	mux.HandleFunc("POST /v2/gameplays/{id}/tiebreak", h.tiebreak)
	mux.HandleFunc("GET /v2/gameplays/{id}/comments", h.comments)
	mux.HandleFunc("POST /v2/gameplays/{id}/comments", h.comment)
	mux.HandleFunc("DELETE /v2/gameplays/{id}/comments/{commentId}", h.deleteComment)
	mux.HandleFunc("POST /v2/gameplays/{id}/comments/{commentId}/report", h.report)
}

// pathID le um {id} do caminho. O que nao e UUID nao existe: 404.
func pathID(w http.ResponseWriter, r *http.Request, name string, missing error) (string, bool) {
	id, ok := httpx.PathUUID(r.PathValue(name))
	if !ok {
		httpx.Fail(w, r, missing)
	}
	return id, ok
}

func me(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return "", false
	}
	return id, true
}

// reply escreve a resposta, ou o erro.
func reply[T any](w http.ResponseWriter, r *http.Request, status int, v T, err error) {
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, status, v)
}

func done(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// limit: o parametro ?limit=, dentro de [1, upper].
func limit(r *http.Request, def, upper int) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, errBadLimit
	}
	return min(max(n, 1), upper), nil
}

func (h *Handler) createSingle(w http.ResponseWriter, r *http.Request) { h.create(w, r, nil) }

func (h *Handler) createInGroup(w http.ResponseWriter, r *http.Request) {
	groupID, ok := pathID(w, r, "groupId", errGroupNotFound)
	if !ok {
		return
	}
	h.create(w, r, &groupID)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request, groupID *string) {
	var body createBody
	if !readJSON(w, r, &body, false) {
		return
	}
	in, errs := body.validate(h.svc.now())
	if errs.any() {
		errs.write(w, r)
		return
	}
	userID, ok := me(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.Create(r.Context(), userID, groupID, in)
	reply(w, r, http.StatusCreated, resp, err)
}

func (h *Handler) listGroup(w http.ResponseWriter, r *http.Request) {
	groupID, ok := pathID(w, r, "groupId", errGroupNotFound)
	if !ok {
		return
	}
	query := r.URL.Query()
	var ended bool
	switch query.Get("status") {
	case "", "active":
	case "ended":
		ended = true
	default:
		httpx.Fail(w, r, errBadStatus)
		return
	}
	var before *string
	if raw := query.Get("before"); raw != "" {
		id, ok := httpx.PathUUID(raw)
		if !ok {
			httpx.Fail(w, r, errBadCursor)
			return
		}
		before = &id
	}
	n, err := limit(r, defaultPage, maxPage)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	userID, ok := me(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.ListGroup(r.Context(), userID, groupID, ended, before, n)
	reply(w, r, http.StatusOK, resp, err)
}

func (h *Handler) lobby(w http.ResponseWriter, r *http.Request) {
	userID, ok := me(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.Lobby(r.Context(), userID)
	reply(w, r, http.StatusOK, resp, err)
}

// withGameplay le o {id} e o usuario, os dois comecos de quase toda rota.
func withGameplay(w http.ResponseWriter, r *http.Request) (id, userID string, ok bool) {
	if id, ok = pathID(w, r, "id", errNotFound); !ok {
		return "", "", false
	}
	if userID, ok = me(w, r); !ok {
		return "", "", false
	}
	return id, userID, true
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, userID, ok := withGameplay(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.Get(r.Context(), id, userID)
	reply(w, r, http.StatusOK, resp, err)
}

func (h *Handler) patch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", errNotFound)
	if !ok {
		return
	}
	var body patchBody
	if !readJSON(w, r, &body, false) {
		return
	}
	in, errs := body.validate(h.svc.now())
	if errs.any() {
		errs.write(w, r)
		return
	}
	userID, ok := me(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.Patch(r.Context(), id, userID, in)
	reply(w, r, http.StatusOK, resp, err)
}

func (h *Handler) end(w http.ResponseWriter, r *http.Request) {
	id, userID, ok := withGameplay(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.End(r.Context(), id, userID)
	reply(w, r, http.StatusOK, resp, err)
}

func (h *Handler) preview(w http.ResponseWriter, r *http.Request) {
	userID, ok := me(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.Preview(r.Context(), r.PathValue("code"), userID)
	reply(w, r, http.StatusOK, resp, err)
}

func (h *Handler) join(w http.ResponseWriter, r *http.Request) {
	userID, ok := me(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.Join(r.Context(), r.PathValue("code"), userID)
	reply(w, r, http.StatusOK, resp, err)
}

func (h *Handler) respond(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", errNotFound)
	if !ok {
		return
	}
	var body struct {
		Status *string `json:"status"`
	}
	if !readJSON(w, r, &body, false) {
		return
	}
	if body.Status == nil || !slices.Contains(statuses, *body.Status) {
		f := &fieldErrors{}
		f.add("status", "Status inválido: use GOING, NOT_GOING ou PENDING")
		f.write(w, r)
		return
	}
	userID, ok := me(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.Respond(r.Context(), id, userID, *body.Status)
	reply(w, r, http.StatusOK, resp, err)
}

func (h *Handler) leave(w http.ResponseWriter, r *http.Request) {
	id, userID, ok := withGameplay(w, r)
	if !ok {
		return
	}
	done(w, r, h.svc.Leave(r.Context(), id, userID))
}

func (h *Handler) addGame(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", errNotFound)
	if !ok {
		return
	}
	var body gameBody
	if !readJSON(w, r, &body, false) {
		return
	}
	f := &fieldErrors{}
	in := checkGame(f, "game", body)
	if f.any() {
		f.write(w, r)
		return
	}
	userID, ok := me(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.AddGame(r.Context(), id, userID, in)
	reply(w, r, http.StatusOK, resp, err)
}

func (h *Handler) removeGame(w http.ResponseWriter, r *http.Request) {
	id, userID, ok := withGameplay(w, r)
	if !ok {
		return
	}
	gameID, ok := pathID(w, r, "gameId", errGameNotFound)
	if !ok {
		return
	}
	resp, err := h.svc.RemoveGame(r.Context(), id, userID, gameID)
	reply(w, r, http.StatusOK, resp, err)
}

func (h *Handler) vote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", errNotFound)
	if !ok {
		return
	}
	var body struct {
		Votes *[]string `json:"votes"`
	}
	if !readJSON(w, r, &body, false) {
		return
	}
	if body.Votes == nil {
		f := &fieldErrors{}
		f.add("votes", "Informe os votos: a lista dos jogos escolhidos, ou vazia para tirar o voto")
		f.write(w, r)
		return
	}
	choice := make([]string, 0, len(*body.Votes))
	for _, raw := range *body.Votes {
		gameID, ok := httpx.PathUUID(raw)
		if !ok {
			httpx.Fail(w, r, errNotOnBallot)
			return
		}
		choice = append(choice, gameID)
	}
	userID, ok := me(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.Vote(r.Context(), id, userID, choice)
	reply(w, r, http.StatusOK, resp, err)
}

func (h *Handler) tiebreak(w http.ResponseWriter, r *http.Request) {
	id, userID, ok := withGameplay(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.Tiebreak(r.Context(), id, userID)
	reply(w, r, http.StatusOK, resp, err)
}

func (h *Handler) comments(w http.ResponseWriter, r *http.Request) {
	id, userID, ok := withGameplay(w, r)
	if !ok {
		return
	}
	var before *string
	if raw := r.URL.Query().Get("before"); raw != "" {
		commentID, ok := httpx.PathUUID(raw)
		if !ok {
			httpx.Fail(w, r, errBadComment)
			return
		}
		before = &commentID
	}
	n, err := limit(r, defaultComments, maxCommentsPage)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.Comments(r.Context(), id, userID, before, n)
	reply(w, r, http.StatusOK, resp, err)
}

func (h *Handler) comment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", errNotFound)
	if !ok {
		return
	}
	var body struct {
		Body *string `json:"body"`
	}
	if !readJSON(w, r, &body, false) {
		return
	}
	f := &fieldErrors{}
	text := ""
	if body.Body != nil {
		text = strings.TrimSpace(*body.Body)
	}
	switch {
	case text == "":
		f.add("body", "Escreva o comentário")
	case runes(text) > maxComment:
		f.add("body", "Use até "+itoa(maxComment)+" caracteres")
	}
	if f.any() {
		f.write(w, r)
		return
	}
	userID, ok := me(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.AddComment(r.Context(), id, userID, text)
	reply(w, r, http.StatusCreated, resp, err)
}

func (h *Handler) deleteComment(w http.ResponseWriter, r *http.Request) {
	id, userID, ok := withGameplay(w, r)
	if !ok {
		return
	}
	commentID, ok := pathID(w, r, "commentId", errCommentNotFound)
	if !ok {
		return
	}
	done(w, r, h.svc.DeleteComment(r.Context(), id, userID, commentID))
}

func (h *Handler) report(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", errNotFound)
	if !ok {
		return
	}
	commentID, ok := pathID(w, r, "commentId", errCommentNotFound)
	if !ok {
		return
	}
	var body struct {
		Reason *string `json:"reason"`
	}
	if !readJSON(w, r, &body, true) {
		return
	}
	var reason *string
	if body.Reason != nil {
		if text := strings.TrimSpace(*body.Reason); text != "" {
			if runes(text) > maxReason {
				f := &fieldErrors{}
				f.add("reason", "Use até "+itoa(maxReason)+" caracteres")
				f.write(w, r)
				return
			}
			reason = &text
		}
	}
	userID, ok := me(w, r)
	if !ok {
		return
	}
	done(w, r, h.svc.Report(r.Context(), id, userID, commentID, reason))
}
