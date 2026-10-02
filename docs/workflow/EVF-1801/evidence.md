# EVF-1801 — Evidence (output thô)

Branch `feature/EVF-1801-design-system`, base `fa8ca69`. Chạy trên Windows 11, Node 22, npm.

> Nguyên tắc: chỉ ghi output thật. Mục nào chưa chạy được thì ghi **CHƯA VERIFY**, không suy đoán là pass.

## 1. typecheck — PASS

```
> @eventflow/web@0.1.0 typecheck
> tsc --noEmit
```

Không có output = 0 error.

## 2. lint — PASS (sau khi bổ sung ESLint config)

Lần chạy đầu **không chạy được**:

```
> next lint
? How would you like to configure ESLint? https://nextjs.org/docs/app/building-your-application/configuring/eslint
❯  Strict (recommended)
   Base
   Cancel
```

Nguyên nhân: **repo chưa từng có file ESLint config nào**. Đã xác nhận:

```
$ git ls-files | grep -iE "eslint"     -> rỗng
$ ls -a apps/web | grep -iE "eslint"   -> rỗng
```

Hệ quả: job `lint-web` trong `.github/workflows/ci.yml` (dòng 104, `npm run lint`) **chưa thực sự lint gì** từ trước tới giờ. Đây là phát hiện ngoài phạm vi EVF-1801 nhưng chặn AC-12.

Sau khi thêm `apps/web/eslint.config.mjs` (flat config cho ESLint 9 + FlatCompat cho `eslint-config-next`), lint bắt được 2 lỗi, **cả hai trong file có từ trước**:

```
./src/lib/queue-client.test.ts
195:24  Error: '_' is assigned a value but never used.  @typescript-eslint/no-unused-vars

./src/lib/queue-client.ts
127:14  Error: 'err' is defined but never used.  @typescript-eslint/no-unused-vars
```

Cả hai là binding cố ý không dùng (`catch (err)` chỉ để backoff, `for await (const _ of ...)` chỉ để đếm vòng lặp). Vì `plan.md` §2 cấm sửa 2 file đó, xử lý bằng cách cấu hình rule theo convention (`argsIgnorePattern`/`varsIgnorePattern` `^_`, `caughtErrors: "none"` — đúng mặc định của ESLint 8 mà code này được viết theo). Kết quả:

```
> next lint

✔ No ESLint warnings or errors
```

## 3. build — PASS

```
> next build

   ▲ Next.js 15.0.3
 ✓ Compiled successfully
   Linting and checking validity of types ...
 ✓ Generating static pages (4/4)

Route (app)                              Size     First Load JS
┌ ○ /                                    3.65 kB         104 kB
└ ○ /_not-found                           896 B          101 kB
+ First Load JS shared by all            99.9 kB
```

## 4. AC-11 — Tailwind v4 thực sự sinh utility từ token

Kiểm trên file CSS **đã build** (`.next/static/css/cb758309febc9128.css`), không phải trên source:

```
--ef-bg            có
.rounded-card      có
.text-headline-xl  có
.bg-surface        có
.text-fg-muted     có
.shadow-card       có
.p-md              có
```

Cấu hình CSS-first của v4 hoạt động; không cần `tailwind.config.js`.

## 5. AC-2 — dark mode có mặt trong CSS build

Cả hai nhánh đều có trong output (minifier bỏ dấu ngoặc kép trong attribute selector, nên phải grep dạng không ngoặc):

```
@media (prefers-color-scheme:dark){:root:not([data-theme=light]){color-scheme:dark;--ef-bg:#0b1220 ...
:root[data-theme=dark]{color-scheme:dark; ... --ef-primary:#8b87f5;--ef-primary-hover:#9d99f8;--ef-primary-fg:#0b1220 ...
```

Xác nhận `--ef-primary-fg` ở dark là `#0b1220` (màu tối) chứ không phải trắng — đúng yêu cầu tương phản ở spec §3.4.

## 6. AC-1 — không có màu hard-code ngoài tokens.css — PASS

```
$ grep -rnE "#[0-9a-fA-F]{3,8}\b|rgb\(|hsl\(" src/components src/app --include="*.tsx" --include="*.ts" --include="*.css" | grep -v "\.test\."
(rỗng)

$ grep -rlE "#[0-9a-fA-F]{6}" src/
src/styles/tokens.css
```

Đúng một nơi định nghĩa màu.

## 7. AC-9 — `accent` không bị dùng làm màu chữ — PASS

```
$ grep -rnE "text-accent|color:\s*var\(--ef-accent" src/components src/app
(rỗng)
```

