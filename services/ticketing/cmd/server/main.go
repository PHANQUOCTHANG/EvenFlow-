// Command server phuc vu API ticketing.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/authx"
	"github.com/eventflow/eventflow/libs/go/httpx"
	"github.com/eventflow/eventflow/libs/go/idem"
	"github.com/eventflow/eventflow/libs/go/loadsignal"
	"github.com/eventflow/eventflow/libs/go/otelx"
	httpadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/http"
	metricsadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/metrics"
	postgresadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/postgres"
	redisadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/redis"
	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

func main() {
	log := slog.New(otelx.LogHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if err := run(log); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server dung bat thuong", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTrace, err := otelx.Init(ctx, "ticketing")
	if err != nil {
		return err
	}
	defer func() {
		// Xa not span con dem; khong de Jaeger cham lam treo luc tat.
		sh, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTrace(sh); err != nil {
			log.Warn("tat trace loi", "err", err)
		}
	}()

	cfg, err := pgxpool.ParseConfig(env("DATABASE_URL", ""))
	if err != nil {
		return err
	}
	// Pool nho la CO Y. Duong di toi day da bi phong cho ghim luu luong, nen
	// nut co chai that su la Postgres. Mo qua nhieu ket noi chi lam tang
	// contention va lam p99 xau di, khong lam tang thong luong.
	cfg.MaxConns = 25
	cfg.MinConns = 5
	cfg.MaxConnLifetime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return err
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:         env("REDIS_ADDR", "localhost:6379"),
		PoolSize:     256,
		MinIdleConns: 32,
		ReadTimeout:  200 * time.Millisecond,
		WriteTimeout: 200 * time.Millisecond,
	})
	defer func() {
		if err := rdb.Close(); err != nil {
			log.Warn("dong redis client loi", "err", err)
		}
	}()

	gate, err := redisadapter.NewGate(ctx, rdb)
	if err != nil {
		return err
	}
	repo := postgresadapter.NewRepository(pool)

	prom := metricsadapter.New(prometheus.DefaultRegisterer)
	// Muc dung pool la tin hieu backpressure cho admit controller: pool can thi
	// Postgres dang qua tai.
	poolUsage := func() float64 {
		st := pool.Stat()
		if st.MaxConns() == 0 {
			return 0
		}
		return float64(st.AcquiredConns()) / float64(st.MaxConns())
	}
	prom.RegisterPoolUsage(poolUsage)

	// Tin hieu suc khoe cho admit controller (EV-142). Moi pod tu ghi anh chup cua so 5 giay len
	// Redis moi giay; controller gop lai va tu giam nhip khi p99 / loi / pool vuot nguong.
	signals := loadsignal.NewRecorder()
	go loadsignal.NewPublisher(rdb, loadsignal.DefaultKey, podID(), signals, poolUsage, log).Run(ctx)

	qrSecret := []byte(env("HMAC_QR_SECRET", "evenflow-default-development-secret-key-32b!"))
	qrEngine, err := authx.NewQREngine(qrSecret)
	if err != nil {
		return fmt.Errorf("khoi tao authx qr engine that bai: %w", err)
	}
	issueTicketsUC := app.NewIssueTicketsUseCase(repo, qrEngine, log)
	verifyTicketUC := app.NewVerifyTicketUseCase(repo, qrEngine, log)

	h := httpadapter.New(app.NewCreateHold(gate, repo, log).WithMetrics(signalMetrics{HoldMetrics: prom, rec: signals}), log).
		WithPurchaseLimits(app.NewGetPurchaseLimit(repo)).
		WithTickets(issueTicketsUC, verifyTicketUC, repo)

	// BR-O5. Dinh danh lay tu X-Identity-Id cho toi khi gateway cap JWT (giai doan 3); no nam
	// trong hash cua request nen nguoi khac dung trung key khong nhan duoc response cua nhau.
	idempotency := idem.Middleware(idem.NewPostgresStore(pool),
		idem.WithIdentity(func(r *http.Request) string { return r.Header.Get("X-Identity-Id") }),
		idem.WithLogger(log))

	mux := http.NewServeMux()
	h.Routes(mux, idempotency)
	// Cong khai tren cong dich vu: gateway KHONG duoc route /metrics ra ngoai.
	mux.Handle("GET /metrics", otelx.MetricsHandler())
	httpx.Health(mux,
		pool.Ping,
		func(ctx context.Context) error { return rdb.Ping(ctx).Err() },
	)

	srv := httpx.NewServer(env("HTTP_ADDR", ":8082"),
		httpx.Chain(mux, httpx.RequestID(), otelx.HTTP("ticketing"), httpx.AccessLog(log), httpx.Recover(log)))

	log.Info("ticketing dang lang nghe", "addr", srv.Addr)
	// 20 giay de cac transaction dang duoc commit xong. Cat ngang giua mot
	// transaction tao hold la cach nhanh nhat de tao ra ve bi khoa.
	return httpx.Run(ctx, srv, 20*time.Second)
}

// signalMetrics them viec ghi tin hieu suc khoe cho admit controller vao cung diem do HoldAttempt.
type signalMetrics struct {
	port.HoldMetrics
	rec *loadsignal.Recorder
}

func (m signalMetrics) HoldAttempt(outcome string, d time.Duration) {
	m.HoldMetrics.HoldAttempt(outcome, d)
	// "Chua duoc admit" bi tu choi truoc khi cham Postgres: tinh vao se pha loang p99 dung luc
	// tang duoi dang yeu. Het ve, vuot han muc va thanh cong deu di qua tang duoi.
	if outcome == port.OutcomeNotAdmitted {
		return
	}
	// Chi loi HE THONG tinh la loi; het ve / vuot han muc la ket qua nghiep vu binh thuong.
	m.rec.Observe(d, outcome == port.OutcomeError)
}

// podID dinh danh pod trong tin hieu suc khoe.
func podID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "ticketing"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
