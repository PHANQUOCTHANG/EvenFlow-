package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/httpx"
)

// Test viet tu hanh vi can bao dam cho moi service: loi dung dinh dang chuan,
// panic khong lam sap process va khong ro ri chi tiet, SSE van flush duoc, tat em
// khong cat ngang request dang chay.

func newLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

func TestWriteProblem_DungDinhDangRFC7807(t *testing.T) {
	rec := httptest.NewRecorder()
	httpx.WriteProblem(rec, http.StatusConflict, "rat tiec, hang ve nay da het")

	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("Content-Type = %q", got)
	}
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body khong phai JSON: %v", err)
	}
	want := map[string]any{
		"type": "about:blank", "title": "Conflict", "status": float64(409),
		"detail": "rat tiec, hang ve nay da het",
	}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("%s = %v, muon %v", k, body[k], v)
		}
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	httpx.WriteJSON(rec, http.StatusCreated, map[string]int{"n": 1})

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if strings.TrimSpace(rec.Body.String()) != `{"n":1}` {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestChain_PhanTuDauLaLopNgoaiCung(t *testing.T) {
	var order []string
	mk := func(name string) httpx.Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name+":vao")
				next.ServeHTTP(w, r)
				order = append(order, name+":ra")
			})
		}
	}
	h := httpx.Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		order = append(order, "handler")
	}), mk("a"), mk("b"))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

	want := "a:vao b:vao handler b:ra a:ra"
	if got := strings.Join(order, " "); got != want {
		t.Fatalf("thu tu = %q, muon %q", got, want)
	}
}

func TestRequestID(t *testing.T) {
	var seen string
	h := httpx.RequestID()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = httpx.RequestIDFrom(r.Context())
	}))

	t.Run("giu id hop le tu ben ngoai", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set(httpx.RequestIDHeader, "gw-abc.123")
		h.ServeHTTP(rec, req)

		if seen != "gw-abc.123" || rec.Header().Get(httpx.RequestIDHeader) != "gw-abc.123" {
			t.Fatalf("ctx=%q header=%q", seen, rec.Header().Get(httpx.RequestIDHeader))
		}
	})

	t.Run("tu sinh khi thieu", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

		if seen == "" || rec.Header().Get(httpx.RequestIDHeader) != seen {
			t.Fatalf("ctx=%q header=%q", seen, rec.Header().Get(httpx.RequestIDHeader))
		}
	})

	t.Run("bo id khong an toan", func(t *testing.T) {
		for _, bad := range []string{"co khoang trang", strings.Repeat("a", 65), "<script>"} {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set(httpx.RequestIDHeader, bad)
			h.ServeHTTP(rec, req)

			if seen == bad {
				t.Errorf("id %q khong duoc chap nhan", bad)
			}
		}
	})
}

func TestRecover_PanicThanh500_KhongLoChiTiet(t *testing.T) {
	log, buf := newLogger()
	h := httpx.Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("mat khau db la hunter2")
	}), httpx.Recover(log))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, muon 500", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("Content-Type = %q", got)
	}
	if strings.Contains(rec.Body.String(), "hunter2") {
		t.Errorf("chi tiet panic bi ro ri cho client: %s", rec.Body.String())
	}
	if !strings.Contains(buf.String(), "hunter2") || !strings.Contains(buf.String(), "stack") {
		t.Errorf("log phai co chi tiet panic va stack: %s", buf.String())
	}
}

func TestRecover_DaGuiHeader_KhongGhiDeMaTrangThai(t *testing.T) {
	log, _ := newLogger()
	h := httpx.Chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		panic("giua chung")
	}), httpx.Recover(log))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, header da gui thi phai giu 202", rec.Code)
	}
}

func TestRecover_ErrAbortHandlerDuocNemLai(t *testing.T) {
	log, _ := newLogger()
	h := httpx.Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}), httpx.Recover(log))

	defer func() {
		if r := recover(); r != http.ErrAbortHandler {
			t.Fatalf("muon http.ErrAbortHandler duoc nem lai, nhan %v", r)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
}

func TestAccessLog_GhiRouteMauVaMaTrangThai(t *testing.T) {
	log, buf := newLogger()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/events/{eventID}/holds", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteProblem(w, http.StatusConflict, "het ve")
	})
	h := httpx.Chain(mux, httpx.RequestID(), httpx.AccessLog(log))

	req := httptest.NewRequest("POST", "/v1/events/ev-bi-mat-123/holds", nil)
	req.Header.Set("X-Identity-Id", "00000000-0000-4000-8000-000000000001")
	h.ServeHTTP(httptest.NewRecorder(), req)

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log khong phai JSON: %v\n%s", err, buf.String())
	}
	if line["route"] != "POST /v1/events/{eventID}/holds" {
		t.Errorf("route = %v, muon mau route", line["route"])
	}
	if line["status"] != float64(409) {
		t.Errorf("status = %v", line["status"])
	}
	if line["request_id"] == "" || line["request_id"] == nil {
		t.Errorf("thieu request_id: %v", line)
	}
	// Khong ghi URL that va khong ghi header dinh danh (EVF-15).
	if s := buf.String(); strings.Contains(s, "ev-bi-mat-123") || strings.Contains(s, "00000000-0000-4000") {
		t.Errorf("log khong duoc chua URL that hay identity: %s", s)
	}
}

