package worker_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/port"
	"github.com/eventflow/eventflow/services/ticketing/internal/worker"
)

// Test sweeper viet tu nghiep vu: hold het 10:00 phai duoc tra kho (BR-O2), moi
// hold chi duoc tra dung mot lan (BR-O7), Postgres la nguon su that (BR-O1).

type nhatKy struct {
	mu  sync.Mutex
	ops []string
}

func (n *nhatKy) ghi(s string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.ops = append(n.ops, s)
}

func (n *nhatKy) chup() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.ops...)
}

type sweepGate struct {
	mu         sync.Mutex
	log        *nhatKy
	expired    map[string][]string // tra ve mot lan roi het
	expiredErr map[string]error
	scans      map[string]int
	batch      []int
}

func (g *sweepGate) Hold(context.Context, port.HoldRequest) (port.HoldResult, error) {
	return port.HoldResult{}, errors.New("khong dung")
}

func (g *sweepGate) Release(_ context.Context, eventID, holdID string) error {
	g.log.ghi("redis:" + eventID + ":" + holdID)
	return nil
}

func (g *sweepGate) Consume(context.Context, string, string) error { return nil }

func (g *sweepGate) IsAdmitted(context.Context, string, string) (bool, error) { return true, nil }

func (g *sweepGate) ExpiredHolds(_ context.Context, eventID string, limit int) ([]string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.scans[eventID]++
	g.batch = append(g.batch, limit)
	if err := g.expiredErr[eventID]; err != nil {
		return nil, err
	}
	ids := g.expired[eventID]
	delete(g.expired, eventID)
	return ids, nil
}

func (g *sweepGate) soLanQuet(eventID string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.scans[eventID]
}

type ketQuaRelease struct {
	released bool
	err      error
}

type sweepRepo struct {
	log *nhatKy
	res map[string]ketQuaRelease
}

func (r *sweepRepo) GetSaleConfig(context.Context, string, string) (port.SaleConfig, error) {
	return port.SaleConfig{}, nil
}
func (r *sweepRepo) CountPurchased(context.Context, string, string) (int, error) { return 0, nil }
func (r *sweepRepo) GetActiveHold(context.Context, string, string) (port.HoldView, error) {
	return port.HoldView{}, nil
}
func (r *sweepRepo) PersistHold(context.Context, port.PersistHoldParams) (port.HoldView, error) {
	return port.HoldView{}, nil
}
func (r *sweepRepo) ReleaseHold(_ context.Context, holdID string) (bool, error) {
	r.log.ghi("pg:" + holdID)
	kq := r.res[holdID]
	return kq.released, kq.err
}

type sweepPub struct {
	mu   sync.Mutex
	msgs []map[string]any
	keys []string
}

func (p *sweepPub) Publish(_ context.Context, key string, payload any) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.keys = append(p.keys, key)
	m, _ := payload.(map[string]any)
	p.msgs = append(p.msgs, m)
	return nil
}

func (p *sweepPub) daPhat() ([]string, []map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.keys...), append([]map[string]any(nil), p.msgs...)
}

// chayToiKhi chay sweeper cho toi khi dk() dung (hoac het 2s) roi dung lai.
func chayToiKhi(t *testing.T, s *worker.HoldSweeper, dk func() bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for !dk() {
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatal("sweeper khong dat dieu kien mong doi trong 2s")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("dung sweeper phai tra context.Canceled, nhan %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sweeper khong dung khi context bi huy")
	}
}

