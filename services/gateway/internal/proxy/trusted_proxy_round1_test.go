package proxy_test

import (
	"net/http"
	"testing"
)

// C-3: header IP client do client gui (X-Real-IP, True-Client-IP, X-Original-Forwarded-For,
// CF-Connecting-IP, Forwarded) khong bao gio toi upstream, du trust bat hay tat, peer tin hay
// khong; upstream chi nhan X-Forwarded-For do gateway dat.
func TestTrustedProxy_Round1_ClientIPHeadersStripped(t *testing.T) {
	forged := []string{"X-Real-IP", "True-Client-IP", "X-Original-Forwarded-For", "CF-Connecting-IP", "Forwarded"}
	for _, tc := range []struct {
		name, peer, trust, wantXFF string
	}{
		{"trust off", "203.0.113.9:4000", "", "203.0.113.9"},
		{"untrusted peer", "203.0.113.9:4000", "10.0.0.0/8", "203.0.113.9"},
		{"trusted peer", "10.1.2.3:4000", "10.0.0.0/8", "198.51.100.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var tr = trust()
			if tc.trust != "" {
				tr = trust(tc.trust)
			}
			f := newPeerFixture(t, tc.peer, tr)
			h := xff("198.51.100.1")
			h.Set("X-Real-IP", "5.5.5.5")
			h.Set("True-Client-IP", "7.7.7.7")
			h.Set("X-Original-Forwarded-For", "8.8.8.8")
			h.Set("CF-Connecting-IP", "9.9.9.9")
			h.Set("Forwarded", "for=6.6.6.6")
			for _, path := range []string{"/api/v1/auth/login", "/api/v1/events/abc"} {
				method := http.MethodPost
				if path == "/api/v1/events/abc" {
					method = http.MethodGet
				}
				resp := send(t, f, method, path, "", h)
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("%s %s: status %d", method, path, resp.StatusCode)
				}
				got := f.up.last(t).Header
				for _, k := range forged {
					if v := got.Values(k); len(v) != 0 {
						t.Fatalf("%s %s: upstream got %s=%q, want none", method, path, k, v)
					}
				}
				wantUpstreamXFF(t, f, tc.wantXFF)
			}
		})
	}
}
