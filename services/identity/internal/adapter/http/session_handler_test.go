package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	identityHttp "github.com/eventflow/eventflow/services/identity/internal/adapter/http"
	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/google/uuid"
)

type fakeSessions struct {
	refreshErr    error
	logoutCalls   []string
	logoutAllFor  uuid.UUID
	identity      *domain.Identity
	refreshCalled string
}

func (f *fakeSessions) Refresh(_ context.Context, raw string) (*domain.TokenPair, *domain.Identity, error) {
	f.refreshCalled = raw
	if f.refreshErr != nil {
		return nil, nil, f.refreshErr
	}
	now := time.Now()
	return &domain.TokenPair{
		AccessToken: "new-access", AccessExpiresAt: now.Add(15 * time.Minute),
		RefreshToken: "new-refresh", RefreshExpiresAt: now.Add(24 * time.Hour),
	}, f.identity, nil
}

func (f *fakeSessions) Logout(_ context.Context, raw string) error {
	f.logoutCalls = append(f.logoutCalls, raw)
	return nil
}

func (f *fakeSessions) LogoutAll(_ context.Context, id uuid.UUID) error {
	f.logoutAllFor = id
	return nil
}

const sessionTestSecret = "session-handler-secret-16+"

func newSessionMux(s *fakeSessions) *http.ServeMux {
	h := identityHttp.NewAuthHandler(nil, nil, nil, nil, nil, s, nil)
	mux := http.NewServeMux()
	identityHttp.RegisterRoutes(mux, h, sessionTestSecret, nil)
	return mux
}

func TestRefreshEndpoint_TraCapTokenMoi(t *testing.T) {
	s := &fakeSessions{identity: &domain.Identity{ID: uuid.New(), Role: domain.RoleBuyer}}
	rec := httptest.NewRecorder()
	newSessionMux(s).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		strings.NewReader(`{"refresh_token":"old"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["access_token"] != "new-access" || body["token"] != "new-access" || body["refresh_token"] != "new-refresh" || body["token_type"] != "Bearer" {
		t.Fatalf("body thieu truong token: %v", body)
	}
	if exp, _ := body["expires_in"].(float64); exp < 890 || exp > 900 {
		t.Fatalf("expires_in = %v, muon ~900", body["expires_in"])
	}
	if s.refreshCalled != "old" {
		t.Fatalf("handler phai chuyen dung token cu, nhan %q", s.refreshCalled)
	}
}

func TestRefreshEndpoint_MaLoi(t *testing.T) {
	cases := []struct {
		name string
		body string
		err  error
		want int
	}{
		{"thieu token", `{}`, nil, http.StatusBadRequest},
		{"json hong", `{`, nil, http.StatusBadRequest},
		{"token khong hop le", `{"refresh_token":"x"}`, domain.ErrRefreshTokenInvalid, http.StatusUnauthorized},
		{"tai su dung", `{"refresh_token":"x"}`, domain.ErrRefreshTokenReused, http.StatusUnauthorized},
		{"loi he thong", `{"refresh_token":"x"}`, errors.New("db down"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newSessionMux(&fakeSessions{refreshErr: c.err}).ServeHTTP(rec,
				httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(c.body)))
			if rec.Code != c.want {
				t.Fatalf("status %d, muon %d", rec.Code, c.want)
			}
			if strings.Contains(rec.Body.String(), "db down") {
				t.Fatal("loi noi bo bi lo ra client")
			}
		})
	}
}

func TestLogoutEndpoint_Luon204(t *testing.T) {
	s := &fakeSessions{}
	rec := httptest.NewRecorder()
	newSessionMux(s).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout",
		strings.NewReader(`{"refresh_token":"bat-ky"}`)))
	if rec.Code != http.StatusNoContent || len(s.logoutCalls) != 1 || s.logoutCalls[0] != "bat-ky" {
		t.Fatalf("status %d, calls %v", rec.Code, s.logoutCalls)
	}
}

func TestLogoutAllEndpoint_CanAccessToken(t *testing.T) {
	s := &fakeSessions{}
	mux := newSessionMux(s)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout-all", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("khong co token phai 401, nhan %d", rec.Code)
	}

	id := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout-all", nil)
	req.Header.Set("Authorization", "Bearer "+generateTestToken(sessionTestSecret, id, domain.RoleBuyer, time.Minute))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || s.logoutAllFor != id {
		t.Fatalf("status %d, logoutAllFor %v, muon %v", rec.Code, s.logoutAllFor, id)
	}
}
