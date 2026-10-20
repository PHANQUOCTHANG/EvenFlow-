package httpadapter

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
	"github.com/eventflow/eventflow/libs/go/idem"
)

func holdWith(t *testing.T, h http.Handler, mutate func(r *http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/v1/events/e1/holds", strings.NewReader(holdBody))
	r.Header.Set(idem.Header, "key-"+t.Name())
	setPrincipal(r, testUser, testDevice)
	r.Header.Set("X-Queue-Token", validQueueToken(t))
	mutate(r)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// EV-137: chi token ky dung, con han, DUNG nguoi + thiet bi + su kien moi duoc di tiep.
func TestCreateHold_QueueTokenKyHopLeDuocChapNhan(t *testing.T) {
	holds := &fakeHolds{}
	w := holdWith(t, newServer(holds), func(*http.Request) {})
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	in := holds.lastInput()
	if in.QueueToken != testJTI || in.IdentityID != testUser || in.EventID != "e1" {
		t.Fatalf("use case phai nhan jti + identity tu token/gateway, nhan %+v", in)
	}
}

func TestCreateHold_QueueTokenSaiBiTuChoiTruocKhiChamUseCase(t *testing.T) {
	const otherUser = "22222222-2222-4222-8222-222222222222"
	const otherDevice = "44444444-4444-4444-8444-444444444444"
	now := time.Now()
	sign := func(c authx.QueueClaims) string {
		if c.IssuedAt.IsZero() {
			c.IssuedAt, c.ExpiresAt = now, now.Add(time.Hour)
		}
		return queueToken(t, c)
	}
	base := authx.QueueClaims{EventID: "e1", Subject: testUser, DeviceID: testDevice, JTI: testJTI}

	cases := []struct {
		name   string
		mutate func(*http.Request)
		typ    string
	}{
		{"khong co token", func(r *http.Request) { r.Header.Del("X-Queue-Token") }, ProblemQueueTokenInvalid},
		{"token hex cu (truoc EV-137)", func(r *http.Request) { r.Header.Set("X-Queue-Token", "0123456789abcdef0123456789abcdef") }, ProblemQueueTokenInvalid},
		{"token sua chu ky", func(r *http.Request) { r.Header.Set("X-Queue-Token", validQueueToken(t)+"x") }, ProblemQueueTokenInvalid},
		{"het han", func(r *http.Request) {
			c := base
			c.IssuedAt, c.ExpiresAt = now.Add(-2*time.Hour), now.Add(-time.Hour)
			r.Header.Set("X-Queue-Token", sign(c))
		}, ProblemQueueTokenExpired},
		{"token cua nguoi khac (chia se token)", func(r *http.Request) { setPrincipal(r, otherUser, testDevice) }, ProblemQueueTokenMismatch},
		{"token cua su kien khac", func(r *http.Request) {
			c := base
			c.EventID = "e2"
			r.Header.Set("X-Queue-Token", sign(c))
		}, ProblemQueueTokenMismatch},
		{"sang thiet bi khac", func(r *http.Request) { setPrincipal(r, testUser, otherDevice) }, ProblemQueueTokenMismatch},
		{"phien chua gan thiet bi", func(r *http.Request) { r.Header.Del("X-Device-Id") }, ProblemQueueTokenMismatch},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			holds := &fakeHolds{}
			w := holdWith(t, newServer(holds), c.mutate)
			if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), c.typ) {
				t.Fatalf("status %d body %s, muon 403 %s", w.Code, w.Body, c.typ)
			}
			if holds.calls.Load() != 0 {
				t.Fatal("token sai ma van chay use case tao hold")
			}
		})
	}
}

func TestCreateHold_ThieuDanhTinhLa401(t *testing.T) {
	holds := &fakeHolds{}
	w := holdWith(t, newServer(holds), func(r *http.Request) {
		r.Header.Del("X-Identity-Id")
		r.Header.Del("X-Identity-Role")
	})
	if w.Code != http.StatusUnauthorized || holds.calls.Load() != 0 {
		t.Fatalf("status %d calls %d", w.Code, holds.calls.Load())
	}
}

// Chua cau hinh secret thi dong cua (fail-closed), khong am tham chap nhan moi token.
func TestCreateHold_ThieuSecretThiTuChoi(t *testing.T) {
	holds := &fakeHolds{}
	h := New(holds, nil)
	h.log = newServerLog()
	mux := http.NewServeMux()
	h.Routes(mux, idem.Middleware(&memIdem{}))
	w := holdWith(t, mux, func(*http.Request) {})
	if w.Code != http.StatusServiceUnavailable || holds.calls.Load() != 0 {
		t.Fatalf("status %d calls %d, muon 503 va 0", w.Code, holds.calls.Load())
	}
}
