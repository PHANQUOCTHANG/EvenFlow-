package antibot

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/netip"
)

// FPHash is hex(HMAC-SHA256(key, fp)) truncated to 32 characters (D10). The raw fp is never stored.
func FPHash(key []byte, fp string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(fp))
	var sum [sha256.Size]byte
	return hex.EncodeToString(m.Sum(sum[:0]))[:32]
}

// NetPrefix reduces an address to /24 (IPv4) or /64 (IPv6) after unmapping. Zero for an invalid address.
func NetPrefix(a netip.Addr) netip.Prefix {
	if !a.IsValid() {
		return netip.Prefix{}
	}
	a = a.Unmap()
	bits := 64
	if a.Is4() {
		bits = 24
	}
	p, err := a.WithZone("").Prefix(bits)
	if err != nil {
		return netip.Prefix{}
	}
	return p
}
