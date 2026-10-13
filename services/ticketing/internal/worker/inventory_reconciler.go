package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// InventoryReconciler so ton kho Redis voi Postgres moi chu ky va sua phan lech (EV-132, BR-O1).
//
// Redis chi la tang chan nhanh: khi no lech (failover, mat du lieu, timeout lam mat mot lan tra
// kho) he thong khong oversell vi Postgres con CHECK (available >= 0), nhung se BAO HET VE SOM
// hoac chan khach vo ich. Job nay dua hai tang ve khop nhau.
//
// # Kho "ky vong" cua Redis
//
// Redis tru kho TRUOC, Postgres commit SAU (va nguoc lai khi tra kho), nen o bat ky thoi diem nao
// luon co hold "dang bay" lam hai so hop le nhung khac nhau. Doi soat ngay thang `Redis = Postgres`
// se pha chinh cac hold nay. Thay vao do dung hash hold cua Redis lam so cai:
//
//	ky_vong(bucket) = Postgres.available - tong so luong cac hold dang nam trong hash Redis
//	                  ma Postgres CHUA ghi, hoac DA tra kho
//
// Hold Redis da tru ma Postgres chua commit: Redis thap hon Postgres dung bang so luong do.
// Hold Postgres da tra kho nhung Redis chua kip release: tuong tu. Hai truong hop do KHONG phai
// lech. Phan con lai moi la lech that: `Redis - ky_vong`.
//
// # An toan khi sua
//
//   - Mot do lech chi duoc sua khi thay o HAI chu ky lien tiep cung dau; so sua la gia tri nho
//     hon trong hai lan. Mot nhieu thoang qua (doc Redis va Postgres lech nhau vai ms) khong bao
//     gio thanh phep sua.
//   - Sua bang DELTA (HINCRBY), khong SET: giao hoan voi hold dang tru kho dong thoi. Neu lan sua
//     sai vi mot nhieu hiem gap, lan sau no hien ra nhu mot do lech nguoc va tu duoc sua lai.
//   - Redis phong (drain) la huong an toan (Postgres van chan); Redis thieu (refill) moi gay bao
//     het ve som. Ca hai deu duoc sua.
//   - Khoa theo su kien (SETNX) giu cho hai instance khong cung sua trong mot chu ky.
//
// # Khong lam gi
//
// Hold chua ghi o Postgres va qua han van duoc sweeper don khi het han (TTL): job nay khong tu y
// release hold, vi release mot hold con dang bay se lam Postgres giu cho ma Redis khong co ZSET
// de het han.
//
// Seeder (nap vao bucket chua co, 5 giay) van chay: no lam Redis dung duoc nhanh hon sau khi mat
// du lieu hoan toan; job nay lo phan con lai.
type InventoryReconciler struct {
	src     port.InventorySource
	states  port.HoldStateSource
	ledger  port.InventoryLedger
	log     *slog.Logger
	cfg     ReconcilerConfig
	metrics port.ReconcilerMetrics

	// seen: do lech cua chu ky truoc, theo bucket. Chi giu bucket con lech.
	seen map[bucketKey]int
}

type bucketKey struct {
	event, ticketType string
	bucket            int
}

// ReconcilerConfig cau hinh job doi soat.
type ReconcilerConfig struct {
	EventIDs []string
	// Interval la chu ky doi soat (10 giay). Mot do lech that duoc sua sau toi da 2 chu ky.
	Interval time.Duration
	// LockTTL phai nho hon Interval de instance giu khoa o chu ky nay khong chan chu ky sau.
	LockTTL time.Duration
}

// DefaultReconcilerConfig dung chu ky 10 giay cua EVF-37.
func DefaultReconcilerConfig(eventIDs []string) ReconcilerConfig {
	return ReconcilerConfig{EventIDs: eventIDs, Interval: 10 * time.Second, LockTTL: 8 * time.Second}
}

type noopReconcilerMetrics struct{}

func (noopReconcilerMetrics) InventoryDrift(string, int)  {}
func (noopReconcilerMetrics) DriftCorrected(string, int) {}

// NewInventoryReconciler tao job doi soat.
func NewInventoryReconciler(src port.InventorySource, states port.HoldStateSource,
	ledger port.InventoryLedger, cfg ReconcilerConfig, log *slog.Logger) *InventoryReconciler {
	return &InventoryReconciler{
		src: src, states: states, ledger: ledger, cfg: cfg, log: log,
		metrics: noopReconcilerMetrics{},
		seen:    make(map[bucketKey]int),
	}
}

// WithMetrics gan noi nhan so lieu; mac dinh la no-op.
func (r *InventoryReconciler) WithMetrics(m port.ReconcilerMetrics) *InventoryReconciler {
	r.metrics = m
	return r
}

// Run doi soat ngay roi lap theo Interval cho toi khi ctx bi huy.
func (r *InventoryReconciler) Run(ctx context.Context) error {
	r.ReconcileAll(ctx)

	t := time.NewTicker(r.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			r.ReconcileAll(ctx)
		}
	}
}

