package http_test

// Contract test cho frontend-real-api AC-26: GET /api/v1/events/public.
//
// TEN PORT GIA DINH (contracts.md / plan.md WP5): port.EventRepository them
//
//	ListPublic(ctx context.Context, limit int) ([]*domain.Event, error)
//
// Repo thuc te loc status IN (SCHEDULED, ON_SALE, SOLD_OUT), sap xep sale_start_at ASC va cat o `limit`
// (kiem trong test tich hop postgres). Fake duoi day thuc hien dung hop dong do va ghi lai `limit` ma
// tang tren truyen xuong (phai la 100). Fake KEM tier vao event de dung duoc ca khi tier do repo hay use case gan.
// Phuong thuc nay dinh nghia trong file test moi nay de khong sua handler_test.go (cung package http_test);
// implementer KHONG them ListPublic vao fakeRepo trong handler_test.go.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/google/uuid"
)

var lastPublicLimit int

func (r *fakeRepo) ListPublic(ctx context.Context, limit int) ([]*domain.Event, error) {
	lastPublicLimit = limit
	if r.failList {
		return nil, errors.New("pq: connection refused to 10.0.0.5")
	}
	var list []*domain.Event
	for _, e := range r.events {
		switch e.Status {
		case domain.StatusScheduled, domain.StatusOnSale, domain.StatusSoldOut:
			c := *e
			c.Tiers = r.tiers[e.ID]
			list = append(list, &c)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].SaleStartAt.Before(list[j].SaleStartAt) })
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

func seedPublic(t *testing.T, repo *fakeRepo, status domain.EventStatus, saleStartOffset time.Duration, withTier bool) *domain.Event {
	t.Helper()
	now := time.Now().UTC()
	evt, err := domain.NewEvent(uuid.New(), "pub-"+uuid.NewString()[:8], "Event "+string(status), "mo ta", "venue",
		now.Add(saleStartOffset+48*time.Hour), now.Add(saleStartOffset), nil)
	if err != nil {
		t.Fatal(err)
	}
	evt.Status = status
	repo.events[evt.ID] = evt
	if withTier {
		tier, err := domain.NewTicketTier(evt.ID, "General", 650000, 100)
		if err != nil {
			t.Fatal(err)
		}
		repo.tiers[evt.ID] = append(repo.tiers[evt.ID], *tier)
	}
	return evt
}

type publicBody struct {
	Events []struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		SaleStartAt string `json:"sale_start_at"`
		Tiers       []struct {
			Name       string `json:"name"`
			PriceCents int64  `json:"price_cents"`
			Quota      int    `json:"quota"`
		} `json:"tiers"`
	} `json:"events"`
}

func getPublic(t *testing.T, h http.Handler, mutate func(*http.Request)) (*httptest.ResponseRecorder, publicBody) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/public", nil)
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body publicBody
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

// AC-26: khong can danh tinh; tra {events:[...]} chi gom SCHEDULED/ON_SALE/SOLD_OUT, kem tiers.
func TestPublicEvents_KhongCanTokenVaChiTraTrangThaiCongKhai(t *testing.T) {
	h, repo := setupTestServer()
	scheduled := seedPublic(t, repo, domain.StatusScheduled, 24*time.Hour, true)
	onSale := seedPublic(t, repo, domain.StatusOnSale, 2*time.Hour, true)
	soldOut := seedPublic(t, repo, domain.StatusSoldOut, 1*time.Hour, true)
	for _, st := range []domain.EventStatus{domain.StatusDraft, domain.StatusPendingReview, domain.StatusRejected, domain.StatusClosed, domain.StatusCompleted, domain.StatusCancelled} {
		seedPublic(t, repo, st, 3*time.Hour, true)
	}

	rec, body := getPublic(t, h, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type %q", ct)
	}
	got := map[string]bool{}
	for _, e := range body.Events {
		got[e.ID] = true
		switch e.Status {
		case "SCHEDULED", "ON_SALE", "SOLD_OUT":
		default:
			t.Errorf("trang thai khong cong khai lot vao danh sach: %s", e.Status)
		}
		if len(e.Tiers) != 1 || e.Tiers[0].PriceCents != 650000 || e.Tiers[0].Quota != 100 {
			t.Errorf("su kien %s thieu tiers hoac tier sai: %+v", e.ID, e.Tiers)
		}
	}
	for _, want := range []*domain.Event{scheduled, onSale, soldOut} {
		if !got[want.ID.String()] {
			t.Errorf("thieu su kien %s (%s)", want.ID, want.Status)
		}
	}
	if len(body.Events) != 3 {
		t.Errorf("muon dung 3 su kien, got %d", len(body.Events))
	}
}

