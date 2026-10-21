package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/mq"
	"github.com/eventflow/eventflow/services/notification/internal/adapter/sender"
	"github.com/eventflow/eventflow/services/notification/internal/adapter/store"
	"github.com/eventflow/eventflow/services/notification/internal/adapter/template"
	"github.com/eventflow/eventflow/services/notification/internal/app"
	"github.com/eventflow/eventflow/services/notification/internal/domain"
	"github.com/eventflow/eventflow/services/notification/internal/port"
)

func setupConsumerTest() (*NotificationEventConsumer, *sender.MockEmailSender, *sender.MockSMSSender, *store.MemoryNotificationStore) {
	emailSender := sender.NewMockEmailSender()
	smsSender := sender.NewMockSMSSender()
	renderer, _ := template.NewGoTemplateRenderer()
	memStore := store.NewMemoryNotificationStore()

	orderPaidUC := app.NewProcessOrderPaidUseCase(emailSender, smsSender, renderer, memStore, nil)
	ticketIssuedUC := app.NewProcessTicketIssuedUseCase(emailSender, smsSender, renderer, memStore, nil)
	refundUC := app.NewProcessRefundUseCase(emailSender, smsSender, renderer, memStore, nil)

	consumer := NewNotificationEventConsumer(orderPaidUC, ticketIssuedUC, refundUC, nil)
	return consumer, emailSender, smsSender, memStore
}

func TestConsumer_ParseEvents_OrderPaid(t *testing.T) {
	ctx := context.Background()
	consumer, emailSender, smsSender, _ := setupConsumerTest()

	evt := domain.OrderPaidEvent{
		OrderID:        "ord-mq-1",
		EventID:        "evt-1",
		IdentityID:     "usr-1",
		AmountCents:    500000,
		RecipientEmail: "buyer@example.com",
		RecipientPhone: "0912345678",
		PaidAt:         time.Now(),
	}
	body, _ := json.Marshal(evt)

	delivery := mq.Delivery{
		RoutingKey: "order.paid",
		MessageID:  "msg-001",
		Attempt:    1,
		Body:       body,
		Headers:    map[string]any{"x-trace-id": "trace-123"},
	}

	err := consumer.HandleDelivery(ctx, delivery)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(emailSender.GetSentEmails()) != 1 {
		t.Errorf("expected 1 sent email, got %d", len(emailSender.GetSentEmails()))
	}
	if len(smsSender.GetSentSMS()) != 1 {
		t.Errorf("expected 1 sent sms, got %d", len(smsSender.GetSentSMS()))
	}
}

func TestConsumer_ParseEvents_TicketIssued(t *testing.T) {
	ctx := context.Background()
	consumer, emailSender, _, _ := setupConsumerTest()

	evt := domain.TicketIssuedEvent{
		OrderID:     "ord-mq-2",
		EventID:     "evt-1",
		IdentityID:  "usr-1",
		TicketCount: 1,
		Tickets: []domain.TicketDetail{
			{ID: "t-1", JTI: "jti-1", QRSig: "sig-1"},
		},
		RecipientEmail: "buyer@example.com",
		IssuedAt:       time.Now(),
	}
	body, _ := json.Marshal(evt)

	// Test routing_key "ticketing.ticket.issued"
	delivery := mq.Delivery{
		RoutingKey: "ticketing.ticket.issued",
		MessageID:  "msg-002",
		Attempt:    1,
		Body:       body,
	}

	err := consumer.HandleDelivery(ctx, delivery)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(emailSender.GetSentEmails()) != 1 {
		t.Errorf("expected 1 sent email, got %d", len(emailSender.GetSentEmails()))
	}
}

func TestConsumer_ParseEvents_Refunded(t *testing.T) {
	ctx := context.Background()
	consumer, emailSender, _, _ := setupConsumerTest()

	evt := domain.PaymentRefundedEvent{
		OrderID:        "ord-mq-3",
		EventID:        "evt-1",
		IdentityID:     "usr-1",
		AmountCents:    250000,
		Reason:         "Event cancelled",
		RecipientEmail: "buyer@example.com",
		RefundedAt:     time.Now(),
	}
	body, _ := json.Marshal(evt)

	delivery := mq.Delivery{
		RoutingKey: "payment.refunded",
		MessageID:  "msg-003",
		Attempt:    1,
		Body:       body,
	}

	err := consumer.HandleDelivery(ctx, delivery)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(emailSender.GetSentEmails()) != 1 {
		t.Errorf("expected 1 sent email, got %d", len(emailSender.GetSentEmails()))
	}
}

func TestConsumer_PermanentError_RoutesDLQ(t *testing.T) {
	ctx := context.Background()
	consumer, _, _, _ := setupConsumerTest()

	// 1. Poison pill: Invalid JSON
	delivery := mq.Delivery{
		RoutingKey: "order.paid",
		Body:       []byte("corrupted non json {[["),
	}

	err := consumer.HandleDelivery(ctx, delivery)
	if !errors.Is(err, mq.ErrPermanent) {
		t.Errorf("expected errors.Is(err, mq.ErrPermanent) for invalid json, got %v", err)
	}

	// 2. Unsupported routing key
	delivery2 := mq.Delivery{
		RoutingKey: "unknown.event.type",
		Body:       []byte("{}"),
	}
	err2 := consumer.HandleDelivery(ctx, delivery2)
	if !errors.Is(err2, mq.ErrPermanent) {
		t.Errorf("expected errors.Is(err2, mq.ErrPermanent) for unknown routing key, got %v", err2)
	}
}

