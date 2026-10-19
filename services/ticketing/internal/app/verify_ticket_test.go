package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

func TestVerifyTicket_Success(t *testing.T) {
	signer := testSigner(t)
	log := discardLogger()

	ticketID := "ticket-uuid-1"
	orderID := "order-uuid-1"
	eventID := "event-uuid-1"
	ticketTypeID := "type-uuid-1"
	jti := "jti-unique-test-1"

	claims := authx.TicketClaims{
		TicketID:     ticketID,
		OrderID:      orderID,
		EventID:      eventID,
		TicketTypeID: ticketTypeID,
		JTI:          jti,
		IssuedAt:     time.Now().Unix(),
		ExpiresAt:    time.Now().Add(24 * time.Hour).Unix(),
	}

	qrPayload, sig, err := signer.Sign(claims)
	if err != nil {
		t.Fatal(err)
	}

	repo := &mockTicketRepo{
		getTicketByJTIFn: func(ctx context.Context, key string) (domain.Ticket, error) {
			if key != jti {
				return domain.Ticket{}, domain.ErrTicketNotFound
			}
			return domain.Ticket{
				ID:           ticketID,
				OrderID:      orderID,
				TicketTypeID: ticketTypeID,
				JTI:          jti,
				QRSig:        sig,
				Status:       domain.TicketStatusIssued,
				CreatedAt:    time.Now(),
			}, nil
		},
	}

	uc := app.NewVerifyTicketUseCase(repo, signer, log)
	res, err := uc.Execute(context.Background(), port.VerifyTicketParams{
		QRPayload: qrPayload,
		TraceID:   "trace-verify",
	})
	if err != nil {
		t.Fatalf("xac thuc ve hop le that bai: %v", err)
	}

	if !res.Valid {
		t.Fatalf("ky vong res.Valid la true, got false (message=%s)", res.Message)
	}
	if res.Ticket.ID != ticketID {
		t.Errorf("TicketID khong dung: want %s, got %s", ticketID, res.Ticket.ID)
	}
	if res.Claims.JTI != jti {
		t.Errorf("JTI khong dung: want %s, got %s", jti, res.Claims.JTI)
	}
}

func TestVerifyTicket_TamperedQR_Rejected(t *testing.T) {
	signer := testSigner(t)
	log := discardLogger()

	repo := &mockTicketRepo{}
	uc := app.NewVerifyTicketUseCase(repo, signer, log)

	// Tampered QR
	_, err := uc.Execute(context.Background(), port.VerifyTicketParams{
		QRPayload: "fake-payload.invalid-signature",
	})
	if !errors.Is(err, domain.ErrTamperedQR) {
		t.Fatalf("ky vong ErrTamperedQR cho QR gia mao, got: %v", err)
	}
}

func TestVerifyTicket_ExpiredQR_Rejected(t *testing.T) {
	baseTime := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)
	rawSigner, _ := authx.NewQREngine([]byte("super-secret-test-key-32-bytes!"))
	rawSigner.WithClock(func() time.Time { return baseTime })

	claims := authx.TicketClaims{
		TicketID:  "tid-1",
		OrderID:   "oid-1",
		EventID:   "eid-1",
		JTI:       "jti-1",
		IssuedAt:  baseTime.Unix(),
		ExpiresAt: baseTime.Add(1 * time.Hour).Unix(),
	}

	qrPayload, _, _ := rawSigner.Sign(claims)

	repo := &mockTicketRepo{}
	uc := app.NewVerifyTicketUseCase(repo, rawSigner, discardLogger())
	// 2 tieng sau -> het han
	uc.WithClock(func() time.Time { return baseTime.Add(2 * time.Hour) })
	rawSigner.WithClock(func() time.Time { return baseTime.Add(2 * time.Hour) })

	_, err := uc.Execute(context.Background(), port.VerifyTicketParams{
		QRPayload: qrPayload,
	})
	if !errors.Is(err, domain.ErrTicketExpired) {
		t.Fatalf("ky vong ErrTicketExpired khi QR qua han, got: %v", err)
	}
}

