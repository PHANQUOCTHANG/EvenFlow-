package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	adapterHTTP "github.com/eventflow/eventflow/services/event/internal/adapter/http"
	"github.com/eventflow/eventflow/services/event/internal/domain"
)

// Khach an danh (khong header danh tinh) xem duoc danh sach, chi gom su kien da duyet va chua ket
// thuc; route "GET /api/v1/events" ("cua toi") van doi dang nhap.
func TestHTTP_PublicEvents_AnDanhChiThaySuKienDaDuyet(t *testing.T) {
	router, repo := setupTestServer()
	org := uuid.New()
	want := map[uuid.UUID]bool{}
	for _, st := range []domain.EventStatus{
		domain.StatusDraft, domain.StatusPendingReview, domain.StatusRejected,
		domain.StatusScheduled, domain.StatusOnSale, domain.StatusSoldOut,
		domain.StatusCancelled, domain.StatusCompleted,
	} {
		id := uuid.New()
		repo.events[id] = &domain.Event{ID: id, OrganizerID: org, Slug: "s-" + id.String(), Title: string(st),
			Status: st, SaleStartAt: time.Now(), StartsAt: time.Now().Add(time.Hour)}
		if st == domain.StatusScheduled || st == domain.StatusOnSale || st == domain.StatusSoldOut {
			want[id] = true
		}
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events/public", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=10" {
		t.Errorf("Cache-Control = %q", cc)
	}
	var got []adapterHTTP.EventResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("tra %d su kien, muon %d: %+v", len(got), len(want), got)
	}
	for _, e := range got {
		if !want[e.ID] {
			t.Errorf("lo su kien trang thai %s ra danh sach cong khai", e.Title)
		}
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events", nil))
	if rec.Code == http.StatusOK {
		t.Errorf("GET /api/v1/events an danh phai bi tu choi, nhan %d", rec.Code)
	}
}

func TestHTTP_PublicEvents_LimitSai(t *testing.T) {
	router, _ := setupTestServer()
	for _, q := range []string{"0", "-1", "abc"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events/public?limit="+q, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("limit=%s: status %d, muon 400", q, rec.Code)
		}
	}
}
