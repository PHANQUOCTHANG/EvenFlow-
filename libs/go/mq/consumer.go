package mq

// SEAM KHONG EXPORT cho test (Consume chi la lop mong goi consume):
//
//	// consumeChannel la phan cua *amqp091.Channel ma consume dung.
//	type consumeChannel interface {
//		Qos(prefetchCount, prefetchSize int, global bool) error
//		Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp091.Table) (<-chan amqp091.Delivery, error)
//		Cancel(consumer string, noWait bool) error
//	}
//	// consume validate cfg, goi Qos(Prefetch,0,false), Consume, defer Cancel(tag,false), roi vong lap.
//	// Kenh deliveries dong -> tra loi khac nil. Publisher dung newPublisherWith + kenh publish gia.
//	func consume(ctx context.Context, ch consumeChannel, pub *Publisher, cfg ConsumerConfig, h Handler) error

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// AttemptHeader la header thu vien dung dem so lan xu ly (bat dau tu 1).
const AttemptHeader = "x-ef-attempt"

// RoutingKeyHeader giu routing key luc message duoc publish lan dau. Khi qua hang retry,
// broker dead-letter message ve hang goc voi routing key cua hang tre, nen neu khong co
// header nay handler chon nhanh theo RoutingKey se thay mot key la sau lan thu lai dau tien.
const RoutingKeyHeader = "x-ef-routing-key"

const (
	defaultMaxAttempts = 3
	defaultPrefetch    = 10
)

// ErrPermanent: handler boc loi nay (errors.Is) de day message thang vao DLQ,
// khong retry.
var ErrPermanent = errors.New("mq: permanent failure")

// Delivery la message nhan duoc, gui cho Handler.
type Delivery struct {
	Body       []byte
	Headers    map[string]any
	RoutingKey string
	MessageID  string
	Attempt    int // lan xu ly thu may, tu 1
}

// Handler xu ly mot message. Tra nil -> ack; loi -> retry/DLQ. Giao hang la
// at-least-once nen handler PHAI idempotent.
type Handler func(ctx context.Context, d Delivery) error

// ConsumerConfig cau hinh Consume.
type ConsumerConfig struct {
	Queue       string
	MaxAttempts int // 0 -> 3; tong so lan xu ly truoc khi vao DLQ
	// RetryExchange/RetryKey rong -> khong retry, loi dau tien di thang DLQ.
	RetryExchange string
	RetryKey      string
	// DeadLetterExchange bat buoc (khong de cau hinh lam mat message).
	DeadLetterExchange string
	DeadLetterKey      string
	Prefetch           int // 0 -> 10
}

// validate kiem tra cau hinh va dien gia tri mac dinh.
func (c ConsumerConfig) validate() (ConsumerConfig, error) {
	if c.Queue == "" {
		return c, errors.New("mq: consumer thieu Queue")
	}
	if c.DeadLetterExchange == "" {
		return c, errors.New("mq: consumer thieu DeadLetterExchange")
	}
	if (c.RetryExchange == "") != (c.RetryKey == "") {
		return c, errors.New("mq: RetryExchange va RetryKey phai cung co hoac cung rong")
	}
	if c.MaxAttempts < 0 {
		return c, fmt.Errorf("mq: MaxAttempts %d khong hop le", c.MaxAttempts)
	}
	if c.MaxAttempts == 0 {
		c.MaxAttempts = defaultMaxAttempts
	}
	if c.Prefetch <= 0 {
		c.Prefetch = defaultPrefetch
	}
	return c, nil
}

// Action la quyet dinh sau khi handler chay xong.
type Action int

// Cac hanh dong co the.
const (
	ActionAck Action = iota
	ActionRetry
	ActionDeadLetter
)

// decide la quyet dinh thuan: thanh cong -> Ack; loi vinh vien, het luot hoac
// khong co retry exchange -> DeadLetter; con lai -> Retry.
func decide(handlerErr error, attempt, maxAttempts int, canRetry bool) Action {
	switch {
	case handlerErr == nil:
		return ActionAck
	case errors.Is(handlerErr, ErrPermanent), attempt >= maxAttempts, !canRetry:
		return ActionDeadLetter
	default:
		return ActionRetry
	}
}

