package proxy_test

// ops-warroom AC-19 (black box via the gateway).
// Assumptions: fixture/token/do helpers from gateway_test.go (same package); the six routes of contracts §2 are
// registered; the fixture's fake upstreams echo their name as body, so the resolved upstream = response body.

import (
	"io"
	"net/http"
	"testing"
)

const evUUID = "33333333-3333-4333-8333-333333333333"

type opsRoute struct {
	method, path, upstream string
	role                   string // only role allowed through
}

func opsRoutes() []opsRoute {
	return []opsRoute{
		{http.MethodGet, "/v1/ops/events/" + evUUID + "/queue", "waitingroom", "ops"},
		{http.MethodPut, "/v1/ops/events/" + evUUID + "/admit-override", "waitingroom", "ops"},
		{http.MethodDelete, "/v1/ops/events/" + evUUID + "/admit-override", "waitingroom", "ops"},
		{http.MethodGet, "/v1/ops/events/" + evUUID + "/inventory", "ticketing", "ops"},
		{http.MethodGet, "/v1/ops/ai/mode", "assistant", "ops"},
		{http.MethodGet, "/v1/organizer/events/" + evUUID + "/sales", "ticketing", "organizer"},
	}
}

func TestOpsRoutes_AC19_RoleMatrixAndUpstream(t *testing.T) {
	f := newFixture(t)
	roles := []string{"buyer", "organizer", "moderator", "ops"}
	for _, rt := range opsRoutes() {
		for _, role := range roles {
			before := f.up.count()
			resp := f.do(t, rt.method, rt.path, token(t, opsID, role, f.now), nil, `{"rate_per_sec":1,"reason":"abc"}`)
			body, _ := io.ReadAll(resp.Body)
			if role != rt.role {
				if resp.StatusCode != http.StatusForbidden {
					t.Errorf("%s %s role=%s: %d, want 403", rt.method, rt.path, role, resp.StatusCode)
				}
				if f.up.count() != before {
					t.Errorf("%s %s role=%s: request reached upstream", rt.method, rt.path, role)
				}
				continue
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("%s %s role=%s: %d, want 200", rt.method, rt.path, role, resp.StatusCode)
				continue
			}
			if string(body) != rt.upstream {
				t.Errorf("%s %s: upstream %q, want %q", rt.method, rt.path, body, rt.upstream)
			}
			got := f.up.last(t)
			if got.Path != rt.path || got.Method != rt.method {
				t.Errorf("%s %s: upstream saw %s %s (path must be unchanged)", rt.method, rt.path, got.Method, got.Path)
			}
			if got.Header.Get("X-Identity-Role") != role {
				t.Errorf("%s %s: identity role header %q", rt.method, rt.path, got.Header.Get("X-Identity-Role"))
			}
		}
	}
}

func TestOpsRoutes_AC19_NoTokenIs401(t *testing.T) {
	f := newFixture(t)
	for _, rt := range opsRoutes() {
		before := f.up.count()
		resp := f.do(t, rt.method, rt.path, "", nil, "")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s without token: %d, want 401", rt.method, rt.path, resp.StatusCode)
		}
		if f.up.count() != before {
			t.Errorf("%s %s without token reached upstream", rt.method, rt.path)
		}
	}
}

func TestOpsRoutes_AC19_OtherMethodsNotForwarded(t *testing.T) {
	f := newFixture(t)
	ops := token(t, opsID, "ops", f.now)
	before := f.up.count()
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/v1/ops/events/" + evUUID + "/queue"},
		{http.MethodPost, "/v1/ops/events/" + evUUID + "/admit-override"},
		{http.MethodGet, "/v1/ops/events/" + evUUID + "/admit-override"},
		{http.MethodPost, "/v1/ops/events/" + evUUID + "/inventory"},
	} {
		resp := f.do(t, c.method, c.path, ops, nil, "{}")
		if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: %d, want 404/405", c.method, c.path, resp.StatusCode)
		}
	}
	if f.up.count() != before {
		t.Fatal("unsupported method was forwarded to an upstream")
	}
}

func TestOpsRoutes_AC19_AdminModeStillBlockedForOps(t *testing.T) {
	f := newFixture(t)
	resp := f.do(t, http.MethodGet, "/admin/mode", token(t, opsID, "ops", f.now), nil, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("/admin/mode for ops: %d, want 404", resp.StatusCode)
	}
}

// Spoofed identity headers must not elevate a buyer on the new control-plane routes.
func TestOpsRoutes_AC19_SpoofedRoleHeaderIgnored(t *testing.T) {
	f := newFixture(t)
	buyer := token(t, buyerID, "buyer", f.now)
	resp := f.do(t, http.MethodPut, "/v1/ops/events/"+evUUID+"/admit-override", buyer,
		map[string]string{"X-Identity-Role": "ops"}, `{"rate_per_sec":0,"reason":"abc"}`)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("buyer with spoofed role header: %d, want 403", resp.StatusCode)
	}
}
