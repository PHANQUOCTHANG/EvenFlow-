package mq

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Unit test cho Publisher voi kenh gia (code review vong 1: B1, M1, m5; plan §8).
//
// SEAM KHONG EXPORT ma test nay gia dinh (implementer them vao publisher.go; NewPublisher dung adapter cua
// *amqp091.Channel roi goi newPublisherWith):
//
//	// confirmation la phan cua *amqp091.DeferredConfirmation ma Publisher dung.
//	type confirmation interface {
//		WaitContext(ctx context.Context) (bool, error)
//	}
//
//	// publishChannel la phan cua *amqp091.Channel ma Publisher dung. Adapter cua *amqp091.Channel goi
//	// PublishWithDeferredConfirmWithContext(ctx, exchange, key, mandatory, false, msg).
//	type publishChannel interface {
//		IsClosed() bool
//		publish(ctx context.Context, exchange, key string, mandatory bool, msg amqp091.Publishing) (confirmation, error)
//	}
//
//	// newPublisherWith tao Publisher tu kenh va kenh basic.return da dang ky san (NotifyReturn).
//	func newPublisherWith(ch publishChannel, returns <-chan amqp091.Return) *Publisher
//
// Hanh vi test doi hoi:
//   - Moi Publish gan header "x-ef-publish-id" (chuoi khong rong, khac nhau giua cac lan) vao BAN SAO headers.
//   - Return chi tinh cho dung publish co cung x-ef-publish-id; return khac id bi bo qua (M1).
//   - Kenh returns da dong (doc ra ok=false) -> thoat ngay, tra loi boc ErrClosed VA amqp091.ErrClosed (B1, m5);
//     khong vong lap khong chan.
//   - ctx huy khi dang cho confirm -> loi boc context.Canceled.

const hdrPublishID = "x-ef-publish-id"

// fakeConfirm gia lap DeferredConfirmation (WaitContext, Done, Acked nhu amqp091) de hien thuc
// chon cach cho nao (WaitContext hay select tren Done) cung chay dung.
type fakeConfirm struct {
	ack   bool
	err   error
	block bool          // confirm khong bao gio toi; chi tra khi ctx huy
	done  chan struct{} // khac nil: confirm toi khi kenh nay dong
}

var closedDone = func() chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}()

func (c fakeConfirm) Done() <-chan struct{} {
	switch {
	case c.done != nil:
		return c.done
	case c.block:
		return nil // nil channel: khong bao gio san sang
	default:
		return closedDone
	}
}

func (c fakeConfirm) Acked() bool { return c.ack }

func (c fakeConfirm) WaitContext(ctx context.Context) (bool, error) {
	if c.err != nil {
		return false, c.err
	}
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-c.Done():
		return c.ack, nil
	}
}

// fakePubChannel gia lap kenh publish. onPublish quyet dinh confirm va co the day return / dong kenh.
type fakePubChannel struct {
	mu        sync.Mutex
	closedSeq []bool // ket qua lan luot cua IsClosed; het danh sach thi dung phan tu cuoi
	calls     int
	returns   chan amqp.Return
	published []amqp.Publishing
	mandatory []bool
	targets   []string // "exchange/key" cua tung lan publish
	onPublish func(f *fakePubChannel, exchange, key string, msg amqp.Publishing) (confirmation, error)
}

func newFakePubChannel() *fakePubChannel {
	return &fakePubChannel{closedSeq: []bool{false}, returns: make(chan amqp.Return, 16)}
}

func (f *fakePubChannel) IsClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.calls
	f.calls++
	if i >= len(f.closedSeq) {
		i = len(f.closedSeq) - 1
	}
	return f.closedSeq[i]
}

// setClosed: tu nay IsClosed tra true.
func (f *fakePubChannel) setClosed() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closedSeq = []bool{true}
	f.calls = 0
}

func (f *fakePubChannel) publish(_ context.Context, exchange, key string, mandatory bool, msg amqp.Publishing) (confirmation, error) {
	f.mu.Lock()
	f.published = append(f.published, msg)
	f.mandatory = append(f.mandatory, mandatory)
	f.targets = append(f.targets, exchange+"/"+key)
	on := f.onPublish
	f.mu.Unlock()
	if on == nil {
		return fakeConfirm{ack: true}, nil
	}
	return on(f, exchange, key, msg)
}

