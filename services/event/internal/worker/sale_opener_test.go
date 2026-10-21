package worker_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/eventflow/eventflow/services/event/internal/worker"
)

type fakeOpener struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeOpener) OpenDueSales(context.Context, time.Time) ([]uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return []uuid.UUID{uuid.New()}, f.err
}

func (f *fakeOpener) n() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// Quet ngay khi khoi dong (su kien den gio luc service dang tat duoc mo ngay), roi theo chu ky;
// loi DB khong lam dung vong lap.
func TestSaleOpener_QuetNgayVaTheoChuKyKeCaKhiLoi(t *testing.T) {
	repo := &fakeOpener{err: errors.New("db mat ket noi")}
	o := worker.NewSaleOpener(repo, 5*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- o.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for repo.n() < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v", err)
	}
	if repo.n() < 3 {
		t.Fatalf("chi quet %d lan", repo.n())
	}
}
