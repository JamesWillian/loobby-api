package events

import (
	"net/http"
	"strings"

	"github.com/JamesWillian/loobby-api/api-go/internal/auth"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /groups/{groupId}/events", h.createInGroup)
	mux.HandleFunc("POST /events/instant", h.createInstant)
	mux.HandleFunc("GET /groups/{groupId}/events", h.listByGroup)
	mux.HandleFunc("GET /events/{eventId}", h.get)
	mux.HandleFunc("PUT /events/{eventId}", h.update)
	mux.HandleFunc("DELETE /events/{eventId}", h.delete)
	mux.HandleFunc("PUT /events/{eventId}/rsvps", h.upsertRsvp)
	mux.HandleFunc("GET /events/{eventId}/rsvps", h.listRsvps)
	mux.HandleFunc("GET /events/{eventId}/rsvps/me", h.myRsvp)
	mux.HandleFunc("DELETE /events/{eventId}/rsvps/me", h.deleteMyRsvp)
}

// readGameplay e readSport leem os detalhes aninhados. Eles sao lidos SEMPRE,
// seja qual for o tipo do evento: um "gameplay": {} sem gameName invalida o
// corpo (500) mesmo num evento PARTY, porque no Kotlin a leitura do JSON vem
// antes de qualquer regra.
func readGameplay(body *httpx.Object) *gameplayRequest {
	o := body.Object("gameplay")
	if o == nil {
		return nil
	}
	return &gameplayRequest{GameID: o.String("gameId"), GameName: httpx.Req(o, o.String("gameName"))}
}

func readSport(body *httpx.Object) *sportRequest {
	o := body.Object("sport")
	if o == nil {
		return nil
	}
	req := &sportRequest{
		Arena: o.String("arena"), PricePerPlayer: o.Decimal("pricePerPlayer"),
		MaxPlayers: o.Int("maxPlayers"), AcceptReserve: o.Bool("acceptReserve"),
	}
	// durationMinutes e Int primitivo: ausente ou null vale 0 (e nao 500), e ai
	// quem recusa e a regra "durationMinutes must be > 0". Medido.
	if d := o.Int("durationMinutes"); d != nil {
		req.DurationMinutes = *d
	}
	return req
}

func (h *Handler) createInGroup(w http.ResponseWriter, r *http.Request) {
	groupID, ok := httpx.PathID(w, r, "groupId")
	if !ok {
		return
	}
	h.create(w, r, &groupID)
}

func (h *Handler) createInstant(w http.ResponseWriter, r *http.Request) {
	h.create(w, r, nil)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request, groupID *string) {
	body, err := httpx.ReadObject(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name := body.String("name")
	scheduled := body.Instant("scheduledDatetime")
	req := createRequest{
		EventType:   httpx.Req(body, body.Enum("eventType", eventTypes...)),
		Name:        httpx.Req(body, name),
		Description: body.String("description"),
		Gameplay:    readGameplay(body),
		Sport:       readSport(body),
	}
	req.ScheduledDatetime = httpx.Req(body, scheduled)
	if err := body.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v := httpx.NewViolations(r)
	v.NotBlank("name", name)
	v.FutureOrPresent("scheduledDatetime", scheduled)
	if v.Any() {
		v.Write(w, r)
		return
	}
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.Create(r.Context(), userID, groupID, req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) listByGroup(w http.ResponseWriter, r *http.Request) {
	groupID, ok := httpx.PathID(w, r, "groupId")
	if !ok {
		return
	}
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.ListByGroup(r.Context(), groupID, userID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	eventID, ok := httpx.PathID(w, r, "eventId")
	if !ok {
		return
	}
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.Get(r.Context(), eventID, userID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// ServeInvite atende GET /events/invite/{code}. Fica fora do mux pelo mesmo
// motivo de groups.Handler.ServeInvite: sobreposicao com /events/{eventId}/rsvps
// e /events/{eventId}/teams.
func (h *Handler) ServeInvite(w http.ResponseWriter, r *http.Request, code string) {
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.GetByInviteCode(r.Context(), strings.ToUpper(code), userID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	eventID, ok := httpx.PathID(w, r, "eventId")
	if !ok {
		return
	}
	body, err := httpx.ReadObject(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	req := updateRequest{
		Name:              body.String("name"),
		Description:       body.String("description"),
		ScheduledDatetime: body.Instant("scheduledDatetime"),
		Gameplay:          readGameplay(body),
		Sport:             readSport(body),
	}
	// clearDescription e Boolean primitivo com default false: ausente e false,
	// e null explicito tambem.
	if clear := body.Bool("clearDescription"); clear != nil {
		req.ClearDescription = *clear
	}
	if err := body.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v := httpx.NewViolations(r)
	v.Size("name", req.Name, 1, 255)
	v.FutureOrPresent("scheduledDatetime", req.ScheduledDatetime)
	if v.Any() {
		v.Write(w, r)
		return
	}
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.Update(r.Context(), userID, eventID, req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	eventID, ok := httpx.PathID(w, r, "eventId")
	if !ok {
		return
	}
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.Delete(r.Context(), userID, eventID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) upsertRsvp(w http.ResponseWriter, r *http.Request) {
	eventID, ok := httpx.PathID(w, r, "eventId")
	if !ok {
		return
	}
	body, err := httpx.ReadObject(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	req := rsvpRequest{
		Status: httpx.Req(body, body.Enum("status", rsvpStatuses...)),
		IsPaid: body.Bool("isPaid"),
		Obs:    body.String("obs"),
	}
	if err := body.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.UpsertRsvp(r.Context(), userID, eventID, req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) listRsvps(w http.ResponseWriter, r *http.Request) {
	eventID, ok := httpx.PathID(w, r, "eventId")
	if !ok {
		return
	}
	resp, err := h.svc.ListRsvps(r.Context(), eventID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) myRsvp(w http.ResponseWriter, r *http.Request) {
	eventID, ok := httpx.PathID(w, r, "eventId")
	if !ok {
		return
	}
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, found, err := h.svc.MyRsvp(r.Context(), userID, eventID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !found {
		w.WriteHeader(http.StatusOK) // 200 sem corpo e sem Content-Type (medido)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) deleteMyRsvp(w http.ResponseWriter, r *http.Request) {
	eventID, ok := httpx.PathID(w, r, "eventId")
	if !ok {
		return
	}
	userID, err := auth.UserID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.DeleteMyRsvp(r.Context(), userID, eventID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
