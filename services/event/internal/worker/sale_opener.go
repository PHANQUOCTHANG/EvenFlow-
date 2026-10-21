package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// DueSaleOpener chuyen su kien den gio mo ban sang ON_SALE (postgres.SaleScheduleRepository).
type DueSaleOpener interface {
	OpenDueSales(ctx context.Context, now time.Time) ([]uuid.UUID, error)
}

// SaleOpener mo ban dung T0 thay cho Ops goi POST /publish bang tay (EV-118). Phong cho khong doi
// trang thai nay: lottery va admit chay theo sale_start_ms trong Redis; ON_SALE la trang thai cho
// trang su kien va danh sach cong khai.
type SaleOpener struct {
	repo     DueSaleOpener
	interval time.Duration
	log      *slog.Logger
	now      func() time.Time
}

// NewSaleOpener tao scheduler. interval <= 0 thi 1 giay.
func NewSaleOpener(repo DueSaleOpener, interval time.Duration, log *slog.Logger) *SaleOpener {
	if interval <= 0 {
		interval = time.Second
	}
	return &SaleOpener{repo: repo, interval: interval, log: log, now: time.Now}
}

// Run quet ngay roi theo chu ky toi khi ctx bi huy.
func (o *SaleOpener) Run(ctx context.Context) error {
	o.OpenDue(ctx)
	t := time.NewTicker(o.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			o.OpenDue(ctx)
		}
	}
}

// OpenDue chay mot vong; loi chi log, vong sau thu lai.
func (o *SaleOpener) OpenDue(ctx context.Context) {
	ids, err := o.repo.OpenDueSales(ctx, o.now().UTC())
	if err != nil {
		if ctx.Err() == nil {
			o.log.Error("mo ban su kien den gio that bai", "err", err)
		}
		return
	}
	for _, id := range ids {
		o.log.Info("da mo ban su kien (SCHEDULED -> ON_SALE)", "event", id)
	}
}
