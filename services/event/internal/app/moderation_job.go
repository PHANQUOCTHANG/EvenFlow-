package app

import (
	"time"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/eventflow/eventflow/services/event/internal/port"
	"github.com/google/uuid"
)

const (
	// OutboxAggregateEvent la aggregate_type cua event service trong outbox_events (bang dung
	// chung): relay cua moi service chi lay dong cua chinh minh.
	OutboxAggregateEvent = "event"
	// ModerationRoutingKey khop binding cua exchange ai.direct -> ai.moderation.q
	// (libs/go/mq.AITopology). Khong phai ten queue.
	ModerationRoutingKey = "moderation"
)

// ModerationJobMessage dung job kiem duyet AI (EV-120). Payload la hop dong voi
// ai-worker (app/domain/moderation.py: ModerationJob).
func ModerationJobMessage(evt *domain.Event, now time.Time, jobID uuid.UUID) port.OutboxMessage {
	return port.OutboxMessage{
		AggregateType: OutboxAggregateEvent,
		AggregateID:   evt.ID,
		RoutingKey:    ModerationRoutingKey,
		Payload: map[string]any{
			"job_id":            jobID.String(),
			"event_id":          evt.ID.String(),
			"event_title":       evt.Title,
			"event_description": evt.Description,
			"enqueued_at":       float64(now.UnixNano()) / 1e9,
		},
	}
}
