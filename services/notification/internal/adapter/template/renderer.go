package template

import (
	"bytes"
	"fmt"
	htmltmpl "html/template"
	"strings"
	texttmpl "text/template"

	"github.com/eventflow/eventflow/services/notification/internal/domain"
	"github.com/eventflow/eventflow/services/notification/internal/port"
)

type GoTemplateRenderer struct {
	orderPaidHTML *htmltmpl.Template
	orderPaidText *texttmpl.Template
	ticketHTML    *htmltmpl.Template
	ticketText    *texttmpl.Template
	refundHTML    *htmltmpl.Template
	refundText    *texttmpl.Template
}

var _ port.TemplateRenderer = (*GoTemplateRenderer)(nil)

// NewGoTemplateRenderer khoi tao template renderer tieng Viet chuan hoa.
func NewGoTemplateRenderer() (*GoTemplateRenderer, error) {
	r := &GoTemplateRenderer{}

	var err error
	r.orderPaidHTML, err = htmltmpl.New("orderPaidHTML").Parse(orderPaidHTMLTmpl)
	if err != nil {
		return nil, fmt.Errorf("parse orderPaidHTML: %w", err)
	}

	r.orderPaidText, err = texttmpl.New("orderPaidText").Parse(orderPaidTextTmpl)
	if err != nil {
		return nil, fmt.Errorf("parse orderPaidText: %w", err)
	}

	r.ticketHTML, err = htmltmpl.New("ticketHTML").Parse(ticketHTMLTmpl)
	if err != nil {
		return nil, fmt.Errorf("parse ticketHTML: %w", err)
	}

	r.ticketText, err = texttmpl.New("ticketText").Parse(ticketTextTmpl)
	if err != nil {
		return nil, fmt.Errorf("parse ticketText: %w", err)
	}

	r.refundHTML, err = htmltmpl.New("refundHTML").Parse(refundHTMLTmpl)
	if err != nil {
		return nil, fmt.Errorf("parse refundHTML: %w", err)
	}

	r.refundText, err = texttmpl.New("refundText").Parse(refundTextTmpl)
	if err != nil {
		return nil, fmt.Errorf("parse refundText: %w", err)
	}

	return r, nil
}

type orderPaidViewModel struct {
	OrderID         string
	EventTitle      string
	AmountFormatted string
	Provider        string
	ProviderTxnID   string
	PaidAtFormatted string
}

func (r *GoTemplateRenderer) RenderOrderPaidEmail(evt domain.OrderPaidEvent) (string, string, string, error) {
	title := evt.EventTitle
	if title == "" {
		title = fmt.Sprintf("Sự kiện mã %s", evt.EventID)
	}
	vm := orderPaidViewModel{
		OrderID:         evt.OrderID,
		EventTitle:      title,
		AmountFormatted: domain.FormatCurrencyVND(evt.AmountCents),
		Provider:        evt.Provider,
		ProviderTxnID:   evt.ProviderTxnID,
		PaidAtFormatted: evt.PaidAt.Format("15:04:05 02/01/2006"),
	}

	var htmlBuf bytes.Buffer
	if err := r.orderPaidHTML.Execute(&htmlBuf, vm); err != nil {
		return "", "", "", fmt.Errorf("execute orderPaidHTML: %w", err)
	}

	var textBuf bytes.Buffer
	if err := r.orderPaidText.Execute(&textBuf, vm); err != nil {
		return "", "", "", fmt.Errorf("execute orderPaidText: %w", err)
	}

	subject := fmt.Sprintf("[EvenFlow] Xác nhận thanh toán thành công đơn hàng #%s", evt.OrderID)
	return subject, htmlBuf.String(), textBuf.String(), nil
}

type ticketIssuedViewModel struct {
	OrderID           string
	EventTitle        string
	TicketCount       int
	Tickets           []domain.TicketDetail
	IssuedAtFormatted string
}

