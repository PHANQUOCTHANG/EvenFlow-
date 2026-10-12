package worker_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/port"
	"github.com/eventflow/eventflow/services/ticketing/internal/worker"
)

// Test seeder viet tu nghiep vu: Redis trong thi moi lan giu ghe bi bao het ve,
// nen ton kho phai duoc nap tu Postgres; va khong bao gio ghi de so Redis dang
// giu (BR-O1: Redis khong duoc phong len so voi su that).

type fakeSource struct {
	rows map[string][]port.InventoryRow
	errs map[string]error
}

func (f *fakeSource) ListInventory(_ context.Context, eventID string) ([]port.InventoryRow, error) {
	if err := f.errs[eventID]; err != nil {
		return nil, err
	}
	return f.rows[eventID], nil
}

type seedCall struct {
	event, ticketType string
	buckets           map[int]int
}

type fakeSeedSink struct {
	mu    sync.Mutex
	calls []seedCall
}

func (f *fakeSeedSink) SeedInventory(_ context.Context, eventID, tt string, b map[int]int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, seedCall{eventID, tt, b})
	return nil
}

func (f *fakeSeedSink) snapshot() []seedCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]seedCall(nil), f.calls...)
}

func runSeederOnce(t *testing.T, src *fakeSource, sink *fakeSeedSink, events []string) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := worker.SeederConfig{EventIDs: events, Interval: time.Hour}
	s := worker.NewInventorySeeder(src, sink, cfg, log)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	// Run nap ngay lap tuc truoc khi cho ticker; doi toi khi co ket qua hoac het gio.
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if len(sink.snapshot()) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run: muon context.Canceled, nhan %v", err)
	}
}

func TestSeeder_NapTonKhoTheoTungHangVe(t *testing.T) {
	src := &fakeSource{rows: map[string][]port.InventoryRow{
		"ev1": {
			{TicketTypeID: "std", Bucket: 0, Available: 10},
			{TicketTypeID: "std", Bucket: 1, Available: 7},
			{TicketTypeID: "vip", Bucket: 0, Available: 3},
		},
	}}
	sink := &fakeSeedSink{}

	runSeederOnce(t, src, sink, []string{"ev1"})

	got := map[string]map[int]int{}
	for _, c := range sink.snapshot() {
		if c.event != "ev1" {
			t.Fatalf("nap nham su kien %q", c.event)
		}
		got[c.ticketType] = c.buckets
	}
	if len(got) != 2 {
		t.Fatalf("muon 2 hang ve duoc nap, nhan %d: %v", len(got), got)
	}
	if got["std"][0] != 10 || got["std"][1] != 7 {
		t.Errorf("ton kho hang std sai: %v", got["std"])
	}
	if got["vip"][0] != 3 {
		t.Errorf("ton kho hang vip sai: %v", got["vip"])
	}
}

func TestSeeder_SuKienChuaCoTonKho_KhongNapGiCa(t *testing.T) {
	src := &fakeSource{rows: map[string][]port.InventoryRow{"ev1": nil}}
	sink := &fakeSeedSink{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := worker.NewInventorySeeder(src, sink, worker.SeederConfig{
		EventIDs: []string{"ev1"}, Interval: 10 * time.Millisecond,
	}, log)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	_ = s.Run(ctx)

	if n := len(sink.snapshot()); n != 0 {
		t.Fatalf("su kien chua ton tai khong duoc nap, nhung co %d lan goi", n)
	}
}

func TestSeeder_LoiMotSuKien_KhongChanSuKienKhac(t *testing.T) {
	src := &fakeSource{
		rows: map[string][]port.InventoryRow{
			"ev-ok": {{TicketTypeID: "std", Bucket: 0, Available: 5}},
		},
		errs: map[string]error{"ev-hong": errors.New("postgres chet")},
	}
	sink := &fakeSeedSink{}

	runSeederOnce(t, src, sink, []string{"ev-hong", "ev-ok"})

	calls := sink.snapshot()
	if len(calls) != 1 || calls[0].event != "ev-ok" {
		t.Fatalf("su kien lanh phai van duoc nap, nhan %+v", calls)
	}
}

func TestSeeder_LapLai_GoiLaiMoiVongDeTuLanhSauMatDuLieu(t *testing.T) {
	src := &fakeSource{rows: map[string][]port.InventoryRow{
		"ev1": {{TicketTypeID: "std", Bucket: 0, Available: 5}},
	}}
	sink := &fakeSeedSink{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := worker.NewInventorySeeder(src, sink, worker.SeederConfig{
		EventIDs: []string{"ev1"}, Interval: 10 * time.Millisecond,
	}, log)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	_ = s.Run(ctx)

	if n := len(sink.snapshot()); n < 3 {
		t.Fatalf("seeder phai lap lai moi chu ky de Redis tu lanh lai, chi nap %d lan", n)
	}
}
