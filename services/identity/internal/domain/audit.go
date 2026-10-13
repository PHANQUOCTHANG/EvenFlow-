package domain

import (
	"context"
	"time"
)

const (
	ActorTypeHuman  = "human"
	ActorTypeAI     = "ai"
	ActorTypeSystem = "system"

	ActionAccessDenied            = "ACCESS_DENIED"
	ActionAccessDeniedCrossTenant = "ACCESS_DENIED_CROSS_TENANT"
)

// AuditLog đại diện cho một bản ghi kiểm toán an ninh trong bảng audit_logs
type AuditLog struct {
	ID        int64          `json:"id"`
	ActorType string         `json:"actor_type"` // human | ai | system
	ActorID   string         `json:"actor_id"`
	Action    string         `json:"action"`
	Subject   string         `json:"subject"`
	Reason    string         `json:"reason"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
}

// AuditRepository định nghĩa giao tiếp lưu trữ audit log
type AuditRepository interface {
	Save(ctx context.Context, log *AuditLog) error
}
