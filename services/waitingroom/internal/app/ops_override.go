package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Loi cua OpsOverride. Handler doi chung sang problem type cua contracts 3.2.
var (
	// ErrInvalidOverride: rate khong phai so nguyen 0..MaxOverrideRate hoac reason sai do dai.
	ErrInvalidOverride = errors.New("lenh ghi de khong hop le")
	// ErrAuditUnavailable: khong co kho audit (DATABASE_URL thieu/hong). Fail-closed: khong ap dung gi.
	ErrAuditUnavailable = errors.New("khong co kho audit")
)

// Gioi han dau vao (contracts 3.2, AC-9).
const (
	MaxOverrideRate = 10000
	minReasonLen    = 3
	maxReasonLen    = 200
)

// Hanh dong ghi vao audit_logs.action.
const (
	ActionOverrideSet     = "ADMIT_OVERRIDE_SET"
	ActionPaused          = "ADMIT_PAUSED"
	ActionOverrideCleared = "ADMIT_OVERRIDE_CLEARED"
)

// Ket qua audit trong phan hoi.
const (
	AuditRecorded = "recorded"
	AuditFailed   = "failed"
)

// Thoi gian toi da cho lenh ghi Redis va cho cau INSERT audit (contracts 3.4).
const (
	overrideApplyTimeout = 1 * time.Second
	auditTimeout         = 2 * time.Second
)

// OverrideStore la phan Redis ma OpsOverride can.
type OverrideStore interface {
	RoomConfigured(ctx context.Context, eventID string) (bool, error)
	RateOverride(ctx context.Context, eventID string) (rate float64, active bool, err error)
	SetRateOverride(ctx context.Context, eventID string, ratePerSec float64) error
	ClearRateOverride(ctx context.Context, eventID string) error
}

// AuditEntry la mot dong audit_logs.
type AuditEntry struct {
	ActorType string
	ActorID   string
	Action    string
	Subject   string
	Reason    string
	Metadata  map[string]any
}

// Auditor ghi audit_logs.
type Auditor interface {
	Record(ctx context.Context, e AuditEntry) error
}

// OverrideCommand la mot lenh cua Ops da qua xac thuc danh tinh.
type OverrideCommand struct {
	ActorID   string
	EventID   string
	Reason    string
	RequestID string
}

// OverrideResult la trang thai sau khi ap dung (contracts 3.2/3.3).
type OverrideResult struct {
	Active     bool
	RatePerSec *float64
	Paused     bool
	AppliedAt  time.Time
	Audit      string
}

// OpsOverride la use case EVF-60: Ops dat / bo ghi de admit rate, moi lan ap dung deu duoc audit.
//
// Thu tu: kiem tra -> ap dung tren Redis -> ghi audit (D2). Audit hong KHONG hoan tac lenh: mot lenh
// tam dung khan cap khong duoc phu thuoc vao viec Postgres con khoe; thay vao do phan hoi bao
// audit:"failed" va ghi mot dong log ERROR du thong tin de doi chieu sau.
type OpsOverride struct {
	store   OverrideStore
	auditor Auditor
	log     *slog.Logger
	now     func() time.Time
}

// NewOpsOverride. auditor nil = khong co kho audit: moi lenh ghi tra ErrAuditUnavailable (AC-12).
func NewOpsOverride(store OverrideStore, auditor Auditor, log *slog.Logger) *OpsOverride {
	if log == nil {
		log = slog.Default()
	}
	return &OpsOverride{store: store, auditor: auditor, log: log, now: time.Now}
}

// ValidRate: so nguyen huu han 0..MaxOverrideRate. So le (vd 0.3) bi tu choi vi nhip < 0.5 cho lo admit 0
// trong khi paused=false (CR-1).
func ValidRate(rate float64) bool {
	return !math.IsNaN(rate) && !math.IsInf(rate, 0) && rate >= 0 && rate <= MaxOverrideRate && rate == math.Trunc(rate)
}

// zeroWidth: ky tu khong hien thi (U+200B..U+200D, U+FEFF) khong duoc tinh vao do dai toi thieu.
func zeroWidth(r rune) bool {
	return (r >= 0x200B && r <= 0x200D) || r == 0xFEFF
}

// NormalizeReason cat khoang trang Unicode va ky tu zero-width hai dau, tu choi ky tu dieu khien
// (vd NUL lam INSERT audit hong, R-1) va kiem do dai: it nhat 3 ky tu hien thi, toi da 200 ky tu (X-4).
func NormalizeReason(reason string) (string, bool) {
	r := strings.TrimFunc(reason, func(c rune) bool { return unicode.IsSpace(c) || zeroWidth(c) })
	if !utf8.ValidString(r) {
		return r, false
	}
	visible := 0
	for _, c := range r {
		if unicode.IsControl(c) {
			return r, false
		}
		if !zeroWidth(c) {
			visible++
		}
	}
	return r, visible >= minReasonLen && utf8.RuneCountInString(r) <= maxReasonLen
}

