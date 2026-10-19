// Command server la diem vao cua event-svc.
//
// Hien thuc theo specs EV-117, EV-118, EV-119.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eventflow/eventflow/libs/go/mq"
	adapterHTTP "github.com/eventflow/eventflow/services/event/internal/adapter/http"
	"github.com/eventflow/eventflow/services/event/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/event/internal/app"
	"github.com/eventflow/eventflow/services/event/internal/worker"
	_ "github.com/lib/pq"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://eventflow:eventflow@localhost:5432/eventflow?sslmode=disable"
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Error("failed to open database", "err", err)
	} else {
		db.SetMaxOpenConns(50)
		db.SetMaxIdleConns(10)
		db.SetConnMaxLifetime(5 * time.Minute)
	}

	repo := postgres.NewEventRepository(db)
	modRepo := postgres.NewModerationRepository(db)
	auditRepo := postgres.NewAuditLogRepository(db)

	createUC := app.NewCreateEventUseCase(repo)
	updateUC := app.NewUpdateEventUseCase(repo)
	tiersUC := app.NewManageTiersUseCase(repo)
	transitionUC := app.NewTransitionStateUseCase(repo)
	getUC := app.NewGetEventUseCase(repo)

	aiVerdictUC := app.NewHandleAiVerdictUseCase(repo, modRepo, auditRepo)
	modQueueUC := app.NewListModerationQueueUseCase(repo, modRepo)
	modActionUC := app.NewModeratorActionUseCase(repo, modRepo, auditRepo)

	handler := adapterHTTP.NewEventHandler(createUC, updateUC, tiersUC, transitionUC, getUC).
		WithModeration(aiVerdictUC, modQueueUC, modActionUC).
		WithAudit(auditRepo)
	router := adapterHTTP.NewRouter(handler)

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8084"
	}

	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Info("event service dang lang nghe", "addr", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server loi", "err", err)
			os.Exit(1)
		}
	}()

	// EV-120: relay outbox -> RabbitMQ (job kiem duyet AI). Khong co RABBITMQ_URL thi job van
	// nam an toan trong outbox_events, chi chua duoc phat -- canh bao to de khong ai tuong
	// kiem duyet dang chay.
	stopRelay := func() {}
	if rabbitURL := os.Getenv("RABBITMQ_URL"); rabbitURL != "" && db != nil {
		stopRelay, err = startOutboxRelay(db, rabbitURL, log)
		if err != nil {
			log.Error("khoi dong outbox relay that bai", "err", err)
			os.Exit(1)
		}
	} else {
		log.Warn("RABBITMQ_URL trong: job kiem duyet AI se nam trong outbox_events, KHONG duoc phat")
	}

	<-stop
	log.Info("dang tat event service...")
	stopRelay()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Error("loi khi tat server", "err", err)
	}
	if db != nil {
		_ = db.Close()
	}
	log.Info("event service da tat an toan")
}

// startOutboxRelay ket noi RabbitMQ, khai bao topology AI va chay relay trong goroutine.
// Tra ve ham dung: huy relay, cho no thoat, roi dong ket noi.
func startOutboxRelay(db *sql.DB, rabbitURL string, log *slog.Logger) (func(), error) {
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return nil, fmt.Errorf("ket noi rabbitmq: %w", err)
	}
	fail := func(err error) (func(), error) {
		_ = conn.Close()
		return nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		return fail(fmt.Errorf("mo kenh rabbitmq: %w", err))
	}
	// Topology AI do code Go khai bao (nguon chuan, EVF-7); idempotent voi ai-worker.
	if err := mq.Declare(ch, mq.AITopology()); err != nil {
		return fail(fmt.Errorf("khai bao topology AI: %w", err))
	}
	pub, err := mq.NewPublisher(ch)
	if err != nil {
		return fail(fmt.Errorf("tao publisher: %w", err))
	}
	relay, err := worker.NewOutboxRelay(db, pub, worker.DefaultRoutes(), worker.DefaultOutboxRelayConfig(), log)
	if err != nil {
		return fail(fmt.Errorf("cau hinh outbox relay: %w", err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = relay.Run(ctx)
	}()
	log.Info("outbox relay dang chay", "aggregate_type", app.OutboxAggregateEvent)
	return func() {
		cancel()
		<-done
		_ = conn.Close()
	}, nil
}
