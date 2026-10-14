package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/eventflow/eventflow/services/event/internal/port"
	"github.com/google/uuid"
)

type outboxRepository struct {
	db *sql.DB
}

func NewOutboxRepository(db *sql.DB) port.OutboxRepository {
	return &outboxRepository{db: db}
}

func (r *outboxRepository) Publish(
	ctx context.Context,
	aggregateType string,
	aggregateID uuid.UUID,
	routingKey string,
	payload any,
) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO outbox_events (
			aggregate_type, aggregate_id, routing_key, payload, created_at
		) VALUES (
			$1, $2, $3, $4, $5
		)
	`

	_, err = r.db.ExecContext(
		ctx, query,
		aggregateType,
		aggregateID,
		routingKey,
		payloadJSON,
		time.Now().UTC(),
	)
	return err
}