// Set dat ghi de nhip admit = rate (nguoi/giay). rate 0 = tam dung khan cap.
func (o *OpsOverride) Set(ctx context.Context, cmd OverrideCommand, rate float64) (OverrideResult, error) {
	reason, ok := NormalizeReason(cmd.Reason)
	if !ok || !ValidRate(rate) {
		return OverrideResult{}, ErrInvalidOverride
	}
	cmd.Reason = reason
	prev, err := o.precheck(ctx, cmd.EventID)
	if err != nil {
		return OverrideResult{}, err
	}

	// Lenh ghi va audit chay tren context tach khoi request: client ngat ket noi sau khi lenh da
	// ap dung khong duoc lam mat dong audit.
	detached := context.WithoutCancel(ctx)
	if err := o.withTimeout(detached, overrideApplyTimeout, func(c context.Context) error {
		return o.store.SetRateOverride(c, cmd.EventID, rate)
	}); err != nil {
		return OverrideResult{}, fmt.Errorf("ap dung ghi de: %w", err)
	}
	appliedAt := o.now().UTC()

	// Phan hoi dung trang thai da luu doc lai (override_rate/1000), khong lap lai request. Neu mot lenh bo ghi de
	// khac chen vao ngay sau HSET thi bao dung trang thai do (active:false), X-3.
	stored, storedActive := rate, true
	var readErr error
	_ = o.withTimeout(detached, overrideApplyTimeout, func(c context.Context) error {
		r, active, err := o.store.RateOverride(c, cmd.EventID)
		if err != nil {
			readErr = err
			return nil
		}
		storedActive = active
		if active {
			stored = r
		}
		return nil
	})
	if readErr != nil {
		o.log.Warn("doc lai ghi de sau khi ap dung that bai, dung gia tri da gui", "event", cmd.EventID, "err", readErr)
	}

	res := OverrideResult{Active: false, AppliedAt: appliedAt}
	auditRate := rate // lenh da ap dung; bi bo ngay sau thi audit van ghi nhip da dat
	if storedActive {
		res = OverrideResult{Active: true, RatePerSec: &stored, Paused: stored == 0, AppliedAt: appliedAt}
		auditRate = stored
	}
	action := ActionOverrideSet
	if auditRate == 0 {
		action = ActionPaused
	}
	res.Audit = o.audit(detached, cmd, action, map[string]any{
		"rate_per_sec": auditRate,
		"previous":     prev,
		"request_id":   cmd.RequestID,
	})
	return res, nil
}

// Clear bo ghi de, tra quyen cho AIMD. Idempotent: bo khi khong co ghi de cung thanh cong (va duoc audit).
func (o *OpsOverride) Clear(ctx context.Context, cmd OverrideCommand) (OverrideResult, error) {
	reason, ok := NormalizeReason(cmd.Reason)
	if !ok {
		return OverrideResult{}, ErrInvalidOverride
	}
	cmd.Reason = reason
	prev, err := o.precheck(ctx, cmd.EventID)
	if err != nil {
		return OverrideResult{}, err
	}

	detached := context.WithoutCancel(ctx)
	if err := o.withTimeout(detached, overrideApplyTimeout, func(c context.Context) error {
		return o.store.ClearRateOverride(c, cmd.EventID)
	}); err != nil {
		return OverrideResult{}, fmt.Errorf("bo ghi de: %w", err)
	}
	res := OverrideResult{Active: false, AppliedAt: o.now().UTC()}
	res.Audit = o.audit(detached, cmd, ActionOverrideCleared, map[string]any{
		"rate_per_sec": nil,
		"previous":     prev,
		"request_id":   cmd.RequestID,
	})
	return res, nil
}

// previousOverride la trang thai truoc lenh, ghi vao metadata.previous.
type previousOverride struct {
	Active     bool     `json:"active"`
	RatePerSec *float64 `json:"rate_per_sec"`
}

// precheck: kho audit phai co TRUOC moi lenh ghi (AC-12), phong phai da cau hinh (404), roi doc trang
// thai truoc do cho audit.
func (o *OpsOverride) precheck(ctx context.Context, eventID string) (previousOverride, error) {
	if o.auditor == nil {
		return previousOverride{}, ErrAuditUnavailable
	}
	configured, err := o.store.RoomConfigured(ctx, eventID)
	if err != nil {
		return previousOverride{}, fmt.Errorf("doc cau hinh phong: %w", err)
	}
	if !configured {
		return previousOverride{}, ErrRoomNotConfigured
	}
	rate, active, err := o.store.RateOverride(ctx, eventID)
	if err != nil {
		return previousOverride{}, fmt.Errorf("doc ghi de hien tai: %w", err)
	}
	prev := previousOverride{Active: active}
	if active {
		prev.RatePerSec = &rate
	}
	return prev, nil
}

func (o *OpsOverride) withTimeout(ctx context.Context, d time.Duration, f func(context.Context) error) error {
	c, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	return f(c)
}

// audit ghi mot dong audit_logs. Loi -> AuditFailed + mot dong log ERROR mang du truong (D2).
func (o *OpsOverride) audit(ctx context.Context, cmd OverrideCommand, action string, meta map[string]any) string {
	entry := AuditEntry{
		ActorType: "human",
		ActorID:   cmd.ActorID,
		Action:    action,
		Subject:   "event:" + cmd.EventID,
		Reason:    cmd.Reason,
		Metadata:  meta,
	}
	err := o.withTimeout(ctx, auditTimeout, func(c context.Context) (err error) {
		// Panic trong auditor (driver, typed-nil) sau khi lenh da ap dung: coi la audit hong, khong de no
		// thanh 500 trong khi ghi de dang song (X-2).
		defer func() {
			if p := recover(); p != nil {
				err = fmt.Errorf("auditor panic: %v", p)
			}
		}()
		return o.auditor.Record(c, entry)
	})
	if err == nil {
		return AuditRecorded
	}
	metaJSON, _ := json.Marshal(meta)
	o.log.Error("da ap dung ghi de admit rate nhung ghi audit that bai",
		"actor_id", entry.ActorID,
		"action", entry.Action,
		"subject", entry.Subject,
		"reason", entry.Reason,
		"metadata", string(metaJSON),
		"err", err)
	return AuditFailed
}
