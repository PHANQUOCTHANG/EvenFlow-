package port

import (
	"github.com/eventflow/eventflow/services/notification/internal/domain"
)

// TemplateRenderer la cong render template email va SMS tieng Viet.
type TemplateRenderer interface {
	RenderOrderPaidEmail(evt domain.OrderPaidEvent) (subject, htmlBody, textBody string, err error)
	RenderTicketIssuedEmail(evt domain.TicketIssuedEvent) (subject, htmlBody, textBody string, err error)
	RenderRefundEmail(evt domain.PaymentRefundedEvent) (subject, htmlBody, textBody string, err error)
	RenderSMS(eventType string, data any) (body string, err error)
}