Amber chỉ xuất hiện ở `border-accent` (Alert variant warning). Chữ cảnh báo dùng `text-warning-fg`.

## 8. AC-13 — YAML workflow hợp lệ — PASS

Parse bằng `js-yaml`, không chỉ đọc mắt:

```
OK   .github/workflows/cd-web.yml | jobs: publish
     on: {"workflow_run":{"workflows":["CI"],"types":["completed"],"branches":["main"]},"workflow_dispatch":{}}
     permissions: {"contents":"read","packages":"write"}
     concurrency: {"group":"cd-web","cancel-in-progress":false}
OK   .github/workflows/ci.yml | jobs: changes, lint-go, lint-web, lint-python, test-go, test-python, test-web, build
```

## 9. AC-13 — `docker build` — CHƯA VERIFY

```
$ docker --version
Docker version 28.5.1, build e180ab8

$ docker build -f deploy/docker/web.Dockerfile -t eventflow/web:local .
ERROR: error during connect: Head "http://%2F%2F.%2Fpipe%2FdockerDesktopLinuxEngine/_ping":
open //./pipe/dockerDesktopLinuxEngine: The system cannot find the file specified.
```

Docker CLI có, **daemon không chạy** (Docker Desktop chưa bật). Không phải lỗi Dockerfile, nhưng cũng **không phải bằng chứng là Dockerfile đúng**.

Đường verify còn lại: `.github/workflows/trivy.yml` đã build `deploy/docker/web.Dockerfile` trên mọi PR chạm `apps/web` (matrix `web`), nên CI sẽ chứng minh thay. Phần `npm ci` cần `package-lock.json` có trong build context — đã xác nhận file tồn tại và được git track.

## 10. Test suite

Baseline trước khi làm (`baseline.md`): 2 file, 12 test, 12 pass.

Lần chạy sau khi hiện thực xong (test do agent độc lập viết):

```
 Test Files  2 failed | 15 passed (17)
      Tests  2 failed | 230 passed (232)
```

2 test đỏ cùng một nguyên nhân — **bug thật trong hiện thực**: `FieldLabel` render dấu `*` của field
required BÊN TRONG `<label>`, làm `textContent` của label thành `"Loại vé *"`, nên
`getByLabelText("Loại vé")` trượt trên mọi field required. Đã sửa `field.tsx` cho `*` ra ngoài `<label>`.

Sau khi sửa:

```
 Test Files  1 failed | 16 passed (17)
      Tests  1 failed | 231 passed (232)
```

Test còn đỏ: `field.test.tsx > FieldLabel > required them dau hieu bat buoc nhung van giu nguyen nhan`,
assert `reqText.length > plainText.length` trên textContent của `<label>`. Assertion này mâu thuẫn với
chính tên test và với 2 test của `input.test.tsx` / `select.test.tsx`. Nguyên nhân gốc: `plan.md` §3
ban đầu **không chốt** dấu hiệu required nằm trong hay ngoài `<label>` — agent test đã tự nêu đúng lỗ
hổng này. Đã bổ sung hợp đồng vào `plan.md` §3 và giao lại cho agent test sửa đúng một assertion đó
(người hiện thực không tự sửa test).

Sau khi hợp đồng `FieldLabel` được chốt trong `plan.md` §3 và agent test sửa assertion đó, cộng thêm
`theme-script.test.tsx` (13 test, đóng khoảng trống AC-2 chống FOUC) và 2 test aria cho RadioGroup:

```
 Test Files  18 passed (18)
      Tests  248 passed (248)
exit=0
```

**248/248 pass, 0 fail, 0 skip.** `queue-client.test.ts` vẫn 11/11 và `page.test.tsx` vẫn pass **mà
không bị sửa** — đã xác nhận `git diff fa8ca69` trên 2 file đó rỗng.

## 11. AC-10 — coverage — PASS

```
 % Coverage report from v8
-------------------|---------|----------|---------|---------|
File               | % Stmts | % Branch | % Funcs | % Lines |
-------------------|---------|----------|---------|---------|
All files          |   99.66 |    96.45 |   95.38 |   99.66 |
 app               |     100 |      100 |     100 |     100 |
 components/theme  |   99.59 |    94.23 |     100 |   99.59 |
 components/ui     |   99.66 |    97.26 |   91.17 |   99.66 |
 hooks             |     100 |      100 |     100 |     100 |
 lib               |   99.52 |    93.22 |     100 |   99.52 |
-------------------|---------|----------|---------|---------|
```

Ngưỡng yêu cầu lines/functions/statements ≥ 70, branches ≥ 60 → đạt, và `exit=0` nghĩa là vitest
cũng tự xác nhận không vi phạm ngưỡng nào.

