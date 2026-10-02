# EVF-1801 — Baseline (trạng thái trước khi thay đổi)

Chụp lúc: 2026-09-28, commit `bb7bb61` (develop), working tree sạch.

## `npm test` trong `apps/web`

```
> @eventflow/web@0.1.0 test
> vitest run

 RUN  v2.1.8 D:/Documents/Nam4-1/Project1/EvenFlow-/apps/web

 ✓ src/lib/queue-client.test.ts (11 tests) 140ms
 ✓ src/app/page.test.tsx (1 test) 182ms

 Test Files  2 passed (2)
      Tests  12 passed (12)
   Duration  31.62s
```

**Kết luận: 12/12 pass, 0 fail. Không có test nào đang đỏ trước khi làm.** Mọi test đỏ xuất hiện sau đây đều là hồi quy do thay đổi của EVF-1801, không được bỏ qua.

## Test hiện có sẽ bị ảnh hưởng

| File | Nội dung assert | Ảnh hưởng dự kiến |
|---|---|---|
| `src/app/page.test.tsx` | `getByRole("heading", { level: 1, name: "EventFlow" })` | Refactor `page.tsx` bỏ inline style **phải giữ** `<h1>EventFlow</h1>`. Test này **không được sửa** — nếu nó đỏ thì là lỗi của refactor. |
| `src/lib/queue-client.test.ts` | 11 test cho polling client | Không đụng tới. Phải vẫn 11/11 pass. |

## Ngưỡng coverage đang cấu hình

Từ `apps/web/vitest.config.ts`:

```
thresholds: { lines: 70, functions: 70, branches: 60, statements: 70 }
```

- `include: ["src/**/*.{ts,tsx}"]`, `exclude: ["src/**/*.d.ts", "src/app/layout.tsx"]`
- Lưu ý: `npm test` (`vitest run`) **không** áp ngưỡng coverage; chỉ `npm run test:coverage` mới áp. Phải chạy `test:coverage` trước khi push, không được chỉ dựa vào `npm test`.

## CI hiện có — liên quan AC-9

`.github/workflows/ci.yml`, job `lint-web` (dòng 88–105) **đã** chạy:

```
- run: npm run lint
- run: npm run typecheck
```

**AC-9 phần "lint + typecheck chạy trong CI" coi như đã có sẵn** nhờ CI do teammate thêm (~2026-09-27). Việc cần làm chỉ là đảm bảo 2 lệnh đó vẫn pass sau thay đổi — và kiểm tra job `test-web` có áp coverage hay không.

Các workflow khác: `e2e-nightly.yml` (`make test-e2e`), `oversell-gate.yml` (`make test-oversell`), `integration.yml`, `trivy.yml`.
