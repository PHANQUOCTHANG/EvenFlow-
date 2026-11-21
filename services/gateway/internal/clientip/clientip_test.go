package clientip

import (
	"net"
	"net/netip"
	"strings"
	"testing"
)

func prefixes(t testing.TB, ss ...string) []netip.Prefix {
	t.Helper()
	out := make([]netip.Prefix, 0, len(ss))
	for _, s := range ss {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}

// hops tra n phan tu tin cay 10.0.0.1 .. 10.0.0.n, trai sang phai.
func hops(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = netip.AddrFrom4([4]byte{10, 0, 0, byte(i + 1)}).String()
	}
	return out
}

func joinHops(left string, n int) string {
	parts := hops(n)
	if left != "" {
		parts = append([]string{left}, parts...)
	}
	return strings.Join(parts, ", ")
}

type resolveCase struct {
	name     string
	trusted  []string
	remote   string
	xff      []string
	want     string
	wantFrom bool // true khi ket qua lay tu X-Forwarded-For
}

var trust10 = []string{"10.0.0.0/8"}

func resolveCases() []resolveCase {
	return []resolveCase{
		// AC-1: trust rong = develop. XFF bi bo qua voi moi RemoteAddr.
		{"AC1 trust off ipv4 peer", nil, "1.2.3.4:5", []string{"9.9.9.9"}, "1.2.3.4", false},
		{"AC1 trust off peer would be trusted", nil, "10.1.2.3:5", []string{"198.51.100.1"}, "10.1.2.3", false},
		{"AC1 trust off mapped peer", nil, "[::ffff:1.2.3.4]:5", []string{"9.9.9.9"}, "::ffff:1.2.3.4", false},
		{"AC1 trust off zoned peer", nil, "[fe80::1%eth0]:5", []string{"9.9.9.9"}, "fe80::1%eth0", false},
		{"AC1 trust off garbage peer", nil, "garbage", []string{"9.9.9.9"}, "garbage", false},
		{"AC1 trust off empty RemoteAddr", nil, "", []string{"9.9.9.9"}, "", false},
		{"AC1 trust empty slice", []string{}, "10.1.2.3:5", []string{"198.51.100.1"}, "10.1.2.3", false},

		// AC-2: peer khong tin cay -> XFF bi bo qua.
		{"AC2 untrusted peer", trust10, "203.0.113.9:4000", []string{"1.2.3.4"}, "203.0.113.9", false},
		{"AC2 untrusted ipv6 peer", trust10, "[2001:db8::9]:4000", []string{"1.2.3.4"}, "2001:db8::9", false},

		// AC-3: peer tin cay -> client tu XFF.
		{"AC3 trusted peer single client", trust10, "10.1.2.3:4000", []string{"198.51.100.1"}, "198.51.100.1", true},
		{"AC3 trusted peer without port", trust10, "10.1.2.3", []string{"198.51.100.1"}, "198.51.100.1", true},
		{"AC3 trusted ipv6 peer", []string{"fd00::/8"}, "[fd00::1]:443", []string{"198.51.100.1"}, "198.51.100.1", true},

		// AC-4: gia mao phan tu trai nhat.
		{"AC4 forged leftmost", trust10, "10.1.2.3:4000", []string{"6.6.6.6, 198.51.100.1"}, "198.51.100.1", true},
		{"AC4 forged leftmost other", trust10, "10.1.2.3:4000", []string{"7.7.7.7, 8.8.8.8, 198.51.100.1"}, "198.51.100.1", true},
		{"AC4 forged leftmost trusted-looking", trust10, "10.1.2.3:4000", []string{"10.9.9.9, 198.51.100.1"}, "198.51.100.1", true},

		// AC-5: chuoi proxy.
		{"AC5 chain", []string{"10.0.0.0/8", "192.0.2.0/24"}, "10.1.2.3:4000", []string{"198.51.100.1, 192.0.2.10"}, "198.51.100.1", true},
		{"AC5 untrusted middle hop", []string{"10.0.0.0/8", "192.0.2.0/24"}, "10.1.2.3:4000", []string{"198.51.100.1, 203.0.113.50, 192.0.2.10"}, "203.0.113.50", true},
		{"AC5 second cidr not trusted when absent", trust10, "10.1.2.3:4000", []string{"198.51.100.1, 192.0.2.10"}, "192.0.2.10", true},

		// AC-6: dinh dang.
		{"AC6 two header lines", trust10, "10.1.2.3:4000", []string{"198.51.100.1", "10.0.0.5"}, "198.51.100.1", true},
		{"AC6 one line equivalent", trust10, "10.1.2.3:4000", []string{"198.51.100.1, 10.0.0.5"}, "198.51.100.1", true},
		{"AC6 last line is rightmost", trust10, "10.1.2.3:4000", []string{"198.51.100.1", "203.0.113.7"}, "203.0.113.7", true},
		{"AC6 lines with several elements", trust10, "10.1.2.3:4000", []string{"6.6.6.6, 198.51.100.1", "10.0.0.5, 10.0.0.6"}, "198.51.100.1", true},
		{"AC6 empty middle line", trust10, "10.1.2.3:4000", []string{"198.51.100.1", "", "10.0.0.5"}, "198.51.100.1", true},
		{"AC6 spaces tabs empty elements", trust10, "10.1.2.3:4000", []string{" \t198.51.100.1\t , ,, \t10.0.0.5 , "}, "198.51.100.1", true},
		{"AC6 trailing empty elements", trust10, "10.1.2.3:4000", []string{"198.51.100.1,,,"}, "198.51.100.1", true},
		{"AC6 ipv4 with port", trust10, "10.1.2.3:4000", []string{"198.51.100.1:5555"}, "198.51.100.1", true},
		{"AC6 trusted hop with port", trust10, "10.1.2.3:4000", []string{"198.51.100.1, 10.0.0.5:80"}, "198.51.100.1", true},
		{"AC6 ipv6 brackets and port", trust10, "10.1.2.3:4000", []string{"[2001:db8::1]:443"}, "2001:db8::1", true},
		{"AC6 ipv6 brackets only", trust10, "10.1.2.3:4000", []string{"[2001:db8::1]"}, "2001:db8::1", true},
		{"AC6 ipv6 upper case", trust10, "10.1.2.3:4000", []string{"2001:DB8::1"}, "2001:db8::1", true},
		{"AC6 ipv6 uncompressed", trust10, "10.1.2.3:4000", []string{"2001:db8:0:0:0:0:0:1"}, "2001:db8::1", true},
		{"AC6 bare ipv6 is not host:port", trust10, "10.1.2.3:4000", []string{"2001:db8::443"}, "2001:db8::443", true},
		{"AC6 mapped element unmapped", trust10, "10.1.2.3:4000", []string{"::ffff:198.51.100.1"}, "198.51.100.1", true},
		{"AC6 mapped element bracket port", trust10, "10.1.2.3:4000", []string{"[::ffff:198.51.100.1]:80"}, "198.51.100.1", true},
		{"AC6 mapped trusted hop matches ipv4 cidr", trust10, "10.1.2.3:4000", []string{"198.51.100.1, ::ffff:10.0.0.5"}, "198.51.100.1", true},
		{"AC6 mapped peer matches ipv4 cidr", trust10, "[::ffff:10.1.2.3]:4000", []string{"198.51.100.1"}, "198.51.100.1", true},
		{"AC6 mapped peer fallback key", trust10, "[::ffff:10.1.2.3]:4000", nil, "::ffff:10.1.2.3", false},
		{"AC6 ipv6 trusted cidr", []string{"10.0.0.0/8", "fd00::/8"}, "10.1.2.3:4000", []string{"198.51.100.1, [fd00::7]:80"}, "198.51.100.1", true},

		// AC-7: hong / bien.
		{"AC7 rightmost unknown", trust10, "10.1.2.3:4000", []string{"198.51.100.1, unknown"}, "10.1.2.3", false},
		{"AC7 rightmost short ipv4", trust10, "10.1.2.3:4000", []string{"198.51.100.1, 1.2.3"}, "10.1.2.3", false},
		{"AC7 rightmost zoned", trust10, "10.1.2.3:4000", []string{"198.51.100.1, fe80::1%eth0"}, "10.1.2.3", false},
		{"AC7 rightmost zoned bracket", trust10, "10.1.2.3:4000", []string{"198.51.100.1, [fe80::1%eth0]:80"}, "10.1.2.3", false},
		{"AC7 rightmost script", trust10, "10.1.2.3:4000", []string{"198.51.100.1, <script>"}, "10.1.2.3", false},
		{"AC7 rightmost obfuscated", trust10, "10.1.2.3:4000", []string{"198.51.100.1, _obfuscated"}, "10.1.2.3", false},
		{"AC7 only malformed", trust10, "10.1.2.3:4000", []string{"unknown"}, "10.1.2.3", false},
		{"AC7 malformed after trusted hop", trust10, "10.1.2.3:4000", []string{"198.51.100.1, unknown, 10.0.0.5"}, "10.1.2.3", false},
		{"AC7 malformed in last line", trust10, "10.1.2.3:4000", []string{"198.51.100.1", "unknown"}, "10.1.2.3", false},
		{"AC7 malformed left of client ignored", trust10, "10.1.2.3:4000", []string{"unknown, 198.51.100.1"}, "198.51.100.1", true},
		{"AC7 malformed line left of client ignored", trust10, "10.1.2.3:4000", []string{"<script>", "198.51.100.1"}, "198.51.100.1", true},
		{"AC7 no xff", trust10, "10.1.2.3:4000", nil, "10.1.2.3", false},
		{"AC7 empty xff line", trust10, "10.1.2.3:4000", []string{""}, "10.1.2.3", false},
		{"AC7 only empty elements", trust10, "10.1.2.3:4000", []string{" , ,\t,", ","}, "10.1.2.3", false},
		{"AC7 all trusted single", trust10, "10.1.2.3:4000", []string{"10.0.0.7"}, "10.0.0.7", true},
		{"AC7 all trusted leftmost", trust10, "10.1.2.3:4000", []string{"10.0.0.7, 10.0.0.8"}, "10.0.0.7", true},
		{"AC7 all trusted leftmost across lines", trust10, "10.1.2.3:4000", []string{"10.0.0.7", "10.0.0.8"}, "10.0.0.7", true},
		{"AC7 16 trusted hops leftmost", trust10, "10.1.2.3:4000", []string{joinHops("", 16)}, "10.0.0.1", true},
		{"AC7 client then 16 trusted hops", trust10, "10.1.2.3:4000", []string{joinHops("198.51.100.1", 16)}, "198.51.100.1", true},
		{"AC7 17 trusted hops", trust10, "10.1.2.3:4000", []string{joinHops("", 17)}, "10.1.2.3", false},
		{"AC7 client then 17 trusted hops", trust10, "10.1.2.3:4000", []string{joinHops("198.51.100.1", 17)}, "10.1.2.3", false},
		{"AC7 17 trusted hops across lines", trust10, "10.1.2.3:4000", append([]string{"198.51.100.1"}, hops(17)...), "10.1.2.3", false},
		{"AC7 empty elements do not count as hops", trust10, "10.1.2.3:4000", []string{"198.51.100.1, " + strings.Repeat(" ,", 40) + joinHops("", 16)}, "198.51.100.1", true},
		{"AC7 zoned peer trust on", trust10, "[fe80::1%eth0]:51234", []string{"198.51.100.1"}, "fe80::1%eth0", false},
		{"AC7 zoned peer never trusted", []string{"fe80::/10"}, "[fe80::1%eth0]:51234", []string{"198.51.100.1"}, "fe80::1%eth0", false},
		{"AC7 unparsable RemoteAddr", trust10, "garbage", []string{"198.51.100.1"}, "garbage", false},
		{"AC7 RemoteAddr with empty port is trusted", trust10, "10.1.2.3:", []string{"198.51.100.1"}, "198.51.100.1", true},
		{"AC7 empty RemoteAddr", trust10, "", []string{"198.51.100.1"}, "", false},
	}
}

