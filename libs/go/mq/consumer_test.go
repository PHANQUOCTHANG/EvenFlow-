package mq

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Test logic quyet dinh cua consumer (spec AC-2, AC-3, AC-6; plan §1, §7 M5/m3/m4/m5), khong can broker.
//
// Ten KHONG export ma test nay gia dinh (implementer phai khop dung ten va chu ky):
//
//	type Action int
//	const ( ActionAck Action = iota; ActionRetry; ActionDeadLetter )   // gia tri cu the khong quan trong
//	func decide(handlerErr error, attempt, maxAttempts int, canRetry bool) Action
//	func attemptOf(headers amqp091.Table) int              // doc AttemptHeader; thieu/hong/<1 -> 1
//	func copyHeaders(in amqp091.Table) amqp091.Table       // ban sao moi (khong bao gio nil), bo header do broker quan ly
//	func (c ConsumerConfig) validate() (ConsumerConfig, error) // kiem cau hinh + dien mac dinh
//	func callHandler(ctx context.Context, h Handler, d Delivery) error // goi handler, panic -> error
//	func newDelivery(d amqp091.Delivery) Delivery          // Delivery cho handler: Attempt = attemptOf, Headers la BAN SAO (m4)
//	func forwardMessage(d amqp091.Delivery, attempt int) Message // ban sao retry/DLQ: copyHeaders(d.Headers) + AttemptHeader
//
// copyHeaders con phai bo "x-ef-publish-id" (header tuong quan return cua Publisher, plan §8 M1).
// validate: RetryExchange va RetryKey phai cung co hoac cung rong (plan §8 m6).
//
// Ten export: Handler, Delivery, ErrPermanent, ConsumerConfig, AttemptHeader, Consume.
// (attemptOf/copyHeaders co the nhan/tra map[string]any thay vi amqp091.Table: test van bien dich.)

func TestAttemptHeader_TenHeader(t *testing.T) {
	if AttemptHeader != "x-ef-attempt" {
		t.Fatalf("AttemptHeader = %q, muon x-ef-attempt", AttemptHeader)
	}
}

func TestDecide_BangQuyetDinh(t *testing.T) {
	transient := errors.New("loi tam thoi")
	cases := []struct {
		name     string
		err      error
		attempt  int
		max      int
		canRetry bool
		want     Action
	}{
		{"thanh cong lan dau", nil, 1, 3, true, ActionAck},
		{"thanh cong lan cuoi", nil, 3, 3, true, ActionAck},
		{"thanh cong du khong retry duoc", nil, 1, 3, false, ActionAck},
		{"loi lan 1 con retry", transient, 1, 3, true, ActionRetry},
		{"loi lan 2 con retry", transient, 2, 3, true, ActionRetry},
		{"loi lan 3 bang max", transient, 3, 3, true, ActionDeadLetter},
		{"loi attempt vuot max", transient, 5, 3, true, ActionDeadLetter},
		{"max 1 loi lan dau", transient, 1, 1, true, ActionDeadLetter},
		{"loi vinh vien lan dau", ErrPermanent, 1, 3, true, ActionDeadLetter},
		{"loi vinh vien bi boc", fmt.Errorf("goi model: %w", ErrPermanent), 1, 3, true, ActionDeadLetter},
		{"loi vinh vien boc hai lop", fmt.Errorf("a: %w", fmt.Errorf("b: %w", ErrPermanent)), 2, 3, true, ActionDeadLetter},
		{"khong co retry exchange", transient, 1, 3, false, ActionDeadLetter},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := decide(c.err, c.attempt, c.max, c.canRetry); got != c.want {
				t.Fatalf("decide(%v, %d, %d, %v) = %v, muon %v", c.err, c.attempt, c.max, c.canRetry, got, c.want)
			}
		})
	}
}

func TestDecide_BaHanhDongKhacNhau(t *testing.T) {
	if ActionAck == ActionRetry || ActionRetry == ActionDeadLetter || ActionAck == ActionDeadLetter {
		t.Fatal("ActionAck/ActionRetry/ActionDeadLetter phai khac nhau")
	}
}

