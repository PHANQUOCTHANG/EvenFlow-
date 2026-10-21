package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/eventflow/eventflow/services/event/internal/port"
	"github.com/google/uuid"
)

type eventRepository struct {
	db *sql.DB
}

func NewEventRepository(db *sql.DB) port.EventRepository {
	return &eventRepository{db: db}
}

func (r *eventRepository) Create(ctx context.Context, e *domain.Event) error {
	query := `
		INSERT INTO events (
			id, organizer_id, slug, title, description, venue,
			starts_at, sale_start_at, sale_end_at, status,
			max_per_identity, max_per_order, hold_ttl_seconds,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10,
			$11, $12, $13,
			$14, $15
		)
	`
	_, err := r.db.ExecContext(
		ctx, query,
		e.ID, e.OrganizerID, e.Slug, e.Title, e.Description, e.Venue,
		e.StartsAt, e.SaleStartAt, e.SaleEndAt, string(e.Status),
		e.MaxPerIdentity, e.MaxPerOrder, e.HoldTTLSeconds,
		e.CreatedAt, e.UpdatedAt,
	)
	return err
}

func (r *eventRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Event, error) {
	query := `
		SELECT
			id, organizer_id, slug, title, description, venue,
			starts_at, sale_start_at, sale_end_at, status,
			max_per_identity, max_per_order, hold_ttl_seconds,
			created_at, updated_at
		FROM events
		WHERE id = $1
	`
	row := r.db.QueryRowContext(ctx, query, id)

	var e domain.Event
	var status string
	var desc, venue sql.NullString
	var saleEndAt sql.NullTime

	err := row.Scan(
		&e.ID, &e.OrganizerID, &e.Slug, &e.Title, &desc, &venue,
		&e.StartsAt, &e.SaleStartAt, &saleEndAt, &status,
		&e.MaxPerIdentity, &e.MaxPerOrder, &e.HoldTTLSeconds,
		&e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrEventNotFound
		}
		return nil, err
	}

	e.Description = desc.String
	e.Venue = venue.String
	e.Status = domain.EventStatus(status)
	if saleEndAt.Valid {
		e.SaleEndAt = &saleEndAt.Time
	}

	tiers, _ := r.GetTiersByEventID(ctx, e.ID)
	e.Tiers = tiers
	return &e, nil
}

func (r *eventRepository) GetBySlug(ctx context.Context, slug string) (*domain.Event, error) {
	query := `
		SELECT
			id, organizer_id, slug, title, description, venue,
			starts_at, sale_start_at, sale_end_at, status,
			max_per_identity, max_per_order, hold_ttl_seconds,
			created_at, updated_at
		FROM events
		WHERE slug = $1
	`
	row := r.db.QueryRowContext(ctx, query, slug)

	var e domain.Event
	var status string
	var desc, venue sql.NullString
	var saleEndAt sql.NullTime

	err := row.Scan(
		&e.ID, &e.OrganizerID, &e.Slug, &e.Title, &desc, &venue,
		&e.StartsAt, &e.SaleStartAt, &saleEndAt, &status,
		&e.MaxPerIdentity, &e.MaxPerOrder, &e.HoldTTLSeconds,
		&e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrEventNotFound
		}
		return nil, err
	}

	e.Description = desc.String
	e.Venue = venue.String
	e.Status = domain.EventStatus(status)
	if saleEndAt.Valid {
		e.SaleEndAt = &saleEndAt.Time
	}

	tiers, _ := r.GetTiersByEventID(ctx, e.ID)
	e.Tiers = tiers
	return &e, nil
}