Riêng file hiện thực (bỏ file test ra): `button.tsx` 94.73% stmts, `checkbox.tsx` 93.61%, các file còn
lại 100%. `queue-client.ts` 97.53% là mức có từ trước, không bị kéo xuống.

**Lưu ý về cấu hình, không phải kết quả:** `vitest.config.ts` đặt `include: ["src/**/*.{ts,tsx}"]` nên
**file test cũng bị tính vào coverage**. File test luôn ~100% covered theo định nghĩa, nên con số tổng
bị đẩy lên và ngưỡng 70% thực chất dễ hơn ý định ban đầu. Đây là vấn đề có từ trước EVF-1801 và
`plan.md` §2 cấm sửa `vitest.config.ts`, nên **chỉ báo lại**: nên thêm `"src/**/*.{test,spec}.{ts,tsx}"`
vào `coverage.exclude` trong một task riêng. Số liệu ở trên vẫn đúng với cấu hình hiện tại của team.

## 12. Trạng thái Definition of Done

| Hạng mục | Trạng thái |
|---|---|
| 248/248 test pass, 0 skip | ✅ |
| coverage vượt ngưỡng, vitest exit 0 | ✅ |
| không hồi quy so với baseline (12 test cũ) | ✅ |
| lint / typecheck / build | ✅ |
| AC-1, AC-9 grep | ✅ |
| YAML workflow parse được | ✅ |
| diff không chạm file bị cấm | ✅ |
| `docker build` | ⚠️ chưa verify — Docker Desktop chưa chạy; `trivy.yml` trong CI sẽ build thay |
| `make test` từ thư mục gốc | ⚠️ không chạy được tại máy này (xem mục 13) |
| Code review độc lập | đang chạy |
| Xác nhận của người dùng về việc cắt scope | chờ |

## 13. AC-12 — `make test` — CHƯA VERIFY (thiếu toolchain tại máy)

```
$ make test
bash: make: command not found        (exit 127)

$ Get-Command make  -> KHONG CO
$ Get-Command go    -> KHONG CO
$ Get-Command uv    -> uv 0.11.31
```

Máy này **không có `make` và không có Go toolchain**, nên không chạy được target `test` của Makefile
(target đó gồm `go test ./...`, `uv run pytest -q`, `cd apps/web && npm test`).

Đây là giới hạn môi trường, không phải lỗi của thay đổi. Điều cần lưu ý khi đánh giá:

- Phần duy nhất của `make test` mà diff này ảnh hưởng là `cd apps/web && npm test` — đã chạy riêng,
  **248/248 pass** (mục 10).
- Diff không chạm `services/**`, `libs/**`, `go.work`, hay `services/ai-worker/**`, nên phần `go test`
  và `pytest` không thể bị ảnh hưởng. Hai phần đó được CI chạy ở job `test-go` và `test-python`.
- **Không được coi AC-12 là đã đạt dựa trên suy luận này.** Nó sẽ được chứng minh khi CI chạy trên PR.

## 14. Tính lại token "soft" bằng hex tường minh (xử lý M-4 của code review)

Sau khi code review chỉ ra fallback `color-mix` làm tương phản tụt về 1:1, 5 token `--ef-*-soft` được
thay bằng hex tường minh. Giá trị **không chọn bằng mắt**: chạy script tìm tỉ lệ tint LỚN NHẤT (tint
đậm nhất, dễ phân biệt nhất) mà vẫn đạt **>= 4.75:1** với màu chữ thật — chừa biên trên ngưỡng AA 4.5.

Công thức dùng đúng định nghĩa WCAG 2: linearize sRGB rồi `L = 0.2126R + 0.7152G + 0.0722B`,
`ratio = (Lmax + 0.05) / (Lmin + 0.05)`.

```
== light (surface #ffffff) ==
--ef-info-soft      #f0f0fd   tint  9%   chu #5b55e7   4.77:1
--ef-success-soft   #eaf3ef   tint  9%   chu #18794e   4.78:1
--ef-warning-soft   #fbefde   tint 17%   chu #9a5b00   4.78:1
--ef-danger-soft    #f1d5d3   tint 19%   chu #b42318   4.76:1
--ef-neutral-soft   #dbdee1   tint 21%   chu #526071   4.75:1

== dark (surface #121c2e) ==
--ef-info-soft      #1f2844   tint 11%   chu #8b87f5   4.76:1
--ef-success-soft   #1d4242   tint 22%   chu #45c78a   5.12:1
--ef-warning-soft   #433d37   tint 22%   chu #f0b355   5.75:1
--ef-danger-soft    #392a38   tint 17%   chu #f97066   4.82:1
--ef-neutral-soft   #323d4f   tint 22%   chu #a3b1c4   5.03:1
```

