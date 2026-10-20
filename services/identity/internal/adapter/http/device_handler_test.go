package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	identityHttp "github.com/eventflow/eventflow/services/identity/internal/adapter/http"
	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/google/uuid"
)

type fakeRegistrar struct {
	got []domain.DeviceInfo
	id  uuid.UUID
	err error
}

func (f *fakeRegistrar) Register(_ context.Context, _ uuid.UUID, d domain.DeviceInfo) (uuid.UUID, error) {
	f.got = append(f.got, d)
	return f.id, f.err
}

func deviceMux(s *fakeSessions, reg *fakeRegistrar) *http.ServeMux {
	h := identityHttp.NewAuthHandler(nil, nil, nil, nil, nil, s, nil).WithDevices(reg)
	mux := http.NewServeMux()
	identityHttp.RegisterRoutes(mux, h, sessionTestSecret, nil)
	return mux
}

func authed(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+generateTestToken(sessionTestSecret, uuid.New(), domain.RoleBuyer, time.Minute))
	return req
}

func TestRegisterDevice_ThanhCongVaMaLoi(t *testing.T) {
	did := uuid.New()
	cases := []struct {
		name string
		body string
		err  error
		want int
		typ  string
	}{
		{"thanh cong", `{"fingerprint":"may-A-fingerprint"}`, nil, http.StatusOK, ""},
		{"fingerprint khong hop le", `{"fingerprint":"x"}`, domain.ErrInvalidFingerprint, http.StatusBadRequest, identityHttp.ProblemInvalidFingerprint},
		{"qua nhieu thiet bi", `{"fingerprint":"may-moi-12345"}`, domain.ErrTooManyDevices, http.StatusTooManyRequests, identityHttp.ProblemTooManyDevices},
		{"json hong", `{`, nil, http.StatusBadRequest, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			deviceMux(&fakeSessions{}, &fakeRegistrar{id: did, err: c.err}).ServeHTTP(rec, authed("POST", "/api/v1/auth/devices", c.body))
			if rec.Code != c.want {
				t.Fatalf("status %d, muon %d: %s", rec.Code, c.want, rec.Body)
			}
			var body map[string]any
			_ = json.Unmarshal(rec.Body.Bytes(), &body)
			if c.want == http.StatusOK && body["device_id"] != did.String() {
				t.Fatalf("device_id = %v", body["device_id"])
			}
			if c.typ != "" && body["type"] != c.typ {
				t.Fatalf("type %v, muon %s", body["type"], c.typ)
			}
		})
	}
}

func TestRegisterDevice_CanDangNhap(t *testing.T) {
	rec := httptest.NewRecorder()
	deviceMux(&fakeSessions{}, &fakeRegistrar{}).ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/auth/devices", strings.NewReader(`{"fingerprint":"may-A-fingerprint"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("khong token: %d, muon 401", rec.Code)
	}
}

func TestRegisterDevice_IPLayTuXForwardedFor(t *testing.T) {
	reg := &fakeRegistrar{id: uuid.New()}
	req := authed("POST", "/api/v1/auth/devices", `{"fingerprint":"may-A-fingerprint"}`)
	req.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")
	deviceMux(&fakeSessions{}, reg).ServeHTTP(httptest.NewRecorder(), req)
	if len(reg.got) != 1 || reg.got[0].IP != "203.0.113.7" {
		t.Fatalf("ip = %+v, muon 203.0.113.7", reg.got)
	}
}

func TestRefreshEndpoint_ChuyenFingerprintVaMaLoiThietBi(t *testing.T) {
	s := &fakeSessions{identity: &domain.Identity{ID: uuid.New(), Role: domain.RoleBuyer}}
	rec := httptest.NewRecorder()
	deviceMux(s, &fakeRegistrar{}).ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/auth/refresh",
		strings.NewReader(`{"refresh_token":"old","device_fingerprint":"may-A-fingerprint"}`)))
	if rec.Code != http.StatusOK || s.refreshDevice == nil || s.refreshDevice.Fingerprint != "may-A-fingerprint" {
		t.Fatalf("status %d, device %+v", rec.Code, s.refreshDevice)
	}

	rec = httptest.NewRecorder()
	s2 := &fakeSessions{refreshErr: domain.ErrTooManyDevices}
	deviceMux(s2, &fakeRegistrar{}).ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/auth/refresh",
		strings.NewReader(`{"refresh_token":"old","device_fingerprint":"may-moi-12345"}`)))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("qua nhieu thiet bi khi refresh: %d, muon 429", rec.Code)
	}

	rec = httptest.NewRecorder()
	s3 := &fakeSessions{identity: s.identity}
	deviceMux(s3, &fakeRegistrar{}).ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/auth/refresh", strings.NewReader(`{"refresh_token":"old"}`)))
	if s3.refreshDevice != nil {
		t.Fatal("khong gui fingerprint thi device phai nil")
	}
}
