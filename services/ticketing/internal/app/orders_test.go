package app_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

type fakeOrders struct {
	view port.OrderView
	err  error
}

func (f *fakeOrders) GetOrder(_ context.Context, _, _ string) (port.OrderView, error) {
	return f.view, f.err
}

func (f *fakeOrders) OrderOwnedBy(context.Context, string, string) (bool, error) { return true, nil }

type fakeTicketLister struct {
	tickets []domain.Ticket
	calls   int
}

func (f *fakeTicketLister) GetTicketsByOrderID(context.Context, string) ([]domain.Ticket, error) {
	f.calls++
	return f.tickets, nil
}

func TestGetOrder_ChiDocVeKhiDonDaPhatHanh(t *testing.T) {
	lister := &fakeTicketLister{tickets: []domain.Ticket{{ID: "t1"}}}
	for status, wantTickets := range map[string]int{"HELD": 0, "PAID": 0, "ISSUED": 1, "CANCELLED": 0} {
		lister.calls = 0
		uc := app.NewGetOrder(&fakeOrders{view: port.OrderView{OrderID: "o1", Status: status}}, lister)
		d, err := uc.Execute(context.Background(), "o1", "u1")
		if err != nil || len(d.Tickets) != wantTickets {
			t.Fatalf("%s: %d ve, err %v, muon %d", status, len(d.Tickets), err, wantTickets)
		}
		if (status == "ISSUED") != (lister.calls == 1) {
			t.Fatalf("%s: doc ve %d lan", status, lister.calls)
		}
	}
}

func TestGetOrder_LoiNguonDuocTraNguyen(t *testing.T) {
	uc := app.NewGetOrder(&fakeOrders{err: domain.ErrOrderNotFound}, nil)
	if _, err := uc.Execute(context.Background(), "o1", "u1"); !errors.Is(err, domain.ErrOrderNotFound) {
		t.Fatalf("muon ErrOrderNotFound, nhan %v", err)
	}
}

type fakeCanceller struct {
	res   port.CancelResult
	err   error
	calls int
}

func (f *fakeCanceller) CancelHold(context.Context, string, string) (port.CancelResult, error) {
	f.calls++
	return f.res, f.err
}

type fakeReleaser struct {
	err      error
	released []string
}

func (f *fakeReleaser) Release(_ context.Context, eventID, holdID string) error {
	f.released = append(f.released, eventID+"/"+holdID)
	return f.err
}

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestCancelHold_PostgresTruocRedisSau(t *testing.T) {
	repo := &fakeCanceller{res: port.CancelResult{EventID: "e1", Cancelled: true}}
	gate := &fakeReleaser{}
	if err := app.NewCancelHold(repo, gate, discard()).Execute(context.Background(), "h1", "u1"); err != nil {
		t.Fatal(err)
	}
	if len(gate.released) != 1 || gate.released[0] != "e1/h1" {
		t.Fatalf("Redis phai duoc tra cho e1/h1, nhan %v", gate.released)
	}
}

// Redis luu hold bang hex 32 ky tu; client giu UUID co gach ngang (Postgres tra ve).
func TestCancelHold_DoiIdHoldSangDangRedis(t *testing.T) {
	gate := &fakeReleaser{}
	uc := app.NewCancelHold(&fakeCanceller{res: port.CancelResult{EventID: "e1"}}, gate, discard())
	if err := uc.Execute(context.Background(), "0A1B2C3D-4E5F-4A6B-8C7D-9E0F1A2B3C4D", "u1"); err != nil {
		t.Fatal(err)
	}
	if want := "e1/0a1b2c3d4e5f4a6b8c7d9e0f1a2b3c4d"; gate.released[0] != want {
		t.Fatalf("Redis nhan %q, muon %q", gate.released[0], want)
	}
}

