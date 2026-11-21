# EV-121 — Baseline

| Mục | Giá trị |
|---|---|
| Chụp lúc | 2026-10-21, `feature/EV-121-moderator-workspace` = `develop` @ `43375eb` (sau PR #85) |
| Máy | Windows 11, Docker Desktop 29.6.2; Go 1.26.9 trong `golang:1.26`; Node 22, Vitest 2.1.8 |

| Lệnh | Kết quả |
|---|---|
| `services/event`: `go test ./...` | PASS — `adapter/http`, `app`, `domain`, `worker` |
| `services/event`: `go test -tags=integration ./internal/...` | PASS — kể cả `adapter/postgres`, `worker` |
| `apps/web`: `npx vitest run` | **2342 passed / 152 file** |

Test có thể bị ảnh hưởng: `services/event/internal/app/moderation_action_test.go`, `handle_ai_verdict_test.go`
(fake `ModerationRepository`), `adapter/http/handler_test.go` (`setupModerationTestServer`),
`apps/web/src/components/layout/nav-config.test.ts`.

Nhánh đồng đội đang mở: `feature/frontend-debt-r4` sửa `apps/web/src/lib/api-client.ts` — slice này **không** sửa
file đó.
