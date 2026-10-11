package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type callerCtxKey struct{}

// CallerIdentity lưu trữ danh tính và vai trò của người gọi được trích xuất từ JWT token
type CallerIdentity struct {
	ID   uuid.UUID
	Role domain.Role
}

// CallerFromContext lấy thông tin CallerIdentity từ request context
func CallerFromContext(ctx context.Context) (*CallerIdentity, bool) {
	caller, ok := ctx.Value(callerCtxKey{}).(*CallerIdentity)
	return caller, ok && caller != nil
}

// ContextWithCaller gắn CallerIdentity vào request context (tiện ích cho unit test & mock)
func ContextWithCaller(ctx context.Context, caller *CallerIdentity) context.Context {
	return context.WithValue(ctx, callerCtxKey{}, caller)
}

// writeProblemDetails ghi phản hồi lỗi theo chuẩn RFC 7807 (application/problem+json)
func writeProblemDetails(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "about:blank",
		"title":  http.StatusText(status),
		"status": status,
		"detail": detail,
	})
}

// Authenticate trích xuất và xác minh JWT Bearer token từ header Authorization
func Authenticate(secretKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeProblemDetails(w, http.StatusUnauthorized, "Yêu cầu đăng nhập: thiếu header Authorization")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				writeProblemDetails(w, http.StatusUnauthorized, "Định dạng Authorization không hợp lệ (cần 'Bearer <token>')")
				return
			}

			tokenStr := strings.TrimSpace(parts[1])
			token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("thuật toán ký không hợp lệ: %v", t.Header["alg"])
				}
				return []byte(secretKey), nil
			})

			if err != nil || !token.Valid {
				writeProblemDetails(w, http.StatusUnauthorized, "Token không hợp lệ hoặc đã hết hạn")
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				writeProblemDetails(w, http.StatusUnauthorized, "Claims của token không hợp lệ")
				return
			}

			subStr, ok := claims["sub"].(string)
			if !ok {
				writeProblemDetails(w, http.StatusUnauthorized, "Token thiếu claim 'sub'")
				return
			}

			identityID, err := uuid.Parse(subStr)
			if err != nil {
				writeProblemDetails(w, http.StatusUnauthorized, "Claim 'sub' không phải UUID hợp lệ")
				return
			}

			roleStr, ok := claims["role"].(string)
			if !ok {
				writeProblemDetails(w, http.StatusUnauthorized, "Token thiếu claim 'role'")
				return
			}

			callerRole := domain.Role(roleStr)
			if !domain.IsValidRole(callerRole) {
				writeProblemDetails(w, http.StatusUnauthorized, "Claim 'role' không hợp lệ")
				return
			}

			caller := &CallerIdentity{
				ID:   identityID,
				Role: callerRole,
			}

			ctx := ContextWithCaller(r.Context(), caller)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole chặn các request nếu vai trò của caller không nằm trong danh sách cho phép.
