//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/eventflow/eventflow/libs/go/testx"
	"github.com/eventflow/eventflow/services/event/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/eventflow/eventflow/services/event/internal/port"
)

type modFixture struct {
	db     *sql.DB
	events port.EventRepository
	mod    port.ModerationRepository
	audit  port.AuditLogRepository
	store  port.ModerationDecisionStore
}

func newModFixture(t *testing.T) modFixture {
	t.Helper()
	db, err := sql.Open("postgres", testx.Postgres(t))
	if err != nil {
		t.Fatalf("mo postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return modFixture{
		db: db, events: postgres.NewEventRepository(db), mod: postgres.NewModerationRepository(db),
		audit: postgres.NewAuditLogRepository(db), store: postgres.NewModerationDecisionStore(db),
	}
}

// Su kien PENDING_REVIEW (hoac REJECTED) kem review AI chua co quyet dinh nguoi.
func seedReviewed(t *testing.T, f modFixture, status domain.EventStatus, verdict domain.AiVerdict) (*domain.Event, *domain.ModerationReview) {
	t.Helper()
	ctx := context.Background()
	org := uuid.New()
	if _, err := f.db.Exec(`INSERT INTO identities (id, email, password_hash, role) VALUES ($1, $2, 'x', 'organizer')`,
		org, org.String()+"@t.local"); err != nil {
		t.Fatalf("organizer: %v", err)
	}
	evt, err := domain.NewEvent(org, "mod-"+org.String()[:8], "Ho so", "Mo ta", "SVD", time.Now().Add(72*time.Hour), time.Now().Add(24*time.Hour), nil)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	evt.Status = status
	if err := f.events.Create(ctx, evt); err != nil {
		t.Fatalf("Create: %v", err)
	}
	rev := &domain.ModerationReview{ID: uuid.New(), EventID: evt.ID, AiScore: 0.6, AiVerdict: verdict,
		AiReasons: []string{"nghi van"}, CreatedAt: time.Now().UTC()}
	if err := f.mod.Create(ctx, rev); err != nil {
		t.Fatalf("review: %v", err)
	}
	return evt, rev
}

func decision(evt *domain.Event, prev domain.EventStatus, rev *domain.ModerationReview, newStatus domain.EventStatus, verdict string) port.ModerationDecision {
	moderator := uuid.New()
	actor := moderator.String()
	reason := "ly do " + verdict
	e := *evt
	e.Status = newStatus
	id := rev.ID
	return port.ModerationDecision{
		Event: &e, PrevStatus: prev, ReviewID: &id, HumanID: moderator, Verdict: verdict, Note: reason,
		DecidedAt: time.Now().UTC(),
		Audit: &domain.AuditLog{ActorType: "human", ActorID: &actor, Action: "event.moderator_" + verdict,
			Subject: "event:" + evt.ID.String(), Reason: &reason, Metadata: map[string]any{"override": false}, CreatedAt: time.Now().UTC()},
	}
}

func TestTichHop_ApplyDecision_GhiCaBaTrongMotTransaction(t *testing.T) {
	ctx := context.Background()
	f := newModFixture(t)
	evt, rev := seedReviewed(t, f, domain.StatusPendingReview, domain.VerdictNeedsHuman)
	// human_id tham chieu identities: Moderator phai ton tai.
	d := decision(evt, domain.StatusPendingReview, rev, domain.StatusRejected, "REJECTED")
	if _, err := f.db.Exec(`INSERT INTO identities (id, email, password_hash, role) VALUES ($1, $2, 'x', 'moderator')`,
		d.HumanID, d.HumanID.String()+"@t.local"); err != nil {
		t.Fatalf("moderator: %v", err)
	}

	if err := f.store.ApplyDecision(ctx, d); err != nil {
		t.Fatalf("ApplyDecision: %v", err)
	}

	got, _ := f.events.GetByID(ctx, evt.ID)
	if got.Status != domain.StatusRejected {
		t.Errorf("status = %s", got.Status)
	}
	r, _ := f.mod.GetLatestByEventID(ctx, evt.ID)
	if r.HumanVerdict == nil || *r.HumanVerdict != "REJECTED" || r.HumanNote == nil || *r.HumanNote != "ly do REJECTED" {
		t.Errorf("review chua ghi: %+v", r)
	}
	logs, _ := f.audit.ListBySubject(ctx, "event:"+evt.ID.String())
	if len(logs) != 1 || logs[0].Action != "event.moderator_REJECTED" {
		t.Errorf("audit: %+v", logs)
	}
}

// Audit hong (actor_type NULL vi pham NOT NULL) o BUOC CUOI -> su kien va review khong doi.
func TestTichHop_ApplyDecision_AuditHong_KhongGhiGi(t *testing.T) {
	ctx := context.Background()
	f := newModFixture(t)
	evt, rev := seedReviewed(t, f, domain.StatusPendingReview, domain.VerdictNeedsHuman)
	d := decision(evt, domain.StatusPendingReview, rev, domain.StatusRejected, "REJECTED")
	d.ReviewID = nil                  // tranh FK human_id; kiem rieng su kien + audit
	d.Audit.Metadata = make(chan int) // khong marshal duoc -> loi o buoc audit

	if err := f.store.ApplyDecision(ctx, d); err == nil {
		t.Fatal("audit hong phai tra loi")
	}
	got, _ := f.events.GetByID(ctx, evt.ID)
	if got.Status != domain.StatusPendingReview {
		t.Errorf("su kien khong duoc doi khi audit hong, got %s", got.Status)
	}
	logs, _ := f.audit.ListBySubject(ctx, "event:"+evt.ID.String())
	if len(logs) != 0 {
		t.Errorf("khong duoc con audit, got %d", len(logs))
	}
}

// Hai Moderator quyet DONG THOI: dung mot nguoi thang, nguoi kia ErrAlreadyDecided.
func TestTichHop_ApplyDecision_HaiQuyetDinhDongThoi_MotThang(t *testing.T) {
	ctx := context.Background()
	f := newModFixture(t)
	evt, rev := seedReviewed(t, f, domain.StatusPendingReview, domain.VerdictNeedsHuman)
	approve := decision(evt, domain.StatusPendingReview, rev, domain.StatusScheduled, "APPROVED")
	reject := decision(evt, domain.StatusPendingReview, rev, domain.StatusRejected, "REJECTED")
	approve.ReviewID, reject.ReviewID = nil, nil // dieu kien tren trang thai su kien la du de tranh chap

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, d := range []port.ModerationDecision{approve, reject} {
		wg.Add(1)
		go func(i int, d port.ModerationDecision) {
			defer wg.Done()
			errs[i] = f.store.ApplyDecision(ctx, d)
		}(i, d)
	}
	wg.Wait()

	ok, locked := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, domain.ErrAlreadyDecided):
			locked++
		default:
			t.Fatalf("loi la: %v", err)
		}
	}
	if ok != 1 || locked != 1 {
		t.Fatalf("phai dung 1 thang 1 bi khoa, got ok=%d locked=%d", ok, locked)
	}
	logs, _ := f.audit.ListBySubject(ctx, "event:"+evt.ID.String())
	if len(logs) != 1 {
		t.Errorf("chi quyet dinh thang moi co audit, got %d", len(logs))
	}
}

