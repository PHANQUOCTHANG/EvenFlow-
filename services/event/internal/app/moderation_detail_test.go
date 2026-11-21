package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/event/internal/app"
	"github.com/eventflow/eventflow/services/event/internal/domain"
)

func TestModerationDetail_GomSuKienReviewTrucVaAudit(t *testing.T) {
	ctx := context.Background()
	env := newModEnv()
	evt := createPendingEvent(t, env.events)
	addReview(t, env.mod, evt.ID, domain.VerdictNeedsHuman, 0.55)
	subject := "event:" + evt.ID.String()
	t0 := time.Now().UTC()
	// Audit AI cu (truc cu) roi audit AI moi: phai lay truc cua ban MOI NHAT.
	_ = env.audit.Create(ctx, &domain.AuditLog{ActorType: "ai", Action: "event.needs_human_review", Subject: subject,
		Metadata: map[string]any{"axes": map[string]any{"misleading": 0.9}}, CreatedAt: t0.Add(-time.Hour)})
	_ = env.audit.Create(ctx, &domain.AuditLog{ActorType: "ai", Action: "event.needs_human_review", Subject: subject,
		Metadata:  map[string]any{"axes": map[string]any{"forbidden_content": 0.1, "misleading": 0.5, "copyright": 0.3, "incomplete_info": 0.6}},
		CreatedAt: t0})
	_ = env.audit.Create(ctx, &domain.AuditLog{ActorType: "system", Action: "event.submitted", Subject: subject, CreatedAt: t0.Add(-2 * time.Hour)})

	d, err := app.NewGetModerationDetailUseCase(env.events, env.mod, env.audit).Execute(ctx, evt.ID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if d.Event.ID != evt.ID || len(d.Event.Tiers) == 0 {
		t.Errorf("phai co su kien kem hang ve: %+v", d.Event)
	}
	if d.Review == nil || d.Review.AiScore != 0.55 {
		t.Errorf("review sai: %+v", d.Review)
	}
	want := map[string]float64{"forbidden_content": 0.1, "misleading": 0.5, "copyright": 0.3, "incomplete_info": 0.6}
	if len(d.AiAxes) != 4 {
		t.Fatalf("4 truc tu audit AI moi nhat, got %v", d.AiAxes)
	}
	for k, v := range want {
		if d.AiAxes[k] != v {
			t.Errorf("truc %s = %v, want %v", k, d.AiAxes[k], v)
		}
	}
	// Lich su theo thoi gian tang dan, khong phu thuoc thu tu repo tra ve.
	for i := 1; i < len(d.Audit); i++ {
		if d.Audit[i].CreatedAt.Before(d.Audit[i-1].CreatedAt) {
			t.Fatalf("audit phai sap tang dan theo thoi gian")
		}
	}
	if len(d.Audit) != 3 {
		t.Errorf("expected 3 audit, got %d", len(d.Audit))
	}
}

func TestModerationDetail_ChuaCoReviewVaTruc_VanTraVe(t *testing.T) {
	env := newModEnv()
	evt := createPendingEvent(t, env.events)

	d, err := app.NewGetModerationDetailUseCase(env.events, env.mod, env.audit).Execute(context.Background(), evt.ID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if d.Review != nil || len(d.AiAxes) != 0 || len(d.Audit) != 0 {
		t.Errorf("thieu du lieu AI thi de trong, khong loi: %+v", d)
	}
}

func TestModerationDetail_KhongTonTai(t *testing.T) {
	env := newModEnv()
	_, err := app.NewGetModerationDetailUseCase(env.events, env.mod, env.audit).Execute(context.Background(), [16]byte{1})
	if !errors.Is(err, domain.ErrEventNotFound) {
		t.Fatalf("expected ErrEventNotFound, got %v", err)
	}
}
