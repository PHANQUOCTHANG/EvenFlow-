package idem

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- store gia: cung ngu nghia voi PostgresStore, co dong ho gia de thu TTL va lease ---

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type memRow struct {
	hash    string
	done    bool
	res     Response
	created time.Time
	expires time.Time
	token   string
}

type memStore struct {
	mu       sync.Mutex
	clk      *clock
	rows     map[[2]string]*memRow
	seq      int
	beginErr error
	// Dem de test khang dinh hanh vi.
	abandoned atomic.Int32
}

func newMemStore(clk *clock) *memStore {
	return &memStore{clk: clk, rows: map[[2]string]*memRow{}}
}

func (m *memStore) Begin(_ context.Context, req Request, ttl, lease time.Duration) (Outcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.beginErr != nil {
		return Outcome{}, m.beginErr
	}
	now := m.clk.now()
	k := [2]string{req.Key, req.Endpoint}
	row, ok := m.rows[k]
	if !ok || !row.expires.After(now) || (!row.done && !row.created.Add(lease).After(now)) {
		m.seq++
		m.rows[k] = &memRow{hash: req.Hash, created: now, expires: now.Add(ttl), token: strconv.Itoa(m.seq)}
		return Outcome{State: Acquired, Token: strconv.Itoa(m.seq)}, nil
	}
	switch {
	case row.hash != req.Hash:
		return Outcome{State: Mismatch}, nil
	case !row.done:
		return Outcome{State: InProgress}, nil
	}
	return Outcome{State: Replay, Response: row.res}, nil
}

func (m *memStore) Complete(ctx context.Context, req Request, token string, res Response) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[[2]string{req.Key, req.Endpoint}]
	if !ok || row.token != token || row.done {
		return ErrLeaseLost
	}
	row.done, row.res = true, res
	return nil
}

func (m *memStore) Abandon(_ context.Context, req Request, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := [2]string{req.Key, req.Endpoint}
	if row, ok := m.rows[k]; ok && row.token == token {
		delete(m.rows, k)
		m.abandoned.Add(1)
	}
	return nil
}

// --- tien ich ---

func newClock() *clock { return &clock{t: time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)} }

func identityHeader(r *http.Request) string { return r.Header.Get("X-Identity-Id") }

type call struct {
	key, identity, body, path string
}

