# Contracts — đóng băng trước Test Design

Test Design viết test **chỉ** từ `spec.md` + file này, không đọc phần hiện thực.

## K1 — `.github/scripts/run-and-report-tests.sh` (AC-4)

```
bash .github/scripts/run-and-report-tests.sh <test-regex> <cmd> [args...]
```

| | |
|---|---|
| Chạy | `<cmd> [args...]`; stdout + stderr **vừa in ra console vừa được ghi lại** |
| Đếm | số dòng khớp `^[[:space:]]*--- (PASS\|FAIL\|SKIP): <test-regex>` (định dạng `go test -v`), tách theo PASS / FAIL / SKIP |
| Summary | nếu `GITHUB_STEP_SUMMARY` được đặt ⇒ **append** (không ghi đè) một bảng markdown có ba số đếm và exit code. Không đặt ⇒ không ghi file nào |
| Exit | `<cmd>` exit ≠ 0 ⇒ script exit **đúng mã đó**. `<cmd>` exit 0 nhưng FAIL > 0 ⇒ exit `1`. Ngoài ra exit `0` |
| Cảnh báo | `<cmd>` exit 0 **và** PASS = 0 ⇒ in ra stdout **đúng một** dòng bắt đầu bằng `::warning` và chứa chuỗi `KHONG kiem gi`. Áp dụng cả khi SKIP = 0 (regex không khớp test nào) |
| Không cảnh báo | PASS > 0 ⇒ **không** có dòng `::warning` nào |
| Tham số | thiếu `<test-regex>` hoặc `<cmd>` ⇒ in cách dùng ra stderr, exit `2` |
| Regex *(bổ sung sau Challenge vòng 1)* | `<test-regex>` là ERE, được **nhóm** trước khi ghép (`TestA\|TestB` nghĩa là một trong hai tên, không phải "TestA hoặc TestB ở bất kỳ đâu"). Một `^` ở đầu và một `$` ở cuối — cú pháp quen của `go test -run` — được **bỏ** trước khi ghép, vì tên test đứng giữa dòng `--- PASS: ` |
| Fail-closed *(bổ sung sau Challenge vòng 1)* | regex không hợp lệ (`grep` exit ≥ 2) hoặc không tạo được file tạm ⇒ in lỗi ra stderr, exit `2`. Không được rơi về "đếm = 0, exit 0" |
| Giới hạn đã biết | đếm dựa trên định dạng `-v`: dòng `--- PASS` dính sau output không xuống dòng thì không đếm được; `t.Log` in ra chuỗi giống `--- FAIL: <regex>` thì bị đếm là FAIL (hỏng theo hướng **đỏ**, an toàn). `go test -json` bền hơn nhưng là thay đổi lớn hơn — chưa làm |
| Môi trường | chạy được trên `ubuntu-latest` **và** Git Bash Windows; chỉ dùng bash + coreutils + grep |

Ví dụ dòng đầu vào nó phải đếm đúng (lưu ý thụt lề của subtest):

```
=== RUN   TestNoOversell_UnderExtremeConcurrency
    helpers.go:33: TODO(EVF-39): can hien thuc testcontainers truoc khi bat test nay
--- SKIP: TestNoOversell_UnderExtremeConcurrency (0.00s)
--- PASS: TestNoOversell_Basic (1.20s)
    --- PASS: TestNoOversell_Basic/sub (0.10s)
--- FAIL: TestNoOversell_Edge (0.30s)
```

Với `<test-regex>` = `TestNoOversell`: SKIP = 1, PASS = 2, FAIL = 1.

## K2 — Makefile `test-oversell` nhận thêm cờ (AC-4)

`make test-oversell TESTFLAGS=-v` ⇒ thêm `-v` vào lệnh `go test` hiện có. Không truyền `TESTFLAGS` ⇒
lệnh **y hệt** hiện tại (chạy tay không đổi hành vi).

## K3 — Tên job hiện có không được đổi (AC-11)

`ci.yml`: `detect-changes`, `lint-go`, `lint-web`, `lint-python`, `test-go`, `test-web`,
`test-python`, `build`. `trivy.yml`: `detect-changes`, `trivy`, `trivy-gate`. `cd-web.yml`:
`build & push ghcr web`. `integration.yml`: `integration`. `oversell-gate.yml`: `oversell-gate`.
`e2e-nightly.yml`: `e2e`. Job **mới** được phép thêm.

## K4 — Hình dạng của CD sau khi sửa (AC-2, AC-3)

Trong job `build & push ghcr web`, theo đúng thứ tự:

1. build image vào daemon local (**không** push): `docker/build-push-action` với `load: true`, không
   có `push: true`
2. quét Trivy image đó: `severity: CRITICAL`, `ignore-unfixed: true`, `exit-code: '1'`
3. `docker push` các tag — **chính image đã quét**, không build lại. Bước này chỉ chạy khi bước 2
   thành công (mặc định của GitHub Actions: step fail ⇒ các step sau không chạy)

Build-arg: `NEXT_PUBLIC_API_BASE=${{ vars.NEXT_PUBLIC_API_BASE }}`. Biến rỗng ⇒ một dòng `::warning`.
Biến **không** được nội suy thẳng vào script shell (`run:`); đi qua `env:` của step.

Mọi `uses:` của action bên thứ ba trong `cd-web.yml` (không phải `actions/*`) có dạng
`owner/repo@<40 ký tự hex>` (kèm comment tag là tuỳ chọn).

*(Bổ sung sau Code Review + Challenge vòng 1)*
- Step Trivy **không** có `if:`; `image-ref` là một trong các ref được `docker push`; mọi ref được
  `docker push` là một tag của `build-push-action`.
- `docker/login-action` đứng **sau** step Trivy và **trước** `docker push`: token `packages: write`
  không được nằm trong `~/.docker/config.json` lúc binary Trivy (tải về lúc chạy) thực thi.
- `actions/checkout` có `persist-credentials: false`.

## K5 — Hình dạng của job gate (AC-8)

Áp dụng cho `trivy-gate` (`trivy.yml`) và `build` (`ci.yml`):

- `if:` của job **đúng bằng** `always()` — không chứa `failure`, `cancelled`, `contains`
- có một step mà, khi `needs.*.result` chứa `failure` hoặc `cancelled`, in `::error` và `exit 1`
- các step khác của `build` giữ nguyên điều kiện theo filter
- *(bổ sung sau Challenge vòng 1)* `if:` của **step** gate bỏ trống hoặc **đúng bằng** `always()`
  (không có thì step mang `success()` ngầm và có thể bị skip khi run bị huỷ); step và job **không** có
  `continue-on-error`; step so kết quả biểu thức với đúng literal `"true"`
- *(bổ sung sau Challenge vòng 1)* `needs:` của gate chứa **mọi** job id khác trong cùng file — job mới
  thêm vào `ci.yml` phải vào `build.needs`, nếu không nó fail mà `build` vẫn xanh

