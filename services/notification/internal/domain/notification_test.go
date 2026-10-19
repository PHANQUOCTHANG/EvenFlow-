package domain

import (
	"errors"
	"testing"
	"time"
)

func TestChannel_IsValid(t *testing.T) {
	if !ChannelEmail.IsValid() {
		t.Errorf("expected ChannelEmail to be valid")
	}
	if !ChannelSMS.IsValid() {
		t.Errorf("expected ChannelSMS to be valid")
	}
	if Channel("push").IsValid() {
		t.Errorf("expected Channel('push') to be invalid")
	}
}

func TestEmailMessage_Validate(t *testing.T) {
	tests := []struct {
		name    string
		msg     EmailMessage
		wantErr error
	}{
		{
			name: "Valid email message",
			msg: EmailMessage{
				To:       "user@example.com",
				Subject:  "Xac nhan",
				HTMLBody: "<p>Hello</p>",
			},
			wantErr: nil,
		},
		{
			name: "Invalid email recipient",
			msg: EmailMessage{
				To:       "invalid-email",
				Subject:  "Xac nhan",
				HTMLBody: "<p>Hello</p>",
			},
			wantErr: ErrInvalidRecipient,
		},
		{
			name: "Empty recipient",
			msg: EmailMessage{
				To:       "",
				Subject:  "Xac nhan",
				HTMLBody: "<p>Hello</p>",
			},
			wantErr: ErrInvalidRecipient,
		},
		{
			name: "Empty subject",
			msg: EmailMessage{
				To:       "user@example.com",
				Subject:  "   ",
				HTMLBody: "<p>Hello</p>",
			},
			wantErr: ErrEmptyContent,
		},
		{
			name: "Empty content body",
			msg: EmailMessage{
				To:       "user@example.com",
				Subject:  "Xac nhan",
				HTMLBody: "",
				TextBody: "  ",
			},
			wantErr: ErrEmptyContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestSMSMessage_Validate(t *testing.T) {
	tests := []struct {
		name    string
		msg     SMSMessage
		wantErr error
	}{
		{
			name: "Valid phone 09xxxxxxxx",
			msg: SMSMessage{
				To:          "0912345678",
				MessageBody: "EvenFlow xac nhan don hang",
			},
			wantErr: nil,
		},
		{
			name: "Valid phone +849xxxxxxxx",
			msg: SMSMessage{
				To:          "+84912345678",
				MessageBody: "EvenFlow xac nhan don hang",
			},
			wantErr: nil,
		},
		{
			name: "Invalid phone",
			msg: SMSMessage{
				To:          "12345",
				MessageBody: "EvenFlow xac nhan",
			},
			wantErr: ErrInvalidRecipient,
		},
		{
			name: "Empty message body",
			msg: SMSMessage{
				To:          "0912345678",
				MessageBody: "   ",
			},
			wantErr: ErrEmptyContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestOrderPaidEvent_Validate(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name    string
		evt     OrderPaidEvent
		wantErr error
	}{
		{
			name: "Valid order paid event",
			evt: OrderPaidEvent{
				OrderID:        "ord-123",
				IdentityID:     "usr-456",
				RecipientEmail: "buyer@example.com",
				AmountCents:    500000,
				PaidAt:         now,
			},
			wantErr: nil,
		},
		{
			name: "Missing order ID",
			evt: OrderPaidEvent{
				OrderID:        "",
				IdentityID:     "usr-456",
				RecipientEmail: "buyer@example.com",
			},
			wantErr: ErrInvalidEventPayload,
		},
		{
			name: "Missing recipients",
			evt: OrderPaidEvent{
				OrderID:    "ord-123",
				IdentityID: "usr-456",
			},
			wantErr: ErrInvalidRecipient,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.evt.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestTicketIssuedEvent_Validate(t *testing.T) {
	tests := []struct {
		name    string
		evt     TicketIssuedEvent
		wantErr error
	}{
		{
			name: "Valid ticket issued event",
			evt: TicketIssuedEvent{
				OrderID:        "ord-123",
				IdentityID:     "usr-456",
				RecipientEmail: "buyer@example.com",
				TicketCount:    1,
				Tickets: []TicketDetail{
					{ID: "t-1", JTI: "jti-1", QRSig: "sig-1"},
				},
			},
			wantErr: nil,
		},
		{
			name: "Missing IdentityID",
			evt: TicketIssuedEvent{
				OrderID:        "ord-123",
				IdentityID:     "",
				RecipientEmail: "buyer@example.com",
			},
			wantErr: ErrInvalidEventPayload,
		},
		{
			name: "Missing recipient",
			evt: TicketIssuedEvent{
				OrderID:    "ord-123",
				IdentityID: "usr-456",
			},
			wantErr: ErrInvalidRecipient,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.evt.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestPaymentRefundedEvent_Validate(t *testing.T) {
	tests := []struct {
		name    string
		evt     PaymentRefundedEvent
		wantErr error
	}{
		{
			name: "Valid payment refunded event",
			evt: PaymentRefundedEvent{
				OrderID:        "ord-123",
				IdentityID:     "usr-456",
				RecipientEmail: "buyer@example.com",
				AmountCents:    250000,
				Reason:         "Event cancelled",
			},
			wantErr: nil,
		},
		{
			name: "Missing OrderID",
			evt: PaymentRefundedEvent{
				OrderID:        "",
				IdentityID:     "usr-456",
				RecipientEmail: "buyer@example.com",
			},
			wantErr: ErrInvalidEventPayload,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.evt.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestMakeDedupKey(t *testing.T) {
	key := MakeDedupKey("order.paid", "ord-999", ChannelEmail)
	expected := "order.paid:ord-999:email"
	if key != expected {
		t.Errorf("expected %s, got %s", expected, key)
	}
}

func TestFormatCurrencyVND(t *testing.T) {
	tests := []struct {
		amount   int64
		expected string
	}{
		{0, "0 VND"},
		{500, "500 VND"},
		{1000, "1.000 VND"},
		{50000, "50.000 VND"},
		{500000, "500.000 VND"},
		{12500000, "12.500.000 VND"},
		{-250000, "-250.000 VND"},
	}

	for _, tt := range tests {
		got := FormatCurrencyVND(tt.amount)
		if got != tt.expected {
			t.Errorf("FormatCurrencyVND(%d) = %q; want %q", tt.amount, got, tt.expected)
		}
	}
}
