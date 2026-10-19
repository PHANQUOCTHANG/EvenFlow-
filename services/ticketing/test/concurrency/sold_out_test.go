//go:build integration

package concurrency

import (
	"context"
	"strings"
	"testing"

	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
	"github.com/eventflow/eventflow/services/ticketing/internal/worker"
)

// EV-146 -- tin hieu het ve tren Postgres + Redis THAT.

func (e *TestEnv) soldOutWatcher(eventID string) *worker.SoldOutWatcher {
	cfg := worker.DefaultSoldOutWatcherConfig([]string{eventID})
	return worker.NewSoldOutWatcher(worker.NewTierSource(e.repo), e.gate, e.gate, cfg, e.Logger)
}

// flags doc hai co ticketing ghi cho phong cho.
func (e *TestEnv) flags(t *testing.T, eventID string) (soldOut, stockZero bool) {
	t.Helper()
	vals, err := e.admin.HMGet(context.Background(), "wr:{"+eventID+"}:meta", "sold_out", "stock_zero").Result()
	if err != nil {
		t.Fatal(err)
	}
	return vals[0] == "1", vals[1] == "1"
}

// setStock dat truc tiep ton kho Redis (bucket 0 = total, con lai 0): hold.lua chi quet toi da 3 bucket
// nen khong the giu het ve bang hold that mot cach tat dinh.
func (e *TestEnv) setStock(t *testing.T, eventID, ticketTypeID string, total int) {
	t.Helper()
	pipe := e.admin.Pipeline()
	for b := range bucketCount {
		v := 0
		if b == 0 {
			v = total
		}
		pipe.HSet(context.Background(), "inv:{"+eventID+"}:tt:"+ticketTypeID, b, v)
	}
	if _, err := pipe.Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// soldOutHold giu 1 ve cho khach thu n bang use case that (Redis + Postgres).
func (e *TestEnv) soldOutHold(t *testing.T, eventID, ticketTypeID string, n int) port.HoldView {
	t.Helper()
	id := e.SeedIdentity(t, n)
	out, err := app.NewCreateHold(e.Gate, e.Repo, e.Logger).Execute(context.Background(), app.CreateHoldInput{
		EventID: eventID, TicketTypeID: ticketTypeID, IdentityID: id,
		QueueToken: e.AdmitToken(t, eventID, id), Quantity: 1,
	})
	if err != nil {
		t.Fatalf("tao hold: %v", err)
	}
	return out
}

func (e *TestEnv) checkN(t *testing.T, w *worker.SoldOutWatcher, eventID string, n int) {
	t.Helper()
	for range n {
		if err := w.Check(context.Background(), eventID); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSoldOut_VongDoiTheoKhoVaHold(t *testing.T) {
	env := setupEnv(t)
	eventID, ticketTypeID := env.SeedEvent(t, quota)
	ctx := context.Background()
	w := env.soldOutWatcher(eventID)

	// 1. Moi nap kho, con ve: khong co co nao.
	env.checkN(t, w, eventID, 4)
	if so, sz := env.flags(t, eventID); so || sz {
		t.Fatalf("con ve ma co co sold_out=%v stock_zero=%v", so, sz)
	}

	// 2. Mot khach dang giu ve (hold that), kho Redis ve 0: tam het -- ngung tha nguoi, chua bao het han.
	hold := env.soldOutHold(t, eventID, ticketTypeID, 9501)
	env.setStock(t, eventID, ticketTypeID, 0)
	env.checkN(t, w, eventID, 6)
	if so, sz := env.flags(t, eventID); so || !sz {
		t.Fatalf("kho 0 + con hold: sold_out=%v stock_zero=%v, muon false/true", so, sz)
	}

	// 3. Hold het han tra kho (Release that): kho > 0 nen go het co.
	if err := env.gate.Release(ctx, eventID, strings.ReplaceAll(hold.HoldID, "-", "")); err != nil {
		t.Fatal(err)
	}
	env.checkN(t, w, eventID, 1)
	if so, sz := env.flags(t, eventID); so || sz {
		t.Fatalf("hold tra kho: sold_out=%v stock_zero=%v, muon cung false", so, sz)
	}

	// 4. Kho 0 va khong con hold: sau du so lan quan sat lien tiep moi het han.
	env.setStock(t, eventID, ticketTypeID, 0)
	env.checkN(t, w, eventID, 2)
	if so, _ := env.flags(t, eventID); so {
		t.Fatal("moi 2 lan quan sat da bao het han")
	}
	env.checkN(t, w, eventID, 1)
	if so, sz := env.flags(t, eventID); !so || !sz {
		t.Fatalf("sold_out=%v stock_zero=%v, muon cung true", so, sz)
	}

	// 5. Hoan tien tra ve 1 ve: tu go.
	env.setStock(t, eventID, ticketTypeID, 1)
	env.checkN(t, w, eventID, 1)
	if so, sz := env.flags(t, eventID); so || sz {
		t.Fatalf("co ve lai: sold_out=%v stock_zero=%v", so, sz)
	}
}

// Redis chua nap kho (hoac bi xoa) khong duoc bao thanh het ve.
func TestSoldOut_ChuaNapKhoKhongBaoHetVe(t *testing.T) {
	env := setupEnv(t)
	eventID, ticketTypeID := env.SeedEvent(t, quota)
	if err := env.admin.Del(context.Background(), "inv:{"+eventID+"}:tt:"+ticketTypeID).Err(); err != nil {
		t.Fatal(err)
	}
	w := env.soldOutWatcher(eventID)
	env.checkN(t, w, eventID, 5)
	if so, sz := env.flags(t, eventID); so || sz {
		t.Fatalf("kho chua nap bi coi la het ve: sold_out=%v stock_zero=%v", so, sz)
	}
}

// Ve cuoi het do hold that thanh cong cuoi cung (khong chi do ghi truc tiep Redis): hold tra ErrSoldOut
// khong du bang chung, nhung tong kho 0 thi co.
func TestSoldOut_HoldThatLamHetKho(t *testing.T) {
	env := setupEnv(t)
	eventID, ticketTypeID := env.SeedEvent(t, quota)
	env.setStock(t, eventID, ticketTypeID, 1) // dung 1 ve o bucket 0

	// Giu ve cuoi bang use case that, thu den khi trung bucket 0 (script quet toi da 3 bucket tu vi tri ngau nhien).
	var held bool
	for n := 9600; n < 9600+400 && !held; n++ {
		id := env.SeedIdentity(t, n)
		_, err := app.NewCreateHold(env.Gate, env.Repo, env.Logger).Execute(context.Background(), app.CreateHoldInput{
			EventID: eventID, TicketTypeID: ticketTypeID, IdentityID: id,
			QueueToken: env.AdmitToken(t, eventID, id), Quantity: 1,
		})
		held = err == nil
	}
	if !held {
		t.Skip("khong giu duoc ve cuoi trong 400 lan thu (hold.lua chi quet 3 bucket); khong phai loi cua watcher")
	}

	w := env.soldOutWatcher(eventID)
	env.checkN(t, w, eventID, 5)
	// Co 1 hold dang song -> chi tam het.
	if so, sz := env.flags(t, eventID); so || !sz {
		t.Fatalf("sold_out=%v stock_zero=%v, muon false/true", so, sz)
	}
}