// Quyet dinh dua tren trang thai cu (su kien da doi): bi khoa, khong ghi gi.
func TestTichHop_ApplyDecision_TrangThaiDaDoi_BiKhoa(t *testing.T) {
	ctx := context.Background()
	f := newModFixture(t)
	evt, rev := seedReviewed(t, f, domain.StatusRejected, domain.VerdictAutoRejected)
	d := decision(evt, domain.StatusPendingReview, rev, domain.StatusScheduled, "APPROVED") // PrevStatus sai
	d.ReviewID = nil

	if err := f.store.ApplyDecision(ctx, d); !errors.Is(err, domain.ErrAlreadyDecided) {
		t.Fatalf("expected ErrAlreadyDecided, got %v", err)
	}
	got, _ := f.events.GetByID(ctx, evt.ID)
	logs, _ := f.audit.ListBySubject(ctx, "event:"+evt.ID.String())
	if got.Status != domain.StatusRejected || len(logs) != 0 {
		t.Errorf("khong duoc ghi gi: status=%s audit=%d", got.Status, len(logs))
	}
}

func TestTichHop_ListPendingHuman_ReviewMoiNhatCuaMoiSuKien(t *testing.T) {
	ctx := context.Background()
	f := newModFixture(t)
	if _, err := f.db.Exec(`DELETE FROM moderation_reviews`); err != nil {
		t.Fatal(err)
	}
	needs, _ := seedReviewed(t, f, domain.StatusPendingReview, domain.VerdictNeedsHuman)
	rejected, _ := seedReviewed(t, f, domain.StatusRejected, domain.VerdictAutoRejected)
	seedReviewed(t, f, domain.StatusScheduled, domain.VerdictAutoApproved)
	// Su kien co review cu NEEDS_HUMAN nhung review MOI NHAT da AUTO_APPROVED -> khong con cho.
	old, _ := seedReviewed(t, f, domain.StatusScheduled, domain.VerdictNeedsHuman)
	_ = f.mod.Create(ctx, &domain.ModerationReview{ID: uuid.New(), EventID: old.ID, AiScore: 0.1,
		AiVerdict: domain.VerdictAutoApproved, CreatedAt: time.Now().UTC().Add(time.Second)})

	list, err := f.mod.ListPendingHuman(ctx)
	if err != nil {
		t.Fatalf("ListPendingHuman: %v", err)
	}
	got := map[uuid.UUID]domain.AiVerdict{}
	for _, r := range list {
		got[r.EventID] = r.AiVerdict
	}
	if len(got) != 2 || got[needs.ID] != domain.VerdictNeedsHuman || got[rejected.ID] != domain.VerdictAutoRejected {
		t.Fatalf("got %v", got)
	}
}
