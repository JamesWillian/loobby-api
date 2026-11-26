package library

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/JamesWillian/loobby-api/api-go/internal/auth"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
	"github.com/JamesWillian/loobby-api/api-go/internal/platform"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register: todas as rotas exigem token (o auth.Filter cuida disso).
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v2/me/library", h.mine)
	mux.HandleFunc("PUT /v2/me/library/{gameId}", h.put)
	mux.HandleFunc("DELETE /v2/me/library/{gameId}", h.remove)
	mux.HandleFunc("GET /v2/groups/{groupId}/library", h.group)
	mux.HandleFunc("PUT /v2/groups/{groupId}/library/{gameId}", h.putGroup)
	mux.HandleFunc("DELETE /v2/groups/{groupId}/library/{gameId}", h.removeGroup)
}

func (h *Handler) mine(w http.ResponseWriter, r *http.Request) {
	userID, ok := me(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.Mine(r.Context(), userID)
	reply(w, r, resp, err)
}

// putBody: {"ownedPlatforms": [...]}. A lista e obrigatoria; vazia = so na
// biblioteca.
type putBody struct {
	OwnedPlatforms *[]string `json:"ownedPlatforms"`
}

func (h *Handler) put(w http.ResponseWriter, r *http.Request) {
	gameID, ok := pathGame(w, r)
	if !ok {
		return
	}
	var body putBody
	if !readJSON(w, r, &body) {
		return
	}
	owned, msg := checkOwned(body.OwnedPlatforms)
	if msg != "" {
		fieldError(w, r, "ownedPlatforms", msg)
		return
	}
	userID, ok := me(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.Put(r.Context(), userID, gameID, owned)
	reply(w, r, resp, err)
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request) {
	gameID, ok := pathGame(w, r)
	if !ok {
		return
	}
	userID, ok := me(w, r)
	if !ok {
		return
	}
	done(w, r, h.svc.Remove(r.Context(), userID, gameID))
}

func (h *Handler) group(w http.ResponseWriter, r *http.Request) {
	groupID, ok := pathGroup(w, r)
	if !ok {
		return
	}
	userID, ok := me(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.Group(r.Context(), userID, groupID)
	reply(w, r, resp, err)
}

func (h *Handler) putGroup(w http.ResponseWriter, r *http.Request) {
	groupID, ok := pathGroup(w, r)
	if !ok {
		return
	}
	gameID, ok := pathGame(w, r)
	if !ok {
		return
	}
	userID, ok := me(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.PutGroup(r.Context(), userID, groupID, gameID)
	reply(w, r, resp, err)
}

func (h *Handler) removeGroup(w http.ResponseWriter, r *http.Request) {
	groupID, ok := pathGroup(w, r)
	if !ok {
		return
	}
	gameID, ok := pathGame(w, r)
	if !ok {
		return
	}
	userID, ok := me(w, r)
	if !ok {
		return
	}
	done(w, r, h.svc.RemoveGroup(r.Context(), userID, groupID, gameID))
}

// checkOwned valida o "tenho o jogo": so as 5 plataformas, sem repetir. Devolve
// a lista na ordem de platform.All, ou a mensagem do fieldError.
func checkOwned(list *[]string) ([]string, string) {
	if list == nil {
		return nil, "Informe as plataformas em que você tem o jogo (lista vazia: só na biblioteca)"
	}
	seen := map[string]bool{}
	for _, p := range *list {
		if !platform.Valid(p) {
			return nil, platform.Invalid
		}
		if seen[p] {
			return nil, "Plataforma repetida"
		}
		seen[p] = true
	}
	out := []string{}
	for _, p := range platform.All {
		if seen[p] {
			out = append(out, p)
		}
	}
	return out, ""
}

// pathGame le o {gameId}: o id do RAWG, so digitos. O resto nao existe no
// catalogo, e nem chega a ser procurado: 404.
func pathGame(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("gameId")
	valid := id != "" && len(id) <= maxGameID
	for _, c := range id {
		if c < '0' || c > '9' {
			valid = false
		}
	}
	if !valid {
		httpx.Fail(w, r, errGameNotFound)
	}
	return id, valid
}

// pathGroup le o {groupId}. O que nao e UUID nao existe: 404.
func pathGroup(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, ok := httpx.PathUUID(r.PathValue("groupId"))
	if !ok {
		httpx.Fail(w, r, errGroupNotFound)
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

// readJSON le o corpo em dst, como nas gameplays: JSON, ate maxBody. Falhou, ja
// respondeu 400 e devolve false.
func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	mt, _, mtErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	isJSON := mtErr == nil && (mt == "application/json" ||
		(strings.HasPrefix(mt, "application/") && strings.HasSuffix(mt, "+json")))
	if err != nil || len(raw) > maxBody || !isJSON || len(bytes.TrimSpace(raw)) == 0 || json.Unmarshal(raw, dst) != nil {
		httpx.Fail(w, r, errBadBody)
		return false
	}
	return true
}

func reply[T any](w http.ResponseWriter, r *http.Request, v T, err error) {
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, v)
}

func done(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
