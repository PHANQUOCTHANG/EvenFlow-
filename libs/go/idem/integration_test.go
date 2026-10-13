//go:build integration

package idem

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eventflow/eventflow/libs/go/testx"
)

// Test tich hop voi Postgres that (schema tu migrations/0001_core.sql). Chay:
//
//	go test -tags=integration ./idem/...
//
// Khong co Docker thi FAIL (khong skip) theo hop dong cua testx. Cac test dung chung mot
// container va co lap nhau bang key rieng.

func TestMain(m *testing.M) {
	code := m.Run()
	testx.Shutdown()
	os.Exit(code)
}

var keySeq atomic.Int64

func uniqueKey(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%s-%d-%d", t.Name(), time.Now().UnixNano(), keySeq.Add(1))
}

func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testx.Postgres(t))
	if err != nil {
		t.Fatalf("mo pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func realHandler(t *testing.T, next http.Handler, opts ...Option) (http.Handler, *pgxpool.Pool) {
	t.Helper()
	pool := newPool(t)
	all := append([]Option{WithIdentity(identityHeader)}, opts...)
	return Middleware(NewPostgresStore(pool), all...)(next), pool
}

func rowOf(t *testing.T, pool *pgxpool.Pool, key string) (status *int, identity *string, found bool) {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`SELECT status_code, identity_id::text FROM idempotency_keys WHERE key = $1`, key).Scan(&status, &identity)
	if err != nil {
		return nil, nil, false
	}
	return status, identity, true
}

const uuidA = "00000000-0000-4000-8000-00000000000a"

func TestTichHop_GoiLaiTraDungResponseCuTungByte(t *testing.T) {
	// JSONB sap xep lai khoa va bo khoang trang; response phat lai van phai giong tung byte.
	const body = "{\"z\":1,   \"a\":[3,2,1] ,\"hold_id\":\"h-é\"}\n"
	var n atomic.Int32
	h, _ := realHandler(t, counting(&n, 200, body))
	key := uniqueKey(t)

	first := call{key: key, identity: uuidA, body: `{"q":1}`}.do(h)
	second := call{key: key, identity: uuidA, body: `{"q":1}`}.do(h)

	if n.Load() != 1 {
		t.Fatalf("handler chay %d lan, muon 1", n.Load())
	}
	if first.Body.String() != body || second.Body.String() != body {
		t.Fatalf("body lech:\n goc : %q\n lan1: %q\n lan2: %q", body, first.Body.String(), second.Body.String())
	}
	if second.Header().Get(ReplayedHeader) != "true" || second.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("header phat lai sai: %v", second.Header())
	}
}

func TestTichHop_DongThoiCungKhoaChiMotHandlerChay(t *testing.T) {
	var n atomic.Int32
	// Handler du cham de moi request thua deu den khi khoa con dang xu ly.
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hold_id":"h1"}`))
	})
	h, _ := realHandler(t, next)
	key := uniqueKey(t)

	const total = 50
	var (
		wg        sync.WaitGroup
		start     = make(chan struct{})
		ok        atomic.Int32
		conflict  atomic.Int32
		replayed  atomic.Int32
		unexpecte atomic.Int32
	)
	for range total {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			w := call{key: key, identity: uuidA, body: `{"q":1}`}.do(h)
			switch {
			case w.Code == http.StatusConflict:
				conflict.Add(1)
			case w.Code == 200 && w.Header().Get(ReplayedHeader) == "true":
				replayed.Add(1)
			case w.Code == 200:
				ok.Add(1)
			default:
				unexpecte.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if n.Load() != 1 {
		t.Fatalf("handler chay %d lan, muon DUNG 1 (ok=%d conflict=%d replayed=%d)",
			n.Load(), ok.Load(), conflict.Load(), replayed.Load())
	}
	if ok.Load() != 1 || unexpecte.Load() != 0 || ok.Load()+conflict.Load()+replayed.Load() != total {
		t.Fatalf("ok=%d conflict=%d replayed=%d la=%d", ok.Load(), conflict.Load(), replayed.Load(), unexpecte.Load())
	}
}

func TestTichHop_KhacBodyHoacKhacNguoi_Tra422(t *testing.T) {
	var n atomic.Int32
	h, _ := realHandler(t, counting(&n, 200, `{"hold_id":"cua-A"}`))
	key := uniqueKey(t)

	call{key: key, identity: uuidA, body: `{"quantity":1}`}.do(h)
	diffBody := call{key: key, identity: uuidA, body: `{"quantity":4}`}.do(h)
	diffUser := call{key: key, identity: "00000000-0000-4000-8000-00000000000b", body: `{"quantity":1}`}.do(h)

	for name, w := range map[string]*httptest.ResponseRecorder{"khac body": diffBody, "khac nguoi": diffUser} {
		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: status %d, muon 422", name, w.Code)
		}
		if strings.Contains(w.Body.String(), "cua-A") {
			t.Errorf("%s: LO response cua nguoi khac: %s", name, w.Body.String())
		}
	}
	if n.Load() != 1 {
		t.Fatalf("handler chay %d lan, muon 1", n.Load())
	}
}

