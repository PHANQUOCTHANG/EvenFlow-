package ratelimit_test

import (
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/gateway/internal/ratelimit"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func TestLimiter_ChoDotBienRoiChan(t *testing.T) {
	c := &clock{t: time.Unix(0, 0)}
	l := ratelimit.New(1, 3, c.now)

	for i := range 3 {
		if ok, _ := l.Allow("a"); !ok {
			t.Fatalf("request %d trong burst phai qua", i+1)
		}
	}
	ok, wait := l.Allow("a")
	if ok {
		t.Fatal("request thu 4 phai bi chan")
	}
	if wait <= 0 || wait > time.Second {
		t.Fatalf("thoi gian cho = %v, muon (0, 1s]", wait)
	}
}

func TestLimiter_NapLaiTheoThoiGian(t *testing.T) {
	c := &clock{t: time.Unix(0, 0)}
	l := ratelimit.New(2, 1, c.now) // 2 req/s

	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("request dau phai qua")
	}
	if ok, _ := l.Allow("a"); ok {
		t.Fatal("het token phai chan")
	}
	c.t = c.t.Add(500 * time.Millisecond)
	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("sau 0.5s voi 2 req/s phai co lai 1 token")
	}
}

func TestLimiter_KhoaDocLap(t *testing.T) {
	c := &clock{t: time.Unix(0, 0)}
	l := ratelimit.New(1, 1, c.now)

	_, _ = l.Allow("a")
	if ok, _ := l.Allow("b"); !ok {
		t.Fatal("khoa b khong duoc bi anh huong boi a")
	}
}

func TestLimiter_DonBucketNhanRoi(t *testing.T) {
	c := &clock{t: time.Unix(0, 0)}
	l := ratelimit.New(1, 1, c.now)
	for _, k := range []string{"a", "b", "c"} {
		_, _ = l.Allow(k)
	}
	c.t = c.t.Add(11 * time.Minute)
	_, _ = l.Allow("d")
	if n := l.Len(); n != 1 {
		t.Fatalf("con %d bucket, muon 1 (chi d)", n)
	}
}
