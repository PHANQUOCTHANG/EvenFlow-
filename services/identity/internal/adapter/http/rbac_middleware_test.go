package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
	identityHttp "github.com/eventflow/eventflow/services/identity/internal/adapter/http"
	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// mockAuditRepo hỗ trợ kiểm tra xem Audit Log có được ghi lại đúng khi truy cập trái quyền (AC-2)
type mockAuditRepo struct {
	mu   sync.Mutex
	logs []*domain.AuditLog
}

func (m *mockAuditRepo) Save(ctx context.Context, log *domain.AuditLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logs = append(m.logs, log)
	return nil
}

func (m *mockAuditRepo) GetLogs() []*domain.AuditLog {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]*domain.AuditLog, len(m.logs))
	copy(copied, m.logs)
	return copied
}

// generateTestToken ký access token đúng hợp đồng authx (EV-112), hết hạn sau exp.
func generateTestToken(secret string, id uuid.UUID, role domain.Role, exp time.Duration) string {
	now := time.Now()
	iat := now
	if exp < 0 {
		iat = now.Add(exp - time.Minute)
	}
	tokenStr, err := authx.SignAccessToken([]byte(secret), authx.AccessClaims{
		Subject:   id.String(),
		Role:      string(role),
		ID:        uuid.NewString(),
		IssuedAt:  iat,
		ExpiresAt: now.Add(exp),
	})
	if err != nil {
		panic(err)
	}
	return tokenStr
}

// TestAuthenticate_TuChoiJWTCu24Gio: JWT kiểu cũ (không iss/aud/typ, sống 24 giờ) phải bị từ
// chối sau EV-112, dù ký đúng secret.
func TestAuthenticate_TuChoiJWTCu24Gio(t *testing.T) {
	secret := "test-secret-key-at-least-16"
	legacy, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  uuid.NewString(),
		"role": string(domain.RoleOps),
		"iat":  time.Now().Unix(),
		"exp":  time.Now().Add(24 * time.Hour).Unix(),
	}).SignedString([]byte(secret))

	mw := identityHttp.Authenticate(secret)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler không được chạy với JWT cũ")
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+legacy)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("JWT cũ phải nhận 401, nhận %d", rec.Code)
	}
}

func TestAuthenticate_Middleware(t *testing.T) {
	secret := "test-secret-key-at-least-16"
	testID := uuid.New()

	successHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller, ok := identityHttp.CallerFromContext(r.Context())
		if !ok || caller == nil {
			t.Errorf("Kỳ vọng caller có trong context nhưng không tìm thấy")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if caller.ID != testID || caller.Role != domain.RoleOrganizer {
			t.Errorf("Caller mismatch: got ID %v, Role %v", caller.ID, caller.Role)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mw := identityHttp.Authenticate(secret)(successHandler)

	t.Run("Thành công với Bearer token hợp lệ", func(t *testing.T) {
		token := generateTestToken(secret, testID, domain.RoleOrganizer, time.Hour)
		req := httptest.NewRequest("GET", "/api/v1/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		mw.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("Mong muốn status 200, nhận %d", rec.Code)
		}
	})

	t.Run("Thất bại khi thiếu Authorization header -> 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/protected", nil)
		rec := httptest.NewRecorder()

		mw.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Mong muốn status 401, nhận %d", rec.Code)
		}
	})

	t.Run("Thất bại với Token sai secret key -> 401", func(t *testing.T) {
		badToken := generateTestToken("wrong-secret-key-at-least-16", testID, domain.RoleBuyer, time.Hour)
		req := httptest.NewRequest("GET", "/api/v1/protected", nil)
		req.Header.Set("Authorization", "Bearer "+badToken)
		rec := httptest.NewRecorder()

		mw.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Mong muốn status 401, nhận %d", rec.Code)
		}
	})

	t.Run("Thất bại với Token đã hết hạn -> 401", func(t *testing.T) {
		expiredToken := generateTestToken(secret, testID, domain.RoleBuyer, -time.Hour)
		req := httptest.NewRequest("GET", "/api/v1/protected", nil)
		req.Header.Set("Authorization", "Bearer "+expiredToken)
		rec := httptest.NewRecorder()

		mw.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Mong muốn status 401, nhận %d", rec.Code)
		}
	})
}