func (r *eventRepository) ListPublic(ctx context.Context, limit int) ([]*domain.Event, error) {
	query := `
		SELECT
			id, organizer_id, slug, title, description, venue,
			starts_at, sale_start_at, sale_end_at, status,
			max_per_identity, max_per_order, hold_ttl_seconds,
			created_at, updated_at
		FROM events
		WHERE status IN ('SCHEDULED', 'ON_SALE', 'SOLD_OUT')
		ORDER BY sale_start_at ASC, id ASC
		LIMIT $1
	`
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []*domain.Event{}
	byID := map[uuid.UUID]*domain.Event{}
	for rows.Next() {
		e := &domain.Event{}
		var status string
		var desc, venue sql.NullString
		var saleEndAt sql.NullTime

		if err := rows.Scan(
			&e.ID, &e.OrganizerID, &e.Slug, &e.Title, &desc, &venue,
			&e.StartsAt, &e.SaleStartAt, &saleEndAt, &status,
			&e.MaxPerIdentity, &e.MaxPerOrder, &e.HoldTTLSeconds,
			&e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, err
		}

		e.Description = desc.String
		e.Venue = venue.String
		e.Status = domain.EventStatus(status)
		if saleEndAt.Valid {
			e.SaleEndAt = &saleEndAt.Time
		}
		events = append(events, e)
		byID[e.ID] = e
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if len(events) == 0 {
		return events, nil
	}

	// Mot query tiers cho ca danh sach (tranh N+1). Placeholder $1..$n, n <= limit.
	args := make([]any, len(events))
	holders := make([]string, len(events))
	for i, e := range events {
		args[i] = e.ID
		holders[i] = "$" + strconv.Itoa(i+1)
	}
	tierQuery := `
		SELECT id, event_id, name, price_cents, currency, quota, bucket_count, created_at
		FROM ticket_types
		WHERE event_id IN (` + strings.Join(holders, ", ") + `)
		ORDER BY price_cents ASC
	`
	trows, err := r.db.QueryContext(ctx, tierQuery, args...)
	if err != nil {
		return nil, err
	}
	defer trows.Close()
	for trows.Next() {
		var t domain.TicketTier
		if err := trows.Scan(
			&t.ID, &t.EventID, &t.Name, &t.PriceCents,
			&t.Currency, &t.Quota, &t.BucketCount, &t.CreatedAt,
		); err != nil {
			return nil, err
		}
		if e := byID[t.EventID]; e != nil {
			e.Tiers = append(e.Tiers, t)
		}
	}
	return events, trows.Err()
}

func (r *eventRepository) ListByOrganizer(ctx context.Context, organizerID uuid.UUID) ([]*domain.Event, error) {
	query := `
		SELECT
			id, organizer_id, slug, title, description, venue,
			starts_at, sale_start_at, sale_end_at, status,
			max_per_identity, max_per_order, hold_ttl_seconds,
			created_at, updated_at
		FROM events
		WHERE organizer_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, organizerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*domain.Event
	for rows.Next() {
		var e domain.Event
		var status string
		var desc, venue sql.NullString
		var saleEndAt sql.NullTime

		if err := rows.Scan(
			&e.ID, &e.OrganizerID, &e.Slug, &e.Title, &desc, &venue,
			&e.StartsAt, &e.SaleStartAt, &saleEndAt, &status,
			&e.MaxPerIdentity, &e.MaxPerOrder, &e.HoldTTLSeconds,
			&e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, err
		}

		e.Description = desc.String
		e.Venue = venue.String
		e.Status = domain.EventStatus(status)
		if saleEndAt.Valid {
			e.SaleEndAt = &saleEndAt.Time
		}
		events = append(events, &e)
	}

	return events, rows.Err()
}

func (r *eventRepository) Update(ctx context.Context, e *domain.Event) error {
	return updateEvent(ctx, r.db, e)
}

// UpdateWithOutbox cap nhat su kien va ghi outbox trong MOT transaction (EV-120, bat bien 1).
func (r *eventRepository) UpdateWithOutbox(ctx context.Context, e *domain.Event, msgs ...port.OutboxMessage) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := updateEvent(ctx, tx, e); err != nil {
		return err
	}
	const insertOutbox = `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, routing_key, payload)
		VALUES ($1, $2, $3, $4)`
	for _, m := range msgs {
		payload, err := json.Marshal(m.Payload)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, insertOutbox, m.AggregateType, m.AggregateID, m.RoutingKey, payload); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func updateEvent(ctx context.Context, db execer, e *domain.Event) error {
	query := `
		UPDATE events SET
			title = $1,
			description = $2,
			venue = $3,
			starts_at = $4,
			sale_start_at = $5,
			sale_end_at = $6,
			status = $7,
			updated_at = $8
		WHERE id = $9
	`
	_, err := db.ExecContext(
		ctx, query,
		e.Title, e.Description, e.Venue,
		e.StartsAt, e.SaleStartAt, e.SaleEndAt,
		string(e.Status), e.UpdatedAt, e.ID,
	)
	return err
}

func (r *eventRepository) AddTier(ctx context.Context, tier *domain.TicketTier) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	query := `
		INSERT INTO ticket_types (
			id, event_id, name, price_cents, currency, quota, bucket_count, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		)
	`
	_, err = tx.ExecContext(
		ctx, query,
		tier.ID, tier.EventID, tier.Name, tier.PriceCents,
		tier.Currency, tier.Quota, tier.BucketCount, tier.CreatedAt,
	)
	if err != nil {
		return err
	}

	// EV-125: Khởi tạo 32 bucket trong ticket_inventory
	buckets := tier.DistributeQuota()
	invQuery := `
		INSERT INTO ticket_inventory (ticket_type_id, bucket, capacity, available)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (ticket_type_id, bucket) DO NOTHING
	`
	for b, qty := range buckets {
		if _, err := tx.ExecContext(ctx, invQuery, tier.ID, b, qty, qty); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *eventRepository) UpdateTier(ctx context.Context, tier *domain.TicketTier) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var oldQuota, bucketCount int
	err = tx.QueryRowContext(ctx, "SELECT quota, bucket_count FROM ticket_types WHERE id = $1 FOR UPDATE", tier.ID).Scan(&oldQuota, &bucketCount)
	if err != nil {
		return err
	}

	query := `
		UPDATE ticket_types SET
			name = $1,
			price_cents = $2,
			quota = $3
		WHERE id = $4
	`
	_, err = tx.ExecContext(ctx, query, tier.Name, tier.PriceCents, tier.Quota, tier.ID)
	if err != nil {
		return err
	}

	delta := tier.Quota - oldQuota
	if delta > 0 {
		tier.BucketCount = bucketCount
		addBuckets := tier.DistributeAdditionalQuota(delta)
		updateInv := `
			UPDATE ticket_inventory
			   SET capacity = capacity + $3,
			       available = available + $3
			 WHERE ticket_type_id = $1 AND bucket = $2
		`
		for b, add := range addBuckets {
			if _, err := tx.ExecContext(ctx, updateInv, tier.ID, b, add); err != nil {
				return err
			}
		}
	} else if delta < 0 {
		tier.BucketCount = bucketCount
		newBuckets := tier.DistributeQuota()
		updateInv := `
			UPDATE ticket_inventory
			   SET capacity = $3,
			       available = $3
			 WHERE ticket_type_id = $1 AND bucket = $2
		`
		for b, qty := range newBuckets {
			if _, err := tx.ExecContext(ctx, updateInv, tier.ID, b, qty); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (r *eventRepository) GetTiersByEventID(ctx context.Context, eventID uuid.UUID) ([]domain.TicketTier, error) {
	query := `
		SELECT id, event_id, name, price_cents, currency, quota, bucket_count, created_at
		FROM ticket_types
		WHERE event_id = $1
		ORDER BY price_cents ASC
	`
	rows, err := r.db.QueryContext(ctx, query, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tiers []domain.TicketTier
	for rows.Next() {
		var t domain.TicketTier
		if err := rows.Scan(
			&t.ID, &t.EventID, &t.Name, &t.PriceCents,
			&t.Currency, &t.Quota, &t.BucketCount, &t.CreatedAt,
		); err != nil {
			return nil, err
		}
		tiers = append(tiers, t)
	}
	return tiers, rows.Err()
}