// attemptOf doc AttemptHeader; thieu, hong hoac < 1 thi coi la 1.
func attemptOf(headers amqp.Table) int {
	var n int64
	switch v := headers[AttemptHeader].(type) {
	case int:
		n = int64(v)
	case int8:
		n = int64(v)
	case int16:
		n = int64(v)
	case int32:
		n = int64(v)
	case int64:
		n = v
	case uint:
		if uint64(v) > math.MaxInt32 {
			return 1
		}
		n = int64(v)
	case uint8:
		n = int64(v)
	case uint16:
		n = int64(v)
	case uint32:
		n = int64(v)
	case uint64:
		if v > math.MaxInt32 {
			return 1
		}
		n = int64(v)
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 1
		}
		n = parsed
	default:
		return 1
	}
	if n < 1 || n > math.MaxInt32 {
		return 1
	}
	return int(n)
}

// copyHeaders tao ban sao moi (khong bao gio nil), bo header do broker quan ly
// (x-death, x-first-death-*, x-last-death-*, x-delivery-count).
func copyHeaders(in amqp.Table) amqp.Table {
	out := make(amqp.Table, len(in)+1)
	for k, v := range in {
		if k == "x-death" || k == "x-delivery-count" || k == publishIDHeader ||
			strings.HasPrefix(k, "x-first-death-") || strings.HasPrefix(k, "x-last-death-") {
			continue
		}
		out[k] = v
	}
	return out
}

// cloneHeaders sao chep nong headers de handler sua khong anh huong ban chuyen tiep.
func cloneHeaders(in amqp.Table) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// consumeChannel la phan cua *amqp091.Channel ma consume dung.
type consumeChannel interface {
	Qos(prefetchCount, prefetchSize int, global bool) error
	Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error)
	Cancel(consumer string, noWait bool) error
}

// callHandler goi h; panic duoc recover thanh loi thuong (di retry/DLQ).
func callHandler(ctx context.Context, h Handler, d Delivery) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("mq: handler panic: %v", r)
		}
	}()
	return h(ctx, d)
}

// Consume nhan message tu cfg.Queue va chay h tuan tu cho toi khi ctx bi huy
// (tra loi boc context.Canceled/DeadlineExceeded), kenh deliveries dong, hoac
// publish retry/DLQ that bai (message duoc nack requeue, Consume tra loi de
// process supervisor khoi dong lai). ch nen la kenh rieng cho consume; pub nen
// dung kenh publish rieng.
func Consume(ctx context.Context, ch *amqp.Channel, pub *Publisher, cfg ConsumerConfig, h Handler) error {
	cfg, err := cfg.validate()
	if err != nil {
		return err
	}
	if ch == nil || pub == nil || h == nil {
		return errors.New("mq: Consume can kenh, publisher va handler")
	}
	return consume(ctx, ch, pub, cfg, h)
}

// consume la phan noi day cua Consume, nhan consumeChannel de test bang kenh gia.
func consume(ctx context.Context, ch consumeChannel, pub *Publisher, cfg ConsumerConfig, h Handler) error {
	cfg, err := cfg.validate()
	if err != nil {
		return err
	}
	if err := ch.Qos(cfg.Prefetch, 0, false); err != nil {
		return fmt.Errorf("mq: qos %s: %w", cfg.Queue, err)
	}
	tag := "mq." + cfg.Queue + "." + newPublishID()
	deliveries, err := ch.Consume(cfg.Queue, tag, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("mq: consume %s: %w", cfg.Queue, err)
	}
	// Dung nhan message moi khi thoat; caller van phai dong kenh.
	defer func() { _ = ch.Cancel(tag, false) }()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("mq: consume %s dung: %w", cfg.Queue, ctx.Err())
		case d, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("mq: consume %s: kenh deliveries da dong", cfg.Queue)
			}
			if err := handleDelivery(ctx, pub, cfg, h, d); err != nil {
				return err
			}
		}
	}
}

