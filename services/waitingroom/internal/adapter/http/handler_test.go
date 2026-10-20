package httpadapter_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
	httpadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/http"
	"github.com/eventflow/eventflow/services/waitingroom/internal/domain"
)

const (
	userID   = "11111111-1111-4111-8111-111111111111"
	otherID  = "22222222-2222-4222-8222-222222222222"
	deviceID = "33333333-3333-4333-8333-333333333333"
	otherDev = "44444444-4444-4444-8444-444444444444"
	jti      = "member-jti-1"
)

var secret = []byte("waitingroom-test-secret-16bytes+")

type fakeStore struct {
	joins       int
	statusCalls []string
	soldOut     bool // Join tra SOLD_OUT, khong cap token (het ve han)
}

func (f *fakeStore) Join(_ context.Context, _, _, token string) (httpadapter.JoinResult, error) {
	f.joins++
	if f.soldOut {
		return httpadapter.JoinResult{State: domain.StateSoldOut, Rank: -1}, nil
	}
	return httpadapter.JoinResult{State: domain.StateLobby, Token: jti, Rank: -1, IsNew: true}, nil
}

func (f *fakeStore) Status(_ context.Context, _, token string) (domain.Position, error) {
	f.statusCalls = append(f.statusCalls, token)
	return domain.Position{State: domain.StateQueued}, nil
}

func newMux(store *fakeStore, requireDevice bool) *http.ServeMux {
	mux := http.NewServeMux()
	httpadapter.New(store, nil, slog.New(slog.NewTextHandler(io.Discard, nil))).
		WithQueueTokens(secret, requireDevice).Routes(mux)
	return mux
}

func verifiedBuyer(extra map[string]string) map[string]string {
	h := map[string]string{"X-Identity-Id": userID, "X-Identity-Role": "buyer", "X-Identity-Verified": "true", "X-Device-Id": deviceID}
	for k, v := range extra {
		h[k] = v
	}
	return h
}

