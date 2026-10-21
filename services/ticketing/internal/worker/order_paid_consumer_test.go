package worker_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/eventflow/eventflow/libs/go/mq"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
	"github.com/eventflow/eventflow/services/ticketing/internal/worker"
)

type fakeIssuer struct {
	calls []port.IssueTicketsParams
	err   error
	res   port.IssueTicketsResult
}

func (f *fakeIssuer) Execute(_ context.Context, in port.IssueTicketsParams) (port.IssueTicketsResult, error) {
	f.calls = append(f.calls, in)
	return f.res, f.err
}

func handle(t *testing.T, issuer *fakeIssuer, body string) error {
	t.Helper()
	h := worker.NewOrderPaidHandler(issuer, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return h.Handle(context.Background(), mq.Delivery{
		Body:       []byte(body),
		RoutingKey: "order.paid",
		MessageID:  "42",
		Headers:    map[string]any{"x-trace-id": "trace-1"},
		Attempt:    1,
	})
}

func TestOrderPaid_PhatVeTheoOrderID(t *testing.T) {
	issuer := &fakeIssuer{}
	if err := handle(t, issuer, `{"order_id":"o-1","payment_id":"p-1","amount_cents":500}`); err != nil {
		t.Fatalf("Handle = %v", err)
	}
	if len(issuer.calls) != 1 || issuer.calls[0].OrderID != "o-1" || issuer.calls[0].TraceID != "trace-1" {
		t.Fatalf("calls = %+v", issuer.calls)
	}
}

// Message giao lap (at-least-once): use case tra AlreadyIssued, handler ack chu khong loi.
func TestOrderPaid_GiaoLapLaAck(t *testing.T) {
	issuer := &fakeIssuer{res: port.IssueTicketsResult{OrderID: "o-1", AlreadyIssued: true}}
	for i := 0; i < 2; i++ {
		if err := handle(t, issuer, `{"order_id":"o-1"}`); err != nil {
			t.Fatalf("lan %d: Handle = %v", i+1, err)
		}
	}
}

func TestOrderPaid_LoiVinhVienVaoDLQ(t *testing.T) {
	cases := map[string]struct {
		body string
		err  error
	}{
		"json hong":      {`{`, nil},
		"thieu order_id": {`{"payment_id":"p"}`, nil},
		"don khong co":   {`{"order_id":"o-1"}`, domain.ErrOrderNotFound},
		"don khong PAID": {`{"order_id":"o-1"}`, domain.ErrOrderNotPaid},
	}
	for name, c := range cases {
		err := handle(t, &fakeIssuer{err: c.err}, c.body)
		if !errors.Is(err, mq.ErrPermanent) {
			t.Errorf("%s: err = %v, muon boc mq.ErrPermanent", name, err)
		}
	}
}

// DB tam mat: loi thuong de mq.Consume retry, khong day thang DLQ.
func TestOrderPaid_LoiTamThoiDuocRetry(t *testing.T) {
	err := handle(t, &fakeIssuer{err: errors.New("connection refused")}, `{"order_id":"o-1"}`)
	if err == nil || errors.Is(err, mq.ErrPermanent) {
		t.Fatalf("err = %v, muon loi thuong (retry)", err)
	}
}

func TestOrderPaidConsumerConfig_KhopTopology(t *testing.T) {
	cfg := worker.OrderPaidConsumerConfig()
	if cfg.Queue != mq.TicketingOrderPaidQueue || cfg.RetryExchange != mq.TicketingRetryExchange ||
		cfg.RetryKey != mq.TicketingOrderPaidKey || cfg.DeadLetterExchange != mq.TicketingDeadLetterEx ||
		cfg.DeadLetterKey != mq.TicketingOrderPaidKey {
		t.Errorf("cau hinh lech topology: %+v", cfg)
	}
}
