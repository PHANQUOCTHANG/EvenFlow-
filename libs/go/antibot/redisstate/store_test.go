package redisstate_test

import (
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/antibot"
	"github.com/eventflow/eventflow/libs/go/antibot/redisstate"
)

// R3-1: every key observe.lua touches is declared, shares the event hash tag and carries no raw IP.
func TestKeys_R31_AllKeysDeclaredUnderOneTag(t *testing.T) {
	now := time.Date(2026, 12, 1, 12, 3, 30, 0, time.UTC)
	in := antibot.Input{EventID: "ev-9", IdentityID: "id-1", ClientIP: netip.MustParseAddr("203.0.113.77"), Now: now}
	keys := redisstate.Keys(in, "0123456789abcdef0123456789abcdef", netip.MustParsePrefix("203.0.113.0/24"), "00000000000000aa")
	if len(keys) != 16 {
		t.Fatalf("%d keys, want 16", len(keys))
	}
	if keys[0] != "wr:{ev-9}:tok" || keys[1] != "ab:{ev-9}:blk:id-1" {
		t.Fatalf("slot / flag keys %q %q", keys[0], keys[1])
	}
	minute := now.UnixMilli() / 60_000
	if !strings.HasSuffix(keys[10], ":jc:"+itoa(minute)) || !strings.HasSuffix(keys[6], ":jc:"+itoa(minute-4)) {
		t.Fatalf("jc buckets %q..%q not built from Input.Now", keys[6], keys[10])
	}
	if keys[15] != "ab:{ev-9}:nj:203.0.113.0/24:"+itoa(minute) {
		t.Fatalf("nj bucket %q", keys[15])
	}
	for _, k := range keys {
		if !strings.Contains(k, "{ev-9}") {
			t.Errorf("key %q outside the event hash tag", k)
		}
		if strings.Contains(k, "203.0.113.77") {
			t.Errorf("key %q carries the full IP", k)
		}
	}
}

func itoa(n int64) string {
	b := []byte{}
	if n == 0 {
		return "0"
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
