# EV-186 — Baseline (Trạng thái trước khi thay đổi)

Chụp lúc: 2026-10-11 12:17:44 +07:00, commit `0108c5f` (nhánh develop), working tree sạch (chỉ có tài liệu quy trình).

## 1. Kết quả chạy test hiện tại
- Lệnh: `npm test` trong `apps/web`
- Kết quả: **78 test files passed (78), 1290 tests passed (1290), 0 failed**
- Thời gian chạy: 91.87s
- Nhận định: Toàn bộ test hiện có đều xanh. Mọi test fail sau này là lỗi hồi quy.

## 2. Các test case có thể bị ảnh hưởng
| File | Mục tiêu assert | Ảnh hưởng dự kiến |
|---|---|---|
| `apps/web/src/app/(organizer)/layout.test.tsx` | Layout của route group `(organizer)` | Đảm bảo WorkspaceShell, landmark nav, landmark main vẫn pass khi bổ sung các trang con |
| `apps/web/src/components/layout/nav-config.test.ts` | Cấu hình nav theo vai trò | Cập nhật `ORGANIZER_NAV` `ready: true`, đảm bảo các test `activeHref` và kiểm tra `ready` của các role khác không bị ảnh hưởng |
| `apps/web/src/components/layout/workspace-shell.test.tsx` | Shell điều hướng cho trang quản trị | Không được làm gãy |
| `apps/web/src/lib/event-service.test.ts` | Snapshot dữ liệu sự kiện | Không được làm gãy |

## 3. Ngưỡng coverage yêu cầu
- Lines: >= 70%, Functions: >= 70%, Branches: >= 60%, Statements: >= 70%.
