package app_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

type mockTicketRepo struct {
	getOrderFn          func(ctx context.Context, orderID string) (port.OrderIssuanceInfo, error)
	issueTicketsTxFn    func(ctx context.Context, order port.OrderIssuanceInfo, tickets []domain.Ticket, traceID string) (bool, error)
	getTicketsByOrderFn func(ctx context.Context, orderID string) ([]domain.Ticket, error)
	getTicketByJTIFn    func(ctx context.Context, jti string) (domain.Ticket, error)
	updateStatusFn      func(ctx context.Context, ticketID string, currentStatus, newStatus domain.TicketStatus, checkedInAt *time.Time) error
}

func (m *mockTicketRepo) GetOrderForIssuance(ctx context.Context, orderID string) (port.OrderIssuanceInfo, error) {
	if m.getOrderFn != nil {
		return m.getOrderFn(ctx, orderID)
	}
	return port.OrderIssuanceInfo{}, nil
}

func (m *mockTicketRepo) IssueTicketsTx(ctx context.Context, order port.OrderIssuanceInfo, tickets []domain.Ticket, traceID string) (bool, error) {
	if m.issueTicketsTxFn != nil {
		return m.issueTicketsTxFn(ctx, order, tickets, traceID)
	}
	return false, nil
}

func (m *mockTicketRepo) GetTicketsByOrderID(ctx context.Context, orderID string) ([]domain.Ticket, error) {
	if m.getTicketsByOrderFn != nil {
		return m.getTicketsByOrderFn(ctx, orderID)
	}
	return nil, nil
}

func (m *mockTicketRepo) GetTicketByJTI(ctx context.Context, jti string) (domain.Ticket, error) {
	if m.getTicketByJTIFn != nil {
		return m.getTicketByJTIFn(ctx, jti)
	}
	return domain.Ticket{}, nil
}

func (m *mockTicketRepo) UpdateTicketStatus(ctx context.Context, ticketID string, currentStatus, newStatus domain.TicketStatus, checkedInAt *time.Time) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, ticketID, currentStatus, newStatus, checkedInAt)
	}
	return nil
}

