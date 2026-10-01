# Đóng góp vào EventFlow

Backlog và tiêu chí nghiệm thu nằm ở Jira (project **EVF**) —
xem [docs/04-jira-backlog.md](docs/04-jira-backlog.md). Tài liệu này chỉ nói
về quy trình Git/GitHub.

## Luồng nhánh

```
main (bảo vệ, luôn deploy được)
 ^
 |  PR bắt buộc CI xanh + Trivy + oversell-gate (200 lần) + 1 review
develop (bảo vệ, tích hợp)
 ^
 |  PR bắt buộc CI xanh + integration test + 1 review
feat/EVF-123-mo-ta-ngan   (nhánh làm việc, tạo từ develop)
fix/EVF-123-mo-ta-ngan
```

- Tạo nhánh từ `develop`, đặt tên `feat/EVF-<mã>-mo-ta-ngan` hoặc
  `fix/EVF-<mã>-mo-ta-ngan`.
- Không commit thẳng vào `main`/`develop` — luôn qua Pull Request.
- Commit theo [Conventional Commits](https://www.conventionalcommits.org/),
  kèm mã Jira: `feat(ticketing): EVF-31 them redis hold gate`.
- Mở PR vào `develop`; PR vào `main` chỉ để phát hành (release), gộp từ `develop`.

## CI chạy gì trên mỗi PR

| Workflow | Nội dung | Bắt buộc cho |
|---|---|---|
| `CI` | lint, unit test, coverage domain/app ≥ 70%, build | `develop`, `main` |
| `Integration` | test tích hợp (testcontainers) + oversell smoke 20 lần | `develop`, `main` |
| `Trivy scan` | quét lỗ hổng CRITICAL trên 4 image đã có Dockerfile | `develop`, `main` |
| `Oversell Gate (EVF-39)` | `TestNoOversell` chạy đủ **200 lần** | chỉ `main` |
| `E2E nightly` | Playwright luồng đầy đủ trên `make up` | lịch đêm + thủ công, không chặn PR |

Chi tiết từng job nằm trong `.github/workflows/`.

## Trước khi mở PR

```
make lint
make test
make test-integration   # can Docker
```

## Definition of Done

Xem checklist trong template PR (tự động điền khi tạo PR) và mục
*Definition of Done* trong [docs/04-jira-backlog.md](docs/04-jira-backlog.md).
