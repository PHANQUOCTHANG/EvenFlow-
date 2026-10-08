# EV-181 — Baseline (Trạng thái trước khi thay đổi)

Chụp lúc: 2026-10-06, commit `c848afa` (nhánh develop), working tree sạch.
Nhánh thực hiện: `feature/EV-181-event-page-isr`

---

## 1. Kết quả chạy test hiện tại (`apps/web`)

Lệnh: `npm test` (vitest run)

```
Test Files  44 passed (44)
     Tests  1086 passed (1086)
  Start at  21:35:07
  Duration  105.09s
```

**Nhận định:**
- Toàn bộ 44 test suite với 1086 test case hiện có đều xanh 100% (0 failed).
- Mọi test bị đỏ trong quá trình triển khai task EV-181 đều là lỗi hồi quy (regression) và bắt buộc phải xử lý triệt để.

---

## 2. Các test case và module có thể bị ảnh hưởng

| File | Mục tiêu assert | Ảnh hưởng dự kiến |
|---|---|---|
| `src/lib/server-time.ts` / `server-time.test.ts` | Tính offset và chuẩn hóa epoch mốc thời gian | Tái sử dụng, giữ nguyên vẹn logic cốt lõi |
| `src/hooks/use-server-countdown.ts` / `use-server-countdown.test.tsx` | Đếm ngược theo mốc server | Tái sử dụng, không làm thay đổi hợp đồng |
| `src/components/ui/server-expiry-countdown.tsx` | Hiển thị countdown theo variant `sale-start` | Tái sử dụng cho trang chi tiết sự kiện |
| `src/components/layout/nav-config.ts` | Danh sách navigation của ứng dụng | Cập nhật trạng thái `ready: true` cho route `/events` |
| `src/app/(marketing)/layout.tsx` | PublicShell bọc các trang marketing / công khai | Giữ nguyên vẹn tính bao đóng |

---

## 3. Ngưỡng coverage yêu cầu (`vitest.config.ts`)

Từ `apps/web/vitest.config.ts`:
- Lines: $\ge 70\%$
- Functions: $\ge 70\%$
- Branches: $\ge 60\%$
- Statements: $\ge 70\%$

Toàn bộ code mới cho task EV-181 (hook, jitter utility, mock/api client, UI component, page) bắt buộc phải có unit test bao phủ đạt trên ngưỡng quy định.
