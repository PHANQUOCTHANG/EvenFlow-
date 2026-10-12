# EV-185 — Baseline (Trạng thái trước khi thay đổi)

Chụp lúc: 2026-10-10 22:19:40 +07:00, commit `f12e04d` (nhánh develop), working tree sạch (chỉ có tài liệu quy trình).

## 1. Kết quả chạy test hiện tại
- Lệnh: `npm test` trong `apps/web`
- Kết quả: **73 test files passed (73), 1253 tests passed (1253), 0 failed**
- Thời gian chạy: 142.80s
- Nhận định: Toàn bộ test hiện có đều xanh. Mọi test fail sau này là lỗi hồi quy.

## 2. Các test case có thể bị ảnh hưởng
| File | Mục tiêu assert | Ảnh hưởng dự kiến |
|---|---|---|
| `apps/web/src/components/queue/waiting-room.test.tsx` | Quản lý token, poll trạng thái queue, chuyển hướng ADMITTED | Không được làm gãy khi tích hợp AssistantPanel |
| `apps/web/src/components/queue/queue-position.test.tsx` | Render rank, ETA, progress | Không được làm gãy |
| `apps/web/src/components/queue/admit-banner.test.tsx` | Banner thông báo tới lượt và đếm ngược giữ chỗ | Không được làm gãy |
| `apps/web/src/app/(queue)/waiting/[eventId]/page.test.tsx` | Route phòng chờ `waiting/[eventId]` | Không được làm gãy |
| `apps/web/src/hooks/use-queue-status.test.tsx` | Hook polling & SSE trạng thái phòng chờ | Không được làm gãy |
| `apps/web/src/lib/sse.test.ts` | Parser SSE, reconnection và retry backoff | Không được làm gãy |

## 3. Ngưỡng coverage yêu cầu
- Lines: >= 70%, Functions: >= 70%, Branches: >= 60%, Statements: >= 70%.
