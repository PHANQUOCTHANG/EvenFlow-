//go:build integration

package concurrency

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
	"github.com/eventflow/eventflow/services/ticketing/internal/worker"
)

// Test doi soat ton kho (EV-132, EVF-37) tren Redis + Postgres THAT.
//
// Khong mock: cac loi de sai nhat nam o ranh gioi hai tang -- dinh dang id hold (hex 32 trong Redis,
// UUID co gach trong Postgres), JSON cua hash hold, va thu tu hai lan doc.

func newReconciler(e *TestEnv, eventID string, lockTTL time.Duration) *worker.InventoryReconciler {
	cfg := worker.ReconcilerConfig{EventIDs: []string{eventID}, Interval: 5 * time.Second, LockTTL: lockTTL}
	return worker.NewInventoryReconciler(e.repo, e.repo, e.gate, cfg, e.Logger)
}

func (e *TestEnv) invKey(eventID, ticketTypeID string) string {
	return fmt.Sprintf("inv:{%s}:tt:%s", eventID, ticketTypeID)
}

func (e *TestEnv) redisBuckets(t *testing.T, eventID, ticketTypeID string) map[int]int {
	t.Helper()
	raw, err := e.admin.HGetAll(context.Background(), e.invKey(eventID, ticketTypeID)).Result()
	if err != nil {
		t.Fatalf("doc ton kho redis: %v", err)
	}
	out := make(map[int]int, len(raw))
	for f, v := range raw {
		b, _ := strconv.Atoi(f)
		n, _ := strconv.Atoi(v)
		out[b] = n
	}
	return out
}

func (e *TestEnv) pgBuckets(t *testing.T, eventID string) map[int]int {
	t.Helper()
	rows, err := e.repo.ListInventory(context.Background(), eventID)
	if err != nil {
		t.Fatalf("doc ton kho postgres: %v", err)
	}
	out := make(map[int]int, len(rows))
	for _, r := range rows {
		out[r.Bucket] = r.Available
	}
	return out
}

// requireBucketsEqual kiem tung bucket chu khong chi tong: hai bucket lech nguoc nhau co the
// triet tieu nhau trong tong.
func requireBucketsEqual(t *testing.T, redis, pg map[int]int) {
	t.Helper()
	for b, want := range pg {
		if got := redis[b]; got != want {
			t.Errorf("bucket %d: redis=%d postgres=%d", b, got, want)
		}
	}
}

// cycles chay n chu ky lien tiep, cach nhau hon LockTTL de khoa cua chu ky truoc da het han.
func cycles(t *testing.T, r *worker.InventoryReconciler, eventID string, n int) {
	t.Helper()
	for i := range n {
		if err := r.Reconcile(context.Background(), eventID); err != nil {
			t.Fatalf("chu ky %d: %v", i+1, err)
		}
		time.Sleep(60 * time.Millisecond)
	}
}

func TestReconciler_LamLechRedis_TuSuaTrongHaiChuKy(t *testing.T) {
	env := setupEnv(t)
	eventID, tt := env.SeedEvent(t, 100)
	rec := newReconciler(env, eventID, 30*time.Millisecond)
	ctx := context.Background()

	// Co y lam lech: bucket 0 thieu 5, bucket 7 phong 4 (hai huong khac nhau).
	key := env.invKey(eventID, tt)
	if err := env.admin.HIncrBy(ctx, key, "0", -3).Err(); err != nil {
		t.Fatal(err)
	}
	if err := env.admin.HIncrBy(ctx, key, "0", -2).Err(); err != nil {
		t.Fatal(err)
	}
	if err := env.admin.HIncrBy(ctx, key, "7", 4).Err(); err != nil {
		t.Fatal(err)
	}
	if d := env.InventoryDrift(t, eventID, tt); d != -1 {
		t.Fatalf("khong tao duoc do lech co chu y: drift rong = %d", d)
	}

	cycles(t, rec, eventID, 2)

	requireBucketsEqual(t, env.redisBuckets(t, eventID, tt), env.pgBuckets(t, eventID))
	if d := env.InventoryDrift(t, eventID, tt); d != 0 {
		t.Fatalf("sau 2 chu ky van lech %d", d)
	}
}

func TestReconciler_KhoRedisBiXoaHet_DuocNapLai(t *testing.T) {
	env := setupEnv(t)
	eventID, tt := env.SeedEvent(t, 100)
	rec := newReconciler(env, eventID, 30*time.Millisecond)

	if err := env.admin.Del(context.Background(), env.invKey(eventID, tt)).Err(); err != nil {
		t.Fatal(err)
	}

	cycles(t, rec, eventID, 2)

	requireBucketsEqual(t, env.redisBuckets(t, eventID, tt), env.pgBuckets(t, eventID))
}

