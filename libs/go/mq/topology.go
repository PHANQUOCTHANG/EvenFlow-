package mq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Exchange mo ta mot exchange.
type Exchange struct {
	Name    string
	Kind    string
	Durable bool
}

// Queue mo ta mot queue. So trong Args dung kieu int64.
type Queue struct {
	Name    string
	Durable bool
	Args    map[string]any
}

// Binding noi Source (exchange) toi Destination (queue) theo RoutingKey.
type Binding struct {
	Source      string
	Destination string
	RoutingKey  string
}

// Topology la tap exchange, queue va binding can khai bao.
type Topology struct {
	Exchanges []Exchange
	Queues    []Queue
	Bindings  []Binding
}

// aiJobSpec mo ta mot cong viec AI: hang goc co TTL rieng, hang tre rieng.
type aiJobSpec struct {
	key      string
	workTTL  int64 // ms, het han thi bo (khong retry)
	delayTTL int64 // ms, thoi gian cho truoc khi quay lai hang goc
}

var aiJobSpecs = []aiJobSpec{
	{"chat", 60_000, 5_000},
	{"moderation", 600_000, 30_000},
	{"anomaly", 1_800_000, 30_000},
	{"report", 7_200_000, 120_000},
}

// AITopology tra ve topology AI: moi cong viec co hang goc ai.<key>.q (khong
// x-dead-letter, het TTL thi bo), hang tre rieng ai.<key>.retry dead-letter ve
// ai.direct voi key cua chinh no, va ai.dlq nhan tu ai.dlx theo tung key.
func AITopology() Topology {
	t := Topology{
		Exchanges: []Exchange{
			{Name: "ai.direct", Kind: "direct", Durable: true},
			{Name: "ai.retry", Kind: "direct", Durable: true},
			{Name: "ai.dlx", Kind: "direct", Durable: true},
		},
	}
	for _, j := range aiJobSpecs {
		t.Queues = append(t.Queues,
			Queue{Name: "ai." + j.key + ".q", Durable: true, Args: map[string]any{
				"x-queue-type":  "quorum",
				"x-message-ttl": j.workTTL,
			}},
			Queue{Name: "ai." + j.key + ".retry", Durable: true, Args: map[string]any{
				"x-queue-type":              "quorum",
				"x-message-ttl":             j.delayTTL,
				"x-dead-letter-exchange":    "ai.direct",
				"x-dead-letter-routing-key": j.key,
				"x-dead-letter-strategy":    "at-least-once",
				"x-overflow":                "reject-publish",
			}},
		)
	}
	t.Queues = append(t.Queues, Queue{Name: "ai.dlq", Durable: true, Args: map[string]any{"x-queue-type": "quorum"}})
	for _, j := range aiJobSpecs {
		t.Bindings = append(t.Bindings,
			Binding{Source: "ai.direct", Destination: "ai." + j.key + ".q", RoutingKey: j.key},
			Binding{Source: "ai.retry", Destination: "ai." + j.key + ".retry", RoutingKey: j.key},
			Binding{Source: "ai.dlx", Destination: "ai.dlq", RoutingKey: j.key},
		)
	}
	return t
}

// sinkMaxLength: hang sink chi giu ban moi nhat de tra cuu khi debug, khong phai de xu ly.
const sinkMaxLength int64 = 1000

// sinkTopology gan mot hang "#" vao exchange topic. Publisher bat mandatory: message khong co
// hang nao nhan bi broker tra ve va Publish bao loi. Relay outbox dung o dong loi dau tien de
// giu thu tu, nen MOT routing key chua co consumer (vd ticketing.hold.created) se chan moi su
// kien phia sau no mai mai. Sink bao dam moi key luon co it nhat mot hang nhan.
func sinkTopology(exchange string) ([]Queue, []Binding) {
	q := exchange + ".sink.q"
	return []Queue{{Name: q, Durable: true, Args: map[string]any{
			"x-queue-type": "quorum",
			"x-max-length": sinkMaxLength,
			"x-overflow":   "drop-head",
		}}},
		[]Binding{{Source: exchange, Destination: q, RoutingKey: "#"}}
}

