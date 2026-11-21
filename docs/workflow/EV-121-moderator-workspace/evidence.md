# EV-121 — Bằng chứng

Ngày chạy: 2026-10-21 → 22. Máy: Windows 11, Docker Desktop 29.6.2. Go 1.26.9 trong `golang:1.26` (mount Docker
socket cho testcontainers); Node 22, Vitest 2.1.8. Nhánh `feature/EV-121-moderator-workspace` từ `develop` @
`43375eb`. Review và thử phá do người hiện thực tự làm — cần 1 reviewer khi mở PR.

## 1. Kết quả

| Kiểm | Kết quả |
|---|---|
| `services/event`: `go vet ./...`, `go vet -tags=integration ./...`, `gofmt` các file đã sửa | sạch |
| `services/event`: `go test -race ./...` | PASS — `adapter/http`, `app`, `domain`, `worker` |
| `services/event`: `go test -tags=integration ./internal/...` (Postgres 16 thật) | PASS — kể cả 5 test `TichHop_ApplyDecision_*` / `ListPendingHuman` mới |
| Cổng coverage `domain/` + `app/` | **79,7%** (baseline 78,7%; ngưỡng 70%) |
| golangci-lint v2.12.2 (bản LF) trên `services/event` | Không có lỗi trong code mới. Còn 8 lỗi có sẵn trên `develop` (`rows.Close` không kiểm lỗi, `ifElseChain`, gofmt `domain/event.go`, `domain/ticket_tier.go`); CI hiện không lint event |
| `apps/web`: `bash .github/scripts/gates.sh` (lint, typecheck, coverage, build, C4.2/C4.3/C4.5) | **TẤT CẢ GATE ĐẠT** — **2388 test / 155 file** (baseline 2342 / 152) |

## 2. Chạy thật (event service từ nhánh + Postgres của compose)

Event service chạy bằng `go run ./cmd/server` trong `golang:1.26`, nối mạng `eventflow_default`; gọi thẳng `:8084`
với header principal mà gateway gắn (`X-Identity-Id`, `X-Identity-Role`). Organizer tạo + thêm hạng vé + nộp 3 sự
kiện; giả lập callback AI qua `PATCH /internal/v1/events/{id}/ai-verdict` (kèm 4 trục).

| Bước | Kết quả |
|---|---|
| Moderator `GET /api/v1/moderation/queue` | A `PENDING_REVIEW / NEEDS_HUMAN`, B và C `REJECTED / AUTO_REJECTED` |
| `GET /api/v1/moderation/events/{A}` | nội dung gốc + 1 hạng vé; AI `NEEDS_HUMAN 0.55`; 4 trục `{copyright 0.7, forbidden_content 0.05, incomplete_info 0.2, misleading 0.4}`; audit `event.needs_human_review` |
| Lật ngược B, lý do rỗng | **422** `type: /problems/override-reason-required` |
| Lật ngược B có lý do | 200; DB: sự kiện `SCHEDULED`, review `APPROVED / AI nham…`, audit `event.moderator_overrode_ai` kèm lý do |
| Từ chối A có lý do | 200 |
| Hai Moderator lật ngược C **cùng lúc** | **200 và 409**, đúng **1** bản ghi audit |
| Organizer xem hàng đợi / tự duyệt | 403 / 403 |

### Lỗi chỉ lộ khi chạy thật, đã sửa

Lần chạy đầu của bước "hai Moderator cùng lúc" trả **200 và 422**: request thứ hai tới sau khi request đầu commit, đọc
thấy `SCHEDULED` và rơi vào `ErrInvalidStateTransition`. Moderator sẽ thấy "quyết định không hợp lệ" thay vì "người khác
vừa quyết". Sửa bằng `alreadyDecided()` (spec §4.3), thêm 3 test app; chạy lại → 200 / 409.

### Không chạy được qua trình duyệt

Web gọi qua gateway `:8080` cần JWT từ identity; gateway / identity / event chưa có trong compose (X3). Giao diện được
kiểm bằng Vitest + Testing Library (23 test trang) và `next build` trong gates.

## 3. Thử phá

| # | Đột biến | Kết quả | Bắt bởi |
|---|---|---|---|
| K1 | Bỏ bắt buộc lý do khi lật ngược | Đỏ | `TestModeratorApprove_LatNguocAI_ThieuLyDo_BiTuChoi` |
| K2 | `ApplyDecision` bỏ điều kiện `status = trạng thái cũ` | Đỏ | `TestTichHop_ApplyDecision_HaiQuyetDinhDongThoi_MotThang` |
| K3 | Không nhận diện "đã quyết" | Đỏ | `TestModeratorDecision_NguoiDenSau_LaDaQuyetChuKhongPhaiLoiTrangThai` |
| K4 | Hàng đợi không lọc review hết hiệu lực | Đỏ | `TestListModerationQueue_GomCanNguoiDuyetVaAIDaTuChoi` |
| K5 | Commit trước khi ghi audit | Đỏ | `TestTichHop_ApplyDecision_*` |
| K6 | 4 trục lấy bản AI cũ nhất | Đỏ | `TestModerationDetail_GomSuKienReviewTrucVaAudit` |
| K7 | Lật ngược AI ghi action chung | Đỏ | `TestModeratorApprove_LatNguocAI_CoLyDo_GhiAuditRieng` |
| U1 | UI không bắt lý do khi lật ngược | Đỏ | `lật ngược AI (sự kiện REJECTED)…` |
| U2 | UI bỏ xử lý 409 | Đỏ | `409: hồ sơ vừa được Moderator khác quyết định…` |
| U3 | Tìm kiếm không bỏ dấu | Đỏ | `tìm theo tên hoặc địa điểm, không phân biệt dấu` |
| U4 | Lọc "AI đã từ chối" = "không phải NEEDS_HUMAN" | **Sống sót → thêm hồ sơ thiếu review vào test → Đỏ** | `lọc 'AI đã từ chối' chỉ còn hồ sơ AUTO_REJECTED` |
| U5 | `isOverride` luôn false | Đỏ | contract + trang |
| U6 | Bỏ câu miễn trừ AI | Đỏ | `AI chỉ là khuyến nghị…` |
| U7 | 403 hàng đợi hiện "Thử lại" | Đỏ | `403 từ API…` |
| U8 | Gửi `moderator_id` trong body | Đỏ | contract `approveEvent` |

## 4. Ghi nhận

- Một test có sẵn đổi **dữ liệu dựng** (không đổi kỳ vọng): `TestHTTP_ModerationQueue` tạo sự kiện **không có hạng vé**
  nên `SubmitForReview` thất bại (BR-E1, lỗi bị `_ =`) và sự kiện ở `DRAFT`; hàng đợi cũ không kiểm trạng thái nên vẫn
  xanh. Thêm hạng vé để dữ liệu hợp lệ.
- Test của `apps/web` bắt được lỗi thật trong lúc viết: `DecisionPanel` có `key={status}` nên duyệt xong bị dựng lại và
  **mất thông báo "Đã duyệt"** — đưa trạng thái "vừa quyết" lên `ReviewDetail`.
- Cảnh báo `act(...)` khi render component server async trong test (cùng khuôn với trang organizer có sẵn, 7 cảnh báo
  ở `review-status`). Không làm test sai; test 409 được siết để chờ dữ liệu tải lại thật sự được áp dụng.
- Tạo sự kiện trùng slug trả 500 (có sẵn) — xem spec §5.
