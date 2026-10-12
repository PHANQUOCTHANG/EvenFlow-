package app_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// Metric phai phan anh dung ket qua nghiep vu: alert oversell_guard doc counter
// nay, va HealthProbe cua admit controller doc do tre theo ket qua.

type spyHoldMetrics struct {
	mu       sync.Mutex
	outcomes []string
	durs     []time.Duration
	guard    int
}

func (s *spyHoldMetrics) HoldAttempt(outcome string, d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outcomes = append(s.outcomes, outcome)
	s.durs = append(s.durs, d)
}

func (s *spyHoldMetrics) OversellGuardRejected() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.guard++
}

func TestCreateHold_Metrics_KetQuaNghiepVu(t *testing.T) {
	cases := []struct {
		name  string
		setup func(g *fakeGate, r *fakeRepo)
		qty   int
		want  string
	}{
		{"thanh cong", func(*fakeGate, *fakeRepo) {}, 1, port.OutcomeOK},
		{"chua duoc admit", func(g *fakeGate, _ *fakeRepo) { g.admitted = false }, 1, port.OutcomeNotAdmitted},
		{"het ve", func(g *fakeGate, _ *fakeRepo) {
			g.holdFn = func(port.HoldRequest) (port.HoldResult, error) {
				return port.HoldResult{Status: port.HoldSoldOut}, nil
			}
		}, 1, port.OutcomeSoldOut},
		{"vuot gioi han don", func(*fakeGate, *fakeRepo) {}, 5, port.OutcomeLimit},
		{"vuot gioi han moi nguoi", func(_ *fakeGate, r *fakeRepo) { r.purchased = 4 }, 1, port.OutcomeLimit},
		{"loi he thong", func(_ *fakeGate, r *fakeRepo) { r.persistErr = errors.New("postgres chet") }, 1, port.OutcomeError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gate, repo := moiTruongHopLe()
			tc.setup(gate, repo)
			spy := &spyHoldMetrics{}
			uc := app.NewCreateHold(gate, repo, loggerBo()).WithMetrics(spy)

			_, _ = uc.Execute(context.Background(), dauVaoHopLe(tc.qty))

			if len(spy.outcomes) != 1 || spy.outcomes[0] != tc.want {
				t.Fatalf("ket qua = %v, muon [%s]", spy.outcomes, tc.want)
			}
			if spy.durs[0] < 0 {
				t.Errorf("thoi gian am: %v", spy.durs[0])
			}
			if spy.guard != 0 {
				t.Errorf("khong co lech Redis/Postgres nhung guard = %d", spy.guard)
			}
		})
	}
}

// Redis noi con ve nhung Postgres tu choi: day la tin hieu Redis lech so. Khach van
// nhan het ve (an toan), nhung alert nghiem trong phai biet.
func TestCreateHold_Metrics_RedisLechSoVoiPostgres_GhiOversellGuard(t *testing.T) {
	gate, repo := moiTruongHopLe()
	repo.persistErr = domain.ErrInventoryExhausted
	spy := &spyHoldMetrics{}
	uc := app.NewCreateHold(gate, repo, loggerBo()).WithMetrics(spy)

	_, err := uc.Execute(context.Background(), dauVaoHopLe(1))

	if !errors.Is(err, domain.ErrSoldOut) {
		t.Fatalf("khach phai nhan het ve, nhan %v", err)
	}
	if spy.guard != 1 {
		t.Errorf("oversell guard = %d, muon 1", spy.guard)
	}
	if len(spy.outcomes) != 1 || spy.outcomes[0] != port.OutcomeSoldOut {
		t.Errorf("ket qua = %v", spy.outcomes)
	}
}

func TestCreateHold_KhongGanMetrics_VanChayBinhThuong(t *testing.T) {
	gate, repo := moiTruongHopLe()
	uc := app.NewCreateHold(gate, repo, loggerBo())

	if _, err := uc.Execute(context.Background(), dauVaoHopLe(1)); err != nil {
		t.Fatalf("khong gan metrics van phai chay: %v", err)
	}
}
