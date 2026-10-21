package mq

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Test viet tu spec §2 va plan §1/§7 (EVF-7), khong nhin code hien thuc.
//
// Hop dong ma test nay dua vao (implementer phai khop dung ten va chu ky):
//
//	type Exchange struct{ Name, Kind string; Durable bool }
//	type Queue struct{ Name string; Durable bool; Args map[string]any }
//	type Binding struct{ Source, Destination, RoutingKey string }
//	type Topology struct{ Exchanges []Exchange; Queues []Queue; Bindings []Binding }
//	func AITopology() Topology
//	func TicketingTopology() Topology
//	func NotificationTopology() Topology
//	func Merge(ts ...Topology) Topology
//	type Declarer interface { ExchangeDeclare(...); QueueDeclare(...); QueueBind(...) } // dung chu ky *amqp091.Channel
//	func Declare(ch Declarer, t Topology) error
//
// So trong Args so sanh sau khi chuan hoa ve int64 (plan: Args dung int64).

// --- tien ich chuan hoa ---

// asInt64 doi moi kieu so nguyen (va float64 khong co phan le, nhu so JSON) ve int64.
func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int8:
		return int64(n), true
	case int16:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case uint8:
		return int64(n), true
	case uint16:
		return int64(n), true
	case uint32:
		return int64(n), true
	case uint64:
		if n > math.MaxInt64 {
			return 0, false
		}
		return int64(n), true
	case float64:
		if n == math.Trunc(n) {
			return int64(n), true
		}
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	}
	return 0, false
}

// normArgs chuan hoa bang arguments: so -> int64, map rong/nil -> map rong.
func normArgs(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		if i, ok := asInt64(v); ok {
			out[k] = i
			continue
		}
		out[k] = v
	}
	return out
}

func findQueue(t *testing.T, top Topology, name string) Queue {
	t.Helper()
	for _, q := range top.Queues {
		if q.Name == name {
			return q
		}
	}
	t.Fatalf("khong thay queue %q trong topology", name)
	return Queue{}
}