func TestAttemptOf_DocHeader(t *testing.T) {
	cases := []struct {
		name    string
		headers amqp.Table
		want    int
	}{
		{"headers nil", nil, 1},
		{"thieu header", amqp.Table{"khac": "x"}, 1},
		{"int32", amqp.Table{AttemptHeader: int32(2)}, 2},
		{"int64", amqp.Table{AttemptHeader: int64(3)}, 3},
		{"int", amqp.Table{AttemptHeader: 2}, 2},
		{"int16", amqp.Table{AttemptHeader: int16(2)}, 2},
		{"chuoi so", amqp.Table{AttemptHeader: "2"}, 2},
		{"chuoi rac", amqp.Table{AttemptHeader: "abc"}, 1},
		{"kieu la", amqp.Table{AttemptHeader: true}, 1},
		{"bang 0", amqp.Table{AttemptHeader: int32(0)}, 1},
		{"am", amqp.Table{AttemptHeader: int64(-4)}, 1},
		{"chuoi am", amqp.Table{AttemptHeader: "-1"}, 1},
		// M2 (code review): amqp091 giai ma 'B'->uint8, 'u'->uint16, 'i'->uint32; client khac co the gui uint64.
		{"int8", amqp.Table{AttemptHeader: int8(2)}, 2},
		{"uint8", amqp.Table{AttemptHeader: uint8(2)}, 2},
		{"uint16", amqp.Table{AttemptHeader: uint16(3)}, 3},
		{"uint32", amqp.Table{AttemptHeader: uint32(2)}, 2},
		{"uint64", amqp.Table{AttemptHeader: uint64(3)}, 3},
		{"uint8 bang 0", amqp.Table{AttemptHeader: uint8(0)}, 1},
		{"uint64 bang 0", amqp.Table{AttemptHeader: uint64(0)}, 1},
		{"int8 am", amqp.Table{AttemptHeader: int8(-1)}, 1},
		{"int am", amqp.Table{AttemptHeader: -7}, 1},
		{"uint64 tran", amqp.Table{AttemptHeader: uint64(math.MaxUint64)}, 1},
		{"int64 tran", amqp.Table{AttemptHeader: int64(math.MaxInt64)}, 1},
		{"chuoi tran", amqp.Table{AttemptHeader: "99999999999999999999"}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := attemptOf(c.headers); got != c.want {
				t.Fatalf("attemptOf(%v) = %d, muon %d", c.headers, got, c.want)
			}
		})
	}
}

func TestCopyHeaders_BoHeaderBrokerGiuPhanCon(t *testing.T) {
	in := amqp.Table{
		"traceparent":            "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01",
		AttemptHeader:            int32(2),
		"tenant":                 "evt-1",
		"x-death":                []any{amqp.Table{"count": int64(1)}},
		"x-first-death-exchange": "ai.direct",
		"x-first-death-queue":    "ai.chat.q",
		"x-first-death-reason":   "expired",
		"x-last-death-exchange":  "ai.retry",
		"x-last-death-queue":     "ai.chat.retry",
		"x-last-death-reason":    "expired",
		"x-delivery-count":       int64(1),
		"x-ef-publish-id":        "pub-cu",
	}
	got := copyHeaders(in)
	for _, k := range []string{"x-ef-publish-id", "x-death", "x-first-death-exchange", "x-first-death-queue", "x-first-death-reason",
		"x-last-death-exchange", "x-last-death-queue", "x-last-death-reason", "x-delivery-count"} {
		if _, ok := got[k]; ok {
			t.Errorf("ban sao van con header broker %s", k)
		}
	}
	for _, k := range []string{"traceparent", AttemptHeader, "tenant"} {
		if got[k] != in[k] {
			t.Errorf("header %s = %v, muon %v", k, got[k], in[k])
		}
	}
	if len(got) != 3 {
		t.Errorf("ban sao co %d header, muon 3: %v", len(got), got)
	}
	// Phai la ban sao: sua ban sao khong dung vao ban goc.
	got[AttemptHeader] = int32(3)
	got["moi"] = "x"
	if in[AttemptHeader] != int32(2) {
		t.Error("copyHeaders tra ve map dung chung voi ban goc")
	}
	if _, ok := in["moi"]; ok {
		t.Error("copyHeaders tra ve map dung chung voi ban goc")
	}
	if _, ok := in["x-death"]; !ok {
		t.Error("copyHeaders khong duoc xoa header cua ban goc")
	}
}