func testSigner(t *testing.T) port.QRSigner {
	signer, err := authx.NewQREngine([]byte("super-secret-test-key-32-bytes!"))
	if err != nil {
		t.Fatalf("tao test signer that bai: %v", err)
	}
	return signer
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestIssueTickets_Success_MultipleItems(t *testing.T) {
	signer := testSigner(t)
	log := discardLogger()

	orderID := "order-123"
	eventID := "event-456"
	identityID := "user-789"

	var savedTickets []domain.Ticket

	repo := &mockTicketRepo{
		getOrderFn: func(ctx context.Context, oID string) (port.OrderIssuanceInfo, error) {
			if oID != orderID {
				return port.OrderIssuanceInfo{}, domain.ErrOrderNotFound
			}
			return port.OrderIssuanceInfo{
				OrderID:    orderID,
				EventID:    eventID,
				IdentityID: identityID,
				Status:     "PAID",
				Items: []port.OrderIssuanceItem{
					{TicketTypeID: "type-standard", Quantity: 2, UnitCents: 50000},
					{TicketTypeID: "type-vip", Quantity: 1, UnitCents: 150000},
				},
			}, nil
		},
		issueTicketsTxFn: func(ctx context.Context, order port.OrderIssuanceInfo, tickets []domain.Ticket, traceID string) (bool, error) {
			savedTickets = tickets
			return false, nil
		},
	}

	uc := app.NewIssueTicketsUseCase(repo, signer, log)
	res, err := uc.Execute(context.Background(), port.IssueTicketsParams{
		OrderID: orderID,
		TraceID: "trace-abc",
	})
	if err != nil {
		t.Fatalf("phat hanh ve that bai: %v", err)
	}

	// Tong so ve: 2 + 1 = 3
	if len(res.Tickets) != 3 {
		t.Fatalf("ky vong 3 ve, nhan duoc: %d", len(res.Tickets))
	}
	if len(savedTickets) != 3 {
		t.Fatalf("ky vong 3 ve duoc luu vao DB, nhan duoc: %d", len(savedTickets))
	}
	if res.AlreadyIssued {
		t.Fatal("ky vong already_issued la false")
	}

	// Kiem tra tung ve
	jtiSet := make(map[string]bool)
	typeCounts := make(map[string]int)

	for _, twq := range res.Tickets {
		if twq.OrderID != orderID {
			t.Errorf("OrderID khong khop: %s", twq.OrderID)
		}
		if twq.Status != domain.TicketStatusIssued {
			t.Errorf("trang thai khong phai ISSUED: %s", twq.Status)
		}
		if twq.QRSig == "" {
			t.Errorf("QRSig rong cho ve: %s", twq.ID)
		}
		if twq.QRPayload == "" {
			t.Errorf("QRPayload rong cho ve: %s", twq.ID)
		}

		// Verify chuoi QR payload
		claims, vErr := signer.Verify(twq.QRPayload)
		if vErr != nil {
			t.Fatalf("verify QR sinh ra that bai: %v", vErr)
		}
		if claims.TicketID != twq.ID || claims.JTI != twq.JTI {
			t.Errorf("claims khong khop voi ban ghi ve")
		}

		jtiSet[twq.JTI] = true
		typeCounts[twq.TicketTypeID]++
	}

	// JTIs phai duy nhat
	if len(jtiSet) != 3 {
		t.Errorf("JTIs bi trung lap: %v", jtiSet)
	}
	if typeCounts["type-standard"] != 2 || typeCounts["type-vip"] != 1 {
		t.Errorf("so luong theo hang ve khong dung: %v", typeCounts)
	}
}

func TestIssueTickets_OrderNotPaid_Rejected(t *testing.T) {
	signer := testSigner(t)
	log := discardLogger()

	nonPaidStatuses := []string{"HELD", "PAYMENT_PENDING", "CANCELLED", "EXPIRED"}

	for _, st := range nonPaidStatuses {
		repo := &mockTicketRepo{
			getOrderFn: func(ctx context.Context, oID string) (port.OrderIssuanceInfo, error) {
				return port.OrderIssuanceInfo{
					OrderID: oID,
					Status:  st,
					Items:   []port.OrderIssuanceItem{{TicketTypeID: "type-1", Quantity: 1}},
				}, nil
			},
		}

		uc := app.NewIssueTicketsUseCase(repo, signer, log)
		_, err := uc.Execute(context.Background(), port.IssueTicketsParams{
			OrderID: "order-pending",
		})
		if !errors.Is(err, domain.ErrOrderNotPaid) {
			t.Errorf("status %s: ky vong ErrOrderNotPaid, nhan duoc: %v", st, err)
		}
	}
}

func TestIssueTickets_Idempotent_AlreadyIssued(t *testing.T) {
	signer := testSigner(t)
	log := discardLogger()

	orderID := "order-already-done"
	existing := []domain.Ticket{
		{
			ID:           "ticket-1",
			OrderID:      orderID,
			TicketTypeID: "type-1",
			JTI:          "jti-1",
			QRSig:        "sig-1",
			Status:       domain.TicketStatusIssued,
			CreatedAt:    time.Now().Add(-10 * time.Minute),
		},
	}

	repo := &mockTicketRepo{
		getOrderFn: func(ctx context.Context, oID string) (port.OrderIssuanceInfo, error) {
			return port.OrderIssuanceInfo{
				OrderID:    orderID,
				EventID:    "event-1",
				IdentityID: "user-1",
				Status:     "ISSUED",
			}, nil
		},
		getTicketsByOrderFn: func(ctx context.Context, oID string) ([]domain.Ticket, error) {
			return existing, nil
		},
	}

	uc := app.NewIssueTicketsUseCase(repo, signer, log)
	res, err := uc.Execute(context.Background(), port.IssueTicketsParams{OrderID: orderID})
	if err != nil {
		t.Fatalf("goi idempotent that bai: %v", err)
	}

	if !res.AlreadyIssued {
		t.Fatal("ky vong already_issued la true")
	}
	if len(res.Tickets) != 1 {
		t.Fatalf("ky vong 1 ve da cap, got %d", len(res.Tickets))
	}
	if res.Tickets[0].ID != "ticket-1" {
		t.Errorf("ve tra ve khong khop: %s", res.Tickets[0].ID)
	}
}

func TestIssueTickets_EmptyItems_Rejected(t *testing.T) {
	signer := testSigner(t)
	log := discardLogger()

	repo := &mockTicketRepo{
		getOrderFn: func(ctx context.Context, oID string) (port.OrderIssuanceInfo, error) {
			return port.OrderIssuanceInfo{
				OrderID: oID,
				Status:  "PAID",
				Items:   nil,
			}, nil
		},
	}

	uc := app.NewIssueTicketsUseCase(repo, signer, log)
	_, err := uc.Execute(context.Background(), port.IssueTicketsParams{OrderID: "order-empty"})
	if err == nil {
		t.Fatal("ky vong loi khi order khong co items")
	}
}
