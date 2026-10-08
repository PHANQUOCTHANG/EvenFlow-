package app_test

import (
	"context"
	"sync"
	"testing"

	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// Execute duoc HTTP handler goi DONG THOI -- dung luc mo ban, khi nhieu khach cung giu
// ghe. Test nay co nghia nhat khi chay voi -race (CI test-go chay `go test -race`): ban
// cu dung chung mot *math/rand.Rand cho moi request, kieu do KHONG an toan dong thoi, va
// test nay bao 11 DATA RACE. So hold chot dung bang quota la BR-O1.
//
// Truoc day test nay nam sau build tag bugrepro (create_hold_bug_test.go) vi no FAIL;
// bug da sua bang math/rand/v2 nen no chay mac dinh, de race quay lai la CI do ngay.
func TestCreateHold_NhieuKhachGiuGheDongThoi_KhongDuocDataRace(t *testing.T) {
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