func dungSweeper(gate *sweepGate, repo *sweepRepo, pub *sweepPub, events []string) *worker.HoldSweeper {
	cfg := worker.DefaultSweeperConfig(events)
	cfg.Interval = time.Millisecond
	return worker.NewHoldSweeper(gate, repo, pub, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func moiSweeper(expired map[string][]string, res map[string]ketQuaRelease) (*sweepGate, *sweepRepo, *sweepPub, *nhatKy) {
	nk := &nhatKy{}
	gate := &sweepGate{log: nk, expired: expired, expiredErr: map[string]error{}, scans: map[string]int{}}
	return gate, &sweepRepo{log: nk, res: res}, &sweepPub{}, nk
}

// Cau hinh mac dinh: quet moi giay (khach da doi du 10 phut, khong bat doi them)
// theo lo de mot dot het han lon khong lam nghen.
func TestDefaultSweeperConfig_QuetMoiGiayTheoLo(t *testing.T) {
	cfg := worker.DefaultSweeperConfig([]string{"ev-1"})
	if cfg.Interval != time.Second {
		t.Errorf("chu ky quet phai la 1 giay, nhan %v", cfg.Interval)
	}
	if cfg.Batch <= 0 {
		t.Errorf("kich thuoc lo phai > 0, nhan %d", cfg.Batch)
	}
	if len(cfg.EventIDs) != 1 || cfg.EventIDs[0] != "ev-1" {
		t.Errorf("phai quet dung danh sach su kien, nhan %v", cfg.EventIDs)
	}
}

// BR-O7: hold het han duoc tra kho va bao ra ngoai DUNG MOT LAN. Hold ma Postgres
// noi da xu ly truoc do (released=false) thi khong duoc bao "da tra" lan nua.
func TestHoldSweeper_BRO7_MoiHoldHetHanChiTraKhoDungMotLan(t *testing.T) {
	gate, repo, pub, nk := moiSweeper(
		map[string][]string{"ev-1": {"h1", "h2"}},
		map[string]ketQuaRelease{"h1": {released: true}, "h2": {released: false}},
	)
	s := dungSweeper(gate, repo, pub, []string{"ev-1"})

	// Cho them vai vong quet de chac chan khong tra lap o vong sau.
	chayToiKhi(t, s, func() bool { return gate.soLanQuet("ev-1") >= 3 })

	keys, msgs := pub.daPhat()
	if len(keys) != 1 {
		t.Fatalf("chi hold thuc su duoc tra (h1) moi duoc bao, dung 1 lan; nhan %d: %v", len(keys), msgs)
	}
	if keys[0] != "ticketing.hold.released" {
		t.Errorf("routing key sai: %s", keys[0])
	}
	if msgs[0]["hold_id"] != "h1" || msgs[0]["event_id"] != "ev-1" {
		t.Errorf("su kien phai mang dung hold/su kien, nhan %v", msgs[0])
	}

	pgCount := 0
	for _, op := range nk.chup() {
		if op == "pg:h1" {
			pgCount++
		}
	}
	if pgCount != 1 {
		t.Errorf("h1 chi duoc tra o Postgres mot lan, nhan %d", pgCount)
	}
	for _, b := range gate.batch {
		if b != worker.DefaultSweeperConfig(nil).Batch {
			t.Errorf("phai quet theo kich thuoc lo cau hinh, nhan %d", b)
		}
	}
}

// BR-O1: Postgres la nguon su that. Tra kho phai theo thu tu Postgres truoc,
// Redis sau; Postgres chua tra duoc thi KHONG duoc cong kho Redis (neu khong Redis
// se phong len trong khi Postgres van coi la dang giu). Mot hold loi khong duoc
// chan cac hold khac -- ngung tra dong nghia ve bi khoa vinh vien.
func TestHoldSweeper_BRO1_PostgresTruocRedisSau(t *testing.T) {
	gate, repo, pub, nk := moiSweeper(
		map[string][]string{"ev-1": {"h-loi", "h-ok"}},
		map[string]ketQuaRelease{
			"h-loi": {err: errors.New("postgres: deadlock")},
			"h-ok":  {released: true},
		},
	)
	s := dungSweeper(gate, repo, pub, []string{"ev-1"})
	chayToiKhi(t, s, func() bool { return gate.soLanQuet("ev-1") >= 2 })

	ops := nk.chup()
	viTri := map[string]int{}
	for i, op := range ops {
		if _, ok := viTri[op]; !ok {
			viTri[op] = i
		}
	}
	if _, ok := viTri["redis:ev-1:h-loi"]; ok {
		t.Errorf("Postgres chua tra h-loi thi khong duoc cong kho Redis; thao tac: %v", ops)
	}
	pg, okPg := viTri["pg:h-ok"]
	rd, okRd := viTri["redis:ev-1:h-ok"]
	if !okPg || !okRd {
		t.Fatalf("hold loi khong duoc chan hold khac; h-ok phai duoc tra ca hai tang, thao tac: %v", ops)
	}
	if pg > rd {
		t.Errorf("phai tra Postgres truoc Redis; thao tac: %v", ops)
	}

	keys, msgs := pub.daPhat()
	if len(keys) != 1 || msgs[0]["hold_id"] != "h-ok" {
		t.Errorf("chi h-ok duoc bao da tra, nhan %v", msgs)
	}
}

// Mot su kien quet loi khong duoc lam chet sweeper hay chan su kien khac.
func TestHoldSweeper_LoiMotSuKienKhongChanSuKienKhac(t *testing.T) {
	gate, repo, pub, _ := moiSweeper(
		map[string][]string{"ev-ok": {"h1"}},
		map[string]ketQuaRelease{"h1": {released: true}},
	)
	gate.expiredErr["ev-loi"] = errors.New("redis: timeout")
	s := dungSweeper(gate, repo, pub, []string{"ev-loi", "ev-ok"})

	chayToiKhi(t, s, func() bool {
		keys, _ := pub.daPhat()
		return gate.soLanQuet("ev-loi") >= 3 && len(keys) == 1
	})

	_, msgs := pub.daPhat()
	if msgs[0]["event_id"] != "ev-ok" || msgs[0]["hold_id"] != "h1" {
		t.Errorf("su kien con lai van phai duoc tra kho, nhan %v", msgs)
	}
}
