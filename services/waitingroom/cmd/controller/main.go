// Command controller chay admit controller (AIMD) cho cac su kien dang ban.
//
// Tach khoi cmd/server la co y: server co the scale ngang tuy y theo luong
// join/status, con controller thi PHAI chi co mot instance dieu khien moi su kien --
// hai instance se tha gap doi so nguoi moi giay.
//
// Nen co the chay NHIEU replica de du phong: moi instance tranh lease Redis cua tung su kien
// (libs/go/leader), chi leader moi chay vong admit, va moi lenh ghi mang epoch de Redis tu choi
// leader cu (fencing). Leader chet thi instance khac len sau toi da LEADER_TTL_SECONDS; tat em
// thi nha khoa va failover gan nhu ngay.
//
// Suc khoe tang duoi lay tu anh chup cac pod ticketing tu ghi len Redis (libs/go/loadsignal).
// Khong co tin hieu thi controller coi la suc ep va giam nhip ve san.
//
// Gioi han hien tai:
//   - Suc khoe tinh chung cho moi su kien; moi su kien co controller va tran rieng, nen nhieu su
//     kien mo ban cung luc se cong don toi tong cac tran. Ngan sach nhip chung la viec tiep theo.
//   - Danh sach su kien lay tu ACTIVE_EVENT_IDS; event-svc se thay the.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/leader"
	"github.com/eventflow/eventflow/libs/go/loadsignal"
	"github.com/eventflow/eventflow/libs/go/otelx"
	metricsadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/metrics"
	redisadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/redis"
	signaladapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/signal"
	"github.com/eventflow/eventflow/services/waitingroom/internal/app"
	"github.com/eventflow/eventflow/services/waitingroom/internal/config"
)

const saleStartPollInterval = 2 * time.Second

func main() {
	log := slog.New(otelx.LogHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
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
	leaderCfg, err := config.LeaderConfig(os.Getenv)
	if err != nil {
		return err
	}
	healthCfg, err := config.HealthConfig(os.Getenv)
	if err != nil {
		return err
	}

	shutdownTrace, err := otelx.Init(ctx, "waitingroom-controller")
	if err != nil {
		return err
	}
	defer func() {
		sh, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTrace(sh); err != nil {
			log.Warn("tat trace loi", "err", err)
		}
	}()

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

	var probe app.HealthProbe
	if healthCfg.Static {
		log.Warn("ADMIT_HEALTH_MODE=static: tin hieu suc khoe GIA, admit rate se leo toi tran bat ke "+
			"Postgres ra sao. Chi dung cho local; production phai de mac dinh (signal)",
			"max_rate", os.Getenv("ADMIT_MAX_RATE"))
		probe = staticHealth{}
	} else {
		probe = signaladapter.NewProbe(loadsignal.NewReader(rdb, loadsignal.DefaultKey, healthCfg.MaxAge))
	}

	prom := metricsadapter.New(prometheus.DefaultRegisterer)
	instance := instanceID()
	log.Info("controller khoi dong", "instance", instance,
		"leader_ttl", leaderCfg.TTL.String(), "leader_renew", leaderCfg.RenewEvery.String())

	var wg sync.WaitGroup
	// Controller khong co HTTP API, nen mo cong rieng chi de Prometheus scrape.
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := otelx.ServeMetrics(ctx, env("METRICS_ADDR", ":9091"), log); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("metrics server dung", "err", err)
		}
	}()
	for _, ev := range eventIDs {
		wg.Add(1)
		go func(eventID string) {
			defer wg.Done()
			// Loi o mot su kien khong duoc keo chet cac su kien khac.
			if err := runEvent(ctx, log, rdb, queue, probe, prom, leaderCfg, instance, eventID); err != nil &&
				!errors.Is(err, context.Canceled) {
				log.Error("controller cua su kien dung", "event", eventID, "err", err)
			}
		}(ev)
	}
	log.Info("controller bat dau", "so_su_kien", len(eventIDs))
	wg.Wait()
	return ctx.Err()
}

// runEvent tranh quyen leader cua mot su kien va, moi nhiem ky, chay controller duoi epoch cua no.
func runEvent(ctx context.Context, log *slog.Logger, rdb *redis.Client, queue *redisadapter.Queue,
	probe app.HealthProbe, prom *metricsadapter.Prom, lc config.Leader, instance, eventID string) error {

	el, err := leader.New(rdb, leader.Config{
		LockKey:    fmt.Sprintf("wr:{%s}:leader", eventID),
		EpochKey:   fmt.Sprintf("wr:{%s}:leader_epoch", eventID),
		ID:         instance,
		TTL:        lc.TTL,
		RenewEvery: lc.RenewEvery,
	}, log)
	if err != nil {
		return err
	}
	prom.LeaderState(eventID, false, 0)

	return el.Run(ctx, func(ctx context.Context, epoch int64) error {
		prom.LeaderState(eventID, true, epoch)
		defer prom.LeaderState(eventID, false, epoch)

		fq := queue.Fenced(epoch)
		err := runLeaderTerm(ctx, log, queue, fq, probe, prom, eventID)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	})
}

// runLeaderTerm cho toi khi co T0 trong Redis roi chay controller cua su kien do cho toi khi het
// nhiem ky (mat quyen, bi fencing, hoac tat).
func runLeaderTerm(ctx context.Context, log *slog.Logger, queue *redisadapter.Queue, fq *redisadapter.FencedQueue,
	probe app.HealthProbe, prom *metricsadapter.Prom, eventID string) error {

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

	return app.NewAdmitController(fq, probe, cfg, log).
		WithMetrics(prom).
		WithStateStore(fq).
		Run(ctx)
}

// staticHealth luon bao tang duoi khoe manh. Xem ADMIT_HEALTH_MODE.
type staticHealth struct{}

func (staticHealth) Health(context.Context, string) (app.Health, error) {
	return app.Health{}, nil
}

// instanceID la dinh danh duy nhat cua tien trinh nay trong cuoc bau leader.
func instanceID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	var b [4]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s-%d-%s", host, os.Getpid(), hex.EncodeToString(b[:]))
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