func TestVerifyTicket_NotFoundInDB_Rejected(t *testing.T) {
	signer := testSigner(t)
	claims := authx.TicketClaims{
		TicketID: "tid-not-found",
		OrderID:  "oid-1",
		EventID:  "eid-1",
		JTI:      "jti-non-existent",
	}
	qrPayload, _, _ := signer.Sign(claims)

	repo := &mockTicketRepo{
		getTicketByJTIFn: func(ctx context.Context, jti string) (domain.Ticket, error) {
			return domain.Ticket{}, domain.ErrTicketNotFound
		},
	}

	uc := app.NewVerifyTicketUseCase(repo, signer, discardLogger())
	_, err := uc.Execute(context.Background(), port.VerifyTicketParams{
		QRPayload: qrPayload,
	})
	if !errors.Is(err, domain.ErrTicketNotFound) {
		t.Fatalf("ky vong ErrTicketNotFound khi khong tim thay trong DB, got: %v", err)
	}
}

func TestVerifyTicket_MismatchWithDB_Rejected(t *testing.T) {
	signer := testSigner(t)
	claims := authx.TicketClaims{
		TicketID: "tid-legit-signed",
		OrderID:  "oid-1",
		EventID:  "eid-1",
		JTI:      "jti-tamper-db",
	}
	qrPayload, _, _ := signer.Sign(claims)

	repo := &mockTicketRepo{
		getTicketByJTIFn: func(ctx context.Context, jti string) (domain.Ticket, error) {
			// DB tra ve Ticket ID khac voi trong Claims
			return domain.Ticket{
				ID:      "tid-different-in-db",
				OrderID: "oid-1",
				JTI:     jti,
				Status:  domain.TicketStatusIssued,
			}, nil
		},
	}

	uc := app.NewVerifyTicketUseCase(repo, signer, discardLogger())
	_, err := uc.Execute(context.Background(), port.VerifyTicketParams{
		QRPayload: qrPayload,
	})
	if !errors.Is(err, domain.ErrTamperedQR) {
		t.Fatalf("ky vong ErrTamperedQR khi du lieu DB va claims lech nhau, got: %v", err)
	}
}

func TestVerifyTicket_AlreadyCheckedIn_Rejected(t *testing.T) {
	signer := testSigner(t)
	claims := authx.TicketClaims{
		TicketID: "tid-used",
		OrderID:  "oid-1",
		EventID:  "eid-1",
		JTI:      "jti-used",
	}
	qrPayload, _, _ := signer.Sign(claims)

	checkedInTime := time.Now().Add(-5 * time.Minute)
	repo := &mockTicketRepo{
		getTicketByJTIFn: func(ctx context.Context, jti string) (domain.Ticket, error) {
			return domain.Ticket{
				ID:          "tid-used",
				OrderID:     "oid-1",
				JTI:         jti,
				Status:      domain.TicketStatusCheckedIn,
				CheckedInAt: &checkedInTime,
			}, nil
		},
	}

	uc := app.NewVerifyTicketUseCase(repo, signer, discardLogger())
	_, err := uc.Execute(context.Background(), port.VerifyTicketParams{
		QRPayload: qrPayload,
	})
	if !errors.Is(err, domain.ErrTicketAlreadyUsed) {
		t.Fatalf("ky vong ErrTicketAlreadyUsed (AC-5), got: %v", err)
	}
}

func TestVerifyTicket_VoidOrRefunded_Rejected(t *testing.T) {
	signer := testSigner(t)

	cases := []struct {
		status      domain.TicketStatus
		expectedErr error
	}{
		{domain.TicketStatusVoid, domain.ErrTicketVoid},
		{domain.TicketStatusRefunded, domain.ErrTicketRefunded},
	}

	for _, c := range cases {
		claims := authx.TicketClaims{
			TicketID: "tid-" + string(c.status),
			OrderID:  "oid-1",
			EventID:  "eid-1",
			JTI:      "jti-" + string(c.status),
		}
		qrPayload, _, _ := signer.Sign(claims)

		repo := &mockTicketRepo{
			getTicketByJTIFn: func(ctx context.Context, jti string) (domain.Ticket, error) {
				return domain.Ticket{
					ID:      claims.TicketID,
					OrderID: claims.OrderID,
					JTI:     jti,
					Status:  c.status,
				}, nil
			},
		}

		uc := app.NewVerifyTicketUseCase(repo, signer, discardLogger())
		_, err := uc.Execute(context.Background(), port.VerifyTicketParams{
			QRPayload: qrPayload,
		})
		if !errors.Is(err, c.expectedErr) {
			t.Errorf("status %s: ky vong %v, got: %v", c.status, c.expectedErr, err)
		}
	}
}
