//go:build integration

package leader

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/testx"
)

// Test tich hop tren Redis THAT. Lua cua khoa/epoch va hop dong fencing chi chung minh duoc o day.
// Khong co Docker thi FAIL (khong skip) theo hop dong cua CI.

func newRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: testx.Redis(t)})
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

func uniq(t *testing.T) (lock, epoch string) {
	t.Helper()
	n := time.Now().UnixNano()
	return fmt.Sprintf("test:{%d}:leader", n), fmt.Sprintf("test:{%d}:leader_epoch", n)
}

func newElector(t *testing.T, rdb *redis.Client, lock, epoch, id string, ttl, renew time.Duration) *Elector {
	t.Helper()
	e, err := New(rdb, Config{LockKey: lock, EpochKey: epoch, ID: id, TTL: ttl, RenewEvery: renew},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// Epoch tang don dieu moi lan doi chu, ke ca khi khoa het han; chi chu khoa moi gia han / nha duoc.
func TestTichHop_Leader_LuaKhoaVaEpoch(t *testing.T) {
	rdb, ctx := newRedis(t), context.Background()
	lock, epoch := uniq(t)
	st := &redisStore{rdb: rdb, lock: lock, epoch: epoch}

	e1, err := st.acquire(ctx, "a", time.Minute)
	if err != nil || e1 <= 0 {
		t.Fatalf("lan dau: epoch %d err %v, muon > 0", e1, err)
	}
	if e, _ := st.acquire(ctx, "b", time.Minute); e != 0 {
		t.Fatalf("khoa dang do a ma b lay duoc (epoch %d)", e)
	}
	if ok, _ := st.renew(ctx, "b", time.Minute); ok {
		t.Fatal("b khong phai chu khoa ma gia han duoc")
	}
	if err := st.release(ctx, "b"); err != nil {
		t.Fatal(err)
	}
	if v, _ := rdb.Get(ctx, lock).Result(); v != "a" {
		t.Fatalf("b nha nham khoa cua a, gia tri = %q", v)
	}
	if ok, _ := st.renew(ctx, "a", time.Minute); !ok {
		t.Fatal("chu khoa gia han that bai")
	}

	// Khoa het han (a chet): b len, epoch phai TANG.
	rdb.Del(ctx, lock)
	e2, _ := st.acquire(ctx, "b", time.Minute)
	if e2 <= e1 {
		t.Fatalf("sau khi doi chu epoch = %d, muon > %d", e2, e1)
	}
	// Thu lai cua chinh chu khoa (loi mang) khong lam tang epoch.
	if e, _ := st.acquire(ctx, "b", time.Minute); e != e2 {
		t.Fatalf("chu khoa lay lai: epoch = %d, muon giu %d", e, e2)
	}
}

// Mat key epoch (flush, xoa tay, script don dep) KHONG duoc dua epoch ve so nho: neu khong, mot
// leader cu mang epoch cu lai duoc noi ghi chap nhan va fencing mat tac dung.
func TestTichHop_Leader_MatKeyEpochKhongLamEpochQuayLai(t *testing.T) {
	rdb, ctx := newRedis(t), context.Background()
	lock, epoch := uniq(t)
	st := &redisStore{rdb: rdb, lock: lock, epoch: epoch}

	old, _ := st.acquire(ctx, "a", time.Minute)
	if old <= 0 {
		t.Fatalf("epoch = %d", old)
	}

	time.Sleep(5 * time.Millisecond) // dong ho epoch tinh bang ms
	rdb.Del(ctx, lock, epoch)        // mat sach ca khoa lan epoch
	fresh, _ := st.acquire(ctx, "b", time.Minute)
	if fresh <= old {
		t.Fatalf("sau khi mat key epoch moi = %d, phai > epoch cu %d (leader cu se lot qua fencing)", fresh, old)
	}
}

// Nhieu instance tranh nhau: tai MOI thoi diem chi co tối đa mot leader.
func TestTichHop_Leader_ChiMotLeaderTaiMoiThoiDiem(t *testing.T) {
	rdb := newRedis(t)
	lock, epoch := uniq(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var active, maxActive, terms atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		e := newElector(t, rdb, lock, epoch, fmt.Sprintf("inst-%d", i), 300*time.Millisecond, 50*time.Millisecond)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = e.Run(ctx, func(c context.Context, _ int64) error {
				n := active.Add(1)
				for {
					m := maxActive.Load()
					if n <= m || maxActive.CompareAndSwap(m, n) {
						break
					}
				}
				terms.Add(1)
				select {
				case <-c.Done():
				case <-time.After(150 * time.Millisecond): // lam het mot nhiem ky ngan roi nhuong
				}
				active.Add(-1)
				return nil
			})
		}()
	}
	wg.Wait()

	if maxActive.Load() != 1 {
		t.Fatalf("co luc %d instance cung la leader, muon toi da 1", maxActive.Load())
	}
	if terms.Load() < 3 {
		t.Errorf("chi co %d nhiem ky trong 3 giay: quyen leader khong luan chuyen duoc", terms.Load())
	}
}

// Leader chet dot ngot (khong nha khoa): nguoi khac len sau toi da TTL, voi epoch lon hon.
func TestTichHop_Leader_LeaderChetThiNguoiKhacLenSauTTL(t *testing.T) {
	rdb, ctx := newRedis(t), context.Background()
	lock, epoch := uniq(t)
	st := &redisStore{rdb: rdb, lock: lock, epoch: epoch}

	// "Leader chet": lay khoa roi khong bao gio gia han hay nha.
	deadEpoch, _ := st.acquire(ctx, "chet", 400*time.Millisecond)
	if deadEpoch <= 0 {
		t.Fatalf("epoch = %d", deadEpoch)
	}

	e2 := newElector(t, rdb, lock, epoch, "song", 400*time.Millisecond, 50*time.Millisecond)
	rctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	start := time.Now()
	got := make(chan int64, 1)
	go func() {
		_ = e2.Run(rctx, func(c context.Context, ep int64) error {
			got <- ep
			<-c.Done()
			return nil
		})
	}()

	select {
	case ep := <-got:
		if ep <= deadEpoch {
			t.Fatalf("epoch cua leader moi = %d, muon > %d", ep, deadEpoch)
		}
		if d := time.Since(start); d > 1500*time.Millisecond {
			t.Fatalf("failover mat %v, muon <= TTL (400ms) cong cham tre", d)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("khong co leader moi sau khi leader cu chet")
	}
}

// Tat em: nha khoa nen leader moi len gan nhu ngay, khong phai doi het TTL.
func TestTichHop_Leader_TatEmThiFailoverNhanh(t *testing.T) {
	rdb := newRedis(t)
	lock, epoch := uniq(t)
	ttl, renew := 10*time.Second, 50*time.Millisecond // TTL dai: neu khong nha khoa thi test se timeout

	a := newElector(t, rdb, lock, epoch, "a", ttl, renew)
	actx, acancel := context.WithCancel(context.Background())
	aStarted := make(chan struct{})
	aDone := make(chan struct{})
	go func() {
		defer close(aDone)
		_ = a.Run(actx, func(c context.Context, _ int64) error {
			close(aStarted)
			<-c.Done()
			return nil
		})
	}()
	within(t, 2*time.Second, "a lam leader", aStarted)

	b := newElector(t, rdb, lock, epoch, "b", ttl, renew)
	bctx, bcancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer bcancel()
	bGot := make(chan struct{})
	go func() {
		_ = b.Run(bctx, func(c context.Context, _ int64) error {
			close(bGot)
			<-c.Done()
			return nil
		})
	}()

	start := time.Now()
	acancel()
	<-aDone
	within(t, 2*time.Second, "b len sau khi a tat em", bGot)
	if d := time.Since(start); d > time.Second {
		t.Fatalf("failover khi tat em mat %v, muon < 1s", d)
	}
}

// Kich ban cot loi: leader cu "dung hinh" qua han lease roi tinh lai. Lenh ghi cua no mang epoch
// cu phai bi noi ghi tu choi, trong khi leader moi ghi binh thuong.
func TestTichHop_Leader_LeaderCuDungHinhBiFencingChan(t *testing.T) {
	rdb, ctx := newRedis(t), context.Background()
	lock, epoch := uniq(t)
	st := &redisStore{rdb: rdb, lock: lock, epoch: epoch}
	dataKey := fmt.Sprintf("test:{%d}:data", time.Now().UnixNano())

	// Noi ghi mau theo dung hop dong trong tai lieu cua package.
	write := redis.NewScript(`
if tonumber(redis.call('GET', KEYS[1]) or '0') ~= tonumber(ARGV[1]) then
  return redis.error_reply('FENCED')
end
return redis.call('INCR', KEYS[2])
`)
	do := func(ep int64) error {
		return write.Run(ctx, rdb, []string{epoch, dataKey}, ep).Err()
	}

	aEpoch, _ := st.acquire(ctx, "a", 100*time.Millisecond)
	if err := do(aEpoch); err != nil {
		t.Fatalf("leader hop le bi chan: %v", err)
	}

	// a dung hinh qua han lease; b len voi epoch moi.
	time.Sleep(150 * time.Millisecond)
	bEpoch, _ := st.acquire(ctx, "b", time.Minute)
	if bEpoch <= aEpoch {
		t.Fatalf("epoch khong tang: a=%d b=%d", aEpoch, bEpoch)
	}

	// a tinh lai va ghi voi epoch cu.
	err := do(aEpoch)
	if !IsFenced(err) {
		t.Fatalf("leader cu phai bi FENCED, nhan %v", err)
	}
	if err := do(bEpoch); err != nil {
		t.Fatalf("leader moi bi chan: %v", err)
	}
	if n, _ := rdb.Get(ctx, dataKey).Int(); n != 2 {
		t.Fatalf("du lieu = %d, muon 2 (mot lan cua a luc con hieu luc, mot lan cua b; lan cua a luc cu bi chan)", n)
	}
}
