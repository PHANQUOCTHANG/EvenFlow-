package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/eventflow/eventflow/services/event/internal/port"
	"github.com/google/uuid"
)

type ModeratorApproveInput struct {
	EventID     uuid.UUID
	ModeratorID uuid.UUID
	Note        string // Bat buoc khi lat nguoc quyet dinh AI (BR-E5, EV-121)
}

type ModeratorRejectInput struct {
	EventID     uuid.UUID
	ModeratorID uuid.UUID
	Reason      string // Bat buoc theo BR-E5
}

type ModeratorActionUseCase interface {
	Approve(ctx context.Context, input ModeratorApproveInput) (*domain.Event, error)
	Reject(ctx context.Context, input ModeratorRejectInput) (*domain.Event, error)
}

type moderatorActionUseCase struct {
	eventRepo port.EventRepository
	modRepo   port.ModerationRepository
	decisions port.ModerationDecisionStore
}

// NewModeratorActionUseCase: quyet dinh, review va audit ghi qua `decisions` trong MOT transaction
// (EV-121) -- truoc day ghi rieng tung buoc va nuot loi, audit co the mat ma khong ai biet.
func NewModeratorActionUseCase(
	eventRepo port.EventRepository,
	modRepo port.ModerationRepository,
	decisions port.ModerationDecisionStore,
) ModeratorActionUseCase {
	return &moderatorActionUseCase{eventRepo: eventRepo, modRepo: modRepo, decisions: decisions}
}

func (uc *moderatorActionUseCase) Approve(ctx context.Context, input ModeratorApproveInput) (*domain.Event, error) {
	evt, err := uc.eventRepo.GetByID(ctx, input.EventID)
	if err != nil {
		return nil, err
	}

	// Su kien dang REJECTED ma duoc duyet = lat nguoc quyet dinh truoc do -- cua AI (BR-E5) hoac cua
	// Moderator khac: luon bat buoc ly do (AC-1 Jira).
	override := evt.Status == domain.StatusRejected
	note := strings.TrimSpace(input.Note)
	if override && note == "" {
		return nil, domain.ErrOverrideReasonRequired
	}

	prev := evt.Status
	now := time.Now().UTC()
	if alreadyDecided(evt.Status, "APPROVED") {
		return nil, domain.ErrAlreadyDecided
	}
	if err := evt.ApproveOverride(now); err != nil {
		return nil, err
	}

	rev, err := uc.latestReview(ctx, evt.ID)
	if err != nil {
		return nil, err
	}

	action := "event.moderator_approved"
	meta := map[string]any{"override": override}
	if rev != nil {
		meta["ai_verdict"] = string(rev.AiVerdict)
		meta["ai_score"] = rev.AiScore
		if rev.HumanVerdict != nil {
			meta["previous_human_verdict"] = *rev.HumanVerdict
		}
	}
	if override {
		action = "event.moderator_overrode_decision"
		if rev != nil && rev.AiVerdict == domain.VerdictAutoRejected && rev.HumanVerdict == nil {
			action = "event.moderator_overrode_ai"
		}
	}

	if err := uc.apply(ctx, evt, prev, rev, input.ModeratorID, "APPROVED", note, action, meta, now); err != nil {
		return nil, err
	}
	return evt, nil
}

func (uc *moderatorActionUseCase) Reject(ctx context.Context, input ModeratorRejectInput) (*domain.Event, error) {
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return nil, domain.ErrRejectReasonRequired
	}

	evt, err := uc.eventRepo.GetByID(ctx, input.EventID)
	if err != nil {
		return nil, err
	}

	prev := evt.Status
	if alreadyDecided(evt.Status, "REJECTED") {
		return nil, domain.ErrAlreadyDecided
	}
	if err := evt.Reject(); err != nil {
		return nil, err
	}

	rev, err := uc.latestReview(ctx, evt.ID)
	if err != nil {
		return nil, err
	}

	meta := map[string]any{"override": false}
	if rev != nil {
		meta["ai_verdict"] = string(rev.AiVerdict)
		meta["ai_score"] = rev.AiScore
	}

	now := time.Now().UTC()
	if err := uc.apply(ctx, evt, prev, rev, input.ModeratorID, "REJECTED", reason, "event.moderator_rejected", meta, now); err != nil {
		return nil, err
	}
	return evt, nil
}

// alreadyDecided: su kien da roi trang thai cho quyet dinh VI DA CO NGUOI QUYET (Moderator khac vua
// duyet / tu choi truoc ta). Tra ErrAlreadyDecided (409) de UI noi "nguoi khac vua quyet" thay vi
// loi chuyen trang thai chung chung. DRAFT, CANCELLED... khong phai do kiem duyet -> giu loi cu.
func alreadyDecided(status domain.EventStatus, verdict string) bool {
	switch status {
	case domain.StatusScheduled, domain.StatusOnSale:
		return true
	case domain.StatusRejected:
		return verdict == "REJECTED"
	default:
		return false
	}
}

// latestReview: chua co review (su kien chua qua AI) khong phai loi; loi khac thi tra ve.
func (uc *moderatorActionUseCase) latestReview(ctx context.Context, eventID uuid.UUID) (*domain.ModerationReview, error) {
	if uc.modRepo == nil {
		return nil, nil
	}
	rev, err := uc.modRepo.GetLatestByEventID(ctx, eventID)
	if errors.Is(err, domain.ErrModReviewNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return rev, nil
}

func (uc *moderatorActionUseCase) apply(
	ctx context.Context,
	evt *domain.Event,
	prev domain.EventStatus,
	rev *domain.ModerationReview,
	moderatorID uuid.UUID,
	verdict, note, action string,
	meta map[string]any,
	now time.Time,
) error {
	actorID := moderatorID.String()
	reason := note
	d := port.ModerationDecision{
		Event:      evt,
		PrevStatus: prev,
		HumanID:    moderatorID,
		Verdict:    verdict,
		Note:       note,
		DecidedAt:  now,
		Audit: &domain.AuditLog{
			ActorType: "human",
			ActorID:   &actorID,
			Action:    action,
			Subject:   fmt.Sprintf("event:%s", evt.ID),
			Reason:    &reason,
			Metadata:  meta,
			CreatedAt: now,
		},
	}
	if rev != nil {
		id := rev.ID
		d.ReviewID = &id
	}
	return uc.decisions.ApplyDecision(ctx, d)
}