func TestCopyHeaders_NilAnToan(t *testing.T) {
	got := copyHeaders(nil)
	if got == nil {
		t.Fatal("copyHeaders(nil) = nil, muon map rong ghi duoc")
	}
	got[AttemptHeader] = int32(1) // khong duoc panic
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
}

func TestConsumerConfig_Validate(t *testing.T) {
	base := ConsumerConfig{
		Queue:              "ai.chat.q",
		RetryExchange:      "ai.retry",
		RetryKey:           "chat",
		DeadLetterExchange: "ai.dlx",
		DeadLetterKey:      "chat",
	}

	t.Run("mac dinh MaxAttempts 3 Prefetch 10", func(t *testing.T) {
		got, err := base.validate()
		if err != nil {
			t.Fatalf("validate: %v", err)
		}
		if got.MaxAttempts != 3 || got.Prefetch != 10 {
			t.Fatalf("MaxAttempts=%d Prefetch=%d, muon 3 va 10", got.MaxAttempts, got.Prefetch)
		}
		if got.Queue != base.Queue || got.RetryExchange != base.RetryExchange || got.DeadLetterExchange != base.DeadLetterExchange {
			t.Fatalf("validate lam doi truong khac: %+v", got)
		}
	})

	t.Run("giu gia tri tuong minh", func(t *testing.T) {
		c := base
		c.MaxAttempts, c.Prefetch = 5, 1
		got, err := c.validate()
		if err != nil {
			t.Fatalf("validate: %v", err)
		}
		if got.MaxAttempts != 5 || got.Prefetch != 1 {
			t.Fatalf("MaxAttempts=%d Prefetch=%d, muon 5 va 1", got.MaxAttempts, got.Prefetch)
		}
	})

	t.Run("khong retry exchange van hop le", func(t *testing.T) {
		c := base
		c.RetryExchange, c.RetryKey = "", ""
		if _, err := c.validate(); err != nil {
			t.Fatalf("validate: %v", err)
		}
	})

	t.Run("thieu DeadLetterExchange bi tu choi", func(t *testing.T) {
		c := base
		c.DeadLetterExchange = ""
		if _, err := c.validate(); err == nil {
			t.Fatal("muon loi khi DeadLetterExchange rong (M5: khong cho cau hinh lam mat message)")
		}
	})

	t.Run("thieu Queue bi tu choi", func(t *testing.T) {
		c := base
		c.Queue = ""
		if _, err := c.validate(); err == nil {
			t.Fatal("muon loi khi Queue rong")
		}
	})

	t.Run("chi co RetryKey bi tu choi", func(t *testing.T) {
		c := base
		c.RetryExchange = ""
		if _, err := c.validate(); err == nil {
			t.Fatal("muon loi khi RetryKey co ma RetryExchange rong (m6)")
		}
	})

	t.Run("chi co RetryExchange bi tu choi", func(t *testing.T) {
		c := base
		c.RetryKey = ""
		if _, err := c.validate(); err == nil {
			t.Fatal("muon loi khi RetryExchange co ma RetryKey rong (m6)")
		}
	})

	t.Run("MaxAttempts am bi tu choi", func(t *testing.T) {
		c := base
		c.MaxAttempts = -1
		if _, err := c.validate(); err == nil {
			t.Fatal("muon loi khi MaxAttempts < 1")
		}
	})
}

// Consume phai tu choi cau hinh thieu DLX TRUOC khi dung toi kenh (kenh nil o day).
func TestConsume_TuChoiCauHinhThieuDLX(t *testing.T) {
	cfg := ConsumerConfig{Queue: "ai.chat.q", RetryExchange: "ai.retry", RetryKey: "chat"}
	err := Consume(context.Background(), nil, nil, cfg, func(context.Context, Delivery) error { return nil })
	if err == nil {
		t.Fatal("Consume voi DeadLetterExchange rong phai tra loi")
	}
}