func (r *GoTemplateRenderer) RenderTicketIssuedEmail(evt domain.TicketIssuedEvent) (string, string, string, error) {
	title := evt.EventTitle
	if title == "" {
		title = fmt.Sprintf("Sự kiện mã %s", evt.EventID)
	}
	vm := ticketIssuedViewModel{
		OrderID:           evt.OrderID,
		EventTitle:        title,
		TicketCount:       evt.TicketCount,
		Tickets:           evt.Tickets,
		IssuedAtFormatted: evt.IssuedAt.Format("15:04:05 02/01/2006"),
	}

	var htmlBuf bytes.Buffer
	if err := r.ticketHTML.Execute(&htmlBuf, vm); err != nil {
		return "", "", "", fmt.Errorf("execute ticketHTML: %w", err)
	}

	var textBuf bytes.Buffer
	if err := r.ticketText.Execute(&textBuf, vm); err != nil {
		return "", "", "", fmt.Errorf("execute ticketText: %w", err)
	}

	subject := fmt.Sprintf("[EvenFlow] Vé điện tử chính thức cho đơn hàng #%s", evt.OrderID)
	return subject, htmlBuf.String(), textBuf.String(), nil
}

type refundViewModel struct {
	OrderID             string
	AmountFormatted     string
	Reason              string
	RefundedAtFormatted string
}

func (r *GoTemplateRenderer) RenderRefundEmail(evt domain.PaymentRefundedEvent) (string, string, string, error) {
	reason := evt.Reason
	if strings.TrimSpace(reason) == "" {
		reason = "Sự kiện bị hủy hoặc theo yêu cầu ban tổ chức"
	}
	vm := refundViewModel{
		OrderID:             evt.OrderID,
		AmountFormatted:     domain.FormatCurrencyVND(evt.AmountCents),
		Reason:              reason,
		RefundedAtFormatted: evt.RefundedAt.Format("15:04:05 02/01/2006"),
	}

	var htmlBuf bytes.Buffer
	if err := r.refundHTML.Execute(&htmlBuf, vm); err != nil {
		return "", "", "", fmt.Errorf("execute refundHTML: %w", err)
	}

	var textBuf bytes.Buffer
	if err := r.refundText.Execute(&textBuf, vm); err != nil {
		return "", "", "", fmt.Errorf("execute refundText: %w", err)
	}

	subject := fmt.Sprintf("[EvenFlow] Thông báo hoàn tiền cho đơn hàng #%s", evt.OrderID)
	return subject, htmlBuf.String(), textBuf.String(), nil
}

func (r *GoTemplateRenderer) RenderSMS(eventType string, data any) (string, error) {
	switch eventType {
	case "order.paid":
		evt, ok := data.(domain.OrderPaidEvent)
		if !ok {
			return "", domain.ErrInvalidEventPayload
		}
		return fmt.Sprintf("EvenFlow: Don hang %s da thanh toan %s thanh cong qua %s. Ve dien tu se duoc gui vao email cua ban.",
			evt.OrderID, domain.FormatCurrencyVND(evt.AmountCents), evt.Provider), nil

	case "ticket.issued", "ticketing.ticket.issued":
		evt, ok := data.(domain.TicketIssuedEvent)
		if !ok {
			return "", domain.ErrInvalidEventPayload
		}
		return fmt.Sprintf("EvenFlow: %d ve cho don hang %s da duoc phat hanh. Vui long kiem tra email de lay ma QR check-in.",
			evt.TicketCount, evt.OrderID), nil

	case "payment.refunded":
		evt, ok := data.(domain.PaymentRefundedEvent)
		if !ok {
			return "", domain.ErrInvalidEventPayload
		}
		return fmt.Sprintf("EvenFlow: Don hang %s da duoc hoan lai %s. Ly do: %s.",
			evt.OrderID, domain.FormatCurrencyVND(evt.AmountCents), evt.Reason), nil

	default:
		return "", fmt.Errorf("unsupported sms event type: %s", eventType)
	}
}

// ======================== HTML & TEXT TEMPLATES ========================