func TestReconciler_VoiHoldThat_ChiSuaDoLechInject_KhongPhaHold(t *testing.T) {
	env := setupEnv(t)
	// 3200 ve = 100/bucket: hold.lua chi quet 3 bucket, quota nho se bao het ve som (dung thiet ke)
	// va test se do vi ly do khong lien quan.
	const quota = 3200
	eventID, tt := env.SeedEvent(t, quota)
	rec := newReconciler(env, eventID, 30*time.Millisecond)
	ctx := context.Background()

	uc := app.NewCreateHold(env.gate, env.repo, env.Logger)
	const holds, qty = 20, 2
	for i := range holds {
		id := env.SeedIdentity(t, i)
		if _, err := uc.Execute(ctx, app.CreateHoldInput{
			EventID: eventID, TicketTypeID: tt, IdentityID: id,
			QueueToken: env.AdmitToken(t, eventID, id), Quantity: qty,
		}); err != nil {
			t.Fatalf("hold %d: %v", i, err)
		}
	}
	if got := env.TotalHeldOrSold(t, tt); got != holds*qty {
		t.Fatalf("dang giu %d, muon %d", got, holds*qty)
	}
	// Hai tang dang khop sau cac hold that. Neu doi soat dich sai id hold (hex 32 <-> UUID),
	// moi hold bi coi la "chua ghi" va Redis bi sua nham o buoc nay.
	cycles(t, rec, eventID, 3)
	requireBucketsEqual(t, env.redisBuckets(t, eventID, tt), env.pgBuckets(t, eventID))
	if got := env.TotalHeldOrSold(t, tt); got != holds*qty {
		t.Fatalf("doi soat lam doi hold: %d", got)
	}

	// Gio moi lam lech Redis, trong khi hold van con do.
	if err := env.admin.HIncrBy(ctx, env.invKey(eventID, tt), "3", -6).Err(); err != nil {
		t.Fatal(err)
	}
	cycles(t, rec, eventID, 2)

	requireBucketsEqual(t, env.redisBuckets(t, eventID, tt), env.pgBuckets(t, eventID))
	if got := env.TotalAvailable(t, tt) + env.TotalHeldOrSold(t, tt); got != quota {
		t.Fatalf("available + giu = %d, muon %d", got, quota)
	}
}

