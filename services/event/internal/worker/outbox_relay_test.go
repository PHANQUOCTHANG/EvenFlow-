package worker_test

import (
	"io"
	"log/slog"
	"testing"

	"github.com/eventflow/eventflow/services/event/internal/app"
	"github.com/eventflow/eventflow/services/event/internal/worker"
)

func TestDefaultRoutes_ModerationDiAiDirect(t *testing.T) {
	routes := worker.DefaultRoutes()
	if routes[app.ModerationRoutingKey] != "ai.direct" {
		t.Fatalf("job kiem duyet phai di exchange ai.direct, got %q", routes[app.ModerationRoutingKey])
	}
}

func TestNewOutboxRelay_CauHinhSaiThiLoi(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := worker.DefaultOutboxRelayConfig()
	if _, err := worker.NewOutboxRelay(nil, nil, worker.DefaultRoutes(), cfg, log); err == nil {
		t.Error("thieu db/publisher phai loi luc khoi dong")
	}
	bad := cfg
	bad.Batch = 0
	if _, err := worker.NewOutboxRelay(nil, nil, worker.DefaultRoutes(), bad, log); err == nil {
		t.Error("batch 0 phai loi")
	}
}

func TestDefaultOutboxRelayConfig_ChiLayDongCuaEvent(t *testing.T) {
	cfg := worker.DefaultOutboxRelayConfig()
	if cfg.AggregateType != app.OutboxAggregateEvent {
		t.Fatalf("relay event chi duoc lay aggregate_type %q, got %q", app.OutboxAggregateEvent, cfg.AggregateType)
	}
	if cfg.Interval <= 0 || cfg.Batch <= 0 {
		t.Fatalf("cau hinh mac dinh khong hop le: %+v", cfg)
	}
}
