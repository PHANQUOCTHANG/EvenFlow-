package postgresadapter_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	postgresadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/waitingroom/internal/app"
)

func fromEnv(t *testing.T, dsn string) (app.Auditor, string) {
	t.Helper()
	logs := &bytes.Buffer{}
	aud, closeFn := postgresadapter.NewAuditorFromEnv(context.Background(), func(k string) string {
		if k == "DATABASE_URL" {
			return dsn
		}
		return ""
	}, slog.New(slog.NewTextHandler(logs, nil)))
	t.Cleanup(closeFn)
	return aud, logs.String()
}

// AC-12: thieu / rong / khong parse duoc -> interface nil THUAN (khong phai *Auditor nil).
func TestNewAuditorFromEnv_AC12_UnsetEmptyOrUnparsableIsPlainNil(t *testing.T) {
	for _, dsn := range []string{"", "   ", "::not a url"} {
		aud, logs := fromEnv(t, dsn)
		if aud != nil {
			t.Errorf("DATABASE_URL=%q: auditor = %#v, want plain nil", dsn, aud)
		}
		if strings.Contains(logs, "p@") {
			t.Errorf("DATABASE_URL=%q: credentials leaked into logs: %s", dsn, logs)
		}
	}
}

// AC-12 (CR-1): DB khong toi duoc luc khoi dong -> van giu auditor; ping that bai chi WARN.
func TestNewAuditorFromEnv_AC12_UnreachableKeepsAuditor(t *testing.T) {
	aud, logs := fromEnv(t, "postgres://u:secret@127.0.0.1:1/x?connect_timeout=1")
	if aud == nil {
		t.Fatal("a parsable but unreachable DATABASE_URL must keep the auditor")
	}
	if !strings.Contains(logs, "level=WARN") || strings.Contains(logs, "level=ERROR") {
		t.Fatalf("startup ping failure must be a WARN only:\n%s", logs)
	}
	if strings.Contains(logs, "secret") {
		t.Fatalf("password leaked into logs:\n%s", logs)
	}
	if err := aud.Record(context.Background(), app.AuditEntry{ActorType: "human", Action: "X", Subject: "event:x"}); err == nil {
		t.Fatal("Record against an unreachable database must fail")
	}
}