func TestTichHop_Loi5xxVaPanic_XoaKhoa(t *testing.T) {
	var n atomic.Int32
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch n.Add(1) {
		case 1:
			http.Error(w, "boom", http.StatusServiceUnavailable)
		case 2:
			panic("loi lap trinh")
		default:
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	})
	h, pool := realHandler(t, next)
	key := uniqueKey(t)

	if w := (call{key: key, identity: uuidA, body: `{}`}).do(h); w.Code != 503 {
		t.Fatalf("lan 1 = %d", w.Code)
	}
	if _, _, found := rowOf(t, pool, key); found {
		t.Fatal("khoa con ton tai sau 5xx")
	}
	func() {
		defer func() { _ = recover() }()
		call{key: key, identity: uuidA, body: `{}`}.do(h)
	}()
	if _, _, found := rowOf(t, pool, key); found {
		t.Fatal("khoa con ton tai sau panic")
	}
	w := call{key: key, identity: uuidA, body: `{}`}.do(h)
	if w.Code != 200 || n.Load() != 3 {
		t.Fatalf("lan 3: status=%d handler=%d", w.Code, n.Load())
	}
}

func TestTichHop_HetHan_KhoaDuocDungLaiVoiNoiDungKhac(t *testing.T) {
	var n atomic.Int32
	h, pool := realHandler(t, counting(&n, 200, `{"ok":true}`))
	key := uniqueKey(t)

	call{key: key, identity: uuidA, body: `{"v":1}`}.do(h)
	if _, err := pool.Exec(context.Background(),
		`UPDATE idempotency_keys SET expires_at = now() - interval '1 second' WHERE key = $1`, key); err != nil {
		t.Fatal(err)
	}
	w := call{key: key, identity: uuidA, body: `{"v":2}`}.do(h)

	if w.Code != 200 || w.Header().Get(ReplayedHeader) != "" || n.Load() != 2 {
		t.Fatalf("status=%d replayed=%q handler=%d", w.Code, w.Header().Get(ReplayedHeader), n.Load())
	}
}

func TestTichHop_QuaLease_TiepQuanVaChuCuKhongGhiDe(t *testing.T) {
	var n atomic.Int32
	firstStarted, releaseFirst := make(chan struct{}), make(chan struct{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			close(firstStarted)
			<-releaseFirst
			_, _ = w.Write([]byte(`{"chu":"cu"}`))
			return
		}
		_, _ = w.Write([]byte(`{"chu":"moi"}`))
	})
	h, _ := realHandler(t, next, WithLease(400*time.Millisecond))
	key := uniqueKey(t)

	oldDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { oldDone <- call{key: key, identity: uuidA, body: `{}`}.do(h) }()
	<-firstStarted

	time.Sleep(600 * time.Millisecond) // qua lease
	takeover := call{key: key, identity: uuidA, body: `{}`}.do(h)
	if takeover.Body.String() != `{"chu":"moi"}` {
		t.Fatalf("request sau phai tiep quan, nhan %q", takeover.Body.String())
	}

	close(releaseFirst)
	if old := <-oldDone; old.Body.String() != `{"chu":"cu"}` {
		t.Fatalf("chu cu phai nhan response cua chinh no, nhan %q", old.Body.String())
	}

	again := call{key: key, identity: uuidA, body: `{}`}.do(h)
	if again.Body.String() != `{"chu":"moi"}` || again.Header().Get(ReplayedHeader) != "true" {
		t.Fatalf("chu cu da ghi de ket qua cua nguoi tiep quan: %q", again.Body.String())
	}
}