func findExchange(t *testing.T, top Topology, name string) Exchange {
	t.Helper()
	for _, e := range top.Exchanges {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("khong thay exchange %q trong topology", name)
	return Exchange{}
}

func exchangeNames(top Topology) []string {
	var s []string
	for _, e := range top.Exchanges {
		s = append(s, e.Name)
	}
	sort.Strings(s)
	return s
}

func queueNames(top Topology) []string {
	var s []string
	for _, q := range top.Queues {
		s = append(s, q.Name)
	}
	sort.Strings(s)
	return s
}

func bindingKeys(top Topology) []string {
	var s []string
	for _, b := range top.Bindings {
		s = append(s, b.Source+" -> "+b.Destination+" ["+b.RoutingKey+"]")
	}
	sort.Strings(s)
	return s
}

func assertArgs(t *testing.T, queue string, got, want map[string]any) {
	t.Helper()
	g, w := normArgs(got), normArgs(want)
	if !reflect.DeepEqual(g, w) {
		t.Errorf("arguments cua %s:\n got  %v\n want %v", queue, g, w)
	}
}

// --- AITopology ---

var aiJobs = []struct {
	key      string
	workTTL  int64
	delayTTL int64
}{
	{"chat", 60000, 5000},
	{"moderation", 600000, 30000},
	{"anomaly", 1800000, 30000},
	{"report", 7200000, 120000},
}

func TestAITopology_Exchanges(t *testing.T) {
	top := AITopology()
	want := []string{"ai.direct", "ai.dlx", "ai.retry"}
	if got := exchangeNames(top); !reflect.DeepEqual(got, want) {
		t.Fatalf("exchanges = %v, muon %v", got, want)
	}
	for _, name := range want {
		e := findExchange(t, top, name)
		if e.Kind != "direct" || !e.Durable {
			t.Errorf("%s = %+v, muon direct + durable", name, e)
		}
	}
}

func TestAITopology_DanhSachQueue(t *testing.T) {
	top := AITopology()
	want := []string{
		"ai.anomaly.q", "ai.anomaly.retry",
		"ai.chat.q", "ai.chat.retry",
		"ai.dlq",
		"ai.moderation.q", "ai.moderation.retry",
		"ai.report.q", "ai.report.retry",
	}
	if got := queueNames(top); !reflect.DeepEqual(got, want) {
		t.Fatalf("queues = %v, muon %v", got, want)
	}
	for _, q := range top.Queues {
		if !q.Durable {
			t.Errorf("queue %s phai durable", q.Name)
		}
	}
}

// Hang goc: quorum + TTL cua no, KHONG co x-dead-letter-* (T6) va KHONG co x-max-priority (T5).
func TestAITopology_HangGocKhongDLXKhongPriority(t *testing.T) {
	top := AITopology()
	for _, j := range aiJobs {
		name := "ai." + j.key + ".q"
		q := findQueue(t, top, name)
		assertArgs(t, name, q.Args, map[string]any{
			"x-queue-type":  "quorum",
			"x-message-ttl": j.workTTL,
		})
		for k := range q.Args {
			if strings.HasPrefix(k, "x-dead-letter-") || k == "x-max-priority" {
				t.Errorf("%s khong duoc co arg %s", name, k)
			}
		}
	}
}

// Hang tre rieng cho tung cong viec, dead-letter ve ai.direct voi key cua chinh no (T1, T2),
// at-least-once + reject-publish (M3).
func TestAITopology_HangTreRiengTungCongViec(t *testing.T) {
	top := AITopology()
	for _, j := range aiJobs {
		name := "ai." + j.key + ".retry"
		q := findQueue(t, top, name)
		assertArgs(t, name, q.Args, map[string]any{
			"x-queue-type":              "quorum",
			"x-message-ttl":             j.delayTTL,
			"x-dead-letter-exchange":    "ai.direct",
			"x-dead-letter-routing-key": j.key,
			"x-dead-letter-strategy":    "at-least-once",
			"x-overflow":                "reject-publish",
		})
	}
}

func TestAITopology_DLQ(t *testing.T) {
	top := AITopology()
	q := findQueue(t, top, "ai.dlq")
	assertArgs(t, "ai.dlq", q.Args, map[string]any{"x-queue-type": "quorum"})
}

// Binding: ai.direct -> hang goc, ai.retry -> hang tre, ai.dlx -> ai.dlq bang TUNG key (T3, T4).
func TestAITopology_Bindings(t *testing.T) {
	top := AITopology()
	var want []string
	for _, j := range aiJobs {
		want = append(want,
			"ai.direct -> ai."+j.key+".q ["+j.key+"]",
			"ai.retry -> ai."+j.key+".retry ["+j.key+"]",
			"ai.dlx -> ai.dlq ["+j.key+"]",
		)
	}
	sort.Strings(want)
	if got := bindingKeys(top); !reflect.DeepEqual(got, want) {
		t.Fatalf("bindings:\n got  %v\n want %v", got, want)
	}
	for _, b := range top.Bindings {
		if b.RoutingKey == "#" || b.RoutingKey == "*" {
			t.Errorf("binding %+v dung wildcard tren exchange direct", b)
		}
	}
}

// Moi binding phai tro toi exchange/queue co trong cung topology (khong binding mo coi).
func TestAITopology_BindingKhongMoCoi(t *testing.T) {
	tops := map[string]Topology{
		"ai": AITopology(), "payment": PaymentTopology(), "ticketing": TicketingTopology(), "notification": NotificationTopology(),
	}
	// Exchange nguon co the do topology khac khai bao (vd ticketing nghe payment.events);
	// hang dich thi phai la cua chinh topology do.
	ex := map[string]bool{}
	for _, top := range tops {
		for _, e := range top.Exchanges {
			ex[e.Name] = true
		}
	}
	for name, top := range tops {
		qs := map[string]bool{}
		for _, q := range top.Queues {
			qs[q.Name] = true
		}
		for _, b := range top.Bindings {
			if !ex[b.Source] || !qs[b.Destination] {
				t.Errorf("%s: binding %+v tro toi doi tuong khong khai bao", name, b)
			}
		}
	}
}

// --- Ticketing / Notification / Payment ---

func hasBinding(top Topology, want Binding) bool {
	for _, b := range top.Bindings {
		if b == want {
			return true
		}
	}
	return false
}

func TestTicketingTopology_HoldReleasedGiuNguyen(t *testing.T) {
	top := TicketingTopology()
	assertArgs(t, "ticketing.hold.released.q", findQueue(t, top, "ticketing.hold.released.q").Args,
		map[string]any{"x-queue-type": "quorum"})
	if !hasBinding(top, Binding{Source: "ticketing.events", Destination: "ticketing.hold.released.q", RoutingKey: "ticketing.hold.released"}) {
		t.Errorf("mat binding hold.released: %+v", top.Bindings)
	}
}

func TestNotificationTopology_EmailGiuNguyen(t *testing.T) {
	top := NotificationTopology()
	assertArgs(t, "notification.email.q", findQueue(t, top, "notification.email.q").Args,
		map[string]any{"x-queue-type": "quorum"})
	if !hasBinding(top, Binding{Source: "notification.direct", Destination: "notification.email.q", RoutingKey: "email"}) {
		t.Errorf("mat binding email: %+v", top.Bindings)
	}
}

// Moi exchange su kien phai co sink "#": publisher bat mandatory, key chua co consumer ma bi
// tra ve thi relay outbox ket vinh vien o dong do.
func TestSink_MoiExchangeSuKienCoHangNhanMoiKey(t *testing.T) {
	for ex, top := range map[string]Topology{"payment.events": PaymentTopology(), "ticketing.events": TicketingTopology()} {
		q := findQueue(t, top, ex+".sink.q")
		assertArgs(t, q.Name, q.Args, map[string]any{"x-queue-type": "quorum", "x-max-length": int64(1000), "x-overflow": "drop-head"})
		if !hasBinding(top, Binding{Source: ex, Destination: q.Name, RoutingKey: "#"}) {
			t.Errorf("%s thieu binding #", q.Name)
		}
	}
}

// Hang tre dead-letter ve default exchange voi key = ten hang goc: message chi quay lai dung
// consumer da loi, khong lap sang consumer khac cung nghe exchange nguon.
func TestConsumer_RetryQuayVeDungHangGoc(t *testing.T) {
	cases := []struct {
		top                                 Topology
		work, retry, key, retryEx, dlq, dlx string
		ttl                                 int64
		sources                             []Binding
	}{
		{TicketingTopology(), TicketingOrderPaidQueue, "ticketing.order-paid.retry", TicketingOrderPaidKey,
			TicketingRetryExchange, TicketingDeadLetterQueue, TicketingDeadLetterEx, 5000,
			[]Binding{{Source: "payment.events", Destination: TicketingOrderPaidQueue, RoutingKey: "order.paid"}}},
		{NotificationTopology(), NotificationEventsQueue, "notification.events.retry", NotificationEventsKey,
			NotificationRetryExchange, NotificationDeadLetterQueue, NotificationDeadLetterEx, 10000,
			[]Binding{
				{Source: "payment.events", Destination: NotificationEventsQueue, RoutingKey: "order.paid"},
				{Source: "payment.events", Destination: NotificationEventsQueue, RoutingKey: "payment.refunded"},
				{Source: "ticketing.events", Destination: NotificationEventsQueue, RoutingKey: "ticketing.ticket.issued"},
			}},
	}
	for _, c := range cases {
		assertArgs(t, c.work, findQueue(t, c.top, c.work).Args, map[string]any{"x-queue-type": "quorum"})
		assertArgs(t, c.retry, findQueue(t, c.top, c.retry).Args, map[string]any{
			"x-queue-type":              "quorum",
			"x-message-ttl":             c.ttl,
			"x-dead-letter-exchange":    "",
			"x-dead-letter-routing-key": c.work,
			"x-dead-letter-strategy":    "at-least-once",
			"x-overflow":                "reject-publish",
		})
		findQueue(t, c.top, c.dlq)
		for _, want := range append(c.sources,
			Binding{Source: c.retryEx, Destination: c.retry, RoutingKey: c.key},
			Binding{Source: c.dlx, Destination: c.dlq, RoutingKey: c.key}) {
			if !hasBinding(c.top, want) {
				t.Errorf("thieu binding %+v", want)
			}
		}
	}
}

func TestPaymentTopology_Exchange(t *testing.T) {
	top := PaymentTopology()
	if len(top.Exchanges) != 1 || top.Exchanges[0] != (Exchange{Name: "payment.events", Kind: "topic", Durable: true}) {
		t.Errorf("exchanges = %+v", top.Exchanges)
	}
}

// --- Merge ---

func TestMerge_GopTheoThuTu(t *testing.T) {
	a := Topology{
		Exchanges: []Exchange{{Name: "a.x", Kind: "direct", Durable: true}},
		Queues:    []Queue{{Name: "a.q", Durable: true}},
		Bindings:  []Binding{{Source: "a.x", Destination: "a.q", RoutingKey: "a"}},
	}
	b := Topology{
		Exchanges: []Exchange{{Name: "b.x", Kind: "topic", Durable: true}},
		Queues:    []Queue{{Name: "b.q", Durable: true}},
		Bindings:  []Binding{{Source: "b.x", Destination: "b.q", RoutingKey: "b.#"}},
	}
	got := Merge(a, b)
	if len(got.Exchanges) != 2 || got.Exchanges[0].Name != "a.x" || got.Exchanges[1].Name != "b.x" {
		t.Errorf("exchanges = %+v", got.Exchanges)
	}
	if len(got.Queues) != 2 || got.Queues[0].Name != "a.q" || got.Queues[1].Name != "b.q" {
		t.Errorf("queues = %+v", got.Queues)
	}
	if len(got.Bindings) != 2 || got.Bindings[0] != a.Bindings[0] || got.Bindings[1] != b.Bindings[0] {
		t.Errorf("bindings = %+v", got.Bindings)
	}
	if z := Merge(); len(z.Exchanges)+len(z.Queues)+len(z.Bindings) != 0 {
		t.Errorf("Merge() rong = %+v", z)
	}
}

func TestMerge_TopologyThatDuSoLuong(t *testing.T) {
	ai, tk, nt := AITopology(), TicketingTopology(), NotificationTopology()
	all := Merge(ai, tk, nt)
	if n := len(ai.Exchanges) + len(tk.Exchanges) + len(nt.Exchanges); len(all.Exchanges) != n {
		t.Errorf("so exchange = %d, muon %d", len(all.Exchanges), n)
	}
	if n := len(ai.Queues) + len(tk.Queues) + len(nt.Queues); len(all.Queues) != n {
		t.Errorf("so queue = %d, muon %d", len(all.Queues), n)
	}
	if n := len(ai.Bindings) + len(tk.Bindings) + len(nt.Bindings); len(all.Bindings) != n {
		t.Errorf("so binding = %d, muon %d", len(all.Bindings), n)
	}
}

// --- Declare voi Declarer gia ---

type declCall struct {
	op         string // "exchange" | "queue" | "bind"
	name       string // exchange/queue duoc khai bao, hoac queue dich cua binding
	kind       string // exchange: kind; bind: exchange nguon
	key        string // bind: routing key
	durable    bool
	autoDelete bool
	internal   bool // exchange
	exclusive  bool // queue
	noWait     bool
	args       amqp.Table
}

type fakeDeclarer struct {
	calls  []declCall
	failOn string // ten doi tuong se tra loi
	failOp string // rong: moi thao tac cham failOn deu loi; "exchange" | "queue" | "bind": chi thao tac do
	err    error
}

func (f *fakeDeclarer) fails(op string, names ...string) bool {
	if f.failOp != "" && f.failOp != op {
		return false
	}
	for _, n := range names {
		if n == f.failOn {
			return true
		}
	}
	return false
}

func (f *fakeDeclarer) ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error {
	f.calls = append(f.calls, declCall{op: "exchange", name: name, kind: kind, durable: durable, autoDelete: autoDelete, internal: internal, noWait: noWait, args: args})
	if f.fails("exchange", name) {
		return f.err
	}
	return nil
}

func (f *fakeDeclarer) QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error) {
	f.calls = append(f.calls, declCall{op: "queue", name: name, durable: durable, autoDelete: autoDelete, exclusive: exclusive, noWait: noWait, args: args})
	if f.fails("queue", name) {
		return amqp.Queue{}, f.err
	}
	return amqp.Queue{Name: name}, nil
}

