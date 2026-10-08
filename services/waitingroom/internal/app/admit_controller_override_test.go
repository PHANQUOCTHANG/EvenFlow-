package app

// BR-Q5: "Ops co the ghi de thu cong (manual_override)". Ghi de chi co nghia khi controller
// TON TRONG gia tri do o cac tick tiep theo, cho toi khi Ops bo ghi de.
//
// Hai test dau tung nam sau build tag bugrepro (admit_controller_bug_test.go) vi chung FAIL:
// SetRateOverride chi ghi bien rate, tick ke tiep chay AIMD tu gia tri do va ghi de lai --
// Ops dat 42/s thi tick sau thanh 52/s (khoe) hoac 29.4/s (qua tai); Ops dat 0 de dung khan
// cap thi 3 tick sau van tha 60 nguoi. Bug da sua nen chung chay mac dinh.

import (
	"testing"
)

func TestSetRateOverride_IsRespectedOnNextTicks(t *testing.T) {
	for _, h := range []Health{goodHealth, badP99} {
		c, q, _ := newTestController(h, nil)
		c.SetRateOverride(42)
		for i := 0; i < 3; i++ {
			mustTick(t, c)
		}
		_, admits, rates := q.snapshot()
		for i, r := range rates {
			if r != 42 {
				t.Errorf("health %+v, tick %d: Ops ghi de 42/s nhung controller cong bo %v/s", h, i, r)
			}
		}
		for i, a := range admits {
			if a.batch != 42 {
				t.Errorf("health %+v, tick %d: Ops ghi de 42/s nhung tha %d nguoi", h, i, a.batch)
			}
		}
	}
}

func TestSetRateOverride_ZeroPausesAdmissionOnNextTicks(t *testing.T) {
	for _, h := range []Health{goodHealth, badP99} {
		c, q, _ := newTestController(h, nil)
		c.SetRateOverride(0)
		for i := 0; i < 3; i++ {
			mustTick(t, c)
		}
		if _, admits, _ := q.snapshot(); len(admits) != 0 {
			total := 0
			for _, a := range admits {
				total += a.batch
			}
			t.Errorf("health %+v: Ops tam dung (0/s) nhung 3 tick sau van tha %d nguoi qua %d lo", h, total, len(admits))
		}
	}
}

// Bo ghi de thi AIMD phai tu dieu chinh tiep -- khong duoc ket mai o gia tri Ops da dat.
func TestClearRateOverride_HandsControlBackToAIMD(t *testing.T) {
	t.Run("khoe: rate tang lai tu gia tri ghi de", func(t *testing.T) {
		c, _, _ := newTestController(goodHealth, nil)
		c.SetRateOverride(42)
		mustTick(t, c)
		if c.Rate() != 42 {
			t.Fatalf("dang ghi de: rate phai giu 42, nhan %v", c.Rate())
		}
		c.ClearRateOverride()
		mustTick(t, c)
		if c.Rate() <= 42 {
			t.Errorf("da bo ghi de va he thong khoe: rate phai tang khoi 42, nhan %v", c.Rate())
		}
	})
	t.Run("sau khi tam dung 0/s: bat dau lai tu san MinRate, khong dung im o 0", func(t *testing.T) {
		c, q, _ := newTestController(badP99, nil)
		c.SetRateOverride(0)
		mustTick(t, c)
		c.ClearRateOverride()
		mustTick(t, c)
		if c.Rate() < c.cfg.MinRate {
			t.Errorf("da bo ghi de: rate phai >= MinRate %v, nhan %v", c.cfg.MinRate, c.Rate())
		}
		if _, admits, _ := q.snapshot(); len(admits) == 0 {
			t.Errorf("da bo ghi de: hang cho phai nho giot tro lai (BR-Q5 san MinRate), nhung khong tha ai")
		}
	})
}
