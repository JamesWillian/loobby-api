package events

import (
	"context"

	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

var errUserNotFound = httpx.BadRequest("User not found")

func rsvpView(r sqlc.EventRsvp, u sqlc.User, e sqlc.Event, createdAt *httpx.Instant) rsvpResponse {
	return rsvpResponse{
		EventID: r.EventID, UserID: r.UserID, Status: r.Status, IsPaid: r.IsPaid, Obs: r.Obs,
		CreatedAt: createdAt, Username: u.Username, Displayname: &u.Displayname,
		AvatarURL: u.AvatarUrl, IsOwner: u.ID == e.OwnerID,
	}
}

func createdAt(r sqlc.EventRsvp) *httpx.Instant {
	t := httpx.Instant(r.CreatedAt)
	return &t
}

// UpsertRsvp: EventRsvpService.upsertRsvp.
//
//   - Em evento de grupo, so membro (ou o dono do evento) responde.
//   - RSVP novo: isPaid ausente vale false; a resposta sai com createdAt null,
//     porque o Kotlin devolve a entidade sem reler a coluna do banco.
//   - RSVP existente: status sempre muda; isPaid so se veio; obs e SEMPRE
//     sobrescrito -- nao mandar obs apaga a observacao anterior.
//   - Passar a YES (vindo de nada ou de outro status) avisa os demais confirmados.
func (s *Service) UpsertRsvp(ctx context.Context, userID, eventID string, req rsvpRequest) (rsvpResponse, error) {
	var (
		resp      rsvpResponse
		event     sqlc.Event
		confirmed bool
	)
	err := db.InTx(ctx, s.pool, func(q *sqlc.Queries) error {
		e, err := findEvent(ctx, q, eventID)
		if err != nil {
			return err
		}
		event = e
		if e.GroupID != nil {
			isMember, err := q.MemberExists(ctx, sqlc.MemberExistsParams{GroupID: *e.GroupID, UserID: userID})
			if err != nil {
				return err
			}
			if !isMember && e.OwnerID != userID {
				return httpx.BadRequest("User is not member of this group")
			}
		}

		key := sqlc.FindRsvpParams{EventID: eventID, UserID: userID}
		saved, err := q.FindRsvp(ctx, key)
		var created *httpx.Instant
		switch {
		case db.NotFound(err):
			saved = sqlc.EventRsvp{
				EventID: eventID, UserID: userID, Status: req.Status,
				IsPaid: req.IsPaid != nil && *req.IsPaid, Obs: req.Obs,
			}
			err = q.InsertRsvp(ctx, sqlc.InsertRsvpParams{
				EventID: eventID, UserID: userID, Status: saved.Status, IsPaid: saved.IsPaid, Obs: saved.Obs,
			})
			confirmed = req.Status == "YES"
		case err == nil:
			confirmed = req.Status == "YES" && saved.Status != "YES"
			created = createdAt(saved)
			saved.Status = req.Status
			if req.IsPaid != nil {
				saved.IsPaid = *req.IsPaid
			}
			saved.Obs = req.Obs
			err = q.UpdateRsvp(ctx, sqlc.UpdateRsvpParams{
				EventID: eventID, UserID: userID, Status: saved.Status, IsPaid: saved.IsPaid, Obs: saved.Obs,
			})
		}
		if err != nil {
			return err
		}

		u, err := q.FindUser(ctx, userID)
		if db.NotFound(err) {
			return errUserNotFound
		}
		if err != nil {
			return err
		}
		resp = rsvpView(saved, u, e, created)
		return nil
	})
	if err != nil {
		return rsvpResponse{}, err
	}
	if confirmed {
		s.notifier.OnRsvpConfirmed(event, userID)
	}
	return resp, nil
}

// ListRsvps: EventRsvpService.listRsvps, do mais antigo para o mais novo.
func (s *Service) ListRsvps(ctx context.Context, eventID string) ([]rsvpResponse, error) {
	e, err := findEvent(ctx, s.q, eventID)
	if err != nil {
		return nil, err
	}
	rsvps, err := s.q.FindRsvpsByEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	out := make([]rsvpResponse, 0, len(rsvps))
	if len(rsvps) == 0 {
		return out, nil
	}
	userIDs := make([]string, 0, len(rsvps))
	for _, r := range rsvps {
		userIDs = append(userIDs, r.UserID)
	}
	users, err := s.q.FindUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]sqlc.User, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}
	for _, r := range rsvps {
		if u, ok := byID[r.UserID]; ok {
			out = append(out, rsvpView(r, u, e, createdAt(r)))
		}
	}
	return out, nil
}

// MyRsvp: EventRsvpService.getMyRsvp. found=false quando o usuario ainda nao
// respondeu -- o Kotlin devolve null, que sai como 200 com corpo vazio.
func (s *Service) MyRsvp(ctx context.Context, userID, eventID string) (resp rsvpResponse, found bool, err error) {
	e, err := findEvent(ctx, s.q, eventID)
	if err != nil {
		return rsvpResponse{}, false, err
	}
	r, err := s.q.FindRsvp(ctx, sqlc.FindRsvpParams{EventID: eventID, UserID: userID})
	if db.NotFound(err) {
		return rsvpResponse{}, false, nil
	}
	if err != nil {
		return rsvpResponse{}, false, err
	}
	u, err := s.q.FindUser(ctx, userID)
	if db.NotFound(err) {
		return rsvpResponse{}, false, errUserNotFound
	}
	if err != nil {
		return rsvpResponse{}, false, err
	}
	return rsvpView(r, u, e, createdAt(r)), true, nil
}

// DeleteMyRsvp: EventRsvpService.deleteMyRsvp. Idempotente; so o evento
// inexistente e erro.
func (s *Service) DeleteMyRsvp(ctx context.Context, userID, eventID string) error {
	if _, err := findEvent(ctx, s.q, eventID); err != nil {
		return err
	}
	return s.q.DeleteRsvp(ctx, sqlc.DeleteRsvpParams{EventID: eventID, UserID: userID})
}
