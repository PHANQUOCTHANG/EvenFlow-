//go:build integration

// Package soldout kiem EV-146 voi Redis that: cac script Lua join / status / admit phan ung voi co
// meta.sold_out va meta.stock_zero do ticketing ghi.
package soldout

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/leader"
	"github.com/eventflow/eventflow/libs/go/testx"
	redisadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/redis"
	"github.com/eventflow/eventflow/services/waitingroom/internal/domain"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testx.Shutdown()
	os.Exit(code)
}

type env struct {
	q   *redisadapter.Queue
	rdb *redis.Client
	ev  string
	ctx context.Context
}

func setup(t *testing.T) *env {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: testx.Redis(t)})
	t.Cleanup(func() { _ = rdb.Close() })
	q, err := redisadapter.NewQueue(context.Background(), rdb, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return &env{q: q, rdb: rdb, ev: "soldout-" + t.Name(), ctx: context.Background()}
}

func (e *env) meta(field, value string) {
	e.rdb.HSet(e.ctx, "wr:{"+e.ev+"}:meta", field, value)
}

// afterT0 dua su kien qua T0 de join noi duoi FIFO (QUEUED co rank).
func (e *env) afterT0() { e.meta("sale_start_ms", "1") }

func (e *env) join(t *testing.T, identity, token string) redisadapter.JoinResult {
	t.Helper()
	r, err := e.q.Join(e.ctx, e.ev, identity, token)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (e *env) status(t *testing.T, token string) domain.State {
	t.Helper()
	p, err := e.q.Status(e.ctx, e.ev, token)
	if err != nil {
		t.Fatal(err)
	}
	return p.State
}

func (e *env) fencedAdmit(t *testing.T, batch int) []string {
	t.Helper()
	// Nhiem ky leader that: dat epoch roi admit bang chinh script production.
	if err := e.rdb.Set(e.ctx, "wr:{"+e.ev+"}:leader_epoch", 1, 0).Err(); err != nil {
		t.Fatal(err)
	}
	got, err := e.q.Fenced(1).Admit(e.ctx, e.ev, batch, 15*time.Minute)
	if err != nil {
		t.Fatalf("admit: %v (leader.IsFenced=%v)", err, leader.IsFenced(err))
	}
	return got
}

func TestTichHop_SoldOut_StatusDoiSangSoldOutChoNguoiChuaVao(t *testing.T) {
	e := setup(t)
	e.afterT0()
	queued := e.join(t, "alice", "tok-alice")
	if queued.State != domain.StateQueued {
		t.Fatalf("join: %s", queued.State)
	}
	if got := e.status(t, "tok-alice"); got != domain.StateQueued {
		t.Fatalf("truoc khi het ve: %s", got)
	}

	e.meta("sold_out", "1")
	if got := e.status(t, "tok-alice"); got != domain.StateSoldOut {
		t.Fatalf("het ve han: %s, muon SOLD_OUT", got)
	}
	p, _ := e.q.Status(e.ctx, e.ev, "tok-alice")
	if p.PollAfterMs != 30_000 || p.Rank != -1 {
		t.Fatalf("poll=%d rank=%d", p.PollAfterMs, p.Rank)
	}

	// Kho co lai (hoan tien / hold het han): quay lai QUEUED, khong mat cho.
	e.rdb.HDel(e.ctx, "wr:{"+e.ev+"}:meta", "sold_out")
	if got := e.status(t, "tok-alice"); got != domain.StateQueued {
		t.Fatalf("sau khi co ve lai: %s, muon QUEUED", got)
	}
	// Token la van la UNKNOWN, khong bi bien thanh SOLD_OUT.
	e.meta("sold_out", "1")
	if got := e.status(t, "khong-co-trong-hang"); got != domain.StateUnknown {
		t.Fatalf("token la khi het ve: %s, muon UNKNOWN", got)
	}
}

func TestTichHop_SoldOut_LobbyCungNhanSoldOut(t *testing.T) {
	e := setup(t) // chua qua T0 -> LOBBY
	e.meta("sale_start_ms", "99999999999999")
	if r := e.join(t, "alice", "tok-alice"); r.State != domain.StateLobby {
		t.Fatalf("join: %s", r.State)
	}
	e.meta("sold_out", "1")
	if got := e.status(t, "tok-alice"); got != domain.StateSoldOut {
		t.Fatalf("lobby khi het ve: %s", got)
	}
}

func TestTichHop_SoldOut_NguoiDaAdmitVanHoanTatDon(t *testing.T) {
	e := setup(t)
	e.afterT0()
	e.join(t, "alice", "tok-alice")
	if got := e.fencedAdmit(t, 10); len(got) != 1 {
		t.Fatalf("admit: %v", got)
	}
	e.meta("sold_out", "1") // ve cuoi vua duoc giu: nguoi da vao van phai mua duoc

	if got := e.status(t, "tok-alice"); got != domain.StateAdmitted {
		t.Fatalf("nguoi da admit khi het ve: %s, muon ADMITTED", got)
	}
}

func TestTichHop_SoldOut_JoinMoiKhongGhiDanhNhungNguoiCuConCho(t *testing.T) {
	e := setup(t)
	e.afterT0()
	first := e.join(t, "alice", "tok-alice")
	e.meta("sold_out", "1")

	// Nguoi moi: SOLD_OUT, khong token, khong chiem cho trong hang.
	late := e.join(t, "bob", "tok-bob")
	if late.State != domain.StateSoldOut || late.Token != "" || late.IsNew {
		t.Fatalf("join luc het ve: %+v", late)
	}
	if n, _ := e.rdb.ZCard(e.ctx, "wr:{"+e.ev+"}:queue").Result(); n != 1 {
		t.Fatalf("hang co %d nguoi, muon 1 (bob khong duoc ghi danh)", n)
	}
	if has, _ := e.rdb.HExists(e.ctx, "wr:{"+e.ev+"}:tok", "bob").Result(); has {
		t.Fatal("bob bi ghi vao bang identity -> token")
	}
	// Alice join lai van la cho cu (BR-Q3).
	again := e.join(t, "alice", "tok-khac")
	if again.Token != first.Token || again.IsNew {
		t.Fatalf("alice join lai: %+v, muon cung token %s", again, first.Token)
	}
}

func TestTichHop_SoldOut_ControllerNgungThaKhiKhoBang0(t *testing.T) {
	for _, flag := range []string{"sold_out", "stock_zero"} {
		t.Run(flag, func(t *testing.T) {
			e := setup(t)
			e.afterT0()
			e.join(t, "alice", "tok-alice")
			e.join(t, "bob", "tok-bob")

			e.meta(flag, "1")
			if got := e.fencedAdmit(t, 10); len(got) != 0 {
				t.Fatalf("%s=1 nhung van admit %v", flag, got)
			}
			if n, _ := e.rdb.ZCard(e.ctx, "wr:{"+e.ev+"}:queue").Result(); n != 2 {
				t.Fatalf("hang con %d, muon 2 (khong ai bi lay ra)", n)
			}

			e.rdb.HDel(e.ctx, "wr:{"+e.ev+"}:meta", flag)
			if got := e.fencedAdmit(t, 10); len(got) != 2 {
				t.Fatalf("sau khi co ve lai admit %v, muon 2", got)
			}
		})
	}
}

// stock_zero (con hold co the tra lai) KHONG duoc bao SOLD_OUT cho khach: chi dung tha them nguoi.
func TestTichHop_SoldOut_StockZeroKhongDoiTrangThaiKhach(t *testing.T) {
	e := setup(t)
	e.afterT0()
	e.join(t, "alice", "tok-alice")
	e.meta("stock_zero", "1")
	if got := e.status(t, "tok-alice"); got != domain.StateQueued {
		t.Fatalf("stock_zero lam doi trang thai khach thanh %s", got)
	}
	if r := e.join(t, "bob", "tok-bob"); r.State != domain.StateQueued {
		t.Fatalf("stock_zero khong duoc chan ghi danh: %s", r.State)
	}
}
