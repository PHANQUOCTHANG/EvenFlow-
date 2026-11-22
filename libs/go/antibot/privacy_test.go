package antibot_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/netip"
	"regexp"
	"testing"

	"github.com/eventflow/eventflow/libs/go/antibot"
)

var hex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)

// AC-15 / D10: fp hash = hex(HMAC-SHA256(key, fp)) truncated to 32 chars.
func TestFPHash_AC15_IsTruncatedHMAC(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	fp := "device-abc-12345"
	m := hmac.New(sha256.New, key)
	m.Write([]byte(fp))
	want := hex.EncodeToString(m.Sum(nil))[:32]

	got := antibot.FPHash(key, fp)
	if got != want {
		t.Fatalf("FPHash = %q, want %q", got, want)
	}
	if !hex32.MatchString(got) {
		t.Fatalf("FPHash %q is not 32 lower-case hex", got)
	}
	if antibot.FPHash(key, fp) != got {
		t.Fatal("FPHash not stable")
	}
	if antibot.FPHash([]byte("another-key-another-key-another!!"), fp) == got {
		t.Fatal("different key gives the same hash")
	}
	if antibot.FPHash(key, fp+"x") == got {
		t.Fatal("different fp gives the same hash")
	}
}

// AC-15 / D10: IP is reduced to /24 (IPv4) or /64 (IPv6); mapped IPv4 is unmapped first.
func TestNetPrefix_AC15(t *testing.T) {
	cases := []struct{ in, want string }{
		{"203.0.113.77", "203.0.113.0/24"},
		{"203.0.113.0", "203.0.113.0/24"},
		{"10.1.2.255", "10.1.2.0/24"},
		{"::ffff:203.0.113.77", "203.0.113.0/24"},
		{"2001:db8:1:2:3:4:5:6", "2001:db8:1:2::/64"},
		{"2001:db8:1:2::", "2001:db8:1:2::/64"},
	}
	for _, c := range cases {
		got := antibot.NetPrefix(netip.MustParseAddr(c.in))
		if got.String() != c.want {
			t.Errorf("NetPrefix(%s) = %s, want %s", c.in, got, c.want)
		}
		if got.Addr().String() == c.in && c.in != "203.0.113.0" && c.in != "2001:db8:1:2::" {
			t.Errorf("NetPrefix(%s) kept the full address", c.in)
		}
	}
	if p := antibot.NetPrefix(netip.Addr{}); p.IsValid() {
		t.Fatalf("zero address gave valid prefix %s", p)
	}
}
