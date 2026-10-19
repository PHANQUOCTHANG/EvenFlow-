package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
)

func TestTicket_StatusValidation(t *testing.T) {
	validStatuses := []domain.TicketStatus{
		domain.TicketStatusIssued,
		domain.TicketStatusCheckedIn,
		domain.TicketStatusVoid,
		domain.TicketStatusRefunded,
	}

	for _, s := range validStatuses {
		if !domain.IsValidTicketStatus(s) {
			t.Errorf("ky vong trang thai %s la hop le", s)
		}
	}

	invalidStatuses := []domain.TicketStatus{
		"",
		"PENDING",
		"UNKNOWN",
		"CANCELLED",
	}

	for _, s := range invalidStatuses {
		if domain.IsValidTicketStatus(s) {
			t.Errorf("ky vong trang thai %s la khong hop le", s)
		}
	}
}

func TestTicket_CanAdmit(t *testing.T) {
	now := time.Now()

	// 1. ISSUED -> OK
	ticketIssued := domain.Ticket{
		ID:     "t-1",
		Status: domain.TicketStatusIssued,
	}
	if err := ticketIssued.CanAdmit(); err != nil {
		t.Fatalf("ky vong ve ISSUED co the vao cua, nhan duoc err: %v", err)
	}

	// 2. CHECKED_IN -> ErrTicketAlreadyUsed (AC-5)
	ticketCheckedIn := domain.Ticket{
		ID:          "t-2",
		Status:      domain.TicketStatusCheckedIn,
		CheckedInAt: &now,
	}
	if !errors.Is(ticketCheckedIn.CanAdmit(), domain.ErrTicketAlreadyUsed) {
		t.Fatalf("ky vong ErrTicketAlreadyUsed khi CHECKED_IN, nhan duoc: %v", ticketCheckedIn.CanAdmit())
	}

	// 3. VOID -> ErrTicketVoid
	ticketVoid := domain.Ticket{
		ID:     "t-3",
		Status: domain.TicketStatusVoid,
	}
	if !errors.Is(ticketVoid.CanAdmit(), domain.ErrTicketVoid) {
		t.Fatalf("ky vong ErrTicketVoid khi VOID, nhan duoc: %v", ticketVoid.CanAdmit())
	}

	// 4. REFUNDED -> ErrTicketRefunded
	ticketRefunded := domain.Ticket{
		ID:     "t-4",
		Status: domain.TicketStatusRefunded,
	}
	if !errors.Is(ticketRefunded.CanAdmit(), domain.ErrTicketRefunded) {
		t.Fatalf("ky vong ErrTicketRefunded khi REFUNDED, nhan duoc: %v", ticketRefunded.CanAdmit())
	}

	// 5. Unknown status
	ticketBad := domain.Ticket{
		ID:     "t-5",
		Status: "INVALID",
	}
	if !errors.Is(ticketBad.CanAdmit(), domain.ErrInvalidTicketStatus) {
		t.Fatalf("ky vong ErrInvalidTicketStatus voi status la, nhan duoc: %v", ticketBad.CanAdmit())
	}
}

func TestTicket_NewJTI(t *testing.T) {
	jti1 := domain.NewJTI()
	jti2 := domain.NewJTI()

	if len(jti1) != 32 {
		t.Errorf("ky vong do dai JTI la 32 hex chars, got %d (%s)", len(jti1), jti1)
	}
	if jti1 == jti2 {
		t.Fatalf("hai lan goi NewJTI tra ve trung nhau: %s", jti1)
	}
}
