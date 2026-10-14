package app_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// EV-130 -- BR-O4 (gioi han mua) phoi hop voi BR-O3 (mot hold dang hoat dong).

// BR-O3: "Goi tao hold khi da co hold con han -> tra ve DUNG hold dang co". Khach dang giu 3 ve
// bam mua lai voi so luong 2 khong duoc nhan 422 "vuot gioi han": 3 + 2 > 4 chi vi tinh trung
// chinh hold dang giu. Phai nhan lai hold do, khong giu them ghe.
func TestCreateHold_BRO4_DangGiuGheBamMuaLaiThiNhanLaiHoldCu(t *testing.T) {
	holdCu := port.HoldView{
		OrderID:   "order-cu",
		HoldID:    "hold-cu",
		ExpiresAt: time.Now().Add(4 * time.Minute).Truncate(time.Second),
	}
	gate, repo := moiTruongHopLe()
	gate.holdFn = func(port.HoldRequest) (port.HoldResult, error) {
		return port.HoldResult{Status: port.HoldAlreadyHolding, ExistingHold: holdCu.HoldID}, nil
	}
	repo.purchased = 3 // so ve dang chiem han muc gom ca hold dang giu
	repo.active = holdCu
	uc := app.NewCreateHold(gate, repo, loggerBo())

	out, err := uc.Execute(context.Background(), dauVaoHopLe(2))
	if err != nil {
		t.Fatalf("dang giu ghe bam mua lai phai nhan lai hold cu (BR-O3), nhan loi: %v", err)
	}
	if out.HoldID != holdCu.HoldID || !out.ExpiresAt.Equal(holdCu.ExpiresAt) {
		t.Errorf("phai tra DUNG hold dang co %+v, nhan %+v", holdCu, out)
	}
	if len(repo.persistCalls) != 0 {
		t.Errorf("khong duoc giu them ghe, nhan %d lan persist", len(repo.persistCalls))
	}
}

// Khong co hold dang hoat dong thi vuot han muc van la vuot han muc: lop nhanh tu choi, khong
// cham ton kho.
func TestCreateHold_BRO4_VuotHanMucKhongCoHoldThiTuChoi(t *testing.T) {
	gate, repo := moiTruongHopLe()
	repo.purchased = 3
	uc := app.NewCreateHold(gate, repo, loggerBo())

	_, err := uc.Execute(context.Background(), dauVaoHopLe(2))
	if !errors.Is(err, domain.ErrPerIdentityLimit) {
		t.Fatalf("mong doi ErrPerIdentityLimit, nhan %v", err)
	}
	if len(gate.holdCalls) != 0 || len(repo.persistCalls) != 0 {
		t.Errorf("tu choi o lop nhanh thi khong duoc cham ton kho (Redis %d, Postgres %d)",
			len(gate.holdCalls), len(repo.persistCalls))
	}
}

// Lop chot: han muc phai duoc truyen xuong transaction, vi chi o do moi dem dung duoi dong thoi.
func TestCreateHold_BRO4_TruyenHanMucXuongTransaction(t *testing.T) {
	gate, repo := moiTruongHopLe()
	repo.cfg.MaxPerIdentity = 6
	uc := app.NewCreateHold(gate, repo, loggerBo())

	if _, err := uc.Execute(context.Background(), dauVaoHopLe(1)); err != nil {
		t.Fatalf("mong doi thanh cong, nhan %v", err)
	}
	if got := repo.persistCalls[0].MaxPerIdentity; got != 6 {
		t.Fatalf("PersistHold phai nhan max_per_identity = 6, nhan %d", got)
	}
}

// Lop chot tu choi (vd don cu vua chuyen sang PAID giua luc lop nhanh dem va luc ghi): phai tra
// kho Redis dung mot lan, bao ErrPerIdentityLimit, va KHONG coi la Redis lech (khong phai oversell
// guard).
func TestCreateHold_BRO4_LopChotTuChoiThiTraKhoVaBaoVuotHanMuc(t *testing.T) {
	gate, repo := moiTruongHopLe()
	repo.persistErr = fmt.Errorf("persist hold: %w", domain.ErrPerIdentityLimit)
	m := &spyMetrics{}
	uc := app.NewCreateHold(gate, repo, loggerBo()).WithMetrics(m)

	_, err := uc.Execute(context.Background(), dauVaoHopLe(2))
	if !errors.Is(err, domain.ErrPerIdentityLimit) {
		t.Fatalf("mong doi ErrPerIdentityLimit, nhan %v", err)
	}
	if len(gate.releaseCalls) != 1 || gate.releaseCalls[0].HoldID != gate.holdCalls[0].HoldID {
		t.Fatalf("phai tra kho Redis dung hold vua giu, dung 1 lan; nhan %v", gate.releaseCalls)
	}
	if m.guard != 0 {
		t.Errorf("vuot han muc khong phai Redis lech, khong duoc tang oversell guard (nhan %d)", m.guard)
	}
	if len(m.outcomes) != 1 || m.outcomes[0] != port.OutcomeLimit {
		t.Errorf("ket qua metric phai la %q, nhan %v", port.OutcomeLimit, m.outcomes)
	}
}

// Khong tra cuu duoc hold dang co thi khong doan: bao loi ha tang, khong bao vuot han muc sai.
func TestCreateHold_BRO4_KhongTraCuuDuocHoldThiBaoLoi(t *testing.T) {
	gate, repo := moiTruongHopLe()
	repo.purchased = 4
	loi := errors.New("mat ket noi")
	repo.activeErr = loi
	uc := app.NewCreateHold(gate, repo, loggerBo())

	if _, err := uc.Execute(context.Background(), dauVaoHopLe(1)); !errors.Is(err, loi) {
		t.Fatalf("mong doi loi ha tang, nhan %v", err)
	}
}

type spyMetrics struct {
	outcomes []string
	guard    int
}

func (s *spyMetrics) HoldAttempt(o string, _ time.Duration) { s.outcomes = append(s.outcomes, o) }
func (s *spyMetrics) OversellGuardRejected()                { s.guard++ }
