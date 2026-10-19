// Command server la diem vao cua notification-svc (EV-155 / EVF-75).
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

	httpadapter "github.com/eventflow/eventflow/services/notification/internal/adapter/http"
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

	// 6. Khoi tao Consumer RabbitMQ
	_ = rabbitmq.NewNotificationEventConsumer(orderPaidUC, ticketIssuedUC, refundUC, log)

	// 7. Khoi tao HTTP Server
	mux := http.NewServeMux()
	handler := httpadapter.NewNotificationHandler(orderPaidUC, ticketIssuedUC, refundUC, getHistoryUC)
	handler.RegisterRoutes(mux)

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

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("loi khi tat server", "err", err)
	}

	log.Info("notification server da tat an toan")
}