func TestReconciler_HoldDangBay_KhongBiCoiLaLech(t *testing.T) {
	env := setupEnv(t)
	eventID, tt := env.SeedEvent(t, 100)
	rec := newReconciler(env, eventID, 30*time.Millisecond)
	ctx := context.Background()

	// Redis da tru 5 ve cho 5 hold nhung Postgres chua commit (mo phong khoang "dang bay" hoac
	// luc client timeout truoc khi persist). Day la truong hop job KHONG duoc tu y dung vao.
	var ids []string
	for i := range 5 {
		id := fmt.Sprintf("%032x", i+1)
		res, err := env.gate.Hold(ctx, port.HoldRequest{
			EventID: eventID, TicketTypeID: tt, IdentityID: env.SeedIdentity(t, i),
			HoldID: id, Quantity: 1, TTL: time.Hour, BucketCount: bucketCount, StartBucket: i,
		})
		if err != nil || res.Status != port.HoldOK {
			t.Fatalf("hold %d: %v %v", i, res.Status, err)
		}
		ids = append(ids, id)
	}
	before := env.redisBuckets(t, eventID, tt)

	cycles(t, rec, eventID, 4)

	after := env.redisBuckets(t, eventID, tt)
	for b, v := range before {
		if after[b] != v {
			t.Errorf("bucket %d doi %d -> %d: job da dung vao hold dang bay", b, v, after[b])
		}
	}
	// Don dep: tra kho de khong ro ri sang test khac (container dung chung).
	for _, id := range ids {
		if err := env.gate.Release(ctx, eventID, id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReconciler_PostgresDaTraRedisChuaKip_KhongBiCoiLaLech(t *testing.T) {
	env := setupEnv(t)
	eventID, tt := env.SeedEvent(t, 100)
	rec := newReconciler(env, eventID, 30*time.Millisecond)
	ctx := context.Background()

	uc := app.NewCreateHold(env.gate, env.repo, env.Logger)
	id := env.SeedIdentity(t, 1)
	out, err := uc.Execute(ctx, app.CreateHoldInput{
		EventID: eventID, TicketTypeID: tt, IdentityID: id,
		QueueToken: env.AdmitToken(t, eventID, id), Quantity: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Sweeper da tra kho o Postgres nhung chua kip release o Redis (tien trinh chet giua hai buoc).
	if ok, err := env.repo.ReleaseHold(ctx, out.HoldID); err != nil || !ok {
		t.Fatalf("ReleaseHold: ok=%v err=%v", ok, err)
	}
	before := env.redisBuckets(t, eventID, tt)

	cycles(t, rec, eventID, 3)

	after := env.redisBuckets(t, eventID, tt)
	for b, v := range before {
		if after[b] != v {
			t.Errorf("bucket %d doi %d -> %d: hold cho release bi coi la lech", b, v, after[b])
		}
	}

	// Sweeper that hoan tat phan con lai (Release o Redis idempotent): hai tang khop lai.
	// Redis luu id hold dang hex 32 ky tu; HoldView.HoldID la UUID co gach cua Postgres.
	redisID := strings.ReplaceAll(out.HoldID, "-", "")
	if err := env.gate.Release(ctx, eventID, redisID); err != nil {
		t.Fatal(err)
	}
	requireBucketsEqual(t, env.redisBuckets(t, eventID, tt), env.pgBuckets(t, eventID))
}

func TestReconciler_HaiInstanceSongSong_KhongSuaGapDoi(t *testing.T) {
	env := setupEnv(t)
	eventID, tt := env.SeedEvent(t, 100)
	// LockTTL dai hon thoi gian chay mot chu ky nhung ngan hon khoang nghi giua hai chu ky.
	const lockTTL = 250 * time.Millisecond
	a := newReconciler(env, eventID, lockTTL)
	b := newReconciler(env, eventID, lockTTL)

	if err := env.admin.HIncrBy(context.Background(), env.invKey(eventID, tt), "2", -9).Err(); err != nil {
		t.Fatal(err)
	}

	for cycle := range 2 {
		var wg sync.WaitGroup
		for _, r := range []*worker.InventoryReconciler{a, b} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := r.Reconcile(context.Background(), eventID); err != nil {
					t.Errorf("chu ky %d: %v", cycle+1, err)
				}
			}()
		}
		wg.Wait()
		time.Sleep(lockTTL + 100*time.Millisecond)
	}
	// Chu ky 3: instance giu khoa o chu ky 1 co the khong phai instance giu o chu ky 2, nen can
	// mot chu ky nua de nguoi vua giu khoa thay du hai lan lien tiep.
	time.Sleep(lockTTL + 100*time.Millisecond)
	for _, r := range []*worker.InventoryReconciler{a, b} {
		_ = r.Reconcile(context.Background(), eventID)
		time.Sleep(lockTTL + 100*time.Millisecond)
	}

	requireBucketsEqual(t, env.redisBuckets(t, eventID, tt), env.pgBuckets(t, eventID))
	if d := env.InventoryDrift(t, eventID, tt); d != 0 {
		t.Fatalf("lech %d: hai instance da sua gap doi (qua tay) hoac khong sua", d)
	}
}

func TestReconciler_ScriptKhongDeTonKhoXuongAm(t *testing.T) {
	env := setupEnv(t)
	eventID, tt := env.SeedEvent(t, 100)
	ctx := context.Background()

	before := env.redisBuckets(t, eventID, tt)[0]
	got, err := env.gate.AdjustInventory(ctx, eventID, tt, 0, -(before + 50))
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 || env.redisBuckets(t, eventID, tt)[0] != 0 {
		t.Fatalf("ton kho = %d sau khi tru qua tay, muon 0", got)
	}

	got, err = env.gate.AdjustInventory(ctx, eventID, tt, 0, 4)
	if err != nil || got != 4 {
		t.Fatalf("cong lai: got=%d err=%v", got, err)
	}
}

func TestReconciler_MotChuKyVoiLuotXacNhan_SuaNgayKhongCanChuKySau(t *testing.T) {
	env := setupEnv(t)
	eventID, tt := env.SeedEvent(t, 100)
	cfg := worker.ReconcilerConfig{
		EventIDs: []string{eventID}, Interval: 5 * time.Second,
		LockTTL: 3 * time.Second, ConfirmDelay: 100 * time.Millisecond,
	}
	rec := worker.NewInventoryReconciler(env.repo, env.repo, env.gate, cfg, env.Logger)

	if err := env.admin.HIncrBy(context.Background(), env.invKey(eventID, tt), "5", -4).Err(); err != nil {
		t.Fatal(err)
	}

	if err := rec.Reconcile(context.Background(), eventID); err != nil {
		t.Fatal(err)
	}

	requireBucketsEqual(t, env.redisBuckets(t, eventID, tt), env.pgBuckets(t, eventID))
}
