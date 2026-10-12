package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/eventflow/eventflow/services/identity/internal/domain"
)

type auditRepo struct {
	db *sql.DB
}

// NewAuditRepo khởi tạo repository lưu trữ audit log vào bảng audit_logs của Postgres
func NewAuditRepo(db *sql.DB) domain.AuditRepository {
	return &auditRepo{db: db}
}

func (r *auditRepo) Save(ctx context.Context, log *domain.AuditLog) error {
	query := `
		INSERT INTO audit_logs (actor_type, actor_id, action, subject, reason, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`

	metadataJSON, err := json.Marshal(log.Metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	createdAt := log.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	var actorID sql.NullString
	if log.ActorID != "" {
		actorID = sql.NullString{String: log.ActorID, Valid: true}
	}

	return r.db.QueryRowContext(ctx, query,
		log.ActorType,
		actorID,
		log.Action,
		log.Subject,
		log.Reason,
		metadataJSON,
		createdAt,
	).Scan(&log.ID)
}
