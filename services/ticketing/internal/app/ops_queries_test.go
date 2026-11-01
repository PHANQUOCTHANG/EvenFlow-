package app_test

import (
	"context"
	"testing"

	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

type salesReader struct {
	rows  port.SalesRows
	owner string
}

func (s salesReader) EventExists(context.Context, string) (bool, error) { return true, nil }
func (s salesReader) EventOwner(context.Context, string) (string, error) {
	if s.owner == "" {
		return "", domain.ErrEventNotFound
	}
	return s.owner, nil
}
func (s salesReader) TierInventory(context.Context, string) ([]port.TierInventoryRow, error) {
	return nil, nil
}
func (s salesReader) EventSales(context.Context, string) (port.SalesRows, error) { return s.rows, nil }

// contracts 4.2 / AC-15: don da ban khong co payment SUCCEEDED -> nhom "unknown"; tong khop paid_orders va gross.
func TestGetOrganizerSales_AC15_UnknownProviderReconcilesTotals(t *testing.T) {
	r := salesReader{rows: port.SalesRows{
		OrganizerID: "org",
		PaidOrders:  4,
		Tiers: []port.TierSalesRow{
			{Name: "A", Quota: 100, Sold: 6, ActiveHolds: 7, RevenueCents: 600000},
			{Name: "B", Quota: 50, Sold: 1, RevenueCents: 200000},
		},
		Payments: []port.PaymentRow{
			{Provider: "vnpay", Orders: 2, AmountCents: 500000},
			{Provider: "momo", Orders: 1, AmountCents: 200000},
		},
	}}
	got, err := app.NewGetOrganizerSales(r).Execute(context.Background(), "ev")
	if err != nil {
		t.Fatal(err)
	}
	if got.Totals.GrossRevenueCents != 800000 || got.Totals.TicketsSold != 7 || got.Totals.Quota != 150 ||
		got.Totals.ActiveHolds != 7 || got.Totals.PaidOrders != 4 || got.OrganizerID != "org" {
		t.Fatalf("totals: %+v", got)
	}
	var orders int
	var amount int64
	for _, p := range got.Payments {
		orders += p.Orders
		amount += p.AmountCents
	}
	if orders != 4 || amount != 800000 {
		t.Fatalf("breakdown does not reconcile: orders=%d amount=%d %+v", orders, amount, got.Payments)
	}
	want := []port.PaymentRow{
		{Provider: "vnpay", Orders: 2, AmountCents: 500000},
		{Provider: "momo", Orders: 1, AmountCents: 200000},
		{Provider: "unknown", Orders: 1, AmountCents: 100000},
	}
	for i, w := range want {
		if got.Payments[i] != w {
			t.Fatalf("payments[%d] = %+v, want %+v (sorted by amount desc)", i, got.Payments[i], w)
		}
	}
}

func TestGetOrganizerSales_NoUnknownWhenReconciled_AndOwnerFallback(t *testing.T) {
	r := salesReader{owner: "org-2", rows: port.SalesRows{
		PaidOrders: 1,
		Tiers:      []port.TierSalesRow{{RevenueCents: 100}},
		Payments:   []port.PaymentRow{{Provider: "sandbox", Orders: 1, AmountCents: 100}},
	}}
	got, err := app.NewGetOrganizerSales(r).Execute(context.Background(), "ev")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Payments) != 1 || got.OrganizerID != "org-2" {
		t.Fatalf("%+v", got)
	}
	if _, err := app.NewGetOrganizerSales(salesReader{}).Execute(context.Background(), "ev"); err != domain.ErrEventNotFound {
		t.Fatalf("unknown owner: %v", err)
	}
}