Lần tính đầu (tint 12-18% theo giá trị `color-mix` cũ) có **2 cặp FAIL**: light `success-soft` 4.45:1 và
dark `info-soft` 4.25:1. Nếu giữ nguyên tỉ lệ cũ rồi chỉ đổi sang hex thì vẫn vi phạm AC-9. Đây là lý do
phải tính chứ không chuyển đổi máy móc.

Verify trên CSS **đã build** sau khi sửa:

```
$ grep -o 'color-mix' <css> | wc -l
2        <- ca 2 deu la preflight cua Tailwind cho ::placeholder, co fallback `color: currentColor`,
            khong lien quan token cua minh

$ grep -o 'ef-info-soft:[^;}]*' <css> | sort -u
ef-info-soft:#1f2844
ef-info-soft:#f0f0fd
$ grep -o 'ef-neutral-soft:[^;}]*' <css> | sort -u
ef-neutral-soft:#323d4f
ef-neutral-soft:#dbdee1
```

Không còn dòng `--ef-*-soft: var(--ef-*)` nào. Cả 2 theme đều có giá trị riêng.

## 15. Verify m-1 — `.ef-focus-ring` đã thắng cascade

```
$ grep -o '@layer[^{;]*[;{]' <css> | sort -u
@layer base{
@layer components;
@layer properties{
@layer theme{
@layer utilities{

$ node -e "... indexOf ..."
.border-border-strong  @ 9686
.ef-focus-ring         @ 14093
-> ef-focus-ring dung SAU, trong CUNG layer utilities
   => thang ca theo layer, specificity (0-2-0 > 0-1-0) va source order
```

```css
.ef-focus-ring:focus-visible{box-shadow:0 0 0 3px var(--ef-ring);border-color:var(--ef-primary);outline:none}
.ef-focus-ring:focus-visible{outline-offset:2px;outline:2px solid}   /* forced-colors */
```

## 16. Gate chạy lại sau khi xử lý code review

```
$ npm run typecheck   -> 0 error
$ npm run lint        -> ✔ No ESLint warnings or errors
$ npm run build       -> ✓ Compiled successfully, 4/4 static pages
                         Route /  884 B,  First Load JS 104 kB

$ node -e "js-yaml parse .github/workflows/cd-web.yml"
parse OK, jobs: publish
if: (github.event_name == 'workflow_run' && github.event.workflow_run.conclusion == 'success'
     && github.event.workflow_run.event == 'push'
     && github.event.workflow_run.head_repository.full_name == github.repository)
    || (github.event_name == 'workflow_dispatch' && github.ref == 'refs/heads/main')
```

`apps/web/public/.gitkeep` đã tạo → `COPY --from=build /app/public ./public` ở
`web.Dockerfile:22` không còn trỏ vào path không tồn tại (B-1).

## 17. Gate cuoi cung (sau khi xu ly xong code review)

```
$ npm run test:coverage
exit=0
 Test Files  18 passed (18)
      Tests  260 passed (260)

All files          |   99.78 |    96.73 |   95.45 |   99.78 |
  button.tsx       |   94.73 |    88.88 |     100 |   94.73 | 52-54
  checkbox.tsx     |     100 |      100 |     100 |     100 |
  spinner.tsx      |     100 |      100 |     100 |     100 |
```

260/260 pass, 0 fail, 0 skip. `exit=0` nghia la vitest cung tu xac nhan khong vi pham nguong nao.

Thay doi so voi lan truoc (248 test): `checkbox.tsx` tu 93.61% len **100%** nho 3 test hop nhat ref
(m-2), `spinner.tsx` giu 100% voi 8 test moi cho `decorative` (M-3).

`button.tsx` dong 52-54 van chua cover: do la guard `if (blocked) { preventDefault; return }`. jsdom
khong dispatch click tren phan tu `disabled` nen guard khong bao gio chay trong test — dung nhu
reviewer chi ra o m-3. Guard duoc giu lam belt-and-braces; viec doi sang `aria-disabled` de guard co y
nghia that nam o ticket rieng, khong nhet vao EVF-1801.

Mot test dang chu y: `spinner.test.tsx` co test **doi chung** render
`<button>Mua vé<Spinner /></button>` (khong `decorative`) va assert accessible name
`not.toHaveAccessibleName("Mua vé")`. Test nay PASS, tuc la da xac nhan bang may rang gia dinh cua
reviewer dung: `aria-label` cua con THUC SU nhap vao accessible name cua nut. Bug M-3 la bug that,
khong phai suy dien.
