# Review log — Plan Review, Code Review, Challenge

Mỗi bước do một agent riêng, context sạch, chỉ đọc. Bảng dưới ghi **phát hiện → xử lý**, không chép
lại toàn văn báo cáo.

## Plan Review

| Vòng | Kết luận | Phát hiện chính |
|---|---|---|
| 1 | KHÔNG ĐẠT (1 blocker, 3 major, 7 minor) | **B3**: điều kiện "không có job fail" đặt ở `if:` của job ⇒ job bị **skip** ⇒ GitHub tính là Success, kể cả required check (tài liệu GitHub) — áp cho `trivy-gate` và `build`. SC2129 plan sửa sai step. PR sửa `ci.yml` không chạy `lint-go`. `trivy-action` pin bằng tag trong job `packages: write`. |
| 2 | KHÔNG ĐẠT (1 major) | 11/11 phát hiện vòng 1 đã giải quyết; 4 SHA khớp `git ls-remote`. **N1**: chính PR này đỏ 3 job (`lint-go`, `test-go` coverage 0%, `lint-python`) + `build` ⇒ **leo thang cho người dùng** (quyết định thứ tự merge, không phải thiết kế). |

## Code Review (bước 7) — ĐẠT, 7 minor

| # | Phát hiện | Xử lý |
|---|---|---|
| 1 | `COPY services ./services` kéo file chưa track của mọi service (`.env`, `*.pem`) vào build stage; mất cache khi sửa service khác | **Sửa**: chỉ copy `go.mod` của 7 module không dùng. Kiểm: thả `.env` + `stripe.pem` giả ⇒ 0 file lọt vào build stage; build xanh |
| 2 | Comment khẳng định E2E đỏ do image này; spec chỉ nói "nhiều khả năng" | **Sửa** câu chữ |
| 3 | Run bị huỷ: step gate mang `success()` ngầm có thể bị skip ⇒ gate không fail | **Sửa**: `if: always()` ở cả **step** gate (`build`, `trivy-gate`) |
| 4 | Token `packages: write` nằm trong `~/.docker/config.json` (login trước build) và `.git/config` (checkout) trong lúc binary Trivy tải về lúc chạy thực thi | **Sửa**: login dời xuống ngay trước push; `persist-credentials: false`; `TRIVY_IMAGE_SRC: docker` |
| 5 | Action bên thứ ba ở workflow chỉ-đọc vẫn pin tag; không có Dependabot | **Không sửa** — ngoài AC (AC-2 chỉ bắt `cd-web.yml`); ghi lại |
| 6 | Comment `# v3` không ghi bản cụ thể | **Sửa**: `# v3.12.0`, `# v3.7.0`, `# v6.19.2`, `# v0.36.0` (reviewer đối chiếu `ls-remote`) |
| 7 | Dòng summary API base chỉ ghi ở step cuối, mất khi Trivy fail | **Sửa**: ghi ngay ở step kiểm |

## Challenge (bước 8) — BỊ PHÁ

| # | Mức | Phát hiện | Xử lý |
|---|---|---|---|
| 1–4 | major/minor | Bộ test có lỗ: 9 đột biến làm sai hành vi vẫn qua 105/105 + actionlint (`continue-on-error` / `if:` lạ trên step gate, rút `needs`, so `"True"`, `if:` trên step Trivy, `image-ref` khác image push, `\|\| true` sau oversell, bỏ `export BASE_REF`) | **Giao lại Test Design** (chủ sở hữu test), kèm từng đột biến; yêu cầu chứng minh mỗi đột biến bị bắt |
| 5 | major | Filter `trivy.yml` thiếu `go.work`, `services/**`, `.dockerignore` — lộ ra **do chính bản sửa Dockerfile**. Repro: đổi `go` directive ở `services/gateway/go.mod` ⇒ image ticketing đỏ, Trivy không chạy | **Sửa** filter |
| 6 | minor | K1: `TestA\|TestB` không được nhóm ⇒ đỏ oan | **Sửa**: nhóm `(${pattern})`; repro hết lỗi |
| 7 | minor | K1: `^TestX$` không bao giờ khớp ⇒ cảnh báo oan | **Sửa**: bỏ một `^` đầu, một `$` cuối |
| 8 | minor | K1 **fail-open**: regex hỏng / `mktemp` hỏng ⇒ exit 0, không cảnh báo | **Sửa**: exit 2; repro `Test[` ⇒ rc 2, `TMPDIR=/nonexistent` ⇒ rc 2 |
| 9 | minor | K1 đếm sai output `-v` thật (dòng `--- PASS` dính sau `fmt.Print`; `t.Log` in `--- FAIL:`) | **Không sửa** — ghi vào contract K1 là giới hạn đã biết; hướng hỏng của ca thứ hai là **đỏ** (an toàn). `-json` là thay đổi lớn hơn |