func (f *fakeDeclarer) QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error {
	f.calls = append(f.calls, declCall{op: "bind", name: name, kind: exchange, key: key, noWait: noWait, args: args})
	if f.fails("bind", name, exchange) {
		return f.err
	}
	return nil
}

// F5 (challenge): chi QueueDeclare loi, cac binding sau do (neu duoc goi) deu thanh cong ->
// Declare van phai tra loi va dung ngay o queue do.
func TestDeclare_LoiChiOQueueDeclareKhongBiNuot(t *testing.T) {
	sentinel := errors.New("PRECONDITION_FAILED")
	for _, target := range []string{"ai.chat.q", "ai.dlq", "ai.report.retry"} {
		t.Run(target, func(t *testing.T) {
			f := &fakeDeclarer{failOn: target, failOp: "queue", err: sentinel}
			err := Declare(f, AITopology())
			if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), target) {
				t.Fatalf("err = %v, muon boc %v va nhac %q", err, sentinel, target)
			}
			last := f.calls[len(f.calls)-1]
			if last.op != "queue" || last.name != target {
				t.Errorf("Declare khong dung o queue loi: lan goi cuoi %+v", last)
			}
		})
	}
}

// *amqp091.Channel phai thoa Declarer (chu ky chinh xac).
var _ Declarer = (*amqp.Channel)(nil)