func TestResolve(t *testing.T) {
	for _, tc := range resolveCases() {
		t.Run(tc.name, func(t *testing.T) {
			r := New(prefixes(t, tc.trusted...))
			got, from := r.Resolve(tc.remote, tc.xff)
			if got != tc.want || from != tc.wantFrom {
				t.Fatalf("Resolve(%q, %q) = (%q, %v), want (%q, %v)", tc.remote, tc.xff, got, from, tc.want, tc.wantFrom)
			}
			if !from && got != peerKey(tc.remote) {
				t.Fatalf("peer fallback %q != peerKey %q", got, peerKey(tc.remote))
			}
		})
	}
}

// AC-4: phan gia mao ben trai khong bao gio doi ket qua.
func TestResolve_ForgedPrefixNeverChangesResult(t *testing.T) {
	r := New(prefixes(t, trust10...))
	for _, forged := range []string{"", "1.1.1.1", "10.0.0.1", "unknown", "<script>", "a, b, c", "2001:db8::1", strings.Repeat("9.9.9.9, ", 50)} {
		xff := []string{forged + ", 198.51.100.1, 10.0.0.5"}
		if got, _ := r.Resolve("10.1.2.3:4000", xff); got != "198.51.100.1" {
			t.Fatalf("forged prefix %q changed result to %q", forged, got)
		}
		lines := []string{forged, "198.51.100.1", "10.0.0.5"}
		if got, _ := r.Resolve("10.1.2.3:4000", lines); got != "198.51.100.1" {
			t.Fatalf("forged line %q changed result to %q", forged, got)
		}
	}
}

