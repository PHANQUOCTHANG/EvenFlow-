package mq

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/codes"
)

var (
	// ErrNacked: broker tra nack cho message.
	ErrNacked = errors.New("mq: broker nack")
	// ErrUnroutable: message bi broker tra ve vi khong co queue nao nhan (mandatory),
	// ke ca khi broker van ack.
	ErrUnroutable = errors.New("mq: message khong dinh tuyen duoc")
	// ErrClosed: kenh da dong (vd. publish toi exchange khong ton tai). Boc
	// amqp091.ErrClosed nen errors.Is dung voi ca hai.
	ErrClosed = fmt.Errorf("mq: kenh da dong: %w", amqp.ErrClosed)
)

// publishIDHeader la header thu vien gan cho MOI lan publish (gia tri duy nhat) de
// gan dung basic.return cho lan publish do; return cu (id khac) bi bo qua. Header
// nay khong duoc chep sang ban retry/DLQ.
const publishIDHeader = "x-ef-publish-id"

func newPublishID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("t%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// Message la noi dung can publish. Headers duoc sao chep, khong bi sua. Property
// de trong thi khong gui. DeliveryMode luon la persistent.
type Message struct {
	Body            []byte
	ContentType     string
	ContentEncoding string
	Headers         map[string]any
	MessageID       string
	CorrelationID   string
	ReplyTo         string
	Type            string
	AppID           string
	Priority        uint8
	Timestamp       time.Time
}

// Publisher publish qua kenh o che do confirm. Publish chi tra nil khi broker ack
// va message khong bi tra ve. Nen dung kenh rieng cho publish (xem doc.go).
//
// Luu y: Publish giu mutex tu luc gui toi khi nhan confirm, nen moi Publisher chi
// co mot publish tai mot thoi diem. Day la lech co chu y so voi y tuong "khong
// mutex quanh viec cho confirm": mutex cho phep gan dung basic.return cho lan
// publish dang cho. Can thong luong cao thi tao nhieu Publisher tren nhieu kenh.
type Publisher struct {
	ch      publishChannel
	returns <-chan amqp.Return
	mu      sync.Mutex
}

// confirmation la phan cua *amqp091.DeferredConfirmation ma Publisher dung.
type confirmation interface {
	WaitContext(ctx context.Context) (bool, error)
}

// publishChannel la phan cua *amqp091.Channel ma Publisher dung (de unit test bang kenh gia).
type publishChannel interface {
	IsClosed() bool
	publish(ctx context.Context, exchange, key string, mandatory bool, msg amqp.Publishing) (confirmation, error)
}

// amqpChannel la adapter cua *amqp091.Channel.
type amqpChannel struct{ *amqp.Channel }

func (c amqpChannel) publish(ctx context.Context, exchange, key string, mandatory bool, msg amqp.Publishing) (confirmation, error) {
	dc, err := c.PublishWithDeferredConfirmWithContext(ctx, exchange, key, mandatory, false, msg)
	if err != nil {
		return nil, err
	}
	return dc, nil
}

// newPublisherWith tao Publisher tu kenh va kenh basic.return da dang ky san.
func newPublisherWith(ch publishChannel, returns <-chan amqp.Return) *Publisher {
	return &Publisher{ch: ch, returns: returns}
}

// NewPublisher bat che do confirm tren ch va dang ky nhan message bi tra ve.
func NewPublisher(ch *amqp.Channel) (*Publisher, error) {
	if err := ch.Confirm(false); err != nil {
		return nil, fmt.Errorf("mq: bat confirm mode: %w", err)
	}
	return newPublisherWith(amqpChannel{ch}, ch.NotifyReturn(make(chan amqp.Return, 16))), nil
}

// Publish gui msg toi exchange/routingKey (mandatory, persistent) va cho confirm.
// ctx co hieu luc o buoc cho confirm; huy ctx khi dang cho nghia la ket qua khong ro.
func (p *Publisher) Publish(ctx context.Context, exchange, routingKey string, msg Message) (err error) {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("mq: publish %s/%s: %w", exchange, routingKey, err)
	}
	ctx, span, headers := startPublishSpan(ctx, exchange, routingKey, amqp.Table(msg.Headers))
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()
	id := newPublishID()
	headers[publishIDHeader] = id

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ch.IsClosed() {
		return fmt.Errorf("mq: publish %s/%s: %w", exchange, routingKey, ErrClosed)
	}
	// Bo return cu con sot; kenh returns da dong nghia la kenh da dong.
	if _, closed := p.collect(""); closed {
		return fmt.Errorf("mq: publish %s/%s: %w", exchange, routingKey, ErrClosed)
	}

	dc, err := p.ch.publish(ctx, exchange, routingKey, true, amqp.Publishing{
		Headers:         headers,
		ContentType:     msg.ContentType,
		ContentEncoding: msg.ContentEncoding,
		DeliveryMode:    amqp.Persistent,
		Priority:        msg.Priority,
		CorrelationId:   msg.CorrelationID,
		ReplyTo:         msg.ReplyTo,
		MessageId:       msg.MessageID,
		Timestamp:       msg.Timestamp,
		Type:            msg.Type,
		AppId:           msg.AppID,
		Body:            msg.Body,
	})
	if err != nil {
		if p.ch.IsClosed() {
			return fmt.Errorf("mq: publish %s/%s: %w: %w", exchange, routingKey, ErrClosed, err)
		}
		return fmt.Errorf("mq: publish %s/%s: %w", exchange, routingKey, err)
	}
	// WaitContext tra khi co confirm, ctx het han, hoac kenh dong (acked=false).
	acked, err := dc.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("mq: cho confirm %s/%s: %w", exchange, routingKey, err)
	}
	// basic.return di truoc basic.ack nen da nam trong bo dem neu co.
	returned, closed := p.collect(id)
	if returned {
		return fmt.Errorf("mq: publish %s/%s: %w", exchange, routingKey, ErrUnroutable)
	}
	if !acked {
		if closed || p.ch.IsClosed() {
			return fmt.Errorf("mq: publish %s/%s: %w", exchange, routingKey, ErrClosed)
		}
		return fmt.Errorf("mq: publish %s/%s: %w", exchange, routingKey, ErrNacked)
	}
	return nil
}

// collect doc het return dang cho (khong chan, khong bao gio lap vo han). found:
// co return mang publish id da cho; closed: kenh returns da dong. Return co id
// khac (cua lan publish truoc bi huy) bi bo qua.
func (p *Publisher) collect(id string) (found, closed bool) {
	for {
		select {
		case r, ok := <-p.returns:
			if !ok {
				return found, true
			}
			if id != "" && r.Headers[publishIDHeader] == id {
				found = true
			}
		default:
			return found, false
		}
	}
}