func TestDeclare_ThuTuVaDoiSo(t *testing.T) {
	top := Merge(AITopology(), PaymentTopology(), TicketingTopology(), NotificationTopology())
	f := &fakeDeclarer{}
	if err := Declare(f, top); err != nil {
		t.Fatalf("Declare: %v", err)
	}
	n := len(top.Exchanges) + len(top.Queues) + len(top.Bindings)
	if len(f.calls) != n {
		t.Fatalf("so lan goi = %d, muon %d", len(f.calls), n)
	}
	// Thu tu: het exchange -> het queue -> het binding.
	rank := map[string]int{"exchange": 0, "queue": 1, "bind": 2}
	for i := 1; i < len(f.calls); i++ {
		if rank[f.calls[i].op] < rank[f.calls[i-1].op] {
			t.Fatalf("sai thu tu o lan goi %d: %s sau %s", i, f.calls[i].op, f.calls[i-1].op)
		}
	}
	ei, qi, bi := 0, len(top.Exchanges), len(top.Exchanges)+len(top.Queues)
	for i, e := range top.Exchanges {
		c := f.calls[ei+i]
		if c.op != "exchange" || c.name != e.Name || c.kind != e.Kind || c.durable != e.Durable ||
			c.autoDelete || c.internal || c.noWait {
			t.Errorf("exchange %s: goi %+v", e.Name, c)
		}
	}
	for i, q := range top.Queues {
		c := f.calls[qi+i]
		if c.op != "queue" || c.name != q.Name || c.durable != q.Durable || c.autoDelete || c.exclusive || c.noWait {
			t.Errorf("queue %s: goi %+v", q.Name, c)
		}
		assertArgs(t, q.Name, c.args, q.Args)
	}
	for i, b := range top.Bindings {
		c := f.calls[bi+i]
		if c.op != "bind" || c.name != b.Destination || c.kind != b.Source || c.key != b.RoutingKey || c.noWait {
			t.Errorf("binding %+v: goi %+v", b, c)
		}
	}
}