func do(mux http.Handler, method, path string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	for k, v := range hdr {
		if v == "" {
			continue
		}
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func problemType(rec *httptest.ResponseRecorder) string {
	var p struct{ Type string }
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	return p.Type
}

func token(t *testing.T, c authx.QueueClaims) string {
	t.Helper()
	if c.IssuedAt.IsZero() {
		c.IssuedAt = time.Now()
		c.ExpiresAt = c.IssuedAt.Add(time.Hour)
	}
	tok, err := authx.SignQueueToken(secret, c)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func goodClaims() authx.QueueClaims {
	return authx.QueueClaims{EventID: "e1", Subject: userID, DeviceID: deviceID, JTI: jti}
}

// BR-Q3: chi identity da xac thuc OTP moi vao duoc hang cho.
func TestJoin_ChiChoIdentityDaXacThucOTP(t *testing.T) {
	cases := []struct {
		name string
		hdr  map[string]string
		want int
	}{
		{"khong danh tinh", nil, http.StatusUnauthorized},
		{"thieu role (header tu che)", map[string]string{"X-Identity-Id": userID}, http.StatusUnauthorized},
		{"chua xac thuc OTP", verifiedBuyer(map[string]string{"X-Identity-Verified": "false"}), http.StatusForbidden},
		{"thieu co verified", verifiedBuyer(map[string]string{"X-Identity-Verified": ""}), http.StatusForbidden},
		{"da xac thuc", verifiedBuyer(nil), http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := &fakeStore{}
			rec := do(newMux(store, true), http.MethodPost, "/v1/events/e1/queue/join", c.hdr)
			if rec.Code != c.want {
				t.Fatalf("status %d, muon %d: %s", rec.Code, c.want, rec.Body)
			}
			if c.want != http.StatusOK && store.joins != 0 {
				t.Fatal("bi tu choi nhung van ghi danh vao hang")
			}
			if c.want == http.StatusForbidden && problemType(rec) != httpadapter.ProblemOTPRequired {
				t.Fatalf("type %q, muon %q", problemType(rec), httpadapter.ProblemOTPRequired)
			}
		})
	}
}

// EV-137: join tra token KY gan jti voi nguoi + thiet bi + su kien; Redis chi giu jti.
func TestJoin_CapQueueTokenKy(t *testing.T) {
	rec := do(newMux(&fakeStore{}, true), http.MethodPost, "/v1/events/e1/queue/join", verifiedBuyer(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var body struct {
		QueueToken string `json:"queue_token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)

	claims, err := authx.VerifyQueueToken(secret, body.QueueToken, time.Now())
	if err != nil {
		t.Fatalf("token tra ve phai la queue token ky hop le: %v (%q)", err, body.QueueToken)
	}
	if claims.EventID != "e1" || claims.Subject != userID || claims.DeviceID != deviceID || claims.JTI != jti {
		t.Fatalf("claims sai: %+v", claims)
	}
	if time.Until(claims.ExpiresAt) < 23*time.Hour {
		t.Fatalf("han token = %v, muon ~24 gio", claims.ExpiresAt)
	}
}

// EV-146 (BR-Q6): het ve han thi khach moi nhan SOLD_OUT, khong co queue token.
func TestJoin_HetVeHanKhongCapToken(t *testing.T) {
	rec := do(newMux(&fakeStore{soldOut: true}, true), http.MethodPost, "/v1/events/e1/queue/join", verifiedBuyer(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("het ve la ket qua nghiep vu binh thuong, muon 200, nhan %d: %s", rec.Code, rec.Body)
	}
	var body struct {
		State       string `json:"state"`
		QueueToken  string `json:"queue_token"`
		PollAfterMs int64  `json:"poll_after_ms"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.State != "SOLD_OUT" || body.QueueToken != "" || body.PollAfterMs <= 0 {
		t.Fatalf("body = %+v", body)
	}
}

func TestJoin_CanThietBiNeuBatBuoc(t *testing.T) {
	noDevice := verifiedBuyer(map[string]string{"X-Device-Id": ""})

	store := &fakeStore{}
	rec := do(newMux(store, true), http.MethodPost, "/v1/events/e1/queue/join", noDevice)
	if rec.Code != http.StatusForbidden || problemType(rec) != httpadapter.ProblemDeviceRequired || store.joins != 0 {
		t.Fatalf("status %d type %q joins %d", rec.Code, problemType(rec), store.joins)
	}
	if rec := do(newMux(&fakeStore{}, false), http.MethodPost, "/v1/events/e1/queue/join", noDevice); rec.Code != http.StatusOK {
		t.Fatalf("tat requireDevice thi join khong thiet bi phai qua: %d", rec.Code)
	}
}

func TestJoin_ThieuSecretThiTuChoi(t *testing.T) {
	mux := http.NewServeMux()
	httpadapter.New(&fakeStore{}, nil, slog.New(slog.NewTextHandler(io.Discard, nil))).Routes(mux)
	if rec := do(mux, http.MethodPost, "/v1/events/e1/queue/join", verifiedBuyer(nil)); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("thieu secret: %d, muon 503", rec.Code)
	}
}

// EV-137: status / stream chi nhan token ky dung, dung nguoi, dung thiet bi, dung su kien.
func TestStatusVaStream_XacMinhQueueToken(t *testing.T) {
	expired := goodClaims()
	expired.IssuedAt, expired.ExpiresAt = time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)
	otherEvent := goodClaims()
	otherEvent.EventID = "e2"

	cases := []struct {
		name string
		hdr  map[string]string
		want int
		typ  string
	}{
		{"hop le", verifiedBuyer(map[string]string{"X-Queue-Token": token(t, goodClaims())}), http.StatusOK, ""},
		{"thieu token", verifiedBuyer(nil), http.StatusBadRequest, ""},
		{"token hex cu", verifiedBuyer(map[string]string{"X-Queue-Token": "0123456789abcdef0123456789abcdef"}), http.StatusUnauthorized, httpadapter.ProblemQueueTokenInvalid},
		{"het han", verifiedBuyer(map[string]string{"X-Queue-Token": token(t, expired)}), http.StatusUnauthorized, httpadapter.ProblemQueueTokenExpired},
		{"token cua nguoi khac", verifiedBuyer(map[string]string{"X-Queue-Token": token(t, goodClaims()), "X-Identity-Id": otherID}), http.StatusForbidden, httpadapter.ProblemQueueTokenMismatch},
		{"sang thiet bi khac", verifiedBuyer(map[string]string{"X-Queue-Token": token(t, goodClaims()), "X-Device-Id": otherDev}), http.StatusForbidden, httpadapter.ProblemQueueTokenMismatch},
		{"token cua su kien khac", verifiedBuyer(map[string]string{"X-Queue-Token": token(t, otherEvent)}), http.StatusForbidden, httpadapter.ProblemQueueTokenMismatch},
		{"khong danh tinh", map[string]string{"X-Queue-Token": token(t, goodClaims())}, http.StatusUnauthorized, ""},
	}
	for _, c := range cases {
		t.Run("status/"+c.name, func(t *testing.T) {
			store := &fakeStore{}
			rec := do(newMux(store, true), http.MethodGet, "/v1/events/e1/queue/status", c.hdr)
			if rec.Code != c.want {
				t.Fatalf("status %d, muon %d: %s", rec.Code, c.want, rec.Body)
			}
			if c.typ != "" && problemType(rec) != c.typ {
				t.Fatalf("type %q, muon %q", problemType(rec), c.typ)
			}
			if c.want == http.StatusOK {
				if len(store.statusCalls) != 1 || store.statusCalls[0] != jti {
					t.Fatalf("Redis phai duoc hoi bang jti, nhan %v", store.statusCalls)
				}
			} else if len(store.statusCalls) != 0 {
				t.Fatal("token bi tu choi nhung van doc Redis")
			}
		})
		if c.want == http.StatusOK {
			continue // stream hop le giu ket noi mo; chi kiem nhanh tu choi
		}
		t.Run("stream/"+c.name, func(t *testing.T) {
			store := &fakeStore{}
			rec := do(newMux(store, true), http.MethodGet, "/v1/events/e1/queue/stream", c.hdr)
			if rec.Code != c.want || len(store.statusCalls) != 0 {
				t.Fatalf("status %d (muon %d), calls %v", rec.Code, c.want, store.statusCalls)
			}
		})
	}
}
