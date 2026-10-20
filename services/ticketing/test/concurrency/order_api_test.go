//go:build integration

package concurrency

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// EV-NEW-2 -- API don hang tren Postgres + Redis THAT.

// holdFor giu qty ve cho khach thu n bang use case that (Redis + Postgres) va tra ve view.
func (e *TestEnv) holdFor(t *testing.T, eventID, ticketTypeID string, n, qty int) (identityID string, hold port.HoldView) {
	t.Helper()
	identityID = e.SeedIdentity(t, n)
	out, err := app.NewCreateHold(e.Gate, e.Repo, e.Logger).Execute(context.Background(), app.CreateHoldInput{
		EventID: eventID, TicketTypeID: ticketTypeID, IdentityID: identityID,
		QueueToken: e.AdmitToken(t, eventID, identityID), Quantity: qty,
	})
	if err != nil {
		t.Fatalf("tao hold: %v", err)
	}
	return identityID, out
}

func (e *TestEnv) redisHolds(t *testing.T, eventID string) int {
	t.Helper()
	n, err := e.admin.HLen(context.Background(), "hold:{"+eventID+"}:h").Result()
	if err != nil {
		t.Fatal(err)
	}
	return int(n)
}

func TestOrderAPI_GetOrderChiChoChuDon(t *testing.T) {
	env := setupEnv(t)
	eventID, ticketTypeID := env.SeedEvent(t, quota)
	owner, hold := env.holdFor(t, eventID, ticketTypeID, 9301, 2)
	ctx := context.Background()

	o, err := env.repo.GetOrder(ctx, hold.OrderID, owner)
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != "HELD" || o.EventID != eventID || o.TotalCents != 100_000_000 || o.HoldID == "" {
		t.Fatalf("don = %+v", o)
	}
	if len(o.Items) != 1 || o.Items[0].Quantity != 2 || o.Items[0].TierName != "Standard" || o.EventTitle != "Oversell gate" {
		t.Fatalf("dong don = %+v", o.Items)
	}

	// Nguoi khac, id khong ton tai, id rac: cung mot ket qua, khong lo don co ton tai.
	other := env.SeedIdentity(t, 9302)
	for name, args := range map[string][2]string{
		"nguoi khac":    {hold.OrderID, other},
		"don la":        {"11111111-2222-4333-8444-555555555555", owner},
		"id khong uuid": {"khong-phai-uuid", owner},
	} {
		if _, err := env.repo.GetOrder(ctx, args[0], args[1]); !errors.Is(err, domain.ErrOrderNotFound) {
			t.Fatalf("%s: muon ErrOrderNotFound, nhan %v", name, err)
		}
	}
	if ok, _ := env.repo.OrderOwnedBy(ctx, hold.OrderID, owner); !ok {
		t.Fatal("chu don phai duoc nhan ra")
	}
	if ok, _ := env.repo.OrderOwnedBy(ctx, hold.OrderID, other); ok {
		t.Fatal("nguoi khac khong duoc nhan la chu don")
	}
}

func TestOrderAPI_GetOrderThanhToanVaVe(t *testing.T) {
	env := setupEnv(t)
	eventID, ticketTypeID := env.SeedEvent(t, quota)
	owner, hold := env.holdFor(t, eventID, ticketTypeID, 9303, 1)
	ctx := context.Background()

	if _, err := env.pool.Exec(ctx, `
		INSERT INTO payments (order_id, provider, provider_txn_id, amount_cents, status)
		VALUES ($1::uuid, 'vnpay', 'txn-'||$1, 50000000, 'FAILED')`, hold.OrderID); err != nil {
		t.Fatal(err)
	}
	if o, _ := env.repo.GetOrder(ctx, hold.OrderID, owner); !o.PaymentFailed || o.PaidAt != nil || o.PaymentProvider != "vnpay" {
		t.Fatalf("thanh toan that bai: %+v", o)
	}

	if _, err := env.pool.Exec(ctx, `
		INSERT INTO payments (order_id, provider, provider_txn_id, amount_cents, status, created_at)
		VALUES ($1::uuid, 'vnpay', 'ok-'||$1, 50000000, 'SUCCEEDED', now() + interval '1 second')`, hold.OrderID); err != nil {
		t.Fatal(err)
	}
	o, _ := env.repo.GetOrder(ctx, hold.OrderID, owner)
	if o.PaidAt == nil || o.PaymentFailed {
		t.Fatalf("sau khi thanh toan thanh cong: %+v", o)
	}
}

