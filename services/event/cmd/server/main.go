// Command server la diem vao cua event-svc.
//
// Hien thuc theo specs EV-117, EV-118, EV-119.
package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	adapterHTTP "github.com/eventflow/eventflow/services/event/internal/adapter/http"
	"github.com/eventflow/eventflow/services/event/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/event/internal/app"
	_ "github.com/lib/pq"
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
	createUC := app.NewCreateEventUseCase(repo)
	updateUC := app.NewUpdateEventUseCase(repo)
	tiersUC := app.NewManageTiersUseCase(repo)
	transitionUC := app.NewTransitionStateUseCase(repo)
	getUC := app.NewGetEventUseCase(repo)

	handler := adapterHTTP.NewEventHandler(createUC, updateUC, tiersUC, transitionUC, getUC)
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

	<-stop
	log.Info("dang tat event service...")

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
