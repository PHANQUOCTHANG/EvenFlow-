# EV-187 — Baseline (trước khi implement)

Chụp: 2026-10-13, worktree `D:/Documents/Nam4-1/Project1/EvenFlow-187`, nhánh `feature/EV-187-ops-war-room`
(từ `develop` @ `51c4729`). Lệnh: `npx vitest run` trong `apps/web`.

## 1. Trước khi thêm test (code + test gốc của develop)

- **93 test files passed (93), 1528 tests passed (1528), 0 failed.**
- Mọi test fail về sau trong các file có sẵn là hồi quy.

## 2. Sau khi thêm test EV-187 (code CHƯA implement)

- **Test Files: 7 failed | 92 passed (99). Tests: 10 failed | 1528 passed (1538).**
- 1528 test có sẵn vẫn xanh, kể cả 66 test cũ trong `lib/format.test.ts`.

| File | Trạng thái | Lý do (dự kiến) |
|---|---|---|
| `src/lib/ops-service.test.ts` | fail lúc import | chưa có `lib/ops-service.ts` |
| `src/hooks/use-ops-war-room.test.tsx` | fail lúc import | chưa có `hooks/use-ops-war-room.ts` |
| `src/components/ops/ops-war-room.test.tsx` | fail lúc import | chưa có `components/ops/ops-war-room.tsx` |
| `src/app/(ops)/ops/events/page.test.tsx` | fail lúc import | chưa có `page.tsx` |
| `src/app/(ops)/ops/events/[eventId]/page.test.tsx` | fail lúc import | chưa có `page.tsx` |
| `src/app/(ops)/ops/events/[eventId]/not-found.test.tsx` | fail lúc import | chưa có `not-found.tsx` |
| `src/lib/format.test.ts` | 10 test mới fail, 66 test cũ pass | chưa có `formatCount`, `formatTimeVn` |

Số test mới (đếm cả `it.each` đã bung): ops-service 46, hook 26, component 57, trang 5 + 5 + not-found 2,
format 10 → **151 test mới**. Sau khi implement, kỳ vọng 99 files / 1679 tests xanh.

`tsc --noEmit` hiện chỉ báo lỗi do thiếu module/export ở trên (và `implicit any` kéo theo từ module thiếu).

## 3. Test có sẵn đã đổi

**Không đổi assertion nào.** `components/layout/nav-config.test.ts` và `app/(ops)/layout.test.tsx` không có
assertion nào khẳng định War room `ready: false`:

- `nav-config.test.ts` › "ton tai it nhat mot item ready: false" vẫn đúng sau khi đổi vì `/ops/ai` còn `ready: false`.
- `(ops)/layout.test.tsx` › "item ready: true render thanh <a>" / "item ready: false KHONG render thanh <a>" lặp theo
  `OPS_NAV` nên tự áp dụng cho War room khi cờ đổi.

Vì vậy (plan §6 I-1) không sửa hai file đó. Kỳ vọng "War room `ready: true`" (AC-8) được đặt trong file mới
`app/(ops)/ops/events/page.test.tsx` › "mục War room trong OPS_NAV đã ready: true và trỏ /ops/events (AC-8)" —
test này hiện fail cùng file (import `./page` thiếu); khi đó nếu `nav-config.ts` chưa đổi cờ thì riêng nó sẽ fail.

Sửa test có sẵn duy nhất: `lib/format.test.ts` — chỉ **thêm** import `formatCount`, `formatTimeVn`, `vi` và
hai `describe` mới ở cuối file (plan §6 m-1), không đổi test cũ.

## 4. E2E

Thêm `tests/e2e/specs/ops-war-room.spec.ts` (chưa chạy; cần build production + server). Không đụng
`tests/e2e/playwright.config.ts` hay `tests/e2e/fixtures/**` (EV-198).

## 5. Ngưỡng coverage

Lines ≥ 70%, Functions ≥ 70%, Branches ≥ 60%, Statements ≥ 70% (`vitest.config.ts`).
