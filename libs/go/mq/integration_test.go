//go:build integration

package mq

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/modules/rabbitmq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// Test tich hop voi RabbitMQ that (spec AC-1..AC-6). Chay: go test -tags=integration ./mq/...
// Khong co Docker thi test FAIL (khong skip) theo hop dong K1 cua CI.
//
// Ten export ma test dua vao: NewPublisher, Publisher.Publish, Message, Consume, ConsumerConfig, Delivery,
// ErrPermanent, ErrUnroutable, AttemptHeader, Declare, Merge, AITopology, TicketingTopology, NotificationTopology.

const brokerImage = "rabbitmq:3.13-management-alpine"

var (
	brokerOnce sync.Once
	brokerURL  string
	brokerErr  error
	brokerCtr  *rabbitmq.RabbitMQContainer
	prefixSeq  atomic.Int64
)

func TestMain(m *testing.M) {
	code := m.Run()
	if brokerCtr != nil {
		_ = testcontainers.TerminateContainer(brokerCtr)
	}
	os.Exit(code)
}

// broker khoi dong (mot lan cho ca package) container RabbitMQ va tra ve URL AMQP.
func broker(t *testing.T) string {
	t.Helper()
	brokerOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		brokerCtr, brokerErr = rabbitmq.Run(ctx, brokerImage)
		if brokerErr != nil {
			return
		}
		brokerURL, brokerErr = brokerCtr.AmqpURL(ctx)
	})
	if brokerErr != nil {
		t.Fatalf("khong khoi dong duoc RabbitMQ (%s) — can Docker: %v", brokerImage, brokerErr)
	}
	return brokerURL
}

func dial(t *testing.T) *amqp.Connection {
	t.Helper()
	conn, err := amqp.Dial(broker(t))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func channel(t *testing.T, conn *amqp.Connection) *amqp.Channel {
	t.Helper()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("channel: %v", err)
	}
	t.Cleanup(func() { _ = ch.Close() })
	return ch
}

// depth tra ve so message READY trong queue (mo kenh rieng vi passive declare loi se dong kenh).
func depth(t *testing.T, conn *amqp.Connection, queue string) int {
	t.Helper()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("channel: %v", err)
	}
	defer func() {
		// Kenh co the da bi broker dong neu passive declare loi (khi do t.Fatalf da chay).
		if err := ch.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) {
			t.Errorf("dong kenh inspect %s: %v", queue, err)
		}
	}()
	q, err := ch.QueueDeclarePassive(queue, true, false, false, false, nil)
	if err != nil {
		t.Fatalf("inspect %s: %v", queue, err)
	}
	return q.Messages
}