func TestDeclare_LoiBocTenDoiTuongVaDungLai(t *testing.T) {
	sentinel := errors.New("broker tu choi")
	top := AITopology()
	for _, target := range []string{"ai.retry", "ai.chat.retry", "ai.dlq"} {
		t.Run(target, func(t *testing.T) {
			f := &fakeDeclarer{failOn: target, err: sentinel}
			err := Declare(f, top)
			if !errors.Is(err, sentinel) {
				t.Fatalf("err = %v, muon boc %v", err, sentinel)
			}
			if !strings.Contains(err.Error(), target) {
				t.Errorf("err %q khong nhac ten %q", err, target)
			}
			last := f.calls[len(f.calls)-1]
			if last.name != target && last.kind != target {
				t.Errorf("Declare khong dung lai sau loi: lan goi cuoi %+v", last)
			}
		})
	}
}

func TestDeclare_GoiHaiLanKhongLoi(t *testing.T) {
	top := Merge(AITopology(), PaymentTopology(), TicketingTopology(), NotificationTopology())
	f := &fakeDeclarer{}
	if err := Declare(f, top); err != nil {
		t.Fatalf("lan 1: %v", err)
	}
	first := len(f.calls)
	if err := Declare(f, top); err != nil {
		t.Fatalf("lan 2: %v", err)
	}
	if len(f.calls) != 2*first {
		t.Fatalf("lan 2 goi %d lan, muon %d", len(f.calls)-first, first)
	}
	for i := 0; i < first; i++ {
		a, b := f.calls[i], f.calls[first+i]
		if a.op != b.op || a.name != b.name || a.kind != b.kind || a.key != b.key ||
			!reflect.DeepEqual(normArgs(a.args), normArgs(b.args)) {
			t.Errorf("lan goi %d khac nhau giua hai lan: %+v vs %+v", i, a, b)
		}
	}
}

// --- Dong bo voi deploy/rabbitmq/definitions.json (AC-5, code review m7) ---
//
// So KIEU va GIA TRI chinh xac (reflect.DeepEqual sau chuan hoa, khong qua %v): chuoi "5000" khac so 5000.
// Phan tu trung ten (exchange, queue) hoac trung (source, destination, key) o file hay o code deu la loi.
// Exchange so ca auto_delete/internal/arguments; binding so ca arguments (code luon false/rong).