func TestOrderAPI_HuyHoldTraKhoDungMotLan(t *testing.T) {
	env := setupEnv(t)
	eventID, ticketTypeID := env.SeedEvent(t, quota)
	owner, hold := env.holdFor(t, eventID, ticketTypeID, 9304, 3)
	ctx := context.Background()
	cancel := app.NewCancelHold(env.repo, env.gate, env.Logger)

	if got := env.TotalAvailable(t, ticketTypeID); got != quota-3 {
		t.Fatalf("truoc khi huy con %d, muon %d", got, quota-3)
	}

	if err := cancel.Execute(ctx, hold.HoldID, owner); err != nil {
		t.Fatal(err)
	}
	if got := env.TotalAvailable(t, ticketTypeID); got != quota {
		t.Fatalf("sau khi huy con %d, muon %d", got, quota)
	}
	if d := env.InventoryDrift(t, eventID, ticketTypeID); d != 0 {
		t.Fatalf("Redis lech Postgres %d sau khi huy", d)
	}
	if env.redisHolds(t, eventID) != 0 {
		t.Fatal("hold con trong Redis sau khi huy")
	}
	o, _ := env.repo.GetOrder(ctx, hold.OrderID, owner)
	if o.Status != "CANCELLED" || o.HoldID != "" {
		t.Fatalf("don sau khi huy: %+v", o)
	}

	// Huy lai: idempotent, khong tra kho lan hai (neu tra hai lan, inv_available_le_capacity se no).
	for range 3 {
		if err := cancel.Execute(ctx, hold.HoldID, owner); err != nil {
			t.Fatalf("huy lai phai thanh cong: %v", err)
		}
	}
	if got := env.TotalAvailable(t, ticketTypeID); got != quota {
		t.Fatalf("huy lai lam kho thanh %d, muon %d", got, quota)
	}

	// Khach giu lai duoc ngay (BR-O3 da duoc giai phong, han muc tra lai).
	_, again := env.holdFor(t, eventID, ticketTypeID, 9304, 1)
	if again.HoldID == hold.HoldID {
		t.Fatal("hold moi phai co id moi")
	}
}

