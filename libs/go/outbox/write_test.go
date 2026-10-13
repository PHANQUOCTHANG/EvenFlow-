package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Unit test cho Write (spec AC-1, AC-2, AC-5; plan §1.2, §6.4) voi Execer gia.
//
// TEN EXPORT ma test gia dinh:
//
//	type Execer interface {
//		Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
//	}
//	type Event struct {
//		AggregateType string
//		AggregateID   string // UUID
//		RoutingKey    string // ^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*){2,}$
//		Data          any    // marshal JSON -> cot payload (CHI data), <= 64 KiB
//	}
//	const SchemaVersion = 1
//	func Write(ctx context.Context, tx Execer, e Event) (eventID string, err error)
//	type Envelope struct {
//		EventID       string          `json:"event_id"`
//		OccurredAt    time.Time       `json:"occurred_at"`
//		TraceID       string          `json:"trace_id,omitempty"`
//		SchemaVersion int             `json:"schema_version"`
//		Type          string          `json:"type"`
//		Data          json.RawMessage `json:"data"`
//	}
//
// Hop dong SQL cua Write: dung MOT cau `INSERT INTO outbox_events (<cot>, ...) VALUES ($1, $2::uuid, ...)`,
// moi gia tri la mot placeholder (co the kem ::cast). Cot bat buoc: event_id (UUID v4 sinh o Go, trung voi
// gia tri tra ve), aggregate_type, aggregate_id, routing_key, payload (JSON cua Data — []byte/string/
// json.RawMessage), headers (map[string]string hoac JSON cua no; chi khoa trace W3C), trace_id (hex trace ID
// khi ctx co span; NULL/rong khi khong co).
//   Vi pham kiem tra dau vao -> tra loi va KHONG goi Exec.

type execCall struct {
	sql  string
	args []any
}

type fakeExecer struct {
	calls []execCall
	err   error
}