func (c call) do(h http.Handler) *httptest.ResponseRecorder {
	path := c.path
	if path == "" {
		path = "/v1/events/e1/holds"
	}
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(c.body))
	if c.key != "" {
		r.Header.Set(Header, c.key)
	}
	if c.identity != "" {
		r.Header.Set("X-Identity-Id", c.identity)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// counting la handler dem so lan chay, tra JSON co thu tu khoa va khoang trang co chu y.
func counting(n *atomic.Int32, status int, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
}

func setup(t *testing.T, next http.Handler, opts ...Option) (http.Handler, *memStore, *clock) {
	t.Helper()
	clk := newClock()
	st := newMemStore(clk)
	all := append([]Option{WithIdentity(identityHeader)}, opts...)
	return Middleware(st, all...)(next), st, clk
}

// --- AC-1: thieu header -> 400 ---

func TestThieuHoacSaiKhoa_Tra400VaKhongChayHandler(t *testing.T) {
	cases := map[string]string{
		"thieu":      "",
		"co khoang":  "a b",
		"dieu khien": "a\x01b",
		"tieng viet": "khóa",
		"qua dai":    strings.Repeat("k", 256),
	}
	for name, key := range cases {
		t.Run(name, func(t *testing.T) {
			var n atomic.Int32
			h, _, _ := setup(t, counting(&n, 200, `{}`))

			r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{}`))
			if key != "" {
				r.Header[Header] = []string{key} // gan thang: Set se chuan hoa/khong cho ky tu la
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, muon 400", w.Code)
			}
			if n.Load() != 0 {
				t.Fatalf("handler chay %d lan, muon 0", n.Load())
			}
		})
	}
}

func TestKhoaDoi_Tra400(t *testing.T) {
	var n atomic.Int32
	h, _, _ := setup(t, counting(&n, 200, `{}`))
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{}`))
	r.Header.Add(Header, "k1")
	r.Header.Add(Header, "k2")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest || n.Load() != 0 {
		t.Fatalf("status=%d handler=%d", w.Code, n.Load())
	}
}

// --- AC-2: goi lai cung key tra DUNG response cu ---

func TestGoiLai_TraDungResponseCuVaKhongChayLai(t *testing.T) {
	// Khoang trang va thu tu khoa JSON co chu y: phat lai phai dung tung byte.
	const body = "{\"hold_id\":\"h1\",  \"expires_at\":\"2026-10-13T12:10:00Z\" ,\"a\":1}\n"
	var n atomic.Int32
	h, _, _ := setup(t, counting(&n, 200, body))

	first := call{key: "k1", identity: "u1", body: `{"q":1}`}.do(h)
	second := call{key: "k1", identity: "u1", body: `{"q":1}`}.do(h)

	if n.Load() != 1 {
		t.Fatalf("handler chay %d lan, muon 1", n.Load())
	}
	if first.Body.String() != body || second.Body.String() != body {
		t.Fatalf("body khong khop:\n lan 1: %q\n lan 2: %q", first.Body.String(), second.Body.String())
	}
	if second.Code != 200 || second.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status/content-type lan 2 = %d %q", second.Code, second.Header().Get("Content-Type"))
	}
	if second.Header().Get(ReplayedHeader) != "true" {
		t.Fatal("thieu header Idempotent-Replayed tren response phat lai")
	}
	if first.Header().Get(ReplayedHeader) != "" {
		t.Fatal("response dau tien khong duoc co Idempotent-Replayed")
	}
}

func TestLoiNghiepVu4xx_DuocNhoDeKhachKhongBiLamPhienLai(t *testing.T) {
	var n atomic.Int32
	h, _, _ := setup(t, counting(&n, http.StatusConflict, `{"detail":"het ve"}`))

	call{key: "k1", identity: "u1", body: `{}`}.do(h)
	second := call{key: "k1", identity: "u1", body: `{}`}.do(h)

	if n.Load() != 1 || second.Code != http.StatusConflict {
		t.Fatalf("handler=%d status=%d", n.Load(), second.Code)
	}
}

func TestKhacDuongDan_LaHaiKhoaDocLap(t *testing.T) {
	var n atomic.Int32
	h, _, _ := setup(t, counting(&n, 200, `{}`))

	call{key: "k1", identity: "u1", body: `{}`, path: "/v1/events/e1/holds"}.do(h)
	w := call{key: "k1", identity: "u1", body: `{}`, path: "/v1/events/e2/holds"}.do(h)

	if n.Load() != 2 || w.Header().Get(ReplayedHeader) != "" {
		t.Fatalf("handler=%d replayed=%q", n.Load(), w.Header().Get(ReplayedHeader))
	}
}

// --- AC-3: dong thoi cung key -> chi 1 duoc xu ly ---

func TestDongThoiCungKhoa_ChiMotHandlerChay(t *testing.T) {
	var n atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			close(started)
		}
		<-release
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	h, _, _ := setup(t, next)

	const total = 50
	codes := make([]atomic.Int32, total)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { // nguoi thang cuoc khoa
		defer wg.Done()
		codes[0].Store(int32(call{key: "k1", identity: "u1", body: `{}`}.do(h).Code))
	}()
	<-started
	for i := 1; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := call{key: "k1", identity: "u1", body: `{}`}.do(h)
			codes[i].Store(int32(w.Code))
			if w.Code == http.StatusConflict {
				if w.Header().Get("Retry-After") == "" {
					t.Error("409 thieu Retry-After")
				}
				if !strings.Contains(w.Body.String(), ProblemInProgress) {
					t.Errorf("409 thieu type rieng: %s", w.Body.String())
				}
			}
		}()
	}
	// Cho tat ca request thua da nhan 409 truoc khi nha handler.
	deadline := time.After(5 * time.Second)
	for {
		done := 0
		for i := 1; i < total; i++ {
			if codes[i].Load() != 0 {
				done++
			}
		}
		if done == total-1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("request thua khong nhan 409 kip")
		case <-time.After(5 * time.Millisecond):
		}
	}
	close(release)
	wg.Wait()

	if n.Load() != 1 {
		t.Fatalf("handler chay %d lan, muon dung 1", n.Load())
	}
	if codes[0].Load() != 200 {
		t.Fatalf("request thang cuoc nhan %d, muon 200", codes[0].Load())
	}
	for i := 1; i < total; i++ {
		if codes[i].Load() != http.StatusConflict {
			t.Fatalf("request %d nhan %d, muon 409", i, codes[i].Load())
		}
	}
}

