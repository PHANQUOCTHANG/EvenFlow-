package postgresadapter_test

import (
	"encoding/json"
	"testing"
	"time"

	postgresadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
)

func TestOutboxTicketIssuedPayload_JSONMarshal(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	payload := postgresadapter.OutboxTicketIssuedPayload{
		OrderID:     "11111111-1111-1111-1111-111111111111",
		EventID:     "22222222-2222-2222-2222-222222222222",
		IdentityID:  "33333333-3333-3333-3333-333333333333",
		TicketCount: 2,
		Tickets: []domain.Ticket{
			{
				ID:           "44444444-4444-4444-4444-444444444444",
				OrderID:      "11111111-1111-1111-1111-111111111111",
				TicketTypeID: "55555555-5555-5555-5555-555555555555",
				JTI:          "jti-123",
				QRSig:        "sig-abc",
				Status:       domain.TicketStatusIssued,
				CreatedAt:    now,
			},
		},
		IssuedAt: now,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal that bai: %v", err)
	}

	var decoded postgresadapter.OutboxTicketIssuedPayload
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal that bai: %v", err)
	}

	if decoded.OrderID != payload.OrderID {
		t.Errorf("OrderID lech: %s != %s", decoded.OrderID, payload.OrderID)
	}
	if decoded.TicketCount != 2 {
		t.Errorf("TicketCount lech: %d != 2", decoded.TicketCount)
	}
	if len(decoded.Tickets) != 1 || decoded.Tickets[0].JTI != "jti-123" {
		t.Errorf("Tickets lech: %+v", decoded.Tickets)
	}
}