func TestTichHop_ClientNgat_VanLuuResponse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var n atomic.Int32
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		cancel() // client bo di ngay sau khi nghiep vu chay
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	h, pool := realHandler(t, next)
	key := uniqueKey(t)

	r := httptest.NewRequest(http.MethodPost, "/v1/events/e1/holds", strings.NewReader(`{}`)).WithContext(ctx)
	r.Header.Set(Header, key)
	h.ServeHTTP(httptest.NewRecorder(), r)

	status, _, found := rowOf(t, pool, key)
	if !found || status == nil || *status != 200 {
		t.Fatalf("response khong duoc luu sau khi client ngat (found=%v status=%v)", found, status)
	}
	w := call{key: key, body: `{}`}.do(h)
	if w.Header().Get(ReplayedHeader) != "true" || n.Load() != 1 {
		t.Fatalf("replayed=%q handler=%d", w.Header().Get(ReplayedHeader), n.Load())
	}
}

func TestTichHop_DinhDanhKhongPhaiUUID_VanChayVaCotIdentityNull(t *testing.T) {
	// Frontend hien gui "guest-user": khong duoc lam hong luong mua ve.
	var n atomic.Int32
	h, pool := realHandler(t, counting(&n, 200, `{"ok":true}`))
	key := uniqueKey(t)

	w := call{key: key, identity: "guest-user", body: `{}`}.do(h)
	again := call{key: key, identity: "guest-user", body: `{}`}.do(h)

	if w.Code != 200 || again.Header().Get(ReplayedHeader) != "true" || n.Load() != 1 {
		t.Fatalf("status=%d replayed=%q handler=%d", w.Code, again.Header().Get(ReplayedHeader), n.Load())
	}
	if _, id, _ := rowOf(t, pool, key); id != nil {
		t.Fatalf("identity_id = %q, muon NULL", *id)
	}
	// Nguoi khac van khong dung duoc key nay du cot la NULL.
	other := call{key: key, identity: "guest-other", body: `{}`}.do(h)
	if other.Code != http.StatusUnprocessableEntity {
		t.Fatalf("nguoi khac nhan %d, muon 422", other.Code)
	}
}

func TestTichHop_KhoaDaDuocLuuVoiIdentityUUID(t *testing.T) {
	h, pool := realHandler(t, counting(new(atomic.Int32), 200, `{}`))
	key := uniqueKey(t)

	call{key: key, identity: uuidA, body: `{}`}.do(h)

	status, id, found := rowOf(t, pool, key)
	if !found || status == nil || id == nil || *id != uuidA {
		t.Fatalf("found=%v status=%v identity=%v", found, status, id)
	}
}

func TestTichHop_StoreLoi_Tra503(t *testing.T) {
	var n atomic.Int32
	pool := newPool(t)
	h := Middleware(NewPostgresStore(pool), WithIdentity(identityHeader))(counting(&n, 200, `{}`))
	pool.Close() // mat ket noi: Begin phai loi

	w := call{key: uniqueKey(t), identity: uuidA, body: `{}`}.do(h)

	if w.Code != http.StatusServiceUnavailable || n.Load() != 0 {
		t.Fatalf("status=%d handler=%d", w.Code, n.Load())
	}
	if b, _ := io.ReadAll(w.Body); strings.Contains(string(b), "pool") {
		t.Fatalf("lo loi noi bo: %s", b)
	}
}
