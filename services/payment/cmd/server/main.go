// Command server la diem vao cua payment-svc (EV-150 / EVF-70).
//
// Layout hexagonal chuan: xem docs/03-cau-truc-src.md.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eventflow/eventflow/libs/go/httpx"
	"github.com/eventflow/eventflow/libs/go/idem"
	"github.com/eventflow/eventflow/libs/go/otelx"
	gatewayadapter "github.com/eventflow/eventflow/services/payment/internal/adapter/gateway"
	httpadapter "github.com/eventflow/eventflow/services/payment/internal/adapter/http"
	metricsadapter "github.com/eventflow/eventflow/services/payment/internal/adapter/metrics"
	"github.com/eventflow/eventflow/services/payment/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/payment/internal/app"
	"github.com/eventflow/eventflow/services/payment/internal/config"
	"github.com/eventflow/eventflow/services/payment/internal/domain"
	"github.com/eventflow/eventflow/services/payment/internal/port"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

type uuidGenerator struct{}

func (uuidGenerator) NewID() string {
	return uuid.New().String()
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg := config.LoadFromEnv()

	log.Info("khoi dong payment-svc",
		"http_addr", cfg.HTTPAddr,
		"default_provider", cfg.DefaultProvider,
		"enabled_providers", cfg.EnabledProviders,
		"sandbox_mode", cfg.SandboxMode,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var repo port.PaymentRepository
	var idemStore idem.Store

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Warn("khong the ket noi postgres, su dung memory repository cho che do sandbox local", "err", err)
		repo = postgres.NewMemoryRepository()
	} else if pingErr := pool.Ping(ctx); pingErr != nil {
		log.Warn("postgres ping that bai, su dung memory repository cho che do sandbox local", "err", pingErr)
		repo = postgres.NewMemoryRepository()
	} else {
		log.Info("ket noi postgres thanh cong")
		repo = postgres.NewRepository(pool)
		idemStore = idem.NewPostgresStore(pool)
		defer pool.Close()
	}

	// Khoi tao va dang ky cac cong thanh toan vao Gateway Registry (AC-1..AC-5, AC-7)
	registry := gatewayadapter.NewRegistry(domain.NormalizeProvider(cfg.DefaultProvider), cfg.EnabledProviders)

	sbxGW := gatewayadapter.NewSandboxGateway(gatewayadapter.SandboxMode(cfg.SandboxMode))
	registry.Register(sbxGW)

	vnpGW := gatewayadapter.NewVNPayGateway(gatewayadapter.VNPayConfig{
		TmnCode:    cfg.VNPayTmnCode,
		HashSecret: cfg.VNPayHashSecret,
		BaseURL:    cfg.VNPayBaseURL,
		ReturnURL:  cfg.VNPayReturnURL,
	})
	registry.Register(vnpGW)

	momoGW := gatewayadapter.NewMoMoGateway(gatewayadapter.MoMoConfig{
		PartnerCode: cfg.MoMoPartnerCode,
		AccessKey:   cfg.MoMoAccessKey,
		SecretKey:   cfg.MoMoSecretKey,
		Endpoint:    cfg.MoMoEndpoint,
		ReturnURL:   cfg.MoMoReturnURL,
		NotifyURL:   cfg.MoMoNotifyURL,
	})
	registry.Register(momoGW)

	stripeGW := gatewayadapter.NewStripeGateway(gatewayadapter.StripeConfig{
		PublishableKey: cfg.StripePublishableKey,
		SecretKey:      cfg.StripeSecretKey,
		WebhookSecret:  cfg.StripeWebhookSecret,
		SuccessURL:     cfg.StripeSuccessURL,
		CancelURL:      cfg.StripeCancelURL,
	})
	registry.Register(stripeGW)

	// Khoi tao Use cases va Metrics (AC-6, AC-7)
	clock := port.SystemClock{}
	idGen := uuidGenerator{}
	metrics := metricsadapter.New(prometheus.DefaultRegisterer)

	initiatePaymentUC := app.NewInitiatePaymentUseCase(repo, registry, clock, idGen, log)
	getPaymentUC := app.NewGetPaymentUseCase(repo)
	listMethodsUC := app.NewListMethodsUseCase(registry)
	processWebhookUC := app.NewProcessWebhookUseCase(repo, registry, clock, log)
	replayInboxUC := app.NewReplayPaymentInboxUseCase(repo, clock, log)
	reconcileOrdersUC := app.NewReconcileStaleOrdersUseCase(repo, registry, clock, metrics, log)
	refundCancelledEventUC := app.NewRefundCancelledEventUseCase(repo, registry, clock, log)

	// Khoi tao HTTP handler va dang ky routes (bao gom webhook exactly-once BR-O6, inbox replay, doi soat EV-153 va hoan tien EV-154)
	handler := httpadapter.NewHandler(initiatePaymentUC, getPaymentUC, listMethodsUC, processWebhookUC, log).
		WithInboxReplayer(replayInboxUC).
		WithOrderReconciler(reconcileOrdersUC).
		WithEventRefunder(refundCancelledEventUC)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("GET /metrics", otelx.MetricsHandler())

	var idemMiddleware httpx.Middleware
	if idemStore != nil {
		idemMiddleware = idem.Middleware(idemStore, idem.WithLogger(log))
	} else {
		// Neu khong co DB cho idem store luc local offline, yeu cau co header nhung pass qua an toan
		idemMiddleware = func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Idempotency-Key") == "" {
					httpx.WriteProblem(w, http.StatusBadRequest, "bat buoc Idempotency-Key khi goi API ghi (BR-O5)")
					return
				}
				next.ServeHTTP(w, r)
			})
		}
	}

	handler.Routes(mux, idemMiddleware)

	server := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Khoi dong background worker dinh ky replay payment_inbox va quet stale records (EV-152 / BR-O6)
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		log.Info("khoi dong background worker replay payment_inbox", "interval", "30s")

		for {
			select {
			case <-workerCtx.Done():
				log.Info("dung background worker replay payment_inbox")
				return
			case <-ticker.C:
				res, err := replayInboxUC.Execute(workerCtx, 50)
				if err != nil {
					log.Error("background replay payment_inbox gap loi", "err", err)
				} else if res.TotalScanned > 0 || res.StaleDetected > 0 {
					log.Info("ket qua chay background replay payment_inbox",
						"scanned", res.TotalScanned,
						"succeeded", res.Succeeded,
						"already_done", res.AlreadyDone,
						"still_pending", res.StillPending,
						"failed", res.Failed,
						"stale_detected", res.StaleDetected,
					)
				}
			}
		}
	}()

	// Khoi dong background worker dinh ky doi soat don treo qua 15 phut (EV-153 / EVF-73)
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		log.Info("khoi dong background worker doi soat don treo", "interval", "60s", "threshold", "15m")

		for {
			select {
			case <-workerCtx.Done():
				log.Info("dung background worker doi soat don treo")
				return
			case <-ticker.C:
				res, err := reconcileOrdersUC.Execute(workerCtx, 50)
				if err != nil {
					log.Error("background doi soat don treo gap loi", "err", err)
				} else if res.TotalScanned > 0 {
					log.Info("ket qua chay background doi soat don treo",
						"scanned", res.TotalScanned,
						"healed_paid", res.HealedPaid,
						"healed_cancelled", res.HealedCancelled,
						"still_pending", res.StillPending,
						"skipped", res.Skipped,
						"errors", res.Errors,
						"duration", res.Duration,
					)
				}
			}
		}
	}()

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Info("payment-svc dang lang nghe", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server dung dot ngot", "err", err)
			os.Exit(1)
		}
	}()

	<-stopChan
	log.Info("nhan tin hieu tat, dang dong ket noi em ai...")
	workerCancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("dong server gap loi", "err", err)
	}

	log.Info("payment-svc da tat an toan")
}
