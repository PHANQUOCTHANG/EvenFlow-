package port

import (
	"context"
	"time"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/google/uuid"
)

// EventRepository định nghĩa giao tiếp lưu trữ dữ liệu cho Event và TicketTier.
type EventRepository interface {
	Create(ctx context.Context, event *domain.Event) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Event, error)
	GetBySlug(ctx context.Context, slug string) (*domain.Event, error)
	ListByOrganizer(ctx context.Context, organizerID uuid.UUID) ([]*domain.Event, error)
	// ListPublic tra su kien cong khai (SCHEDULED, ON_SALE, SOLD_OUT) kem tiers, sale_start_at tang dan, toi da limit.
	ListPublic(ctx context.Context, limit int) ([]*domain.Event, error)
	Update(ctx context.Context, event *domain.Event) error
	// UpdateWithOutbox cap nhat su kien va ghi cac message outbox trong MOT transaction
	// (quy tac bat bien 1): khong bao gio co su kien da doi trang thai ma thieu job di kem.
	UpdateWithOutbox(ctx context.Context, event *domain.Event, msgs ...OutboxMessage) error

	AddTier(ctx context.Context, tier *domain.TicketTier) error
	UpdateTier(ctx context.Context, tier *domain.TicketTier) error
	GetTiersByEventID(ctx context.Context, eventID uuid.UUID) ([]domain.TicketTier, error)
}

// ModerationRepository định nghĩa thao tác với bảng moderation_reviews (EV-120, EV-121).
type ModerationRepository interface {
	Create(ctx context.Context, review *domain.ModerationReview) error
	GetLatestByEventID(ctx context.Context, eventID uuid.UUID) (*domain.ModerationReview, error)
	// ListPendingHuman: review moi nhat cua moi su kien, AI NEEDS_HUMAN hoac AUTO_REJECTED, chua co
	// quyet dinh nguoi (EV-121: su kien AI tu choi cung phai lat nguoc duoc, BR-E5).
	ListPendingHuman(ctx context.Context) ([]*domain.ModerationReview, error)
	SetHumanDecision(ctx context.Context, reviewID uuid.UUID, humanID uuid.UUID, verdict, note string, decidedAt time.Time) error
}

// AuditLogRepository định nghĩa thao tác ghi và đọc audit_logs (BR-E5).
type AuditLogRepository interface {
	Create(ctx context.Context, log *domain.AuditLog) error
	ListBySubject(ctx context.Context, subject string) ([]*domain.AuditLog, error)
}

// OutboxMessage la mot dong outbox_events ghi cung transaction voi thay doi nghiep vu;
// relay (internal/worker) phat no ra RabbitMQ sau (EV-120).
type OutboxMessage struct {
	AggregateType string
	AggregateID   uuid.UUID
	RoutingKey    string
	Payload       any
}

// ModerationDecision la quyet dinh cua Moderator, ghi trong MOT transaction (EV-121, BR-E5,
// bat bien 1): trang thai su kien, quyet dinh tren review va audit -- hoac ca ba, hoac khong gi.
type ModerationDecision struct {
	Event      *domain.Event      // trang thai MOI
	PrevStatus domain.EventStatus // dieu kien cap nhat: trang thai da doi -> domain.ErrAlreadyDecided
	ReviewID   *uuid.UUID         // nil neu su kien chua co review AI
	HumanID    uuid.UUID
	Verdict    string // "APPROVED" | "REJECTED"
	Note       string
	DecidedAt  time.Time
	Audit      *domain.AuditLog
}

// ModerationDecisionStore ghi quyet dinh cua Moderator. Su kien khong con o PrevStatus ->
// domain.ErrAlreadyDecided: hai Moderator cung quyet tu cung mot trang thai thi dung mot nguoi thang.
type ModerationDecisionStore interface {
	ApplyDecision(ctx context.Context, d ModerationDecision) error
}