const orderPaidHTMLTmpl = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <style>
    body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; background-color: #f4f6f9; color: #1e293b; padding: 20px; }
    .card { max-width: 600px; margin: 0 auto; background: #ffffff; border-radius: 8px; box-shadow: 0 4px 6px -1px rgba(0,0,0,0.1); overflow: hidden; }
    .header { background: #0f172a; color: #ffffff; padding: 24px; text-align: center; }
    .content { padding: 24px; }
    .badge-success { display: inline-block; padding: 6px 12px; background: #ecfdf5; color: #047857; border-radius: 9999px; font-weight: 600; font-size: 14px; }
    .item-row { display: flex; justify-content: space-between; padding: 10px 0; border-bottom: 1px solid #f1f5f9; }
    .label { color: #64748b; }
    .value { font-weight: 600; }
    .footer { text-align: center; padding: 16px; color: #94a3b8; font-size: 12px; }
  </style>
</head>
<body>
  <div class="card">
    <div class="header">
      <h2>EvenFlow</h2>
      <p>Nền tảng bán vé sự kiện trực tuyến</p>
    </div>
    <div class="content">
      <div style="text-align: center; margin-bottom: 20px;">
        <span class="badge-success">Thanh toán thành công</span>
      </div>
      <h3>Xác nhận đơn hàng #{{.OrderID}}</h3>
      <p>Cảm ơn bạn đã đặt vé cho sự kiện <strong>{{.EventTitle}}</strong>.</p>
      
      <div style="margin-top: 16px;">
        <div class="item-row"><span class="label">Mã đơn hàng:</span><span class="value">{{.OrderID}}</span></div>
        <div class="item-row"><span class="label">Tổng tiền thanh toán:</span><span class="value" style="color: #047857;">{{.AmountFormatted}}</span></div>
        <div class="item-row"><span class="label">Cổng thanh toán:</span><span class="value">{{.Provider}} (GD: {{.ProviderTxnID}})</span></div>
        <div class="item-row"><span class="label">Thời gian hoàn tất:</span><span class="value">{{.PaidAtFormatted}}</span></div>
      </div>
      
      <p style="margin-top: 20px; font-size: 14px; color: #475569;">
        Vé điện tử và mã QR check-in chính thức sẽ được gửi đến email này ngay sau khi hệ thống hoàn tất phát hành vé.
      </p>
    </div>
    <div class="footer">
      EvenFlow Platform &copy; 2026. Mọi quyền được bảo lưu.
    </div>
  </div>
</body>
</html>`

const orderPaidTextTmpl = `EvenFlow - Xac nhan thanh toan don hang #{{.OrderID}}

Chuc mung ban da thanh toan thanh cong cho su kien: {{.EventTitle}}
- Ma don hang: {{.OrderID}}
- Tong tien: {{.AmountFormatted}}
- Cong thanh toan: {{.Provider}} (Ma giao dich: {{.ProviderTxnID}})
- Thoi gian: {{.PaidAtFormatted}}

Ve dien tu va ma QR check-in se duoc phat hanh ngay sau thong bao nay.
Cam on ban da dong hanh cung EvenFlow!`

const ticketHTMLTmpl = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <style>
    body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; background-color: #f4f6f9; color: #1e293b; padding: 20px; }
    .card { max-width: 600px; margin: 0 auto; background: #ffffff; border-radius: 8px; box-shadow: 0 4px 6px -1px rgba(0,0,0,0.1); overflow: hidden; }
    .header { background: #047857; color: #ffffff; padding: 24px; text-align: center; }
    .content { padding: 24px; }
    .ticket-box { background: #f8fafc; border: 1px dashed #cbd5e1; border-radius: 6px; padding: 16px; margin-bottom: 16px; }
    .qr-text { font-family: monospace; background: #e2e8f0; padding: 6px; border-radius: 4px; word-break: break-all; font-size: 12px; }
    .footer { text-align: center; padding: 16px; color: #94a3b8; font-size: 12px; }
  </style>
</head>
<body>
  <div class="card">
    <div class="header">
      <h2>Vé Điện Tử EvenFlow</h2>
      <p>{{.EventTitle}}</p>
    </div>
    <div class="content">
      <p>Kính gửi quý khách, dưới đây là danh sách vé chính thức cho đơn hàng <strong>#{{.OrderID}}</strong> (Tổng cộng: {{.TicketCount}} vé):</p>
      
      {{range $i, $t := .Tickets}}
      <div class="ticket-box">
        <h4 style="margin: 0 0 8px 0; color: #047857;">Vé #{{$i}} - {{$t.ID}}</h4>
        <p style="margin: 4px 0; font-size: 14px;"><strong>Mã định danh JTI:</strong> {{$t.JTI}}</p>
        <p style="margin: 4px 0; font-size: 14px;"><strong>Chữ ký QR HMAC:</strong></p>
        <div class="qr-text">{{$t.QRSig}}</div>
      </div>
      {{end}}

      <div style="background: #fffbeb; border-left: 4px solid #f59e0b; padding: 12px; margin-top: 16px; font-size: 13px; color: #b45309;">
        <strong>Lưu ý quan trọng:</strong> Vui lòng xuất trình mã QR / JTI ở trên tại cổng soát vé. Mỗi vé chỉ được phép quét check-in đúng 1 lần (BR-O8).
      </div>
    </div>
    <div class="footer">
      Thoi gian phat hanh: {{.IssuedAtFormatted}} | EvenFlow Platform &copy; 2026.
    </div>
  </div>
</body>
</html>`

const ticketTextTmpl = `EvenFlow - Ve dien tu chinh thuc cho don hang #{{.OrderID}}
Su kien: {{.EventTitle}}
Tong so luong: {{.TicketCount}} ve
Thoi gian phat hanh: {{.IssuedAtFormatted}}

DANH SACH VE:
{{range $i, $t := .Tickets}}
--------------------------------------------------
Ve #{{$i}}
- Ma ve: {{$t.ID}}
- JTI: {{$t.JTI}}
- Chu ky QR HMAC: {{$t.QRSig}}
{{end}}
--------------------------------------------------
Luu y: Vui long xuat trinh ma QR hoac JTI tai cong soat ve. Moi ve chi co gia tri dung 1 lan.`

const refundHTMLTmpl = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <style>
    body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; background-color: #f4f6f9; color: #1e293b; padding: 20px; }
    .card { max-width: 600px; margin: 0 auto; background: #ffffff; border-radius: 8px; box-shadow: 0 4px 6px -1px rgba(0,0,0,0.1); overflow: hidden; }
    .header { background: #b91c1c; color: #ffffff; padding: 24px; text-align: center; }
    .content { padding: 24px; }
    .item-row { display: flex; justify-content: space-between; padding: 10px 0; border-bottom: 1px solid #f1f5f9; }
    .footer { text-align: center; padding: 16px; color: #94a3b8; font-size: 12px; }
  </style>
</head>
<body>
  <div class="card">
    <div class="header">
      <h2>Thông Báo Hoàn Tiền</h2>
      <p>EvenFlow Service</p>
    </div>
    <div class="content">
      <p>Yêu cầu hoàn tiền cho đơn hàng <strong>#{{.OrderID}}</strong> đã được xử lý hoàn tất:</p>
      
      <div style="margin-top: 16px;">
        <div class="item-row"><span>Số tiền hoàn:</span><strong style="color: #b91c1c;">{{.AmountFormatted}}</strong></div>
        <div class="item-row"><span>Lý do hoàn tiền:</span><strong>{{.Reason}}</strong></div>
        <div class="item-row"><span>Thời gian:</span><strong>{{.RefundedAtFormatted}}</strong></div>
      </div>
      
      <p style="margin-top: 20px; font-size: 14px; color: #475569;">
        Số tiền sẽ được ghi có về tài khoản ngân hàng hoặc ví điện tử ban đầu của quý khách trong vòng 1-3 ngày làm việc tùy thuộc chính sách ngân hàng.
      </p>
    </div>
    <div class="footer">
      EvenFlow Platform &copy; 2026.
    </div>
  </div>
</body>
</html>`

const refundTextTmpl = `EvenFlow - Thong bao hoan tien cho don hang #{{.OrderID}}

Yeu cau hoan tien da duoc he thong xu ly thanh cong:
- Ma don hang: {{.OrderID}}
- So tien hoan lai: {{.AmountFormatted}}
- Ly do: {{.Reason}}
- Thoi gian: {{.RefundedAtFormatted}}

Tien se ve tai khoan ban dau cua ban trong 1-3 ngay lam viec.`
