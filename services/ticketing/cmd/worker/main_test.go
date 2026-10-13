package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventflow/eventflow/libs/go/outbox"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
	"github.com/eventflow/eventflow/services/ticketing/internal/worker"
)

// Test cach worker noi relay vao (spec AC-7; plan §6.3 B1, §7 r6, r8; review M5).
//
// SEAM gia dinh trong package main (tach tu run() de test duoc):
//
//	// config la cau hinh doc tu bien moi truong.
//	type config struct {
//		AMQPURL string // bat buoc; sai cu phap (amqp091.ParseURI) -> loi; loi KHONG chua chuoi URL/mat khau
//		// ... DATABASE_URL, REDIS_ADDR, METRICS_ADDR, ACTIVE_EVENT_IDS tuy implementer
//	}
//	func loadConfig(getenv func(string) string) (config, error)
//
//	// amqpDialer tra outbox.Dialer that: amqp091.Dial(url), kenh topology + mq.Declare(mq.TicketingTopology()),
//	// kenh publish + mq.NewPublisher; Closer la *amqp091.Connection. Khong dial luc tao.
//	func amqpDialer(url string) outbox.Dialer
//
//	// relayConfig tra cau hinh relay cua ticketing: Exchange "ticketing.events",
//	// DropUnroutable = {"ticketing.hold.created"} (KHONG co "ticketing.hold.released"), Registerer = reg.
//	func relayConfig(reg prometheus.Registerer) outbox.RelayConfig
//
//	// newPoolConfig parse DATABASE_URL va dat DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe (an toan voi
//	// PgBouncer transaction mode).
//	func newPoolConfig(dsn string) (*pgxpool.Config, error)
//
//	// runLoops chay moi vong lap song song; vong lap dau tien tra ve (loi hoac nil) lam huy ctx chung, cho TAT CA
//	// dung roi tra loi dau tien (context.Canceled/nil khi do ctx cha bi huy). Relay khong bao gio tra ve vi
//	// broker sap nen khong keo sweeper dung theo.
//	func runLoops(ctx context.Context, loops ...func(context.Context) error) error

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func envHopLe() map[string]string {
	return map[string]string{
		"DATABASE_URL":     "postgres://u:p@localhost:6432/eventflow",
		"REDIS_ADDR":       "localhost:6379",
		"ACTIVE_EVENT_IDS": "ev-1",
		"AMQP_URL":         "amqp://guest:guest@rabbitmq:5672/",
	}
}

func TestLoadConfig_AMQPURLHopLe(t *testing.T) {
	cfg, err := loadConfig(envMap(envHopLe()))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.AMQPURL != "amqp://guest:guest@rabbitmq:5672/" {
		t.Errorf("AMQPURL = %q", cfg.AMQPURL)
	}
}

func TestLoadConfig_ThieuAMQPURLFailFast(t *testing.T) {
	env := envHopLe()
	delete(env, "AMQP_URL")
	_, err := loadConfig(envMap(env))
	if err == nil {
		t.Fatal("thieu AMQP_URL phai loi ngay khi khoi dong")
	}
	if !strings.Contains(err.Error(), "AMQP_URL") {
		t.Errorf("loi phai neu ten bien AMQP_URL: %v", err)
	}
}

func TestLoadConfig_AMQPURLSaiCuPhapKhongLoMatKhau(t *testing.T) {
	for _, u := range []string{
		"http://guest:sieu-bi-mat@rabbitmq:5672/",
		"amqp://guest:sieu-bi-mat@rabbitmq:khong-phai-cong/",
		"khong co scheme sieu-bi-mat",
	} {
		env := envHopLe()
		env["AMQP_URL"] = u
		_, err := loadConfig(envMap(env))
		if err == nil {
			t.Errorf("AMQP_URL %q sai cu phap phai loi", u)
			continue
		}
		if strings.Contains(err.Error(), "sieu-bi-mat") {
			t.Errorf("loi lam lo mat khau/URL: %v", err)
		}
	}
}

func TestRelayConfig_ChiBoUnroutableChoHoldCreated(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := relayConfig(reg)
	if cfg.Exchange != "ticketing.events" {
		t.Errorf("Exchange = %q, muon ticketing.events", cfg.Exchange)
	}
	var created, released bool
	for _, k := range cfg.DropUnroutable {
		created = created || k == "ticketing.hold.created"
		released = released || k == "ticketing.hold.released"
	}
	if !created {
		t.Error("ticketing.hold.created chua co consumer -> phai nam trong DropUnroutable (AC-6)")
	}
	if released {
		t.Error("ticketing.hold.released KHONG duoc bo khi unroutable (mat event, review M5)")
	}
	if cfg.Registerer != prometheus.Registerer(reg) {
		t.Error("Registerer phai la registry truyen vao")
	}
	if _, err := outbox.NewRelay(errDB{}, amqpDialer("amqp://guest:guest@127.0.0.1:1/"), cfg); err != nil {
		t.Errorf("cau hinh relay cua worker khong hop le: %v", err)
	}
}

