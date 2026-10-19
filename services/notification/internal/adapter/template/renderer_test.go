package template

import (
	"strings"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/notification/internal/domain"
)

func TestRenderer_OrderPaidEmail_Vietnamese(t *testing.T) {
	r, err := NewGoTemplateRenderer()
	if err != nil {
		t.Fatalf("failed to init renderer: %v", err)
	}

	evt := domain.OrderPaidEvent{
		OrderID:        "ord-98765",
		EventID:        "evt-12345",
		EventTitle:     "Đại Nhạc Hội Mùa Hè 2026",
		AmountCents:    750000,
		Provider:       "VNPAY",
		ProviderTxnID:  "VNPAY-TRANS-999",
		RecipientEmail: "customer@example.com",
		PaidAt:         time.Date(2026, 10, 19, 14, 30, 0, 0, time.UTC),
	}

	subject, htmlBody, textBody, err := r.RenderOrderPaidEmail(evt)
	if err != nil {
		t.Fatalf("unexpected error rendering order paid email: %v", err)
	}

	if !strings.Contains(subject, "ord-98765") {
		t.Errorf("subject must contain order ID, got: %s", subject)
	}
	if !strings.Contains(htmlBody, "750.000 VND") {
		t.Errorf("htmlBody must contain formatted amount, got: %s", htmlBody)
	}
	if !strings.Contains(htmlBody, "Đại Nhạc Hội Mùa Hè 2026") {
		t.Errorf("htmlBody must contain event title")
	}
	if !strings.Contains(textBody, "VNPAY-TRANS-999") {
		t.Errorf("textBody must contain transaction ID")
	}
}

func TestRenderer_TicketIssuedEmail_WithQRDetails(t *testing.T) {
	r, err := NewGoTemplateRenderer()
	if err != nil {
		t.Fatalf("failed to init renderer: %v", err)
	}

	evt := domain.TicketIssuedEvent{
		OrderID:     "ord-55555",
		EventID:     "evt-123",
		EventTitle:  "Hội Nghị Công Nghệ EvenFlow",
		TicketCount: 2,
		Tickets: []domain.TicketDetail{
			{
				ID:    "t-001",
				JTI:   "jti-uuid-001",
				QRSig: "hmac-sha256-signature-111111",
			},
			{
				ID:    "t-002",
				JTI:   "jti-uuid-002",
				QRSig: "hmac-sha256-signature-222222",
			},
		},
		RecipientEmail: "customer@example.com",
		IssuedAt:       time.Now(),
	}

	subject, htmlBody, textBody, err := r.RenderTicketIssuedEmail(evt)
	if err != nil {
		t.Fatalf("unexpected error rendering ticket email: %v", err)
	}

	if !strings.Contains(subject, "ord-55555") {
		t.Errorf("subject must contain order ID")
	}
	if !strings.Contains(htmlBody, "jti-uuid-001") || !strings.Contains(htmlBody, "jti-uuid-002") {
		t.Errorf("htmlBody must contain all ticket JTIs")
	}
	if !strings.Contains(htmlBody, "hmac-sha256-signature-111111") {
		t.Errorf("htmlBody must contain QR signatures")
	}
	if !strings.Contains(textBody, "t-002") {
		t.Errorf("textBody must contain ticket details")
	}
}

func TestRenderer_RefundEmail_Vietnamese(t *testing.T) {
	r, err := NewGoTemplateRenderer()
	if err != nil {
		t.Fatalf("failed to init renderer: %v", err)
	}

	evt := domain.PaymentRefundedEvent{
		OrderID:        "ord-777",
		AmountCents:    1000000,
		Reason:         "Thời tiết xấu, ban tổ chức hủy sự kiện",
		RecipientEmail: "customer@example.com",
		RefundedAt:     time.Now(),
	}

	subject, htmlBody, textBody, err := r.RenderRefundEmail(evt)
	if err != nil {
		t.Fatalf("unexpected error rendering refund email: %v", err)
	}

	if !strings.Contains(subject, "ord-777") {
		t.Errorf("subject must contain order ID")
	}
	if !strings.Contains(htmlBody, "1.000.000 VND") {
		t.Errorf("htmlBody must contain formatted refund amount")
	}
	if !strings.Contains(htmlBody, "Thời tiết xấu") {
		t.Errorf("htmlBody must contain refund reason")
	}
	if !strings.Contains(textBody, "1-3 ngay") {
		t.Errorf("textBody must contain processing time notice")
	}
}

func TestRenderer_SMS_Compact160Chars(t *testing.T) {
	r, err := NewGoTemplateRenderer()
	if err != nil {
		t.Fatalf("failed to init renderer: %v", err)
	}

	// 1. Order paid SMS
	paidSMS, err := r.RenderSMS("order.paid", domain.OrderPaidEvent{
		OrderID:     "ord-111",
		AmountCents: 500000,
		Provider:    "MOMO",
	})
	if err != nil {
		t.Fatalf("unexpected error rendering paid SMS: %v", err)
	}
	if !strings.Contains(paidSMS, "ord-111") || !strings.Contains(paidSMS, "500.000 VND") {
		t.Errorf("SMS must contain order ID and amount")
	}

	// 2. Ticket issued SMS
	ticketSMS, err := r.RenderSMS("ticket.issued", domain.TicketIssuedEvent{
		OrderID:     "ord-222",
		TicketCount: 3,
	})
	if err != nil {
		t.Fatalf("unexpected error rendering ticket SMS: %v", err)
	}
	if !strings.Contains(ticketSMS, "3 ve") || !strings.Contains(ticketSMS, "ord-222") {
		t.Errorf("SMS must contain ticket count and order ID")
	}

	// 3. Refund SMS
	refundSMS, err := r.RenderSMS("payment.refunded", domain.PaymentRefundedEvent{
		OrderID:     "ord-333",
		AmountCents: 200000,
		Reason:      "Huy su kien",
	})
	if err != nil {
		t.Fatalf("unexpected error rendering refund SMS: %v", err)
	}
	if !strings.Contains(refundSMS, "ord-333") || !strings.Contains(refundSMS, "200.000 VND") {
		t.Errorf("SMS must contain refund details")
	}

	// 4. Invalid event type
	_, err = r.RenderSMS("unknown.event", nil)
	if err == nil {
		t.Errorf("expected error for unknown event type")
	}
}
