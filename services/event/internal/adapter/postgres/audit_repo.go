package postgres

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/eventflow/eventflow/services/event/internal/port"
)

type auditLogRepository struct {
	db *sql.DB
}

func NewAuditLogRepository(db *sql.DB) port.AuditLogRepository {
	return &auditLogRepository{db: db}
}

func (r *auditLogRepository) Create(ctx context.Context, log *domain.AuditLog) error {
	var metaJSON []byte
	var err error
	if log.Metadata != nil {
		metaJSON, err = json.Marshal(log.Metadata)
		if err != nil {
			return err
		}
	}

	query := `
		INSERT INTO audit_logs (
			actor_type, actor_id, action, subject, reason, metadata, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7
		) RETURNING id
	`

	var id int64
	err = r.db.QueryRowContext(
		ctx, query,
		log.ActorType,
		log.ActorID,
		log.Action,
		log.Subject,
		log.Reason,
		metaJSON,
		log.CreatedAt,
	).Scan(&id)
	if err != nil {
		return err
	}

	log.ID = id
	return nil
}

func (r *auditLogRepository) ListBySubject(ctx context.Context, subject string) ([]*domain.AuditLog, error) {
	query := `
		SELECT id, actor_type, actor_id, action, subject, reason, metadata, created_at
		FROM audit_logs
		WHERE subject = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*domain.AuditLog
	for rows.Next() {
		var l domain.AuditLog
		var actorID sql.NullString
		var reason sql.NullString
		var metaRaw []byte

		if err := rows.Scan(
			&l.ID,
			&l.ActorType,
			&actorID,
			&l.Action,
			&l.Subject,
			&reason,
			&metaRaw,
			&l.CreatedAt,
		); err != nil {
			return nil, err
		}

		if actorID.Valid {
			idStr := actorID.String
			l.ActorID = &idStr
		}
		if reason.Valid {
			rStr := reason.String
			l.Reason = &rStr
		}
		if len(metaRaw) > 0 {
			var m any
			if err := json.Unmarshal(metaRaw, &m); err == nil {
				l.Metadata = m
			}
		}

		logs = append(logs, &l)
	}

	return logs, rows.Err()
}
