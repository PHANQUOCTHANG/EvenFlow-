// Package mq la lop mong tren amqp091-go cho RabbitMQ: publisher co confirm,
// consumer co retry gioi han + dead-letter, va topology khai bao bang code.
//
// Gia dinh va hop dong:
//
//   - Giao hang at-least-once: mot message co the duoc xu ly nhieu hon mot lan
//     (retry, nack requeue, mat ket noi). Handler PHAI idempotent.
//   - Gia dinh RabbitMQ 3.13 (anh trong compose). Retry do consumer dieu khien,
//     khong dua vao delivery-limit cua quorum queue (4.x doi mac dinh thanh 20).
//   - Nen dung kenh (channel) rieng cho publish va cho consume: kenh publish o
//     che do confirm va co the bi broker dong khi publish loi, khong nen keo theo
//     kenh consume. Consume nhan mot *Publisher rieng de publish ban retry/DLQ.
//   - Retry: loi thu n < MaxAttempts -> publish ban sao sang RetryExchange voi
//     header x-ef-attempt = n+1 roi ack ban goc. Loi thu MaxAttempts hoac loi
//     boc ErrPermanent -> publish sang DeadLetterExchange roi ack. Publish retry
//     hoac DLQ that bai -> nack requeue va Consume tra loi (khong mat message).
//   - Trace context di theo header (otelx.InjectAMQP/ExtractAMQP) va giu nguyen
//     qua cac lan retry.
//   - Khong tu reconnect: khi mat ket noi Consume tra loi de process supervisor
//     khoi dong lai.
//
// Topology (AITopology, TicketingTopology, NotificationTopology) la nguon chuan;
// deploy/rabbitmq/definitions.json phai khop (co unit test doi chieu).
package mq
