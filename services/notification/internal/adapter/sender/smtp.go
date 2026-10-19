package sender

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/eventflow/eventflow/services/notification/internal/domain"
	"github.com/eventflow/eventflow/services/notification/internal/port"
)

// SMTPEmailSender gui email qua may chu SMTP tieu chuan.
type SMTPEmailSender struct {
	Host        string
	Port        int
	Username    string
	Password    string
	FromAddress string
	// SendMailFn cho phep mock lop mang net/smtp trong unit test
	SendMailFn func(addr string, a smtp.Auth, from string, to []string, msg []byte) error
}

var _ port.EmailSender = (*SMTPEmailSender)(nil)

func NewSMTPEmailSender(host string, port int, username, password, fromAddress string) *SMTPEmailSender {
	return &SMTPEmailSender{
		Host:        host,
		Port:        port,
		Username:    username,
		Password:    password,
		FromAddress: fromAddress,
		SendMailFn:  smtp.SendMail,
	}
}

func (s *SMTPEmailSender) SendEmail(ctx context.Context, msg domain.EmailMessage) error {
	if err := msg.Validate(); err != nil {
		return fmt.Errorf("%w: %w", domain.ErrSenderFailed, err)
	}

	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
	var auth smtp.Auth
	if s.Username != "" && s.Password != "" {
		auth = smtp.PlainAuth("", s.Username, s.Password, s.Host)
	}

	headers := make(map[string]string)
	headers["From"] = s.FromAddress
	headers["To"] = msg.To
	headers["Subject"] = msg.Subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=UTF-8"

	var sb strings.Builder
	for k, v := range headers {
		sb.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	sb.WriteString("\r\n")

	body := msg.HTMLBody
	if body == "" {
		body = msg.TextBody
	}
	sb.WriteString(body)

	err := s.SendMailFn(addr, auth, s.FromAddress, []string{msg.To}, []byte(sb.String()))
	if err != nil {
		return fmt.Errorf("smtp sendmail to %s: %w", msg.To, err)
	}
	return nil
}
