# Kịch bản demo backend EventFlow (≈ 5 phút)

Người trình bày: chạy lệnh và dẫn. Khán giả: nhóm dự án, giảng viên, người quan tâm hệ thống chịu tải.
Phần demo chạy **một lệnh** (`make demo`); tài liệu này chỉ nói *chiếu gì, nói gì, khi nào*.

## Chuẩn bị (làm trước 10 phút)

```bash
make down                      # xoá sạch volume cũ
make up && make migrate        # ~30 giây; 18 container
make seed                      # ~5 giây: tạo 4 tài khoản demo
```

Mở sẵn 4 tab trình duyệt: **Mailpit** http://localhost:8025 · **RabbitMQ** http://localhost:15672 (`eventflow`/`eventflow`) ·
**Jaeger** http://localhost:16686 · **Grafana** http://localhost:3001. Một cửa sổ terminal cỡ chữ lớn ở thư mục gốc repo.

Kiểm nhanh: `curl -s localhost:8080/healthz` trả `ok`. Nếu chưa, đợi thêm 20 giây rồi kiểm lại.

Tài khoản (mật khẩu chung `Demo@123456`): `organizer@`, `moderator@`, `ops@`, `demo@eventflow.local`.

## Diễn biến (thời gian đo khi diễn tập 2 lần từ volume trống: 101–104 giây)

Chạy `make demo`. Mỗi bước in `ok` khi xong; nói trong lúc nó chạy, không đợi.

| Giây | Bước trên màn hình | Nói gì | Chiếu gì |
|---|---|---|---|
| 0–2 | 0–1. Đăng ký khách, OTP, đăng nhập | "Mọi thứ đi qua một cổng duy nhất là gateway; các service bên trong không mở ra ngoài." | Tab Mailpit: mail OTP vừa tới |
| 2–6 | 2. Organizer tạo sự kiện, nộp duyệt | "AI kiểm duyệt nội dung trong ~1 giây; sự kiện đáng ngờ mới chuyển cho Moderator." | Terminal |
| 6–80 | 3. Chờ giờ mở bán | **Đây là lúc nói về kiến trúc** (xem "Lời dẫn khi chờ" bên dưới). Khách đã vào phòng chờ nhưng chưa ai được mua | RabbitMQ (hàng đợi `payment.events.sink.q`, `ai.*`), Grafana, Jaeger |
| 80 | Đến giờ mở bán | "Bốc thăm xong: người bấm sớm không có lợi thế. Sự kiện tự chuyển `ON_SALE`, không ai bấm tay." | Terminal: `ADMITTED`, `ON_SALE` |
| 80–84 | 4–5. Giữ ghế → thanh toán → **vé tự phát** → email → kiểm QR → check-in | "Webhook thanh toán ghi `order.paid` cùng transaction với đơn; relay đưa lên RabbitMQ, ticketing tự phát vé, notification tự gửi mail. QR sửa 1 ký tự bị từ chối; check-in chỉ nhân viên làm được và quét lại lần hai là 409." | **Mailpit: 2 mail mới** (xác nhận thanh toán, vé điện tử). Tab RabbitMQ: số message tăng rồi về 0 |
| 84–88 | 6. Khách 2 huỷ giữ ghế | "Huỷ trả kho đúng một lần dù gọi lại." | Terminal |
| 88 | 7. Bảo mật | "Giả header danh tính, xem đơn người khác, dùng token của người khác: đều bị chặn." | Terminal |
| 88–89 | 8. Trợ lý AI | "Khách chờ có thể hỏi trợ lý; có giới hạn tốc độ để không chạm trần Gemini." (Đang chạy chế độ giả lập `GEMINI_MOCK=1`) | Terminal |
| 89–101 | 9. Hết vé | "Hết vé thì người đang chờ được báo ngay, không bị đẩy vào hàng vô vọng." | Terminal |

Kết thúc: dòng `[demo] DAT: 36 buoc trong ~95s`.

### Lời dẫn khi chờ giờ mở bán (~70 giây)

- **Vấn đề:** 300.000 người bấm cùng lúc cho ~20.000 vé. Phòng chờ ảo biến đỉnh tải thành dòng chảy đều: Postgres chỉ thấy ~200 yêu cầu/giây dù hàng triệu người xếp hàng.
- **Công bằng:** trước giờ mở bán ai vào cũng ở `LOBBY`; đúng giờ bốc thăm ngẫu nhiên.
- **Không bán quá vé:** bốn lớp độc lập (Redis Lua → `UPDATE … WHERE available >= qty` → `CHECK (available >= 0)` → đối soát 10 giây).
  Có thể chạy `make test-oversell` (10.000 goroutine tranh 100 vé) nếu khán giả hỏi.
- **Không mất sự kiện:** ghi trạng thái và outbox cùng transaction, relay phát lại đến khi broker xác nhận, consumer idempotent.

## Khi có sự cố

| Triệu chứng | Làm gì |
|---|---|
| `dang nhap … that bai: chay 'make seed' truoc` | `make seed` rồi chạy lại |
| `khong thay email OTP` | Mailpit chưa lên: `docker compose -f deploy/compose/docker-compose.yml ps mailpit`; chạy lại `make demo` |
| Kẹt ở "cho gio mo ban" quá 3 phút | `docker compose … logs --tail=50 waitingroom-controller ticketing-worker`; thường do worker vừa khởi động lại, chờ 30 giây rồi chạy lại |
| `don khong sang ISSUED trong 20s` | Kiểm RabbitMQ tab: `ticketing.order-paid.q` có consumer không? `docker compose … restart ticketing-worker payment` rồi chạy lại |
| Bất kỳ bước nào FAIL | Script dừng ngay và in phản hồi cuối. Chạy lại `make demo` được ngay: mỗi lần tạo sự kiện và người mua mới |
| Mất quá nhiều thời gian | Phương án rút gọn: chạy `make smoke` (≈ 40 giây, không đăng ký/organizer) và mở `tests/demo/eventflow.http` để bấm vài bước tay |

Giảm thời gian chờ khi diễn tập nội bộ: `DEMO_LEAD=65 make demo` (tối thiểu hơn 60 giây, là `MIN_SALE_LEAD_SECONDS` của compose).
Bỏ bước hết vé: `DEMO_SOLDOUT=0 make demo`.

## Biết trước (nói thật nếu bị hỏi)

- Đây là bản demo trên máy cục bộ; chưa chạy load test 100.000 người, chưa có anti-bot (EV-173…177), chưa có K8s/HPA.
- Trợ lý AI ở chế độ giả lập: trả lời mẫu, chưa gọi Gemini thật.
- Email vé hiện hiển thị chữ ký QR dạng chữ, chưa có ảnh QR.
- Trace Jaeger mới có waitingroom, ticketing, ticketing-worker; gateway, identity, event chưa gắn tracing (EV-103).
