package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// BR-O3 (docs/01-nghiep-vu.md, sua 2026-10-08): "Goi tao hold khi da co hold con han -> tra ve
// DUNG hold dang co (cung hang ve, so luong va expires_at; dong ho khong reset, khong giu
// them ghe)."
//
// Truoc ngay do BR-O3 ghi "hold moi huy hold cu va tra kho ngay"; test repro theo ban cu nam
// sau build tag bugrepro. Nhom quyet sua TAI LIEU theo code (khong co endpoint huy hold, va
// bam lai khong nen lam khach mat cho), nen test do duoc thay bang test nay -- chay mac dinh.
func TestCreateHold_BRO3_DaCoHoldConHanThiTraLaiDungHoldDo(t *testing.T) {
	holdCu := port.HoldView{
		OrderID:   "order-cu",
		HoldID:    "hold-cu",
		Bucket:    7,
		ExpiresAt: time.Now().Add(5 * time.Minute).Truncate(time.Second),
	}
	gate := &fakeGate{admitted: true}
	gate.holdFn = func(port.HoldRequest) (port.HoldResult, error) {
		return port.HoldResult{Status: port.HoldAlreadyHolding, ExistingHold: holdCu.HoldID}, nil
	}
	repo := &fakeRepo{cfg: cauHinhMacDinh(), active: holdCu}
	uc := app.NewCreateHold(gate, repo, loggerBo())

	out, err := uc.Execute(context.Background(), dauVaoHopLe(2))
	if err != nil {
		t.Fatalf("da co hold con han: phai tra lai hold do, khong phai loi; nhan %v", err)
	}

	if out.HoldID != holdCu.HoldID || out.OrderID != holdCu.OrderID {
		t.Errorf("phai tra DUNG hold dang co %s/%s, nhan %s/%s", holdCu.OrderID, holdCu.HoldID, out.OrderID, out.HoldID)
	}
	if !out.ExpiresAt.Equal(holdCu.ExpiresAt) {
		t.Errorf("dong ho KHONG duoc reset: expires_at phai giu %v, nhan %v", holdCu.ExpiresAt, out.ExpiresAt)
	}
	if len(repo.persistCalls) != 0 {
		t.Errorf("khong duoc tao hold moi / giu them ghe o Postgres, nhan %d lan persist", len(repo.persistCalls))
	}
	if len(gate.releaseCalls) != 0 || len(repo.releaseCalls) != 0 {
		t.Errorf("khong duoc tra kho hold dang co (Redis %d, Postgres %d lan)", len(gate.releaseCalls), len(repo.releaseCalls))
	}
}
