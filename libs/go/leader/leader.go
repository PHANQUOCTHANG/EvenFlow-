// Package leader bau mot leader duy nhat cho moi khoa bang Redis lease, kem fencing token.
//
// Khoa chi la nua bai toan. Leader cu bi "dung hinh" (GC pause, CPU throttle, mang chap chon)
// qua han lease roi tinh lai, va khong biet minh da mat khoa; kiem tra "con giu khoa khong" truoc
// moi lenh ghi cung khong cuu duoc, vi no co the dung hinh ngay SAU khi kiem tra. Loi giai chuan
// la fencing token (Kleppmann, "How to do distributed locking"): moi lan lay khoa duoc cap mot
// epoch tang dan, va NOI NHAN LENH GHI tu choi epoch cu.
//
// Package nay cap epoch va giu lease. Viec tu choi epoch cu la cua noi ghi, trong CUNG mot lenh
// nguyen tu voi lenh ghi -- xem mau fencing o duoi.
//
// Gioi han can biet: lease nam tren mot Redis. Neu primary sap truoc khi ban sao nhan khoa hoac
// epoch (replication bat dong bo), co the co vai giay hai leader. Fencing van chan duoc leader cu
// ngay khi no cham vao epoch moi; khoang cua so con lai chi la mot nhom lenh ghi cung luc.
// Muon het cua so do phai dung he dong thuan (etcd, Consul).
package leader

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrFenced la loi noi ghi tra ve khi epoch cua leader da cu. Leader gap loi nay phai thoai vi
// ngay (tra ve tu fn cua Run), khong duoc thu lai.
var ErrFenced = errors.New("leader: epoch da cu, bi fencing chan")

// Mau fencing cho noi ghi: dat o DAU moi script ghi cua leader, truoc moi lenh ghi.
// KEYS[1] la epoch key, ARGV[1] la epoch cua nguoi goi:
//
//	if tonumber(redis.call('GET', KEYS[1]) or '0') ~= tonumber(ARGV[1]) then
//	  return redis.error_reply('FENCED')
//	end
//
// Kiem va ghi nam trong cung mot script nen khong co khe ho nao de leader cu chen vao.

// IsFenced nhan ra loi FENCED do script tra ve (hoac ErrFenced da boc).
func IsFenced(err error) bool {
	return err != nil && (errors.Is(err, ErrFenced) || strings.Contains(err.Error(), "FENCED"))
}

// Config cau hinh mot Elector.
type Config struct {
	// LockKey giu ten instance dang la leader (SET NX PX).
	LockKey string
	// EpochKey dem so lan doi leader. KHONG co TTL: epoch phai tang don dieu ke ca khi khoa het han.
	EpochKey string
	// ID dinh danh instance. Phai duy nhat moi tien trinh (hostname + pid + ngau nhien).
	ID string
	// TTL la thoi han lease. Leader chet dot ngot thi nguoi khac len sau toi da TTL.
	TTL time.Duration
	// RenewEvery la chu ky gia han va chu ky thu lay khoa cua instance chua phai leader.
	RenewEvery time.Duration
}

func (c Config) validate() error {
	switch {
	case c.LockKey == "" || c.EpochKey == "":
		return errors.New("leader: thieu LockKey/EpochKey")
	case c.ID == "":
		return errors.New("leader: thieu ID")
	case c.TTL <= 0 || c.RenewEvery <= 0:
		return errors.New("leader: TTL va RenewEvery phai > 0")
	case c.RenewEvery*2 > c.TTL:
		// Gia han it nhat hai lan moi lease, neu khong mot lan mat goi la mat khoa.
		return fmt.Errorf("leader: RenewEvery (%v) phai <= TTL/2 (%v)", c.RenewEvery, c.TTL/2)
	}
	return nil
}

// store la phan Redis ma vong doi leader can; tach ra de test vong doi bang fake.
type store interface {
	// acquire tra ve epoch moi (> 0) neu lay duoc khoa, 0 neu khoa dang do nguoi khac giu.
	acquire(ctx context.Context, id string, ttl time.Duration) (int64, error)
	// renew tra ve false neu khoa khong con la cua id.
	renew(ctx context.Context, id string, ttl time.Duration) (bool, error)
	release(ctx context.Context, id string) error
}

