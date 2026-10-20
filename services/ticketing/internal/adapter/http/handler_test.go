package httpadapter

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
	"github.com/eventflow/eventflow/libs/go/idem"
	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
)

// fakeHolds dem so lan use case that su chay: dieu BR-O5 phai chan.
type fakeHolds struct {
	calls atomic.Int32
	err   error

	mu   sync.Mutex
	last app.CreateHoldInput
}

func (f *fakeHolds) lastInput() app.CreateHoldInput {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.last
}

func (f *fakeHolds) Execute(_ context.Context, in app.CreateHoldInput) (app.CreateHoldOutput, error) {
	f.mu.Lock()
	f.last = in
	f.mu.Unlock()
	n := f.calls.Add(1)
	if f.err != nil {
		return app.CreateHoldOutput{}, f.err
	}
	return app.CreateHoldOutput{
		OrderID:   "order-" + strconv.Itoa(int(n)),
		HoldID:    "hold-" + strconv.Itoa(int(n)),
		ExpiresAt: time.Date(2026, 10, 13, 12, 10, 0, 0, time.UTC),
	}, nil
}

// memIdem la idem.Store toi thieu: du de chung minh Routes noi dung middleware vao dung route.
type memIdem struct {
	mu   sync.Mutex
	rows map[string]*memRow
	seq  int
}

type memRow struct {
	hash  string
	done  bool
	res   idem.Response
	token string
}

func (m *memIdem) Begin(_ context.Context, r idem.Request, _, _ time.Duration) (idem.Outcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rows == nil {
		m.rows = map[string]*memRow{}
	}
	k := r.Key + "|" + r.Endpoint
	row, ok := m.rows[k]
	if !ok {
		m.seq++
		m.rows[k] = &memRow{hash: r.Hash, token: strconv.Itoa(m.seq)}
		return idem.Outcome{State: idem.Acquired, Token: strconv.Itoa(m.seq)}, nil
	}
	switch {
	case row.hash != r.Hash:
		return idem.Outcome{State: idem.Mismatch}, nil
	case !row.done:
		return idem.Outcome{State: idem.InProgress}, nil
	}
	return idem.Outcome{State: idem.Replay, Response: row.res}, nil
}

func (m *memIdem) Complete(_ context.Context, r idem.Request, _ string, res idem.Response) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	row := m.rows[r.Key+"|"+r.Endpoint]
	row.done, row.res = true, res
	return nil
}

func (m *memIdem) Abandon(_ context.Context, r idem.Request, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rows, r.Key+"|"+r.Endpoint)
	return nil
}

// Danh tinh va queue token hop le cho su kien e1 (EV-137).
const (
	testUser   = "11111111-1111-4111-8111-111111111111"
	testDevice = "33333333-3333-4333-8333-333333333333"
	testJTI    = "jti-test-1"
)

var testQueueSecret = []byte("ticketing-test-queue-secret-16+")

func queueToken(_ testing.TB, c authx.QueueClaims) string {
	if c.IssuedAt.IsZero() {
		c.IssuedAt = time.Now()
		c.ExpiresAt = c.IssuedAt.Add(time.Hour)
	}
	tok, err := authx.SignQueueToken(testQueueSecret, c)
	if err != nil {
		panic(err)
	}
	return tok
}

func validQueueToken(t testing.TB) string {
	return queueToken(t, authx.QueueClaims{EventID: "e1", Subject: testUser, DeviceID: testDevice, JTI: testJTI})
}

func setPrincipal(r *http.Request, id, device string) {
	r.Header.Set("X-Identity-Id", id)
	r.Header.Set("X-Identity-Role", "buyer")
	if device != "" {
		r.Header.Set("X-Device-Id", device)
	}
}

func newServerLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newServer(holds HoldCreator) http.Handler {
	h := New(holds, slog.New(slog.NewTextHandler(io.Discard, nil))).WithQueueTokens(testQueueSecret)
	mux := http.NewServeMux()
	h.Routes(mux, idem.Middleware(&memIdem{},
		idem.WithIdentity(func(r *http.Request) string { return r.Header.Get("X-Identity-Id") })))
	return mux
}

func post(h http.Handler, key, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/v1/events/e1/holds", strings.NewReader(body))
	if key != "" {
		r.Header.Set(idem.Header, key)
	}
	setPrincipal(r, testUser, testDevice)
	r.Header.Set("X-Queue-Token", validQueueToken(nil))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

const holdBody = `{"ticket_type_id":"tt1","quantity":2}`

func TestCreateHold_ThieuIdempotencyKey_400VaKhongTaoHold(t *testing.T) {
	holds := &fakeHolds{}
	w := post(newServer(holds), "", holdBody)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, muon 400", w.Code)
	}
	if holds.calls.Load() != 0 {
		t.Fatalf("use case chay %d lan khi thieu key (BR-O5)", holds.calls.Load())
	}
}

func TestCreateHold_GoiLaiCungKey_TraDungResponseCuVaChiTaoMotHold(t *testing.T) {
	holds := &fakeHolds{}
	srv := newServer(holds)

	first := post(srv, "key-1", holdBody)
	second := post(srv, "key-1", holdBody)

	if holds.calls.Load() != 1 {
		t.Fatalf("use case chay %d lan, muon 1 (BR-O5)", holds.calls.Load())
	}
	if first.Code != 200 || second.Code != 200 {
		t.Fatalf("status %d / %d", first.Code, second.Code)
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("response lech:\n%s\n%s", first.Body.String(), second.Body.String())
	}
	if second.Header().Get(idem.ReplayedHeader) != "true" {
		t.Fatal("thieu Idempotent-Replayed")
	}
}

func TestCreateHold_HetVe409_DuocNhoDeKhongHoiLaiNghiepVu(t *testing.T) {
	holds := &fakeHolds{err: domain.ErrSoldOut}
	srv := newServer(holds)

	post(srv, "key-1", holdBody)
	second := post(srv, "key-1", holdBody)

	if second.Code != http.StatusConflict || holds.calls.Load() != 1 {
		t.Fatalf("status=%d use-case=%d", second.Code, holds.calls.Load())
	}
}

func TestCreateHold_LoiHeThong503_KhongBiKhoaVaoKey(t *testing.T) {
	holds := &fakeHolds{err: context.DeadlineExceeded}
	srv := newServer(holds)

	first := post(srv, "key-1", holdBody)
	holds.err = nil
	second := post(srv, "key-1", holdBody)

	if first.Code != http.StatusServiceUnavailable {
		t.Fatalf("lan 1 = %d, muon 503", first.Code)
	}
	if second.Code != 200 || second.Header().Get(idem.ReplayedHeader) != "" {
		t.Fatalf("lan 2 phai chay lai that su: %d replayed=%q", second.Code, second.Header().Get(idem.ReplayedHeader))
	}
}
