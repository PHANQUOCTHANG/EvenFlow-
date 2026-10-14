package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

type fakeLimitSource struct {
	l   port.PurchaseLimit
	err error
}

func (f fakeLimitSource) GetPurchaseLimit(context.Context, string, string) (port.PurchaseLimit, error) {
	return f.l, f.err
}

// BR-O4: UI nhan so ve con chon duoc do server tinh; BR-O3: dang giu ghe thi khong chon them.
func TestGetPurchaseLimit_TinhSoVeConChonDuoc(t *testing.T) {
	cases := []struct {
		name string
		in   port.PurchaseLimit
		want int
	}{
		{"chua mua gi", port.PurchaseLimit{MaxPerOrder: 4, MaxPerIdentity: 4}, 4},
		{"da mua 3", port.PurchaseLimit{MaxPerOrder: 4, MaxPerIdentity: 4, Used: 3}, 1},
		{"da mua du", port.PurchaseLimit{MaxPerOrder: 4, MaxPerIdentity: 4, Used: 4}, 0},
		{"max_per_order chat hon", port.PurchaseLimit{MaxPerOrder: 2, MaxPerIdentity: 6, Used: 1}, 2},
		{"dang giu ghe thi khong chon them", port.PurchaseLimit{MaxPerOrder: 4, MaxPerIdentity: 4, Used: 2, HasActiveHold: true}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := app.NewGetPurchaseLimit(fakeLimitSource{l: tc.in}).Execute(context.Background(), suKien, khachHang)
			if err != nil {
				t.Fatalf("mong doi thanh cong, nhan %v", err)
			}
			if got.MaxSelectable != tc.want {
				t.Fatalf("max_selectable = %d, mong doi %d", got.MaxSelectable, tc.want)
			}
			if got.Used != tc.in.Used || got.MaxPerOrder != tc.in.MaxPerOrder || got.MaxPerIdentity != tc.in.MaxPerIdentity {
				t.Errorf("khong duoc doi so lieu goc: vao %+v, ra %+v", tc.in, got)
			}
		})
	}
}

func TestGetPurchaseLimit_LoiNguonDuocTraNguyen(t *testing.T) {
	_, err := app.NewGetPurchaseLimit(fakeLimitSource{err: domain.ErrEventNotFound}).
		Execute(context.Background(), suKien, khachHang)
	if !errors.Is(err, domain.ErrEventNotFound) {
		t.Fatalf("mong doi ErrEventNotFound, nhan %v", err)
	}
}
