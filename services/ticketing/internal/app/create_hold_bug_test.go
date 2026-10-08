//go:build bugrepro

package app_test

// Cac test o day DUNG nghiep vu nhung hien dang FAIL tren ma nguon. Dat sau build
// tag bugrepro de khong chan CI; chay bang:
//
//	go test -tags bugrepro -race -count=1 -run TestBug ./internal/app/

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// BUG BR-O3: "Tao hold moi -> hold cu bi huy va tra kho ngay". Ma nguon hien tra
// lai hold cu (create_hold.go, nhanh HoldAlreadyHolding) thay vi huy no va tao
// hold moi.
func TestBug_BRO3_TaoHoldMoiPhaiHuyHoldCuVaTraKho(t *testing.T) {
	const holdCu = "hold-cu"
	gate := &fakeGate{admitted: true}
	repo := &fakeRepo{
		cfg:    cauHinhMacDinh(),
		active: port.HoldView{OrderID: "order-cu", HoldID: holdCu, ExpiresAt: time.Now().Add(5 * time.Minute)},
	}
	daTraHoldCu := false
	gate.holdFn = func(port.HoldRequest) (port.HoldResult, error) {
		if !daTraHoldCu {
			// Redis van thay hold cu con han cho toi khi no duoc tra.
			for _, rc := range gate.releaseCalls {
				if rc.HoldID == holdCu {
					daTraHoldCu = true
				}
			}
		}
		if !daTraHoldCu {
			return port.HoldResult{Status: port.HoldAlreadyHolding, ExistingHold: holdCu}, nil
		}
		return port.HoldResult{Status: port.HoldOK, Bucket: 3}, nil
	}
	uc := app.NewCreateHold(gate, repo, loggerBo())

	out, err := uc.Execute(context.Background(), dauVaoHopLe(2))
	if err != nil {
		t.Fatalf("tao hold moi khong nen loi, nhan %v", err)
	}

	traRedis, traDB := false, false
	for _, rc := range gate.releaseCalls {
		if rc.HoldID == holdCu {
			traRedis = true
		}
	}
	for _, id := range repo.releaseCalls {
		if id == holdCu {
			traDB = true
		}
	}
	if !traRedis || !traDB {
		t.Errorf("BR-O3: hold cu phai bi huy va tra kho ngay (Redis=%v, Postgres=%v)", traRedis, traDB)
	}
	if out.HoldID == holdCu {
		t.Errorf("BR-O3: khach phai nhan hold MOI, nhan lai hold cu %s", out.HoldID)
	}
	if len(repo.persistCalls) != 1 {
		t.Errorf("BR-O3: phai chot dung 1 hold moi, nhan %d", len(repo.persistCalls))
	}
}

// BUG (chay voi -race): CreateHold dung chung mot *rand.Rand cho moi request.
// rand.Rand KHONG an toan cho nhieu goroutine, trong khi Execute duoc goi dong
// thoi tu HTTP handler -- dung luc mo ban, khi nhieu khach cung giu ghe (BR-O1).
// Race nay lam hong trang thai nguon ngau nhien va co the panic giua request.
func TestBug_NhieuKhachGiuGheDongThoi_KhongDuocDataRace(t *testing.T) {
	const quota = 10
	const soKhach = 50

	var mu sync.Mutex
	conLai := quota
	gate := &fakeGate{admitted: true}
	gate.holdFn = func(port.HoldRequest) (port.HoldResult, error) {
		mu.Lock()
		defer mu.Unlock()
		if conLai <= 0 {
			return port.HoldResult{Status: port.HoldSoldOut}, nil
		}
		conLai--
		return port.HoldResult{Status: port.HoldOK}, nil
	}
	repo := &fakeRepo{cfg: cauHinhMacDinh()}
	uc := app.NewCreateHold(gate, repo, loggerBo())

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < soKhach; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _ = uc.Execute(context.Background(), dauVaoHopLe(1))
		}()
	}
	close(start)
	wg.Wait()

	if n := len(repo.persistCalls); n != quota {
		t.Errorf("BR-O1: so hold chot phai dung bang quota %d, nhan %d", quota, n)
	}
}
