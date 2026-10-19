//go:build integration

// Package queuetoken kiem EV-137 voi Redis that: handler phong cho + adapter Redis + token ky.
package queuetoken

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/testx"
	httpadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/http"
	redisadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/redis"
	"github.com/eventflow/eventflow/services/waitingroom/internal/domain"
)

const (
	alice  = "11111111-1111-4111-8111-111111111111"
	bob    = "22222222-2222-4222-8222-222222222222"
	laptop = "33333333-3333-4333-8333-333333333333"
	phone  = "44444444-4444-4444-8444-444444444444"
)

var secret = []byte("integration-test-secret-16bytes+")

func TestMain(m *testing.M) {
	code := m.Run()
	testx.Shutdown()
	os.Exit(code)
}

type store struct{ q *redisadapter.Queue }

func (s store) Join(ctx context.Context, eventID, identityID, token string) (httpadapter.JoinResult, error) {
	r, err := s.q.Join(ctx, eventID, identityID, token)
	return httpadapter.JoinResult{State: r.State, Token: r.Token, Rank: r.Rank, IsNew: r.IsNew}, err
}

func (s store) Status(ctx context.Context, eventID, token string) (domain.Position, error) {
	return s.q.Status(ctx, eventID, token)
}

type env struct {
	mux http.Handler
	rdb *redis.Client
	q   *redisadapter.Queue
	ev  string
}

func setup(t *testing.T) *env {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: testx.Redis(t)})
	t.Cleanup(func() { _ = rdb.Close() })
	q, err := redisadapter.NewQueue(context.Background(), rdb, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	httpadapter.New(store{q}, nil, slog.New(slog.NewTextHandler(io.Discard, nil))).WithQueueTokens(secret, true).Routes(mux)
	return &env{mux: mux, rdb: rdb, q: q, ev: "ev-" + strings.ReplaceAll(t.Name(), "/", "-")}
}

func (e *env) call(method, path string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/v1/events/"+e.ev+path, strings.NewReader(`{}`))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	return rec
}

func user(id, device string, extra ...string) map[string]string {
	h := map[string]string{"X-Identity-Id": id, "X-Identity-Role": "buyer", "X-Identity-Verified": "true", "X-Device-Id": device}
	for i := 0; i+1 < len(extra); i += 2 {
		h[extra[i]] = extra[i+1]
	}
	return h
}

func joinToken(t *testing.T, e *env, id, device string) string {
	t.Helper()
	rec := e.call(http.MethodPost, "/queue/join", user(id, device))
	if rec.Code != http.StatusOK {
		t.Fatalf("join %d: %s", rec.Code, rec.Body)
	}
	var b struct {
		Token string `json:"queue_token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &b)
	return b.Token
}

func state(t *testing.T, e *env, id, device, token string) (int, string) {
	t.Helper()
	rec := e.call(http.MethodGet, "/queue/status", user(id, device, "X-Queue-Token", token))
	var p struct {
		State string `json:"state"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	return rec.Code, p.State
}

func TestTichHop_QueueTokenKy_JoinStatusVaChiaSe(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	tokA := joinToken(t, e, alice, laptop)
	if !strings.HasPrefix(tokA, "q1.") {
		t.Fatalf("token phai la token ky, nhan %q", tokA)
	}
	if code, st := state(t, e, alice, laptop, tokA); code != http.StatusOK || st != "LOBBY" && st != "QUEUED" {
		t.Fatalf("alice status: %d %s", code, st)
	}

	// Chia se: Bob dung token cua Alice -> 403, Redis khong bi hoi.
	if code, _ := state(t, e, bob, laptop, tokA); code != http.StatusForbidden {
		t.Fatalf("bob dung token cua alice: %d, muon 403", code)
	}
	// Alice sang may khac voi cung token -> 403.
	if code, _ := state(t, e, alice, phone, tokA); code != http.StatusForbidden {
		t.Fatalf("alice sang thiet bi khac: %d, muon 403", code)
	}

	// Alice join lai tu dien thoai: BR-Q3 -- cung cho trong hang (cung jti), token moi gan voi dien thoai.
	tokPhone := joinToken(t, e, alice, phone)
	if tokPhone == tokA {
		t.Fatal("token moi phai gan thiet bi moi")
	}
	if code, _ := state(t, e, alice, phone, tokPhone); code != http.StatusOK {
		t.Fatalf("token dien thoai: %d", code)
	}
	members, _ := e.rdb.ZCard(ctx, "wr:{"+e.ev+"}:queue").Result()
	lobby, _ := e.rdb.SCard(ctx, "wr:{"+e.ev+"}:lobby").Result()
	if members+lobby != 1 {
		t.Fatalf("alice chiem %d cho (queue %d + lobby %d), muon dung 1", members+lobby, members, lobby)
	}

	// Redis chi giu jti (khong giu token ky dai).
	toks, _ := e.rdb.HGetAll(ctx, "wr:{"+e.ev+"}:tok").Result()
	for _, v := range toks {
		if strings.HasPrefix(v, "q1.") || len(v) > 64 {
			t.Fatalf("Redis dang luu token ky thay vi jti: %q", v)
		}
	}
}

// Token cua mot su kien khong dung duoc o su kien khac, ke ca khi cung nguoi va thiet bi.
func TestTichHop_QueueToken_KhongDungChoSuKienKhac(t *testing.T) {
	e := setup(t)
	tok := joinToken(t, e, alice, laptop)

	other := *e
	other.ev = e.ev + "-khac"
	if code, _ := state(t, &other, alice, laptop, tok); code != http.StatusForbidden {
		t.Fatalf("token su kien A dung o su kien B: %d, muon 403", code)
	}
}

// Admit theo jti: sau khi controller tha, status tra ADMITTED qua token ky.
func TestTichHop_QueueToken_AdmitTheoJTI(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	tok := joinToken(t, e, alice, laptop)

	// Qua T0 va dua jti sang hang cho FIFO, roi admit bang chinh script admit.
	jti, err := e.rdb.HGet(ctx, "wr:{"+e.ev+"}:tok", alice).Result()
	if err != nil {
		t.Fatal(err)
	}
	e.rdb.SRem(ctx, "wr:{"+e.ev+"}:lobby", jti)
	e.rdb.ZAdd(ctx, "wr:{"+e.ev+"}:queue", redis.Z{Score: 1, Member: jti})
	e.rdb.ZAdd(ctx, "wr:{"+e.ev+"}:admitted", redis.Z{Score: float64(time.Now().Add(15 * time.Minute).UnixMilli()), Member: jti})
	e.rdb.ZRem(ctx, "wr:{"+e.ev+"}:queue", jti)

	if code, st := state(t, e, alice, laptop, tok); code != http.StatusOK || st != "ADMITTED" {
		t.Fatalf("status = %d %s, muon ADMITTED", code, st)
	}
	// Dung jti tho (token cu) khong con duoc chap nhan.
	if code, _ := state(t, e, alice, laptop, jti); code != http.StatusUnauthorized {
		t.Fatalf("jti tho: %d, muon 401", code)
	}
}
