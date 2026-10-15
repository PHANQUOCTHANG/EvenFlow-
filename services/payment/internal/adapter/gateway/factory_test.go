package gateway_test

import (
	"errors"
	"testing"

	"github.com/eventflow/eventflow/services/payment/internal/adapter/gateway"
	"github.com/eventflow/eventflow/services/payment/internal/domain"
)

func TestGatewayRegistry_ResolveAndDefault(t *testing.T) {
	reg := gateway.NewRegistry(domain.ProviderVNPay, []string{"vnpay", "momo", "sandbox"})

	sbx := gateway.NewSandboxGateway(gateway.SandboxModeSuccess)
	vnp := gateway.NewVNPayGateway(gateway.VNPayConfig{})
	momo := gateway.NewMoMoGateway(gateway.MoMoConfig{})

	reg.Register(sbx)
	reg.Register(vnp)
	reg.Register(momo)

	// 1. Resolve explicit provider
	gw, err := reg.Get(domain.ProviderVNPay)
	if err != nil {
		t.Fatalf("failed to get vnpay: %v", err)
	}
	if gw.Provider() != domain.ProviderVNPay {
		t.Errorf("expected vnpay, got %s", gw.Provider())
	}

	// 2. Resolve with empty provider -> falls back to default (VNPay)
	defaultGW, err := reg.Get("")
	if err != nil {
		t.Fatalf("failed to get default gateway: %v", err)
	}
	if defaultGW.Provider() != domain.ProviderVNPay {
		t.Errorf("expected default vnpay, got %s", defaultGW.Provider())
	}

	// 3. Resolve disabled provider (stripe is not in enabled list)
	_, err = reg.Get(domain.ProviderStripe)
	if !errors.Is(err, domain.ErrProviderDisabled) {
		t.Errorf("expected ErrProviderDisabled for stripe, got %v", err)
	}

	// 4. Resolve invalid provider
	_, err = reg.Get(domain.Provider("bitcoin"))
	if !errors.Is(err, domain.ErrUnsupportedProvider) {
		t.Errorf("expected ErrUnsupportedProvider for bitcoin, got %v", err)
	}
}

func TestGatewayRegistry_ListEnabled(t *testing.T) {
	reg := gateway.NewRegistry(domain.ProviderSandbox, []string{"sandbox", "vnpay"})

	sbx := gateway.NewSandboxGateway(gateway.SandboxModeSuccess)
	vnp := gateway.NewVNPayGateway(gateway.VNPayConfig{})
	reg.Register(sbx)
	reg.Register(vnp)

	methods := reg.ListEnabled()
	if len(methods) != 2 {
		t.Fatalf("expected 2 enabled methods, got %d", len(methods))
	}

	hasVNPay := false
	hasSandbox := false
	for _, m := range methods {
		if m.Code == domain.ProviderVNPay {
			hasVNPay = true
		}
		if m.Code == domain.ProviderSandbox {
			hasSandbox = true
		}
	}
	if !hasVNPay || !hasSandbox {
		t.Errorf("missing expected methods in list: %+v", methods)
	}
}
