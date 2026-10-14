//go:build integration

package loadsignal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/testx"
)

func newRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: testx.Redis(t)})
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

func putSnapshot(t *testing.T, rdb *redis.Client, key string, s Snapshot) {
	t.Helper()
	raw, _ := json.Marshal(s)
	if err := rdb.HSet(context.Background(), key, s.Pod, raw).Err(); err != nil {
		t.Fatal(err)
	}
}

// Gop nhieu pod: p99 va pool lay pod te nhat, ty le loi tinh tren tong.
func TestTichHop_Loadsignal_GopPodTeNhat(t *testing.T) {
	rdb := newRedis(t)
	key := fmt.Sprintf("ls:%d", time.Now().UnixNano())
	now := time.Now().UnixMilli()

	putSnapshot(t, rdb, key, Snapshot{Pod: "a", AtMs: now, Requests: 100, Errors: 1, P99Ms: 120, PoolUsage: 0.30})
	putSnapshot(t, rdb, key, Snapshot{Pod: "b", AtMs: now, Requests: 300, Errors: 9, P99Ms: 900, PoolUsage: 0.85})

	a, err := NewReader(rdb, key, 3*time.Second).Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if a.Pods != 2 || a.Requests != 400 || a.Errors != 10 {
		t.Fatalf("gop sai: %+v", a)
	}
	if a.P99 != 900*time.Millisecond || a.PoolUsage != 0.85 {
		t.Errorf("p99/pool phai lay pod te nhat: %+v", a)
	}
	if a.ErrorRate != 0.025 {
		t.Errorf("ErrorRate = %v, muon 0.025 (10/400)", a.ErrorRate)
	}
}

// Pod chet (anh chup cu) khong duoc coi la "van khoe"; het pod nao con song la KHONG BIET.
func TestTichHop_Loadsignal_AnhChupCuBiBoQua_KhongPodThiErrNoSignal(t *testing.T) {
	rdb := newRedis(t)
	key := fmt.Sprintf("ls:%d", time.Now().UnixNano())
	old := time.Now().Add(-time.Minute).UnixMilli()

	putSnapshot(t, rdb, key, Snapshot{Pod: "chet", AtMs: old, Requests: 10, P99Ms: 10})

	_, err := NewReader(rdb, key, 3*time.Second).Read(context.Background())
	if !errors.Is(err, ErrNoSignal) {
		t.Fatalf("muon ErrNoSignal, nhan %v", err)
	}
	// Anh chup cu hon 10*maxAge duoc don de hash khong phinh mai.
	if n, _ := rdb.HLen(context.Background(), key).Result(); n != 0 {
		t.Errorf("anh chup cu chua duoc don, con %d", n)
	}

	// Key chua tung ton tai cung la KHONG BIET.
	_, err = NewReader(rdb, key+":khong-co", 3*time.Second).Read(context.Background())
	if !errors.Is(err, ErrNoSignal) {
		t.Fatalf("key khong ton tai: muon ErrNoSignal, nhan %v", err)
	}
}

// Publisher -> Reader qua Redis that: tin hieu cua Recorder toi duoc ben doc trong ~1 giay.
func TestTichHop_Loadsignal_PublisherToiReader(t *testing.T) {
	rdb := newRedis(t)
	key := fmt.Sprintf("ls:%d", time.Now().UnixNano())
	rec := NewRecorder()
	for i := 0; i < 20; i++ {
		rec.Observe(40*time.Millisecond, false)
	}
	rec.Observe(2*time.Second, true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go NewPublisher(rdb, key, "pod-x", rec, func() float64 { return 0.5 }, nil).Run(ctx)

	deadline := time.Now().Add(3 * time.Second)
	for {
		a, err := NewReader(rdb, key, 3*time.Second).Read(ctx)
		if err == nil {
			if a.Pods != 1 || a.Requests != 21 || a.Errors != 1 || a.PoolUsage != 0.5 || a.P99 != 2*time.Second {
				t.Fatalf("anh chup sai: %+v", a)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("publisher khong ghi trong 3 giay: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
