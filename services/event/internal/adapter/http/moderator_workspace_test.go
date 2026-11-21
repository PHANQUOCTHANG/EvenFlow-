package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	adapterHTTP "github.com/eventflow/eventflow/services/event/internal/adapter/http"
	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/eventflow/eventflow/services/event/internal/port"
	"github.com/google/uuid"
)

// fakeDecisionStore ghi ca ba nhu mot transaction; err != nil thi khong ghi gi (EV-121).
type fakeDecisionStore struct {
	repo  *fakeRepo
	mod   *fakeModRepo
	audit *fakeAuditRepo
	err   error
}

func (s *fakeDecisionStore) ApplyDecision(ctx context.Context, d port.ModerationDecision) error {
	if s.err != nil {
		return s.err
	}
	if d.ReviewID != nil {
		if rev, ok := s.mod.reviews[*d.ReviewID]; ok {
			v, n, at, h := d.Verdict, d.Note, d.DecidedAt, d.HumanID
			rev.HumanVerdict, rev.HumanNote, rev.DecidedAt, rev.HumanID = &v, &n, &at, &h
		}
	}
	s.repo.events[d.Event.ID] = d.Event
	s.audit.logs = append(s.audit.logs, d.Audit)
	return nil
}

// lastDecisionStore: store cua server tao gan nhat boi setupModerationTestServer (de tiem loi).
var lastDecisionStore *fakeDecisionStore

func seedModerationEvent(t *testing.T, repo *fakeRepo, modRepo *fakeModRepo, status domain.EventStatus, v domain.AiVerdict) *domain.Event {
	t.Helper()
	now := time.Now().UTC()
	evt, _ := domain.NewEvent(uuid.New(), "ev121-"+uuid.NewString()[:6], "Ho so EV-121", "Noi dung goc", "SVD",
		now.Add(48*time.Hour), now.Add(3*time.Hour), nil)
	tier, _ := domain.NewTicketTier(evt.ID, "GA", 100000, 50)
	_ = evt.AddTier(*tier)
	_ = evt.SubmitForReview()
	evt.Status = status
	_ = repo.Create(context.Background(), evt)
	_ = modRepo.Create(context.Background(), &domain.ModerationReview{ID: uuid.New(), EventID: evt.ID, AiScore: 0.9,
		AiVerdict: v, AiReasons: []string{"nghi van ban quyen"}, CreatedAt: now})
	return evt
}

