package port

import (
	"context"

	"github.com/eventflow/eventflow/services/notification/internal/domain"
)

// EmailSender la cong truu tuong de phat thu dien tu (SMTP hoac Mock).
type EmailSender interface {
	SendEmail(ctx context.Context, msg domain.EmailMessage) error
}

// SMSSender la cong truu tuong de gui tin nhan SMS (Provider hoac Mock).
type SMSSender interface {
	SendSMS(ctx context.Context, msg domain.SMSMessage) error
}