func TestCallHandler_PanicThanhLoi(t *testing.T) {
	err := callHandler(context.Background(), func(context.Context, Delivery) error {
		panic("handler no")
	}, Delivery{Body: []byte("x")})
	if err == nil {
		t.Fatal("panic cua handler phai thanh loi")
	}
	if errors.Is(err, ErrPermanent) {
		t.Error("panic phai la loi thuong (di retry), khong phai ErrPermanent")
	}
	if got := decide(err, 1, 3, true); got != ActionRetry {
		t.Errorf("panic lan 1 -> %v, muon ActionRetry", got)
	}
}

func TestCallHandler_TraNguyenLoiVaNil(t *testing.T) {
	want := errors.New("x")
	var seen Delivery
	err := callHandler(context.Background(), func(_ context.Context, d Delivery) error {
		seen = d
		return want
	}, Delivery{Body: []byte("b"), Attempt: 2, RoutingKey: "chat"})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, muon %v", err, want)
	}
	if string(seen.Body) != "b" || seen.Attempt != 2 || seen.RoutingKey != "chat" {
		t.Fatalf("handler nhan %+v", seen)
	}
	if err := callHandler(context.Background(), func(context.Context, Delivery) error { return nil }, Delivery{}); err != nil {
		t.Fatalf("handler nil -> %v", err)
	}
}

// m4: handler sua Delivery.Headers khong duoc lot vao ban retry/DLQ.
func TestNewDelivery_HeadersLaBanSao(t *testing.T) {
	d := amqp.Delivery{
		Headers: amqp.Table{
			"tenant":          "evt-1",
			AttemptHeader:     int32(2),
			"x-death":         []any{amqp.Table{"count": int64(1)}},
			"x-ef-publish-id": "pub-cu",
		},
		Body:       []byte("noi-dung"),
		RoutingKey: "chat",
		MessageId:  "m-1",
	}
	hd := newDelivery(d)
	if hd.Attempt != 2 || string(hd.Body) != "noi-dung" || hd.RoutingKey != "chat" || hd.MessageID != "m-1" {
		t.Fatalf("newDelivery = %+v", hd)
	}
	if hd.Headers["tenant"] != "evt-1" {
		t.Fatalf("handler khong thay header nguoi gui: %v", hd.Headers)
	}
	// Handler "nghich" header.
	hd.Headers["tenant"] = "bi-sua"
	hd.Headers["handler-them"] = true
	delete(hd.Headers, AttemptHeader)

	if d.Headers["tenant"] != "evt-1" {
		t.Error("sua Delivery.Headers lam doi header cua delivery goc")
	}
	fm := forwardMessage(d, 3)
	if fm.Headers["tenant"] != "evt-1" {
		t.Errorf("ban chuyen tiep mang header da bi handler sua: %v", fm.Headers)
	}
	if _, ok := fm.Headers["handler-them"]; ok {
		t.Errorf("ban chuyen tiep mang header handler them: %v", fm.Headers)
	}
	if n, ok := asInt64(fm.Headers[AttemptHeader]); !ok || n != 3 {
		t.Errorf("%s = %v, muon 3", AttemptHeader, fm.Headers[AttemptHeader])
	}
	for _, k := range []string{"x-death", "x-ef-publish-id"} {
		if _, ok := fm.Headers[k]; ok {
			t.Errorf("ban chuyen tiep con header %s", k)
		}
	}
	if string(fm.Body) != "noi-dung" || fm.MessageID != "m-1" {
		t.Errorf("forwardMessage = body %q id %q", fm.Body, fm.MessageID)
	}
	// forwardMessage khong duoc sua header cua delivery goc.
	if d.Headers[AttemptHeader] != int32(2) {
		t.Errorf("forwardMessage sua header goc: %v", d.Headers)
	}
}

// m3 (plan §8): ban retry/DLQ giu cac property AMQP cua message goc.
func TestForwardMessage_GiuThuocTinh(t *testing.T) {
	ts := time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC)
	d := amqp.Delivery{
		Body:            []byte("b"),
		ContentType:     "application/json",
		ContentEncoding: "gzip",
		CorrelationId:   "corr-1",
		ReplyTo:         "amq.rabbitmq.reply-to",
		Type:            "chat.ask",
		AppId:           "gateway",
		Priority:        3,
		Timestamp:       ts,
		MessageId:       "m-1",
	}
	fm := forwardMessage(d, 2)
	if fm.ContentType != d.ContentType || fm.ContentEncoding != d.ContentEncoding || fm.CorrelationID != d.CorrelationId ||
		fm.ReplyTo != d.ReplyTo || fm.Type != d.Type || fm.AppID != d.AppId || fm.Priority != d.Priority ||
		!fm.Timestamp.Equal(ts) || fm.MessageID != d.MessageId || string(fm.Body) != "b" {
		t.Fatalf("forwardMessage mat property: %+v", fm)
	}
}

