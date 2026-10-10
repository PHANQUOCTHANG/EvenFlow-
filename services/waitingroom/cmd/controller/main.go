// Command controller chay admit controller (AIMD) cho cac su kien dang ban.
//
// Tach khoi cmd/server la co y: server co the scale ngang tuy y theo luong
// join/status, con controller thi PHAI chi co mot instance cho moi su kien --
// hai instance se tha gap doi so nguoi moi giay.
//
// Gioi han hien tai (giai doan 0):
//   - TODO(EVF-55): chua bau leader qua Redis lock co fencing. Compose chi chay
//     dung mot ban. admit.lua dung ZPOPMIN nen chay nham hai ban chi lam nhip admit
//     gap doi, khong admit trung mot nguoi.
//   - TODO(EVF-55): HealthProbe la ban tinh (luon bao khoe), vi chua service nao
//     xuat p99 / error rate / pool usage. Rate se leo toi ADMIT_MAX_RATE, nen o
//     local hay dat tran thap.
//   - Danh sach su kien lay tu ACTIVE_EVENT_IDS; event-svc (EVF-20) se thay the.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	redisadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/redis"
	"github.com/eventflow/eventflow/services/waitingroom/internal/app"
	"github.com/eventflow/eventflow/services/waitingroom/internal/config"
)

const saleStartPollInterval = 2 * time.Second

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("controller dung bat thuong", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	eventIDs := config.SplitCSV(os.Getenv("ACTIVE_EVENT_IDS"))
	if len(eventIDs) == 0 {
		return errors.New("thieu ACTIVE_EVENT_IDS: khong co su kien nao de dieu khien")
	}

	// Kiem tra cau hinh TRUOC khi cham vao ha tang: sai thi chet ngay.
	if _, err := config.AdmitConfig(os.Getenv, eventIDs[0], time.Time{}); err != nil {
		return err
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:         env("REDIS_ADDR", "localhost:6379"),
		ReadTimeout:  500 * time.Millisecond,
		WriteTimeout: 500 * time.Millisecond,
	})
	defer func() {
		if err := rdb.Close(); err != nil {
			log.Warn("dong redis client loi", "err", err)
		}
	}()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return err
	}

	queue, err := redisadapter.NewQueue(ctx, rdb, 24*time.Hour)
	if err != nil {
		return err
	}

	log.Warn("HealthProbe tinh: admit rate se khong tu giam khi checkout cham (TODO EVF-55)",
		"max_rate", os.Getenv("ADMIT_MAX_RATE"))

	var wg sync.WaitGroup
	for _, ev := range eventIDs {
		wg.Add(1)
		go func(eventID string) {
			defer wg.Done()
			// Loi o mot su kien khong duoc keo chet cac su kien khac.
			if err := runEvent(ctx, log, queue, eventID); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("controller cua su kien dung", "event", eventID, "err", err)
			}
		}(ev)
	}
	log.Info("controller bat dau", "so_su_kien", len(eventIDs))
	wg.Wait()
	return ctx.Err()
}

// runEvent cho toi khi co T0 trong Redis roi chay controller cua su kien do.
func runEvent(ctx context.Context, log *slog.Logger, queue *redisadapter.Queue, eventID string) error {
	var saleStart time.Time
	for {
		t, ok, err := queue.SaleStart(ctx, eventID)
		switch {
		case err != nil:
			log.Error("doc sale_start_ms that bai", "event", eventID, "err", err)
		case ok:
			saleStart = t
		default:
			log.Info("chua co sale_start_ms, tiep tuc cho", "event", eventID)
		}
		if !saleStart.IsZero() {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(saleStartPollInterval):
		}
	}

	cfg, err := config.AdmitConfig(os.Getenv, eventID, saleStart)
	if err != nil {
		return err
	}
	log.Info("khoi dong admit controller", "event", eventID,
		"sale_start", saleStart.UTC().Format(time.RFC3339),
		"min_rate", cfg.MinRate, "max_rate", cfg.MaxRate)

	return app.NewAdmitController(queue, staticHealth{}, cfg, log).Run(ctx)
}

// staticHealth luon bao tang duoi khoe manh. Xem ghi chu dau file.
type staticHealth struct{}

func (staticHealth) Health(context.Context, string) (app.Health, error) {
	return app.Health{}, nil
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
