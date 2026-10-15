package gateway

import (
	"fmt"
	"sync"

	"github.com/eventflow/eventflow/services/payment/internal/domain"
	"github.com/eventflow/eventflow/services/payment/internal/port"
)

// Registry quan ly va dieu phoi cac cong thanh toan duoc kich hoat trong he thong.
type Registry struct {
	mu              sync.RWMutex
	gateways        map[domain.Provider]port.PaymentGateway
	enabled         map[domain.Provider]bool
	defaultProvider domain.Provider
}

// NewRegistry khoi tao registry voi cong mac dinh va danh sach cong duoc kich hoat.
func NewRegistry(defaultProvider domain.Provider, enabledList []string) *Registry {
	enabledMap := make(map[domain.Provider]bool)
	for _, p := range enabledList {
		normalized := domain.NormalizeProvider(p)
		if domain.IsValidProvider(normalized) {
			enabledMap[normalized] = true
		}
	}

	normDefault := domain.NormalizeProvider(string(defaultProvider))
	if !domain.IsValidProvider(normDefault) {
		normDefault = domain.ProviderSandbox
	}
	enabledMap[normDefault] = true

	return &Registry{
		gateways:        make(map[domain.Provider]port.PaymentGateway),
		enabled:         enabledMap,
		defaultProvider: normDefault,
	}
}

// Register dang ky mot cong thanh toan vao he thong.
func (r *Registry) Register(gw port.PaymentGateway) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gateways[gw.Provider()] = gw
}

// Get tra ve cong thanh toan tuong ung. Neu khong truyen provider, dung cong mac dinh.
func (r *Registry) Get(p domain.Provider) (port.PaymentGateway, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	target := p
	if target == "" {
		target = r.defaultProvider
	}

	if !domain.IsValidProvider(target) {
		return nil, fmt.Errorf("%w: %s", domain.ErrUnsupportedProvider, p)
	}

	if !r.enabled[target] {
		return nil, fmt.Errorf("%w: %s", domain.ErrProviderDisabled, target)
	}

	gw, exists := r.gateways[target]
	if !exists {
		return nil, fmt.Errorf("%w: chua dang ky hien thuc cho %s", domain.ErrUnsupportedProvider, target)
	}

	return gw, nil
}

// DefaultProvider tra ve cong mac dinh.
func (r *Registry) DefaultProvider() domain.Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.defaultProvider
}

// ListEnabled tra ve danh sach cac phuong thuc thanh toan san sang phuc vu khach hang.
func (r *Registry) ListEnabled() []domain.PaymentMethodInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	catalog := map[domain.Provider]domain.PaymentMethodInfo{
		domain.ProviderVNPay: {
			Code:        domain.ProviderVNPay,
			Name:        "VNPay",
			Description: "Thanh toán qua ứng dụng ngân hàng hoặc ví VNPAY-QR",
			Icon:        "vnpay-icon",
		},
		domain.ProviderMoMo: {
			Code:        domain.ProviderMoMo,
			Name:        "Ví MoMo",
			Description: "Thanh toán siêu tốc qua ví điện tử MoMo",
			Icon:        "momo-icon",
		},
		domain.ProviderStripe: {
			Code:        domain.ProviderStripe,
			Name:        "Thẻ quốc tế (Visa/Mastercard)",
			Description: "Thanh toán an toàn qua cổng bảo mật Stripe",
			Icon:        "credit-card-icon",
		},
		domain.ProviderSandbox: {
			Code:        domain.ProviderSandbox,
			Name:        "Sandbox Mock",
			Description: "Môi trường giả lập thanh toán tức thì phục vụ kiểm thử",
			Icon:        "sandbox-icon",
		},
	}

	order := []domain.Provider{
		domain.ProviderVNPay,
		domain.ProviderMoMo,
		domain.ProviderStripe,
		domain.ProviderSandbox,
	}

	var res []domain.PaymentMethodInfo
	for _, p := range order {
		if r.enabled[p] {
			if _, registered := r.gateways[p]; registered {
				info := catalog[p]
				info.Enabled = true
				res = append(res, info)
			}
		}
	}

	return res
}