func (f *fakePubChannel) lastPublished(t *testing.T) amqp.Publishing {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.published) == 0 {
		t.Fatal("kenh gia chua nhan publish nao")
	}
	return f.published[len(f.published)-1]
}

// within chay f va bat buoc no tra ve trong d (bat loi quay vong / treo).
func within(t *testing.T, d time.Duration, f func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- f() }()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		t.Fatalf("Publish khong tra ve sau %v (treo hoac quay vong)", d)
		return nil
	}
}

func publishIDOf(t *testing.T, m amqp.Publishing) string {
	t.Helper()
	id, ok := m.Headers[hdrPublishID].(string)
	if !ok || id == "" {
		t.Fatalf("publish thieu header %s (chuoi khong rong): %v", hdrPublishID, m.Headers)
	}
	return id
}

func TestPublisher_AckTraNilVaDatThuocTinh(t *testing.T) {
	f := newFakePubChannel()
	p := newPublisherWith(f, f.returns)
	caller := map[string]any{"tenant": "evt-1"}
	err := within(t, 2*time.Second, func() error {
		return p.Publish(context.Background(), "ai.direct", "chat",
			Message{Body: []byte("b"), ContentType: "application/json", Headers: caller, MessageID: "m-1"})
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	m := f.lastPublished(t)
	if !f.mandatory[0] {
		t.Error("Publish phai dat mandatory=true (M2)")
	}
	if m.DeliveryMode != amqp.Persistent || m.ContentType != "application/json" || m.MessageId != "m-1" || string(m.Body) != "b" {
		t.Errorf("publishing = %+v", m)
	}
	if m.Headers["tenant"] != "evt-1" {
		t.Errorf("mat header nguoi goi: %v", m.Headers)
	}
	publishIDOf(t, m)
	if len(caller) != 1 {
		t.Errorf("Publish sua map headers cua nguoi goi: %v", caller)
	}
}

func TestPublisher_PublishIDKhacNhauMoiLan(t *testing.T) {
	f := newFakePubChannel()
	p := newPublisherWith(f, f.returns)
	for i := 0; i < 2; i++ {
		if err := within(t, 2*time.Second, func() error { return p.Publish(context.Background(), "ex", "k", Message{}) }); err != nil {
			t.Fatalf("Publish %d: %v", i+1, err)
		}
	}
	a, b := publishIDOf(t, f.published[0]), publishIDOf(t, f.published[1])
	if a == b {
		t.Fatalf("hai lan publish cung %s = %q", hdrPublishID, a)
	}
}

// M1: return cu (khac id) toi giua luc cho confirm cua lan nay -> bo qua, Publish thanh cong.
func TestPublisher_ReturnCuKhacIDBiBoQua(t *testing.T) {
	f := newFakePubChannel()
	f.onPublish = func(f *fakePubChannel, ex, key string, _ amqp.Publishing) (confirmation, error) {
		f.returns <- amqp.Return{Exchange: ex, RoutingKey: key, ReplyCode: 312, ReplyText: "NO_ROUTE",
			Headers: amqp.Table{hdrPublishID: "id-cua-lan-truoc"}}
		return fakeConfirm{ack: true}, nil
	}
	p := newPublisherWith(f, f.returns)
	if err := within(t, 2*time.Second, func() error { return p.Publish(context.Background(), "ai.retry", "chat", Message{}) }); err != nil {
		t.Fatalf("return cu khac id lam Publish loi: %v", err)
	}
}

// M1: return dung id -> ErrUnroutable du broker van ack (va MessageID rong).
func TestPublisher_ReturnDungIDThanhErrUnroutable(t *testing.T) {
	f := newFakePubChannel()
	f.onPublish = func(f *fakePubChannel, ex, key string, msg amqp.Publishing) (confirmation, error) {
		f.returns <- amqp.Return{Exchange: ex, RoutingKey: key, ReplyCode: 312, ReplyText: "NO_ROUTE",
			Headers: amqp.Table{hdrPublishID: msg.Headers[hdrPublishID]}}
		return fakeConfirm{ack: true}, nil
	}
	p := newPublisherWith(f, f.returns)
	err := within(t, 2*time.Second, func() error { return p.Publish(context.Background(), "ai.retry", "khong-co-binding", Message{}) })
	if !errors.Is(err, ErrUnroutable) {
		t.Fatalf("err = %v, muon ErrUnroutable", err)
	}
}

func TestPublisher_NackThanhErrNacked(t *testing.T) {
	f := newFakePubChannel()
	f.onPublish = func(*fakePubChannel, string, string, amqp.Publishing) (confirmation, error) {
		return fakeConfirm{ack: false}, nil
	}
	p := newPublisherWith(f, f.returns)
	err := within(t, 2*time.Second, func() error { return p.Publish(context.Background(), "ex", "k", Message{}) })
	if !errors.Is(err, ErrNacked) {
		t.Fatalf("err = %v, muon ErrNacked", err)
	}
	if errors.Is(err, ErrClosed) {
		t.Errorf("nack that khong duoc bao la ErrClosed: %v", err)
	}
}

// B1: kenh dong khi dang cho confirm -> thu vien dong kenh returns roi confirm tra (false, nil).
// Publish phai tra ErrClosed ngay, khong quay vong tren kenh returns da dong.
func TestPublisher_KenhDongKhiChoConfirmKhongQuayVong(t *testing.T) {
	f := newFakePubChannel()
	f.onPublish = func(f *fakePubChannel, _, _ string, _ amqp.Publishing) (confirmation, error) {
		f.setClosed()
		close(f.returns)
		return fakeConfirm{ack: false}, nil
	}
	p := newPublisherWith(f, f.returns)
	err := within(t, 2*time.Second, func() error { return p.Publish(context.Background(), "khong.ton.tai", "k", Message{}) })
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("err = %v, muon ErrClosed", err)
	}
	if !errors.Is(err, amqp.ErrClosed) {
		t.Errorf("err = %v, muon boc ca amqp091.ErrClosed (m5)", err)
	}
}

// B1 (drainStale): kenh dong ngay sau lan kiem IsClosed dau tien, returns da dong truoc khi publish.
func TestPublisher_ReturnsDaDongTruocKhiPublishKhongQuayVong(t *testing.T) {
	f := newFakePubChannel()
	f.closedSeq = []bool{false, true} // lan kiem dau con mo, sau do dong
	close(f.returns)
	f.onPublish = func(*fakePubChannel, string, string, amqp.Publishing) (confirmation, error) {
		return nil, amqp.ErrClosed
	}
	p := newPublisherWith(f, f.returns)
	err := within(t, 2*time.Second, func() error { return p.Publish(context.Background(), "ex", "k", Message{}) })
	if !errors.Is(err, ErrClosed) || !errors.Is(err, amqp.ErrClosed) {
		t.Fatalf("err = %v, muon boc ErrClosed va amqp091.ErrClosed", err)
	}
}

// m5: kenh da dong tu truoc -> ErrClosed boc ca amqp091.ErrClosed.
func TestPublisher_KenhDaDongTuTruoc(t *testing.T) {
	f := newFakePubChannel()
	f.setClosed()
	p := newPublisherWith(f, f.returns)
	err := within(t, 2*time.Second, func() error { return p.Publish(context.Background(), "ex", "k", Message{}) })
	if !errors.Is(err, ErrClosed) || !errors.Is(err, amqp.ErrClosed) {
		t.Fatalf("err = %v, muon boc ErrClosed va amqp091.ErrClosed", err)
	}
}

// Huy ctx khi dang cho confirm -> loi boc context.Canceled, khong treo; publish sau do van chay.
func TestPublisher_HuyCtxKhiDangChoConfirm(t *testing.T) {
	f := newFakePubChannel()
	f.onPublish = func(*fakePubChannel, string, string, amqp.Publishing) (confirmation, error) {
		return fakeConfirm{block: true}, nil
	}
	p := newPublisherWith(f, f.returns)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	err := within(t, 2*time.Second, func() error { return p.Publish(ctx, "ex", "k", Message{}) })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, muon boc context.Canceled", err)
	}
	f.onPublish = nil // lan sau ack binh thuong; mutex phai da nha
	if err := within(t, 2*time.Second, func() error { return p.Publish(context.Background(), "ex", "k", Message{}) }); err != nil {
		t.Fatalf("Publish sau khi huy: %v", err)
	}
}

