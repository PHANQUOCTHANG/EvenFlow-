//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/pii"
	"github.com/eventflow/eventflow/services/identity/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/identity/internal/config"
	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/google/uuid"
)

func protector(t *testing.T) *pii.Protector {
	t.Helper()
	p, err := pii.NewProtector(config.DevPIIEncKey, config.DevPIIHashKey)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// uniquePhone sinh so di dong hop le, khac nhau giua cac lan chay (DB dung chung trong tien trinh).
func uniquePhone() string {
	n := uuid.New().ID() % 100_000_000
	return "09" + strings.Repeat("0", 8-len(itoa(n))) + itoa(n)
}

func itoa(n uint32) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func rawPhoneColumns(t *testing.T, db *sql.DB, id uuid.UUID) (hash string, enc []byte) {
	t.Helper()
	if err := db.QueryRow(`SELECT phone_hash, phone_enc FROM identities WHERE id = $1`, id).Scan(&hash, &enc); err != nil {
		t.Fatal(err)
	}
	return hash, enc
}

func TestIdentityRepoPII_KhongLuuSoDienThoaiTho(t *testing.T) {
	db := openDB(t)
	p := protector(t)
	repo := postgres.NewIdentityRepo(db, p)
	ctx := context.Background()

	local := uniquePhone()                       // 09xxxxxxxx
	normalized, _ := pii.NormalizePhoneVN(local) // 849xxxxxxxx
	ident := &domain.Identity{ID: uuid.New(), Phone: &normalized, PasswordHash: "x", Role: domain.RoleBuyer, CreatedAt: time.Now()}
	if err := repo.Save(ctx, ident); err != nil {
		t.Fatal(err)
	}

	hash, enc := rawPhoneColumns(t, db, ident.ID)
	if len(hash) != 64 || strings.Contains(hash, normalized[2:]) {
		t.Fatalf("phone_hash phai la HMAC 64 hex, nhan %q", hash)
	}
	if strings.Contains(string(enc), normalized[2:]) {
		t.Fatal("phone_enc chua so dien thoai tho")
	}
	if got, err := p.Decrypt(enc, "identities.phone_enc:"+ident.ID.String()); err != nil || got != normalized {
		t.Fatalf("giai ma phone_enc = %q %v", got, err)
	}

	// Tra cuu bang ca hai dang nguoi dung hay go.
	for _, in := range []string{local, normalized, "+84 " + normalized[2:]} {
		got, err := repo.FindByEmailOrPhone(ctx, in)
		if err != nil || got.ID != ident.ID {
			t.Fatalf("tim theo %q: %v %v", in, got, err)
		}
	}

	// Cung so, dang khac -> trung (UNIQUE tren HMAC cua dang chuan hoa).
	dup := &domain.Identity{ID: uuid.New(), Phone: &normalized, PasswordHash: "x", Role: domain.RoleBuyer, CreatedAt: time.Now()}
	if err := repo.Save(ctx, dup); err != domain.ErrIdentityExists {
		t.Fatalf("dang ky lai cung so phai ErrIdentityExists, nhan %v", err)
	}
}

func TestIdentityRepoPII_ChuyenDuLieuCuVaIdempotent(t *testing.T) {
	db := openDB(t)
	p := protector(t)
	repo := postgres.NewIdentityRepo(db, p)
	ctx := context.Background()

	// Dong cu: SDT tho trong phone_hash (hanh vi truoc EV-115). Hai dong cung mot so o hai dang.
	local := uniquePhone()
	normalized, _ := pii.NormalizePhoneVN(local)
	legacyA, legacyB := uuid.New(), uuid.New()
	for id, raw := range map[uuid.UUID]string{legacyA: local, legacyB: normalized} {
		if _, err := db.Exec(`INSERT INTO identities (id, phone_hash, password_hash) VALUES ($1, $2, 'x')`, id, raw); err != nil {
			t.Fatal(err)
		}
	}

	done, skipped, err := repo.ProtectLegacyPhones(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if done < 1 || skipped < 1 {
		t.Fatalf("done=%d skipped=%d, muon >=1 va >=1 (dong trung bi bo qua)", done, skipped)
	}

	converted := 0
	for _, id := range []uuid.UUID{legacyA, legacyB} {
		hash, enc := rawPhoneColumns(t, db, id)
		if len(hash) == 64 {
			converted++
			if got, err := p.Decrypt(enc, "identities.phone_enc:"+id.String()); err != nil || got != normalized {
				t.Fatalf("dong da chuyen giai ma = %q %v", got, err)
			}
		}
	}
	if converted != 1 {
		t.Fatalf("dung 1 trong 2 dong trung duoc chuyen, nhan %d", converted)
	}
	if got, err := repo.FindByEmailOrPhone(ctx, local); err != nil || (got.ID != legacyA && got.ID != legacyB) {
		t.Fatalf("sau khi chuyen van phai dang nhap duoc bang SDT: %v %v", got, err)
	}

	// Chay lai: khong doi gi them ngoai dong trung con lai.
	done2, _, err := repo.ProtectLegacyPhones(ctx)
	if err != nil || done2 != 0 {
		t.Fatalf("chay lai phai idempotent: done=%d err=%v", done2, err)
	}
}

// EV-111 (BR-Q3): Save ghi otp_verified_at; migration 0004 backfill dong cu bang created_at.
func TestIdentityRepo_GhiOtpVerifiedAt(t *testing.T) {
	db := openDB(t)
	repo := postgres.NewIdentityRepo(db, protector(t))
	ctx := context.Background()
	email := uuid.NewString() + "@test.local"
	at := time.Now().UTC().Truncate(time.Microsecond)

	ident := &domain.Identity{ID: uuid.New(), Email: &email, PasswordHash: "x", Role: domain.RoleBuyer, CreatedAt: at, OtpVerifiedAt: &at}
	if err := repo.Save(ctx, ident); err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindByEmailOrPhone(ctx, email)
	if err != nil || got.OtpVerifiedAt == nil || !got.OtpVerifiedAt.Equal(at) {
		t.Fatalf("otp_verified_at = %v, %v; muon %v", got, err, at)
	}

	// Dong cu chua co gia tri + chay lai cau backfill cua migration.
	oldID := uuid.New()
	if _, err := db.Exec(`INSERT INTO identities (id, email, password_hash, created_at) VALUES ($1, $2, 'x', $3)`,
		oldID, uuid.NewString()+"@old.local", at.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE identities SET otp_verified_at = created_at WHERE otp_verified_at IS NULL`); err != nil {
		t.Fatal(err)
	}
	var verified sql.NullTime
	if err := db.QueryRow(`SELECT otp_verified_at FROM identities WHERE id = $1`, oldID).Scan(&verified); err != nil || !verified.Valid {
		t.Fatalf("backfill khong ghi: %v %v", verified, err)
	}
}
