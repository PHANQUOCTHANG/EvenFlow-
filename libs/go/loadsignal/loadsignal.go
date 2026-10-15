// Package loadsignal truyen tin hieu suc khoe cua mot tang duoi (p99, ty le loi, muc dung pool DB)
// toi bo dieu khien tai, qua Redis.
//
// Vi sao khong hoi Prometheus: Prometheus scrape moi 15 giay roi cong cua so rate(), nen tin hieu
// tre hon 20 giay -- cham hon muc tieu "giam trong <= 10 giay" cua admit controller, va dua mot
// he thong giam sat thanh diem chet cua vong dieu khien. O day moi pod tu ghi anh chup cua cua so
// 5 giay len Redis moi giay; ben doc gop lai.
//
//	pod ticketing:  Recorder.Observe(...)  ->  Publisher (HSET moi giay)
//	controller:     Reader.Read()          ->  Aggregate (gop moi pod con song)
//
// Tin hieu cu bi bo qua (pod chet thi khong con "khoe" mai). Khong co pod nao con bao cao nghia la
// KHONG BIET, va nguoi goi phai coi do la dau hieu xau.
package loadsignal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sort"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// DefaultKey la hash chua anh chup cua moi pod ticketing: field = pod, value = Snapshot (JSON).
const DefaultKey = "loadsignal:ticketing"

const (
	windowSeconds = 5    // cua so lam muot: mot mau 1 giay don le khong du de quyet dinh
	maxSamples    = 2048 // so mau do tre giu moi giay (reservoir): bo nho co dinh du hang chuc nghin rps
)

// Snapshot la anh chup suc khoe cua mot pod trong cua so gan nhat.
type Snapshot struct {
	Pod       string  `json:"pod"`
	AtMs      int64   `json:"at_ms"`
	Requests  int64   `json:"requests"`
	Errors    int64   `json:"errors"`
	P99Ms     float64 `json:"p99_ms"`
	PoolUsage float64 `json:"pool_usage"`
}

// Recorder gom do tre va loi cua mot pod trong cua so truot 5 giay.
type Recorder struct {
	mu      sync.Mutex
	now     func() time.Time
	buckets [windowSeconds]bucket
}

type bucket struct {
	sec     int64
	reqs    int64
	errs    int64
	seen    int64
	samples []float64
}

func NewRecorder() *Recorder { return &Recorder{now: time.Now} }

// Observe ghi nhan mot yeu cau da xong. isErr chi dung cho loi HE THONG (khong tinh het ve,
// vuot han muc, chua duoc admit: do la ket qua nghiep vu binh thuong, khong phai tang duoi yeu).
func (r *Recorder) Observe(d time.Duration, isErr bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sec := r.now().Unix()
	b := &r.buckets[sec%windowSeconds]
	if b.sec != sec {
		*b = bucket{sec: sec, samples: b.samples[:0]}
	}
	b.reqs++
	if isErr {
		b.errs++
	}
	b.seen++
	ms := float64(d) / float64(time.Millisecond)
	switch {
	case len(b.samples) < maxSamples:
		b.samples = append(b.samples, ms)
	default:
		// Reservoir sampling: mau thu k giu lai voi xac suat maxSamples/k.
		if j := rand.Int64N(b.seen); j < maxSamples {
			b.samples[j] = ms
		}
	}
}

// Snapshot tong hop cac bucket con trong cua so. pod va poolUsage do noi goi cung cap.
func (r *Recorder) Snapshot(pod string, poolUsage float64) Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	cur := now.Unix()
	s := Snapshot{Pod: pod, AtMs: now.UnixMilli(), PoolUsage: poolUsage}
	var all []float64
	for i := range r.buckets {
		b := &r.buckets[i]
		if b.sec <= cur-windowSeconds || b.sec > cur {
			continue
		}
		s.Requests += b.reqs
		s.Errors += b.errs
		all = append(all, b.samples...)
	}
	if len(all) > 0 {
		sort.Float64s(all)
		idx := int(float64(len(all))*0.99+0.9999999) - 1 // nearest-rank
		idx = max(0, min(idx, len(all)-1))
		s.P99Ms = all[idx]
	}
	return s
}

