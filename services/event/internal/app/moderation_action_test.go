package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/event/internal/app"
	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/google/uuid"
)

type modEnv struct {
	events *fakeEventRepo
	mod    *fakeModerationRepo
	audit  *fakeAuditRepo
	store  *fakeDecisionStore
	uc     app.ModeratorActionUseCase
}

func newModEnv() modEnv {
	e, m, a := newFakeRepo(), newFakeModerationRepo(), newFakeAuditRepo()
	s := newFakeDecisionStore(e, m, a)
	return modEnv{events: e, mod: m, audit: a, store: s, uc: app.NewModeratorActionUseCase(e, m, s)}
}

func addReview(t *testing.T, m *fakeModerationRepo, eventID uuid.UUID, v domain.AiVerdict, score float64) *domain.ModerationReview {
	t.Helper()
	rev := &domain.ModerationReview{ID: uuid.New(), EventID: eventID, AiScore: score, AiVerdict: v, CreatedAt: time.Now().UTC()}
	_ = m.Create(context.Background(), rev)
	return rev
}

// Su kien AI da tu choi (EV-120): REJECTED + review AUTO_REJECTED.
func aiRejectedEvent(t *testing.T, env modEnv) *domain.Event {
	t.Helper()
	evt := createPendingEvent(t, env.events)
	_ = evt.Reject()
	_ = env.events.Update(context.Background(), evt)
	addReview(t, env.mod, evt.ID, domain.VerdictAutoRejected, 0.9)
	return evt
}