// peerKey phai trung clientIP cua develop: host cua SplitHostPort, hoac RemoteAddr tho.
func TestPeerKeyMatchesDevelop(t *testing.T) {
	for _, ra := range []string{"1.2.3.4:5", "[::ffff:1.2.3.4]:5", "[fe80::1%eth0]:5", "[2001:DB8::1]:443", "garbage", "", "10.1.2.3", "10.1.2.3:", "a:b:c"} {
		want := ra
		if h, _, err := net.SplitHostPort(ra); err == nil {
			want = h
		}
		if got := peerKey(ra); got != want {
			t.Fatalf("peerKey(%q) = %q, want %q", ra, got, want)
		}
	}
}

// AC-8 (lop parser): ParseTrusted.
func TestParseTrusted(t *testing.T) {
	valid := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{" , ,", nil},
		{"10.244.0.0/16, fd00::/8", []string{"10.244.0.0/16", "fd00::/8"}},
		{"10.244.0.0/16,fd00::/8", []string{"10.244.0.0/16", "fd00::/8"}},
		{"10.0.0.1", []string{"10.0.0.1/32"}},
		{"2001:db8::1", []string{"2001:db8::1/128"}},
		{" 10.0.0.0/8 ,, ", []string{"10.0.0.0/8"}},
		{"\t10.0.0.0/8\t", []string{"10.0.0.0/8"}},
		{"192.0.2.0/24, 10.1.2.3, 2001:db8::/32", []string{"192.0.2.0/24", "10.1.2.3/32", "2001:db8::/32"}},
	}
	for _, tc := range valid {
		got, err := ParseTrusted(tc.in)
		if err != nil {
			t.Fatalf("ParseTrusted(%q) error %v", tc.in, err)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("ParseTrusted(%q) = %v, want %v", tc.in, got, tc.want)
		}
		for i, w := range tc.want {
			if got[i] != netip.MustParsePrefix(w) {
				t.Fatalf("ParseTrusted(%q)[%d] = %v, want %v", tc.in, i, got[i], w)
			}
		}
	}

	invalid := []struct{ in, bad string }{
		{"10.0.0.1/8", "10.0.0.1/8"},
		{"0.0.0.0/0", "0.0.0.0/0"},
		{"::/0", "::/0"},
		{"10.0.0.0/33", "10.0.0.0/33"},
		{"abc", "abc"},
		{"fe80::1%eth0", "fe80::1%eth0"},
		{"fe80::%eth0/64", "fe80::%eth0/64"},
		{"::ffff:10.0.0.0/104", "::ffff:10.0.0.0/104"},
		{"::ffff:10.0.0.1", "::ffff:10.0.0.1"},
		{"2001:db8::1/32", "2001:db8::1/32"},
		{"10.0.0.0/", "10.0.0.0/"},
		{"10.0.0.0/-1", "10.0.0.0/-1"},
		{"1.2.3", "1.2.3"},
		{"10.0.0.0/8, abc", "abc"},
		{"10.0.0.0/8, 0.0.0.0/0", "0.0.0.0/0"},
		{" 10.0.0.0/8 , 192.168.1.1/16 ", "192.168.1.1/16"},
	}
	for _, tc := range invalid {
		got, err := ParseTrusted(tc.in)
		if err == nil {
			t.Fatalf("ParseTrusted(%q) = %v, want error", tc.in, got)
		}
		if !strings.Contains(err.Error(), tc.bad) {
			t.Fatalf("ParseTrusted(%q) error %q does not name %q", tc.in, err, tc.bad)
		}
	}
}

