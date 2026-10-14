package domain

import (
	"time"

	"github.com/google/uuid"
)

type AiVerdict string

const (
	VerdictAutoApproved AiVerdict = "AUTO_APPROVED"
	VerdictNeedsHuman   AiVerdict = "NEEDS_HUMAN"
	VerdictAutoRejected AiVerdict = "AUTO_REJECTED"
)

// ModerationReview dai dien cho ket qua kiem duyet AI va quyet dinh cua Moderator (EV-120, EV-121).
type ModerationReview struct {
	ID           uuid.UUID  `json:"id"`
	EventID      uuid.UUID  `json:"event_id"`
	AiScore      float64    `json:"ai_score"`
	AiVerdict    AiVerdict  `json:"ai_verdict"`
	AiReasons    []string   `json:"ai_reasons"`
	HumanID      *uuid.UUID `json:"human_id,omitempty"`
	HumanVerdict *string    `json:"human_verdict,omitempty"` // "APPROVED" | "REJECTED"
	HumanNote    *string    `json:"human_note,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	DecidedAt    *time.Time `json:"decided_at,omitempty"`
}

// AuditLog dai dien cho mot ban ghi nhat ky kiem toan he thong (BR-E5).
type AuditLog struct {
	ID        int64     `json:"id"`
	ActorType string    `json:"actor_type"` // "ai" | "human" | "system"
	ActorID   *string   `json:"actor_id,omitempty"`
	Action    string    `json:"action"`
	Subject   string    `json:"subject"` // e.g. "event:<uuid>"
	Reason    *string   `json:"reason,omitempty"`
	Metadata  any       `json:"metadata,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
