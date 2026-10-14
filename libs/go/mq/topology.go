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

// TicketingTopology tra ve topology cua ticketing.
func TicketingTopology() Topology {
	return Topology{
		Exchanges: []Exchange{{Name: "ticketing.events", Kind: "topic", Durable: true}},
		Queues: []Queue{{Name: "ticketing.hold.released.q", Durable: true,
			Args: map[string]any{"x-queue-type": "quorum"}}},
		Bindings: []Binding{{Source: "ticketing.events", Destination: "ticketing.hold.released.q",
			RoutingKey: "ticketing.hold.released"}},
	}
}

// NotificationTopology tra ve topology cua notification.
func NotificationTopology() Topology {
	return Topology{
		Exchanges: []Exchange{{Name: "notification.direct", Kind: "direct", Durable: true}},
		Queues: []Queue{{Name: "notification.email.q", Durable: true,
			Args: map[string]any{"x-queue-type": "quorum"}}},
		Bindings: []Binding{{Source: "notification.direct", Destination: "notification.email.q",
			RoutingKey: "email"}},
	}
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
