// Package clientip xac dinh IP client cua mot request. X-Forwarded-For chi duoc doc khi ket
// noi truc tiep (RemoteAddr) den tu mot proxy tin cay (TRUSTED_PROXY_CIDRS).
package clientip

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// maxTrustedHops gioi han so phan tu tin cay duoc duyet trong X-Forwarded-For.
const maxTrustedHops = 16

// Resolver tra IP client theo tap proxy tin cay. Gia tri zero (hoac tap rong) khong bao gio
// tin X-Forwarded-For.
type Resolver struct {
	trusted []netip.Prefix
}

// New tao Resolver tu tap proxy tin cay (da kiem tra boi ParseTrusted).
func New(trusted []netip.Prefix) *Resolver {
	return &Resolver{trusted: trusted}
}

// ParseTrusted doc danh sach CIDR hoac IP ngan cach boi dau phay. Phan tu rong bi bo qua; IP
// tran thanh /32 hoac /128. Phan tu hong lam loi, thong bao neu ten phan tu.
func ParseTrusted(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, e := range strings.Split(s, ",") {
		e = strings.Trim(e, " \t")
		if e == "" {
			continue
		}
		p, err := parseTrustedElement(e)
		if err != nil {
			return nil, fmt.Errorf("phan tu %q: %w", e, err)
		}
		out = append(out, p)
	}
	return out, nil
}

func parseTrustedElement(e string) (netip.Prefix, error) {
	if !strings.Contains(e, "/") {
		a, err := netip.ParseAddr(e)
		if err != nil {
			return netip.Prefix{}, fmt.Errorf("khong phai IP hay CIDR")
		}
		if a.Zone() != "" {
			return netip.Prefix{}, fmt.Errorf("khong chap nhan zone")
		}
		if a.Is4In6() {
			return netip.Prefix{}, fmt.Errorf("IPv4-mapped IPv6, hay viet dang IPv4")
		}
		return netip.PrefixFrom(a, a.BitLen()), nil
	}
	p, err := netip.ParsePrefix(e)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("khong phai CIDR hop le")
	}
	if p.Addr().Is4In6() {
		return netip.Prefix{}, fmt.Errorf("IPv4-mapped IPv6, hay viet dang IPv4")
	}
	if p.Bits() == 0 {
		return netip.Prefix{}, fmt.Errorf("prefix /0 tin ca the gioi")
	}
	if p != p.Masked() {
		return netip.Prefix{}, fmt.Errorf("co bit host, hay dung %s", p.Masked())
	}
	return p, nil
}

// Resolve tra IP client va true neu no lay tu X-Forwarded-For. Moi truong hop lui ve peer
// tra peerKey(remoteAddr), trung clientIP cu cua gateway.
func (r *Resolver) Resolve(remoteAddr string, xff []string) (string, bool) {
	key := peerKey(remoteAddr)
	if r == nil || len(r.trusted) == 0 {
		return key, false
	}
	peer, err := netip.ParseAddr(key)
	if err != nil || peer.Zone() != "" || !r.isTrusted(peer.Unmap()) {
		return key, false
	}

	// Duyet phai sang trai, dong cuoi truoc, khong tach toan bo danh sach.
	var leftmost netip.Addr
	hops := 0
	for i := len(xff) - 1; i >= 0; i-- {
		line := xff[i]
		end := len(line)
		for j := end - 1; j >= -1; j-- {
			if j >= 0 && line[j] != ',' {
				continue
			}
			e := trimSpaceTab(line[j+1 : end])
			end = j
			if e != "" {
				a, ok := parseAddr(e)
				if !ok {
					return key, false
				}
				if !r.isTrusted(a) {
					return a.String(), true
				}
				hops++
				if hops > maxTrustedHops {
					return key, false
				}
				leftmost = a
			}
		}
	}
	if leftmost.IsValid() {
		return leftmost.String(), true
	}
	return key, false
}

func (r *Resolver) isTrusted(a netip.Addr) bool {
	for _, p := range r.trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func trimSpaceTab(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

// peerKey la host cua RemoteAddr, hoac RemoteAddr tho neu khong tach duoc (hanh vi cu).
func peerKey(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

// parseAddr doc mot phan tu X-Forwarded-For: IP, IP:port, [IPv6], [IPv6]:port. IPv4-mapped
// duoc unmap; zone va moi dang khac la hong.
func parseAddr(e string) (netip.Addr, bool) {
	host := e
	switch {
	case len(e) >= 2 && e[0] == '[' && e[len(e)-1] == ']':
		host = e[1 : len(e)-1]
	case e[0] == '[' || strings.Count(e, ":") == 1:
		h, _, err := net.SplitHostPort(e)
		if err != nil {
			return netip.Addr{}, false
		}
		host = h
	}
	a, err := netip.ParseAddr(host)
	if err != nil || a.Zone() != "" {
		return netip.Addr{}, false
	}
	return a.Unmap(), true
}
