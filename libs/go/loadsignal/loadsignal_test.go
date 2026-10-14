package loadsignal

import (
	"testing"
	"time"
)

// Recorder: nghiep vu la "p99 va ty le loi cua 5 giay gan nhat", khong phai cua ca doi tien trinh.

func recAt(t0 time.Time) (*Recorder, *time.Time) {
	now := t0
	r := NewRecorder()
	r.now = func() time.Time { return now }
	return r, &now
}

func TestRecorder_P99VaTyLeLoi(t *testing.T) {
	r, _ := recAt(time.Unix(1_000_000, 0))
	for i := 1; i <= 100; i++ { // 1..100 ms
		r.Observe(time.Duration(i)*time.Millisecond, i > 95) // 5 loi
	}
	s := r.Snapshot("pod-1", 0.4)

	if s.Requests != 100 || s.Errors != 5 {
		t.Fatalf("requests/errors = %d/%d, muon 100/5", s.Requests, s.Errors)
	}
	if s.P99Ms != 99 {
		t.Errorf("p99 = %v ms, muon 99 (nearest-rank cua 1..100)", s.P99Ms)
	}
	if s.PoolUsage != 0.4 || s.Pod != "pod-1" {
		t.Errorf("snapshot = %+v", s)
	}
}

// Mau cu hon 5 giay phai roi khoi cua so: mot dot cham da qua khong duoc giu he thong o toc do thap.
func TestRecorder_CuaSoTruot_MauCuBiLoai(t *testing.T) {
	r, now := recAt(time.Unix(1_000_000, 0))
	r.Observe(3*time.Second, true) // dot cham o giay 0

	*now = now.Add(2 * time.Second)
	r.Observe(50*time.Millisecond, false)
	if s := r.Snapshot("p", 0); s.P99Ms != 3000 || s.Errors != 1 {
		t.Fatalf("giay 2 van phai thay dot cham: %+v", s)
	}

	*now = now.Add(4 * time.Second) // giay 6: mau o giay 0 da ra khoi cua so 5 giay
	r.Observe(50*time.Millisecond, false)
	s := r.Snapshot("p", 0)
	if s.P99Ms != 50 || s.Errors != 0 || s.Requests != 2 {
		t.Fatalf("giay 6 chi con mau gan: %+v, muon p99=50 errors=0 requests=2", s)
	}
}

// Du khong co yeu cau nao: Snapshot hop le (p99 0, 0 yeu cau) -- he thong "khoe" truoc T0.
func TestRecorder_KhongCoYeuCau(t *testing.T) {
	r, _ := recAt(time.Unix(1_000_000, 0))
	s := r.Snapshot("p", 0.1)
	if s.Requests != 0 || s.P99Ms != 0 || s.Errors != 0 {
		t.Fatalf("snapshot rong sai: %+v", s)
	}
}

// Bo nho co dinh du so mau vuot xa maxSamples; p99 van xap xi dung.
func TestRecorder_ReservoirGioiHanBoNhoVaGiuP99(t *testing.T) {
	r, _ := recAt(time.Unix(1_000_000, 0))
	const n = 50_000
	for i := 0; i < n; i++ {
		d := 10 * time.Millisecond
		if i%100 == 0 { // dung 1% cham 1s
			d = time.Second
		}
		r.Observe(d, false)
	}
	s := r.Snapshot("p", 0)
	if s.Requests != n {
		t.Fatalf("requests = %d, muon %d", s.Requests, n)
	}
	for i := range r.buckets {
		if len(r.buckets[i].samples) > maxSamples {
			t.Fatalf("bucket %d giu %d mau, vuot gioi han %d", i, len(r.buckets[i].samples), maxSamples)
		}
	}
	// 1% cham dung bang nguong p99: chap nhan ca hai phia, nhung khong duoc thap hon nhieu.
	if s.P99Ms != 10 && s.P99Ms != 1000 {
		t.Errorf("p99 = %v, muon 10 hoac 1000", s.P99Ms)
	}
}
