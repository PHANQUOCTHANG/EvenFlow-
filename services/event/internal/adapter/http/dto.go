package http

import (
	"time"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/google/uuid"
)

type CreateEventRequest struct {
	OrganizerID string     `json:"organizer_id"`
	Slug        string     `json:"slug"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Venue       string     `json:"venue"`
	StartsAt    time.Time  `json:"starts_at"`
	SaleStartAt time.Time  `json:"sale_start_at"`
	SaleEndAt   *time.Time `json:"sale_end_at,omitempty"`
}

type UpdateEventRequest struct {
	OrganizerID string     `json:"organizer_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Venue       string     `json:"venue"`
	StartsAt    time.Time  `json:"starts_at"`
	SaleStartAt time.Time  `json:"sale_start_at"`
	SaleEndAt   *time.Time `json:"sale_end_at,omitempty"`
}

type AddTierRequest struct {
	OrganizerID string `json:"organizer_id"`
	Name        string `json:"name"`
	PriceCents  int64  `json:"price_cents"`
	Quota       int    `json:"quota"`
}

type UpdateTierQuotaRequest struct {
	OrganizerID string `json:"organizer_id"`
	NewQuota    int    `json:"new_quota"`
}

type EventResponse struct {
	ID             uuid.UUID           `json:"id"`
	OrganizerID    uuid.UUID           `json:"organizer_id"`
	Slug           string              `json:"slug"`
	Title          string              `json:"title"`
	Description    string              `json:"description"`
	Venue          string              `json:"venue"`
	StartsAt       time.Time           `json:"starts_at"`
	SaleStartAt    time.Time           `json:"sale_start_at"`
	SaleEndAt      *time.Time          `json:"sale_end_at,omitempty"`
	Status         domain.EventStatus  `json:"status"`
	MaxPerIdentity int                 `json:"max_per_identity"`
	MaxPerOrder    int                 `json:"max_per_order"`
	HoldTTLSeconds int                 `json:"hold_ttl_seconds"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
	Tiers          []domain.TicketTier `json:"tiers,omitempty"`
}

// EV-120: DTO cho internal AI verdict callback
type AiVerdictRequest struct {
	Score   float64            `json:"score"`
	Verdict domain.AiVerdict   `json:"verdict"`
	Reasons []string           `json:"reasons"`
	Axes    map[string]float64 `json:"axes,omitempty"`
}

// EV-121: DTO cho Moderator thao tac
type ModeratorApproveRequest struct {
	ModeratorID string `json:"moderator_id"`
	Note        string `json:"note"`
}

type ModeratorRejectRequest struct {
	ModeratorID string `json:"moderator_id"`
	Reason      string `json:"reason"`
}

type ModerationQueueItemResponse struct {
	EventID     uuid.UUID                `json:"event_id"`
	EventTitle  string                   `json:"event_title"`
	EventStatus domain.EventStatus       `json:"event_status"`
	Venue       string                   `json:"venue"`
	StartsAt    time.Time                `json:"starts_at"`
	SaleStartAt time.Time                `json:"sale_start_at"`
	Review      *domain.ModerationReview `json:"review,omitempty"`
}

func ToEventResponse(e *domain.Event) EventResponse {
	return EventResponse{
		ID:             e.ID,
		OrganizerID:    e.OrganizerID,
		Slug:           e.Slug,
		Title:          e.Title,
		Description:    e.Description,
		Venue:          e.Venue,
		StartsAt:       e.StartsAt,
		SaleStartAt:    e.SaleStartAt,
		SaleEndAt:      e.SaleEndAt,
		Status:         e.Status,
		MaxPerIdentity: e.MaxPerIdentity,
		MaxPerOrder:    e.MaxPerOrder,
		HoldTTLSeconds: e.HoldTTLSeconds,
		CreatedAt:      e.CreatedAt,
		UpdatedAt:      e.UpdatedAt,
		Tiers:          e.Tiers,
	}
}
