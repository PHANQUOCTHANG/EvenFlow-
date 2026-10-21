package postgres

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/eventflow/eventflow/services/event/internal/port"
)

type decisionStore struct {
	db *sql.DB
}

// NewModerationDecisionStore ghi quyet dinh cua Moderator trong MOT transaction (EV-121).
func NewModerationDecisionStore(db *sql.DB) port.ModerationDecisionStore {
	return &decisionStore{db: db}
}

// ApplyDecision: su kien (co dieu kien trang thai cu), quyet dinh tren review va audit.
// Buoc nao hong -> rollback ca ba. Dieu kien khong khop -> domain.ErrAlreadyDecided: hai Moderator
// cung quyet thi dung mot nguoi thang (UPDATE ... WHERE status = $prev khoa dong roi kiem lai).
func (s *decisionStore) ApplyDecision(ctx context.Context, d port.ModerationDecision) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`UPDATE events SET status = $1, updated_at = $2 WHERE id = $3 AND status = $4`,
		string(d.Event.Status), d.Event.UpdatedAt, d.Event.ID, string(d.PrevStatus))
	if err != nil {
		return err
	}
	if err := requireOneRow(res); err != nil {
		return err
	}

	if d.ReviewID != nil {
		res, err := tx.ExecContext(ctx, `
			UPDATE moderation_reviews
			SET human_id = $1, human_verdict = $2, human_note = $3, decided_at = $4
			WHERE id = $5`,
			d.HumanID, d.Verdict, d.Note, d.DecidedAt, *d.ReviewID)
		if err != nil {
			return err
		}
		if err := requireOneRow(res); err != nil {
			return err
		}
	}

	var meta []byte
	if d.Audit.Metadata != nil {
		if meta, err = json.Marshal(d.Audit.Metadata); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs (actor_type, actor_id, action, subject, reason, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		d.Audit.ActorType, d.Audit.ActorID, d.Audit.Action, d.Audit.Subject, d.Audit.Reason, meta, d.Audit.CreatedAt,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func requireOneRow(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return domain.ErrAlreadyDecided
	}
	return nil
}
