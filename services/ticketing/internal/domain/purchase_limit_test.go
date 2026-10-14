package domain_test

import (
	"slices"
	"testing"

	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
)

// BR-O4: so ve con chon duoc = min(max_per_order, max_per_identity - da chiem), khong am.
func TestMaxSelectable_BRO4(t *testing.T) {
	cases := []struct {
		name                     string
		maxOrder, maxIdent, used int
		want                     int
	}{
		{"chua mua gi, mac dinh 4/4", 4, 4, 0, 4},
		{"da mua 1", 4, 4, 1, 3},
		{"da mua du", 4, 4, 4, 0},
		{"du lieu cu vuot han muc thi van la 0, khong am", 4, 4, 6, 0},
		{"max_per_order chat hon", 2, 6, 1, 2},
		{"han muc ca nhan chat hon", 4, 6, 4, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := domain.MaxSelectable(tc.maxOrder, tc.maxIdent, tc.used); got != tc.want {
				t.Fatalf("MaxSelectable(%d, %d, %d) = %d, mong doi %d",
					tc.maxOrder, tc.maxIdent, tc.used, got, tc.want)
			}
		})
	}
}

// Don da chot la tap con cua don chiem han muc: neu lop nhanh dem mot trang thai ma lop chot
// khong dem, lop nhanh se tu choi nhung don ma nguon su that cho phep.
func TestOrderStatusSets_DaChotLaTapConCuaChiemHanMuc(t *testing.T) {
	for _, s := range domain.SettledOrderStatuses {
		if !slices.Contains(domain.QuotaOrderStatuses, s) {
			t.Errorf("trang thai da chot %q khong nam trong tap chiem han muc", s)
		}
	}
	// Don het han / huy / da hoan tien tra lai han muc (BR-O4).
	for _, s := range []string{"EXPIRED", "CANCELLED", "REFUNDED"} {
		if slices.Contains(domain.QuotaOrderStatuses, s) {
			t.Errorf("don %s phai tra lai han muc, khong duoc dem", s)
		}
	}
	// Don dang hoat dong chiem han muc nhung KHONG duoc lop nhanh dem (BR-O3: bam lai tra hold cu).
	for _, s := range []string{"HELD", "PAYMENT_PENDING"} {
		if !slices.Contains(domain.QuotaOrderStatuses, s) || slices.Contains(domain.SettledOrderStatuses, s) {
			t.Errorf("don %s phai chiem han muc nhung khong phai don da chot", s)
		}
	}
}
