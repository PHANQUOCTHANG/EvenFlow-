// Command server la diem vao cua notification-svc (EV-155 / EVF-75).
//
// Layout hexagonal chuan: xem docs/03-cau-truc-src.md.
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
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/eventflow/eventflow/libs/go/mq"
	"github.com/eventflow/eventflow/libs/go/otelx"
	httpadapter "github.com/eventflow/eventflow/services/notification/internal/adapter/http"
	pgadapter "github.com/eventflow/eventflow/services/notification/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/notification/internal/adapter/rabbitmq"
	"github.com/eventflow/eventflow/services/notification/internal/adapter/sender"
	"github.com/eventflow/eventflow/services/notification/internal/adapter/store"
	"github.com/eventflow/eventflow/services/notification/internal/adapter/template"
	"github.com/eventflow/eventflow/services/notification/internal/app"
	"github.com/eventflow/eventflow/services/notification/internal/config"
	"github.com/eventflow/eventflow/services/notification/internal/port"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg := config.LoadFromEnv()

	log.Info("khoi dong notification-svc (EV-155)",
		"http_addr", cfg.HTTPAddr,
		"email_provider", cfg.EmailProvider,
		"sms_provider", cfg.SMSProvider,
	)

	// EV-103: span cua consumer (mq.Consume) noi tiep trace cua payment/ticketing qua header AMQP.
	shutdownTrace, err := otelx.Init(context.Background(), "notification")
	if err != nil {
		log.Error("khoi tao trace that bai", "err", err)
		os.Exit(1)
	}
	defer func() { _ = shutdownTrace(context.Background()) }()

	// 1. Khoi tao Template Renderer tieng Viet
	renderer, err := template.NewGoTemplateRenderer()
	if err != nil {
		log.Error("khong the khoi tao template renderer", "err", err)
		os.Exit(1)
	}

	// 2. Khoi tao Store (Memory / Idempotency)
	memStore := store.NewMemoryNotificationStore()

	// 3. Khoi tao Email Sender
	var emailSender port.EmailSender
	if cfg.EmailProvider == "smtp" {
		log.Info("su dung SMTP Email Sender", "host", cfg.SMTPHost, "port", cfg.SMTPPort, "from", cfg.SMTPFrom)
		emailSender = sender.NewSMTPEmailSender(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPassword, cfg.SMTPFrom)
	} else {
		log.Info("su dung Mock Email Sender cho local / testing")
		emailSender = sender.NewMockEmailSender()
	}

	// 4. Khoi tao SMS Sender
	var smsSender port.SMSSender
	if cfg.SMSProvider == "webhook" && cfg.SMSWebhookURL != "" {
		log.Info("su dung Webhook SMS Sender", "url", cfg.SMSWebhookURL)
		smsSender = sender.NewWebhookSMSSender(cfg.SMSWebhookURL, cfg.SMSAPIKey)
	} else {
		log.Info("su dung Mock SMS Sender cho local / testing")
		smsSender = sender.NewMockSMSSender()
	}

	// 5. Khoi tao App Use Cases
	orderPaidUC := app.NewProcessOrderPaidUseCase(emailSender, smsSender, renderer, memStore, log)
	ticketIssuedUC := app.NewProcessTicketIssuedUseCase(emailSender, smsSender, renderer, memStore, log)
	refundUC := app.NewProcessRefundUseCase(emailSender, smsSender, renderer, memStore, log)
	getHistoryUC := app.NewGetNotificationHistoryUseCase(memStore)

	// 6. Consumer RabbitMQ: order.paid, payment.refunded, ticketing.ticket.issued -> email/SMS.
	// Payment/ticketing chi phat identity_id, email lay tu Postgres (DATABASE_URL).
	consumer := rabbitmq.NewNotificationEventConsumer(orderPaidUC, ticketIssuedUC, refundUC, log)
	consumeCtx, stopConsume := context.WithCancel(context.Background())
	defer stopConsume()
	if cfg.DatabaseURL != "" {
		pool, err := pgxpool.New(consumeCtx, cfg.DatabaseURL)
		if err != nil {
			log.Error("ket noi postgres that bai", "err", err)
			os.Exit(1)
		}
		defer pool.Close()
		consumer.WithDirectory(pgadapter.NewDirectory(pool))
	} else {
		log.Warn("khong co DATABASE_URL: chi gui duoc message da mang san recipient_email")
	}
	if os.Getenv("AMQP_URL") == "" {
		log.Warn("khong co AMQP_URL: notification khong nghe RabbitMQ, chi nhan qua HTTP")
	} else {
		closeMQ, err := startConsumer(consumeCtx, cfg.AMQPURL, consumer, log)
		if err != nil {
			log.Error("khoi dong consumer rabbitmq that bai", "err", err)
			os.Exit(1)
		}
		defer closeMQ()
	}

	// 7. Khoi tao HTTP Server
	mux := http.NewServeMux()
	handler := httpadapter.NewNotificationHandler(orderPaidUC, ticketIssuedUC, refundUC, getHistoryUC)
	handler.RegisterRoutes(mux)
	mux.Handle("GET /metrics", otelx.MetricsHandler())

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Lang nghe tin hieu dung (Graceful shutdown)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Info("notification server dang lang nghe", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server HTTP gap loi", "err", err)
			os.Exit(1)
		}
	}()

	<-stop
	log.Info("nhan tin hieu tat, dang dung server gracefully...")
	stopConsume()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("loi khi tat server", "err", err)
	}

	log.Info("notification server da tat an toan")
}

// startConsumer ket noi RabbitMQ, khai bao topology (kem exchange nguon payment.events va
// ticketing.events) va consume notification.events.q trong goroutine. Consumer dung hoac mat
// ket noi thi thoat process de compose/k8s khoi dong lai.
func startConsumer(ctx context.Context, url string, c *rabbitmq.NotificationEventConsumer, log *slog.Logger) (func(), error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("ket noi rabbitmq: %w", err)
	}
	fail := func(what string, err error) (func(), error) {
		_ = conn.Close()
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	declCh, err := conn.Channel()
	if err != nil {
		return fail("mo channel", err)
	}
	if err := mq.Declare(declCh, mq.Merge(mq.PaymentTopology(), mq.TicketingTopology(), mq.NotificationTopology())); err != nil {
		return fail("khai bao topology", err)
	}
	consumeCh, err := conn.Channel()
	if err != nil {
		return fail("mo channel consume", err)
	}
	forwardCh, err := conn.Channel()
	if err != nil {
		return fail("mo channel retry", err)
	}
	forwardPub, err := mq.NewPublisher(forwardCh)
	if err != nil {
		return fail("tao publisher retry", err)
	}

	cfg := mq.ConsumerConfig{
		Queue:              mq.NotificationEventsQueue,
		MaxAttempts:        5,
		RetryExchange:      mq.NotificationRetryExchange,
		RetryKey:           mq.NotificationEventsKey,
		DeadLetterExchange: mq.NotificationDeadLetterEx,
		DeadLetterKey:      mq.NotificationEventsKey,
	}
	go func() {
		err := mq.Consume(ctx, consumeCh, forwardPub, cfg, c.HandleDelivery)
		if ctx.Err() == nil {
			log.Error("consumer rabbitmq dung, thoat de khoi dong lai", "err", err)
			os.Exit(1)
		}
	}()
	log.Info("consumer rabbitmq dang chay", "queue", cfg.Queue)
	return func() { _ = conn.Close() }, nil
}
