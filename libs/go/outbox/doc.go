// Package outbox hien thuc transactional outbox + relay ra RabbitMQ.
//
// Luong chinh:
//
//  1. Nghiep vu goi Write trong CUNG transaction voi thay doi du lieu: mot row duoc them vao
//     outbox_events (event_id, aggregate, routing_key, payload = chi phan "data", headers trace).
//     Rollback thi khong co row.
//  2. Relay (Run / RunOnce) claim mot lo row bang lease (UPDATE ... FOR UPDATE SKIP LOCKED trong mot
//     transaction ngan), publish tung row NGOAI transaction qua mq.Publisher (confirm), roi danh dau
//     tung ket qua bang cau UPDATE rieng (autocommit).
//
// Bao dam va gioi han:
//
//   - At-least-once: chet giua publish va danh dau thi row duoc gui lai sau khi lease het han.
//     Consumer PHAI khu trung theo event_id lay tu phong bi (Envelope.EventID, cung la AMQP
//     MessageId). Trong "data" cua row cu (truoc migration 0002) khoa event_id la id su kien hoa nhac,
//     khong phai id message.
//   - KHONG bao dam thu tu, ke ca giua cac event cung mot aggregate; consumer dua vao occurred_at.
//   - Khong dua PII vao data: payload nam nguyen trong DB, trong message va trong log loi.
//   - Khong bao gio log URL ket noi (AMQP_URL, DATABASE_URL); Dialer cung phai boc loi khong kem URL.
//   - Relay tu quan ly ket noi qua Dialer: broker sap thi Run dial lai (lui 1s -> 30s, co jitter) va
//     KHONG tra loi; chi claim khi dang ket noi. Run chi tra ve (nil) khi ctx bi huy.
//   - Lam tang attempts: broker nack, unroutable (key ngoai DropUnroutable), het PublishTimeout
//     khi kenh con mo, loi dung message/panic, va MOI loi publish khac khong phan loai (chong row
//     doc thu mai). KHONG tang attempts: ErrClosed, dial loi, huy ctx, het BatchDeadline, row chua
//     xu ly khi lo dung som -- cac row nay duoc tra lease ngay (khong bao gio gan lai row da do).
//     Du MaxAttempts thi row bi "do" (next_attempt_at = 'infinity'), xem gauge outbox_dead va
//     runbook trong docs/02. Truoc moi lan claim relay kiem tra ket noi con song (IsClosed) va dial
//     lai neu chet.
//   - No (chua xu ly): khi broker CHAN ket noi (canh bao bo nho/dia, connection.blocked) publish het
//     han va bi tinh la lan thu; canh bao keo dai ~1 gio co the do ca ticketing.hold.released. Can
//     theo doi NotifyBlocked / outbox_dead va gỡ đỗ theo runbook.
package outbox
