//go:build integration

// Package concurrency chua cong chat luong quan trong nhat cua ca he thong.
//
// EVF-39. Neu test nay fail, khong duoc merge va khong duoc release -- bat ke
// deadline. Ban qua ve la loi khong the sua bang hotfix: ve da ban, khach da
// den san, va cho ngoi thi khong co.
package concurrency

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
)

const (
	quota       = 100    // so ve that su co
	concurrency = 10_000 // so nguoi cung bam mua trong cung mot thoi diem
)

// TestNoOversell_UnderExtremeConcurrency mo phong dung giay 20:00:00.
//
// 10.000 goroutine cung lao vao mua 100 ve. Ket qua bat buoc:
//   - DUNG 100 don thanh cong  -- khong hon (oversell), khong kem (under-sell)
//   - Ton kho cuoi cung = 0
//   - Moi nguoi that bai deu nhan ErrSoldOut, khong phai loi he thong
//
// Under-sell cung la loi: no nghia la co ve bi khoa mat trong mot trang thai
// nua voi nao do, va ban to chuc mat doanh thu that.
func TestNoOversell_UnderExtremeConcurrency(t *testing.T) {
	env := setupEnv(t) // testcontainers: Postgres + Redis that
	ctx := context.Background()

	eventID, ticketTypeID := env.SeedEvent(t, quota)

	uc := app.NewCreateHold(env.Gate, env.Repo, env.Logger)

	var (
		succeeded atomic.Int64
		soldOut   atomic.Int64
		other     atomic.Int64
		start     = make(chan struct{})
		wg        sync.WaitGroup
	)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()

			identityID := env.SeedIdentity(t, n)
			token := env.AdmitToken(t, eventID, identityID)

			<-start // tat ca xuat phat cung luc -- day moi la phep thu that

			_, err := uc.Execute(ctx, app.CreateHoldInput{
				EventID:      eventID,
				TicketTypeID: ticketTypeID,
				IdentityID:   identityID,
				QueueToken:   token,
				Quantity:     1,
			})
			switch {
			case err == nil:
				succeeded.Add(1)
			case errors.Is(err, domain.ErrSoldOut):
				soldOut.Add(1)
			default:
				other.Add(1)
				t.Logf("loi khong mong doi: %v", err)
			}
		}(i)
	}

	close(start)
	wg.Wait()

	// ---- Bat bien 1: dung so ve, khong hon khong kem ----
	if got := succeeded.Load(); got != quota {
		t.Fatalf("BAT BIEN BI VI PHAM: ban duoc %d ve, ky vong dung %d "+
			"(%d bao het ve, %d loi khac)", got, quota, soldOut.Load(), other.Load())
	}

	// ---- Bat bien 2: ton kho trong Postgres ve dung 0 ----
	if avail := env.TotalAvailable(t, ticketTypeID); avail != 0 {
		t.Fatalf("ton kho con lai %d, ky vong 0", avail)
	}

	// ---- Bat bien 3: Redis va Postgres khong lech ----
	if drift := env.InventoryDrift(t, eventID, ticketTypeID); drift != 0 {
		t.Fatalf("Redis lech Postgres %d ve", drift)
	}

	// ---- Bat bien 4: moi nguoi that bai deu nhan cau tra loi dung ----
	if other.Load() > 0 {
		t.Fatalf("%d request that bai vi loi he thong thay vi bao het ve", other.Load())
	}
}

// TestNoOversell_WithConcurrentExpiry kiem tra tinh huong kho nhat: sweeper dang
// tra kho trong khi nguoi khac dang mua.
//
// Day la noi de sinh bug oversell nhat -- mot hold bi tra kho hai lan se lam
// ton kho phong len va ban vuot quota. Chay sweeper hai instance song song
// chinh la de bat loi do.
func TestNoOversell_WithConcurrentExpiry(t *testing.T) {
	env := setupEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	eventID, ticketTypeID := env.SeedEvent(t, quota)
	env.SetHoldTTL(t, eventID, 2*time.Second) // het han that nhanh de ep tinh huong

	// Hai sweeper chay song song -- mo phong luc K8s co 2 pod, hoac luc dang
	// rolling update. Viec tra kho phai idempotent tuyet doi.
	for i := 0; i < 2; i++ {
		go env.RunSweeper(ctx, eventID)
	}

	uc := app.NewCreateHold(env.Gate, env.Repo, env.Logger)

	var wg sync.WaitGroup
	for round := 0; round < 5; round++ {
		for i := 0; i < 2000; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				identityID := env.SeedIdentity(t, n)
				token := env.AdmitToken(t, eventID, identityID)
				_, _ = uc.Execute(ctx, app.CreateHoldInput{
					EventID:      eventID,
					TicketTypeID: ticketTypeID,
					IdentityID:   identityID,
					QueueToken:   token,
					Quantity:     1,
				})
			}(round*2000 + i)
		}
		wg.Wait()
		time.Sleep(3 * time.Second) // cho hold het han va sweeper tra kho
	}

	// Sau moi vong giu-va-nha, tong ton kho phai tro ve dung quota ban dau.
	// Ton kho lon hon quota nghia la co cho nao do da tra kho hai lan.
	total := env.TotalAvailable(t, ticketTypeID) + env.TotalHeldOrSold(t, ticketTypeID)
	if total != quota {
		t.Fatalf("KHO BI PHONG: tong = %d, ky vong %d "+
			"(dau hieu tra kho hai lan -- xem BR-O7)", total, quota)
	}
}