func (f *fakeExecer) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.calls = append(f.calls, execCall{sql, args})
	if f.err != nil {
		return pgconn.CommandTag{}, f.err
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

var _ Execer = pgx.Tx(nil) // pgx.Tx phai thoa Execer

var insertRe = regexp.MustCompile(`(?is)^\s*insert\s+into\s+outbox_events\s*\(([^)]*)\)\s*values\s*\((.*)\)\s*;?\s*$`)
var placeholderRe = regexp.MustCompile(`^\$(\d+)(::[a-z_]+)?$`)

// insertCols tach cau INSERT thanh map cot -> tham so.
func insertCols(t *testing.T, c execCall) map[string]any {
	t.Helper()
	m := insertRe.FindStringSubmatch(c.sql)
	if m == nil {
		t.Fatalf("Write phai dung INSERT INTO outbox_events (...) VALUES (...): %s", c.sql)
	}
	cols := strings.Split(m[1], ",")
	vals := splitTop(m[2])
	if len(cols) != len(vals) {
		t.Fatalf("so cot %d khac so gia tri %d: %s", len(cols), len(vals), c.sql)
	}
	out := map[string]any{}
	for i, col := range cols {
		col = strings.ToLower(strings.TrimSpace(col))
		v := strings.ToLower(strings.TrimSpace(vals[i]))
		pm := placeholderRe.FindStringSubmatch(v)
		if pm == nil {
			t.Fatalf("cot %s phai la tham so ($n), nhan %q", col, v)
		}
		n, _ := strconv.Atoi(pm[1])
		if n < 1 || n > len(c.args) {
			t.Fatalf("cot %s tham chieu $%d, chi co %d tham so", col, n, len(c.args))
		}
		out[col] = c.args[n-1]
	}
	return out
}

func splitTop(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// asText dua gia tri tham so ve chuoi (chap nhan cac kieu pgx ma hoa duoc).
func asText(v any) (string, bool) {
	switch x := v.(type) {
	case nil:
		return "", false
	case string:
		return x, true
	case *string:
		if x == nil {
			return "", false
		}
		return *x, true
	case []byte:
		return string(x), true
	case json.RawMessage:
		return string(x), true
	case [16]byte:
		return fmt.Sprintf("%x-%x-%x-%x-%x", x[0:4], x[4:6], x[6:8], x[8:10], x[10:16]), true
	case pgtype.UUID:
		if !x.Valid {
			return "", false
		}
		b := x.Bytes
		return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), true
	case pgtype.Text:
		return x.String, x.Valid
	case fmt.Stringer:
		return x.String(), true
	}
	return fmt.Sprint(v), true
}

func asJSON(t *testing.T, v any) []byte {
	t.Helper()
	switch x := v.(type) {
	case []byte:
		return x
	case json.RawMessage:
		return x
	case string:
		return []byte(x)
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("tham so khong ma hoa JSON duoc (%T): %v", v, err)
	}
	return b
}

var uuidV4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type duLieuHold struct {
	OrderID     string `json:"order_id"`
	SaleEventID string `json:"sale_event_id"`
	Quantity    int    `json:"quantity"`
}

func suKienMau() Event {
	return Event{
		AggregateType: "order",
		AggregateID:   "5f0c3a52-8a8e-4c1e-9f57-3d1b6f2a9c10",
		RoutingKey:    "ticketing.hold.created",
		Data:          duLieuHold{OrderID: "5f0c3a52-8a8e-4c1e-9f57-3d1b6f2a9c10", SaleEventID: "7d1e2c3b-0000-4000-8000-000000000001", Quantity: 2},
	}
}

func TestWrite_GhiMotRowVoiEventIDVaPayloadChiLaData(t *testing.T) {
	ex := &fakeExecer{}
	id, err := Write(context.Background(), ex, suKienMau())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !uuidV4Re.MatchString(id) {
		t.Errorf("event_id %q khong phai UUID v4 dang chuan", id)
	}
	if len(ex.calls) != 1 {
		t.Fatalf("Exec goi %d lan, muon 1", len(ex.calls))
	}
	cols := insertCols(t, ex.calls[0])
	for _, c := range []string{"event_id", "aggregate_type", "aggregate_id", "routing_key", "payload", "headers"} {
		if _, ok := cols[c]; !ok {
			t.Errorf("INSERT thieu cot %s: %s", c, ex.calls[0].sql)
		}
	}
	if got, _ := asText(cols["event_id"]); strings.ToLower(got) != id {
		t.Errorf("cot event_id = %q, khac gia tri tra ve %q", got, id)
	}
	if got, _ := asText(cols["aggregate_type"]); got != "order" {
		t.Errorf("aggregate_type = %q", got)
	}
	if got, _ := asText(cols["aggregate_id"]); got != suKienMau().AggregateID {
		t.Errorf("aggregate_id = %q", got)
	}
	if got, _ := asText(cols["routing_key"]); got != "ticketing.hold.created" {
		t.Errorf("routing_key = %q", got)
	}
	payload := decodeJSON(t, asJSON(t, cols["payload"]))
	want := map[string]any{"order_id": "5f0c3a52-8a8e-4c1e-9f57-3d1b6f2a9c10",
		"sale_event_id": "7d1e2c3b-0000-4000-8000-000000000001", "quantity": float64(2)}
	if len(payload) != len(want) {
		t.Errorf("payload = %v, muon CHI data %v (phong bi dung luc publish)", payload, want)
	}
	for k, v := range want {
		if payload[k] != v {
			t.Errorf("payload[%s] = %v, muon %v", k, payload[k], v)
		}
	}
	if tid, ok := cols["trace_id"]; ok {
		if s, valid := asText(tid); valid && s != "" {
			t.Errorf("ctx khong co span nhung trace_id = %q", s)
		}
	}
	h := headersArg(t, cols["headers"])
	if _, ok := h["traceparent"]; ok {
		t.Errorf("ctx khong co span nhung headers co traceparent: %v", h)
	}

	id2, err := Write(context.Background(), ex, suKienMau())
	if err != nil {
		t.Fatalf("Write lan 2: %v", err)
	}
	if id2 == id {
		t.Errorf("hai lan Write cung event_id %s", id)
	}
}

func headersArg(t *testing.T, v any) map[string]string {
	t.Helper()
	if m, ok := v.(map[string]string); ok {
		return m
	}
	var m map[string]string
	if err := json.Unmarshal(asJSON(t, v), &m); err != nil {
		t.Fatalf("headers phai la map[string]string (moi gia tri la chuoi): %v", err)
	}
	return m
}

// AC-5: Write luu trace context cua request vao headers + trace_id.
func TestWrite_LuuTraceparentVaTraceIDKhiCoSpan(t *testing.T) {
	newRecorder(t)
	ctx, span := otel.Tracer("test").Start(context.Background(), "tao-hold")
	defer span.End()
	want := span.SpanContext().TraceID().String()

	ex := &fakeExecer{}
	if _, err := Write(ctx, ex, suKienMau()); err != nil {
		t.Fatalf("Write: %v", err)
	}
	cols := insertCols(t, ex.calls[0])
	h := headersArg(t, cols["headers"])
	tp, ok := h["traceparent"]
	if !ok || !strings.Contains(tp, want) {
		t.Errorf("headers = %v, muon traceparent chua trace %s", h, want)
	}
	for k := range h {
		switch k {
		case "traceparent", "tracestate", "baggage":
		default:
			t.Errorf("headers chi duoc chua khoa trace W3C, co %q", k)
		}
	}
	tid, ok := cols["trace_id"]
	if !ok {
		t.Fatalf("INSERT thieu cot trace_id: %s", ex.calls[0].sql)
	}
	if s, _ := asText(tid); s != want {
		t.Errorf("trace_id = %q, muon %q", s, want)
	}
}

func TestWrite_KiemTraDauVao(t *testing.T) {
	lon := strings.Repeat("a", 64*1024) // JSON chuoi = 65538 byte > 64 KiB
	cases := []struct {
		ten string
		sua func(*Event)
		loi bool
	}{
		{"hop le", func(*Event) {}, false},
		{"key 3 doan khac", func(e *Event) { e.RoutingKey = "ticketing.hold.released" }, false},
		{"key nhieu doan, so, gach duoi", func(e *Event) { e.RoutingKey = "x1.y_2.z.w" }, false},
		{"key rong", func(e *Event) { e.RoutingKey = "" }, true},
		{"key 2 doan", func(e *Event) { e.RoutingKey = "ticketing.hold" }, true},
		{"key chu hoa", func(e *Event) { e.RoutingKey = "Ticketing.hold.created" }, true},
		{"key doan rong", func(e *Event) { e.RoutingKey = "ticketing..created" }, true},
		{"key cham cuoi", func(e *Event) { e.RoutingKey = "ticketing.hold.created." }, true},
		{"key bat dau bang so", func(e *Event) { e.RoutingKey = "1ticketing.hold.created" }, true},
		{"key co gach ngang", func(e *Event) { e.RoutingKey = "ticketing.hold.cre-ated" }, true},
		{"key wildcard", func(e *Event) { e.RoutingKey = "ticketing.hold.*" }, true},
		{"key xuong dong", func(e *Event) { e.RoutingKey = "ticketing.hold.created\n" }, true},
		{"aggregate rong", func(e *Event) { e.AggregateID = "" }, true},
		{"aggregate type rong (W6)", func(e *Event) { e.AggregateType = "" }, true},
		// W1: bien dung 64 KiB = 65536 byte JSON (chuoi n ky tu -> n+2 byte).
		{"data dung 65536 byte duoc", func(e *Event) { e.Data = strings.Repeat("a", 65534) }, false},
		{"data 65537 byte", func(e *Event) { e.Data = strings.Repeat("a", 65535) }, true},
		// F2: jsonb cua PostgreSQL khong nhan \u0000.
		{"data chua NUL (F2)", func(e *Event) { e.Data = map[string]string{"x": "a\x00b"} }, true},
		{"data RawMessage chua \\u0000 (F2)", func(e *Event) { e.Data = json.RawMessage(`{"x":"a\u0000b"}`) }, true},
		{"aggregate khong phai UUID", func(e *Event) { e.AggregateID = "h1" }, true},
		{"aggregate UUID cat cut", func(e *Event) { e.AggregateID = "5f0c3a52-8a8e-4c1e-9f57" }, true},
		{"data qua 64 KiB", func(e *Event) { e.Data = map[string]string{"x": lon} }, true},
		{"data duoi 64 KiB", func(e *Event) { e.Data = map[string]string{"x": lon[:60000]} }, false},
		{"data khong marshal duoc", func(e *Event) { e.Data = map[string]any{"c": make(chan int)} }, true},
	}
	for _, tc := range cases {
		t.Run(tc.ten, func(t *testing.T) {
			e := suKienMau()
			tc.sua(&e)
			ex := &fakeExecer{}
			_, err := Write(context.Background(), ex, e)
			if tc.loi {
				if err == nil {
					t.Fatal("muon loi kiem tra dau vao")
				}
				if len(ex.calls) != 0 {
					t.Errorf("dau vao sai van goi Exec: %v", ex.calls)
				}
				return
			}
			if err != nil {
				t.Fatalf("muon hop le, nhan %v", err)
			}
		})
	}
}

func TestWrite_TraLoiCuaExec(t *testing.T) {
	ex := &fakeExecer{err: errTest}
	if _, err := Write(context.Background(), ex, suKienMau()); !errors.Is(err, errTest) {
		t.Fatalf("Write = %v, muon boc loi cua Exec", err)
	}
}

// Envelope ma hoa dung ten truong cua spec AC-2; trace_id rong thi bo.
func TestEnvelope_TenTruongJSON(t *testing.T) {
	env := Envelope{EventID: "e", OccurredAt: baseTime, SchemaVersion: SchemaVersion, Type: "a.b.c",
		Data: json.RawMessage(`{"k":1}`)}
	m := decodeJSON(t, mustJSON(t, env))
	for _, k := range []string{"event_id", "occurred_at", "schema_version", "type", "data"} {
		if _, ok := m[k]; !ok {
			t.Errorf("thieu khoa %s: %v", k, m)
		}
	}
	if _, ok := m["trace_id"]; ok {
		t.Errorf("trace_id rong phai bi bo (omitempty): %v", m)
	}
	env.TraceID = "abc"
	if m := decodeJSON(t, mustJSON(t, env)); m["trace_id"] != "abc" {
		t.Errorf("trace_id = %v", m["trace_id"])
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// W7: headers chi giu khoa trace W3C (traceparent, tracestate, baggage) — giu du ca ba, bo khoa khac.
func TestWrite_HeadersGiuDuBaKhoaW3CVaBoKhoaKhac(t *testing.T) {
	newRecorder(t)
	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}, khoaLaPropagator{}))
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })

	ts, err := trace.ParseTraceState("vendor=abc")
	if err != nil {
		t.Fatal(err)
	}
	tid, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	sid, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: tid, SpanID: sid,
		TraceFlags: trace.FlagsSampled, TraceState: ts, Remote: true})
	ctx := trace.ContextWithRemoteSpanContext(context.Background(), sc)
	m, err := baggage.NewMember("tenant", "evt1")
	if err != nil {
		t.Fatal(err)
	}
	bg, err := baggage.New(m)
	if err != nil {
		t.Fatal(err)
	}
	ctx = baggage.ContextWithBaggage(ctx, bg)

	ex := &fakeExecer{}
	if _, err := Write(ctx, ex, suKienMau()); err != nil {
		t.Fatalf("Write: %v", err)
	}
	h := headersArg(t, insertCols(t, ex.calls[0])["headers"])
	for _, k := range []string{"traceparent", "tracestate", "baggage"} {
		if h[k] == "" {
			t.Errorf("headers thieu %s: %v", k, h)
		}
	}
	if _, ok := h["x-la"]; ok {
		t.Errorf("headers chua khoa ngoai danh sach W3C: %v", h)
	}
}

// khoaLaPropagator ghi them mot khoa khong phai W3C (mo phong propagator khac duoc cai toan cuc).
type khoaLaPropagator struct{}

func (khoaLaPropagator) Inject(_ context.Context, c propagation.TextMapCarrier) { c.Set("x-la", "1") }
func (khoaLaPropagator) Extract(ctx context.Context, _ propagation.TextMapCarrier) context.Context {
	return ctx
}
func (khoaLaPropagator) Fields() []string { return []string{"x-la"} }
