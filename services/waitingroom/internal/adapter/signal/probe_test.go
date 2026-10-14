package signaladapter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/loadsignal"
)

type fakeReader struct {
	a   loadsignal.Aggregate
	err error
}

func (f fakeReader) Read(context.Context) (loadsignal.Aggregate, error) { return f.a, f.err }

func TestProbe_DichDungTinHieu(t *testing.T) {
	h, err := NewProbe(fakeReader{a: loadsignal.Aggregate{
		Pods: 2, P99: 900 * time.Millisecond, ErrorRate: 0.03, PoolUsage: 0.85,
	}}).Health(context.Background(), "ev")
	if err != nil {
		t.Fatal(err)
	}
	if h.CheckoutP99 != 900*time.Millisecond || h.ErrorRate != 0.03 || h.DBPoolUsage != 0.85 {
		t.Fatalf("health = %+v", h)
	}
}

// Khong biet khong phai la khoe: phai tra loi de controller giam nhip (Q5.3).
func TestProbe_KhongCoTinHieu_LaLoiChuKhongPhaiKhoe(t *testing.T) {
	_, err := NewProbe(fakeReader{err: loadsignal.ErrNoSignal}).Health(context.Background(), "ev")
	if !errors.Is(err, loadsignal.ErrNoSignal) {
		t.Fatalf("muon ErrNoSignal, nhan %v", err)
	}
	if _, err := NewProbe(fakeReader{err: errors.New("redis timeout")}).Health(context.Background(), "ev"); err == nil {
		t.Fatal("Redis loi phai la loi")
	}
}

// Truoc T0 chua co yeu cau nao: pod van bao cao 0 -> khoe.
func TestProbe_PodBaoCaoKhongYeuCau_LaKhoe(t *testing.T) {
	h, err := NewProbe(fakeReader{a: loadsignal.Aggregate{Pods: 1}}).Health(context.Background(), "ev")
	if err != nil {
		t.Fatalf("pod bao cao 0 yeu cau phai la khoe, nhan loi %v", err)
	}
	if h.CheckoutP99 != 0 || h.ErrorRate != 0 {
		t.Fatalf("health = %+v", h)
	}
}
