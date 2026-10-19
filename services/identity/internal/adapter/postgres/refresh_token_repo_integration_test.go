//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/testx"
	"github.com/eventflow/eventflow/services/identity/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("postgres", testx.Postgres(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedIdentity(t *testing.T, db *sql.DB) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := db.Exec(`INSERT INTO identities (id, email, password_hash) VALUES ($1, $2, 'x')`,
		id, id.String()+"@test.local"); err != nil {
		t.Fatal(err)
	}
	return id
}

func newToken(identityID, family uuid.UUID, now time.Time) (*domain.RefreshToken, string) {
	raw, hash, _ := domain.NewRefreshTokenValue()
	return &domain.RefreshToken{
		ID: uuid.New(), IdentityID: identityID, FamilyID: family, TokenHash: hash,
		ExpiresAt: now.Add(domain.RefreshTokenTTL), CreatedAt: now,
	}, raw
}

func liveInFamily(t *testing.T, db *sql.DB, family uuid.UUID) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM refresh_tokens WHERE family_id = $1 AND revoked_at IS NULL`, family).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Hai tab (hoac ke trom va chu that) cung dua MOT refresh token len cung luc: dung mot request
// thang, moi request con lai bi coi la tai su dung va ca ho bi thu hoi.
func TestRefreshTokenRepo_RotateDongThoiChiMotRequestThang(t *testing.T) {
	db := openDB(t)
	repo := postgres.NewRefreshTokenRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	identityID := seedIdentity(t, db)
	family := uuid.New()

	first, _ := newToken(identityID, family, now)
	if err := repo.Create(ctx, first); err != nil {
		t.Fatal(err)
	}

	const n = 20
	var (
		wg              sync.WaitGroup
		mu              sync.Mutex
		ok, reused, bad int
	)
	start := make(chan struct{})
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			next, _ := newToken(uuid.Nil, uuid.Nil, now)
			_, err := repo.Rotate(ctx, first.TokenHash, next, now)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, domain.ErrRefreshTokenReused):
				reused++
			default:
				bad++
				t.Errorf("loi ngoai du kien: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if ok != 1 || reused != n-1 || bad != 0 {
		t.Fatalf("ok=%d reused=%d bad=%d, muon ok=1 reused=%d", ok, reused, bad, n-1)
	}
	if live := liveInFamily(t, db, family); live != 0 {
		t.Fatalf("sau khi phat hien tai su dung, ho phai bi thu hoi het; con %d token song", live)
	}
	var reason string
	if err := db.QueryRow(`SELECT revoked_reason FROM refresh_tokens WHERE id = $1`, first.ID).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != domain.RevokeReasonRotated {
		t.Fatalf("token goc phai ghi ly do rotated, nhan %q", reason)
	}
}

func TestRefreshTokenRepo_RotateGiuHoVaHanVaGhiReplacedBy(t *testing.T) {
	db := openDB(t)
	repo := postgres.NewRefreshTokenRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	identityID := seedIdentity(t, db)
	family := uuid.New()

	first, _ := newToken(identityID, family, now)
	_ = repo.Create(ctx, first)
	next, _ := newToken(uuid.Nil, uuid.Nil, now.Add(time.Minute))

	old, err := repo.Rotate(ctx, first.TokenHash, next, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if old.ID != first.ID || next.FamilyID != family || next.IdentityID != identityID || !next.ExpiresAt.Equal(first.ExpiresAt) {
		t.Fatalf("token moi phai cung ho/identity/han: %+v", next)
	}
	var replacedBy uuid.UUID
	if err := db.QueryRow(`SELECT replaced_by FROM refresh_tokens WHERE id = $1`, first.ID).Scan(&replacedBy); err != nil {
		t.Fatal(err)
	}
	if replacedBy != next.ID {
		t.Fatalf("replaced_by = %v, muon %v", replacedBy, next.ID)
	}
	if live := liveInFamily(t, db, family); live != 1 {
		t.Fatalf("sau mot lan xoay ho phai con dung 1 token song, con %d", live)
	}
}

func TestRefreshTokenRepo_HetHanKhongXoayVaKhongThuHoiHo(t *testing.T) {
	db := openDB(t)
	repo := postgres.NewRefreshTokenRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	identityID := seedIdentity(t, db)

	tok, _ := newToken(identityID, uuid.New(), now)
	_ = repo.Create(ctx, tok)
	next, _ := newToken(uuid.Nil, uuid.Nil, now)

	if _, err := repo.Rotate(ctx, tok.TokenHash, next, tok.ExpiresAt); !errors.Is(err, domain.ErrRefreshTokenInvalid) {
		t.Fatalf("token het han phai tra ErrRefreshTokenInvalid, nhan %v", err)
	}
	if _, err := repo.Rotate(ctx, "khong-ton-tai", next, now); !errors.Is(err, domain.ErrRefreshTokenInvalid) {
		t.Fatalf("token la phai tra ErrRefreshTokenInvalid, nhan %v", err)
	}
}

func TestRefreshTokenRepo_LogoutVaLogoutAll(t *testing.T) {
	db := openDB(t)
	repo := postgres.NewRefreshTokenRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	identityID := seedIdentity(t, db)
	famA, famB := uuid.New(), uuid.New()

	a, _ := newToken(identityID, famA, now)
	b, _ := newToken(identityID, famB, now)
	_ = repo.Create(ctx, a)
	_ = repo.Create(ctx, b)

	if err := repo.RevokeFamilyByHash(ctx, a.TokenHash, domain.RevokeReasonLogout, now); err != nil {
		t.Fatal(err)
	}
	if liveInFamily(t, db, famA) != 0 || liveInFamily(t, db, famB) != 1 {
		t.Fatal("logout chi duoc thu hoi ho cua token do")
	}
	if err := repo.RevokeAllForIdentity(ctx, identityID, domain.RevokeReasonLogoutAll, now); err != nil {
		t.Fatal(err)
	}
	if liveInFamily(t, db, famB) != 0 {
		t.Fatal("logout-all phai thu hoi moi ho")
	}
	// Token cua ho da logout bi dua len: coi nhu tai su dung (ho da chet san, khong doi gi).
	next, _ := newToken(uuid.Nil, uuid.Nil, now)
	if _, err := repo.Rotate(ctx, a.TokenHash, next, now); !errors.Is(err, domain.ErrRefreshTokenReused) {
		t.Fatalf("token da logout phai bi tu choi, nhan %v", err)
	}
}
