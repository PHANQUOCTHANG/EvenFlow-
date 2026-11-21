package config_test

import (
	"strings"
	"testing"
)

// Amendment 1 (AC-8): prefix qua rong lam Load loi, thong bao neu ten bien va phan tu.
func TestLoad_Round1_WidePrefixFailsFast(t *testing.T) {
	for _, tc := range []struct{ in, bad string }{
		{"0.0.0.0/1,128.0.0.0/1", "0.0.0.0/1"},
		{"10.244.0.0/16, 2000::/3", "2000::/3"},
		{"::/1,8000::/1", "::/1"},
	} {
		c, err := loadWithTrusted(t, tc.in)
		if err == nil || c != nil {
			t.Fatalf("%s=%q: Load = (%v, %v), want error and nil config", trustedEnv, tc.in, c, err)
		}
		if msg := err.Error(); !strings.Contains(msg, trustedEnv) || !strings.Contains(msg, tc.bad) {
			t.Fatalf("%s=%q: error %q must name %s and %q", trustedEnv, tc.in, msg, trustedEnv, tc.bad)
		}
	}
}

// C-7: gia tri ConfigMap viet bang YAML block scalar (co LF/CRLF cuoi) van nap duoc.
func TestLoad_Round1_TrailingNewlineLoads(t *testing.T) {
	for _, in := range []string{"10.244.0.0/16\n", "10.244.0.0/16\r\n", "10.244.0.0/16,\nfd00::/8\n"} {
		c, err := loadWithTrusted(t, in)
		if err != nil {
			t.Fatalf("%s=%q: Load error %v", trustedEnv, in, err)
		}
		if len(c.TrustedProxies) == 0 || c.TrustedProxies[0].String() != "10.244.0.0/16" {
			t.Fatalf("%s=%q: TrustedProxies = %v", trustedEnv, in, c.TrustedProxies)
		}
	}
}
