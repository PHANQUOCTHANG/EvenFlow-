package clientip

import (
	"strings"
	"testing"
)

// Amendment 1 (C-1): prefix IPv4 rong hon /8 va IPv6 rong hon /7 bi tu choi, loi neu ten phan tu.
func TestParseTrusted_Round1_WidePrefixRejected(t *testing.T) {
	for _, tc := range []struct{ in, bad string }{
		{"0.0.0.0/1", "0.0.0.0/1"},
		{"0.0.0.0/1,128.0.0.0/1", "0.0.0.0/1"},
		{"10.0.0.0/8, 128.0.0.0/1", "128.0.0.0/1"},
		{"0.0.0.0/4", "0.0.0.0/4"},
		{"8.0.0.0/7", "8.0.0.0/7"},
		{"::/1", "::/1"},
		{"::/1,8000::/1", "::/1"},
		{"fd00::/8, 8000::/1", "8000::/1"},
		{"2000::/3", "2000::/3"},
		{"fc00::/6", "fc00::/6"},
	} {
		got, err := ParseTrusted(tc.in)
		if err == nil {
			t.Fatalf("ParseTrusted(%q) = %v, want error", tc.in, got)
		}
		if !strings.Contains(err.Error(), tc.bad) {
			t.Fatalf("ParseTrusted(%q) error %q does not name %q", tc.in, err, tc.bad)
		}
	}
}

// Amendment 1 (C-1): dung muc san van hop le; IP tran khong bi anh huong.
func TestParseTrusted_Round1_FloorBoundaryAccepted(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"10.0.0.0/8", "10.0.0.0/8"},
		{"fc00::/7", "fc00::/7"},
		{"fd00::/8", "fd00::/8"},
		{"10.0.0.1", "10.0.0.1/32"},
		{"2001:db8::1", "2001:db8::1/128"},
	} {
		got, err := ParseTrusted(tc.in)
		if err != nil {
			t.Fatalf("ParseTrusted(%q) error %v", tc.in, err)
		}
		if len(got) != 1 || got[0].String() != tc.want {
			t.Fatalf("ParseTrusted(%q) = %v, want [%s]", tc.in, got, tc.want)
		}
	}
}

// C-7: khoang trang quanh ca gia tri va moi phan tu (CR, LF, tab) bi bo, nhu YAML block scalar.
func TestParseTrusted_Round1_TrimsNewlines(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"10.244.0.0/16\n", []string{"10.244.0.0/16"}},
		{"10.244.0.0/16\r\n", []string{"10.244.0.0/16"}},
		{"\n10.244.0.0/16,\n fd00::/8\n", []string{"10.244.0.0/16", "fd00::/8"}},
		{"10.244.0.0/16\r\n,\r\n192.0.2.0/24\r\n", []string{"10.244.0.0/16", "192.0.2.0/24"}},
		{"\r\n", nil},
	} {
		got, err := ParseTrusted(tc.in)
		if err != nil {
			t.Fatalf("ParseTrusted(%q) error %v", tc.in, err)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("ParseTrusted(%q) = %v, want %v", tc.in, got, tc.want)
		}
		for i, w := range tc.want {
			if got[i].String() != w {
				t.Fatalf("ParseTrusted(%q)[%d] = %v, want %s", tc.in, i, got[i], w)
			}
		}
	}
	// Khoang trang ben trong phan tu van la loi.
	for _, in := range []string{"10.0.0.0 /8", "10.0.0.0/\n8", "10.0.\n0.0/8"} {
		if got, err := ParseTrusted(in); err == nil {
			t.Fatalf("ParseTrusted(%q) = %v, want error", in, got)
		}
	}
}

// C-5/R-2: [IPv4], port rong, port khong phai so hoac ngoai 0..65535 la hong -> lui ve peer.
func TestResolve_Round1_StrictElementForms(t *testing.T) {
	r := New(prefixes(t, "10.0.0.0/8"))
	const peer = "10.1.2.3:1"
	for _, e := range []string{
		"[1.2.3.4]", "[1.2.3.4]:80", "1.2.3.4:", "1.2.3.4:abc", "1.2.3.4:99999999", "1.2.3.4:65536",
		"1.2.3.4:+80", "1.2.3.4:-1", "1.2.3.4: 80", "[2001:db8::1]:", "[2001:db8::1]:x", "[2001:db8::1]:70000",
		"[::ffff:1.2.3.4]:", "198.51.100.1, [10.0.0.5]",
	} {
		if got, fromXFF := r.Resolve(peer, []string{e}); got != "10.1.2.3" || fromXFF {
			t.Fatalf("Resolve(%q) = (%q, %v), want (10.1.2.3, false)", e, got, fromXFF)
		}
	}
	for _, tc := range []struct{ e, want string }{
		{"1.2.3.4:0", "1.2.3.4"},
		{"1.2.3.4:65535", "1.2.3.4"},
		{"1.2.3.4:00080", "1.2.3.4"},
		{"[2001:db8::1]:443", "2001:db8::1"},
		{"[2001:db8::1]", "2001:db8::1"},
		{"[::ffff:1.2.3.4]:80", "1.2.3.4"},
		{"[::ffff:1.2.3.4]", "1.2.3.4"},
	} {
		if got, fromXFF := r.Resolve(peer, []string{tc.e}); got != tc.want || !fromXFF {
			t.Fatalf("Resolve(%q) = (%q, %v), want (%q, true)", tc.e, got, fromXFF, tc.want)
		}
	}
}

// C-6: tab ngay sau dau phay va phan tu giua co tab dau van duoc bo.
func TestResolve_Round1_LeadingTab(t *testing.T) {
	r := New(prefixes(t, "10.0.0.0/8"))
	for _, tc := range []struct {
		xff  []string
		want string
	}{
		{[]string{"198.51.100.1,\t10.0.0.5"}, "198.51.100.1"},
		{[]string{"198.51.100.1,\t10.0.0.5, 10.0.0.6"}, "198.51.100.1"},
		{[]string{"\t198.51.100.1"}, "198.51.100.1"},
		{[]string{"\t\t10.0.0.7"}, "10.0.0.7"},
	} {
		if got, fromXFF := r.Resolve("10.1.2.3:4000", tc.xff); got != tc.want || !fromXFF {
			t.Fatalf("Resolve(%q) = (%q, %v), want (%q, true)", tc.xff, got, fromXFF, tc.want)
		}
	}
}
