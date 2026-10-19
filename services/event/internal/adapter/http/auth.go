package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
	"github.com/eventflow/eventflow/libs/go/httpx"
	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/google/uuid"
)

// Phan quyen route event (EV-114) theo ma tran quyen docs/01-nghiep-vu.md muc 3.
//
// Danh tinh do gateway xac minh token roi gan vao X-Identity-Id / X-Identity-Role
// (authx.PrincipalFromHeaders). organizer_id va moderator_id KHONG BAO GIO lay tu body nua:
// body chi con de doi chieu, lech voi token thi 403.

const (
	roleOrganizer = "organizer"
	roleModerator = "moderator"
	roleOps       = "ops"

	// Trung voi hang so cua identity de mot truy van audit_logs gom duoc ca hai service.
	actionAccessDenied            = "ACCESS_DENIED"
	actionAccessDeniedCrossTenant = "ACCESS_DENIED_CROSS_TENANT"
)

type principalKey struct{}

// Caller la nguoi goi da xac thuc.
type Caller struct {
	ID   uuid.UUID
	Role string
}

func callerFrom(ctx context.Context) (Caller, bool) {
	c, ok := ctx.Value(principalKey{}).(Caller)
	return c, ok
}

// optionalCaller doc danh tinh neu co (route cong khai), khong bao loi khi thieu.
func optionalCaller(r *http.Request) (Caller, bool) {
	p, ok := authx.PrincipalFromHeaders(r.Header)
	if !ok {
		return Caller{}, false
	}
	id, err := uuid.Parse(p.ID)
	if err != nil {
		return Caller{}, false
	}
	return Caller{ID: id, Role: p.Role}, true
}

// require chi cho nguoi goi co mot trong cac role di tiep. Thieu danh tinh -> 401; sai role ->
// 403 va ghi audit ACCESS_DENIED.
func (h *EventHandler) require(next http.HandlerFunc, roles ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := optionalCaller(r)
		if !ok {
			httpx.WriteProblem(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if !slices.Contains(roles, c.Role) {
			h.audit(r, c, actionAccessDenied, fmt.Sprintf("role %q is not allowed, need one of %v", c.Role, roles))
			httpx.WriteProblem(w, http.StatusForbidden, "you are not allowed to perform this action")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, c)))
	})
}

// matchBodyID doi chieu id client gui trong body (truong cu, giu cho tuong thich) voi token.
// Rong thi bo qua; lech thi la gia danh -> 403 + audit.
func (h *EventHandler) matchBodyID(w http.ResponseWriter, r *http.Request, c Caller, field, raw string) bool {
	if raw == "" || raw == c.ID.String() {
		return true
	}
	h.audit(r, c, actionAccessDeniedCrossTenant, fmt.Sprintf("body %s=%s does not match the caller", field, raw))
	httpx.WriteProblem(w, http.StatusForbidden, field+" does not match the authenticated user")
	return false
}

// fail ghi loi nghiep vu; truy cap su kien cua organizer khac duoc audit nhu cross-tenant.
func (h *EventHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, domain.ErrUnauthorizedOrganizer) {
		if c, ok := callerFrom(r.Context()); ok {
			h.audit(r, c, actionAccessDeniedCrossTenant, "organizer tried to change an event they do not own")
		}
	}
	handleDomainError(w, r, err)
}

func (h *EventHandler) audit(r *http.Request, c Caller, action, reason string) {
	if h.auditRepo == nil {
		return
	}
	actor := c.ID.String()
	subject := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
	if id := r.PathValue("id"); id != "" {
		subject = "event:" + id
	}
	if err := h.auditRepo.Create(r.Context(), &domain.AuditLog{
		ActorType: "human",
		ActorID:   &actor,
		Action:    action,
		Subject:   subject,
		Reason:    &reason,
		Metadata: map[string]any{
			"caller_role": c.Role,
			"route":       r.Pattern,
		},
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		slog.ErrorContext(r.Context(), "ghi audit tu choi truy cap that bai", "err", err)
	}
}

// canSeeUnpublished: su kien chua duyet (DRAFT, PENDING_REVIEW, REJECTED) chi chu su kien,
// moderator va ops thay; nguoi khac nhan 404 nhu khong ton tai.
func canSeeUnpublished(evt *domain.Event, c Caller, ok bool) bool {
	switch evt.Status {
	case domain.StatusDraft, domain.StatusPendingReview, domain.StatusRejected:
		if !ok {
			return false
		}
		return c.Role == roleModerator || c.Role == roleOps || (c.Role == roleOrganizer && c.ID == evt.OrganizerID)
	default:
		return true
	}
}
