package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/google/uuid"
)

// Ma tran quyen EV-114 (docs/01 muc 3). Moi o la ma trang thai mong doi khi role do goi route
// tren mot su kien PENDING_REVIEW cua chinh organizer "owner". 401 = khong co danh tinh.
//
// Cac ma 2xx/4xx nghiep vu (422) chi chung minh request DA QUA lop phan quyen; dieu can kiem la
// role khong duoc phep thi nhan 403 va khong doi trang thai.
func TestAuthz_MaTranQuyenTheoRole(t *testing.T) {
	owner := uuid.New()

	type call struct {
		name, method, path, body string
	}
	routes := []call{
		{"tao su kien", "POST", "/api/v1/events", `{"title":"T","starts_at":"2030-01-02T00:00:00Z","sale_start_at":"2030-01-01T00:00:00Z"}`},
		{"sua su kien", "PUT", "/api/v1/events/{id}", `{"title":"T2","starts_at":"2030-01-02T00:00:00Z","sale_start_at":"2030-01-01T00:00:00Z"}`},
		{"them hang ve", "POST", "/api/v1/events/{id}/tiers", `{"name":"B","price_cents":1000,"quota":5}`},
		{"nop duyet", "POST", "/api/v1/events/{id}/submit", ``},
		{"huy", "POST", "/api/v1/events/{id}/cancel", ``},
		{"duyet", "POST", "/api/v1/events/{id}/approve", `{"note":"ok"}`},
		{"tu choi", "POST", "/api/v1/events/{id}/reject", `{"reason":"thieu giay phep"}`},
		{"hang doi kiem duyet", "GET", "/api/v1/moderation/queue", ``},
		{"mo ban", "POST", "/api/v1/events/{id}/publish", ``},
	}
	allowed := map[string][]string{
		"tao su kien":         {"organizer"},
		"sua su kien":         {"organizer"},
		"them hang ve":        {"organizer"},
		"nop duyet":           {"organizer"},
		"huy":                 {"organizer"},
		"duyet":               {"moderator"},
		"tu choi":             {"moderator"},
		"hang doi kiem duyet": {"moderator"},
		"mo ban":              {"ops"},
	}

	for _, rt := range routes {
		for _, role := range []string{"", "buyer", "organizer", "moderator", "ops"} {
			t.Run(rt.name+"/"+orNone(role), func(t *testing.T) {
				router, repo, _, audit := setupModerationTestServer()
				evt := seedPending(t, repo, owner)
				before := evt.Status

				req := httptest.NewRequest(rt.method, pathFor(rt.path, evt.ID), bytes.NewBufferString(rt.body))
				if role != "" {
					as(req, owner, role) // organizer o day CHINH LA chu su kien
				}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)

				switch {
				case role == "":
					if rec.Code != http.StatusUnauthorized {
						t.Fatalf("khong danh tinh: %d, muon 401", rec.Code)
					}
				case contains(allowed[rt.name], role):
					if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
						t.Fatalf("role %s duoc phep nhung nhan %d: %s", role, rec.Code, rec.Body)
					}
				default:
					if rec.Code != http.StatusForbidden {
						t.Fatalf("role %s khong duoc phep nhung nhan %d", role, rec.Code)
					}
					if repo.events[evt.ID].Status != before {
						t.Fatalf("bi 403 ma trang thai van doi %s -> %s", before, repo.events[evt.ID].Status)
					}
					if !hasAction(audit.logs, "ACCESS_DENIED") {
						t.Fatal("403 vi sai role phai ghi audit ACCESS_DENIED")
					}
				}
			})
		}
	}
}

// AC: organizer khac khong sua/nop/huy duoc su kien cua nguoi khac, va bi audit cross-tenant.
func TestAuthz_OrganizerKhacKhongDungDuocSuKienNguoiKhac(t *testing.T) {
	owner, intruder := uuid.New(), uuid.New()
	for _, c := range []struct{ method, path, body string }{
		{"PUT", "/api/v1/events/{id}", `{"title":"Hack","starts_at":"2030-01-02T00:00:00Z","sale_start_at":"2030-01-01T00:00:00Z"}`},
		{"POST", "/api/v1/events/{id}/tiers", `{"name":"Free","price_cents":0,"quota":1}`},
		{"POST", "/api/v1/events/{id}/cancel", ``},
	} {
		router, repo, _, audit := setupModerationTestServer()
		evt := seedDraft(t, repo, owner)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, as(httptest.NewRequest(c.method, pathFor(c.path, evt.ID), bytes.NewBufferString(c.body)), intruder, "organizer"))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s boi organizer khac: %d, muon 403", c.method, c.path, rec.Code)
		}
		if repo.events[evt.ID].Title != "Draft" || repo.events[evt.ID].Status != domain.StatusDraft {
			t.Fatal("su kien bi sua boi nguoi khong so huu")
		}
		if !hasAction(audit.logs, "ACCESS_DENIED_CROSS_TENANT") {
			t.Fatal("phai ghi audit ACCESS_DENIED_CROSS_TENANT")
		}
	}
}

