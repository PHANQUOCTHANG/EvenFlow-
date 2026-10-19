package app_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
	"github.com/eventflow/eventflow/services/identity/internal/app"
	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/google/uuid"
)

var sessionSecret = []byte("session-secret-for-tests-32bytes")

// memRefreshRepo mo phong dung ngu nghia cua refresh_token_repo (Postgres) trong bo nho.
type memRefreshRepo struct {
	mu   sync.Mutex
	rows map[string]*domain.RefreshToken // theo hash
}

func newMemRefreshRepo() *memRefreshRepo {
	return &memRefreshRepo{rows: map[string]*domain.RefreshToken{}}
}

func (m *memRefreshRepo) Create(_ context.Context, t *domain.RefreshToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *t
	m.rows[t.TokenHash] = &cp
	return nil
}

func (m *memRefreshRepo) Rotate(_ context.Context, oldHash string, next *domain.RefreshToken, now time.Time) (*domain.RefreshToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok := m.rows[oldHash]
	if !ok {
		return nil, domain.ErrRefreshTokenInvalid
	}
	if old.RevokedAt != nil {
		m.revokeFamilyLocked(old.FamilyID, now)
		cp := *old
		return &cp, domain.ErrRefreshTokenReused
	}
	if !now.Before(old.ExpiresAt) {
		return nil, domain.ErrRefreshTokenInvalid
	}
	next.IdentityID, next.FamilyID, next.ExpiresAt = old.IdentityID, old.FamilyID, old.ExpiresAt
	cp := *next
	m.rows[next.TokenHash] = &cp
	old.RevokedAt = &now
	oldCp := *old
	return &oldCp, nil
}

func (m *memRefreshRepo) RevokeFamilyByHash(_ context.Context, hash, _ string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if row, ok := m.rows[hash]; ok {
		m.revokeFamilyLocked(row.FamilyID, now)
	}
	return nil
}

func (m *memRefreshRepo) RevokeAllForIdentity(_ context.Context, id uuid.UUID, _ string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rows {
		if r.IdentityID == id && r.RevokedAt == nil {
			r.RevokedAt = &now
		}
	}
	return nil
}

func (m *memRefreshRepo) revokeFamilyLocked(family uuid.UUID, now time.Time) {
	for _, r := range m.rows {
		if r.FamilyID == family && r.RevokedAt == nil {
			r.RevokedAt = &now
		}
	}
}

type sessionFixture struct {
	svc      *app.SessionService
	repo     *memRefreshRepo
	ids      *fakeIdentityRepo
	audit    *recordingAudit
	identity *domain.Identity
	now      time.Time
}

type recordingAudit struct{ logs []*domain.AuditLog }

func (a *recordingAudit) Save(_ context.Context, l *domain.AuditLog) error {
	a.logs = append(a.logs, l)
	return nil
}

func newSessionFixture(t *testing.T) *sessionFixture {
	t.Helper()
	f := &sessionFixture{
		repo:  newMemRefreshRepo(),
		ids:   newFakeRepo(),
		audit: &recordingAudit{},
		now:   time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC),
	}
	email := "buyer@example.com"
	f.identity = &domain.Identity{ID: uuid.New(), Email: &email, Role: domain.RoleBuyer}
	f.ids.identities[email] = f.identity
	f.svc = app.NewSessionService(sessionSecret, f.ids, f.repo, f.audit).
		WithClock(func() time.Time { return f.now })
	return f
}

