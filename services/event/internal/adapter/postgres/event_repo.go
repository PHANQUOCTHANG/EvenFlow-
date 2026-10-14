package postgres

import (
	"context"
	"database/sql"
	"errors"

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
	_, err := r.db.ExecContext(
		ctx, query,
		e.Title, e.Description, e.Venue,
		e.StartsAt, e.SaleStartAt, e.SaleEndAt,
		string(e.Status), e.UpdatedAt, e.ID,
	)
	return err
}

func (r *eventRepository) AddTier(ctx context.Context, tier *domain.TicketTier) error {
	query := `
		INSERT INTO ticket_types (
			id, event_id, name, price_cents, currency, quota, bucket_count, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		)
	`
	_, err := r.db.ExecContext(
		ctx, query,
		tier.ID, tier.EventID, tier.Name, tier.PriceCents,
		tier.Currency, tier.Quota, tier.BucketCount, tier.CreatedAt,
	)
	return err
}

func (r *eventRepository) UpdateTier(ctx context.Context, tier *domain.TicketTier) error {
	query := `
		UPDATE ticket_types SET
			name = $1,
			price_cents = $2,
			quota = $3
		WHERE id = $4
	`
	_, err := r.db.ExecContext(ctx, query, tier.Name, tier.PriceCents, tier.Quota, tier.ID)
	return err
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
