//go:build bugrepro

package app

// Repro bug -- KHONG chay mac dinh. Chay: go test -tags bugrepro ./internal/app/ -run Bug -v
//
// BR-Q5: "Ops co the ghi de thu cong (manual_override)". Ghi de chi co nghia khi controller
// TON TRONG gia tri do o cac tick tiep theo. Hien tai SetRateOverride chi ghi de bien rate;
// tick ke tiep (1 giay sau) chay AIMD tu gia tri do va ghi de lai, nen:
//   - Ops dat 42/s -> tick sau cong bo va admit 52/s (khoe) hoac 29.4/s (qua tai).
//   - Ops dat 0 de tam dung khan cap -> tick sau tha lai 10 nguoi (khoe) hoac MinRate (qua tai).

import (
	"testing"
)

func TestBugRepro_OverrideIsRespectedOnNextTicks(t *testing.T) {
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

func TestBugRepro_OverrideZeroPausesAdmission(t *testing.T) {
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