// defsFile liet ke DU cac khoa duoc phep (DisallowUnknownFields: khoa la o goc hay phan tu la loi, F6).
type defsFile struct {
	Comment []string `json:"_comment"`
	Vhosts  []struct {
		Name string `json:"name"`
	} `json:"vhosts"`
	Users            []json.RawMessage `json:"users"`
	Permissions      []json.RawMessage `json:"permissions"`
	Policies         []json.RawMessage `json:"policies"`
	Parameters       []json.RawMessage `json:"parameters"`
	GlobalParameters []json.RawMessage `json:"global_parameters"`
	Exchanges        []struct {
		Name       string         `json:"name"`
		Vhost      string         `json:"vhost"`
		Type       string         `json:"type"`
		Durable    bool           `json:"durable"`
		AutoDelete bool           `json:"auto_delete"`
		Internal   bool           `json:"internal"`
		Arguments  map[string]any `json:"arguments"`
	} `json:"exchanges"`
	Queues []struct {
		Name       string         `json:"name"`
		Vhost      string         `json:"vhost"`
		Durable    bool           `json:"durable"`
		AutoDelete bool           `json:"auto_delete"`
		Arguments  map[string]any `json:"arguments"`
	} `json:"queues"`
	Bindings []struct {
		Source          string         `json:"source"`
		Vhost           string         `json:"vhost"`
		Destination     string         `json:"destination"`
		DestinationType string         `json:"destination_type"`
		RoutingKey      string         `json:"routing_key"`
		Arguments       map[string]any `json:"arguments"`
	} `json:"bindings"`
}

type exSpec struct {
	Kind       string
	Durable    bool
	AutoDelete bool
	Internal   bool
	Args       map[string]any
}

type qSpec struct {
	Durable    bool
	AutoDelete bool
	Args       map[string]any
}

// normFileValue: so JSON (json.Number) -> int64 neu la so nguyen, nguoc lai float64; kieu khac giu nguyen.
func normFileValue(v any) any {
	n, ok := v.(json.Number)
	if !ok {
		return v
	}
	if i, err := n.Int64(); err == nil {
		return i
	}
	f, _ := n.Float64()
	return f
}

// normCodeValue: moi kieu so nguyen Go -> int64; kieu khac (string, float, bool...) giu nguyen.
func normCodeValue(v any) any {
	switch v.(type) {
	case int, int8, int16, int32, int64, uint8, uint16, uint32, uint64:
		if i, ok := asInt64(v); ok {
			return i
		}
	}
	return v
}

func normMap(in map[string]any, f func(any) any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = f(v)
	}
	return out
}

// uniqueIndex gom phan tu theo khoa va ghi lai cac khoa bi trung.
type uniqueIndex[T any] struct {
	items map[string]T
	dups  []string
}

func (u *uniqueIndex[T]) add(key string, v T) {
	if u.items == nil {
		u.items = map[string]T{}
	}
	if _, ok := u.items[key]; ok {
		u.dups = append(u.dups, key)
		return
	}
	u.items[key] = v
}

func bindingKey(source, destination, key string) string {
	return source + " -> " + destination + " [" + key + "]"
}

func definitionsPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller that bai")
	}
	// libs/go/mq -> goc repo
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "deploy", "rabbitmq", "definitions.json")
}

