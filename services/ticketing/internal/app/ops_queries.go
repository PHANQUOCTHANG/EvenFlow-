package app

import (
	"context"
	"sort"

	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// UnknownProvider gom cac don da ban khong co payment SUCCEEDED nao (contracts 4.2).
const UnknownProvider = "unknown"

// GetOpsInventory: ton kho tung hang ve cho war room Ops (contracts 4.1).
type GetOpsInventory struct{ r port.OpsReader }

func NewGetOpsInventory(r port.OpsReader) *GetOpsInventory { return &GetOpsInventory{r: r} }

func (u *GetOpsInventory) Execute(ctx context.Context, eventID string) ([]port.TierInventoryRow, error) {
	rows, err := u.r.TierInventory(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []port.TierInventoryRow{}
	}
	return rows, nil
}

// SalesTotals tinh tu cac dong tier cua CUNG snapshot.
type SalesTotals struct {
	GrossRevenueCents int64
	TicketsSold       int
	Quota             int
	PaidOrders        int
	ActiveHolds       int
}

// OrganizerSales la doanh so mot su kien kem chu su kien (de handler kiem quyen tren ban cache).
type OrganizerSales struct {
	OrganizerID string
	Totals      SalesTotals
	Tiers       []port.TierSalesRow
	Payments    []port.PaymentRow
}

// GetOrganizerSales: doanh so realtime cho organizer (contracts 4.2). KHONG kiem quyen: ket qua duoc cache
// theo su kien, nen handler so OrganizerID voi nguoi goi tren MOI request, ke ca khi trung cache.
type GetOrganizerSales struct{ r port.OpsReader }

func NewGetOrganizerSales(r port.OpsReader) *GetOrganizerSales { return &GetOrganizerSales{r: r} }

func (u *GetOrganizerSales) Execute(ctx context.Context, eventID string) (OrganizerSales, error) {
	rows, err := u.r.EventSales(ctx, eventID)
	if err != nil {
		return OrganizerSales{}, err
	}
	owner := rows.OrganizerID
	if owner == "" {
		if owner, err = u.r.EventOwner(ctx, eventID); err != nil {
			return OrganizerSales{}, err
		}
	}
	out := OrganizerSales{OrganizerID: owner, Tiers: rows.Tiers, Totals: SalesTotals{PaidOrders: rows.PaidOrders}}
	if out.Tiers == nil {
		out.Tiers = []port.TierSalesRow{}
	}
	for _, t := range out.Tiers {
		out.Totals.GrossRevenueCents += t.RevenueCents
		out.Totals.TicketsSold += t.Sold
		out.Totals.Quota += t.Quota
		out.Totals.ActiveHolds += t.ActiveHolds
	}
	out.Payments = reconcilePayments(rows.Payments, out.Totals)
	return out, nil
}

// reconcilePayments them nhom "unknown" cho phan don / doanh thu khong gan duoc payment SUCCEEDED nao, de
// SUM(orders) = paid_orders va SUM(amount_cents) = gross_revenue_cents; sap xep theo amount giam dan.
func reconcilePayments(in []port.PaymentRow, t SalesTotals) []port.PaymentRow {
	out := make([]port.PaymentRow, 0, len(in)+1)
	var orders int
	var amount int64
	for _, p := range in {
		orders += p.Orders
		amount += p.AmountCents
		out = append(out, p)
	}
	if rest, restAmount := t.PaidOrders-orders, t.GrossRevenueCents-amount; rest > 0 || restAmount > 0 {
		merged := false
		for i := range out {
			if out[i].Provider == UnknownProvider {
				out[i].Orders += rest
				out[i].AmountCents += restAmount
				merged = true
			}
		}
		if !merged {
			out = append(out, port.PaymentRow{Provider: UnknownProvider, Orders: rest, AmountCents: restAmount})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].AmountCents != out[j].AmountCents {
			return out[i].AmountCents > out[j].AmountCents
		}
		return out[i].Provider < out[j].Provider
	})
	return out
}
