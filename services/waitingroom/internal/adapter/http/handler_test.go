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

	httpadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/http"
	"github.com/eventflow/eventflow/services/waitingroom/internal/domain"
)

const userID = "11111111-1111-4111-8111-111111111111"

type fakeStore struct{ joins int }

func (f *fakeStore) Join(_ context.Context, _, _, token string) (httpadapter.JoinResult, error) {
	f.joins++
	return httpadapter.JoinResult{State: domain.StateLobby, Token: token, Rank: -1, IsNew: true}, nil
}

func (f *fakeStore) Status(context.Context, string, string) (domain.Position, error) {
	return domain.Position{State: domain.StateQueued}, nil
}

func join(t *testing.T, store *fakeStore, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	httpadapter.New(store, nil, slog.New(slog.NewTextHandler(io.Discard, nil))).Routes(mux)
	req := httptest.NewRequest(http.MethodPost, "/v1/events/e1/queue/join", strings.NewReader(`{}`))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
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
		{"chua xac thuc OTP", map[string]string{"X-Identity-Id": userID, "X-Identity-Role": "buyer", "X-Identity-Verified": "false"}, http.StatusForbidden},
		{"thieu co verified", map[string]string{"X-Identity-Id": userID, "X-Identity-Role": "buyer"}, http.StatusForbidden},
		{"da xac thuc", map[string]string{"X-Identity-Id": userID, "X-Identity-Role": "buyer", "X-Identity-Verified": "true"}, http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := &fakeStore{}
			rec := join(t, store, c.hdr)
			if rec.Code != c.want {
				t.Fatalf("status %d, muon %d: %s", rec.Code, c.want, rec.Body)
			}
			if c.want != http.StatusOK && store.joins != 0 {
				t.Fatal("bi tu choi nhung van ghi danh vao hang")
			}
			if c.want == http.StatusForbidden {
				var p struct{ Type string }
				_ = json.NewDecoder(rec.Body).Decode(&p)
				if p.Type != httpadapter.ProblemOTPRequired {
					t.Fatalf("type %q, muon %q", p.Type, httpadapter.ProblemOTPRequired)
				}
			}
		})
	}
}
