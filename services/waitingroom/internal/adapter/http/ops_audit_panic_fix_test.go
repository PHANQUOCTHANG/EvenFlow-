package httpadapter_test

// ops-warroom fix round 1: X-2 (auditor panic after apply).

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	httpadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/http"
	"github.com/eventflow/eventflow/services/waitingroom/internal/app"
)

type panicAuditor struct{}

func (panicAuditor) Record(context.Context, app.AuditEntry) error { panic("pg driver panic") }

// X-2: an auditor panic after the apply is recovered: 200 audit:"failed" and one ERROR line with the audit fields.
func TestOpsWriteFix_X2_AuditorPanicIsRecoveredAsFailed(t *testing.T) {
	st := &fakeOverrideStore{configured: true}
	logs := &bytes.Buffer{}
	log := slog.New(slog.NewTextHandler(logs, nil))
	mux := http.NewServeMux()
	httpadapter.NewOpsHandler(&fakeState{}, app.NewOpsOverride(st, panicAuditor{}, log), log).Routes(mux)

	var rec interface {
		Result() *http.Response
	}
	func() {
		defer func() {
			if p := recover(); p != nil {
				t.Fatalf("panic escaped the handler: %v", p)
			}
		}()
		rec = opsCall(mux, http.MethodPut, ovPath(opsEvent), "ops", `{"rate_per_sec":0,"reason":"khan cap"}`)
	}()
	resp := rec.Result()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	body := new(bytes.Buffer)
	_, _ = body.ReadFrom(resp.Body)
	if !strings.Contains(body.String(), `"audit":"failed"`) {
		t.Fatalf("want audit failed: %s", body)
	}
	if !st.active || st.rate != 0 {
		t.Fatal("override must stay applied")
	}
	var errLine string
	for _, l := range strings.Split(logs.String(), "\n") {
		if strings.Contains(l, "level=ERROR") {
			errLine = l
		}
	}
	for _, want := range []string{opsActor, "ADMIT_PAUSED", "event:" + opsEvent, "khan cap", "req-123"} {
		if !strings.Contains(errLine, want) {
			t.Errorf("ERROR line lacks %q: %q", want, errLine)
		}
	}
}
