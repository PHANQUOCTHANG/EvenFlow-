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
	gatewayadapter "github.com/eventflow/eventflow/services/payment/internal/adapter/gateway"
	httpadapter "github.com/eventflow/eventflow/services/payment/internal/adapter/http"
	"github.com/eventflow/eventflow/services/payment/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/payment/internal/app"
	"github.com/eventflow/eventflow/services/payment/internal/config"
	"github.com/eventflow/eventflow/services/payment/internal/domain"
	"github.com/eventflow/eventflow/services/payment/internal/port"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
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

	// Khoi tao Use cases
	clock := port.SystemClock{}
	idGen := uuidGenerator{}

	initiatePaymentUC := app.NewInitiatePaymentUseCase(repo, registry, clock, idGen, log)
	getPaymentUC := app.NewGetPaymentUseCase(repo)
	listMethodsUC := app.NewListMethodsUseCase(registry)

	// Khoi tao HTTP handler va dang ky routes
	handler := httpadapter.NewHandler(initiatePaymentUC, getPaymentUC, listMethodsUC, log)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

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

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("dong server gap loi", "err", err)
	}

	log.Info("payment-svc da tat an toan")
}
