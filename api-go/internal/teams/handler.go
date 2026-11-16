package teams

import (
	"net/http"

	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /events/{eventId}/teams", h.create)
	mux.HandleFunc("GET /events/{eventId}/teams", h.list)
	mux.HandleFunc("PUT /events/{eventId}/teams/{teamId}", h.update)
	mux.HandleFunc("DELETE /events/{eventId}/teams/{teamId}", h.delete)
	mux.HandleFunc("POST /events/{eventId}/teams/{teamId}/players", h.addPlayer)
	mux.HandleFunc("PUT /events/{eventId}/teams/{teamId}/players/{userId}", h.updatePlayer)
	mux.HandleFunc("DELETE /events/{eventId}/teams/{teamId}/players/{userId}", h.removePlayer)
	mux.HandleFunc("POST /events/{eventId}/teams/auto-generate", h.autoGenerate)
}

// pathIDs le varios UUIDs do caminho; se algum nao for UUID, ja respondeu 500.
func pathIDs(w http.ResponseWriter, r *http.Request, names ...string) ([]string, bool) {
	out := make([]string, 0, len(names))
	for _, name := range names {
		id, ok := httpx.PathID(w, r, name)
		if !ok {
			return nil, false
		}
		out = append(out, id)
	}
	return out, true
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	path, ok := pathIDs(w, r, "eventId")
	if !ok {
		return
	}
	body, err := httpx.ReadObject(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name := body.String("name")
	req := createRequest{Name: httpx.Req(body, name), Color: body.String("color"), Order: body.Int("order")}
	// players tem default (lista vazia): ausente e aceito, null explicito nao.
	players, _ := body.Objects("players")
	for _, p := range players {
		req.Players = append(req.Players, playerRequest{UserID: httpx.Req(p, p.UUID("userId")), Role: p.String("role")})
	}
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
	resp, err := h.svc.Create(r.Context(), path[0], req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	path, ok := pathIDs(w, r, "eventId")
	if !ok {
		return
	}
	resp, err := h.svc.List(r.Context(), path[0])
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	path, ok := pathIDs(w, r, "eventId", "teamId")
	if !ok {
		return
	}
	body, err := httpx.ReadObject(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	req := updateRequest{Name: body.String("name"), Color: body.String("color"), Order: body.Int("order")}
	if err := body.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.Update(r.Context(), path[0], path[1], req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// delete responde 200 com corpo vazio (e nao 204): o metodo do Kotlin nao tem
// @ResponseStatus (golden teams-delete).
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	path, ok := pathIDs(w, r, "eventId", "teamId")
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), path[0], path[1]); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) addPlayer(w http.ResponseWriter, r *http.Request) {
	path, ok := pathIDs(w, r, "eventId", "teamId")
	if !ok {
		return
	}
	body, err := httpx.ReadObject(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	req := playerRequest{UserID: httpx.Req(body, body.UUID("userId")), Role: body.String("role")}
	if err := body.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.AddPlayer(r.Context(), path[0], path[1], req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) updatePlayer(w http.ResponseWriter, r *http.Request) {
	path, ok := pathIDs(w, r, "eventId", "teamId", "userId")
	if !ok {
		return
	}
	body, err := httpx.ReadObject(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	newTeamID, role := body.UUID("newTeamId"), body.String("role")
	if err := body.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.UpdatePlayer(r.Context(), path[0], path[1], path[2], newTeamID, role)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) removePlayer(w http.ResponseWriter, r *http.Request) {
	path, ok := pathIDs(w, r, "eventId", "teamId", "userId")
	if !ok {
		return
	}
	resp, err := h.svc.RemovePlayer(r.Context(), path[0], path[1], path[2])
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) autoGenerate(w http.ResponseWriter, r *http.Request) {
	path, ok := pathIDs(w, r, "eventId")
	if !ok {
		return
	}
	body, err := httpx.ReadObject(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	req := autoGenerateRequest{TeamSize: body.Int("teamSize"), TeamCount: body.Int("teamCount")}
	// includeReserves e Boolean primitivo com default true: ausente vale true,
	// mas null explicito vale false (o zero do primitivo).
	if !body.Has("includeReserves") {
		req.IncludeReserves = true
	} else if v := body.Bool("includeReserves"); v != nil {
		req.IncludeReserves = *v
	}
	if err := body.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.AutoGenerate(r.Context(), path[0], req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}