func TestNewPoolConfig_DungCacheDescribeChoPgBouncer(t *testing.T) {
	pc, err := newPoolConfig("postgres://u:p@localhost:6432/eventflow")
	if err != nil {
		t.Fatalf("newPoolConfig: %v", err)
	}
	if pc.ConnConfig.DefaultQueryExecMode != pgx.QueryExecModeCacheDescribe {
		t.Errorf("DefaultQueryExecMode = %v, muon QueryExecModeCacheDescribe", pc.ConnConfig.DefaultQueryExecMode)
	}
}

func TestAmqpDialer_BrokerKhongToiDuocTraLoi(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pub, closer, err := amqpDialer("amqp://guest:guest@127.0.0.1:1/")(ctx)
	if err == nil {
		if closer != nil {
			_ = closer.Close()
		}
		t.Fatalf("dial toi cong 1 phai loi, nhan publisher %v", pub)
	}
	if ctx.Err() != nil {
		t.Error("dial treo toi het han ctx")
	}
}

// --- B1: relay khong ket noi duoc khong keo sweeper chet ---

// errDB la outbox.DB luon loi (relay khong ket noi duoc thi khong duoc dung toi DB de claim).
type errDB struct{}

var errKhongDung = errors.New("khong duoc goi DB khi relay chua ket noi")

func (errDB) Begin(context.Context) (pgx.Tx, error) { return nil, errKhongDung }
func (errDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errKhongDung
}
func (errDB) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, errKhongDung }
func (errDB) QueryRow(context.Context, string, ...any) pgx.Row        { return errRow{} }

type errRow struct{}

func (errRow) Scan(...any) error { return errKhongDung }

// demGate dem so vong quet cua sweeper.
type demGate struct {
	mu    sync.Mutex
	scans int
}

func (g *demGate) Hold(context.Context, port.HoldRequest) (port.HoldResult, error) {
	return port.HoldResult{}, nil
}
func (g *demGate) Release(context.Context, string, string) error            { return nil }
func (g *demGate) Consume(context.Context, string, string) error            { return nil }
func (g *demGate) IsAdmitted(context.Context, string, string) (bool, error) { return true, nil }
func (g *demGate) ExpiredHolds(context.Context, string, int) ([]string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.scans++
	return nil, nil
}
func (g *demGate) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.scans
}

type nopRepo struct{}

func (nopRepo) GetSaleConfig(context.Context, string, string) (port.SaleConfig, error) {
	return port.SaleConfig{}, nil
}
func (nopRepo) CountPurchased(context.Context, string, string) (int, error) { return 0, nil }
func (nopRepo) GetActiveHold(context.Context, string, string) (port.HoldView, error) {
	return port.HoldView{}, nil
}
func (nopRepo) PersistHold(context.Context, port.PersistHoldParams) (port.HoldView, error) {
	return port.HoldView{}, nil
}
func (nopRepo) ReleaseHold(context.Context, string) (bool, error) { return false, nil }

func TestRunLoops_RelayKhongKetNoiDuocSweeperVanChay(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	gate := &demGate{}
	scfg := worker.DefaultSweeperConfig([]string{"ev-1"})
	scfg.Interval = 10 * time.Millisecond
	sweeper := worker.NewHoldSweeper(gate, nopRepo{}, scfg, log)

	relay, err := outbox.NewRelay(errDB{}, amqpDialer("amqp://guest:guest@127.0.0.1:1/"), relayConfig(prometheus.NewRegistry()))
	if err != nil {
		t.Fatalf("NewRelay: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runLoops(ctx, sweeper.Run, relay.Run) }()

	time.Sleep(500 * time.Millisecond)
	mid := gate.count()
	select {
	case err := <-done:
		t.Fatalf("runLoops tra ve %v khi broker khong toi duoc — sweeper bi dung theo (B1)", err)
	case <-time.After(time.Second):
	}
	if end := gate.count(); end <= mid || mid == 0 {
		t.Errorf("sweeper phai tiep tuc quet khi relay mat ket noi: %d vong o 0.5s, %d vong o 1.5s", mid, end)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("runLoops sau khi huy ctx = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runLoops khong dung sau khi huy ctx")
	}
}

func TestRunLoops_MotVongLapLoiThiDungTatCa(t *testing.T) {
	loi := errors.New("metrics server chet")
	stopped := make(chan struct{})
	err := runLoops(context.Background(),
		func(context.Context) error { return loi },
		func(ctx context.Context) error {
			<-ctx.Done()
			close(stopped)
			return ctx.Err()
		},
	)
	if !errors.Is(err, loi) {
		t.Errorf("runLoops = %v, muon loi dau tien %v", err, loi)
	}
	select {
	case <-stopped:
	default:
		t.Error("runLoops tra ve truoc khi vong lap con lai dung")
	}
}