func TestSession_DangNhapCapAccess15PhutVaRefresh30Ngay(t *testing.T) {
	f := newSessionFixture(t)
	pair, err := f.svc.GenerateTokens(context.Background(), f.identity)
	if err != nil {
		t.Fatal(err)
	}

	claims, err := authx.VerifyAccessToken(sessionSecret, pair.AccessToken, f.now)
	if err != nil {
		t.Fatalf("access token phai hop le: %v", err)
	}
	if claims.Subject != f.identity.ID.String() || claims.Role != "buyer" {
		t.Fatalf("claims sai: %+v", claims)
	}
	if got := pair.AccessExpiresAt.Sub(f.now); got != 15*time.Minute {
		t.Fatalf("access song %v, muon 15 phut", got)
	}
	if got := pair.RefreshExpiresAt.Sub(f.now); got != 30*24*time.Hour {
		t.Fatalf("refresh song %v, muon 30 ngay", got)
	}
	if _, err := authx.VerifyAccessToken(sessionSecret, pair.AccessToken, f.now.Add(16*time.Minute)); !errors.Is(err, authx.ErrExpiredAccessToken) {
		t.Fatalf("sau 16 phut access token phai het han, nhan %v", err)
	}
	// Chi luu hash, khong luu token tho.
	if _, ok := f.repo.rows[pair.RefreshToken]; ok {
		t.Fatal("refresh token tho bi luu vao DB")
	}
	if _, ok := f.repo.rows[domain.HashRefreshToken(pair.RefreshToken)]; !ok {
		t.Fatal("khong thay hash refresh token trong DB")
	}
}

// BR-Q3: access token mang co "da xac thuc OTP" theo cot otp_verified_at, doc lai moi lan refresh.
func TestSession_TokenMangCoXacThucOTP(t *testing.T) {
	f := newSessionFixture(t)
	verifiedOf := func(p *domain.TokenPair) bool {
		c, err := authx.VerifyAccessToken(sessionSecret, p.AccessToken, f.now)
		if err != nil {
			t.Fatal(err)
		}
		return c.Verified
	}

	pair, _ := f.svc.GenerateTokens(context.Background(), f.identity)
	if verifiedOf(pair) {
		t.Fatal("identity chua co otp_verified_at khong duoc mang co verified")
	}
	at := f.now
	f.identity.OtpVerifiedAt = &at
	next, _, err := f.svc.Refresh(context.Background(), pair.RefreshToken)
	if err != nil || !verifiedOf(next) {
		t.Fatalf("sau khi xac thuc, refresh phai mang co verified: %v", err)
	}
}

func TestSession_RefreshXoayVongVaGiuHanCuaPhien(t *testing.T) {
	f := newSessionFixture(t)
	first, _ := f.svc.GenerateTokens(context.Background(), f.identity)

	f.now = f.now.Add(20 * time.Minute)
	second, ident, err := f.svc.Refresh(context.Background(), first.RefreshToken)
	if err != nil {
		t.Fatalf("refresh lan dau phai thanh cong: %v", err)
	}
	if ident.ID != f.identity.ID {
		t.Fatalf("refresh tra sai identity")
	}
	if second.RefreshToken == first.RefreshToken || second.AccessToken == first.AccessToken {
		t.Fatal("refresh phai cap token MOI")
	}
	if !second.RefreshExpiresAt.Equal(first.RefreshExpiresAt) {
		t.Fatalf("refresh khong duoc keo dai phien: %v != %v", second.RefreshExpiresAt, first.RefreshExpiresAt)
	}
	if _, err := authx.VerifyAccessToken(sessionSecret, second.AccessToken, f.now); err != nil {
		t.Fatalf("access token moi phai hop le: %v", err)
	}
}

func TestSession_DungLaiTokenCuThuHoiCaHo(t *testing.T) {
	f := newSessionFixture(t)
	first, _ := f.svc.GenerateTokens(context.Background(), f.identity)
	second, _, err := f.svc.Refresh(context.Background(), first.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}

	// Ke trom dung lai token cu.
	if _, _, err := f.svc.Refresh(context.Background(), first.RefreshToken); !errors.Is(err, domain.ErrRefreshTokenReused) {
		t.Fatalf("dung lai token da xoay phai bi bat, nhan %v", err)
	}
	// Token moi nhat cua chu that cung bi thu hoi: ca hai phai dang nhap lai.
	if _, _, err := f.svc.Refresh(context.Background(), second.RefreshToken); err == nil {
		t.Fatal("sau khi phat hien tai su dung, token moi nhat cua ho cung phai bi thu hoi")
	}
	if len(f.audit.logs) == 0 || f.audit.logs[0].Action != app.ActionRefreshTokenReuse {
		t.Fatalf("phai ghi audit REFRESH_TOKEN_REUSE, co %+v", f.audit.logs)
	}
}