func TestModeratorApprove_FromPending(t *testing.T) {
	ctx := context.Background()
	env := newModEnv()
	evt := createPendingEvent(t, env.events)
	addReview(t, env.mod, evt.ID, domain.VerdictNeedsHuman, 0.6)
	modID := uuid.New()

	approved, err := env.uc.Approve(ctx, app.ModeratorApproveInput{EventID: evt.ID, ModeratorID: modID, Note: "Da xac minh giay phep"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if approved.Status != domain.StatusScheduled {
		t.Errorf("expected SCHEDULED, got %s", approved.Status)
	}
	rev, _ := env.mod.GetLatestByEventID(ctx, evt.ID)
	if rev.HumanVerdict == nil || *rev.HumanVerdict != "APPROVED" || rev.HumanID == nil || *rev.HumanID != modID {
		t.Errorf("review chua ghi quyet dinh nguoi: %+v", rev)
	}
	logs, _ := env.audit.ListBySubject(ctx, "event:"+evt.ID.String())
	if len(logs) != 1 || logs[0].ActorType != "human" || logs[0].Action != "event.moderator_approved" {
		t.Fatalf("audit sai: %+v", logs)
	}
}

func TestModeratorApprove_NeedsHumanKhongCanLyDo(t *testing.T) {
	env := newModEnv()
	evt := createPendingEvent(t, env.events)
	addReview(t, env.mod, evt.ID, domain.VerdictNeedsHuman, 0.5)

	if _, err := env.uc.Approve(context.Background(), app.ModeratorApproveInput{EventID: evt.ID, ModeratorID: uuid.New()}); err != nil {
		t.Fatalf("duyet ho so NEEDS_HUMAN khong bat buoc ly do: %v", err)
	}
}

// AC-1 Jira: lat nguoc quyet dinh AI phai co ly do.
func TestModeratorApprove_LatNguocAI_ThieuLyDo_BiTuChoi(t *testing.T) {
	ctx := context.Background()
	env := newModEnv()
	evt := aiRejectedEvent(t, env)

	_, err := env.uc.Approve(ctx, app.ModeratorApproveInput{EventID: evt.ID, ModeratorID: uuid.New(), Note: "   "})
	if !errors.Is(err, domain.ErrOverrideReasonRequired) {
		t.Fatalf("expected ErrOverrideReasonRequired, got %v", err)
	}
	if len(env.store.calls) != 0 {
		t.Fatal("thieu ly do thi khong duoc ghi gi")
	}
	if st := env.events.committed(evt.ID); st != domain.StatusRejected {
		t.Errorf("su kien phai giu REJECTED, got %s", st)
	}
}

func TestModeratorApprove_LatNguocAI_CoLyDo_GhiAuditRieng(t *testing.T) {
	ctx := context.Background()
	env := newModEnv()
	evt := aiRejectedEvent(t, env)

	approved, err := env.uc.Approve(ctx, app.ModeratorApproveInput{
		EventID: evt.ID, ModeratorID: uuid.New(), Note: "AI nham: giay phep hop le",
	})
	if err != nil {
		t.Fatalf("lat nguoc co ly do: %v", err)
	}
	if approved.Status != domain.StatusScheduled {
		t.Errorf("expected SCHEDULED, got %s", approved.Status)
	}
	logs, _ := env.audit.ListBySubject(ctx, "event:"+evt.ID.String())
	if len(logs) != 1 {
		t.Fatalf("expected 1 audit, got %d", len(logs))
	}
	l := logs[0]
	if l.Action != "event.moderator_overrode_ai" {
		t.Errorf("lat nguoc AI phai co action rieng, got %s", l.Action)
	}
	if l.Reason == nil || *l.Reason != "AI nham: giay phep hop le" {
		t.Errorf("audit phai co ly do (AC-1), got %v", l.Reason)
	}
	meta, _ := l.Metadata.(map[string]any)
	if meta["ai_verdict"] != string(domain.VerdictAutoRejected) || meta["override"] != true {
		t.Errorf("metadata thieu ai_verdict/override: %v", l.Metadata)
	}
}

func TestModeratorReject_Success(t *testing.T) {
	ctx := context.Background()
	env := newModEnv()
	evt := createPendingEvent(t, env.events)
	addReview(t, env.mod, evt.ID, domain.VerdictNeedsHuman, 0.5)

	rejected, err := env.uc.Reject(ctx, app.ModeratorRejectInput{EventID: evt.ID, ModeratorID: uuid.New(), Reason: "Thieu giay phep"})
	if err != nil {
		t.Fatalf("unexpected error rejecting: %v", err)
	}
	if rejected.Status != domain.StatusRejected {
		t.Errorf("expected REJECTED, got %s", rejected.Status)
	}
	logs, _ := env.audit.ListBySubject(ctx, "event:"+evt.ID.String())
	if len(logs) != 1 || logs[0].Action != "event.moderator_rejected" || logs[0].Reason == nil || *logs[0].Reason != "Thieu giay phep" {
		t.Fatalf("audit tu choi sai: %+v", logs)
	}
}

func TestModeratorReject_MissingReason_Fails(t *testing.T) {
	env := newModEnv()
	evt := createPendingEvent(t, env.events)

	_, err := env.uc.Reject(context.Background(), app.ModeratorRejectInput{EventID: evt.ID, ModeratorID: uuid.New(), Reason: "   "})
	if !errors.Is(err, domain.ErrRejectReasonRequired) {
		t.Errorf("expected ErrRejectReasonRequired, got %v", err)
	}
}

// AC-3: buoc ghi hong -> loi tra ve (khong nuot), khong co gi duoc ghi.
func TestModeratorDecision_GhiHong_TraLoiKhongNuot(t *testing.T) {
	ctx := context.Background()
	env := newModEnv()
	evt := createPendingEvent(t, env.events)
	addReview(t, env.mod, evt.ID, domain.VerdictNeedsHuman, 0.5)
	env.store.err = errors.New("db sap giua transaction")

	if _, err := env.uc.Reject(ctx, app.ModeratorRejectInput{EventID: evt.ID, ModeratorID: uuid.New(), Reason: "x"}); err == nil {
		t.Fatal("loi ghi phai tra ve cho nguoi goi")
	}
	if st := env.events.committed(evt.ID); st != domain.StatusPendingReview || len(env.audit.logs) != 0 {
		t.Errorf("khong duoc ghi gi khi transaction hong: status=%s audit=%d", st, len(env.audit.logs))
	}
}

// Duyet lai su kien ma Moderator khac da tu choi = lat nguoc quyet dinh nguoi: can ly do, audit rieng.
func TestModeratorApprove_LatNguocQuyetDinhNguoi_CanLyDo(t *testing.T) {
	ctx := context.Background()
	env := newModEnv()
	evt := createPendingEvent(t, env.events)
	addReview(t, env.mod, evt.ID, domain.VerdictNeedsHuman, 0.5)
	if _, err := env.uc.Reject(ctx, app.ModeratorRejectInput{EventID: evt.ID, ModeratorID: uuid.New(), Reason: "nguoi 1"}); err != nil {
		t.Fatalf("nguoi 1: %v", err)
	}

	if _, err := env.uc.Approve(ctx, app.ModeratorApproveInput{EventID: evt.ID, ModeratorID: uuid.New()}); !errors.Is(err, domain.ErrOverrideReasonRequired) {
		t.Fatalf("lat nguoc quyet dinh nguoi thieu ly do phai bi chan, got %v", err)
	}
	if _, err := env.uc.Approve(ctx, app.ModeratorApproveInput{EventID: evt.ID, ModeratorID: uuid.New(), Note: "BTC da bo sung giay phep"}); err != nil {
		t.Fatalf("co ly do: %v", err)
	}
	last := env.audit.logs[len(env.audit.logs)-1]
	if last.Action != "event.moderator_overrode_decision" {
		t.Errorf("action = %s", last.Action)
	}
	if meta, _ := last.Metadata.(map[string]any); meta["previous_human_verdict"] != "REJECTED" {
		t.Errorf("metadata phai ghi quyet dinh nguoi truoc do: %v", last.Metadata)
	}
}

// Quyet dinh dua tren trang thai cu (nguoi khac vua quyet): store tra ErrAlreadyDecided, khong nuot.
func TestModeratorDecision_TranhChap_TraLoiDaQuyet(t *testing.T) {
	env := newModEnv()
	evt := createPendingEvent(t, env.events)
	env.store.err = domain.ErrAlreadyDecided

	_, err := env.uc.Reject(context.Background(), app.ModeratorRejectInput{EventID: evt.ID, ModeratorID: uuid.New(), Reason: "x"})
	if !errors.Is(err, domain.ErrAlreadyDecided) {
		t.Fatalf("expected ErrAlreadyDecided, got %v", err)
	}
}

func TestListModerationQueue_GomCanNguoiDuyetVaAIDaTuChoi(t *testing.T) {
	ctx := context.Background()
	env := newModEnv()

	needs := createPendingEvent(t, env.events)
	addReview(t, env.mod, needs.ID, domain.VerdictNeedsHuman, 0.65)
	rejected := aiRejectedEvent(t, env)
	approved := createPendingEvent(t, env.events)
	addReview(t, env.mod, approved.ID, domain.VerdictAutoApproved, 0.1)
	// Review cu NEEDS_HUMAN nhung su kien da ve DRAFT (BTC sua lai): khong con trong hang doi.
	stale := createPendingEvent(t, env.events)
	addReview(t, env.mod, stale.ID, domain.VerdictNeedsHuman, 0.5)
	stale.Status = domain.StatusDraft
	_ = env.events.Update(ctx, stale)

	items, err := app.NewListModerationQueueUseCase(env.events, env.mod).Execute(ctx)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	got := map[uuid.UUID]domain.AiVerdict{}
	for _, it := range items {
		got[it.EventID] = it.Review.AiVerdict
	}
	if len(got) != 2 || got[needs.ID] != domain.VerdictNeedsHuman || got[rejected.ID] != domain.VerdictAutoRejected {
		t.Fatalf("hang doi phai gom NEEDS_HUMAN + AUTO_REJECTED dang hieu luc, got %v", got)
	}
}

func TestListModerationQueue_DaQuyetThiKhongConTrongHang(t *testing.T) {
	ctx := context.Background()
	env := newModEnv()
	evt := createPendingEvent(t, env.events)
	addReview(t, env.mod, evt.ID, domain.VerdictNeedsHuman, 0.5)
	_, _ = env.uc.Reject(ctx, app.ModeratorRejectInput{EventID: evt.ID, ModeratorID: uuid.New(), Reason: "x"})

	items, _ := app.NewListModerationQueueUseCase(env.events, env.mod).Execute(ctx)
	if len(items) != 0 {
		t.Fatalf("ho so da quyet khong duoc con trong hang, got %d", len(items))
	}
}

// Chay that (evidence §2): Moderator thu hai den SAU khi nguoi dau da commit thi doc thay su kien da
// doi trang thai. Phai la ErrAlreadyDecided (409 "nguoi khac vua quyet"), khong phai loi chuyen
// trang thai chung chung (422) -- Moderator khong hieu vi sao "khong hop le".
func TestModeratorDecision_NguoiDenSau_LaDaQuyetChuKhongPhaiLoiTrangThai(t *testing.T) {
	ctx := context.Background()
	env := newModEnv()
	evt := aiRejectedEvent(t, env)
	if _, err := env.uc.Approve(ctx, app.ModeratorApproveInput{EventID: evt.ID, ModeratorID: uuid.New(), Note: "nguoi 1"}); err != nil {
		t.Fatalf("nguoi 1: %v", err)
	}

	_, err := env.uc.Approve(ctx, app.ModeratorApproveInput{EventID: evt.ID, ModeratorID: uuid.New(), Note: "nguoi 2"})
	if !errors.Is(err, domain.ErrAlreadyDecided) {
		t.Fatalf("duyet su kien da SCHEDULED: expected ErrAlreadyDecided, got %v", err)
	}
	_, err = env.uc.Reject(ctx, app.ModeratorRejectInput{EventID: evt.ID, ModeratorID: uuid.New(), Reason: "nguoi 3"})
	if !errors.Is(err, domain.ErrAlreadyDecided) {
		t.Fatalf("tu choi su kien da SCHEDULED: expected ErrAlreadyDecided, got %v", err)
	}
}

func TestModeratorReject_SuKienDaBiTuChoi_LaDaQuyet(t *testing.T) {
	env := newModEnv()
	evt := aiRejectedEvent(t, env)
	_, err := env.uc.Reject(context.Background(), app.ModeratorRejectInput{EventID: evt.ID, ModeratorID: uuid.New(), Reason: "x"})
	if !errors.Is(err, domain.ErrAlreadyDecided) {
		t.Fatalf("expected ErrAlreadyDecided, got %v", err)
	}
}

// Trang thai khong phai do kiem duyet quyet (DRAFT chua nop, CANCELLED): van la loi chuyen trang thai.
func TestModeratorDecision_SuKienChuaNop_VanLaLoiTrangThai(t *testing.T) {
	ctx := context.Background()
	env := newModEnv()
	evt := createPendingEvent(t, env.events)
	evt.Status = domain.StatusDraft
	_ = env.events.Update(ctx, evt)

	_, err := env.uc.Approve(ctx, app.ModeratorApproveInput{EventID: evt.ID, ModeratorID: uuid.New()})
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("expected ErrInvalidStateTransition, got %v", err)
	}
}
