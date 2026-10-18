package metricsadapter_test

import (
	"testing"
	"time"

	metricsadapter "github.com/eventflow/eventflow/services/payment/internal/adapter/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

func TestProm_WebhookLagAndReconciliation(t *testing.T) {
	reg := prometheus.NewRegistry()
	prom := metricsadapter.New(reg)

	// Ghi nhan metrics
	prom.ObserveWebhookLag(950 * time.Second)
	prom.IncReconciliation("healed_paid")
	prom.IncReconciliation("healed_cancelled")
	prom.IncReconciliation("healed_cancelled")
	prom.ObserveReconciliationDuration(120 * time.Millisecond)

	// Thu thap toan bo metrics dang text de kiem tra su ton tai cua ten metric
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather metrics failed: %v", err)
	}

	foundLag := false
	foundTotal := false
	foundDur := false

	for _, mf := range mfs {
		switch mf.GetName() {
		case "payment_webhook_lag_seconds":
			foundLag = true
		case "payment_reconciliation_total":
			foundTotal = true
			for _, m := range mf.GetMetric() {
				for _, l := range m.GetLabel() {
					if l.GetName() == "outcome" && l.GetValue() == "healed_cancelled" {
						if m.GetCounter().GetValue() != 2 {
							t.Errorf("expected 2 healed_cancelled, got %v", m.GetCounter().GetValue())
						}
					}
				}
			}
		case "payment_reconciliation_duration_seconds":
			foundDur = true
		}
	}

	if !foundLag {
		t.Errorf("missing metric payment_webhook_lag_seconds")
	}
	if !foundTotal {
		t.Errorf("missing metric payment_reconciliation_total")
	}
	if !foundDur {
		t.Errorf("missing metric payment_reconciliation_duration_seconds")
	}
}

func TestProm_NilSafe(t *testing.T) {
	var nilProm *metricsadapter.Prom
	// Khong duoc panic khi goi tren nil
	nilProm.ObserveWebhookLag(10 * time.Second)
	nilProm.IncReconciliation("error")
	nilProm.ObserveReconciliationDuration(50 * time.Millisecond)
}
