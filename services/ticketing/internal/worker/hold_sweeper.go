// Package worker chua cac tien trinh nen cua ticketing.
package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// HoldSweeper nha cac lenh giu ghe da het han va tra ve vao kho (BR-O2, BR-O7).
//
// Vi sao KHONG dung Redis keyspace notification: thong bao het han cua Redis la
// best-effort -- mat ket noi, failover, hoac client cham mot nhip la mat thong
// bao, va so ve do se bi khoa vinh vien. Voi mot dot mo ban, "mat vai tram ve"
// la sai sot khong the chap nhan.
//
// Thay vao do sweeper quet mot ZSET sap xep theo thoi diem het han. Cach nay:
//   - Khong mat gi khi worker chet: lan chay sau van thay dung nhung hold do.
//   - An toan khi chay nhieu instance song song: viec tra kho la idempotent o
//     CA HAI tang (Lua xoa-truoc-cong-sau, va SQL co dieu kien released_at IS NULL).
type HoldSweeper struct {
	gate  port.InventoryGate
	repo  port.Repository
	pub   port.Publisher
	log   *slog.Logger
	cfg   SweeperConfig
}

type SweeperConfig struct {
	// Danh sach su kien dang mo ban can quet. Trong thuc te lay tu event-svc.
	EventIDs []string
	Interval time.Duration // 1 giay: khach da doi het 10 phut, khong nen bat doi them
	Batch    int
}

func DefaultSweeperConfig(eventIDs []string) SweeperConfig {
	return SweeperConfig{EventIDs: eventIDs, Interval: time.Second, Batch: 500}
}

func NewHoldSweeper(gate port.InventoryGate, repo port.Repository, pub port.Publisher,
	cfg SweeperConfig, log *slog.Logger) *HoldSweeper {
	return &HoldSweeper{gate: gate, repo: repo, pub: pub, cfg: cfg, log: log}
}

func (s *HoldSweeper) Run(ctx context.Context) error {
	t := time.NewTicker(s.cfg.Interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			for _, ev := range s.cfg.EventIDs {
				if err := s.sweep(ctx, ev); err != nil {
					// Loi mot vong quet khong duoc lam chet sweeper: ngung quet
					// dong nghia voi ve bi khoa vinh vien.
					s.log.Error("vong quet that bai", "event", ev, "err", err)
				}
			}
		}
	}
}

func (s *HoldSweeper) sweep(ctx context.Context, eventID string) error {
	holdIDs, err := s.gate.ExpiredHolds(ctx, eventID, s.cfg.Batch)
	if err != nil {
		return err
	}
	if len(holdIDs) == 0 {
		return nil
	}

	var released int
	for _, id := range holdIDs {
		// Thu tu QUAN TRONG: Postgres truoc (nguon su that), Redis sau.
		//
		// Neu doi thu tu va tien trinh chet o giua, Redis se co them ve trong khi
		// Postgres van coi la da giu -> Redis phong len -> nguy co oversell.
		// Voi thu tu nay, tinh huong xau nhat la Postgres da tra kho con Redis
		// chua -- tuc la ban thieu tam thoi, va job doi soat sua lai sau 10 giay.
		ok, err := s.repo.ReleaseHold(ctx, id)
		if err != nil {
			s.log.Error("tra kho trong postgres that bai", "hold", id, "err", err)
			continue
		}
		if err := s.gate.Release(ctx, eventID, id); err != nil {
			s.log.Error("tra kho trong redis that bai", "hold", id, "err", err)
			continue
		}
		if ok {
			released++
			_ = s.pub.Publish(ctx, "ticketing.hold.released", map[string]any{
				"event_id":    eventID,
				"hold_id":     id,
				"released_at": time.Now().UTC(),
			})
		}
	}

	if released > 0 {
		s.log.Info("da nha hold het han", "event", eventID,
			"so_luong", released, "da_quet", len(holdIDs))
	}
	return nil
}
