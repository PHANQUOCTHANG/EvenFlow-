package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"
)

// fakeRunner ghi so lan bat va su kien nao dang chay; chay toi khi ctx huy, hoac thoat ngay neu
// failFast[id] (gia lap runEvent loi).
type fakeRunner struct {
	mu       sync.Mutex
	starts   map[string]int
	live     map[string]bool
	failFast map[string]bool
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{starts: map[string]int{}, live: map[string]bool{}, failFast: map[string]bool{}}
}

func (f *fakeRunner) run(ctx context.Context, id string) {
	f.mu.Lock()
	f.starts[id]++
	fail := f.failFast[id]
	if !fail {
		f.live[id] = true
	}
	f.mu.Unlock()
	if fail {
		return
	}
	<-ctx.Done()
	f.mu.Lock()
	f.live[id] = false
	f.mu.Unlock()
}

func (f *fakeRunner) snapshot() (map[string]int, map[string]bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, l := map[string]int{}, map[string]bool{}
	for k, v := range f.starts {
		s[k] = v
	}
	for k, v := range f.live {
		l[k] = v
	}
	return s, l
}

type listSrc struct {
	mu  sync.Mutex
	ids []string
	err error
}

func (l *listSrc) set(ids []string, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ids, l.err = ids, err
}

func (l *listSrc) list(context.Context) ([]string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.ids), l.err
}

func newSup(src *listSrc, r *fakeRunner) *EventSupervisor {
	return NewEventSupervisor(src.list, r.run, time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("het gio cho: %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestSupervisor_ThemVaBotSuKienTheoDanhSach(t *testing.T) {
	src := &listSrc{ids: []string{"a", "b"}}
	r := newFakeRunner()
	s := newSup(src, r)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Sync(ctx)
	waitFor(t, "a, b chay", func() bool { _, l := r.snapshot(); return l["a"] && l["b"] })

	src.set([]string{"b", "c"}, nil)
	s.Sync(ctx)
	waitFor(t, "a dung, c chay", func() bool { _, l := r.snapshot(); return !l["a"] && l["b"] && l["c"] })

	starts, _ := r.snapshot()
	if starts["b"] != 1 {
		t.Errorf("b bi bat lai %d lan; su kien van trong danh sach phai giu nguyen goroutine (giu leader)", starts["b"])
	}
	if got := s.Running(); !slices.Equal(got, []string{"b", "c"}) {
		t.Errorf("Running = %v", got)
	}
}

// Redis loi: khong dung su kien nao (phong cho van tha nguoi).
func TestSupervisor_DocDanhSachLoiGiuNguyen(t *testing.T) {
	src := &listSrc{ids: []string{"a"}}
	r := newFakeRunner()
	s := newSup(src, r)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Sync(ctx)
	waitFor(t, "a chay", func() bool { _, l := r.snapshot(); return l["a"] })

	src.set(nil, errors.New("redis timeout"))
	s.Sync(ctx)
	time.Sleep(20 * time.Millisecond)
	if _, l := r.snapshot(); !l["a"] {
		t.Fatal("loi doc danh sach lam dung su kien dang chay")
	}
}

// runEvent thoat vi loi: vong dong bo sau chay lai.
func TestSupervisor_GoroutineThoatThiChayLai(t *testing.T) {
	src := &listSrc{ids: []string{"x"}}
	r := newFakeRunner()
	r.failFast["x"] = true
	s := newSup(src, r)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Sync(ctx)
	waitFor(t, "lan 1", func() bool { st, _ := r.snapshot(); return st["x"] == 1 })
	waitFor(t, "x thoat", func() bool { return len(s.Running()) == 0 })
	r.mu.Lock()
	r.failFast["x"] = false
	r.mu.Unlock()
	s.Sync(ctx)
	waitFor(t, "x chay lai", func() bool { _, l := r.snapshot(); return l["x"] })
	if st, _ := r.snapshot(); st["x"] != 2 {
		t.Errorf("x bat %d lan, muon 2", st["x"])
	}
}

// Tat controller: Run huy moi su kien va cho chung thoat truoc khi tra ve.
func TestSupervisor_RunDungThiDungHet(t *testing.T) {
	src := &listSrc{ids: []string{"a", "b"}}
	r := newFakeRunner()
	s := newSup(src, r)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	waitFor(t, "a, b chay", func() bool { _, l := r.snapshot(); return l["a"] && l["b"] })

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v", err)
	}
	if _, l := r.snapshot(); l["a"] || l["b"] {
		t.Fatalf("Run tra ve khi su kien con chay: %v", l)
	}
}
