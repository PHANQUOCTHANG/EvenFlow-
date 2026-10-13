package mq

import (
	"context"
	"errors"
	"sync"
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

// fakeConfirm gia lap DeferredConfirmation.
type fakeConfirm struct {
	ack   bool
	err   error
	block bool // cho toi khi ctx huy
}

func (c fakeConfirm) WaitContext(ctx context.Context) (bool, error) {
	if c.block {
		<-ctx.Done()
		return false, ctx.Err()
	}
	return c.ack, c.err
}

// fakePubChannel gia lap kenh publish. onPublish quyet dinh confirm va co the day return / dong kenh.
type fakePubChannel struct {
	mu        sync.Mutex
	closedSeq []bool // ket qua lan luot cua IsClosed; het danh sach thi dung phan tu cuoi
	calls     int
	returns   chan amqp.Return
	published []amqp.Publishing
	mandatory []bool
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
