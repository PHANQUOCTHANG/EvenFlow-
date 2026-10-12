# EV-182 — Ma trận truy vết AC ↔ Test (Traceability)

| AC | Nội dung yêu cầu | Test File | Test Case Name | Trạng thái |
|---|---|---|---|---|
| **AC-1** | Tôn trọng `poll_after_ms` từ phản hồi 200 | `src/hooks/use-queue-status.test.tsx` | `AC-1: tôn trọng poll_after_ms, chưa đến hẹn không poll lại` | ✅ PASS |
| **AC-1** | 304 Not Modified gửi If-None-Match ETag và đợi theo `X-Poll-After-Ms` | `src/hooks/use-queue-status.test.tsx` | `AC-1: 304 Not Modified giữ ETag và đợi theo X-Poll-After-Ms` | ✅ PASS |
| **AC-1** | 503 xả tải đợi theo `Retry-After` của server | `src/hooks/use-queue-status.test.tsx` | `AC-1: 503 Retry-After đợi theo chỉ định của server` | ✅ PASS |
| **AC-1** | Sanitize: poll_after_ms âm / 0 / NaN / thiếu rơi về 10s; <1s kẹp lên 1s | `src/lib/queue-status.test.ts` | `sanitizePollAfterMs — AC-1 không bao giờ poll nhanh hơn server` | ✅ PASS |
| **AC-2** | Lỗi mạng / 5xx lui theo cấp số nhân 1s, 2s, 4s... trần 30s + jitter | `src/lib/backoff.test.ts` | `backoffDelay — AC-2 lùi theo cấp số nhân` | ✅ PASS |
| **AC-2** | Hook lùi nhịp khi lỗi mạng và đổi trạng thái kết nối sang reconnecting | `src/hooks/use-queue-status.test.tsx` | `AC-2: Lỗi mạng backoff exponential và đổi trạng thái kết nối` | ✅ PASS |
| **AC-2** | SSE stream đứt kết nối reconnect có backoff và giới hạn số lần thử | `src/lib/sse.test.ts` | `AC-2: lỗi mạng -> reconnect theo backoff 1s, 2s` & `AC-2: vượt maxRetries -> 'gave-up'` | ✅ PASS |
| **AC-3** | Tab ẩn (visibility hidden) hoặc window focus không tăng tần suất poll | `src/hooks/use-queue-status.test.tsx` | `AC-3: Tab ẩn hay focus KHÔNG tạo thêm request ngoài nhịp server` | ✅ PASS |
| **AC-4** | Tự động nâng cấp lên SSE `/queue/stream` khi `poll_after_ms <= 3000` | `src/hooks/use-queue-status.test.tsx` | `AC-4: Tự động nâng cấp lên SSE khi poll_after_ms <= 3000` | ✅ PASS |
| **AC-4** | Parser SSE hỗ trợ `position` event và chuẩn hóa CRLF / CR / LF | `src/lib/sse.test.ts` | `createSseParser > event + data, dispatch khi gặp dòng trống` | ✅ PASS |
| **AC-5** | BR-Q1: Ở LOBBY không hiển thị rank, ETA, tiến trình | `src/components/queue/queue-position.test.tsx` | `BR-Q1: LOBBY không hiển thị rank kể cả khi caller truyền vào` | ✅ PASS |
| **AC-5** | ProgressRing tính toán tiến trình trung thực, không vẽ nếu thiếu mẫu số | `src/components/queue/progress-ring.test.tsx` | `không render nếu thiếu initialRank hoặc initialRank <= 0 (no fake progress)` | ✅ PASS |
| **AC-5** | ProgressRing có đầy đủ aria-valuenow, valuemin, valuemax, valuetext | `src/components/queue/progress-ring.test.tsx` | `render progressbar với đầy đủ ARIA attributes` | ✅ PASS |
| **AC-5** | ETA chỉ hiển thị khi server cấp (`eta_seconds >= 0`), thiếu ghi 'đang cập nhật' | `src/components/queue/queue-position.test.tsx` | `QUEUED: etaSeconds = null hoặc âm -> hiển thị 'đang cập nhật'` | ✅ PASS |
| **AC-6** | ADMITTED hiển thị AdmitBanner với role="alert" và đếm ngược suất mua 15' | `src/components/queue/admit-banner.test.tsx` | `render vai trò alert để screen reader thông báo ngay` & `render đồng hồ đếm ngược` | ✅ PASS |
| **AC-6** | Tự động chuyển hướng sang `/checkout/[eventId]` khi tới lượt | `src/components/queue/waiting-room.test.tsx` | `AC-6: khi state = ADMITTED, tự động redirect sang /checkout/[eventId]` | ✅ PASS |
| **AC-7** | Lỗi 4xx (401/403/404) dừng poll, chuyển sang idle không retry vô hạn | `src/hooks/use-queue-status.test.tsx` | `AC-7: 4xx dừng polling, đánh dấu error` | ✅ PASS |
| **AC-7** | Reconnecting thông báo cho khách chưa bị mất chỗ (BR-Q7) | `src/components/queue/waiting-room.tsx` | Render role="status" giải thích giữ chỗ | ✅ PASS |
| **AC-8** | Tái sử dụng token từ sessionStorage, nếu chưa có thì join không jitter | `src/components/queue/waiting-room.test.tsx` | `chưa có token: tự động gọi joinQueue và lưu sessionStorage` | ✅ PASS |
| **AC-8** | Route `/waiting/[eventId]` render đúng tiêu đề sự kiện và metadata noindex | `src/app/(queue)/waiting/[eventId]/page.test.tsx` | `render WaitingRoom component với thông tin sự kiện tìm được` | ✅ PASS |