// --- khoa dung lai voi noi dung / nguoi khac ---

func TestCungKhoaKhacBody_Tra422(t *testing.T) {
	var n atomic.Int32
	h, _, _ := setup(t, counting(&n, 200, `{"a":1}`))

	call{key: "k1", identity: "u1", body: `{"quantity":1}`}.do(h)
	w := call{key: "k1", identity: "u1", body: `{"quantity":4}`}.do(h)

	if w.Code != http.StatusUnprocessableEntity || n.Load() != 1 {
		t.Fatalf("status=%d handler=%d", w.Code, n.Load())
	}
	if !strings.Contains(w.Body.String(), ProblemKeyReused) {
		t.Fatalf("thieu type: %s", w.Body.String())
	}
}

func TestCungKhoaKhacNguoi_Tra422VaKhongLoResponse(t *testing.T) {
	var n atomic.Int32
	h, _, _ := setup(t, counting(&n, 200, `{"hold_id":"cua-nguoi-A"}`))

	call{key: "k1", identity: "A", body: `{}`}.do(h)
	w := call{key: "k1", identity: "B", body: `{}`}.do(h)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d, muon 422", w.Code)
	}
	if strings.Contains(w.Body.String(), "cua-nguoi-A") {
		t.Fatalf("LO response cua nguoi khac: %s", w.Body.String())
	}
	if n.Load() != 1 {
		t.Fatalf("handler chay %d lan", n.Load())
	}
}

func TestHashPhanBietRanhGioiTruong(t *testing.T) {
	// ("ab" + body "c") khac ("a" + body "bc") du noi chuoi giong nhau.
	r1 := httptest.NewRequest(http.MethodPost, "/p", nil)
	a := hashRequest(r1, "ab", []byte("c"))
	b := hashRequest(r1, "a", []byte("bc"))
	if a == b {
		t.Fatal("hash trung khi ranh gioi truong khac nhau")
	}
}

// --- 5xx va panic khong duoc nho ---

func TestLoi5xx_XoaKhoaDeThuLaiThatSu(t *testing.T) {
	var n atomic.Int32
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			http.Error(w, "boom", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	h, st, _ := setup(t, next)

	first := call{key: "k1", identity: "u1", body: `{}`}.do(h)
	second := call{key: "k1", identity: "u1", body: `{}`}.do(h)
	third := call{key: "k1", identity: "u1", body: `{}`}.do(h)

	if first.Code != 503 {
		t.Fatalf("lan 1 = %d", first.Code)
	}
	if second.Code != 200 || second.Header().Get(ReplayedHeader) != "" {
		t.Fatalf("lan 2 phai chay lai that su: %d replayed=%q", second.Code, second.Header().Get(ReplayedHeader))
	}
	if third.Header().Get(ReplayedHeader) != "true" || n.Load() != 2 {
		t.Fatalf("lan 3 phai phat lai: replayed=%q handler=%d", third.Header().Get(ReplayedHeader), n.Load())
	}
	if st.abandoned.Load() != 1 {
		t.Fatalf("abandon = %d, muon 1", st.abandoned.Load())
	}
}

func TestPanic_XoaKhoaVaDePanicDiTiep(t *testing.T) {
	var n atomic.Int32
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			panic("loi lap trinh")
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	h, st, _ := setup(t, next)

	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic phai di tiep cho Recover o ngoai")
			}
		}()
		call{key: "k1", identity: "u1", body: `{}`}.do(h)
	}()
	if st.abandoned.Load() != 1 {
		t.Fatalf("khoa chua bi xoa sau panic (abandon=%d)", st.abandoned.Load())
	}
	w := call{key: "k1", identity: "u1", body: `{}`}.do(h)
	if w.Code != 200 || n.Load() != 2 {
		t.Fatalf("status=%d handler=%d", w.Code, n.Load())
	}
}

// --- TTL va lease ---

func TestHetTTL_KhoaDuocDungLaiNhuMoi(t *testing.T) {
	var n atomic.Int32
	h, _, clk := setup(t, counting(&n, 200, `{}`), WithTTL(24*time.Hour))

	call{key: "k1", identity: "u1", body: `{}`}.do(h)
	clk.advance(23 * time.Hour)
	w := call{key: "k1", identity: "u1", body: `{}`}.do(h)
	if w.Header().Get(ReplayedHeader) != "true" {
		t.Fatal("trong 24h phai phat lai")
	}
	clk.advance(2 * time.Hour)
	// Sau 24h cung key co the mang noi dung khac.
	w = call{key: "k1", identity: "u1", body: `{"khac":1}`}.do(h)
	if w.Code != 200 || w.Header().Get(ReplayedHeader) != "" || n.Load() != 2 {
		t.Fatalf("sau 24h phai chay lai: status=%d handler=%d", w.Code, n.Load())
	}
}

