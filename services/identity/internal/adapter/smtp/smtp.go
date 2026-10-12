package smtp

import (
	"context"
	"fmt"

	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"gopkg.in/gomail.v2"
)

type smtpNotifier struct {
	host     string
	port     int
	user     string
	password string
	fromName string
}

func NewSMTPNotifier(host string, port int, user string, password string, fromName string) domain.OTPNotifier {
	return &smtpNotifier{
		host:     host,
		port:     port,
		user:     user,
		password: password,
		fromName: fromName,
	}
}

func (s *smtpNotifier) SendOTP(ctx context.Context, to string, otp string) error {
	m := gomail.NewMessage()
	m.SetHeader("From", fmt.Sprintf("%s <%s>", s.fromName, s.user))
	m.SetHeader("To", to)
	m.SetHeader("Subject", "[EventFlow] Mã Xác Nhận Đăng Ký")
	
	body := fmt.Sprintf(`
		<div style="font-family: Arial, sans-serif; padding: 20px; color: #333;">
			<h2 style="color: #4f46e5;">Xác minh tài khoản EventFlow</h2>
			<p>Chào bạn,</p>
			<p>Mã OTP của bạn là: <strong style="font-size: 24px; color: #111827;">%s</strong></p>
			<p>Mã này có hiệu lực trong 5 phút. Tuyệt đối không chia sẻ mã này cho bất kỳ ai.</p>
			<hr style="border: none; border-top: 1px solid #eaeaea; margin: 20px 0;" />
			<p style="font-size: 12px; color: #999;">Nếu bạn không yêu cầu mã này, vui lòng bỏ qua email.</p>
		</div>
	`, otp)

	m.SetBody("text/html", body)

	d := gomail.NewDialer(s.host, s.port, s.user, s.password)

	// Gomail send is synchronous and doesn't take context natively,
	// but sending usually takes just a few seconds. 
	// For high-throughput production, this would be passed to RabbitMQ (ai-worker) instead.
	// For this phase, sending directly via Identity service is fine.
	return d.DialAndSend(m)
}
