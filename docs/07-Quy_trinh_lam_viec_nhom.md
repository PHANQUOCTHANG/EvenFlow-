# Quy trình làm việc nhóm & Hướng dẫn đóng góp - EvenFlow

Tài liệu này định nghĩa các nguyên tắc làm việc, quản lý mã nguồn (Git) và các quy chuẩn cốt lõi nhằm giúp các thành viên trong đội phát triển EvenFlow phối hợp trơn tru, hiệu quả và hạn chế xung đột (conflict).

---

## 1. Quy chuẩn Quản lý Mã nguồn (Git Flow)

Dự án áp dụng mô hình **Git Flow** tiêu chuẩn, tách biệt môi trường đang phát triển và môi trường đã ổn định.

### 1.1 Cấu trúc Branch (Nhánh)
- `main`: Nhánh gốc chứa mã nguồn ổn định nhất, luôn ở trạng thái sẵn sàng để deploy lên môi trường Production. **Tuyệt đối không push trực tiếp (commit) lên nhánh này.** Chỉ nhận code đã được test kỹ từ `develop` hoặc từ nhánh `hotfix`.
- `develop`: Nhánh tích hợp chính (Nguồn sự thật cho môi trường Dev/Staging). Chứa các tính năng mới nhất của dự án. **Tất cả các nhánh feature đều phải tách ra từ đây.**

- `feature/<Mã_Jira>-<ten-tinh-nang>`: Nhánh để phát triển tính năng mới, phân nhánh từ `develop`. Ví dụ: `feature/EVF-39-admit-controller`.
- `bugfix/<Mã_Jira>-<ten-bug>`: Nhánh để sửa lỗi trong quá trình test trên staging. Tách từ `develop`. Ví dụ: `bugfix/EVF-45-fix-redis-oversell`.
- `hotfix/<Mã_Jira>-<ten-loi-nghiem-trong>`: Nhánh sửa lỗi khẩn cấp, **tách từ nhánh `main`** (bản Production). Sau khi fix xong, bắt buộc phải merge ngược lại vào cả `main` và `develop`.

### 1.2 Quy trình tạo Pull Request (PR) / Merge Request (MR)
1. Cập nhật nhánh `develop` mới nhất: `git checkout develop && git pull origin develop`
2. Tạo nhánh làm việc: `git checkout -b feature/EVF-XX-short-desc`
3. Lập trình và Commit. Gộp (squash) các commit nhỏ lẻ (như "fix typo", "save") trước khi mở PR.
4. Mở Pull Request trỏ vào nhánh `develop` (KHÔNG trỏ vào `main`).
5. **Yêu cầu PR:**
   - Phải có ít nhất **1 Approve** từ reviewer (thành viên khác).
   - Pass toàn bộ CI/CD pipeline (Lint, Unit Test, Testcontainers).
   - Không làm giảm độ bao phủ code (Code Coverage).

---

## 2. Quy chuẩn Viết Commit Message (Conventional Commits)

Để công cụ CI/CD tự động sinh ra Release Note và Changelog, mọi commit phải tuân thủ cú pháp:
```
<type>(<scope>): <subject>
```

**Các `<type>` hợp lệ:**
- `feat`: Thêm tính năng mới. (VD: `feat(ticketing): thêm cơ chế outbox cho orders`)
- `fix`: Sửa lỗi bug. (VD: `fix(waitingroom): sửa lỗi tính sai rank trong ZSET`)
- `refactor`: Tái cấu trúc code nhưng không thay đổi logic/chức năng.
- `docs`: Thêm/sửa đổi tài liệu dự án.
- `chore`: Các tác vụ nhỏ lẻ, cài đặt thư viện, cấu hình CI/CD.
- `test`: Bổ sung hoặc sửa unit/integration test.

**Lưu ý:** Chữ cái đầu tiên của subject phải viết thường, không có dấu chấm câu ở cuối.

---

## 3. Các Lệnh Chạy Dự Án Cục Bộ (Local Environment)

Vì dự án chạy kiến trúc microservices (Go/Python/Next.js) kèm Redis, Postgres, RabbitMQ, tất cả môi trường dev đã được chuẩn hóa qua `Make` và `Docker Compose`.

Mở terminal tại thư mục gốc của dự án và sử dụng:

| Lệnh | Ý nghĩa & Khi nào dùng |
|---|---|
| `cp .env.example .env` | Dùng 1 lần duy nhất lúc mới clone repo về. |
| `make up` | Khởi động toàn bộ hạ tầng (DB, Redis, MQ) và các microservices ở background. |
| `make down` | Tắt dọn dẹp các container. |
| `make migrate` | Chạy Goose để cập nhật cấu trúc DB (PostgreSQL) lên phiên bản mới nhất. |
| `make seed` | Khởi tạo dữ liệu mẫu: Sinh ra 1 sự kiện trạng thái ON_SALE với 20.000 vé. |
| `make smoke` | Chạy kịch bản mua vé mô phỏng (tự động) để test nhanh hệ thống. |
| `make test-oversell` | **RẤT QUAN TRỌNG:** Chạy bài test ép tải 10,000 luồng. Nếu fail báo cáo ngay lập tức! |

---

## 4. Quy chuẩn Code (Code Style & Linter)

Để giữ cho source code đồng nhất dù nhiều người viết:

- **Golang (Backend Services):**
  - Luôn chạy `go fmt` và `go vet` trước khi commit.
  - Sử dụng `golangci-lint` (CI sẽ chặn nếu không pass linter).
  - Bắt buộc xử lý lỗi triệt để, không bỏ qua lỗi bằng `_`.

- **Python (AI Worker):**
  - Sử dụng `black` để format code.
  - Sử dụng Type Hint đầy đủ.
  - File cấu hình đã có sẵn `pyproject.toml`.

- **Next.js (Frontend Web):**
  - Sử dụng `Prettier` với file `.prettierrc` mặc định.
  - Tuân thủ cấu trúc của ESLint.

---

## 5. Quy tắc Bất biến (Thiết quân luật của dự án)

Nhằm đảm bảo an toàn sinh tử cho dự án bán vé chịu tải cao:
1. **Tuyệt đối không sửa DB ngoài Transaction:** Mọi giao dịch thay đổi tài chính, tồn kho, tạo order phải được đặt trong một DB Transaction an toàn.
2. **Không gọi API bên thứ ba (Third-party) chặn luồng (Sync Call):** Bất cứ tác vụ nào liên quan đến LLM (Gemini), Gửi Email, Cổng thanh toán đều phải ném vào Message Queue (RabbitMQ) và trả về HTTP 202 Accepted.
3. **Không bao giờ bypass (bỏ qua) bài `test-oversell`:** Chống bán vượt (Oversell) là uy tín của nền tảng. Bất kỳ thay đổi code nào làm fail bài test concurrency này đều bị cấm merge.
4. **Viết Log có tâm:** Log phải in theo format JSON (Structured Logging) và luôn đính kèm `trace_id`, `event_id`, và `identity_id` (nếu có) để phục vụ việc truy vết trên Kibana/Grafana sau này. Không ghi log PII (thông tin cá nhân nhạy cảm).