// ReconcileAll chay mot chu ky cho moi su kien. Loi o mot su kien khong chan cac su kien con lai.
func (r *InventoryReconciler) ReconcileAll(ctx context.Context) {
	for _, ev := range r.cfg.EventIDs {
		cctx, cancel := context.WithTimeout(ctx, r.cfg.Interval)
		if err := r.Reconcile(cctx, ev); err != nil {
			r.log.Error("doi soat ton kho that bai", "event", ev, "err", err)
		}
		cancel()
	}
}

// Reconcile chay MOT chu ky doi soat cho mot su kien.
func (r *InventoryReconciler) Reconcile(ctx context.Context, eventID string) error {
	held, err := r.ledger.TryReconcileLock(ctx, eventID, r.cfg.LockTTL)
	if err != nil {
		return err
	}
	if !held {
		return nil // instance khac dang doi soat chu ky nay
	}

	rows, err := r.src.ListInventory(ctx, eventID)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		r.forget(eventID)
		return nil // su kien chua ton tai hoac khong o trang thai dang ban
	}

	ticketTypes := make([]string, 0, 4)
	seenType := make(map[string]bool)
	for _, row := range rows {
		if !seenType[row.TicketTypeID] {
			seenType[row.TicketTypeID] = true
			ticketTypes = append(ticketTypes, row.TicketTypeID)
		}
	}

	// Doc Redis TRUOC roi moi doc Postgres: hold nao co trong anh chup Redis thi Postgres da
	// (hoac sap) ghi, nen "chua ghi" chi co the la dang bay chu khong phai da mat.
	snap, err := r.ledger.Snapshot(ctx, eventID, ticketTypes)
	if err != nil {
		return err
	}
	holdIDs := make([]string, 0, len(snap.Holds))
	for id := range snap.Holds {
		holdIDs = append(holdIDs, id)
	}
	states, err := r.states.HoldStates(ctx, holdIDs)
	if err != nil {
		return err
	}

	// Phan Redis da tru nhung Postgres chua co (bay) hoac da tra (cho release).
	pending := make(map[bucketKey]int)
	for id, h := range snap.Holds {
		if st, found := states[id]; found && !st.Released {
			continue // ca hai tang cung da tru: khong gay lech
		}
		pending[bucketKey{eventID, h.TicketTypeID, h.Bucket}] += h.Quantity
	}

	next := make(map[bucketKey]int)
	driftByType := make(map[string]int, len(ticketTypes))
	for _, row := range rows {
		k := bucketKey{eventID, row.TicketTypeID, row.Bucket}
		expected := row.Available - pending[k]
		if expected < 0 {
			expected = 0
		}
		drift := snap.Inventory[row.TicketTypeID][row.Bucket] - expected

		if drift != 0 {
			if prev := r.seen[k]; sameSign(prev, drift) {
				drift = r.correct(ctx, k, prev, drift)
			}
		}
		if drift != 0 {
			next[k] = drift
			driftByType[row.TicketTypeID] += abs(drift)
		}
	}

	r.forget(eventID)
	for k, v := range next {
		r.seen[k] = v
	}
	for _, tt := range ticketTypes {
		r.metrics.InventoryDrift(tt, driftByType[tt])
	}
	return nil
}

// correct sua do lech da duoc xac nhan o hai chu ky va tra ve phan con lai sau khi sua.
func (r *InventoryReconciler) correct(ctx context.Context, k bucketKey, prev, now int) int {
	// Lay gia tri nho hon (cung dau): chi sua phan chac chan, phan thua de chu ky sau.
	amount := min(abs(prev), abs(now))
	delta, direction := amount, "refill" // Redis thieu -> cong them
	if now > 0 {
		delta, direction = -amount, "drain" // Redis phong -> bot di
	}

	after, err := r.ledger.AdjustInventory(ctx, k.event, k.ticketType, k.bucket, delta)
	if err != nil {
		r.log.Error("sua ton kho redis that bai",
			"event", k.event, "ticket_type", k.ticketType, "bucket", k.bucket, "delta", delta, "err", err)
		return now
	}
	r.metrics.DriftCorrected(direction, amount)
	r.log.Warn("da sua ton kho redis lech so voi postgres",
		"event", k.event, "ticket_type", k.ticketType, "bucket", k.bucket,
		"drift", now, "delta", delta, "redis_sau_khi_sua", after, "huong", direction)

	if now > 0 {
		return now - amount
	}
	return now + amount
}

// forget bo trang thai theo doi cua mot su kien.
func (r *InventoryReconciler) forget(eventID string) {
	for k := range r.seen {
		if k.event == eventID {
			delete(r.seen, k)
		}
	}
}

func sameSign(a, b int) bool { return (a < 0 && b < 0) || (a > 0 && b > 0) }

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
