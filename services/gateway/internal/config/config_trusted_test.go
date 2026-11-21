package config_test

import (
	"net/netip"
	"strings"
	"testing"

	"github.com/eventflow/eventflow/services/gateway/internal/config"
)

const trustedEnv = "TRUSTED_PROXY_CIDRS"

func loadWithTrusted(t *testing.T, v string) (*config.Config, error) {
	t.Helper()
	t.Setenv("JWT_SECRET", "s")
	t.Setenv("APP_ENV", "")
	t.Setenv(trustedEnv, v)
	return config.Load()
}

// AC-8: TRUSTED_PROXY_CIDRS hop le duoc nap vao Config.TrustedProxies.
func TestLoad_TrustedProxyCIDRsHopLe(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"10.244.0.0/16, fd00::/8", []string{"10.244.0.0/16", "fd00::/8"}},
		{"10.0.0.1", []string{"10.0.0.1/32"}},
		{"2001:db8::1", []string{"2001:db8::1/128"}},
		{" 10.0.0.0/8 ,, ", []string{"10.0.0.0/8"}},
	} {
		c, err := loadWithTrusted(t, tc.in)
		if err != nil {
			t.Fatalf("%s=%q: %v", trustedEnv, tc.in, err)
		}
		if len(c.TrustedProxies) != len(tc.want) {
			t.Fatalf("%s=%q: TrustedProxies = %v, want %v", trustedEnv, tc.in, c.TrustedProxies, tc.want)
		}
		for i, w := range tc.want {
			if c.TrustedProxies[i] != netip.MustParsePrefix(w) {
				t.Fatalf("%s=%q: TrustedProxies[%d] = %v, want %v", trustedEnv, tc.in, i, c.TrustedProxies[i], w)
			}
		}
	}
}

// AC-8: phan tu hong lam Load loi (fail fast), thong bao neu ten bien va phan tu.
func TestLoad_TrustedProxyCIDRsHongThiLoi(t *testing.T) {
	for _, tc := range []struct{ in, bad string }{
		{"10.0.0.1/8", "10.0.0.1/8"},
		{"0.0.0.0/0", "0.0.0.0/0"},
		{"::/0", "::/0"},
		{"10.0.0.0/33", "10.0.0.0/33"},
		{"abc", "abc"},
		{"fe80::1%eth0", "fe80::1%eth0"},
		{"::ffff:10.0.0.0/104", "::ffff:10.0.0.0/104"},
		{"::ffff:10.0.0.1", "::ffff:10.0.0.1"},
		{"10.244.0.0/16, abc", "abc"},
	} {
		c, err := loadWithTrusted(t, tc.in)
		if err == nil {
			t.Fatalf("%s=%q: Load ok (%v), want error", trustedEnv, tc.in, c.TrustedProxies)
		}
		if c != nil {
			t.Fatalf("%s=%q: Load returned config with error", trustedEnv, tc.in)
		}
		if msg := err.Error(); !strings.Contains(msg, trustedEnv) || !strings.Contains(msg, tc.bad) {
			t.Fatalf("%s=%q: error %q must name %s and %q", trustedEnv, tc.in, msg, trustedEnv, tc.bad)
		}
	}
}
