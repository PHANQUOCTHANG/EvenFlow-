# EV-121 — Kế hoạch

## 1. Hợp đồng

### Backend (Go, `services/event`)

```go
// domain/errors.go
ErrOverrideReasonRequired = errors.New("override reason is required")   // 422 /problems/override-reason-required
ErrAlreadyDecided         = errors.New("moderation already decided")    // 409 /problems/already-decided

// port/repository.go
type ModerationDecision struct {
    Event      *domain.Event      // trang thai MOI
    PrevStatus domain.EventStatus // dieu kien cap nhat: khac -> ErrAlreadyDecided
    ReviewID   *uuid.UUID         // nil neu su kien chua co review AI
    HumanID    uuid.UUID
    Verdict    string             // APPROVED | REJECTED
    Note       string
    DecidedAt  time.Time
    Audit      *domain.AuditLog
}
type ModerationDecisionStore interface { ApplyDecision(ctx, ModerationDecision) error }   // MOT transaction
ModerationRepository.ListPendingHuman(ctx) ([]*domain.ModerationReview, error)            // thay ListNeedsHuman

// app
NewModeratorActionUseCase(eventRepo, modRepo, decisions)          // bo auditRepo: audit nam trong transaction
type ModerationDetail struct { Event *domain.Event; Review *domain.ModerationReview; AiAxes map[string]float64; Audit []*domain.AuditLog }
NewGetModerationDetailUseCase(eventRepo, modRepo, auditRepo).Execute(ctx, id) (*ModerationDetail, error)
```

HTTP: `GET /api/v1/moderation/events/{id}` → `{"event": EventResponse(+tiers), "review": …, "ai_axes": {…}, "audit": […]}`.

### Frontend (`apps/web`)

```ts
// lib/moderation-service.ts
type QueueItem = { eventId; title; status; venue; startsAt; saleStartAt; aiVerdict; aiScore; aiReasons; submittedAt }
getModerationQueue(): Promise<QueueItem[]>
getModerationDetail(eventId): Promise<ModerationDetail | null>          // 404 -> null
approveEvent(eventId, note?), rejectEvent(eventId, reason)              // ApiError giu status/type
isOverride(detail): boolean                                             // status REJECTED
```

## 2. Thứ tự

| Bước | Nội dung | Test |
|---|---|---|
| 1 | domain lỗi + port | — |
| 2 | `ModeratorActionUseCase` (lý do lật ngược, transaction, đã quyết) | `moderation_action_test.go` (viết lại) |
| 3 | Hàng đợi `ListPendingHuman` + lọc theo trạng thái | `moderation_action_test.go` |
| 4 | `GetModerationDetail` (4 trục từ audit AI mới nhất) | `moderation_detail_test.go` |
| 5 | Postgres `ApplyDecision`, `ListPendingHuman` | `decision_store_integration_test.go`: hỏng giữa chừng không ghi gì; 2 quyết định đồng thời → 1 thắng 1 `ErrAlreadyDecided` |
| 6 | HTTP route + map lỗi + `main.go` | `handler_test.go`: 422 override thiếu lý do, 409, 403 vai trò khác, detail |
| 7 | `moderation-service.ts` | Vitest với `fetch` giả |
| 8 | Route `(moderator)`, nav, component | Vitest + Testing Library: queue (lọc, tìm, rỗng, lỗi), detail (tách gốc/AI, 4 trục, quyết định, xác nhận, lý do bắt buộc, 409, 403) |
| 9 | Chạy thật: compose + event service (Go container) + web dev | evidence |
| 10 | Thử phá, evidence, traceability | — |
