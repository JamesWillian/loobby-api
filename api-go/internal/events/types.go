// Package events e a porta do EventController, do EventService e do
// EventRsvpService do Kotlin: eventos de grupo e instantaneos, com os detalhes
// por tipo (gameplay, esporte) e as confirmacoes de presenca.
package events

import (
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/db/sqlc"
	"github.com/JamesWillian/loobby-api/api-go/internal/httpx"
)

// Constantes dos enums, na ordem de declaracao do Kotlin -- a ordem importa,
// porque o Jackson aceita o indice no lugar do nome.
var (
	eventTypes   = []string{"GAMEPLAY", "SPORT", "PARTY"}
	rsvpStatuses = []string{"YES", "NO", "MAYBE", "RESERVE"}
)

// eventResponse: EventResponse. Ordem dos campos = ordem do JSON; sem omitempty.
type eventResponse struct {
	ID                string            `json:"id"`
	EventType         string            `json:"eventType"`
	GroupID           *string           `json:"groupId"`
	IsInstant         bool              `json:"isInstant"`
	OwnerID           string            `json:"ownerId"`
	ScheduledDatetime httpx.Instant     `json:"scheduledDatetime"`
	Name              string            `json:"name"`
	Description       *string           `json:"description"`
	InviteCode        string            `json:"inviteCode"`
	CreatedAt         *httpx.Instant    `json:"createdAt"`
	RsvpStatus        *string           `json:"rsvpStatus"`
	ConfirmedCount    int               `json:"confirmedCount"`
	ConfirmedAvatars  []*string         `json:"confirmedAvatars"` // nil = null; elementos podem ser null
	Gameplay          *gameplayResponse `json:"gameplay"`
	Sport             *sportResponse    `json:"sport"`
}

type gameplayResponse struct {
	GameID   *string `json:"gameId"`
	GameName string  `json:"gameName"`
}

type sportResponse struct {
	DurationMinutes int32        `json:"durationMinutes"`
	Arena           *string      `json:"arena"`
	PricePerPlayer  httpx.Number `json:"pricePerPlayer"`
	MaxPlayers      *int32       `json:"maxPlayers"`
	AcceptReserve   bool         `json:"acceptReserve"`
}

// rsvpResponse: EventRsvpResponse.
type rsvpResponse struct {
	EventID     string         `json:"eventId"`
	UserID      string         `json:"userId"`
	Status      string         `json:"status"`
	IsPaid      bool           `json:"isPaid"`
	Obs         *string        `json:"obs"`
	CreatedAt   *httpx.Instant `json:"createdAt"`
	Username    string         `json:"username"`
	Displayname *string        `json:"displayname"`
	AvatarURL   *string        `json:"avatarUrl"`
	IsOwner     bool           `json:"isOwner"`
}

// gameplayRequest: GameplayEventDetailsRequest.
type gameplayRequest struct {
	GameID   *string
	GameName string
}

// sportRequest: SportEventDetailsRequest. PricePerPlayer e o literal decimal
// como chegou (ver httpx.Object.Decimal).
type sportRequest struct {
	DurationMinutes int32
	Arena           *string
	PricePerPlayer  *string
	MaxPlayers      *int32
	AcceptReserve   *bool
}

// createRequest: CreateEventRequest.
type createRequest struct {
	EventType         string
	Name              string
	Description       *string
	ScheduledDatetime time.Time
	Gameplay          *gameplayRequest
	Sport             *sportRequest
}

// updateRequest: UpdateEventRequest. nil = nao enviado (nao altera).
type updateRequest struct {
	Name              *string
	Description       *string
	ScheduledDatetime *time.Time
	Gameplay          *gameplayRequest
	Sport             *sportRequest
	ClearDescription  bool
}

// rsvpRequest: UpsertRsvpRequest.
type rsvpRequest struct {
	Status string
	IsPaid *bool
	Obs    *string
}

// Notifier e o que o modulo de eventos pede ao de notificacoes.
type Notifier interface {
	OnRsvpConfirmed(event sqlc.Event, actorID string)
	OnGroupEventCreated(event sqlc.Event, creatorID string)
}

// GameCache e o que o modulo de eventos pede ao de jogos.
type GameCache interface {
	EnsureCached(id string)
}
