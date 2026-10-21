package worker_test

import (
	"slices"
	"testing"

	"github.com/eventflow/eventflow/services/payment/internal/worker"
)

// Moi routing key payment ghi vao outbox (adapter/postgres/repository.go) phai nam trong danh
// sach relay, neu khong dong do nam mai trong outbox_events.
func TestDefaultConfig_PhuMoiRoutingKeyCuaPayment(t *testing.T) {
	cfg := worker.DefaultOutboxRelayConfig()
	for _, k := range []string{"order.paid", "hold.released", "payment.refunded"} {
		if !slices.Contains(cfg.RoutingKeys, k) {
			t.Errorf("thieu routing key %q trong %v", k, cfg.RoutingKeys)
		}
	}
	for _, k := range cfg.RoutingKeys {
		if len(k) >= len("ticketing.") && k[:len("ticketing.")] == "ticketing." {
			t.Errorf("relay payment khong duoc lay dong cua ticketing: %q", k)
		}
	}
	if cfg.Batch <= 0 || cfg.Interval <= 0 {
		t.Errorf("cau hinh mac dinh sai: %+v", cfg)
	}
}