// Publisher ghi anh chup cua pod len Redis theo chu ky.
type Publisher struct {
	rdb      redis.Cmdable
	key      string
	pod      string
	rec      *Recorder
	pool     func() float64
	interval time.Duration
	log      *slog.Logger
}

// NewPublisher. pool tra ve muc dung pool DB 0..1 (nil = 0).
func NewPublisher(rdb redis.Cmdable, key, pod string, rec *Recorder, pool func() float64, log *slog.Logger) *Publisher {
	if key == "" {
		key = DefaultKey
	}
	if pool == nil {
		pool = func() float64 { return 0 }
	}
	if log == nil {
		log = slog.Default()
	}
	return &Publisher{rdb: rdb, key: key, pod: pod, rec: rec, pool: pool, interval: time.Second, log: log}
}

// Run ghi moi giay cho toi khi ctx bi huy. Loi ghi chi duoc log: mat tin hieu thi ben doc se coi
// la khong biet va tu giam toc -- huong an toan.
func (p *Publisher) Run(ctx context.Context) {
	t := time.NewTicker(p.interval)
	defer t.Stop()
	for {
		if err := p.publish(ctx); err != nil && ctx.Err() == nil {
			p.log.Warn("loadsignal: ghi anh chup that bai", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (p *Publisher) publish(ctx context.Context) error {
	raw, err := json.Marshal(p.rec.Snapshot(p.pod, p.pool()))
	if err != nil {
		return err
	}
	return p.rdb.HSet(ctx, p.key, p.pod, raw).Err()
}

// Aggregate la tin hieu da gop cua moi pod con bao cao.
type Aggregate struct {
	Pods      int
	Requests  int64
	Errors    int64
	P99       time.Duration // pod TE NHAT: mot pod cham la du de nguoi dung thay cham
	ErrorRate float64       // tong loi / tong yeu cau
	PoolUsage float64       // pod CAO NHAT
}

// ErrNoSignal: khong pod nao bao cao trong khoang maxAge. Khong phai "khoe" -- la "khong biet".
var ErrNoSignal = errors.New("loadsignal: khong pod nao con bao cao suc khoe")

// Reader gop anh chup cua cac pod.
type Reader struct {
	rdb    redis.Cmdable
	key    string
	maxAge time.Duration
	now    func() time.Time
}

// NewReader. maxAge la tuoi toi da cua mot anh chup con duoc tin (khuyen nghi 3s voi chu ky 1s).
func NewReader(rdb redis.Cmdable, key string, maxAge time.Duration) *Reader {
	if key == "" {
		key = DefaultKey
	}
	return &Reader{rdb: rdb, key: key, maxAge: maxAge, now: time.Now}
}

// Read tra ve ErrNoSignal khi khong co anh chup nao con moi.
func (r *Reader) Read(ctx context.Context) (Aggregate, error) {
	all, err := r.rdb.HGetAll(ctx, r.key).Result()
	if err != nil {
		return Aggregate{}, fmt.Errorf("loadsignal: doc: %w", err)
	}

	var a Aggregate
	var stale []string
	cutoff := r.now().Add(-r.maxAge).UnixMilli()
	for pod, raw := range all {
		var s Snapshot
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			stale = append(stale, pod) // rac: don di, khong de lam hong moi lan doc
			continue
		}
		if s.AtMs < cutoff {
			if s.AtMs < r.now().Add(-10*r.maxAge).UnixMilli() {
				stale = append(stale, pod) // pod da chet tu lau: don de hash khong phinh
			}
			continue
		}
		a.Pods++
		a.Requests += s.Requests
		a.Errors += s.Errors
		a.P99 = max(a.P99, time.Duration(s.P99Ms*float64(time.Millisecond)))
		a.PoolUsage = max(a.PoolUsage, s.PoolUsage)
	}
	if len(stale) > 0 {
		_ = r.rdb.HDel(ctx, r.key, stale...).Err() // tot nhat co the
	}
	if a.Pods == 0 {
		return Aggregate{}, ErrNoSignal
	}
	if a.Requests > 0 {
		a.ErrorRate = float64(a.Errors) / float64(a.Requests)
	}
	return a, nil
}