func TestConsumer_TransientError_TriggersRetry(t *testing.T) {
	ctx := context.Background()
	consumer, emailSender, _, _ := setupConsumerTest()

	// Simulate transient network error
	emailSender.SetSimulateError(errors.New("transient smtp connection timeout"))

	evt := domain.OrderPaidEvent{
		OrderID:        "ord-transient",
		IdentityID:     "usr-1",
		RecipientEmail: "buyer@example.com",
		PaidAt:         time.Now(),
	}
	body, _ := json.Marshal(evt)

	delivery := mq.Delivery{
		RoutingKey: "order.paid",
		Attempt:    1,
		Body:       body,
	}

	err := consumer.HandleDelivery(ctx, delivery)
	if err == nil {
		t.Fatalf("expected error on transient failure")
	}
	// Transient error MUST NOT be ErrPermanent so that libs/go/mq will retry!
	if errors.Is(err, mq.ErrPermanent) {
		t.Errorf("transient error must not be ErrPermanent, got: %v", err)
	}
}

type fakeDirectory struct {
	r     port.Recipient
	err   error
	calls int
}

func (f *fakeDirectory) Lookup(_ context.Context, _, _ string) (port.Recipient, error) {
	f.calls++
	return f.r, f.err
}

// Payload that ma ticketing (OutboxTicketIssuedPayload) va payment (OutboxOrderPaidPayload) phat:
// khong co recipient_email. Consumer phai tra email theo identity_id roi moi gui.
func TestConsumer_TraEmailKhiMessageKhongMangSan(t *testing.T) {
	cases := map[string]string{
		"ticketing.ticket.issued": `{"order_id":"o-1","event_id":"e-1","identity_id":"u-1","ticket_count":1,
			"tickets":[{"id":"t-1","order_id":"o-1","ticket_type_id":"tt-1","jti":"j-1","qr_sig":"sig","status":"ISSUED"}],
			"issued_at":"2026-10-21T12:00:00Z"}`,
		"order.paid": `{"order_id":"o-1","event_id":"e-1","identity_id":"u-1","payment_id":"p-1","provider":"sandbox",
			"provider_txn_id":"x","amount_cents":50000000,"currency":"VND","paid_at":"2026-10-21T12:00:00Z"}`,
	}
	for key, body := range cases {
		consumer, emailSender, _, _ := setupConsumerTest()
		dir := &fakeDirectory{r: port.Recipient{Email: "buyer@example.com", EventTitle: "EventFlow Demo Concert"}}
		consumer.WithDirectory(dir)

		err := consumer.HandleDelivery(context.Background(), mq.Delivery{RoutingKey: key, Body: []byte(body), Attempt: 1})
		if err != nil {
			t.Fatalf("%s: HandleDelivery = %v", key, err)
		}
		sent := emailSender.GetSentEmails()
		if len(sent) != 1 || sent[0].To != "buyer@example.com" {
			t.Fatalf("%s: email da gui = %+v", key, sent)
		}
		if dir.calls != 1 {
			t.Errorf("%s: Lookup goi %d lan, muon 1", key, dir.calls)
		}
	}
}

// Message da co email (vd goi qua HTTP noi bo): khong can tra cuu.
func TestConsumer_CoEmailVaTenThiKhongTraCuu(t *testing.T) {
	consumer, emailSender, _, _ := setupConsumerTest()
	dir := &fakeDirectory{}
	consumer.WithDirectory(dir)
	body := `{"order_id":"o-1","event_id":"e-1","event_title":"T","identity_id":"u-1","recipient_email":"a@example.com","paid_at":"2026-10-21T12:00:00Z"}`
	if err := consumer.HandleDelivery(context.Background(), mq.Delivery{RoutingKey: "order.paid", Body: []byte(body)}); err != nil {
		t.Fatalf("HandleDelivery = %v", err)
	}
	if dir.calls != 0 || len(emailSender.GetSentEmails()) != 1 {
		t.Errorf("calls = %d, emails = %d", dir.calls, len(emailSender.GetSentEmails()))
	}
}

func TestConsumer_TraCuuLoiThiRetryKhongTimThayThiDLQ(t *testing.T) {
	body := []byte(`{"order_id":"o-1","event_id":"e-1","identity_id":"u-1","paid_at":"2026-10-21T12:00:00Z"}`)

	consumer, _, _, _ := setupConsumerTest()
	consumer.WithDirectory(&fakeDirectory{err: errors.New("db mat ket noi")})
	err := consumer.HandleDelivery(context.Background(), mq.Delivery{RoutingKey: "order.paid", Body: body})
	if err == nil || errors.Is(err, mq.ErrPermanent) {
		t.Errorf("DB loi: err = %v, muon loi thuong de retry", err)
	}

	consumer, _, _, _ = setupConsumerTest()
	consumer.WithDirectory(&fakeDirectory{})
	err = consumer.HandleDelivery(context.Background(), mq.Delivery{RoutingKey: "order.paid", Body: body})
	if !errors.Is(err, mq.ErrPermanent) {
		t.Errorf("khong tim thay identity: err = %v, muon ErrPermanent", err)
	}
}
