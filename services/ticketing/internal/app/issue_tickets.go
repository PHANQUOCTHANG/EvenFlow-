package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
	"github.com/google/uuid"
)

// IssueTicketsUseCase dieu phoi nghiep vu phat hanh ve cho don hang da thanh toan (AC-1, AC-2, AC-3).
type IssueTicketsUseCase struct {
	repo   port.TicketRepository
	signer port.QRSigner
	log    *slog.Logger
	clock  func() time.Time

	holds    OrderHoldLister // tuy chon: cung voi gate de don hold Redis sau khi phat ve
	holdGate HoldConsumer
}

// OrderHoldLister liet ke id hold (seat_holds) cua mot don.
type OrderHoldLister interface {
	ListOrderHoldIDs(ctx context.Context, orderID string) ([]string, error)
}

// HoldConsumer xoa hold khoi Redis KHONG tra kho (port.InventoryGate.Consume).
type HoldConsumer interface {
	Consume(ctx context.Context, eventID, holdID string) error
}

// NewIssueTicketsUseCase khoi tao use case phat hanh ve.
func NewIssueTicketsUseCase(repo port.TicketRepository, signer port.QRSigner, log *slog.Logger) *IssueTicketsUseCase {
	return &IssueTicketsUseCase{
		repo:   repo,
		signer: signer,
		log:    log,
		clock:  time.Now,
	}
}

// WithHoldCleanup bat viec don hold Redis cua don sau khi phat ve. Thieu buoc nay hold da tra tien van
// nam trong Redis toi khi het han (mac dinh 10 phut), nen: (1) tin hieu het ve (EV-146) tre ngan ay
// lan sau ve cuoi cung vi no dem moi hold con song, ke ca hold da thanh toan; (2) so hold dang hoat
// dong bi thoi phong.
func (uc *IssueTicketsUseCase) WithHoldCleanup(holds OrderHoldLister, gate HoldConsumer) *IssueTicketsUseCase {
	uc.holds, uc.holdGate = holds, gate
	return uc
}

// consumeHolds xoa hold cua don khoi Redis. Chi ghi log khi loi: ve da phat va Postgres da danh dau
// consumed, hold Redis con lai se tu het han; viec nay chay lai o moi lan goi issue-tickets/order.paid.
func (uc *IssueTicketsUseCase) consumeHolds(ctx context.Context, orderID, eventID string) {
	if uc.holds == nil || uc.holdGate == nil {
		return
	}
	ids, err := uc.holds.ListOrderHoldIDs(ctx, orderID)
	if err != nil {
		uc.log.Warn("khong liet ke duoc hold de don Redis sau khi phat ve", "order_id", orderID, "err", err)
		return
	}
	for _, id := range ids {
		if err := uc.holdGate.Consume(ctx, eventID, redisHoldID(id)); err != nil {
			uc.log.Warn("don hold Redis sau khi phat ve that bai", "order_id", orderID, "hold", id, "err", err)
		}
	}
}

// WithClock cho phep ghi de ham dong ho de phuc vu unit test.
func (uc *IssueTicketsUseCase) WithClock(clock func() time.Time) *IssueTicketsUseCase {
	if clock != nil {
		uc.clock = clock
	}
	return uc
}