func TestDefinitionsJSON_KhopTopologyCode(t *testing.T) {
	raw, err := os.ReadFile(definitionsPath(t))
	if err != nil {
		t.Fatalf("doc definitions.json: %v", err)
	}
	defs, err := parseDefinitions(raw)
	if err != nil {
		t.Fatalf("definitions.json: %v", err)
	}
	code := Merge(AITopology(), PaymentTopology(), TicketingTopology(), NotificationTopology())

	// Exchanges
	var fileEx, codeEx uniqueIndex[exSpec]
	for _, e := range defs.Exchanges {
		if e.Vhost != "/" {
			t.Errorf("exchange %s: vhost %q, muon \"/\"", e.Name, e.Vhost)
		}
		fileEx.add(e.Name, exSpec{Kind: e.Type, Durable: e.Durable, AutoDelete: e.AutoDelete, Internal: e.Internal,
			Args: normMap(e.Arguments, normFileValue)})
	}
	for _, e := range code.Exchanges {
		codeEx.add(e.Name, exSpec{Kind: e.Kind, Durable: e.Durable, Args: map[string]any{}})
	}
	compareIndex(t, "exchange", fileEx, codeEx)

	// Queues (ca arguments, dung kieu)
	var fileQ, codeQ uniqueIndex[qSpec]
	for _, q := range defs.Queues {
		if q.Vhost != "/" {
			t.Errorf("queue %s: vhost %q, muon \"/\"", q.Name, q.Vhost)
		}
		fileQ.add(q.Name, qSpec{Durable: q.Durable, AutoDelete: q.AutoDelete, Args: normMap(q.Arguments, normFileValue)})
	}
	for _, q := range code.Queues {
		codeQ.add(q.Name, qSpec{Durable: q.Durable, Args: normMap(q.Args, normCodeValue)})
	}
	compareIndex(t, "queue", fileQ, codeQ)

	// Bindings
	var fileB, codeB uniqueIndex[map[string]any]
	for _, b := range defs.Bindings {
		if b.DestinationType != "queue" || b.Vhost != "/" {
			t.Errorf("binding %s -> %s: destination_type %q vhost %q", b.Source, b.Destination, b.DestinationType, b.Vhost)
		}
		fileB.add(bindingKey(b.Source, b.Destination, b.RoutingKey), normMap(b.Arguments, normFileValue))
	}
	for _, b := range code.Bindings {
		codeB.add(bindingKey(b.Source, b.Destination, b.RoutingKey), map[string]any{})
	}
	compareIndex(t, "binding", fileB, codeB)
}

// compareIndex bao loi cho phan tu trung, thieu, thua hoac lech (so bang reflect.DeepEqual, in %#v).
func compareIndex[T any](t *testing.T, what string, file, code uniqueIndex[T]) {
	t.Helper()
	for _, k := range file.dups {
		t.Errorf("%s %s bi khai bao trung trong definitions.json", what, k)
	}
	for _, k := range code.dups {
		t.Errorf("%s %s bi khai bao trung trong code", what, k)
	}
	keys := map[string]bool{}
	for k := range file.items {
		keys[k] = true
	}
	for k := range code.items {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		f, inFile := file.items[k]
		c, inCode := code.items[k]
		switch {
		case !inFile:
			t.Errorf("%s %s co trong code nhung thieu trong definitions.json", what, k)
		case !inCode:
			t.Errorf("%s %s co trong definitions.json nhung khong co trong code", what, k)
		case !reflect.DeepEqual(f, c):
			t.Errorf("%s %s lech:\n file %#v\n code %#v", what, k, f, c)
		}
	}
}

// Bao dam bo so sanh tren khong de lot loi kieu (m7): chuoi "5000" khac so 5000, 5000.5 khac 5000.
func TestDefinitionsJSON_ChuanHoaPhanBietKieu(t *testing.T) {
	code := normMap(map[string]any{"x-message-ttl": int64(5000), "x-queue-type": "quorum"}, normCodeValue)
	same := normMap(map[string]any{"x-message-ttl": json.Number("5000"), "x-queue-type": "quorum"}, normFileValue)
	if !reflect.DeepEqual(code, same) {
		t.Fatalf("so JSON 5000 phai khop int64 5000: %#v vs %#v", same, code)
	}
	for name, file := range map[string]map[string]any{
		"chuoi so":     {"x-message-ttl": "5000", "x-queue-type": "quorum"},
		"so thap phan": {"x-message-ttl": json.Number("5000.5"), "x-queue-type": "quorum"},
		"bool":         {"x-message-ttl": json.Number("5000"), "x-queue-type": true},
	} {
		if reflect.DeepEqual(code, normMap(file, normFileValue)) {
			t.Errorf("%s: phai bi coi la lech", name)
		}
	}
	// So thuc trong code khong duoc coi la khop so nguyen trong file.
	if reflect.DeepEqual(normMap(map[string]any{"a": 5000.0}, normCodeValue),
		normMap(map[string]any{"a": json.Number("5000")}, normFileValue)) {
		t.Error("float64 trong code phai bi coi la lech voi so nguyen trong file")
	}
}

func TestUniqueIndex_BaoTrung(t *testing.T) {
	var u uniqueIndex[int]
	u.add("ai.chat.q", 1)
	u.add("ai.dlq", 2)
	u.add("ai.chat.q", 3)
	if len(u.dups) != 1 || u.dups[0] != "ai.chat.q" {
		t.Fatalf("dups = %v, muon [ai.chat.q]", u.dups)
	}
	if u.items["ai.chat.q"] != 1 {
		t.Errorf("phan tu dau tien phai duoc giu: %v", u.items)
	}
}