// canonicalCandidates tra dang chuan (Unmap().String(), khong zone) cua moi cach doc hop ly
// mot phan tu XFF. Oracle rong rai: ket qua cua Resolve phai nam trong tap nay.
func canonicalCandidates(lines []string) map[string]bool {
	out := map[string]bool{}
	add := func(s string) {
		if a, err := netip.ParseAddr(s); err == nil && a.Zone() == "" {
			out[a.Unmap().String()] = true
		}
	}
	for _, line := range lines {
		for _, e := range strings.Split(line, ",") {
			e = strings.Trim(e, " \t")
			add(e)
			if h, _, err := net.SplitHostPort(e); err == nil {
				add(h)
			}
			if len(e) >= 2 && e[0] == '[' && e[len(e)-1] == ']' {
				add(e[1 : len(e)-1])
			}
		}
	}
	return out
}

func peerTrusted(trusted []netip.Prefix, remote string) bool {
	a, err := netip.ParseAddr(peerKey(remote))
	if err != nil || a.Zone() != "" {
		return false
	}
	a = a.Unmap()
	for _, p := range trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// AC-11: khong panic; ket qua la peerKey hoac mot dia chi co trong input; peer khong tin
// cay (hoac trust rong) thi ket qua la peerKey.
func FuzzResolve(f *testing.F) {
	for _, tc := range resolveCases() {
		f.Add(strings.Join(tc.xff, "\n"), tc.remote)
	}
	f.Add("1.2.3.4\n\n,,,\n[::1]:80", "[::ffff:10.0.0.1]:1")
	f.Add("[fe80::1%25eth0]:80, ::ffff:0:0", "10.0.0.1:1")
	f.Add(strings.Repeat("10.0.0.1,", 40), "10.255.255.255:65535")
	f.Add("[2001:db8::1", "[2001:db8::2]:1")
	f.Add("2001:db8::1]:80", "[2001:db8::3]")

	on := prefixes(f, "10.0.0.0/8", "2001:db8::/32")
	resolvers := []struct {
		trusted []netip.Prefix
		r       *Resolver
	}{
		{on, New(on)},
		{nil, New(nil)},
	}
	f.Fuzz(func(t *testing.T, xff, peer string) {
		lines := strings.Split(xff, "\n")
		key := peerKey(peer)
		cands := canonicalCandidates(lines)
		for _, rv := range resolvers {
			got, from := rv.r.Resolve(peer, lines)
			if !from {
				if got != key {
					t.Fatalf("trust=%v peer=%q xff=%q: fallback %q != peerKey %q", rv.trusted, peer, xff, got, key)
				}
			} else if !cands[got] {
				t.Fatalf("trust=%v peer=%q xff=%q: %q is neither peerKey nor a canonical XFF element", rv.trusted, peer, xff, got)
			}
			if !peerTrusted(rv.trusted, peer) && (got != key || from) {
				t.Fatalf("trust=%v untrusted peer=%q xff=%q: got (%q, %v), want peerKey %q", rv.trusted, peer, xff, got, from, key)
			}
		}
	})
}

// Sanity (khong phai gate): chi phi tuyen tinh theo kich thuoc header, khong cap phat moi phan tu.
func BenchmarkResolve(b *testing.B) {
	const mib = 1 << 20
	r := New(prefixes(b, trust10...))
	cases := []struct {
		name string
		xff  []string
	}{
		{"1MiB_empty_elements", []string{"198.51.100.1" + strings.Repeat(", ", (mib-12)/2)}},
		{"1MiB_whitespace_elements", []string{"198.51.100.1" + strings.Repeat(",\t ", (mib-12)/3)}},
		{"1MiB_trusted_elements", []string{strings.Repeat("10.0.0.1, ", mib/10)}},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(c.xff[0])))
			for b.Loop() {
				r.Resolve("10.1.2.3:4000", c.xff)
			}
		})
	}
}
