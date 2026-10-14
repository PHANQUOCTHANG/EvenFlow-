package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/eventflow/eventflow/services/event/internal/port"
	"github.com/google/uuid"
)

type moderationRepository struct {
	db *sql.DB
}

func NewModerationRepository(db *sql.DB) port.ModerationRepository {
	return &moderationRepository{db: db}
}

func (r *moderationRepository) Create(ctx context.Context, review *domain.ModerationReview) error {
	reasonsJSON, err := json.Marshal(review.AiReasons)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO moderation_reviews (
			id, event_id, ai_score, ai_verdict, ai_reasons,
			human_id, human_verdict, human_note, created_at, decided_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10
		)
	`

	_, err = r.db.ExecContext(
		ctx, query,
		review.ID,
		review.EventID,
		review.AiScore,
		string(review.AiVerdict),
		reasonsJSON,
		review.HumanID,
		review.HumanVerdict,
		review.HumanNote,
		review.CreatedAt,
		review.DecidedAt,
	)
	return err
}

func (r *moderationRepository) GetLatestByEventID(ctx context.Context, eventID uuid.UUID) (*domain.ModerationReview, error) {
	query := `
		SELECT
			id, event_id, ai_score, ai_verdict, ai_reasons,
			human_id, human_verdict, human_note, created_at, decided_at
		FROM moderation_reviews
		WHERE event_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`

	row := r.db.QueryRowContext(ctx, query, eventID)

	var rev domain.ModerationReview
	var aiVerdict sql.NullString
	var aiScore sql.NullFloat64
	var reasonsRaw []byte
	var humanIDStr sql.NullString
	var humanVerdict sql.NullString
	var humanNote sql.NullString
	var decidedAt sql.NullTime

	err := row.Scan(
		&rev.ID,
		&rev.EventID,
		&aiScore,
		&aiVerdict,
		&reasonsRaw,
		&humanIDStr,
		&humanVerdict,
		&humanNote,
		&rev.CreatedAt,
		&decidedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrModReviewNotFound
		}
		return nil, err
	}

	if aiScore.Valid {
		rev.AiScore = aiScore.Float64
	}
	if aiVerdict.Valid {
		rev.AiVerdict = domain.AiVerdict(aiVerdict.String)
	}
	if len(reasonsRaw) > 0 {
		_ = json.Unmarshal(reasonsRaw, &rev.AiReasons)
	}
	if humanIDStr.Valid {
		hID, err := uuid.Parse(humanIDStr.String)
		if err == nil {
			rev.HumanID = &hID
		}
	}
	if humanVerdict.Valid {
		v := humanVerdict.String
		rev.HumanVerdict = &v
	}
	if humanNote.Valid {
		n := humanNote.String
		rev.HumanNote = &n
	}
	if decidedAt.Valid {
		t := decidedAt.Time
		rev.DecidedAt = &t
	}

	return &rev, nil
}

func (r *moderationRepository) ListNeedsHuman(ctx context.Context) ([]*domain.ModerationReview, error) {
	query := `
		SELECT
			id, event_id, ai_score, ai_verdict, ai_reasons,
			human_id, human_verdict, human_note, created_at, decided_at
		FROM moderation_reviews
		WHERE ai_verdict = 'NEEDS_HUMAN' AND human_verdict IS NULL
		ORDER BY created_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*domain.ModerationReview
	for rows.Next() {
		var rev domain.ModerationReview
		var aiVerdict sql.NullString
		var aiScore sql.NullFloat64
		var reasonsRaw []byte
		var humanIDStr sql.NullString
		var humanVerdict sql.NullString
		var humanNote sql.NullString
		var decidedAt sql.NullTime

		if err := rows.Scan(
			&rev.ID,
			&rev.EventID,
			&aiScore,
			&aiVerdict,
			&reasonsRaw,
			&humanIDStr,
			&humanVerdict,
			&humanNote,
			&rev.CreatedAt,
			&decidedAt,
		); err != nil {
			return nil, err
		}

		if aiScore.Valid {
			rev.AiScore = aiScore.Float64
		}
		if aiVerdict.Valid {
			rev.AiVerdict = domain.AiVerdict(aiVerdict.String)
		}
		if len(reasonsRaw) > 0 {
			_ = json.Unmarshal(reasonsRaw, &rev.AiReasons)
		}
		if humanIDStr.Valid {
			hID, err := uuid.Parse(humanIDStr.String)
			if err == nil {
				rev.HumanID = &hID
			}
		}
		if humanVerdict.Valid {
			v := humanVerdict.String
			rev.HumanVerdict = &v
		}
		if humanNote.Valid {
			n := humanNote.String
			rev.HumanNote = &n
		}
		if decidedAt.Valid {
			t := decidedAt.Time
			rev.DecidedAt = &t
		}

		list = append(list, &rev)
	}

	return list, rows.Err()
}

func (r *moderationRepository) SetHumanDecision(
	ctx context.Context,
	reviewID uuid.UUID,
	humanID uuid.UUID,
	verdict, note string,
	decidedAt time.Time,
) error {
	query := `
		UPDATE moderation_reviews
		SET human_id = $1,
		    human_verdict = $2,
		    human_note = $3,
		    decided_at = $4
		WHERE id = $5
	`
	res, err := r.db.ExecContext(ctx, query, humanID, verdict, note, decidedAt, reviewID)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return domain.ErrModReviewNotFound
	}
	return nil
}
