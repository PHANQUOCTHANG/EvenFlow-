package app_test

// Mutation gap T08 (ops-warroom verification): contract 4.2 invariant. The payment breakdown, including the
// "unknown" group, must always sum to totals: SUM(orders) = paid_orders and SUM(amount_cents) = gross_revenue_cents.

import (
	"context"
	"testing"

	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

func TestGetOrganizerSales_Contract42_BreakdownSumsToTotals(t *testing.T) {
	cases := []struct {
		name        string
		rows        port.SalesRows
		wantUnknown *port.PaymentRow // nil = no unknown row expected
	}{
		{
			name: "remaining orders with zero amount (free tier, no SUCCEEDED payment)",
			rows: port.SalesRows{PaidOrders: 3,
				Tiers:    []port.TierSalesRow{{Name: "Paid", Sold: 2, RevenueCents: 100000}, {Name: "Free", Sold: 1, RevenueCents: 0}},
				Payments: []port.PaymentRow{{Provider: "vnpay", Orders: 2, AmountCents: 100000}}},
			wantUnknown: &port.PaymentRow{Provider: app.UnknownProvider, Orders: 1, AmountCents: 0},
		},
		{
			name: "all orders free and unpaid",
			rows: port.SalesRows{PaidOrders: 2,
				Tiers: []port.TierSalesRow{{Name: "Free", Sold: 2, RevenueCents: 0}}},
			wantUnknown: &port.PaymentRow{Provider: app.UnknownProvider, Orders: 2, AmountCents: 0},
		},
		{
			name: "counts match but amount differs",
			rows: port.SalesRows{PaidOrders: 2,
				Tiers:    []port.TierSalesRow{{Name: "A", Sold: 2, RevenueCents: 300}},
				Payments: []port.PaymentRow{{Provider: "momo", Orders: 2, AmountCents: 200}}},
			wantUnknown: &port.PaymentRow{Provider: app.UnknownProvider, Orders: 0, AmountCents: 100},
		},
		{
			name: "remaining orders and amount both positive",
			rows: port.SalesRows{PaidOrders: 3,
				Tiers:    []port.TierSalesRow{{Name: "A", Sold: 3, RevenueCents: 300}},
				Payments: []port.PaymentRow{{Provider: "momo", Orders: 1, AmountCents: 100}}},
			wantUnknown: &port.PaymentRow{Provider: app.UnknownProvider, Orders: 2, AmountCents: 200},
		},
		{
			name: "fully reconciled: no unknown row",
			rows: port.SalesRows{PaidOrders: 2,
				Tiers:    []port.TierSalesRow{{Name: "A", Sold: 2, RevenueCents: 200}},
				Payments: []port.PaymentRow{{Provider: "momo", Orders: 2, AmountCents: 200}}},
		},
		{
			name: "empty event: no rows at all",
			rows: port.SalesRows{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.rows.OrganizerID = "org"
			got, err := app.NewGetOrganizerSales(salesReader{rows: tc.rows}).Execute(context.Background(), "ev")
			if err != nil {
				t.Fatal(err)
			}
			var orders int
			var amount int64
			var unknown *port.PaymentRow
			for i, p := range got.Payments {
				orders += p.Orders
				amount += p.AmountCents
				if p.Provider == app.UnknownProvider {
					if unknown != nil {
						t.Fatalf("two unknown rows: %+v", got.Payments)
					}
					unknown = &got.Payments[i]
				}
			}
			if orders != got.Totals.PaidOrders || amount != got.Totals.GrossRevenueCents {
				t.Fatalf("breakdown sums orders=%d amount=%d, totals paid_orders=%d gross=%d (%+v)",
					orders, amount, got.Totals.PaidOrders, got.Totals.GrossRevenueCents, got.Payments)
			}
			switch {
			case tc.wantUnknown == nil && unknown != nil:
				t.Fatalf("unexpected unknown row %+v", *unknown)
			case tc.wantUnknown != nil && (unknown == nil || *unknown != *tc.wantUnknown):
				t.Fatalf("unknown row = %+v, want %+v", unknown, *tc.wantUnknown)
			}
		})
	}
}