// notifyAllSim mo phong notifyAll cua amqp091 v1.15.0: gui return cho listener, cho toi da timeout
// roi BO gia tri neu bo dem day (tra false khi bi bo).
func notifyAllSim(ch chan amqp.Return, r amqp.Return, timeout time.Duration) bool {
	select {
	case ch <- r:
		return true
	case <-time.After(timeout):
		return false
	}
}

// F1 (challenge): >16 publish bi huy ctx khi dang cho; return cua chung toi muon, trong luc mot publish
// unroutable moi dang cho confirm. Publish phai doc returns DONG THOI khi cho, nen bo dem 16 khong day,
// return cua chinh no khong bi bo va khong co lan ket 5s nao.
func TestPublisher_NhieuReturnCuKhongLamMatReturnMoi(t *testing.T) {
	f := newFakePubChannel()
	p := newPublisherWith(f, f.returns)

	// Giai doan 1: 20 publish bi huy giua luc cho confirm.
	f.onPublish = func(*fakePubChannel, string, string, amqp.Publishing) (confirmation, error) {
		return fakeConfirm{block: true}, nil
	}
	const stale = 20
	for i := 0; i < stale; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		err := within(t, 2*time.Second, func() error { return p.Publish(ctx, "ai.retry", "khong-co-binding", Message{}) })
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("publish bi huy %d: err = %v, muon DeadlineExceeded", i, err)
		}
	}
	f.mu.Lock()
	var staleIDs []any
	for _, m := range f.published {
		staleIDs = append(staleIDs, m.Headers[hdrPublishID])
	}
	f.mu.Unlock()

	// Giai doan 2: publish moi; "goroutine doc cua connection" day 20 return cu roi return cua lan nay,
	// sau do moi ack (return di truoc ack). Moi lan gui cho toi 5s nhu notifyAll that.
	var dropped atomic.Bool
	f.onPublish = func(f *fakePubChannel, ex, key string, msg amqp.Publishing) (confirmation, error) {
		done := make(chan struct{})
		go func() {
			defer close(done)
			for _, id := range staleIDs {
				if !notifyAllSim(f.returns, amqp.Return{Exchange: ex, RoutingKey: key, ReplyCode: 312,
					Headers: amqp.Table{hdrPublishID: id}}, 5*time.Second) {
					dropped.Store(true)
				}
			}
			if !notifyAllSim(f.returns, amqp.Return{Exchange: ex, RoutingKey: key, ReplyCode: 312,
				Headers: amqp.Table{hdrPublishID: msg.Headers[hdrPublishID]}}, 5*time.Second) {
				dropped.Store(true)
			}
		}()
		return fakeConfirm{ack: true, done: done}, nil
	}
	start := time.Now()
	err := within(t, 3*time.Second, func() error {
		return p.Publish(context.Background(), "ai.retry", "khong-co-binding", Message{})
	})
	if !errors.Is(err, ErrUnroutable) {
		t.Fatalf("err = %v, muon ErrUnroutable (return cua lan nay bi bo?)", err)
	}
	if dropped.Load() {
		t.Error("co return bi bo vi bo dem day (Publish khong doc returns khi dang cho)")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("Publish mat %v — co lan ket cho bo dem", d)
	}
}

