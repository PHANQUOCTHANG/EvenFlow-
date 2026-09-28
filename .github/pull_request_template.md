## Mã Jira

<!-- EVF-123 -->

## Tóm tắt thay đổi

<!-- Thay đổi gì, tại sao. -->

## Loại thay đổi

- [ ] Tính năng mới
- [ ] Sửa lỗi
- [ ] Tái cấu trúc / dọn dẹp
- [ ] Tài liệu
- [ ] Hạ tầng / CI

## Definition of Done (đối chiếu docs/04-jira-backlog.md)

- [ ] Thoả toàn bộ AC của issue Jira liên quan
- [ ] Có test tự động cho thay đổi (unit và/hoặc integration)
- [ ] CI xanh: lint, unit test, build (và integration/Trivy nếu áp dụng)
- [ ] Coverage `domain/` và `app/` vẫn ≥ 70%, không phá vỡ EVF-39
- [ ] Không log PII dạng thường; có metric/log/trace cho luồng mới nếu cần
- [ ] Đã cập nhật tài liệu liên quan (OpenAPI, docs/, runbook) nếu hành vi thay đổi
- [ ] Đã tự chạy được trên local (`make up` / `make test`)

## Cách kiểm chứng

<!-- Lệnh hoặc bước cụ thể để reviewer tự chạy lại và thấy kết quả. -->
