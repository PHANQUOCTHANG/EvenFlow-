package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// VerifyTicketUseCase dieu phoi quy trinh xac thuc chu ky QR va kiem tra trang thai ve (AC-4, AC-5).
type VerifyTicketUseCase struct {
	repo   port.TicketRepository
	signer port.QRSigner
	log    *slog.Logger
	clock  func() time.Time
}

// NewVerifyTicketUseCase khoi tao use case xac thuc ve.
func NewVerifyTicketUseCase(repo port.TicketRepository, signer port.QRSigner, log *slog.Logger) *VerifyTicketUseCase {
	return &VerifyTicketUseCase{
		repo:   repo,
		signer: signer,
		log:    log,
		clock:  time.Now,
	}
}

// WithClock cho phep tuy bien dong ho kiem thu.
func (uc *VerifyTicketUseCase) WithClock(clock func() time.Time) *VerifyTicketUseCase {
	if clock != nil {
		uc.clock = clock
	}
	return uc
}

// Execute thuc hien kiem tra:
// 1. Giai ma va xac thuc chu ky HMAC-SHA256 cua chuoi QR (AC-4).
// 2. Kiem tra han su dung cua ve.
// 3. Tra cuu ban ghi ve trong co so du lieu theo JTI (BR-O8).
// 4. Kiem tra tinh toan ven giua claims va du lieu DB.
// 5. Kiem tra trang thai ve (tu choi neu CHECKED_IN, VOID, REFUNDED - AC-5).
func (uc *VerifyTicketUseCase) Execute(ctx context.Context, in port.VerifyTicketParams) (port.VerifyTicketResult, error) {
	if in.QRPayload == "" {
		return port.VerifyTicketResult{
			Valid:   false,
			Message: "chuoi QR khong duoc de trong",
		}, fmt.Errorf("verify_ticket: chuoi QR rong")
	}

	// 1. Xac thuc chu ky HMAC-SHA256 (AC-4)
	claims, err := uc.signer.Verify(in.QRPayload)
	if err != nil {
		if errors.Is(err, authx.ErrExpiredQR) {
			uc.log.Warn("tu choi ve: ma QR da het han su dung",
				"trace_id", in.TraceID,
			)
			return port.VerifyTicketResult{
				Valid:   false,
				Message: "ma QR da het han su dung",
				Claims:  claims,
			}, domain.ErrTicketExpired
		}

		uc.log.Warn("tu choi ve: ma QR gia mao hoac chu ky khong hop le (AC-4)",
			"err", err,
			"trace_id", in.TraceID,
		)
		return port.VerifyTicketResult{
			Valid:   false,
			Message: "ma QR gia mao hoac chu ky khong hop le",
		}, domain.ErrTamperedQR
	}

	// 2. Tra cuu ve theo JTI (AC-5, BR-O8)
	ticket, err := uc.repo.GetTicketByJTI(ctx, claims.JTI)
	if err != nil {
		if errors.Is(err, domain.ErrTicketNotFound) {
			uc.log.Warn("tu choi ve: khong tim thay JTI trong co so du lieu",
				"jti", claims.JTI,
				"trace_id", in.TraceID,
			)
			return port.VerifyTicketResult{
				Valid:   false,
				Message: "khong tim thay ve trong he thong",
				Claims:  claims,
			}, domain.ErrTicketNotFound
		}

		uc.log.Error("tra cuu ve theo JTI that bai",
			"jti", claims.JTI,
			"err", err,
			"trace_id", in.TraceID,
		)
		return port.VerifyTicketResult{}, err
	}

	// 3. Kiem tra tinh toan ven giua claims va ticket trong DB
	if ticket.ID != claims.TicketID || ticket.OrderID != claims.OrderID {
		uc.log.Warn("tu choi ve: du lieu claims khong khop voi ban ghi ve trong DB (du lieu bi sua doi)",
			"claim_ticket_id", claims.TicketID,
			"db_ticket_id", ticket.ID,
			"jti", claims.JTI,
			"trace_id", in.TraceID,
		)
		return port.VerifyTicketResult{
			Valid:   false,
			Message: "du lieu ve khong khop voi he thong",
			Ticket:  ticket,
			Claims:  claims,
		}, domain.ErrTamperedQR
	}

	// 4. Kiem tra trang thai ve (AC-5)
	if admitErr := ticket.CanAdmit(); admitErr != nil {
		uc.log.Warn("tu choi vao cua: ve khong o trang thai ISSUED (da su dung / huy / hoan tien)",
			"ticket_id", ticket.ID,
			"status", ticket.Status,
			"checked_in_at", ticket.CheckedInAt,
			"trace_id", in.TraceID,
		)

		var msg string
		switch {
		case errors.Is(admitErr, domain.ErrTicketAlreadyUsed):
			msg = "ve da duoc su dung de vao cua truoc do"
		case errors.Is(admitErr, domain.ErrTicketVoid):
			msg = "ve da bi vo hieu hoa"
		case errors.Is(admitErr, domain.ErrTicketRefunded):
			msg = "ve da duoc hoan tien"
		default:
			msg = "ve khong hop le de vao cua"
		}

		return port.VerifyTicketResult{
			Valid:   false,
			Message: msg,
			Ticket:  ticket,
			Claims:  claims,
		}, admitErr
	}

	uc.log.Info("xac thuc ve thanh cong, duoc phep vao cua (BR-O8)",
		"ticket_id", ticket.ID,
		"order_id", ticket.OrderID,
		"jti", ticket.JTI,
		"trace_id", in.TraceID,
	)

	return port.VerifyTicketResult{
		Valid:   true,
		Message: "ve hop le, duoc phep vao cua",
		Ticket:  ticket,
		Claims:  claims,
	}, nil
}
