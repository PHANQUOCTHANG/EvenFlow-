package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// SoldOutWatcher theo doi ton kho Redis va bao cho phong cho khi het ve (EV-146, BR-Q6).
//
// Hai muc, vi hold chua thanh toan co the het han va tra kho lai:
//
//   - StockZero: kho = 0 (da nap). Phong cho ngung tha them nguoi ngay, nhung khach khong thay doi.
//   - SoldOut: kho = 0 va KHONG con hold nao, lien tiep ConfirmTicks lan. Khach chua vao nhan SOLD_OUT.
//
// Khong dung ket qua "giu ve that bai" lam bang chung het ve: hold.lua chi quet toi da 3 bucket nen no co
// the bao het khi bucket khac con ve. Tin hieu o day dua tren TONG ton kho.
//
// Co tu go khi kho co lai (hoan tien, hold het han) o vong tiep theo.
//
// Check chi duoc goi tu mot goroutine (Run); mu chi bao ve map state.
type SoldOutWatcher struct {
	tiers TierSource
	stock port.StockReader
	sink  port.SaleStateSink
	cfg   SoldOutWatcherConfig
	log   *slog.Logger

	mu    sync.Mutex
	state map[string]*watchState
	now   func() time.Time
}

// SoldOutWatcherConfig cau hinh watcher.
type SoldOutWatcherConfig struct {
	EventIDs []string
	// Interval la chu ky kiem tra. 1 giay: khach thay SOLD_OUT trong vai giay sau khi ve cuoi het.
	Interval time.Duration
	// ConfirmTicks la so lan lien tiep thay kho 0 va khong hold truoc khi bao SoldOut: loc nhiu khi
	// job doi soat sua lech Redis.
	ConfirmTicks int
	// TierRefresh la thoi gian giu danh sach hang ve (lay tu Postgres) truoc khi hoi lai.
	TierRefresh time.Duration
}

func DefaultSoldOutWatcherConfig(eventIDs []string) SoldOutWatcherConfig {
	return SoldOutWatcherConfig{EventIDs: eventIDs, Interval: time.Second, ConfirmTicks: 3, TierRefresh: 30 * time.Second}
}

// TierSource liet ke hang ve cua su kien (port.InventorySource thoa man qua ListInventory; xem NewTierSource).
type TierSource interface {
	TicketTypeIDs(ctx context.Context, eventID string) ([]string, error)
}

type inventoryTiers struct{ src port.InventorySource }

// NewTierSource lay danh sach hang ve tu ton kho Postgres (chi su kien dang/sap ban).
func NewTierSource(src port.InventorySource) TierSource { return inventoryTiers{src} }

func (t inventoryTiers) TicketTypeIDs(ctx context.Context, eventID string) ([]string, error) {
	rows, err := t.src.ListInventory(ctx, eventID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var ids []string
	for _, r := range rows {
		if !seen[r.TicketTypeID] {
			seen[r.TicketTypeID] = true
			ids = append(ids, r.TicketTypeID)
		}
	}
	return ids, nil
}

type watchState struct {
	tiers      []string
	tiersAt    time.Time
	zeroTicks  int
	published  port.SaleState
	haveSignal bool // da ghi it nhat mot lan
}

func NewSoldOutWatcher(tiers TierSource, stock port.StockReader, sink port.SaleStateSink,
	cfg SoldOutWatcherConfig, log *slog.Logger) *SoldOutWatcher {
	return &SoldOutWatcher{tiers: tiers, stock: stock, sink: sink, cfg: cfg, log: log,
		state: map[string]*watchState{}, now: time.Now}
}

func (w *SoldOutWatcher) Run(ctx context.Context) error {
	t := time.NewTicker(w.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			w.CheckAll(ctx)
		}
	}
}

// CheckAll kiem tra moi su kien mot lan. Loi o mot su kien khong chan cac su kien khac.
func (w *SoldOutWatcher) CheckAll(ctx context.Context) {
	for _, ev := range w.cfg.EventIDs {
		if err := w.Check(ctx, ev); err != nil {
			w.log.Error("kiem tra het ve that bai", "event", ev, "err", err)
		}
	}
}

// Check kiem tra mot su kien va cap nhat co neu doi.
func (w *SoldOutWatcher) Check(ctx context.Context, eventID string) error {
	w.mu.Lock()
	st := w.state[eventID]
	if st == nil {
		st = &watchState{}
		w.state[eventID] = st
	}
	w.mu.Unlock()

	if st.tiers == nil || w.now().Sub(st.tiersAt) >= w.cfg.TierRefresh {
		ids, err := w.tiers.TicketTypeIDs(ctx, eventID)
		if err != nil {
			return err
		}
		st.tiers, st.tiersAt = ids, w.now()
	}
	if len(st.tiers) == 0 {
		return nil // su kien chua dang ban hoac khong ton tai: khong co gi de bao
	}

	info, err := w.stock.Stock(ctx, eventID, st.tiers)
	if err != nil {
		return err
	}
	if !info.Seeded {
		// Chua nap kho vao Redis: "0" o day la chua nap chu khong phai het ve. Khong ghi gi.
		st.zeroTicks = 0
		return nil
	}

	zero := info.Total == 0
	if zero && info.ActiveHolds == 0 {
		st.zeroTicks++
	} else {
		st.zeroTicks = 0
	}
	want := port.SaleState{StockZero: zero, SoldOut: st.zeroTicks >= w.cfg.ConfirmTicks}

	if st.haveSignal && want == st.published {
		return nil
	}
	if err := w.sink.SetSaleState(ctx, eventID, want); err != nil {
		return err // thu lai o vong sau, vi st.published chua doi
	}
	prev := st.published
	st.published, st.haveSignal = want, true
	switch {
	case want != (port.SaleState{}):
		w.log.Warn("doi trang thai het ve", "event", eventID,
			"stock_zero", want.StockZero, "sold_out", want.SoldOut, "active_holds", info.ActiveHolds)
	case prev != (port.SaleState{}):
		w.log.Info("kho co ve tro lai", "event", eventID)
	}
	return nil
}