// AC: organizer_id / moderator_id trong body khong con quyet dinh gi; lech voi token -> 403.
func TestAuthz_GiaDanhQuaBodyBiChan(t *testing.T) {
	owner, victim := uuid.New(), uuid.New()

	router, repo, _, audit := setupModerationTestServer()
	body, _ := json.Marshal(map[string]any{
		"organizer_id": victim.String(), "title": "Fake",
		"starts_at": time.Now().Add(48 * time.Hour), "sale_start_at": time.Now().Add(24 * time.Hour),
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, as(httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader(body)), owner, "organizer"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("tao su kien danh nghia nguoi khac: %d, muon 403", rec.Code)
	}
	if len(repo.events) != 0 {
		t.Fatal("khong duoc tao su kien")
	}
	if !hasAction(audit.logs, "ACCESS_DENIED_CROSS_TENANT") {
		t.Fatal("phai audit gia danh")
	}

	// Khong gui organizer_id: lay tu token.
	body, _ = json.Marshal(map[string]any{
		"title": "Mine", "starts_at": time.Now().Add(48 * time.Hour), "sale_start_at": time.Now().Add(24 * time.Hour),
	})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, as(httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader(body)), owner, "organizer"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("tao su kien: %d %s", rec.Code, rec.Body)
	}
	for _, e := range repo.events {
		if e.OrganizerID != owner {
			t.Fatalf("organizer_id = %s, muon lay tu token %s", e.OrganizerID, owner)
		}
	}

	// Moderator gia danh moderator khac.
	evt := seedPending(t, repo, owner)
	modID := uuid.New()
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, as(httptest.NewRequest("POST", "/api/v1/events/"+evt.ID.String()+"/reject",
		bytes.NewBufferString(`{"moderator_id":"`+uuid.NewString()+`","reason":"x"}`)), modID, "moderator"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("moderator_id lech token: %d, muon 403", rec.Code)
	}
}

// AC: duyet/tu choi ghi dung moderator tu token vao quyet dinh.
func TestAuthz_DuyetGhiModeratorTuToken(t *testing.T) {
	router, repo, modRepo, _ := setupModerationTestServer()
	evt := seedPending(t, repo, uuid.New())
	review := &domain.ModerationReview{ID: uuid.New(), EventID: evt.ID, AiVerdict: domain.VerdictNeedsHuman, CreatedAt: time.Now()}
	_ = modRepo.Create(context.Background(), review)

	modID := uuid.New()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, as(httptest.NewRequest("POST", "/api/v1/events/"+evt.ID.String()+"/approve", nil), modID, "moderator"))
	if rec.Code != http.StatusOK {
		t.Fatalf("duyet: %d %s", rec.Code, rec.Body)
	}
	if review.HumanID == nil || *review.HumanID != modID {
		t.Fatalf("human_id = %v, muon %s (tu token)", review.HumanID, modID)
	}
}

// AC: su kien chua duyet chi chu su kien / moderator / ops thay; nguoi khac nhan 404.
func TestAuthz_SuKienChuaDuyetBiAn(t *testing.T) {
	owner := uuid.New()
	router, repo, _, _ := setupModerationTestServer()
	draft := seedDraft(t, repo, owner)

	cases := []struct {
		name string
		id   uuid.UUID
		role string
		want int
	}{
		{"khach", uuid.Nil, "", http.StatusNotFound},
		{"buyer", uuid.New(), "buyer", http.StatusNotFound},
		{"organizer khac", uuid.New(), "organizer", http.StatusNotFound},
		{"chu su kien", owner, "organizer", http.StatusOK},
		{"moderator", uuid.New(), "moderator", http.StatusOK},
		{"ops", uuid.New(), "ops", http.StatusOK},
	}
	for _, c := range cases {
		for _, path := range []string{"/api/v1/events/" + draft.ID.String(), "/api/v1/events/slug/" + draft.Slug} {
			req := httptest.NewRequest("GET", path, nil)
			if c.role != "" {
				as(req, c.id, c.role)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Fatalf("%s GET %s: %d, muon %d", c.name, path, rec.Code, c.want)
			}
		}
	}

	// Su kien da duyet: ai cung xem duoc.
	sched := seedPending(t, repo, owner)
	_ = sched.ApproveOverride(time.Now())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/events/"+sched.ID.String(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("su kien da duyet voi khach: %d", rec.Code)
	}
}