// --- F4 (challenge): phan noi day cua Consume, qua seam cua implementer ---
//
// Seam KHONG EXPORT dang co trong consumer.go (test dung dung ten nay):
//
//	type consumeChannel interface {
//		Qos(prefetchCount, prefetchSize int, global bool) error
//		Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp091.Table) (<-chan amqp091.Delivery, error)
//		Cancel(consumer string, noWait bool) error
//	}
//	func consume(ctx context.Context, ch consumeChannel, pub *Publisher, cfg ConsumerConfig, h Handler) error

type fakeConsumeChannel struct {
	mu         sync.Mutex
	qos        []int
	consumeTag string
	cancelTags []string
	deliveries chan amqp.Delivery
}

func (f *fakeConsumeChannel) Qos(prefetchCount, _ int, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.qos = append(f.qos, prefetchCount)
	return nil
}

func (f *fakeConsumeChannel) Consume(_, consumer string, autoAck, _, _, _ bool, _ amqp.Table) (<-chan amqp.Delivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if autoAck {
		return nil, errors.New("consumer phai ack tay")
	}
	f.consumeTag = consumer
	return f.deliveries, nil
}

func (f *fakeConsumeChannel) Cancel(consumer string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelTags = append(f.cancelTags, consumer)
	return nil
}

// fakeAck ghi lai ack/nack cua delivery.
type fakeAck struct {
	mu                    sync.Mutex
	acks, nacks, requeued int
}

func (a *fakeAck) Ack(uint64, bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.acks++
	return nil
}

func (a *fakeAck) Nack(_ uint64, _ bool, requeue bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.nacks++
	if requeue {
		a.requeued++
	}
	return nil
}

func (a *fakeAck) Reject(_ uint64, requeue bool) error { return a.Nack(0, false, requeue) }

func wiringConfig() ConsumerConfig {
	return ConsumerConfig{Queue: "ai.chat.q", RetryExchange: "ai.retry", RetryKey: "chat",
		DeadLetterExchange: "ai.dlx", DeadLetterKey: "chat"}
}

// runConsume chay consume va bat buoc tra ve trong 2s.
func runConsume(ctx context.Context, t *testing.T, ch *fakeConsumeChannel, pub *Publisher, cfg ConsumerConfig, h Handler) error {
	t.Helper()
	return within(t, 2*time.Second, func() error { return consume(ctx, ch, pub, cfg, h) })
}

// C27 + C25 + C26: kenh deliveries dong (basic.cancel) -> loi (khong nil); Qos(prefetch); Cancel(tag) khi thoat.
func TestConsumeWiring_DeliveriesDongTraLoiQosVaCancel(t *testing.T) {
	for _, c := range []struct {
		name     string
		prefetch int
		want     int
	}{{"prefetch tuong minh", 7, 7}, {"prefetch mac dinh", 0, 10}} {
		t.Run(c.name, func(t *testing.T) {
			ch := &fakeConsumeChannel{deliveries: make(chan amqp.Delivery)}
			close(ch.deliveries)
			pf := newFakePubChannel()
			cfg := wiringConfig()
			cfg.Prefetch = c.prefetch
			err := runConsume(context.Background(), t, ch, newPublisherWith(pf, pf.returns), cfg,
				func(context.Context, Delivery) error { return nil })
			if err == nil {
				t.Fatal("deliveries dong -> consume phai tra loi (de supervisor khoi dong lai), khong nil")
			}
			if errors.Is(err, context.Canceled) {
				t.Errorf("err = %v, khong duoc la context.Canceled", err)
			}
			ch.mu.Lock()
			defer ch.mu.Unlock()
			if len(ch.qos) != 1 || ch.qos[0] != c.want {
				t.Errorf("Qos goi voi %v, muon [%d]", ch.qos, c.want)
			}
			if ch.consumeTag == "" {
				t.Error("consume tag rong — khong Cancel duoc dung consumer")
			}
			if len(ch.cancelTags) != 1 || ch.cancelTags[0] != ch.consumeTag {
				t.Errorf("Cancel goi voi %v, muon [%q]", ch.cancelTags, ch.consumeTag)
			}
		})
	}
}

