//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/identity/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/google/uuid"
)

func deviceRows(t *testing.T, db *sql.DB, identityID uuid.UUID) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM devices WHERE identity_id = $1`, identityID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDeviceRepo_IdempotentVaChiLuuHash(t *testing.T) {
	db := openDB(t)
	repo := postgres.NewDeviceRepo(db, protector(t))
	ctx := context.Background()
	identityID := seedIdentity(t, db)

	a, err := repo.Register(ctx, identityID, "fingerprint-may-A", "203.0.113.7", 5)
	if err != nil {
		t.Fatal(err)
	}
	b, err := repo.Register(ctx, identityID, "fingerprint-may-A", "203.0.113.9", 5)
	if err != nil || a != b {
		t.Fatalf("dang ky lai cung fingerprint phai tra cung id: %v %v %v", a, b, err)
	}

	var fp, ip string
	if err := db.QueryRow(`SELECT fingerprint, host(last_ip) FROM devices WHERE id = $1`, a).Scan(&fp, &ip); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fp, "fingerprint-may-A") || len(fp) != 64 {
		t.Fatalf("cot fingerprint phai la HMAC 64 hex, nhan %q", fp)
	}
	if ip != "203.0.113.9" {
		t.Fatalf("last_ip = %q, muon 203.0.113.9", ip)
	}
	// IP khong hop le khong ghi de last_ip va khong gay loi.
	if _, err := repo.Register(ctx, identityID, "fingerprint-may-A", "khong-phai-ip", 5); err != nil {
		t.Fatal(err)
	}
	_ = db.QueryRow(`SELECT host(last_ip) FROM devices WHERE id = $1`, a).Scan(&ip)
	if ip != "203.0.113.9" {
		t.Fatalf("ip hong lam mat last_ip: %q", ip)
	}
}

// Nhieu dang nhap dong thoi tren thiet bi moi: dung toi da `max` thiet bi duoc luu.
func TestDeviceRepo_GioiHanDungDuoiDongThoi(t *testing.T) {
	db := openDB(t)
	repo := postgres.NewDeviceRepo(db, protector(t))
	ctx := context.Background()
	identityID := seedIdentity(t, db)

	const attempts, max = 20, 5
	var (
		wg          sync.WaitGroup
		mu          sync.Mutex
		ok, tooMany int
	)
	start := make(chan struct{})
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := repo.Register(ctx, identityID, "fingerprint-"+uuid.NewString(), "", max)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, domain.ErrTooManyDevices):
				tooMany++
			default:
				t.Errorf("loi ngoai du kien: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	rows := deviceRows(t, db, identityID)
	if ok != max || tooMany != attempts-max || rows != max {
		t.Fatalf("ok=%d tooMany=%d hang=%d, muon %d/%d/%d", ok, tooMany, rows, max, attempts-max, max)
	}
	if _, err := repo.Register(ctx, identityID, "fingerprint-moi-khi-da-du", "", max); !errors.Is(err, domain.ErrTooManyDevices) {
		t.Fatalf("thiet bi moi khi da du phai bi tu choi, nhan %v", err)
	}
}

func TestRefreshTokenRepo_ThietBiTheoPhien(t *testing.T) {
	db := openDB(t)
	devices := postgres.NewDeviceRepo(db, protector(t))
	tokens := postgres.NewRefreshTokenRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	identityID := seedIdentity(t, db)

	dev, err := devices.Register(ctx, identityID, "fingerprint-may-A", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := newToken(identityID, uuid.New(), now)
	first.DeviceID = &dev
	if err := tokens.Create(ctx, first); err != nil {
		t.Fatal(err)
	}

	// Xoay: token moi giu thiet bi.
	next, _ := newToken(uuid.Nil, uuid.Nil, now)
	old, err := tokens.Rotate(ctx, first.TokenHash, next, now)
	if err != nil || old.DeviceID == nil || *old.DeviceID != dev || next.DeviceID == nil || *next.DeviceID != dev {
		t.Fatalf("xoay phai giu thiet bi: old=%v next=%v err=%v", old.DeviceID, next.DeviceID, err)
	}

	// Doi thiet bi cua phien, roi xoay tiep: thiet bi moi duoc giu.
	dev2, _ := devices.Register(ctx, identityID, "fingerprint-may-B", "", 5)
	if err := tokens.SetDevice(ctx, next.ID, dev2); err != nil {
		t.Fatal(err)
	}
	third, _ := newToken(uuid.Nil, uuid.Nil, now)
	if _, err := tokens.Rotate(ctx, next.TokenHash, third, now); err != nil || third.DeviceID == nil || *third.DeviceID != dev2 {
		t.Fatalf("sau SetDevice phai xoay voi thiet bi moi: %v %v", third.DeviceID, err)
	}

	// Phien khong gan thiet bi: DeviceID nil.
	bare, _ := newToken(identityID, uuid.New(), now)
	_ = tokens.Create(ctx, bare)
	nb, _ := newToken(uuid.Nil, uuid.Nil, now)
	if old, err := tokens.Rotate(ctx, bare.TokenHash, nb, now); err != nil || old.DeviceID != nil || nb.DeviceID != nil {
		t.Fatalf("phien khong thiet bi: old=%v next=%v err=%v", old.DeviceID, nb.DeviceID, err)
	}
}
