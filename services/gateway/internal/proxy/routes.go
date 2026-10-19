// Package proxy la loi cua gateway: bang route, xac thuc access token, gan danh tinh, gioi han
// toc do va chuyen tiep toi service phia sau.
package proxy

import "github.com/eventflow/eventflow/services/gateway/internal/config"

// Access la muc bao ve cua mot route.
type Access int

const (
	// Public: khong can token. Token hop le (neu co) van duoc gan danh tinh; token hong bi bo
	// qua nhu khach vang lai (vd trang chi tiet su kien khi token vua het han).
	Public Access = iota
	// Authenticated: bat buoc access token hop le.
	Authenticated
	// Blocked: khong bao gio chuyen tiep (route noi bo, metric, admin). Tra 404 de khong lo
	// su ton tai cua route.
	Blocked
)

// Route mo ta mot nhom duong dan. Pattern theo cu phap http.ServeMux (Go 1.22+).
type Route struct {
	Pattern  string
	Upstream string
	Access   Access
	Roles    []string // rong = moi role da dang nhap
	Rate     string   // lop gioi han toc do (config.Rate*)
}

// Routes la bang dinh tuyen DUY NHAT cua gateway. Gateway chi kiem phan "ai" (xac thuc) va cac
// route chi danh cho mot role ro rang; quyen chi tiet (chu su kien, trang thai) van do service
// tu kiem (EV-114).
func Routes() []Route {
	return []Route{
		// Identity: dang ky, dang nhap, OTP, refresh. /me va /logout-all do identity tu xac minh
		// token (Authorization duoc chuyen nguyen).
		{Pattern: "/api/v1/auth/", Upstream: config.Identity, Access: Public, Rate: config.RateAuth},

		// Event: xem chi tiet cong khai; tao/sua/nop duyet/duyet can dang nhap.
		{Pattern: "GET /api/v1/events/{id}", Upstream: config.Event, Access: Public, Rate: config.RateDefault},
		{Pattern: "GET /api/v1/events/slug/{slug}", Upstream: config.Event, Access: Public, Rate: config.RateDefault},
		{Pattern: "/api/v1/events", Upstream: config.Event, Access: Authenticated, Rate: config.RateDefault},
		{Pattern: "/api/v1/events/", Upstream: config.Event, Access: Authenticated, Rate: config.RateDefault},
		{Pattern: "/api/v1/moderation/", Upstream: config.Event, Access: Authenticated, Roles: []string{"moderator"}, Rate: config.RateDefault},

		// Phong cho: poll status co lop rieng, chan poll day hon poll_after_ms (BR-Q4).
		{Pattern: "GET /v1/events/{eventID}/queue/status", Upstream: config.Waitingroom, Access: Authenticated, Rate: config.RateQueuePoll},
		{Pattern: "/v1/events/{eventID}/queue/", Upstream: config.Waitingroom, Access: Authenticated, Rate: config.RateDefault},

		// Ticketing.
		{Pattern: "/v1/events/{eventID}/holds", Upstream: config.Ticketing, Access: Authenticated, Rate: config.RateDefault},
		{Pattern: "/v1/events/{eventID}/purchase-limit", Upstream: config.Ticketing, Access: Authenticated, Rate: config.RateDefault},
		{Pattern: "/v1/orders/", Upstream: config.Ticketing, Access: Authenticated, Rate: config.RateDefault},
		{Pattern: "/v1/holds/", Upstream: config.Ticketing, Access: Authenticated, Rate: config.RateDefault},
		// Muc con ve cong khai (khong lo so chinh xac); ticketing gom request va dat Cache-Control.
		{Pattern: "GET /v1/events/{eventID}/availability", Upstream: config.Ticketing, Access: Public, Rate: config.RateDefault},
		{Pattern: "/v1/tickets/", Upstream: config.Ticketing, Access: Authenticated, Rate: config.RateDefault},

		// Payment. Webhook cong khai vi cong thanh toan goi vao; payment tu xac minh chu ky.
		{Pattern: "/v1/events/{eventID}/payments", Upstream: config.Payment, Access: Authenticated, Rate: config.RateDefault},
		{Pattern: "/v1/payments/webhook/", Upstream: config.Payment, Access: Public, Rate: config.RateDefault},
		{Pattern: "GET /v1/payments/methods", Upstream: config.Payment, Access: Public, Rate: config.RateDefault},
		{Pattern: "/v1/payments/inbox/", Upstream: config.Payment, Access: Authenticated, Roles: []string{"ops"}, Rate: config.RateDefault},
		{Pattern: "/v1/payments/reconcile", Upstream: config.Payment, Access: Authenticated, Roles: []string{"ops"}, Rate: config.RateDefault},
		{Pattern: "/v1/payments/{paymentID}", Upstream: config.Payment, Access: Authenticated, Rate: config.RateDefault},

		// Tro ly AI (ai-worker).
		{Pattern: "/v1/events/{eventID}/assistant/", Upstream: config.Assistant, Access: Public, Rate: config.RateDefault},
		{Pattern: "/v1/assistant/", Upstream: config.Assistant, Access: Public, Rate: config.RateDefault},

		// Khong bao gio ra ngoai: callback noi bo (vd PATCH /internal/v1/events/{id}/ai-verdict),
		// metric, cong admin cua ai-worker.
		{Pattern: "/internal/", Access: Blocked},
		{Pattern: "/metrics", Access: Blocked},
		{Pattern: "/admin/", Access: Blocked},
	}
}
