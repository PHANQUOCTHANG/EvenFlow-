package worker_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/port"
	"github.com/eventflow/eventflow/services/ticketing/internal/worker"
)

type fakeSellable struct {
	events []port.SellableEvent
	err    error
}

func (f *fakeSellable) ListSellableEvents(context.Context, time.Duration) ([]port.SellableEvent, error) {
	return f.events, f.err
}

type fakeActivePub struct {
	calls [][]port.SellableEvent
	err   error
}

func (f *fakeActivePub) PublishActiveEvents(_ context.Context, ev []port.SellableEvent, _ time.Time) error {
	f.calls = append(f.calls, slices.Clone(ev))
	return f.err
}

func newDiscovery(src *fakeSellable, pub *fakeActivePub, active *worker.ActiveEvents) *worker.EventDiscovery {
	return worker.NewEventDiscovery(src, pub, active, worker.DefaultEventDiscoveryConfig(),
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// Su kien moi trong Postgres vao danh sach cua moi tien trinh nen, cong voi ACTIVE_EVENT_IDS.
func TestDiscovery_ThemSuKienMoiCongVoiDanhSachCoDinh(t *testing.T) {
	t0 := time.Date(2026, 10, 30, 20, 0, 0, 0, time.UTC)
	src := &fakeSellable{events: []port.SellableEvent{{ID: "ev-new", SaleStartAt: t0}}}
	pub := &fakeActivePub{}
	active := worker.NewActiveEvents([]string{"ev-smoke"})

	newDiscovery(src, pub, active).Discover(context.Background())

	if got := active.List(); !slices.Equal(got, []string{"ev-new", "ev-smoke"}) {
		t.Fatalf("List = %v", got)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("Publish goi %d lan", len(pub.calls))
	}
	got := pub.calls[0]
	if len(got) != 2 || got[0] != (port.SellableEvent{ID: "ev-new", SaleStartAt: t0}) || got[1] != (port.SellableEvent{ID: "ev-smoke"}) {
		t.Errorf("publish = %+v; su kien co dinh phai co mat, khong T0", got)
	}
}

// Su kien ket thuc/huy roi khoi danh sach o vong sau; su kien co dinh thi khong bao gio roi.
func TestDiscovery_SuKienHetBanRoiDanhSach(t *testing.T) {
	src := &fakeSellable{events: []port.SellableEvent{{ID: "a"}, {ID: "b"}}}
	active := worker.NewActiveEvents([]string{"fixed"})
	d := newDiscovery(src, &fakeActivePub{}, active)
	d.Discover(context.Background())

	src.events = []port.SellableEvent{{ID: "b"}}
	d.Discover(context.Background())
	if got := active.List(); !slices.Equal(got, []string{"b", "fixed"}) {
		t.Fatalf("List = %v, muon [b fixed]", got)
	}
}

// Mot lan mat ket noi Postgres khong duoc xoa su kien dang ban khoi phong cho.
func TestDiscovery_PostgresLoiGiuDanhSachCuKhongGhiRedis(t *testing.T) {
	src := &fakeSellable{events: []port.SellableEvent{{ID: "a"}}}
	pub := &fakeActivePub{}
	active := worker.NewActiveEvents(nil)
	d := newDiscovery(src, pub, active)
	d.Discover(context.Background())

	src.err = errors.New("connection refused")
	src.events = nil
	d.Discover(context.Background())
	if got := active.List(); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("List = %v, muon giu [a]", got)
	}
	if len(pub.calls) != 1 {
		t.Errorf("Postgres loi van ghi Redis (%d lan)", len(pub.calls))
	}
}

// Redis loi: danh sach trong worker van cap nhat (worker van tra kho/nap kho duoc).
func TestDiscovery_RedisLoiVanCapNhatDanhSachTrongWorker(t *testing.T) {
	src := &fakeSellable{events: []port.SellableEvent{{ID: "a"}}}
	active := worker.NewActiveEvents(nil)
	newDiscovery(src, &fakeActivePub{err: errors.New("redis down")}, active).Discover(context.Background())
	if got := active.List(); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("List = %v", got)
	}
}

// List tra ban sao: nguoi goi sua khong lam hong danh sach dung chung.
func TestActiveEvents_ListLaBanSao(t *testing.T) {
	active := worker.NewActiveEvents([]string{"b", "a", "a"})
	l := active.List()
	if !slices.Equal(l, []string{"a", "b"}) {
		t.Fatalf("List = %v", l)
	}
	l[0] = "hong"
	if active.List()[0] != "a" {
		t.Error("sua ban tra ve lam doi danh sach")
	}
}

// Seeder (dai dien cho 4 tien trinh dung SweeperConfig/SeederConfig/... .Events) doc danh sach dong o
// MOI vong quet: su kien EventDiscovery them sau khi worker chay van duoc nap kho.
func TestSeeder_DungDanhSachDongMoiVong(t *testing.T) {
	src := &fakeSource{rows: map[string][]port.InventoryRow{
		"ev-new": {{TicketTypeID: "tt", Bucket: 0, Available: 5}},
	}}
	sink := &fakeSeedSink{}
	active := worker.NewActiveEvents(nil)
	cfg := worker.SeederConfig{EventIDs: []string{"bo-qua"}, Events: active.List, Interval: 10 * time.Millisecond}
	s := worker.NewInventorySeeder(src, sink, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	time.Sleep(30 * time.Millisecond)
	if n := len(sink.snapshot()); n != 0 {
		t.Fatalf("chua co su kien nao ma da nap %d lan (Events phai thay EventIDs)", n)
	}

	newDiscovery(&fakeSellable{events: []port.SellableEvent{{ID: "ev-new"}}}, &fakeActivePub{}, active).
		Discover(context.Background())
	deadline := time.Now().Add(2 * time.Second)
	for len(sink.snapshot()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	calls := sink.snapshot()
	if len(calls) == 0 || calls[0].event != "ev-new" {
		t.Fatalf("su kien moi khong duoc nap kho: %+v", calls)
	}
}
