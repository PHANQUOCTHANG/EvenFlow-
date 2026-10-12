package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// InventorySeeder nap ton kho tu Postgres vao Redis cho cac su kien dang ban.
//
// Redis chi la tang chan nhanh: khi no trong (khoi dong lan dau, mat du lieu,
// failover), hold.lua doc ton = 0 va tra SOLD_OUT cho moi request. Seeder lap lai
// deu dan thay vi chay mot lan luc khoi dong, vi worker thuong len truoc khi su
// kien duoc tao, va de Redis tu lanh lai sau su co.
//
// An toan khi chay lap: SeedInventory chi dien vao bucket chua co (HSETNX).
//
// Day la ban toi thieu cua EVF-37; job doi soat day du (so sanh va sua lech) se
// thay the o giai doan sau.
type InventorySeeder struct {
	src  port.InventorySource
	dst  port.InventorySeeder
	log  *slog.Logger
	cfg  SeederConfig
	seen map[string]int // su kien -> so bucket da nap lan truoc, de log khi co thay doi
}

type SeederConfig struct {
	EventIDs []string
	Interval time.Duration
}

func DefaultSeederConfig(eventIDs []string) SeederConfig {
	return SeederConfig{EventIDs: eventIDs, Interval: 5 * time.Second}
}

func NewInventorySeeder(src port.InventorySource, dst port.InventorySeeder,
	cfg SeederConfig, log *slog.Logger) *InventorySeeder {
	return &InventorySeeder{src: src, dst: dst, cfg: cfg, log: log, seen: make(map[string]int)}
}

// Run nap ngay lap tuc roi lap theo Interval cho toi khi ctx bi huy.
func (s *InventorySeeder) Run(ctx context.Context) error {
	s.seedAll(ctx)

	t := time.NewTicker(s.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			s.seedAll(ctx)
		}
	}
}

func (s *InventorySeeder) seedAll(ctx context.Context) {
	for _, ev := range s.cfg.EventIDs {
		// Loi o mot su kien khong duoc chan cac su kien con lai.
		if err := s.seed(ctx, ev); err != nil {
			s.log.Error("nap ton kho that bai", "event", ev, "err", err)
		}
	}
}

func (s *InventorySeeder) seed(ctx context.Context, eventID string) error {
	rows, err := s.src.ListInventory(ctx, eventID)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil // su kien chua ton tai hoac khong o trang thai dang ban
	}

	byType := make(map[string]map[int]int)
	for _, r := range rows {
		if byType[r.TicketTypeID] == nil {
			byType[r.TicketTypeID] = make(map[int]int)
		}
		byType[r.TicketTypeID][r.Bucket] = r.Available
	}
	for tt, buckets := range byType {
		if err := s.dst.SeedInventory(ctx, eventID, tt, buckets); err != nil {
			return err
		}
	}

	if s.seen[eventID] != len(rows) {
		s.seen[eventID] = len(rows)
		s.log.Info("da nap ton kho vao redis", "event", eventID,
			"hang_ve", len(byType), "so_bucket", len(rows))
	}
	return nil
}