// consumerQueues la bo hang cho mot consumer co retry: hang goc <name>.q, hang tre <name>.retry
// (het TTL thi dead-letter ve hang goc qua default exchange) va binding retry theo key = name.
// Hang goc nhan message tu cac binding nguon do topology goi tu khai bao.
func consumerQueues(name, retryExchange string, delayTTL int64) ([]Queue, []Binding) {
	work, retry := name+".q", name+".retry"
	return []Queue{
			{Name: work, Durable: true, Args: map[string]any{"x-queue-type": "quorum"}},
			{Name: retry, Durable: true, Args: map[string]any{
				"x-queue-type":  "quorum",
				"x-message-ttl": delayTTL,
				// Default exchange (""): routing key la ten hang goc, nen message chi quay lai
				// dung hang nay, khong lap sang consumer khac cung binding nguon.
				"x-dead-letter-exchange":    "",
				"x-dead-letter-routing-key": work,
				"x-dead-letter-strategy":    "at-least-once",
				"x-overflow":                "reject-publish",
			}},
		},
		[]Binding{
			{Source: retryExchange, Destination: retry, RoutingKey: name},
		}
}

// PaymentTopology tra ve topology cua payment: exchange su kien va hang sink.
// Consumer cua order.paid / payment.refunded tu khai bao binding toi exchange nay.
func PaymentTopology() Topology {
	q, b := sinkTopology("payment.events")
	return Topology{
		Exchanges: []Exchange{{Name: "payment.events", Kind: "topic", Durable: true}},
		Queues:    q,
		Bindings:  b,
	}
}

// Ten consumer phat ve khi nhan order.paid (ConsumerConfig dung cac ten nay).
const (
	TicketingOrderPaidQueue  = "ticketing.order-paid.q"
	TicketingOrderPaidKey    = "ticketing.order-paid"
	TicketingRetryExchange   = "ticketing.retry"
	TicketingDeadLetterEx    = "ticketing.dlx"
	TicketingDeadLetterQueue = "ticketing.dlq"
)

// TicketingTopology tra ve topology cua ticketing: exchange su kien, hang hold.released, sink,
// va consumer order.paid -> phat ve. Binding toi payment.events can PaymentTopology khai bao
// exchange truoc: goi Declare(ch, Merge(PaymentTopology(), TicketingTopology())).
func TicketingTopology() Topology {
	sinkQ, sinkB := sinkTopology("ticketing.events")
	consQ, consB := consumerQueues("ticketing.order-paid", TicketingRetryExchange, 5_000)
	t := Topology{
		Exchanges: []Exchange{
			{Name: "ticketing.events", Kind: "topic", Durable: true},
			{Name: TicketingRetryExchange, Kind: "direct", Durable: true},
			{Name: TicketingDeadLetterEx, Kind: "direct", Durable: true},
		},
		Queues: []Queue{{Name: "ticketing.hold.released.q", Durable: true,
			Args: map[string]any{"x-queue-type": "quorum"}}},
		Bindings: []Binding{{Source: "ticketing.events", Destination: "ticketing.hold.released.q",
			RoutingKey: "ticketing.hold.released"}},
	}
	t.Queues = append(t.Queues, sinkQ...)
	t.Queues = append(t.Queues, consQ...)
	t.Queues = append(t.Queues, Queue{Name: TicketingDeadLetterQueue, Durable: true, Args: map[string]any{"x-queue-type": "quorum"}})
	t.Bindings = append(t.Bindings, sinkB...)
	t.Bindings = append(t.Bindings, consB...)
	t.Bindings = append(t.Bindings,
		Binding{Source: "payment.events", Destination: TicketingOrderPaidQueue, RoutingKey: "order.paid"},
		Binding{Source: TicketingDeadLetterEx, Destination: TicketingDeadLetterQueue, RoutingKey: TicketingOrderPaidKey},
	)
	return t
}

