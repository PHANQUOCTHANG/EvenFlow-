package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

type fakeHoldLister struct {
	ids []string
	err error
}

func (f fakeHoldLister) ListOrderHoldIDs(context.Context, string) ([]string, error) {
	return f.ids, f.err
}

type fakeConsumer struct {
	calls []string // "event/holdhex"
	err   error
}

func (f *fakeConsumer) Consume(_ context.Context, eventID, holdID string) error {
	f.calls = append(f.calls, eventID+"/"+holdID)
	return f.err
}

func paidOrderRepo(status string) *mockTicketRepo {
	return &mockTicketRepo{
		getOrderFn: func(context.Context, string) (port.OrderIssuanceInfo, error) {
			return port.OrderIssuanceInfo{
				OrderID: "o-1", EventID: "ev-1", IdentityID: "u-1", Status: status,
				Items: []port.OrderIssuanceItem{{TicketTypeID: "tt", Quantity: 1, UnitCents: 1}},
			}, nil
		},
		getTicketsByOrderFn: func(context.Context, string) ([]domain.Ticket, error) {
			return []domain.Ticket{{ID: "t-1", OrderID: "o-1", TicketTypeID: "tt", JTI: "j", CreatedAt: time.Now()}}, nil
		},
	}
}

// Sau khi phat ve, hold cua don duoc don khoi Redis bang id dang Redis (hex 32 ky tu, khong gach
// ngang) -- dung dinh dang Release dung, neu khong HDEL khong tim thay gi.
func TestIssueTickets_DonHoldRedisSauKhiPhatVe(t *testing.T) {
	gate := &fakeConsumer{}
	uc := app.NewIssueTicketsUseCase(paidOrderRepo("PAID"), testSigner(t), discardLogger()).
		WithHoldCleanup(fakeHoldLister{ids: []string{"3F2504E0-4F89-11D3-9A0C-0305E82C3301"}}, gate)
	if _, err := uc.Execute(context.Background(), port.IssueTicketsParams{OrderID: "o-1"}); err != nil {
		t.Fatal(err)
	}
	if len(gate.calls) != 1 || gate.calls[0] != "ev-1/3f2504e04f8911d39a0c0305e82c3301" {
		t.Fatalf("Consume = %v", gate.calls)
	}
}

// Don da ISSUED (goi lai / message giao lap): van don, de sua truong hop lan truoc chet giua Postgres va Redis.
func TestIssueTickets_DonHoldRedisCaKhiDonDaIssued(t *testing.T) {
	gate := &fakeConsumer{}
	uc := app.NewIssueTicketsUseCase(paidOrderRepo("ISSUED"), testSigner(t), discardLogger()).
		WithHoldCleanup(fakeHoldLister{ids: []string{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}}, gate)
	res, err := uc.Execute(context.Background(), port.IssueTicketsParams{OrderID: "o-1"})
	if err != nil || !res.AlreadyIssued {
		t.Fatalf("Execute = %+v, %v", res, err)
	}
	if len(gate.calls) != 1 {
		t.Fatalf("Consume = %v", gate.calls)
	}
}

// Don Redis chi la don dep: loi o do khong duoc lam hong viec phat ve da xong.
func TestIssueTickets_LoiDonRedisKhongLamHongPhatVe(t *testing.T) {
	for name, uc := range map[string]*app.IssueTicketsUseCase{
		"liet ke hold loi": app.NewIssueTicketsUseCase(paidOrderRepo("PAID"), testSigner(t), discardLogger()).
			WithHoldCleanup(fakeHoldLister{err: errors.New("db")}, &fakeConsumer{}),
		"consume loi": app.NewIssueTicketsUseCase(paidOrderRepo("PAID"), testSigner(t), discardLogger()).
			WithHoldCleanup(fakeHoldLister{ids: []string{"x"}}, &fakeConsumer{err: errors.New("redis down")}),
	} {
		res, err := uc.Execute(context.Background(), port.IssueTicketsParams{OrderID: "o-1"})
		if err != nil || len(res.Tickets) != 1 {
			t.Errorf("%s: Execute = %+v, %v; muon phat ve binh thuong", name, res, err)
		}
	}
}

// Khong cau hinh don dep thi hanh vi cu giu nguyen.
func TestIssueTickets_KhongCauHinhDonDepThiBoQua(t *testing.T) {
	uc := app.NewIssueTicketsUseCase(paidOrderRepo("PAID"), testSigner(t), discardLogger())
	if _, err := uc.Execute(context.Background(), port.IssueTicketsParams{OrderID: "o-1"}); err != nil {
		t.Fatal(err)
	}
}