// handleDelivery xu ly mot message. Chi tra loi khi can dung Consume.
func handleDelivery(ctx context.Context, pub *Publisher, cfg ConsumerConfig, h Handler, d amqp.Delivery) error {
	attempt := attemptOf(d.Headers)
	spanCtx, span := startConsumeSpan(ctx, cfg.Queue, d.Headers)
	defer span.End()

	herr := callHandler(spanCtx, h, newDelivery(d))
	if herr != nil {
		span.RecordError(herr)
		span.SetStatus(codes.Error, herr.Error())
	}

	// Dang tat: khong publish retry/DLQ bang ctx da huy, tra message lai hang.
	if herr != nil && ctx.Err() != nil {
		_ = d.Nack(false, true)
		return fmt.Errorf("mq: consume %s dung: %w", cfg.Queue, ctx.Err())
	}

	switch decide(herr, attempt, cfg.MaxAttempts, cfg.RetryExchange != "") {
	case ActionAck:
		return ackDelivery(d, cfg.Queue)
	case ActionRetry:
		return forward(spanCtx, span, pub, d, cfg.Queue, cfg.RetryExchange, cfg.RetryKey, attempt+1)
	default:
		return forward(spanCtx, span, pub, d, cfg.Queue, cfg.DeadLetterExchange, cfg.DeadLetterKey, attempt)
	}
}

// forward publish ban sao sang exchange/key voi AttemptHeader = attempt roi ack
// ban goc. Publish loi -> nack requeue va tra loi.
func forward(ctx context.Context, span trace.Span, pub *Publisher, d amqp.Delivery, queue, exchange, key string, attempt int) error {
	err := pub.Publish(ctx, exchange, key, forwardMessage(d, attempt))
	if err != nil {
		_ = d.Nack(false, true)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("mq: chuyen message %s sang %s/%s: %w", queue, exchange, key, err)
	}
	return ackDelivery(d, queue)
}

// newDelivery dung Delivery cho handler; Headers la ban sao. RoutingKey la key luc publish
// lan dau (RoutingKeyHeader) neu message da di qua retry/DLQ.
func newDelivery(d amqp.Delivery) Delivery {
	return Delivery{
		Body:       d.Body,
		Headers:    cloneHeaders(d.Headers),
		RoutingKey: originalRoutingKey(d),
		MessageID:  d.MessageId,
		Attempt:    attemptOf(d.Headers),
	}
}

// originalRoutingKey tra RoutingKeyHeader neu co (message da duoc chuyen tiep), nguoc lai
// routing key cua lan giao nay.
func originalRoutingKey(d amqp.Delivery) string {
	if k, ok := d.Headers[RoutingKeyHeader].(string); ok && k != "" {
		return k
	}
	return d.RoutingKey
}

// forwardMessage dung ban sao retry/DLQ: giu body va property, header da bo header
// cua broker/publish-id, AttemptHeader = attempt (int32 co dau), RoutingKeyHeader = key goc.
func forwardMessage(d amqp.Delivery, attempt int) Message {
	headers := copyHeaders(d.Headers)
	headers[AttemptHeader] = int32(attempt)
	headers[RoutingKeyHeader] = originalRoutingKey(d)
	return Message{
		Body:            d.Body,
		ContentType:     d.ContentType,
		ContentEncoding: d.ContentEncoding,
		Headers:         headers,
		MessageID:       d.MessageId,
		CorrelationID:   d.CorrelationId,
		ReplyTo:         d.ReplyTo,
		Type:            d.Type,
		AppID:           d.AppId,
		Priority:        d.Priority,
		Timestamp:       d.Timestamp,
	}
}

func ackDelivery(d amqp.Delivery, queue string) error {
	if err := d.Ack(false); err != nil {
		return fmt.Errorf("mq: ack %s: %w", queue, err)
	}
	return nil
}
