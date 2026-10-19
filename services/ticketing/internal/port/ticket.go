package port

import (
	"context"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
)

// OrderIssuanceItem dai dien cho mot hang ve va so luong trong don hang.
type OrderIssuanceItem struct {
	TicketTypeID string
	Quantity     int
	UnitCents    int64
}

// OrderIssuanceInfo chua thong tin day du ve don hang can phat hanh ve.
type OrderIssuanceInfo struct {
	OrderID    string
	EventID    string
	IdentityID string
	Status     string
	ExpiresAt  time.Time
	Items      []OrderIssuanceItem
}

// TicketRepository dinh nghia cac thao tac ben vung voi ve va don hang tren Postgres.
type TicketRepository interface {
	// GetOrderForIssuance doc thong tin don hang va cac order_items.
	GetOrderForIssuance(ctx context.Context, orderID string) (OrderIssuanceInfo, error)

	// IssueTicketsTx thuc thi trong CUNG MOT TRANSACTION:
	// 1. Khoa don hang FOR UPDATE de chong race condition phat ve song song.
	// 2. Kiem tra tinh hop le cua don hang (phai la PAID hoac da ISSUED).
	// 3. Neu don da ISSUED va da co ve, tra ve idempotent khong loi.
	// 4. Chen cac ban ghi ve vao bang tickets.
	// 5. Chuyen seat_holds thanh consumed_at = now() (BR-O7).
	// 6. Cap nhat orders.status = 'ISSUED'.
	// 7. Ghi outbox_events voi routing_key 'ticketing.ticket.issued' (Quy tac bat bien 1).
	IssueTicketsTx(ctx context.Context, order OrderIssuanceInfo, tickets []domain.Ticket, traceID string) (alreadyIssued bool, err error)

	// GetTicketsByOrderID tra ve danh sach cac ve da phat hanh cua mot don hang.
	GetTicketsByOrderID(ctx context.Context, orderID string) ([]domain.Ticket, error)

	// GetTicketByJTI tra ve thong tin ve dua vao JTI (AC-4, AC-5).
	GetTicketByJTI(ctx context.Context, jti string) (domain.Ticket, error)

	// UpdateTicketStatus cap nhat trang thai cua mot ve (vi du chuyen sang CHECKED_IN).
	UpdateTicketStatus(ctx context.Context, ticketID string, currentStatus, newStatus domain.TicketStatus, checkedInAt *time.Time) error
}

// QRSigner dinh nghia cong cu ky va xac thuc chu ky so HMAC cho QR code.
// *authx.QREngine thoa man giao dien nay.
type QRSigner interface {
	Sign(claims authx.TicketClaims) (qrPayload string, signature string, err error)
	Verify(qrPayload string) (authx.TicketClaims, error)
}

// IssueTicketsParams tham so dau vao de phat hanh ve cho mot don hang.
type IssueTicketsParams struct {
	OrderID string
	TraceID string
}

// IssueTicketsResult ket qua tra ve khi phat hanh ve thanh cong hoac idempotent.
type IssueTicketsResult struct {
	OrderID       string                `json:"order_id"`
	EventID       string                `json:"event_id"`
	IdentityID    string                `json:"identity_id"`
	Tickets       []domain.TicketWithQR `json:"tickets"`
	AlreadyIssued bool                  `json:"already_issued"`
}

// VerifyTicketParams tham so khi may quet hoac he thong gui ma QR vao de xac minh.
type VerifyTicketParams struct {
	QRPayload string
	TraceID   string
}

// VerifyTicketResult ket qua tham dinh ma QR tai cua soat ve.
type VerifyTicketResult struct {
	Valid   bool               `json:"valid"`
	Message string             `json:"message"`
	Ticket  domain.Ticket      `json:"ticket"`
	Claims  authx.TicketClaims `json:"claims"`
}