func TestRequireRole_And_AuditLog(t *testing.T) {
	auditRepo := &mockAuditRepo{}
	testID := uuid.New()

	targetHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Chỉ cho phép Ops
	guard := identityHttp.RequireRole(auditRepo, domain.RoleOps)(targetHandler)

	t.Run("Ops truy cập hợp lệ -> 200 OK, không ghi audit", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/ops/action", nil)
		ctx := identityHttp.ContextWithCaller(req.Context(), &identityHttp.CallerIdentity{
			ID:   testID,
			Role: domain.RoleOps,
		})
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		guard.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("Mong muốn status 200, nhận %d", rec.Code)
		}
		if len(auditRepo.GetLogs()) != 0 {
			t.Errorf("Không được ghi audit log khi truy cập hợp lệ, số log: %d", len(auditRepo.GetLogs()))
		}
	})

	t.Run("Buyer cố truy cập endpoint của Ops -> 403 Forbidden và ghi audit_logs (AC-2)", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/ops/action", nil)
		buyerID := uuid.New()
		ctx := identityHttp.ContextWithCaller(req.Context(), &identityHttp.CallerIdentity{
			ID:   buyerID,
			Role: domain.RoleBuyer,
		})
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		guard.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("Mong muốn status 403, nhận %d", rec.Code)
		}

		logs := auditRepo.GetLogs()
		if len(logs) != 1 {
			t.Fatalf("Kỳ vọng 1 bản ghi audit log được tạo, thực tế có: %d", len(logs))
		}
		if logs[0].Action != domain.ActionAccessDenied {
			t.Errorf("Action audit mong muốn %s, nhận: %s", domain.ActionAccessDenied, logs[0].Action)
		}
		if logs[0].ActorID != buyerID.String() {
			t.Errorf("ActorID audit mong muốn %s, nhận: %s", buyerID.String(), logs[0].ActorID)
		}
	})
}

func TestRequirePermission_AC1_AC2(t *testing.T) {
	auditRepo := &mockAuditRepo{}

	targetHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Endpoint yêu cầu quyền PermEventModerate (Duyệt sự kiện)
	guard := identityHttp.RequirePermission(auditRepo, domain.PermEventModerate)(targetHandler)

	t.Run("Moderator có quyền duyệt sự kiện -> 200 OK", func(t *testing.T) {
		modID := uuid.New()
		req := httptest.NewRequest("POST", "/api/v1/events/123/review", nil)
		ctx := identityHttp.ContextWithCaller(req.Context(), &identityHttp.CallerIdentity{
			ID:   modID,
			Role: domain.RoleModerator,
		})
		rec := httptest.NewRecorder()

		guard.ServeHTTP(rec, req.WithContext(ctx))
		if rec.Code != http.StatusOK {
			t.Errorf("Mong muốn status 200, nhận %d", rec.Code)
		}
	})

	t.Run("Organizer cố duyệt sự kiện -> 403 Forbidden + ghi audit_logs", func(t *testing.T) {
		orgID := uuid.New()
		req := httptest.NewRequest("POST", "/api/v1/events/123/review", nil)
		ctx := identityHttp.ContextWithCaller(req.Context(), &identityHttp.CallerIdentity{
			ID:   orgID,
			Role: domain.RoleOrganizer,
		})
		rec := httptest.NewRecorder()

		guard.ServeHTTP(rec, req.WithContext(ctx))
		if rec.Code != http.StatusForbidden {
			t.Errorf("Mong muốn status 403, nhận %d", rec.Code)
		}

		logs := auditRepo.GetLogs()
		found := false
		for _, l := range logs {
			if l.ActorID == orgID.String() && l.Action == domain.ActionAccessDenied {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Không tìm thấy bản ghi audit_log cho Organizer vi phạm quyền duyệt sự kiện")
		}
	})
}

func TestCheckResourceOwnership_AC3(t *testing.T) {
	auditRepo := &mockAuditRepo{}
	organizerA := uuid.New()
	organizerB := uuid.New()

	t.Run("Organizer A sửa sự kiện của Organizer A -> Hợp lệ", func(t *testing.T) {
		req := httptest.NewRequest("PUT", "/api/v1/events/111", nil)
		ctx := identityHttp.ContextWithCaller(req.Context(), &identityHttp.CallerIdentity{
			ID:   organizerA,
			Role: domain.RoleOrganizer,
		})
		rec := httptest.NewRecorder()

		ok := identityHttp.CheckResourceOwnership(rec, req.WithContext(ctx), organizerA, auditRepo)
		if !ok {
			t.Errorf("Kỳ vọng CheckResourceOwnership trả về true khi chính chủ")
		}
	})

	t.Run("Organizer A sửa sự kiện của Organizer B -> 403 Forbidden + ghi audit CROSS_TENANT (AC-3)", func(t *testing.T) {
		req := httptest.NewRequest("PUT", "/api/v1/events/222", nil)
		ctx := identityHttp.ContextWithCaller(req.Context(), &identityHttp.CallerIdentity{
			ID:   organizerA,
			Role: domain.RoleOrganizer,
		})
		rec := httptest.NewRecorder()

		ok := identityHttp.CheckResourceOwnership(rec, req.WithContext(ctx), organizerB, auditRepo)
		if ok {
			t.Errorf("Kỳ vọng CheckResourceOwnership trả về false khi truy cập chéo tài nguyên")
		}
		if rec.Code != http.StatusForbidden {
			t.Errorf("Mong muốn status 403, nhận %d", rec.Code)
		}

		logs := auditRepo.GetLogs()
		foundCrossTenant := false
		for _, l := range logs {
			if l.ActorID == organizerA.String() && l.Action == domain.ActionAccessDeniedCrossTenant {
				foundCrossTenant = true
				break
			}
		}
		if !foundCrossTenant {
			t.Errorf("Kỳ vọng tìm thấy log ACCESS_DENIED_CROSS_TENANT nhưng không có")
		}
	})
}