// Ten consumer su kien cua notification (email/SMS xac nhan don, ve, hoan tien).
const (
	NotificationEventsQueue     = "notification.events.q"
	NotificationEventsKey       = "notification.events"
	NotificationRetryExchange   = "notification.retry"
	NotificationDeadLetterEx    = "notification.dlx"
	NotificationDeadLetterQueue = "notification.dlq"
)

// NotificationTopology tra ve topology cua notification: hang email cu va consumer su kien
// nghe payment.events + ticketing.events. Can khai bao kem PaymentTopology va TicketingTopology
// (exchange nguon): Declare(ch, Merge(PaymentTopology(), TicketingTopology(), NotificationTopology())).
func NotificationTopology() Topology {
	consQ, consB := consumerQueues("notification.events", NotificationRetryExchange, 10_000)
	t := Topology{
		Exchanges: []Exchange{
			{Name: "notification.direct", Kind: "direct", Durable: true},
			{Name: NotificationRetryExchange, Kind: "direct", Durable: true},
			{Name: NotificationDeadLetterEx, Kind: "direct", Durable: true},
		},
		Queues: []Queue{{Name: "notification.email.q", Durable: true,
			Args: map[string]any{"x-queue-type": "quorum"}}},
		Bindings: []Binding{{Source: "notification.direct", Destination: "notification.email.q",
			RoutingKey: "email"}},
	}
	t.Queues = append(t.Queues, consQ...)
	t.Queues = append(t.Queues, Queue{Name: NotificationDeadLetterQueue, Durable: true, Args: map[string]any{"x-queue-type": "quorum"}})
	t.Bindings = append(t.Bindings, consB...)
	t.Bindings = append(t.Bindings,
		Binding{Source: "payment.events", Destination: NotificationEventsQueue, RoutingKey: "order.paid"},
		Binding{Source: "payment.events", Destination: NotificationEventsQueue, RoutingKey: "payment.refunded"},
		Binding{Source: "ticketing.events", Destination: NotificationEventsQueue, RoutingKey: "ticketing.ticket.issued"},
		Binding{Source: NotificationDeadLetterEx, Destination: NotificationDeadLetterQueue, RoutingKey: NotificationEventsKey},
	)
	return t
}

// Merge noi cac topology theo thu tu, khong loai trung.
func Merge(ts ...Topology) Topology {
	var out Topology
	for _, t := range ts {
		out.Exchanges = append(out.Exchanges, t.Exchanges...)
		out.Queues = append(out.Queues, t.Queues...)
		out.Bindings = append(out.Bindings, t.Bindings...)
	}
	return out
}

// Declarer la phan cua *amqp091.Channel ma Declare can (de unit test bang fake).
type Declarer interface {
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error
}

// Declare khai bao topology theo thu tu exchange, queue, binding. Khai bao lai
// cung tham so la idempotent. Dung ngay o loi dau tien, loi co kem ten doi tuong.
func Declare(ch Declarer, t Topology) error {
	for _, e := range t.Exchanges {
		if err := ch.ExchangeDeclare(e.Name, e.Kind, e.Durable, false, false, false, nil); err != nil {
			return fmt.Errorf("mq: declare exchange %s: %w", e.Name, err)
		}
	}
	for _, q := range t.Queues {
		var args amqp.Table
		if q.Args != nil {
			args = amqp.Table(q.Args)
		}
		if _, err := ch.QueueDeclare(q.Name, q.Durable, false, false, false, args); err != nil {
			return fmt.Errorf("mq: declare queue %s: %w", q.Name, err)
		}
	}
	for _, b := range t.Bindings {
		if err := ch.QueueBind(b.Destination, b.RoutingKey, b.Source, false, nil); err != nil {
			return fmt.Errorf("mq: bind queue %s to exchange %s [%s]: %w", b.Destination, b.Source, b.RoutingKey, err)
		}
	}
	return nil
}