// AC: danh sach su kien cua organizer lay tu token.
func TestAuthz_DanhSachSuKienTheoToken(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	router, repo, _, _ := setupModerationTestServer()
	seedDraft(t, repo, a)
	seedDraft(t, repo, b)

	list := func(id uuid.UUID, role, query string) (int, int) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, as(httptest.NewRequest("GET", "/api/v1/events"+query, nil), id, role))
		var items []map[string]any
		_ = json.NewDecoder(rec.Body).Decode(&items)
		return rec.Code, len(items)
	}

	if code, n := list(a, "organizer", ""); code != http.StatusOK || n != 1 {
		t.Fatalf("organizer a xem cua minh: %d, %d muc", code, n)
	}
	if code, _ := list(a, "organizer", "?organizer_id="+b.String()); code != http.StatusForbidden {
		t.Fatalf("organizer a xem cua b: %d, muon 403", code)
	}
	if code, n := list(uuid.New(), "moderator", "?organizer_id="+b.String()); code != http.StatusOK || n != 1 {
		t.Fatalf("moderator xem cua b: %d, %d muc", code, n)
	}
	if code, _ := list(uuid.New(), "moderator", ""); code != http.StatusBadRequest {
		t.Fatalf("moderator khong chi dinh organizer_id: %d, muon 400", code)
	}
	if code, _ := list(uuid.New(), "buyer", ""); code != http.StatusForbidden {
		t.Fatalf("buyer liet ke su kien organizer: %d, muon 403", code)
	}
}

// Header danh tinh hong (khong phai UUID, role la) bi coi nhu khong co.
func TestAuthz_HeaderDanhTinhHongLa401(t *testing.T) {
	router, _, _, _ := setupModerationTestServer()
	for _, h := range [][2]string{{"not-a-uuid", "organizer"}, {uuid.NewString(), "admin"}} {
		req := httptest.NewRequest("POST", "/api/v1/events", bytes.NewBufferString(`{}`))
		req.Header.Set("X-Identity-Id", h[0])
		req.Header.Set("X-Identity-Role", h[1])
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("header %v: %d, muon 401", h, rec.Code)
		}
	}
}

// Loi noi bo khong lo ra client.
func TestAuthz_Loi500KhongLoChiTiet(t *testing.T) {
	router, repo, _, _ := setupModerationTestServer()
	repo.failList = true
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, as(httptest.NewRequest("GET", "/api/v1/events", nil), uuid.New(), "organizer"))
	if rec.Code != http.StatusInternalServerError || bytes.Contains(rec.Body.Bytes(), []byte("pq:")) {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
}

func seedDraft(t *testing.T, repo *fakeRepo, owner uuid.UUID) *domain.Event {
	t.Helper()
	now := time.Now().UTC()
	evt, err := domain.NewEvent(owner, "draft-"+uuid.NewString()[:8], "Draft", "d", "v", now.Add(48*time.Hour), now.Add(24*time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = repo.Create(context.Background(), evt)
	return evt
}

func seedPending(t *testing.T, repo *fakeRepo, owner uuid.UUID) *domain.Event {
	t.Helper()
	evt := seedDraft(t, repo, owner)
	tier, _ := domain.NewTicketTier(evt.ID, "General", 10000, 10)
	_ = evt.AddTier(*tier)
	_ = repo.AddTier(context.Background(), tier)
	if err := evt.SubmitForReview(); err != nil {
		t.Fatal(err)
	}
	return evt
}

func pathFor(p string, id uuid.UUID) string {
	return string(bytes.ReplaceAll([]byte(p), []byte("{id}"), []byte(id.String())))
}

func orNone(s string) string {
	if s == "" {
		return "khong-danh-tinh"
	}
	return s
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func hasAction(logs []*domain.AuditLog, action string) bool {
	for _, l := range logs {
		if l.Action == action {
			return true
		}
	}
	return false
}