// Execute thuc thi quy trinh phat hanh ve:
// 1. Kiem tra don hang ton tai va co trang thai PAID (hoac ISSUED).
// 2. Tinh toan so luong ve can tao tu tung order_items.
// 3. Sinh JTI va ky HMAC-SHA256 cho tung ve bang signer.
// 4. Thuc thi ghi nhan ve, doi status va outbox trong CUNG MOT TRANSACTION.
// 5. Dam bao Idempotent: neu don da cap ve thi tra ve danh sach cu khong tao trung.
func (uc *IssueTicketsUseCase) Execute(ctx context.Context, in port.IssueTicketsParams) (port.IssueTicketsResult, error) {
	if in.OrderID == "" {
		return port.IssueTicketsResult{}, fmt.Errorf("issue_tickets: thieu order_id")
	}

	order, err := uc.repo.GetOrderForIssuance(ctx, in.OrderID)
	if err != nil {
		return port.IssueTicketsResult{}, err
	}

	// Neu don hang da o trang thai ISSUED -> doc lai danh sach ve da cap (Idempotent AC-3)
	if order.Status == string(domain.TicketStatusIssued) {
		existingTickets, err := uc.repo.GetTicketsByOrderID(ctx, in.OrderID)
		if err != nil {
			return port.IssueTicketsResult{}, fmt.Errorf("issue_tickets: doc ve da cap that bai: %w", err)
		}
		if len(existingTickets) > 0 {
			resTickets := make([]domain.TicketWithQR, 0, len(existingTickets))
			for _, t := range existingTickets {
				// Tai tao QR payload bang cach ky lai claims
				claims := authx.TicketClaims{
					TicketID:     t.ID,
					OrderID:      t.OrderID,
					EventID:      order.EventID,
					TicketTypeID: t.TicketTypeID,
					IdentityID:   order.IdentityID,
					JTI:          t.JTI,
					IssuedAt:     t.CreatedAt.Unix(),
				}
				if !order.ExpiresAt.IsZero() {
					claims.ExpiresAt = order.ExpiresAt.Unix()
				}
				qrPayload, _, _ := uc.signer.Sign(claims)
				resTickets = append(resTickets, domain.TicketWithQR{
					Ticket:    t,
					QRPayload: qrPayload,
				})
			}

			uc.consumeHolds(ctx, in.OrderID, order.EventID)
			uc.log.Info("don hang da phat hanh ve truoc do, tra ve danh sach ve idempotent (AC-3)",
				"order_id", in.OrderID,
				"ticket_count", len(resTickets),
				"trace_id", in.TraceID,
			)

			return port.IssueTicketsResult{
				OrderID:       in.OrderID,
				EventID:       order.EventID,
				IdentityID:    order.IdentityID,
				Tickets:       resTickets,
				AlreadyIssued: true,
			}, nil
		}
	}

	// Bat buoc don hang phai la PAID moi duoc phat hanh ve (AC-1)
	if order.Status != "PAID" {
		uc.log.Warn("tu choi phat hanh ve: don hang chua o trang thai PAID",
			"order_id", in.OrderID,
			"current_status", order.Status,
			"trace_id", in.TraceID,
		)
		return port.IssueTicketsResult{}, domain.ErrOrderNotPaid
	}

	now := uc.clock().UTC()
	var domainTickets []domain.Ticket
	var resTickets []domain.TicketWithQR

	// Duyet qua tung order_item de sinh so luong ve tuong ung (AC-1)
	for _, item := range order.Items {
		for i := 0; i < item.Quantity; i++ {
			ticketID := uuid.New().String()
			jti := domain.NewJTI()

			claims := authx.TicketClaims{
				TicketID:     ticketID,
				OrderID:      order.OrderID,
				EventID:      order.EventID,
				TicketTypeID: item.TicketTypeID,
				IdentityID:   order.IdentityID,
				JTI:          jti,
				IssuedAt:     now.Unix(),
			}
			if !order.ExpiresAt.IsZero() {
				claims.ExpiresAt = order.ExpiresAt.Unix()
			}

			qrPayload, signature, signErr := uc.signer.Sign(claims)
			if signErr != nil {
				return port.IssueTicketsResult{}, fmt.Errorf("issue_tickets: ky QR cho ve that bai: %w", signErr)
			}

			t := domain.Ticket{
				ID:           ticketID,
				OrderID:      order.OrderID,
				TicketTypeID: item.TicketTypeID,
				JTI:          jti,
				QRSig:        signature,
				Status:       domain.TicketStatusIssued,
				CreatedAt:    now,
			}

			domainTickets = append(domainTickets, t)
			resTickets = append(resTickets, domain.TicketWithQR{
				Ticket:    t,
				QRPayload: qrPayload,
			})
		}
	}

	if len(domainTickets) == 0 {
		return port.IssueTicketsResult{}, fmt.Errorf("issue_tickets: don hang khong co order_items de phat ve")
	}

	// Luu vao database trong CUNG MOT TRANSACTION an toan (AC-6, Quy tac bat bien 1)
	alreadyIssued, err := uc.repo.IssueTicketsTx(ctx, order, domainTickets, in.TraceID)
	if err != nil {
		uc.log.Error("thuc thi IssueTicketsTx that bai",
			"order_id", in.OrderID,
			"err", err,
			"trace_id", in.TraceID,
		)
		return port.IssueTicketsResult{}, err
	}

	if alreadyIssued {
		uc.log.Info("phat hien don hang vua duoc cap boi tien trinh khac, doc lai danh sach ve",
			"order_id", in.OrderID,
			"trace_id", in.TraceID,
		)
		existing, gErr := uc.repo.GetTicketsByOrderID(ctx, in.OrderID)
		if gErr == nil && len(existing) > 0 {
			resTickets = make([]domain.TicketWithQR, 0, len(existing))
			for _, t := range existing {
				claims := authx.TicketClaims{
					TicketID:     t.ID,
					OrderID:      t.OrderID,
					EventID:      order.EventID,
					TicketTypeID: t.TicketTypeID,
					IdentityID:   order.IdentityID,
					JTI:          t.JTI,
					IssuedAt:     t.CreatedAt.Unix(),
				}
				if !order.ExpiresAt.IsZero() {
					claims.ExpiresAt = order.ExpiresAt.Unix()
				}
				qrPayload, _, _ := uc.signer.Sign(claims)
				resTickets = append(resTickets, domain.TicketWithQR{
					Ticket:    t,
					QRPayload: qrPayload,
				})
			}
		}
	} else {
		uc.log.Info("phat hanh ve thanh cong cho don hang PAID",
			"order_id", in.OrderID,
			"ticket_count", len(resTickets),
			"trace_id", in.TraceID,
		)
	}

	uc.consumeHolds(ctx, in.OrderID, order.EventID)
	return port.IssueTicketsResult{
		OrderID:       in.OrderID,
		EventID:       order.EventID,
		IdentityID:    order.IdentityID,
		Tickets:       resTickets,
		AlreadyIssued: alreadyIssued,
	}, nil
}