// F2 (challenge): Publish gui DU property AMQP cua Message len kenh.
func TestPublisher_GuiDuThuocTinhAMQP(t *testing.T) {
	f := newFakePubChannel()
	p := newPublisherWith(f, f.returns)
	ts := time.Date(2026, 10, 13, 9, 30, 0, 0, time.UTC)
	msg := Message{
		Body:            []byte("noi-dung"),
		ContentType:     "application/json",
		ContentEncoding: "gzip",
		Headers:         map[string]any{"tenant": "evt-1"},
		MessageID:       "m-1",
		CorrelationID:   "corr-1",
		ReplyTo:         "amq.rabbitmq.reply-to",
		Type:            "chat.ask",
		AppID:           "gateway",
		Priority:        4,
		Timestamp:       ts,
	}
	if err := within(t, 2*time.Second, func() error { return p.Publish(context.Background(), "ex", "k", msg) }); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	m := f.lastPublished(t)
	if string(m.Body) != "noi-dung" || m.ContentType != msg.ContentType || m.ContentEncoding != msg.ContentEncoding ||
		m.MessageId != msg.MessageID || m.CorrelationId != msg.CorrelationID || m.ReplyTo != msg.ReplyTo ||
		m.Type != msg.Type || m.AppId != msg.AppID || m.Priority != msg.Priority || !m.Timestamp.Equal(ts) ||
		m.DeliveryMode != amqp.Persistent {
		t.Fatalf("publishing thieu/sai property: %+v", m)
	}
	if m.Headers["tenant"] != "evt-1" {
		t.Errorf("mat header: %v", m.Headers)
	}
}