// parseDefinitions doc definitions.json mot cach chat (F6): khoa JSON trung, khoa la, va policy/parameter
// (co the gan lai DLX/delivery-limit ngoai code) deu la loi.
func parseDefinitions(raw []byte) (defsFile, error) {
	var defs defsFile
	if dups, err := findDuplicateKeys(raw); err != nil {
		return defs, fmt.Errorf("quet token: %w", err)
	} else if len(dups) > 0 {
		return defs, fmt.Errorf("khoa JSON trung: %v", dups)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	dec.DisallowUnknownFields()
	if err := dec.Decode(&defs); err != nil {
		return defs, fmt.Errorf("parse: %w", err)
	}
	if len(defs.Policies)+len(defs.Parameters)+len(defs.GlobalParameters) > 0 {
		return defs, fmt.Errorf("policies/parameters/global_parameters phai rong (topology chi do code quyet dinh): %d/%d/%d",
			len(defs.Policies), len(defs.Parameters), len(defs.GlobalParameters))
	}
	return defs, nil
}

// findDuplicateKeys duyet JSON theo token va tra ve duong dan cac khoa bi lap trong cung mot object.
func findDuplicateKeys(raw []byte) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var dups []string
	var walk func(path string) error
	walk = func(path string) error {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tok {
		case json.Delim('{'):
			seen := map[string]bool{}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return err
				}
				k, _ := kt.(string)
				if seen[k] {
					dups = append(dups, path+"."+k)
				}
				seen[k] = true
				if err := walk(path + "." + k); err != nil {
					return err
				}
			}
			_, err = dec.Token() // '}'
			return err
		case json.Delim('['):
			for i := 0; dec.More(); i++ {
				if err := walk(fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
			_, err = dec.Token() // ']'
			return err
		}
		return nil
	}
	if err := walk("$"); err != nil {
		return nil, err
	}
	return dups, nil
}

// F6: bo doc chat bat duoc khoa trung, khoa la, va policy.
func TestParseDefinitions_TuChoiFileSai(t *testing.T) {
	base := `{"vhosts":[{"name":"/"}],"exchanges":[],"bindings":[],"policies":[],"parameters":[],
"queues":[{"name":"q","vhost":"/","durable":true,"arguments":{"x-queue-type":"quorum"%s}%s}]%s}`
	cases := map[string]struct {
		json string
		want string // rong: hop le; khac rong: loi phai chua chuoi nay
	}{
		"hop le":           {fmt.Sprintf(base, "", "", ""), ""},
		"khoa trung":       {fmt.Sprintf(base, `,"x-message-ttl":1,"x-message-ttl":60000`, "", ""), "khoa JSON trung"},
		"khoa la o queue":  {fmt.Sprintf(base, "", `,"type":"classic"`, ""), "unknown field"},
		"khoa la o goc":    {fmt.Sprintf(base, "", "", `,"rabbit_version":"3.13"`), "unknown field"},
		"co policy":        {strings.Replace(fmt.Sprintf(base, "", "", ""), `"policies":[]`, `"policies":[{"name":"p","pattern":"^ai\\.chat\\.q$","definition":{"dead-letter-exchange":"ai.retry"}}]`, 1), "policies"},
		"co parameter":     {strings.Replace(fmt.Sprintf(base, "", "", ""), `"parameters":[]`, `"parameters":[{"name":"x"}]`, 1), "policies"},
		"global parameter": {fmt.Sprintf(base, "", "", `,"global_parameters":[{"name":"cluster_name"}]`), "policies"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parseDefinitions([]byte(c.json))
			switch {
			case c.want == "" && err != nil:
				t.Fatalf("err = %v, muon hop le\n%s", err, c.json)
			case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
				t.Fatalf("err = %v, muon loi chua %q\n%s", err, c.want, c.json)
			}
		})
	}
}

func TestFindDuplicateKeys_LongNhau(t *testing.T) {
	dups, err := findDuplicateKeys([]byte(`{"a":1,"b":{"c":1,"c":2},"d":[{"e":1},{"e":1,"e":2}],"a":3}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"$.b.c", "$.d[1].e", "$.a"}
	if !reflect.DeepEqual(dups, want) {
		t.Fatalf("dups = %v, muon %v", dups, want)
	}
}
