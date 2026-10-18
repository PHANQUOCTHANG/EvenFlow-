# EV-157 — Thử phá (Adversarial challenge)

| Mục | Giá trị |
|---|---|
| Người thực hiện | Người hiện thực, 2026-10-17 — không phải challenger context sạch |
| Đối tượng | Thay đổi chưa commit trên `feature/EV-157-ai-worker-consumers` |
| Cách làm | Script trong scratchpad: thay đúng một chuỗi (phải khớp đúng 1 lần) → chạy bộ test liên quan → trả file về trong `finally`. Đột biến cần broker chạy bộ tích hợp (RabbitMQ 3.13 + Redis 7 thật) |
| Kết luận | **PASS** sau khi siết test: 24/25 đột biến bị bắt; 1 sống sót có giải thích (M17, tương đương về hành vi). Thử phá làm lộ 1 lỗ hổng test (AC-3) và sửa 1 lập luận sai trong spec |

## 1. Kết quả

| # | Đột biến | Kết quả | Test đỏ |
|---|---|---|---|
| M1 | Pool chat chỉ chạy 1 worker | **Sống sót ở lần đầu → thêm test → Đỏ** | `PoolChaySongSongDungSoConsumer` |
| M2 | `prefetch = 1` cho mọi pool | **Sống sót ở lần đầu → thêm test → Đỏ** | `PoolChaySongSongDungSoConsumer` |
| M3 | Chuyển tiếp lỗi mà vẫn ack bản gốc | Đỏ | `ChuyenTiepLoi_NackRequeueKhongAck` |
| M4 | Hết lượt dùng `>` thay vì `>=` | Đỏ | `decide_BangQuyetDinh[…3…]` |
| M5 | Giữ `x-death` khi chuyển tiếp | Đỏ | `forward_headers_BoHeaderCuaBrokerVaDatAttempt` |
| M6 | Tắt không chờ việc đang chạy | Đỏ | `TatEm_ViecDangChayXongPhanConLaiQuayLaiHang` |
| M7 | Sampler không đặt lag về 0 | Đỏ | `LagMetric_BonQueueVaVeKhongKhiTrong` |
| M8 | Bỏ chặn kiểu `bool` trong `attempt_of` | **Tương đương → bỏ đoạn chặn thừa** | — (`True` → 1, `False` → 0 → 1: kết quả như nhau) |
| M9 | 429 không đổi thành `Retry` | Đỏ | `429_ThanhRetryKhongTraFallbackChoKhach` |
| M10 | Đang tắt vẫn gọi handler | Đỏ | `DangTat_ChuaXuLyThiTraLaiHangKhongGoiHandler` |
| M11 | 429 không hoàn token | Đỏ | `429_ThanhRetryKhongTraFallbackChoKhach` |
| M12 | Timeout vẫn hoàn hết token | Đỏ | `GeminiLoiKhac_KhachVanNhanFallback_KhongHoanToken` |
| M13 | `PUBLISH` trước `SET` | Đỏ | `GhiTruocRoiMoiPublish` |
| M14 | Formatter ghi `question` vào log | Đỏ | `KhongBaoGioGhiCauHoi` |
| M15 | Body hỏng là lỗi tạm | Đỏ | `BodyHong_LaLoiVinhVien` |
| M16 | Topology bỏ binding | Đỏ | `KhaiBaoTopology_HaiLanVanOK` |
| M17 | Mọi pool dùng chung một channel | **Sống sót — tương đương** | — (xem §2) |
| M18 | Chung channel + `basic.qos global` | Đỏ | Broker đóng kết nối: `540 NOT_IMPLEMENTED - queue 'ai.chat.q' … does not support global qos` |
| M19 | Report gọi `time.sleep` trực tiếp (chặn event loop) | Đỏ | `ReportNang_KhongLamChatTreQua5Giay[thread]` |
| M20 | Lần thử cuối vẫn `Retry` (bỏ trả lời dự phòng — B1) | Đỏ | `LanCuoiLimiterVanTuChoi_TraLoiDuPhongVaAck` |
| M21 | Sampler dùng channel của pool (m1) | Đỏ | `Sampler_LoiDungChannelRiengKhongDungPool` |

Bổ sung 2026-10-18 (sau khi sửa B2 và log uvicorn). M11, M12 đổi đích sang `app/gemini/guarded.py` vì logic hoàn
token đã chuyển về đó — chạy lại vẫn đỏ:

| # | Đột biến | Kết quả | Test đỏ |
|---|---|---|---|
| M22 | Kiểm duyệt bỏ qua limiter | Đỏ | `KiemDuyetThat_DiQuaLimiter` |
| M23 | Kiểm duyệt lần cuối vẫn retry (không chuyển người duyệt) | Đỏ | `KiemDuyet_LanCuoiVanKhongGoiDuoc_ChuyenNguoiDuyet` |
| M24 | Limiter từ chối mà vẫn gọi Gemini | Đỏ | `LimiterChuaChoPhep_Retry` |
| M25 | uvicorn giữ handler text | Đỏ | `configure_LogUvicornCungLaJson` |

Lần chạy đầu của M13, M14 ghi "đỏ" bởi test B1 đang đỏ sẵn (chạy `-x`). Kết quả đó **không tính**; đã chạy lại sau khi
sửa B1 và bảng trên là lần chạy lại.

## 2. Phát hiện

| # | Mức | Nội dung |
|---|---|---|
| F1 | **Lỗ hổng test** | M1, M2 sống sót: không test nào kiểm AC-3 (đủ số handler song song). Test AC-1 không đủ tải để lộ ra. Đã thêm `PoolChaySongSongDungSoConsumer` (8 luồng, đo đỉnh đồng thời = 8, 16 việc xong < 2,5 giây) |
| F2 | **Lập luận sai trong spec** | M17 sống sót vì RabbitMQ áp `basic.qos` (global = false, mặc định của aio-pika) **theo từng consumer**, không theo channel. Spec §4.2 bản đầu ghi "theo từng channel" — đã sửa. Vẫn giữ channel riêng vì cách ly lỗi (channel đóng do publish hỏng chỉ kéo theo một pool) |
| F3 | Thông tin | M18: quorum queue (3.13) từ chối global qos. Phương án "một prefetch chung" mà spec loại bỏ **không thể** xảy ra trên topology này — broker chặn trước |
| F4 | Thông tin | M19 xác nhận rủi ro thật còn lại của AC-1 là handler chặn event loop; test AC-1 bắt được. Quy tắc "việc nặng CPU qua `asyncio.to_thread`" ghi ở docstring `runtime.py` và spec §4.2 |
| F5 | Thông tin | Đột biến M8 tương đương → đoạn mã thừa, đã bỏ |