// F3 (challenge): hai Publish dong thoi tren mot Publisher; chi lan unroutable nhan ErrUnroutable.
// A bat dau truoc va cho confirm 80ms; B bat dau o 20ms. Neu khong co mutex, B publish va cho cung luc voi A
// nen mot ben doc mat return cua ben kia:
//   - "A unroutable": return cua A toi o 40ms; B (doc sau khi cho, hoac pre-drain) co the nuot no.
//   - "B unroutable": return cua B toi 20ms sau khi B gui; A dang cho tu truoc (nguoi nhan cho lau nhat
//     cua kenh Go) se doc va bo no, B nhan nil.
//
// Moi kich ban lap 5 lan.
func TestPublisher_PublishDongThoiChiLanUnroutableLoi(t *testing.T) {
	for _, aUnroutable := range []bool{true, false} {
		for i := 0; i < 5; i++ {
			runConcurrentPair(t, aUnroutable, i)
		}
	}
}

func runConcurrentPair(t *testing.T, aUnroutable bool, iter int) {
	t.Helper()
	keyA, keyB := "chat", "khong-co-binding"
	if aUnroutable {
		keyA, keyB = keyB, keyA
	}
	f := newFakePubChannel()
	f.onPublish = func(f *fakePubChannel, ex, key string, msg amqp.Publishing) (confirmation, error) {
		done := make(chan struct{})
		id := msg.Headers[hdrPublishID]
		isA := key == keyA
		go func() {
			if key == "khong-co-binding" {
				if isA {
					time.Sleep(40 * time.Millisecond)
				} else {
					time.Sleep(20 * time.Millisecond)
				}
				f.returns <- amqp.Return{Exchange: ex, RoutingKey: key, ReplyCode: 312, Headers: amqp.Table{hdrPublishID: id}}
			}
			if isA {
				time.Sleep(80 * time.Millisecond)
			} else {
				time.Sleep(60 * time.Millisecond)
			}
			close(done)
		}()
		return fakeConfirm{ack: true, done: done}, nil
	}
	p := newPublisherWith(f, f.returns)

	errA, errB := make(chan error, 1), make(chan error, 1)
	go func() { errA <- p.Publish(context.Background(), "ai.retry", keyA, Message{}) }()
	time.Sleep(20 * time.Millisecond)
	go func() { errB <- p.Publish(context.Background(), "ai.retry", keyB, Message{}) }()

	for _, side := range []struct {
		name string
		ch   chan error
		key  string
	}{{"A", errA, keyA}, {"B", errB, keyB}} {
		select {
		case err := <-side.ch:
			if side.key == "khong-co-binding" && !errors.Is(err, ErrUnroutable) {
				t.Fatalf("A unroutable=%v lan %d: %s = %v, muon ErrUnroutable (return bi publish khac nuot)",
					aUnroutable, iter, side.name, err)
			}
			if side.key != "khong-co-binding" && err != nil {
				t.Fatalf("A unroutable=%v lan %d: %s = %v, muon nil", aUnroutable, iter, side.name, err)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("A unroutable=%v lan %d: %s khong tra ve", aUnroutable, iter, side.name)
		}
	}
}

// P6 (challenge): kenh dong dung luc gui (publish tra amqp091.ErrClosed, sau do IsClosed = true)
// -> loi boc ErrClosed cua thu vien.
func TestPublisher_LoiGuiKhiKenhVuaDongThanhErrClosed(t *testing.T) {
	f := newFakePubChannel()
	f.closedSeq = []bool{false, true}
	f.onPublish = func(*fakePubChannel, string, string, amqp.Publishing) (confirmation, error) {
		return nil, amqp.ErrClosed
	}
	p := newPublisherWith(f, f.returns)
	err := within(t, 2*time.Second, func() error { return p.Publish(context.Background(), "ex", "k", Message{}) })
	if !errors.Is(err, ErrClosed) || !errors.Is(err, amqp.ErrClosed) {
		t.Fatalf("err = %v, muon boc ErrClosed va amqp091.ErrClosed", err)
	}
}