func TestAccessLog_ProbeSucKhoeGhiODebug(t *testing.T) {
	log, buf := newLogger()
	mux := http.NewServeMux()
	httpx.Health(mux)
	h := httpx.Chain(mux, httpx.AccessLog(log))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/readyz", nil))

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("khong co log: %v", err)
	}
	if line["level"] != "DEBUG" {
		t.Errorf("level = %v, probe phai o DEBUG", line["level"])
	}
}

// SSE cua phong cho kiem tra w.(http.Flusher). Neu middleware che mat Flusher thi
// moi stream tra 500.
func TestMiddleware_GiuDuocFlusherChoSSE(t *testing.T) {
	log, _ := newLogger()
	var flushable bool
	h := httpx.Chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f, ok := w.(http.Flusher)
		flushable = ok
		if ok {
			_, _ = io.WriteString(w, "event: position\n\n")
			f.Flush()
		}
	}), httpx.RequestID(), httpx.AccessLog(log), httpx.Recover(log))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if !flushable {
		t.Fatal("middleware da che mat http.Flusher")
	}
	if !rec.Flushed {
		t.Error("Flush khong duoc chuyen tiep toi writer goc")
	}
}

func TestHealth(t *testing.T) {
	get := func(mux *http.ServeMux, path string) int {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec.Code
	}

	t.Run("khong co check thi san sang", func(t *testing.T) {
		mux := http.NewServeMux()
		httpx.Health(mux)
		if c := get(mux, "/readyz"); c != 200 {
			t.Errorf("readyz = %d", c)
		}
	})

	t.Run("readyz 503 khi co phu thuoc hong, healthz van 200", func(t *testing.T) {
		mux := http.NewServeMux()
		httpx.Health(mux,
			func(context.Context) error { return nil },
			func(context.Context) error { return errors.New("redis chet") },
		)
		if c := get(mux, "/readyz"); c != 503 {
			t.Errorf("readyz = %d, muon 503", c)
		}
		// Mat phu thuoc khong duoc lam pod bi giet giua dot mo ban.
		if c := get(mux, "/healthz"); c != 200 {
			t.Errorf("healthz = %d, muon 200", c)
		}
	})

	t.Run("check nhan context co han", func(t *testing.T) {
		mux := http.NewServeMux()
		var hasDeadline bool
		httpx.Health(mux, func(ctx context.Context) error {
			_, hasDeadline = ctx.Deadline()
			return nil
		})
		get(mux, "/readyz")
		if !hasDeadline {
			t.Error("check phai nhan context co deadline de khong treo probe")
		}
	})
}

func TestServe_TatEm_ChoRequestDangChayXong(t *testing.T) {
	started := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /slow", func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		time.Sleep(200 * time.Millisecond)
		_, _ = io.WriteString(w, "xong")
	})
	srv := httpx.NewServer("", mux)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- httpx.Serve(ctx, srv, ln, 5*time.Second) }()

	var wg sync.WaitGroup
	var body string
	var reqErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		resp, err := http.Get("http://" + ln.Addr().String() + "/slow")
		if err != nil {
			reqErr = err
			return
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		body = string(b)
	}()

	<-started
	cancel() // SIGTERM giua luc request dang chay

	wg.Wait()
	if reqErr != nil {
		t.Fatalf("request dang chay bi cat ngang: %v", reqErr)
	}
	if body != "xong" {
		t.Errorf("body = %q", body)
	}
	if err := <-done; err != nil {
		t.Fatalf("Serve phai tra nil khi tat em thanh cong, nhan %v", err)
	}
}

func TestServe_LoiListener_DuocTraVe(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = ln.Close() // listener da dong: Serve phai tra loi that, khong phai nil

	err = httpx.Serve(context.Background(), httpx.NewServer("", http.NewServeMux()), ln, time.Second)
	if err == nil {
		t.Fatal("muon loi vi listener da dong")
	}
}

func TestRun_DiaChiDangDung_TraLoi(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	srv := httpx.NewServer(ln.Addr().String(), http.NewServeMux())
	if err := httpx.Run(context.Background(), srv, time.Second); err == nil {
		t.Fatal("muon loi khi cong da bi chiem")
	}
}