// eventually thu cond moi 50ms cho toi khi dung hoac het han.
func eventually(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("het %v ma van chua: %s", timeout, what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// testTopo mo phong mau AI voi mot cong viec "work" va hang tre TTL ngan.
type testTopo struct {
	direct, retry, dlx string
	work, delay, dlq   string
	key                string
	delayTTL           time.Duration
}

func newTestTopo(t *testing.T, conn *amqp.Connection, delayTTL time.Duration) testTopo {
	t.Helper()
	p := fmt.Sprintf("it%d", prefixSeq.Add(1))
	tt := testTopo{
		direct: p + ".direct", retry: p + ".retry", dlx: p + ".dlx",
		work: p + ".work.q", delay: p + ".work.retry", dlq: p + ".dlq",
		key: "work", delayTTL: delayTTL,
	}
	top := Topology{
		Exchanges: []Exchange{
			{Name: tt.direct, Kind: "direct", Durable: true},
			{Name: tt.retry, Kind: "direct", Durable: true},
			{Name: tt.dlx, Kind: "direct", Durable: true},
		},
		Queues: []Queue{
			{Name: tt.work, Durable: true, Args: map[string]any{"x-queue-type": "quorum"}},
			{Name: tt.delay, Durable: true, Args: map[string]any{
				"x-queue-type":              "quorum",
				"x-message-ttl":             delayTTL.Milliseconds(),
				"x-dead-letter-exchange":    tt.direct,
				"x-dead-letter-routing-key": tt.key,
				"x-dead-letter-strategy":    "at-least-once",
				"x-overflow":                "reject-publish",
			}},
			{Name: tt.dlq, Durable: true, Args: map[string]any{"x-queue-type": "quorum"}},
		},
		Bindings: []Binding{
			{Source: tt.direct, Destination: tt.work, RoutingKey: tt.key},
			{Source: tt.retry, Destination: tt.delay, RoutingKey: tt.key},
			{Source: tt.dlx, Destination: tt.dlq, RoutingKey: tt.key},
		},
	}
	ch := channel(t, conn)
	for i := 0; i < 2; i++ { // hai lan: idempotent
		if err := Declare(ch, top); err != nil {
			t.Fatalf("Declare lan %d: %v", i+1, err)
		}
	}
	return tt
}

func (tt testTopo) config() ConsumerConfig {
	return ConsumerConfig{
		Queue:              tt.work,
		MaxAttempts:        3,
		RetryExchange:      tt.retry,
		RetryKey:           tt.key,
		DeadLetterExchange: tt.dlx,
		DeadLetterKey:      tt.key,
		Prefetch:           1,
	}
}

func newPublisher(t *testing.T, conn *amqp.Connection) *Publisher {
	t.Helper()
	p, err := NewPublisher(channel(t, conn))
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	return p
}

// consumerRun la mot Consume dang chay trong goroutine.
type consumerRun struct {
	cancel   context.CancelFunc
	finished chan struct{} // dong khi Consume tra ve
	err      error         // loi Consume tra ve (doc sau khi finished dong)
}

// stop huy ctx, cho Consume tra ve va tra loi cua no.
func (r *consumerRun) stop() error {
	r.cancel()
	select {
	case <-r.finished:
		return r.err
	case <-time.After(10 * time.Second):
		return errors.New("Consume khong tra ve sau khi huy ctx")
	}
}

// runConsumer chay Consume trong goroutine voi kenh consume va publisher retry rieng (plan m2).
func runConsumer(t *testing.T, conn *amqp.Connection, cfg ConsumerConfig, h Handler) *consumerRun {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	consCh := channel(t, conn)
	retryPub := newPublisher(t, conn)
	r := &consumerRun{cancel: cancel, finished: make(chan struct{})}
	go func() {
		r.err = Consume(ctx, consCh, retryPub, cfg, h)
		close(r.finished)
	}()
	t.Cleanup(func() { _ = r.stop() })
	return r
}

func getOne(t *testing.T, conn *amqp.Connection, queue string) (amqp.Delivery, bool) {
	t.Helper()
	ch := channel(t, conn)
	d, ok, err := ch.Get(queue, true)
	if err != nil {
		t.Fatalf("get %s: %v", queue, err)
	}
	return d, ok
}

// --- AC-1: publisher confirm ---

func TestTichHop_PublishDuocAckVaVaoQueue(t *testing.T) {
	conn := dial(t)
	tt := newTestTopo(t, conn, 200*time.Millisecond)
	pub := newPublisher(t, conn)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := pub.Publish(ctx, tt.direct, tt.key, Message{
		Body: []byte(`{"q":"xin chao"}`), ContentType: "application/json",
		Headers: map[string]any{"tenant": "evt-1"}, MessageID: "m-1",
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	d, ok := getOne(t, conn, tt.work)
	if !ok {
		t.Fatal("message khong co trong hang goc sau khi Publish tra nil")
	}
	if string(d.Body) != `{"q":"xin chao"}` || d.ContentType != "application/json" || d.MessageId != "m-1" {
		t.Errorf("message = body %q type %q id %q", d.Body, d.ContentType, d.MessageId)
	}
	if d.DeliveryMode != amqp.Persistent {
		t.Errorf("DeliveryMode = %d, muon persistent", d.DeliveryMode)
	}
	if d.Headers["tenant"] != "evt-1" {
		t.Errorf("mat header cua nguoi goi: %v", d.Headers)
	}
	if _, ok := d.Headers["traceparent"]; !ok {
		t.Errorf("thieu traceparent: %v", d.Headers)
	}
}

func TestTichHop_PublishKeyKhongDinhTuyenDuoc(t *testing.T) {
	conn := dial(t)
	tt := newTestTopo(t, conn, 200*time.Millisecond)
	pub := newPublisher(t, conn)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := pub.Publish(ctx, tt.direct, "khong-co-binding", Message{Body: []byte("x")})
	if !errors.Is(err, ErrUnroutable) {
		t.Fatalf("err = %v, muon ErrUnroutable", err)
	}
	// Publisher van dung duoc sau mot lan bi return.
	if err := pub.Publish(ctx, tt.direct, tt.key, Message{Body: []byte("y")}); err != nil {
		t.Fatalf("Publish sau return: %v", err)
	}
}

func TestTichHop_PublishExchangeKhongTonTaiTraLoi(t *testing.T) {
	conn := dial(t)
	pub := newPublisher(t, conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := pub.Publish(ctx, "khong.ton.tai", "k", Message{Body: []byte("x")})
	if ctx.Err() != nil {
		t.Fatal("Publish treo toi het han ctx")
	}
	if !errors.Is(err, ErrClosed) || !errors.Is(err, amqp.ErrClosed) {
		t.Fatalf("err = %v, muon boc ErrClosed va amqp091.ErrClosed", err)
	}
	// Publish tiep tren kenh da dong cung phai tra ErrClosed ngay.
	err = pub.Publish(ctx, "khong.ton.tai", "k", Message{Body: []byte("y")})
	if !errors.Is(err, ErrClosed) || !errors.Is(err, amqp.ErrClosed) {
		t.Fatalf("lan 2: err = %v, muon boc ErrClosed va amqp091.ErrClosed", err)
	}
}

// m9: broker nack that -> ErrNacked (queue x-max-length 0 + x-overflow reject-publish).
func TestTichHop_BrokerNackThanhErrNacked(t *testing.T) {
	conn := dial(t)
	p := fmt.Sprintf("it%d", prefixSeq.Add(1))
	top := Topology{
		Exchanges: []Exchange{{Name: p + ".full", Kind: "direct", Durable: true}},
		Queues: []Queue{{Name: p + ".full.q", Durable: true, Args: map[string]any{
			"x-max-length": int64(0),
			"x-overflow":   "reject-publish",
		}}},
		Bindings: []Binding{{Source: p + ".full", Destination: p + ".full.q", RoutingKey: "full"}},
	}
	if err := Declare(channel(t, conn), top); err != nil {
		t.Fatalf("Declare: %v", err)
	}
	pub := newPublisher(t, conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := pub.Publish(ctx, p+".full", "full", Message{Body: []byte("x")})
	if !errors.Is(err, ErrNacked) {
		t.Fatalf("err = %v, muon ErrNacked", err)
	}
	if errors.Is(err, ErrClosed) || errors.Is(err, ErrUnroutable) {
		t.Errorf("nack that bi bao nham: %v", err)
	}
}

// rabbitctl chay lenh trong container broker va tra ve output.
func rabbitctl(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	code, out, err := brokerCtr.Exec(ctx, args, tcexec.Multiplexed())
	if err != nil {
		t.Fatalf("exec %v: %v", args, err)
	}
	b, err := io.ReadAll(out)
	if err != nil {
		t.Fatalf("doc output %v: %v", args, err)
	}
	if code != 0 {
		t.Fatalf("exec %v: exit %d: %s", args, code, b)
	}
	return string(b)
}

func memoryAlarmOn(t *testing.T) bool {
	t.Helper()
	return strings.Contains(rabbitctl(t, "rabbitmqctl", "eval", "rabbit_alarm:get_alarms()."), "memory")
}

// m9: ctx huy khi dang cho confirm -> loi boc context.Canceled. Bat memory alarm (watermark 0) de broker
// ngung doc tu ket noi publish nen confirm khong bao gio toi.
func TestTichHop_PublishHuyCtxKhiDangChoConfirm(t *testing.T) {
	conn := dial(t)
	tt := newTestTopo(t, conn, 200*time.Millisecond)
	pub := newPublisher(t, conn)

	rabbitctl(t, "rabbitmqctl", "set_vm_memory_high_watermark", "0")
	// Cleanup chay LIFO: tat alarm truoc khi dong ket noi (ket noi bi block co the khong dong duoc).
	t.Cleanup(func() {
		rabbitctl(t, "rabbitmqctl", "set_vm_memory_high_watermark", "0.4")
		eventually(t, 30*time.Second, "memory alarm tat", func() bool { return !memoryAlarmOn(t) })
	})
	eventually(t, 30*time.Second, "memory alarm bat", func() bool { return memoryAlarmOn(t) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(500*time.Millisecond, cancel)
	done := make(chan error, 1)
	go func() { done <- pub.Publish(ctx, tt.direct, tt.key, Message{Body: []byte("cho")}) }()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, muon boc context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Publish khong tra ve sau khi huy ctx")
	}
}

func TestTichHop_PublishCtxDaHuyTraLoi(t *testing.T) {
	conn := dial(t)
	tt := newTestTopo(t, conn, 200*time.Millisecond)
	pub := newPublisher(t, conn)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := pub.Publish(ctx, tt.direct, tt.key, Message{Body: []byte("x")}); err == nil {
		t.Fatal("Publish voi ctx da huy phai tra loi")
	}
}

// --- AC-2 + AC-4: loi 3 lan vao DLQ, trace giu nguyen ---

func TestTichHop_LoiBaLanVaoDLQGiuTrace(t *testing.T) {
	newRecorder(t)
	conn := dial(t)
	tt := newTestTopo(t, conn, 200*time.Millisecond)
	pub := newPublisher(t, conn)

	var mu sync.Mutex
	var attempts []int
	var traces []trace.TraceID
	run := runConsumer(t, conn, tt.config(), func(ctx context.Context, d Delivery) error {
		mu.Lock()
		defer mu.Unlock()
		attempts = append(attempts, d.Attempt)
		traces = append(traces, trace.SpanContextFromContext(ctx).TraceID())
		return errors.New("model qua tai")
	})

	root, rootSpan := otel.Tracer("test").Start(context.Background(), "nguoi-goi")
	wantTrace := rootSpan.SpanContext().TraceID()
	ctx, cancel := context.WithTimeout(root, 10*time.Second)
	defer cancel()
	body := []byte(`{"job":"chat","n":42}`)
	if err := pub.Publish(ctx, tt.direct, tt.key, Message{Body: body, ContentType: "application/json"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	rootSpan.End()

	eventually(t, 30*time.Second, "message vao DLQ", func() bool { return depth(t, conn, tt.dlq) == 1 })
	// Cho them hon 2 chu ky TTL de chac khong co lan xu ly thu 4.
	time.Sleep(3 * tt.delayTTL)
	if err := run.stop(); !errors.Is(err, context.Canceled) {
		t.Errorf("Consume sau khi huy ctx tra %v, muon context.Canceled", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(attempts) != 3 || attempts[0] != 1 || attempts[1] != 2 || attempts[2] != 3 {
		t.Fatalf("handler duoc goi voi attempts %v, muon [1 2 3]", attempts)
	}
	for i, tr := range traces {
		if tr != wantTrace {
			t.Errorf("lan %d: trace ID %s, muon %s", i+1, tr, wantTrace)
		}
	}

	d, ok := getOne(t, conn, tt.dlq)
	if !ok {
		t.Fatal("DLQ rong")
	}
	if n, ok := asInt64(d.Headers[AttemptHeader]); !ok || n != 3 {
		t.Errorf("%s o DLQ = %v (%T), muon 3", AttemptHeader, d.Headers[AttemptHeader], d.Headers[AttemptHeader])
	}
	if string(d.Body) != string(body) || d.ContentType != "application/json" {
		t.Errorf("body/content-type o DLQ = %q %q", d.Body, d.ContentType)
	}
	if w, dl := depth(t, conn, tt.work), depth(t, conn, tt.delay); w != 0 || dl != 0 {
		t.Errorf("hang goc %d, hang tre %d — muon ca hai rong", w, dl)
	}
}

// --- AC-3 ---

func TestTichHop_LoiVinhVienVaoDLQNgay(t *testing.T) {
	conn := dial(t)
	tt := newTestTopo(t, conn, 200*time.Millisecond)
	pub := newPublisher(t, conn)

	var calls atomic.Int32
	run := runConsumer(t, conn, tt.config(), func(context.Context, Delivery) error {
		calls.Add(1)
		return fmt.Errorf("prompt khong hop le: %w", ErrPermanent)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := pub.Publish(ctx, tt.direct, tt.key, Message{Body: []byte("p")}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	eventually(t, 15*time.Second, "message vao DLQ", func() bool { return depth(t, conn, tt.dlq) == 1 })
	time.Sleep(3 * tt.delayTTL)
	_ = run.stop()

	if n := calls.Load(); n != 1 {
		t.Errorf("handler goi %d lan, muon 1", n)
	}
	d, ok := getOne(t, conn, tt.dlq)
	if !ok {
		t.Fatal("DLQ rong")
	}
	if n, ok := asInt64(d.Headers[AttemptHeader]); !ok || n != 1 {
		t.Errorf("%s o DLQ = %v, muon 1", AttemptHeader, d.Headers[AttemptHeader])
	}
	if string(d.Body) != "p" {
		t.Errorf("body = %q", d.Body)
	}
}

func TestTichHop_ThanhCongAckKhongRetry(t *testing.T) {
	conn := dial(t)
	tt := newTestTopo(t, conn, 200*time.Millisecond)
	pub := newPublisher(t, conn)

	var calls atomic.Int32
	run := runConsumer(t, conn, tt.config(), func(_ context.Context, d Delivery) error {
		calls.Add(1)
		if d.Attempt != 1 {
			t.Errorf("Attempt = %d, muon 1", d.Attempt)
		}
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := pub.Publish(ctx, tt.direct, tt.key, Message{Body: []byte("ok")}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	eventually(t, 15*time.Second, "handler duoc goi", func() bool { return calls.Load() >= 1 })
	time.Sleep(3 * tt.delayTTL)
	if err := run.stop(); !errors.Is(err, context.Canceled) {
		t.Errorf("Consume tra %v, muon context.Canceled", err)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("handler goi %d lan, muon 1", n)
	}
	for _, q := range []string{tt.work, tt.delay, tt.dlq} {
		if n := depth(t, conn, q); n != 0 {
			t.Errorf("%s con %d message, muon 0 (da ack)", q, n)
		}
	}
}

// --- AC-6 + M2/M4: publish retry khong dinh tuyen duoc -> nack requeue, Consume tra loi, khong mat message ---

func TestTichHop_PublishRetryLoiKhongMatMessage(t *testing.T) {
	conn := dial(t)
	tt := newTestTopo(t, conn, 200*time.Millisecond)
	pub := newPublisher(t, conn)

	cfg := tt.config()
	cfg.RetryKey = "khong-co-binding"
	var calls atomic.Int32
	run := runConsumer(t, conn, cfg, func(context.Context, Delivery) error {
		calls.Add(1)
		return errors.New("loi tam thoi")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := pub.Publish(ctx, tt.direct, tt.key, Message{Body: []byte("giu-lai")}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	select {
	case <-run.finished:
		if err := run.err; err == nil || errors.Is(err, context.Canceled) {
			t.Fatalf("Consume tra %v, muon loi publish retry", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Consume khong tra loi khi publish retry that bai")
	}
	if calls.Load() < 1 {
		t.Fatal("handler chua duoc goi")
	}
	// Dong ket noi consume de message dang unacked (neu con) duoc tra lai, roi kiem message van trong hang goc.
	conn2 := dial(t)
	_ = conn.Close()
	eventually(t, 10*time.Second, "message quay lai hang goc", func() bool { return depth(t, conn2, tt.work) == 1 })
	if n := depth(t, conn2, tt.dlq); n != 0 {
		t.Errorf("DLQ co %d message, muon 0", n)
	}
	if n := depth(t, conn2, tt.delay); n != 0 {
		t.Errorf("hang tre co %d message, muon 0", n)
	}
}

// --- AC-5: topology that khai bao duoc tren broker that (bat T5) va idempotent ---

func TestTichHop_DeclareTopologyThatHaiLan(t *testing.T) {
	conn := dial(t)
	top := Merge(AITopology(), TicketingTopology(), NotificationTopology())
	ch := channel(t, conn)
	for i := 0; i < 2; i++ {
		if err := Declare(ch, top); err != nil {
			t.Fatalf("Declare lan %d: %v", i+1, err)
		}
	}
	for _, q := range top.Queues {
		if n := depth(t, conn, q.Name); n != 0 {
			t.Errorf("%s co %d message", q.Name, n)
		}
	}
}
