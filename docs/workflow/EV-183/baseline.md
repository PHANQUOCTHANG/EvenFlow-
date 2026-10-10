# EV-183 — Baseline (Trạng thái trước khi thay đổi)

Chụp lúc: 2026-10-10, commit `7aceaa8` (nhánh develop), working tree sạch.

## 1. Kết quả chạy test hiện tại
- Lệnh: `npm test` trong `apps/web`
- Kết quả: **62 test files passed (62), 1196 tests passed (1196), 0 failed**
- Nhận định: Toàn bộ test hiện có đều xanh. Mọi test fail sau này là lỗi hồi quy.

## 2. Các test case có thể bị ảnh hưởng
| File | Mục tiêu assert | Ảnh hưởng dự kiến |
|---|---|---|
| `apps/web/src/components/checkout/order-summary.test.tsx` | Test các trạng thái idle, creating, active, expired, sold_out, ambiguous | Không được làm gãy |
| `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | Test chọn số lượng, giới hạn server, nhãn tồn kho BR-O1 | Không được làm gãy |
| `apps/web/src/hooks/use-server-countdown.test.tsx` | Đếm ngược theo mốc server, tab background | Không được làm gãy |
| `apps/web/src/app/(checkout)/layout.test.tsx` | Layout checkout shell, landmark semantics | Không được làm gãy |

## 3. Ngưỡng coverage yêu cầu
- Lines: >= 70%, Functions: >= 70%, Branches: >= 60%, Statements: >= 70%.