func TestSession_HetHanHoacTokenLaBiTuChoi(t *testing.T) {
	f := newSessionFixture(t)
	pair, _ := f.svc.GenerateTokens(context.Background(), f.identity)

	for name, tok := range map[string]string{"rong": "", "la": "khong-phai-token"} {
		if _, _, err := f.svc.Refresh(context.Background(), tok); !errors.Is(err, domain.ErrRefreshTokenInvalid) {
			t.Fatalf("%s: phai tra ErrRefreshTokenInvalid, nhan %v", name, err)
		}
	}

	f.now = f.now.Add(domain.RefreshTokenTTL)
	if _, _, err := f.svc.Refresh(context.Background(), pair.RefreshToken); !errors.Is(err, domain.ErrRefreshTokenInvalid) {
		t.Fatalf("het 30 ngay phai tu choi, nhan %v", err)
	}
}

func TestSession_LogoutThuHoiPhienNayKhongDungPhienKhac(t *testing.T) {
	f := newSessionFixture(t)
	laptop, _ := f.svc.GenerateTokens(context.Background(), f.identity)
	phone, _ := f.svc.GenerateTokens(context.Background(), f.identity)

	if err := f.svc.Logout(context.Background(), laptop.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.Refresh(context.Background(), laptop.RefreshToken); err == nil {
		t.Fatal("phien da logout khong duoc refresh")
	}
	if _, _, err := f.svc.Refresh(context.Background(), phone.RefreshToken); err != nil {
		t.Fatalf("phien tren thiet bi khac phai con song: %v", err)
	}
	// Logout token la: idempotent, khong loi.
	if err := f.svc.Logout(context.Background(), "khong-ton-tai"); err != nil {
		t.Fatalf("logout token la phai tra nil, nhan %v", err)
	}
}

func TestSession_LogoutAllThuHoiMoiPhien(t *testing.T) {
	f := newSessionFixture(t)
	a, _ := f.svc.GenerateTokens(context.Background(), f.identity)
	b, _ := f.svc.GenerateTokens(context.Background(), f.identity)

	if err := f.svc.LogoutAll(context.Background(), f.identity.ID); err != nil {
		t.Fatal(err)
	}
	for _, p := range []*domain.TokenPair{a, b} {
		if _, _, err := f.svc.Refresh(context.Background(), p.RefreshToken); err == nil {
			t.Fatal("logout-all phai thu hoi moi phien")
		}
	}
}

func TestSession_RefreshDocLaiRoleTuDB(t *testing.T) {
	f := newSessionFixture(t)
	pair, _ := f.svc.GenerateTokens(context.Background(), f.identity)

	f.identity.Role = domain.RoleOrganizer // Ops nang quyen sau khi dang nhap
	next, _, err := f.svc.Refresh(context.Background(), pair.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	claims, _ := authx.VerifyAccessToken(sessionSecret, next.AccessToken, f.now)
	if claims.Role != "organizer" {
		t.Fatalf("role trong token moi = %q, muon organizer", claims.Role)
	}
}

func TestSession_IdentityBiXoaThiRefreshThatBai(t *testing.T) {
	f := newSessionFixture(t)
	pair, _ := f.svc.GenerateTokens(context.Background(), f.identity)
	delete(f.ids.identities, *f.identity.Email)

	if _, _, err := f.svc.Refresh(context.Background(), pair.RefreshToken); !errors.Is(err, domain.ErrRefreshTokenInvalid) {
		t.Fatalf("identity khong con thi phai tu choi, nhan %v", err)
	}
}

func TestForgotPassword_DatLaiMatKhauThuHoiMoiPhien(t *testing.T) {
	f := newSessionFixture(t)
	pair, _ := f.svc.GenerateTokens(context.Background(), f.identity)

	redisClient := newFakeRedis()
	redisClient.otpStore["reset_buyer@example.com"] = "123456"
	uc := app.NewForgotPasswordUseCase(f.ids, &fakePasswordHasher{}, redisClient, nil, app.WithSessionRevoker(f.repo))
	if err := uc.ResetPassword(context.Background(), app.ResetPasswordInput{
		Identifier: "buyer@example.com", OTP: "123456", NewPassword: "mat-khau-moi-123",
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.Refresh(context.Background(), pair.RefreshToken); err == nil {
		t.Fatal("doi mat khau xong phien cu phai bi thu hoi")
	}
}