// Elector chay vong doi leader cho mot khoa.
type Elector struct {
	cfg Config
	st  store
	log *slog.Logger
	now func() time.Time
}

// New tao Elector chay tren Redis.
func New(rdb redis.Scripter, cfg Config, log *slog.Logger) (*Elector, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if log == nil {
		log = slog.Default()
	}
	return &Elector{cfg: cfg, st: &redisStore{rdb: rdb, lock: cfg.LockKey, epoch: cfg.EpochKey}, log: log, now: time.Now}, nil
}

// Run chay cho toi khi ctx bi huy. Moi lan tro thanh leader, Run goi fn(ctx, epoch); ctx cua fn
// bi huy ngay khi mat quyen leader. fn tra ve (vi het viec, hoac bi fencing) thi Run nha khoa va
// quay lai tranh khoa -- nen mot instance co the lam leader nhieu nhiem ky.
//
// fn PHAI ton trong ctx va tra ve som: Run cho fn tra ve truoc khi thu lay khoa lai, de khong bao
// gio co hai nhiem ky cua cung mot instance chay chong len nhau.
func (e *Elector) Run(ctx context.Context, fn func(ctx context.Context, epoch int64) error) error {
	t := time.NewTicker(e.cfg.RenewEvery)
	defer t.Stop()

	for {
		epoch, err := e.st.acquire(ctx, e.cfg.ID, e.cfg.TTL)
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case err != nil:
			e.log.Warn("leader: khong lay duoc khoa", "key", e.cfg.LockKey, "err", err)
		case epoch > 0:
			e.log.Info("leader: tro thanh leader", "key", e.cfg.LockKey, "epoch", epoch, "id", e.cfg.ID)
			e.lead(ctx, epoch, fn)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Het nhiem ky: cho mot chu ky roi moi tranh lai. Khong cho thi fn that bai ngay
			// (cau hinh sai, phu thuoc chet) se bien thanh vong lay-nha khoa chay nong.
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// lead giu lease va chay fn cho toi khi mat quyen hoac fn xong.
func (e *Elector) lead(parent context.Context, epoch int64, fn func(context.Context, int64) error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- fn(ctx, epoch) }()

	lastOK := e.now()
	t := time.NewTicker(e.cfg.RenewEvery)
	defer t.Stop()

	stop := func(reason string) {
		e.log.Warn("leader: thoai vi", "key", e.cfg.LockKey, "epoch", epoch, "ly_do", reason)
		cancel()
		<-done
	}

	for {
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				e.log.Warn("leader: viec cua leader ket thuc voi loi", "key", e.cfg.LockKey, "epoch", epoch, "err", err)
			}
			e.releaseLock()
			return
		case <-parent.Done():
			cancel()
			<-done
			e.releaseLock()
			return
		case <-t.C:
			ok, err := e.st.renew(ctx, e.cfg.ID, e.cfg.TTL)
			switch {
			case err == nil && ok:
				lastOK = e.now()
			case err == nil && !ok:
				// Khoa da thuoc nguoi khac: khong con la leader, KHONG nha khoa (khong phai cua minh).
				stop("khoa da bi nguoi khac giu")
				return
			default:
				// Redis loi: chua biet mat khoa chua. Chi thoai vi khi lease co the da het han o
				// phia server -- sau thoi diem do nguoi khac co the da len, nen khong duoc tiep tuc.
				if e.now().Sub(lastOK) >= e.cfg.TTL-e.cfg.RenewEvery {
					stop("khong gia han duoc, lease co the da het han")
					return
				}
			}
		}
	}
}

func (e *Elector) releaseLock() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := e.st.release(ctx, e.cfg.ID); err != nil {
		// Khong nha duoc thi lease tu het han sau TTL: cham hon nhung van an toan.
		e.log.Warn("leader: nha khoa that bai, cho lease het han", "key", e.cfg.LockKey, "err", err)
	}
}
