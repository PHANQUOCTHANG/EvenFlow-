package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// TicketVerifier la use case kiem QR (VerifyTicketUseCase).
type TicketVerifier interface {
	Execute(ctx context.Context, in port.VerifyTicketParams) (port.VerifyTicketResult, error)
}

// TicketStatusUpdater doi trang thai ve co dieu kien (port.TicketRepository).
type TicketStatusUpdater interface {
	UpdateTicketStatus(ctx context.Context, ticketID string, current, next domain.TicketStatus, checkedInAt *time.Time) error
}

// CheckInTicketUseCase cho khach vao cua: kiem QR roi doi ISSUED -> CHECKED_IN dung mot lan (EV-135, BR-O8).
//
// Tinh dung mot lan nam o UPDATE co dieu kien (WHERE status = 'ISSUED'), khong nam o buoc kiem truoc:
// hai may quet cung luc ca hai deu thay ISSUED, nhung chi mot UPDATE doi duoc dong; may con lai nhan
// ErrTicketAlreadyUsed.
type CheckInTicketUseCase struct {
	verifier TicketVerifier
	tickets  TicketStatusUpdater
	log      *slog.Logger
	clock    func() time.Time
}

// NewCheckInTicketUseCase tao use case.
func NewCheckInTicketUseCase(verifier TicketVerifier, tickets TicketStatusUpdater, log *slog.Logger) *CheckInTicketUseCase {
	return &CheckInTicketUseCase{verifier: verifier, tickets: tickets, log: log, clock: time.Now}
}

// WithClock ghi de dong ho (test).
func (uc *CheckInTicketUseCase) WithClock(c func() time.Time) *CheckInTicketUseCase {
	if c != nil {
		uc.clock = c
	}
	return uc
}

// Execute kiem QR roi check-in. Loi tra ve cung kieu voi VerifyTicketUseCase (ErrTamperedQR,
// ErrTicketNotFound, ErrTicketAlreadyUsed, ...), nen handler dung chung bang anh xa loi.
func (uc *CheckInTicketUseCase) Execute(ctx context.Context, in port.VerifyTicketParams) (port.VerifyTicketResult, error) {
	res, err := uc.verifier.Execute(ctx, in)
	if err != nil {
		return res, err
	}
	now := uc.clock().UTC()
	if err := uc.tickets.UpdateTicketStatus(ctx, res.Ticket.ID, domain.TicketStatusIssued, domain.TicketStatusCheckedIn, &now); err != nil {
		if errors.Is(err, domain.ErrTicketAlreadyUsed) {
			// Thua cuoc dua voi may quet khac giua luc kiem va doi trang thai.
			uc.log.Warn("check-in: ve vua duoc may khac quet", "ticket_id", res.Ticket.ID, "trace_id", in.TraceID)
			return port.VerifyTicketResult{
				Valid:   false,
				Message: "ve da duoc su dung de vao cua truoc do",
				Ticket:  res.Ticket,
				Claims:  res.Claims,
			}, domain.ErrTicketAlreadyUsed
		}
		return port.VerifyTicketResult{}, err
	}
	res.Ticket.Status = domain.TicketStatusCheckedIn
	res.Ticket.CheckedInAt = &now
	res.Message = "check-in thanh cong"
	uc.log.Info("check-in thanh cong", "ticket_id", res.Ticket.ID, "order_id", res.Ticket.OrderID, "trace_id", in.TraceID)
	return res, nil
}
