package worker

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/eventflow/eventflow/libs/go/activeevents"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// ActiveEvents la danh sach su kien ma cac tien trinh nen phuc vu: ACTIVE_EVENT_IDS (co dinh) cong
// voi su kien EventDiscovery tim thay trong Postgres. An toan cho nhieu goroutine.
type ActiveEvents struct {
	mu      sync.RWMutex
	static  []string
	current []string
}

// NewActiveEvents tao danh sach voi phan co dinh.
func NewActiveEvents(static []string) *ActiveEvents {
	a := &ActiveEvents{static: activeevents.Union(static)}
	a.current = a.static
	return a
}

// List tra ban sao danh sach hien tai, da sap xep, khong trung.
func (a *ActiveEvents) List() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return slices.Clone(a.current)
}

// set thay phan tim thay; tra true neu danh sach doi.
func (a *ActiveEvents) set(found []string) bool {
	next := activeevents.Union(a.static, found)
	a.mu.Lock()
	defer a.mu.Unlock()
	if slices.Equal(next, a.current) {
		return false
	}
	a.current = next
	return true
}

// SellableEventSource doc su kien can phuc vu tu Postgres.
type SellableEventSource interface {
	ListSellableEvents(ctx context.Context, horizon time.Duration) ([]port.SellableEvent, error)
}

// ActiveEventsPublisher ghi danh sach + T0 sang Redis cho phong cho.
type ActiveEventsPublisher interface {
	PublishActiveEvents(ctx context.Context, events []port.SellableEvent, now time.Time) error
}

// EventDiscoveryConfig cau hinh vong quet.
type EventDiscoveryConfig struct {
	Interval time.Duration
	// Horizon: su kien SCHEDULED duoc dua vao truoc T0 bao lau (de phong cho mo LOBBY dung gio).
	Horizon time.Duration
}

// DefaultEventDiscoveryConfig: quet moi 5 giay, nhin truoc 24 gio.
func DefaultEventDiscoveryConfig() EventDiscoveryConfig {
	return EventDiscoveryConfig{Interval: 5 * time.Second, Horizon: 24 * time.Hour}
}

// EventDiscovery tim su kien dang/sap mo ban (EV-118): organizer duyet + mo ban xong thi sau toi da
// mot chu ky phong cho nhan khach, ticketing nap kho, tra hold het han va bao het ve, khong can sua
// ACTIVE_EVENT_IDS hay khoi dong lai.
type EventDiscovery struct {
	src    SellableEventSource
	pub    ActiveEventsPublisher
	active *ActiveEvents
	cfg    EventDiscoveryConfig
	log    *slog.Logger
	now    func() time.Time
}

// NewEventDiscovery tao vong quet.
func NewEventDiscovery(src SellableEventSource, pub ActiveEventsPublisher, active *ActiveEvents,
	cfg EventDiscoveryConfig, log *slog.Logger) *EventDiscovery {
	return &EventDiscovery{src: src, pub: pub, active: active, cfg: cfg, log: log, now: time.Now}
}

// Run quet ngay khi khoi dong roi theo chu ky.
func (d *EventDiscovery) Run(ctx context.Context) error {
	d.Discover(ctx)
	t := time.NewTicker(d.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			d.Discover(ctx)
		}
	}
}

// Discover chay mot vong. Doc Postgres loi thi GIU danh sach cu (khong xoa su kien dang ban vi mot lan
// mat ket noi) va khong ghi gi sang Redis.
func (d *EventDiscovery) Discover(ctx context.Context) {
	events, err := d.src.ListSellableEvents(ctx, d.cfg.Horizon)
	if err != nil {
		d.log.Error("quet su kien dang ban that bai, giu danh sach cu", "err", err)
		return
	}
	// Su kien co dinh (ACTIVE_EVENT_IDS) cung di vao tap Redis de controller thay; T0 cua chung do
	// seed.sh ghi, khong co trong events thi khong cham.
	found := make([]string, 0, len(events))
	for _, e := range events {
		found = append(found, e.ID)
	}
	if d.active.set(found) {
		d.log.Info("danh sach su kien dang ban doi", "su_kien", d.active.List())
	}
	all := make([]port.SellableEvent, 0, len(events))
	all = append(all, events...)
	seen := make(map[string]bool, len(events))
	for _, e := range events {
		seen[e.ID] = true
	}
	for _, id := range d.active.static {
		if !seen[id] {
			all = append(all, port.SellableEvent{ID: id})
		}
	}
	if err := d.pub.PublishActiveEvents(ctx, all, d.now()); err != nil {
		d.log.Error("ghi danh sach su kien sang Redis that bai", "err", err)
	}
}
