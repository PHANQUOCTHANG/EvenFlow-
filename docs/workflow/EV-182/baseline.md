# EV-182 — Baseline (Trạng thái trước khi thay đổi)

Chụp lúc: 2026-10-09, commit `5eeafdc3db5f1b5aed915102c78aef9a9dfc7cf1` (nhánh `develop`, đã `git pull` — up to date).
Nhánh làm việc: `feature/EV-182-waiting-room-ui` tách từ commit trên. Working tree sạch
(chỉ có `docs/09-Quy_trinh_vibe_coding.md` untracked từ trước, không thuộc task này).

## 1. Kết quả chạy test hiện tại

- Lệnh: `cd apps/web && npx vitest run`
- Kết quả:

```
 Test Files  53 passed (53)
      Tests  1119 passed (1119)
   Start at  19:45:17
   Duration  91.74s
```

- Nhận định: Toàn bộ test hiện có đều xanh. Mọi test fail sau này là lỗi hồi quy.

## 2. Các test case có thể bị ảnh hưởng

| File | Mục tiêu assert | Ảnh hưởng dự kiến |
|---|---|---|
| `apps/web/src/lib/queue-client.test.ts` | jitter join, poll theo `poll_after_ms`, ETag/304, Retry-After, backoff | Không sửa `queue-client.ts` → không được làm gãy |
| `apps/web/src/components/queue/queue-status-panel.test.tsx` | Copy 8 trạng thái, BR-Q1 ẩn rank ở LOBBY, progress không nhảy ngược | Chỉ tái sử dụng, không sửa |
| `apps/web/src/app/(queue)/layout.test.tsx` | Shell phòng chờ không có nav | Page mới nằm trong group `(queue)` — không được thêm nav |
| `apps/web/src/components/event/event-action-panel.test.tsx` | Sau jitter điều hướng tới `/waiting/[eventId]` | Route mới phải khớp đúng đường dẫn này |

## 3. Ngưỡng coverage yêu cầu

- Lines: 70%, Functions: 70%, Branches: 60%, Statements: 70% (`apps/web/vitest.config.ts`).