**Không có Challenge vòng 2 riêng.** Bản tăng cứng test được kiểm bằng: (a) Test Design tự áp 29 đột
biến (gồm cả 9 đột biến của challenger) lên bản sao; (b) implementer tự áp lại một đột biến (M5) và
thấy bị bắt; (c) Verification độc lập (dưới). Ghi rõ để không ai đọc thành "đã qua Challenge vòng 2".

**Không có Plan Review vòng 3 riêng.** Plan vòng 3 chỉ áp 5 minor của vòng 2 (N2–N6); major còn lại
(N1) là quyết định leo thang cho người dùng, không phải thiết kế. Phần hiện thực thay vào đó đi qua
Code Review + Challenge + Verification. Mở rộng phạm vi AC-12 (Dockerfile Go) cũng **chưa** qua Plan
Review — nó được Code Review và Challenge soi trực tiếp.

## Verification độc lập (bước 9)

| Vòng | Kết luận | Phát hiện | Xử lý |
|---|---|---|---|
| 1 | **KHÔNG ĐẠT** | **Blocker trong bộ test**: hai assertion K2 fail trên Linux **có GNU make** — nhánh `make -n` không nối dòng tiếp nối `\`. Bộ test chỉ xanh trên máy không có make (Git Bash, ubuntu trơn) vì đi nhánh mô phỏng awk; runner `ubuntu-latest` có make ⇒ chính `lint-ci` của PR này sẽ đỏ. Có bằng chứng thật: `ubuntu:24.04` + `apt install make` ⇒ 126/128 | Giao lại Test Design (chủ sở hữu), yêu cầu chạy lại trong Linux có và không có make |
| 1 | | Bằng chứng **thật** đạt: AC-4 (trong `golang:1.23`: 400 SKIP ⇒ đúng 1 `::warning`, exit 0; test FAIL giả ⇒ exit 1; PASS giả ⇒ không cảnh báo), AC-10, AC-12 (Dockerfile gốc exit 1, đã sửa exit 0), `gates.sh` exit 0, 4 SHA khớp, phạm vi đúng | — |
| 1 | | 13 mâu thuẫn tài liệu: số trước Challenge còn trong `gate-report.md`; khẳng định "Trivy đỏ vì pgx" đã rút ở spec §1.3 nhưng còn ở spec B2 và traceability; plan còn mô tả cách grep `NEEDS_JSON` đã bỏ; spec còn ghi "chưa lấy được job/step" và "E2E chưa biết nguyên cớ" dù §1.4 đã có; không ghi rõ không có Plan Review vòng 3 / Challenge vòng 2 | Sửa ở các file của implementer (spec, plan, gate-report, review-log); file của Test Design (baseline, traceability) giao lại |
| 2 | **ĐẠT** | Bộ test đúng ở cả 4 cấu hình (Linux có/không make × worktree/base); nhánh make bắt được đột biến `-count=200` và `-tags=integration`; 13/13 mâu thuẫn vòng 1 đã xử lý; 3 lỗi chữ mới (C2-1..C2-3) | Sửa cả ba; C2-3 nằm trong `baseline.md` (file của Test Design) — Integrator sửa chữ và ghi chú tại chỗ |

Challenger cũng **không phá được**: K4 (build → quét → push đúng image), K5 (mẫu khớp
`re-actors/alls-green`), `gates-web` với `before` = 40 số 0 / SHA không tồn tại / force-push / lịch sử
orphan (đều fail rõ ràng), injection, lỗi biên dịch ở service khác không kéo ticketing đỏ.
