# EV-157 — Review code (Code review)

| Mục | Giá trị |
|---|---|
| Người review | Người hiện thực tự review, 2026-10-17 — **không phải reviewer context sạch** như EVF-7 / EV-142; cần 1 reviewer người khi mở PR |
| Đầu vào | Thay đổi chưa commit trên `feature/EV-157-ai-worker-consumers` so với `develop` @ `c241ea1` |
| Đối chiếu | `spec.md` (AC-1..AC-11), `plan.md`; `libs/go/mq/consumer.go` (giao thức); mã aio-pika 10.1.0, google-genai 2.29.0 |
| Kết luận | **PASS sau khi sửa** — 2 major (B1, B2), 3 minor (m1, m2, m4) đã sửa trong slice; còn lại ghi nhận |

## 1. Phát hiện và xử lý

| # | Mức | Phát hiện | Xử lý |
|---|---|---|---|
| B1 | **Major** (nghiệp vụ, BR-A1) | Limiter từ chối → `Retry` → sau 3 lần job chat vào DLQ, khách không bao giờ nhận câu trả lời. Lúc mở bán limiter từ chối là bình thường (5.000 câu/phút vs 55 RPM), nên đây là đường đi chính chứ không phải ngoại lệ | Lần thử cuối (`attempt ≥ MAX_ATTEMPTS`) gặp `Retry` → trả câu trả lời dự phòng, ack, outcome `fallback`. Test `LanCuoiLimiterVanTuChoi_TraLoiDuPhongVaAck`, `LanCuoi429_CungTraLoiDuPhong`, `ChuaToiLanCuoi_VanRetry` |
| m1 | Minor | Sampler khai báo passive trên channel của pool: queue bị xoá → `404` đóng channel → pool chết theo | Sampler dùng channel riêng, hỏng thì mở lại ở nhịp sau |
| m2 | Minor | `runtime.stop` đăng ký vào `AsyncExitStack` **sau** `start()`: pool thứ 2 lỗi thì worker của pool 1 bị bỏ rơi | Đăng ký `stop` trước `start`; `stop` chịu được pool chưa mở |
| m3 | Ghi nhận | `Retry(delay_ms)` của limiter bị bỏ qua: luôn chờ 5 giây cố định của `ai.chat.retry` | Đúng thiết kế topology (hàng trễ TTL cố định); ghi trong spec §4.3 |
| m4 | Minor | Log của uvicorn (`INFO: Uvicorn running…`) vẫn dạng text — uvicorn tự gắn handler riêng; `color_message` (mã ANSI) lọt vào JSON | **Đã sửa (2026-10-18):** `uvicorn --log-config app/logging.json` dùng `JsonFormatter` từ dòng đầu; `logx.configure` thay handler uvicorn; bỏ `color_message`. Compose: 0 dòng log không phải JSON |
| B2 | **Major** (bất biến L6) | Kiểm duyệt nhánh Gemini thật gọi thẳng `generate`, bỏ qua limiter (O3 trong spec) — slice này bật nhánh thật nên lỗ này thành thật | **Đã sửa (2026-10-18):** `app/gemini/guarded.py` là đường duy nhất (chat dùng lại, bỏ bản tự viết); kiểm duyệt truyền `generate` đã qua limiter; lần cuối vẫn không gọi được → `NEEDS_HUMAN` |
| m5 | Ghi nhận | `GeminiClient.embed` không đi qua limiter | Quota embedding tách riêng với quota generate; đưa vào EV-158 khi đo |
| m6 | Ghi nhận | `moderation.make_handler` coi `AttributeError` là lỗi vĩnh viễn (body JSON không phải object) — có thể che một bug lập trình | Message vào DLQ chứ không mất; chấp nhận |

## 2. Đối chiếu AC

| AC | Kết luận | Bằng chứng chính |
|---|---|---|
| AC-1 | Đạt | Tích hợp `ReportNang_KhongLamChatTreQua5Giay[async/thread]`; đột biến M19 (chặn event loop) làm test đỏ |
| AC-2 | Đạt | `LagMetric_BonQueueVaVeKhongKhiTrong`; `/metrics` trên compose có đủ 4 nhãn |
| AC-3 | Đạt | `PoolChaySongSongDungSoConsumer` (thêm sau khi M1, M2 sống sót) |
| AC-4 | Đạt | `KhaiBaoTopology_HaiLanVanOK`, `KhaiBaoTopology_LechThamSoThiLoi`; compose từ volume trống tự có 9 queue |
| AC-5 | Đạt | Unit `test_protocol.py`, `test_runtime.py`; tích hợp `LoiTam_TuPhucHoi`, `HetLuot_VaoDLQ`, `LoiVinhVien_VaoDLQNgay`, `HeaderAttemptKieuGo` |
| AC-6 | Đạt | `test_results.py`; tích hợp `Chat_DauCuoi_KetQuaVaoRedis`; compose: key có TTL 296 giây |
| AC-7 | Đạt (SDK giả) | `test_gemini_client.py`. **Chưa gọi Gemini thật** — không có API key trên máy; nhánh thật chỉ được kiểm bằng SDK giả đúng chữ ký google-genai 2.29.0 |
| AC-8 | Đạt | `TatEm_ViecDangChayXong…`, `TatEm_QuaHanThiHuy…`; `docker compose stop` log tắt êm |
| AC-9 | Đạt | `test_main.py`; compose: job report vào `ai.dlq` với lý do `EV-168` |
| AC-10 | Đạt | `test_config.py` |
| AC-11 | Đạt | `test_logx.py`; log compose có `trace_id` lấy từ `traceparent`, kể cả log uvicorn là JSON |
| AC-12 | Đạt | `test_guarded.py`; đột biến M22–M24 |

## 3. Quy tắc dự án

- Bất biến 2 (không gọi bên thứ ba đồng bộ trong request khách): worker chỉ chạy sau hàng đợi — giữ nguyên.
- Bất biến 4 (log JSON, `trace_id`, không PII): đạt; `question`, `answer`, `prompt`, `body` bị chặn ở formatter.
- Quy ước Redis key: `ai:job:{<job_id>}:result` có hash tag.
- Không sửa `libs/go/**`, `definitions.json`, `services/event/**` (O1, O2 ghi ở spec để mở issue riêng).