func TestOrderAPI_HuyHoldDongThoiChiTraKhoMotLan(t *testing.T) {
	env := setupEnv(t)
	eventID, ticketTypeID := env.SeedEvent(t, quota)
	owner, hold := env.holdFor(t, eventID, ticketTypeID, 9305, 3)
	ctx := context.Background()

	const n = 20
	var (
		wg        sync.WaitGroup
		cancelled atomic.Int32
		failed    atomic.Int32
	)
	start := make(chan struct{})
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			res, err := env.repo.CancelHold(ctx, hold.HoldID, owner)
			switch {
			case err != nil:
				failed.Add(1)
			case res.Cancelled:
				cancelled.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if cancelled.Load() != 1 || failed.Load() != 0 {
		t.Fatalf("cancelled=%d failed=%d, muon 1 va 0", cancelled.Load(), failed.Load())
	}
	if got := env.TotalAvailable(t, ticketTypeID); got != quota {
		t.Fatalf("kho = %d, muon %d (tra dung mot lan)", got, quota)
	}
}

// Huy dua voi sweeper het han: chi mot ben tra kho.
func TestOrderAPI_HuyHoldDaHetHanKhongTraKhoLanHai(t *testing.T) {
	env := setupEnv(t)
	eventID, ticketTypeID := env.SeedEvent(t, quota)
	owner, hold := env.holdFor(t, eventID, ticketTypeID, 9306, 2)
	ctx := context.Background()

	if released, err := env.repo.ReleaseHold(ctx, hold.HoldID); err != nil || !released { // sweeper
		t.Fatalf("sweeper tra kho: %v %v", released, err)
	}
	res, err := env.repo.CancelHold(ctx, hold.HoldID, owner)
	if err != nil || res.Cancelled {
		t.Fatalf("huy hold da het han: res=%+v err=%v, muon thanh cong va Cancelled=false", res, err)
	}
	if got := env.TotalAvailable(t, ticketTypeID); got != quota-2+2 {
		t.Fatalf("kho = %d, muon %d", got, quota)
	}
	if o, _ := env.repo.GetOrder(ctx, hold.OrderID, owner); o.Status != "EXPIRED" {
		t.Fatalf("don het han khong duoc doi thanh %s", o.Status)
	}
}

func TestOrderAPI_KhongHuyDuocHoldCuaNguoiKhacHoacDonDaThanhToan(t *testing.T) {
	env := setupEnv(t)
	eventID, ticketTypeID := env.SeedEvent(t, quota)
	owner, hold := env.holdFor(t, eventID, ticketTypeID, 9307, 1)
	stranger := env.SeedIdentity(t, 9308)
	ctx := context.Background()

	if _, err := env.repo.CancelHold(ctx, hold.HoldID, stranger); !errors.Is(err, domain.ErrOrderNotFound) {
		t.Fatalf("huy hold cua nguoi khac: muon ErrOrderNotFound, nhan %v", err)
	}
	if _, err := env.repo.CancelHold(ctx, "khong-phai-uuid", owner); !errors.Is(err, domain.ErrOrderNotFound) {
		t.Fatalf("id rac: muon ErrOrderNotFound, nhan %v", err)
	}
	if got := env.TotalAvailable(t, ticketTypeID); got != quota-1 {
		t.Fatalf("huy bi tu choi nhung kho doi: %d", got)
	}

	for _, status := range []string{"PAYMENT_PENDING", "PAID"} {
		if _, err := env.pool.Exec(ctx, `UPDATE orders SET status = $2 WHERE id = $1::uuid`, hold.OrderID, status); err != nil {
			t.Fatal(err)
		}
		if _, err := env.repo.CancelHold(ctx, hold.HoldID, owner); !errors.Is(err, domain.ErrHoldNotCancellable) {
			t.Fatalf("%s: muon ErrHoldNotCancellable, nhan %v", status, err)
		}
	}
	if got := env.TotalAvailable(t, ticketTypeID); got != quota-1 {
		t.Fatalf("don dang thanh toan bi tra kho: %d", got)
	}
}

func TestOrderAPI_TonKhoCongKhai(t *testing.T) {
	env := setupEnv(t)
	eventID, ticketTypeID := env.SeedEvent(t, 100)
	ctx := context.Background()
	uc := app.NewGetAvailability(compositeAvail{env})

	level := func() domain.AvailabilityLevel {
		t.Helper()
		got, err := uc.Execute(ctx, eventID)
		if err != nil || len(got) != 1 || got[0].TicketTypeID != ticketTypeID {
			t.Fatalf("got=%+v err=%v", got, err)
		}
		return got[0].Availability
	}

	if l := level(); l != domain.AvailabilityAvailable {
		t.Fatalf("moi mo ban: %s", l)
	}
	// 30 khach giu 1 ve: con 70/100, van AVAILABLE. Hold that di qua Redis nen kho giam that.
	for n := 9400; n < 9400+30; n++ {
		env.holdFor(t, eventID, ticketTypeID, n, 1)
	}
	if l := level(); l != domain.AvailabilityAvailable {
		t.Fatalf("con 70/100: %s", l)
	}

	// Cac muc con lai dat truc tiep so ve trong Redis. Khong giu het bang hold that: hold.lua chi
	// quet toi da 3 bucket nen vao luc cuoi co the bao "het" khi bucket khac con ve -- ket qua se
	// khong tat dinh.
	key := "inv:{" + eventID + "}:tt:" + ticketTypeID
	setStock := func(total int) {
		t.Helper()
		pipe := env.admin.Pipeline()
		for b := range bucketCount {
			v := 0
			if b == 0 {
				v = total
			}
			pipe.HSet(ctx, key, b, v)
		}
		if _, err := pipe.Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}
	setStock(9)
	if l := level(); l != domain.AvailabilityFewLeft {
		t.Fatalf("con 9/100: %s", l)
	}
	setStock(0)
	if l := level(); l != domain.AvailabilitySoldOut {
		t.Fatalf("het ve: %s", l)
	}
	if err := env.admin.Del(ctx, key).Err(); err != nil {
		t.Fatal(err)
	}
	if l := level(); l != domain.AvailabilitySoldOut {
		t.Fatalf("chua nap Redis phai la het (tin phia an toan): %s", l)
	}

	if _, err := uc.Execute(ctx, newUUID(t)); !errors.Is(err, domain.ErrEventNotFound) {
		t.Fatalf("su kien la: muon ErrEventNotFound, nhan %v", err)
	}
}

type compositeAvail struct{ env *TestEnv }

func (c compositeAvail) ListTiers(ctx context.Context, eventID string) ([]port.TierInfo, error) {
	return c.env.repo.ListTiers(ctx, eventID)
}

func (c compositeAvail) Remaining(ctx context.Context, eventID string, ids []string) (map[string]int, error) {
	return c.env.gate.Remaining(ctx, eventID, ids)
}