func TestQuaLease_RequestSauTiepQuanVaChuCuKhongGhiDe(t *testing.T) {
	var n atomic.Int32
	firstStarted, releaseFirst := make(chan struct{}), make(chan struct{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch n.Add(1) {
		case 1:
			close(firstStarted)
			<-releaseFirst
			_, _ = w.Write([]byte(`{"chu":"cu"}`))
		default:
			_, _ = w.Write([]byte(`{"chu":"moi"}`))
		}
	})
	h, _, clk := setup(t, next, WithLease(30*time.Second))

	oldDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { oldDone <- call{key: "k1", identity: "u1", body: `{}`}.do(h) }()
	<-firstStarted

	inLease := call{key: "k1", identity: "u1", body: `{}`}.do(h)
	if inLease.Code != http.StatusConflict {
		t.Fatalf("trong lease phai 409, nhan %d", inLease.Code)
	}
	clk.advance(31 * time.Second)

	takeover := call{key: "k1", identity: "u1", body: `{}`}.do(h)
	if takeover.Body.String() != `{"chu":"moi"}` {
		t.Fatalf("request sau phai tiep quan: %q", takeover.Body.String())
	}

	close(releaseFirst)
	old := <-oldDone
	if old.Body.String() != `{"chu":"cu"}` {
		t.Fatalf("chu cu van phai nhan response cua chinh no: %q", old.Body.String())
	}

	again := call{key: "k1", identity: "u1", body: `{}`}.do(h)
	if again.Body.String() != `{"chu":"moi"}` || again.Header().Get(ReplayedHeader) != "true" {
		t.Fatalf("chu cu da ghi de ket qua cua nguoi tiep quan: %q", again.Body.String())
	}
}

// --- client ngat ket noi ---

func TestClientNgatGiuaChung_VanLuuResponse(t *testing.T) {
	var n atomic.Int32
	clk := newClock()
	st := newMemStore(clk)
	ctx, cancel := context.WithCancel(context.Background())
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		cancel() // client bo di sau khi nghiep vu da chay
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	h := Middleware(st, WithIdentity(identityHeader))(next)

	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{}`)).WithContext(ctx)
	r.Header.Set(Header, "k1")
	h.ServeHTTP(httptest.NewRecorder(), r)

	w := call{key: "k1", body: `{}`, path: "/x"}.do(h)
	if w.Header().Get(ReplayedHeader) != "true" || n.Load() != 1 {
		t.Fatalf("response khong duoc luu sau khi client ngat: replayed=%q handler=%d",
			w.Header().Get(ReplayedHeader), n.Load())
	}
}

// --- loi ha tang ---

func TestStoreLoi_Tra503VaKhongChayHandler(t *testing.T) {
	var n atomic.Int32
	h, st, _ := setup(t, counting(&n, 200, `{}`))
	st.beginErr = errors.New("postgres chet")

	w := call{key: "k1", identity: "u1", body: `{}`}.do(h)

	if w.Code != http.StatusServiceUnavailable || n.Load() != 0 {
		t.Fatalf("status=%d handler=%d", w.Code, n.Load())
	}
	if strings.Contains(w.Body.String(), "postgres") {
		t.Fatalf("lo loi noi bo: %s", w.Body.String())
	}
}

// --- body ---

func TestHandlerNhanDuocBodyNguyenVen(t *testing.T) {
	var got string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
	})
	h, _, _ := setup(t, next)

	call{key: "k1", identity: "u1", body: `{"quantity":2}`}.do(h)

	if got != `{"quantity":2}` {
		t.Fatalf("handler nhan %q", got)
	}
}

func TestBodyQuaLon_Tra413(t *testing.T) {
	var n atomic.Int32
	h, _, _ := setup(t, counting(&n, 200, `{}`), WithMaxBody(8))

	w := call{key: "k1", identity: "u1", body: strings.Repeat("x", 9)}.do(h)

	if w.Code != http.StatusRequestEntityTooLarge || n.Load() != 0 {
		t.Fatalf("status=%d handler=%d", w.Code, n.Load())
	}
}