func decide(router http.Handler, evtID uuid.UUID, action string, body any, role string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := as(httptest.NewRequest("POST", "/api/v1/events/"+evtID.String()+"/"+action, bytes.NewReader(b)), uuid.New(), role)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func problemType(rec *httptest.ResponseRecorder) string {
	var p struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	return p.Type
}

func TestHTTP_LatNguocAI_ThieuLyDo_422CoType(t *testing.T) {
	router, repo, modRepo, auditRepo := setupModerationTestServer()
	evt := seedModerationEvent(t, repo, modRepo, domain.StatusRejected, domain.VerdictAutoRejected)

	rec := decide(router, evt.ID, "approve", adapterHTTP.ModeratorApproveRequest{Note: " "}, "moderator")
	if rec.Code != http.StatusUnprocessableEntity || problemType(rec) != adapterHTTP.ProblemOverrideReasonRequired {
		t.Fatalf("expected 422 %s, got %d %s", adapterHTTP.ProblemOverrideReasonRequired, rec.Code, rec.Body.String())
	}
	if len(auditRepo.logs) != 0 {
		t.Fatal("khong duoc ghi audit khi thieu ly do")
	}

	rec = decide(router, evt.ID, "approve", adapterHTTP.ModeratorApproveRequest{Note: "AI nham"}, "moderator")
	if rec.Code != http.StatusOK {
		t.Fatalf("co ly do: %d %s", rec.Code, rec.Body.String())
	}
	if auditRepo.logs[0].Action != "event.moderator_overrode_ai" || *auditRepo.logs[0].Reason != "AI nham" {
		t.Errorf("audit lat nguoc: %+v", auditRepo.logs[0])
	}
}

func TestHTTP_QuyetDinhBiTranhChap_409CoType(t *testing.T) {
	router, repo, modRepo, _ := setupModerationTestServer()
	evt := seedModerationEvent(t, repo, modRepo, domain.StatusPendingReview, domain.VerdictNeedsHuman)
	lastDecisionStore.err = domain.ErrAlreadyDecided

	rec := decide(router, evt.ID, "reject", adapterHTTP.ModeratorRejectRequest{Reason: "x"}, "moderator")
	if rec.Code != http.StatusConflict || problemType(rec) != adapterHTTP.ProblemAlreadyDecided {
		t.Fatalf("expected 409 %s, got %d %s", adapterHTTP.ProblemAlreadyDecided, rec.Code, rec.Body.String())
	}
}

func TestHTTP_ChiModeratorQuyetDinhDuoc(t *testing.T) {
	router, repo, modRepo, auditRepo := setupModerationTestServer()
	evt := seedModerationEvent(t, repo, modRepo, domain.StatusPendingReview, domain.VerdictNeedsHuman)

	for _, role := range []string{"buyer", "organizer", "ops"} {
		if rec := decide(router, evt.ID, "approve", adapterHTTP.ModeratorApproveRequest{Note: "x"}, role); rec.Code != http.StatusForbidden {
			t.Errorf("vai tro %s phai 403, got %d", role, rec.Code)
		}
	}
	for _, l := range auditRepo.logs {
		if l.ActorType == "human" && l.Action != "authz.denied" && l.Action != "access.denied" {
			if l.Action == "event.moderator_approved" {
				t.Fatal("vai tro khac khong duoc tao quyet dinh")
			}
		}
	}
	if evt.Status != domain.StatusPendingReview {
		t.Errorf("su kien khong duoc doi, got %s", evt.Status)
	}
}

func TestHTTP_HoSoThamDinh_TachNoiDungGocVaAI(t *testing.T) {
	router, repo, modRepo, auditRepo := setupModerationTestServer()
	evt := seedModerationEvent(t, repo, modRepo, domain.StatusPendingReview, domain.VerdictNeedsHuman)
	_ = auditRepo.Create(context.Background(), &domain.AuditLog{ActorType: "ai", Action: "event.needs_human_review",
		Subject: "event:" + evt.ID.String(), CreatedAt: time.Now().UTC(),
		Metadata: map[string]any{"axes": map[string]any{"copyright": 0.7, "misleading": 0.2}}})

	req := as(httptest.NewRequest("GET", "/api/v1/moderation/events/"+evt.ID.String(), nil), uuid.New(), "moderator")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Event struct {
			ID    string `json:"id"`
			Tiers []any  `json:"tiers"`
		} `json:"event"`
		Review struct {
			AiVerdict string   `json:"ai_verdict"`
			AiReasons []string `json:"ai_reasons"`
		} `json:"review"`
		AiAxes map[string]float64 `json:"ai_axes"`
		Audit  []any              `json:"audit"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Event.ID != evt.ID.String() || len(body.Event.Tiers) != 1 {
		t.Errorf("noi dung goc: %+v", body.Event)
	}
	if body.Review.AiVerdict != "NEEDS_HUMAN" || len(body.Review.AiReasons) != 1 {
		t.Errorf("review: %+v", body.Review)
	}
	if body.AiAxes["copyright"] != 0.7 || len(body.Audit) != 1 {
		t.Errorf("truc/audit: %v %d", body.AiAxes, len(body.Audit))
	}

	req = as(httptest.NewRequest("GET", "/api/v1/moderation/events/"+evt.ID.String(), nil), uuid.New(), "organizer")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("organizer phai 403, got %d", rec.Code)
	}
}

func TestHTTP_HangDoi_CoCaSuKienAIDaTuChoi(t *testing.T) {
	router, repo, modRepo, _ := setupModerationTestServer()
	needs := seedModerationEvent(t, repo, modRepo, domain.StatusPendingReview, domain.VerdictNeedsHuman)
	rej := seedModerationEvent(t, repo, modRepo, domain.StatusRejected, domain.VerdictAutoRejected)

	req := as(httptest.NewRequest("GET", "/api/v1/moderation/queue", nil), uuid.New(), "moderator")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var items []struct {
		EventID string `json:"event_id"`
		Review  struct {
			AiVerdict string `json:"ai_verdict"`
		} `json:"review"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &items)
	got := map[string]string{}
	for _, it := range items {
		got[it.EventID] = it.Review.AiVerdict
	}
	if got[needs.ID.String()] != "NEEDS_HUMAN" || got[rej.ID.String()] != "AUTO_REJECTED" {
		t.Fatalf("hang doi: %v", got)
	}
}
