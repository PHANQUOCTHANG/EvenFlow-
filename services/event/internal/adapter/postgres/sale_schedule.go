package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// SaleScheduleRepository mo ban su kien den gio (EV-118).
type SaleScheduleRepository struct{ db *sql.DB }

// NewSaleScheduleRepository tao repository.
func NewSaleScheduleRepository(db *sql.DB) *SaleScheduleRepository {
	return &SaleScheduleRepository{db: db}
}

// OpenDueSales chuyen moi su kien SCHEDULED co sale_start_at <= now sang ON_SALE va tra id da doi.
// Mot lenh UPDATE co dieu kien: nhieu instance cung chay thi moi dong chi mot instance doi duoc,
// khong can khoa rieng. Cung dieu kien voi domain.Event.OpenSale.
func (r *SaleScheduleRepository) OpenDueSales(ctx context.Context, now time.Time) ([]uuid.UUID, error) {
	rows, err := r.db.QueryContext(ctx, `
		UPDATE events
		   SET status = 'ON_SALE', updated_at = now()
		 WHERE status = 'SCHEDULED' AND sale_start_at <= $1
		RETURNING id`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
