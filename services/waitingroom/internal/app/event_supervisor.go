package app

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"
)

// EventLister tra danh sach su kien can dieu khien o thoi diem goi.
type EventLister func(ctx context.Context) ([]string, error)

// EventRunner dieu khien mot su kien toi khi ctx bi huy (tranh leader, chay vong admit). Tra ve som
// (loi) thi supervisor chay lai o vong dong bo sau neu su kien van con trong danh sach.
type EventRunner func(ctx context.Context, eventID string)

// EventSupervisor giu dung mot goroutine EventRunner cho moi su kien trong danh sach (EV-118):
// su kien moi duoc bat trong toi da mot chu ky, su kien roi danh sach thi bi huy (leader nha khoa).
//
// Doc danh sach loi thi GIU nguyen cac su kien dang chay: mot lan Redis cham khong duoc lam phong
// cho ngung tha nguoi.
type EventSupervisor struct {
	list     EventLister
	run      EventRunner
	interval time.Duration
	log      *slog.Logger

	mu      sync.Mutex
	running map[string]*supervised
	wg      sync.WaitGroup
}

type supervised struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// NewEventSupervisor tao supervisor.
func NewEventSupervisor(list EventLister, run EventRunner, interval time.Duration, log *slog.Logger) *EventSupervisor {
	return &EventSupervisor{list: list, run: run, interval: interval, log: log, running: map[string]*supervised{}}
}

// Run dong bo ngay roi theo chu ky; ctx bi huy thi dung moi su kien va cho chung thoat.
func (s *EventSupervisor) Run(ctx context.Context) error {
	defer s.stopAll()
	s.Sync(ctx)
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			s.Sync(ctx)
		}
	}
}

// Sync dua tap goroutine ve dung danh sach hien tai.
func (s *EventSupervisor) Sync(ctx context.Context) {
	ids, err := s.list(ctx)
	if err != nil {
		s.log.Error("doc danh sach su kien that bai, giu cac su kien dang dieu khien", "err", err)
		return
	}
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// Don goroutine da tu thoat (loi) de vong nay chay lai neu su kien van can.
	for id, r := range s.running {
		select {
		case <-r.done:
			delete(s.running, id)
		default:
		}
	}
	for id, r := range s.running {
		if !want[id] {
			s.log.Info("su kien roi danh sach, dung dieu khien", "event", id)
			r.cancel()
			delete(s.running, id)
		}
	}
	for _, id := range ids {
		if _, ok := s.running[id]; ok || ctx.Err() != nil {
			continue
		}
		evCtx, cancel := context.WithCancel(ctx)
		r := &supervised{cancel: cancel, done: make(chan struct{})}
		s.running[id] = r
		s.wg.Add(1)
		go func(id string) {
			defer s.wg.Done()
			defer close(r.done)
			s.run(evCtx, id)
		}(id)
		s.log.Info("bat dau dieu khien su kien", "event", id)
	}
}

// Running tra cac su kien dang co goroutine, da sap xep (cho test va log).
func (s *EventSupervisor) Running() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.running))
	for id, r := range s.running {
		select {
		case <-r.done:
		default:
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

func (s *EventSupervisor) stopAll() {
	s.mu.Lock()
	for id, r := range s.running {
		r.cancel()
		delete(s.running, id)
	}
	s.mu.Unlock()
	s.wg.Wait()
}