// Lan huy truoc co the chet giua hai buoc: Postgres bao "da huy" (Cancelled=false) van phai tra Redis.
func TestCancelHold_DaHuyTruocDoVanTraRedis(t *testing.T) {
	gate := &fakeReleaser{}
	uc := app.NewCancelHold(&fakeCanceller{res: port.CancelResult{EventID: "e1", Cancelled: false}}, gate, discard())
	if err := uc.Execute(context.Background(), "h1", "u1"); err != nil || len(gate.released) != 1 {
		t.Fatalf("err %v, released %v", err, gate.released)
	}
}

func TestCancelHold_LoiPostgresKhongChamRedis(t *testing.T) {
	for _, e := range []error{domain.ErrOrderNotFound, domain.ErrHoldNotCancellable, errors.New("db down")} {
		gate := &fakeReleaser{}
		err := app.NewCancelHold(&fakeCanceller{err: e}, gate, discard()).Execute(context.Background(), "h1", "u1")
		if !errors.Is(err, e) || len(gate.released) != 0 {
			t.Fatalf("%v: err %v, released %v", e, err, gate.released)
		}
	}
}

func TestCancelHold_LoiRedisDuocBaoDeClientThuLai(t *testing.T) {
	gate := &fakeReleaser{err: errors.New("redis down")}
	err := app.NewCancelHold(&fakeCanceller{res: port.CancelResult{EventID: "e1", Cancelled: true}}, gate, discard()).
		Execute(context.Background(), "h1", "u1")
	if err == nil {
		t.Fatal("Redis loi phai duoc bao de client thu lai")
	}
}

type fakeAvail struct {
	tiers     []port.TierInfo
	remaining map[string]int
	tierErr   error
	stockErr  error
}

func (f *fakeAvail) ListTiers(context.Context, string) ([]port.TierInfo, error) {
	return f.tiers, f.tierErr
}

func (f *fakeAvail) Remaining(context.Context, string, []string) (map[string]int, error) {
	return f.remaining, f.stockErr
}

func TestGetAvailability_MucConVeKhongLoSoChinhXac(t *testing.T) {
	src := &fakeAvail{
		tiers: []port.TierInfo{
			{TicketTypeID: "vip", Name: "VIP", Capacity: 100},
			{TicketTypeID: "std", Name: "Standard", Capacity: 1000},
			{TicketTypeID: "eco", Name: "Economy", Capacity: 500},
			{TicketTypeID: "new", Name: "Chua nap Redis", Capacity: 50},
		},
		remaining: map[string]int{"vip": 4, "std": 700, "eco": 0},
	}
	got, err := app.NewGetAvailability(src).Execute(context.Background(), "e1")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]domain.AvailabilityLevel{
		"vip": domain.AvailabilityFewLeft, "std": domain.AvailabilityAvailable,
		"eco": domain.AvailabilitySoldOut, "new": domain.AvailabilitySoldOut, // chua nap = tin phia an toan
	}
	if len(got) != 4 {
		t.Fatalf("muon 4 hang ve, nhan %d", len(got))
	}
	for _, g := range got {
		if g.Availability != want[g.TicketTypeID] {
			t.Errorf("%s = %s, muon %s", g.TicketTypeID, g.Availability, want[g.TicketTypeID])
		}
	}
}

func TestGetAvailability_LoiNguon(t *testing.T) {
	if _, err := app.NewGetAvailability(&fakeAvail{tierErr: domain.ErrEventNotFound}).Execute(context.Background(), "e1"); !errors.Is(err, domain.ErrEventNotFound) {
		t.Fatalf("muon ErrEventNotFound, nhan %v", err)
	}
	src := &fakeAvail{tiers: []port.TierInfo{{TicketTypeID: "a", Capacity: 1}}, stockErr: errors.New("redis down")}
	if _, err := app.NewGetAvailability(src).Execute(context.Background(), "e1"); err == nil {
		t.Fatal("Redis loi phai bao loi, khong duoc tra 'con ve'")
	}
}