// AC-26: sap xep sale_start_at tang dan.
func TestPublicEvents_SapXepSaleStartTangDan(t *testing.T) {
	h, repo := setupTestServer()
	late := seedPublic(t, repo, domain.StatusScheduled, 72*time.Hour, false)
	early := seedPublic(t, repo, domain.StatusOnSale, 1*time.Hour, false)
	mid := seedPublic(t, repo, domain.StatusSoldOut, 24*time.Hour, false)

	_, body := getPublic(t, h, nil)
	var ids []string
	for _, e := range body.Events {
		ids = append(ids, e.ID)
	}
	want := []string{early.ID.String(), mid.ID.String(), late.ID.String()}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("thu tu = %v, muon %v", ids, want)
	}
}

// AC-26: toi da 100 -- handler phai yeu cau limit = 100 tu repo.
func TestPublicEvents_GioiHan100(t *testing.T) {
	h, repo := setupTestServer()
	seedPublic(t, repo, domain.StatusOnSale, time.Hour, false)
	lastPublicLimit = -1
	rec, _ := getPublic(t, h, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if lastPublicLimit != 100 {
		t.Fatalf("limit truyen xuong repo = %d, muon 100", lastPublicLimit)
	}
}

// AC-26: danh sach rong la {"events":[]}, khong phai null (client .map() khong vo).
func TestPublicEvents_RongLaMangRongKhongPhaiNull(t *testing.T) {
	h, _ := setupTestServer()
	rec, _ := getPublic(t, h, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("body khong phai JSON object: %v: %s", err, rec.Body)
	}
	if string(raw["events"]) != "[]" {
		t.Fatalf(`events = %s, muon []`, raw["events"])
	}
}

// AC-26: nguoi dung da dang nhap (buyer) cung duoc, danh tinh khong thay doi ket qua.
func TestPublicEvents_BuyerDaDangNhapCungDuoc(t *testing.T) {
	h, repo := setupTestServer()
	seedPublic(t, repo, domain.StatusOnSale, time.Hour, false)
	seedPublic(t, repo, domain.StatusDraft, time.Hour, false)
	rec, body := getPublic(t, h, func(r *http.Request) { as(r, uuid.New(), "buyer") })
	if rec.Code != http.StatusOK || len(body.Events) != 1 {
		t.Fatalf("status %d, events %d", rec.Code, len(body.Events))
	}
}

// AC-26: loi noi bo khong lo ra client.
func TestPublicEvents_Loi500KhongLoChiTiet(t *testing.T) {
	h, repo := setupTestServer()
	repo.failList = true
	rec, _ := getPublic(t, h, nil)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "pq:") || strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
}

// AC-26: khong lam hong route cu -- /public khong bi hieu nham la {id}; /{id} va danh sach organizer nhu cu.
func TestPublicEvents_KhongLamHongRouteCu(t *testing.T) {
	h, repo := setupTestServer()
	evt := seedPublic(t, repo, domain.StatusOnSale, time.Hour, true)

	// /public khong tra 400 "invalid event uuid".
	if rec, _ := getPublic(t, h, nil); rec.Code == http.StatusBadRequest {
		t.Fatalf("/public bi route nham sang {id}: %s", rec.Body)
	}

	// GET /{id} van hoat dong.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events/"+evt.ID.String(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /{id}: %d", rec.Code)
	}

	// GET /api/v1/events van yeu cau danh tinh organizer/moderator/ops.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/v1/events khong danh tinh: %d, muon 401", rec.Code)
	}

	// Chi GET: POST /public khong duoc tao su kien / khong 2xx.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/events/public", nil))
	if rec.Code < 400 {
		t.Fatalf("POST /public: %d", rec.Code)
	}
}