// Khi vi phạm, tự động ghi nhận vào bảng audit_logs và trả mã lỗi 403 Forbidden (AC-2).
func RequireRole(auditRepo domain.AuditRepository, allowedRoles ...domain.Role) func(http.Handler) http.Handler {
	allowedMap := make(map[domain.Role]bool, len(allowedRoles))
	for _, r := range allowedRoles {
		allowedMap[r] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			caller, ok := CallerFromContext(r.Context())
			if !ok {
				writeProblemDetails(w, http.StatusUnauthorized, "Yêu cầu đăng nhập")
				return
			}

			if !allowedMap[caller.Role] {
				// AC-2: Ghi audit_log khi truy cập trái quyền
				if auditRepo != nil {
					_ = auditRepo.Save(r.Context(), &domain.AuditLog{
						ActorType: domain.ActorTypeHuman,
						ActorID:   caller.ID.String(),
						Action:    domain.ActionAccessDenied,
						Subject:   fmt.Sprintf("%s %s", r.Method, r.URL.Path),
						Reason:    fmt.Sprintf("Vai trò '%s' không được phép. Yêu cầu một trong các vai trò: %v", caller.Role, allowedRoles),
						Metadata: map[string]any{
							"caller_id":     caller.ID.String(),
							"caller_role":   string(caller.Role),
							"allowed_roles": allowedRoles,
							"remote_addr":   r.RemoteAddr,
							"user_agent":    r.UserAgent(),
						},
						CreatedAt: time.Now(),
					})
				}

				writeProblemDetails(w, http.StatusForbidden, "Truy cập bị từ chối: bạn không có quyền thực hiện hành động này")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequirePermission chặn các request nếu vai trò của caller không có quyền hạn tương ứng theo Ma trận quyền.
// Khi vi phạm, tự động ghi nhận vào bảng audit_logs và trả mã lỗi 403 Forbidden (AC-2).
func RequirePermission(auditRepo domain.AuditRepository, perm domain.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			caller, ok := CallerFromContext(r.Context())
			if !ok {
				writeProblemDetails(w, http.StatusUnauthorized, "Yêu cầu đăng nhập")
				return
			}

			if !domain.HasPermission(caller.Role, perm) {
				// AC-2: Ghi audit_log khi truy cập trái quyền
				if auditRepo != nil {
					_ = auditRepo.Save(r.Context(), &domain.AuditLog{
						ActorType: domain.ActorTypeHuman,
						ActorID:   caller.ID.String(),
						Action:    domain.ActionAccessDenied,
						Subject:   fmt.Sprintf("%s %s", r.Method, r.URL.Path),
						Reason:    fmt.Sprintf("Vai trò '%s' không có quyền '%s'", caller.Role, perm),
						Metadata: map[string]any{
							"caller_id":   caller.ID.String(),
							"caller_role": string(caller.Role),
							"permission":  string(perm),
							"remote_addr": r.RemoteAddr,
							"user_agent":  r.UserAgent(),
						},
						CreatedAt: time.Now(),
					})
				}

				writeProblemDetails(w, http.StatusForbidden, "Truy cập bị từ chối: bạn không có quyền thực hiện hành động này")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// CheckResourceOwnership kiểm tra quyền sở hữu dữ liệu (AC-3: Organizer chỉ thao tác sự kiện của mình).
// Nếu caller không phải là chủ sở hữu tài nguyên, trả 403 Forbidden và ghi audit_logs với action ACCESS_DENIED_CROSS_TENANT.
func CheckResourceOwnership(w http.ResponseWriter, r *http.Request, resourceOwnerID uuid.UUID, auditRepo domain.AuditRepository) bool {
	caller, ok := CallerFromContext(r.Context())
	if !ok {
		writeProblemDetails(w, http.StatusUnauthorized, "Yêu cầu đăng nhập")
		return false
	}

	if caller.ID != resourceOwnerID {
		// AC-3: Ghi nhận vi phạm truy cập chéo tài nguyên
		if auditRepo != nil {
			_ = auditRepo.Save(r.Context(), &domain.AuditLog{
				ActorType: domain.ActorTypeHuman,
				ActorID:   caller.ID.String(),
				Action:    domain.ActionAccessDeniedCrossTenant,
				Subject:   fmt.Sprintf("%s %s", r.Method, r.URL.Path),
				Reason:    fmt.Sprintf("Organizer '%s' cố truy cập/sửa tài nguyên của Organizer '%s'", caller.ID, resourceOwnerID),
				Metadata: map[string]any{
					"caller_id":         caller.ID.String(),
					"caller_role":       string(caller.Role),
					"resource_owner_id": resourceOwnerID.String(),
					"remote_addr":       r.RemoteAddr,
					"user_agent":        r.UserAgent(),
				},
				CreatedAt: time.Now(),
			})
		}

		writeProblemDetails(w, http.StatusForbidden, "Truy cập bị từ chối: bạn không sở hữu tài nguyên này")
		return false
	}

	return true
}