// C26: thoat vi ctx huy cung Cancel(tag) va tra loi boc context.Canceled.
func TestConsumeWiring_HuyCtxCancelConsumer(t *testing.T) {
	ch := &fakeConsumeChannel{deliveries: make(chan amqp.Delivery)}
	pf := newFakePubChannel()
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	err := runConsume(ctx, t, ch, newPublisherWith(pf, pf.returns), wiringConfig(),
		func(context.Context, Delivery) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, muon boc context.Canceled", err)
	}
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if len(ch.cancelTags) != 1 || ch.cancelTags[0] != ch.consumeTag || ch.consumeTag == "" {
		t.Errorf("Cancel goi voi %v, consume tag %q", ch.cancelTags, ch.consumeTag)
	}
}

// C20: RetryExchange rong -> loi dau tien di thang DLX (x-ef-attempt = 1) roi ack, khong publish retry.
func TestConsumeWiring_KhongRetryExchangeLoiDauVaoDLQ(t *testing.T) {
	ack := &fakeAck{}
	ch := &fakeConsumeChannel{deliveries: make(chan amqp.Delivery, 1)}
	ch.deliveries <- amqp.Delivery{Acknowledger: ack, DeliveryTag: 1, Body: []byte("x"), RoutingKey: "chat"}
	close(ch.deliveries)
	pf := newFakePubChannel()
	cfg := wiringConfig()
	cfg.RetryExchange, cfg.RetryKey = "", ""
	var calls int
	_ = runConsume(context.Background(), t, ch, newPublisherWith(pf, pf.returns), cfg,
		func(context.Context, Delivery) error { calls++; return errors.New("loi tam thoi") })

	if calls != 1 {
		t.Fatalf("handler goi %d lan, muon 1", calls)
	}
	pf.mu.Lock()
	targets := append([]string(nil), pf.targets...)
	pf.mu.Unlock()
	if len(targets) != 1 || targets[0] != "ai.dlx/chat" {
		t.Fatalf("publish toi %v, muon [ai.dlx/chat]", targets)
	}
	m := pf.lastPublished(t)
	if n, ok := asInt64(m.Headers[AttemptHeader]); !ok || n != 1 {
		t.Errorf("%s = %v, muon 1", AttemptHeader, m.Headers[AttemptHeader])
	}
	ack.mu.Lock()
	defer ack.mu.Unlock()
	if ack.acks != 1 || ack.nacks != 0 {
		t.Errorf("acks=%d nacks=%d, muon 1 va 0", ack.acks, ack.nacks)
	}
}

// Doi chieu: co RetryExchange thi loi dau di retry voi x-ef-attempt = 2.
func TestConsumeWiring_CoRetryExchangeLoiDauDiRetry(t *testing.T) {
	ack := &fakeAck{}
	ch := &fakeConsumeChannel{deliveries: make(chan amqp.Delivery, 1)}
	ch.deliveries <- amqp.Delivery{Acknowledger: ack, DeliveryTag: 1, Body: []byte("x")}
	close(ch.deliveries)
	pf := newFakePubChannel()
	_ = runConsume(context.Background(), t, ch, newPublisherWith(pf, pf.returns), wiringConfig(),
		func(context.Context, Delivery) error { return errors.New("loi tam thoi") })
	pf.mu.Lock()
	targets := append([]string(nil), pf.targets...)
	pf.mu.Unlock()
	if len(targets) != 1 || targets[0] != "ai.retry/chat" {
		t.Fatalf("publish toi %v, muon [ai.retry/chat]", targets)
	}
	if n, ok := asInt64(pf.lastPublished(t).Headers[AttemptHeader]); !ok || n != 2 {
		t.Errorf("%s = %v, muon 2", AttemptHeader, pf.lastPublished(t).Headers[AttemptHeader])
	}
}
