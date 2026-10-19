package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// ===================== Xem don hang =====================

// OrderTicketLister doc ve da phat hanh cua don (port.TicketRepository thoa man).
type OrderTicketLister interface {
	GetTicketsByOrderID(ctx context.Context, orderID string) ([]domain.Ticket, error)
}

// OrderDetails la don hang kem ve da phat hanh.
type OrderDetails struct {
	Order   port.OrderView
	Tickets []domain.Ticket
}

// GetOrder tra don cua khach, kem ve khi don da phat hanh.
type GetOrder struct {
	orders  port.OrderReader
	tickets OrderTicketLister
}

func NewGetOrder(orders port.OrderReader, tickets OrderTicketLister) *GetOrder {
	return &GetOrder{orders: orders, tickets: tickets}
}

func (uc *GetOrder) Execute(ctx context.Context, orderID, identityID string) (OrderDetails, error) {
	order, err := uc.orders.GetOrder(ctx, orderID, identityID)
	if err != nil {
		return OrderDetails{}, err
	}
	d := OrderDetails{Order: order}
	if order.Status == "ISSUED" && uc.tickets != nil {
		if d.Tickets, err = uc.tickets.GetTicketsByOrderID(ctx, orderID); err != nil {
			return OrderDetails{}, fmt.Errorf("doc ve cua don: %w", err)
		}
	}
	return d, nil
}

// ===================== Huy hold =====================

// HoldReleaser tra hold ve Redis. Idempotent (port.InventoryGate thoa man qua Release).
type HoldReleaser interface {
	Release(ctx context.Context, eventID, holdID string) error
}

// CancelHold cho khach tu huy giu ghe.
//
// Thu tu nhu hold sweeper: Postgres (nguon su that) truoc, Redis sau. Redis duoc tra LUON ca khi
// Postgres bao "da huy truoc do": lan huy truoc co the chet giua hai buoc, va tra Redis idempotent
// nen goi lai khong sai. Redis loi -> tra loi de client thu lai; job doi soat cung se sua.
type CancelHold struct {
	repo port.HoldCanceller
	gate HoldReleaser
	log  *slog.Logger
}

func NewCancelHold(repo port.HoldCanceller, gate HoldReleaser, log *slog.Logger) *CancelHold {
	return &CancelHold{repo: repo, gate: gate, log: log}
}

func (uc *CancelHold) Execute(ctx context.Context, holdID, identityID string) error {
	res, err := uc.repo.CancelHold(ctx, holdID, identityID)
	if err != nil {
		return err
	}
	if err := uc.gate.Release(ctx, res.EventID, redisHoldID(holdID)); err != nil {
		uc.log.Error("tra kho redis sau khi huy hold that bai", "hold", holdID, "err", err)
		return fmt.Errorf("tra kho redis: %w", err)
	}
	return nil
}

// redisHoldID doi id hold client giu (UUID co gach ngang, nhu Postgres tra ve) sang dang Redis luu:
// hex 32 ky tu viet thuong. Thieu buoc nay thi Release khong tim thay hold trong Redis, kho Redis
// khong duoc tra va hai tang lech nhau.
func redisHoldID(id string) string {
	return strings.ToLower(strings.ReplaceAll(id, "-", ""))
}

// ===================== Ton kho cong khai =====================

// TierAvailability la muc con ve cong khai cua mot hang ve.
type TierAvailability struct {
	TicketTypeID string                   `json:"ticket_type_id"`
	Name         string                   `json:"name"`
	Availability domain.AvailabilityLevel `json:"availability"`
}

// GetAvailability tra muc con ve (khong lo so chinh xac) cua cac hang ve trong su kien.
type GetAvailability struct {
	src port.AvailabilitySource
}

func NewGetAvailability(src port.AvailabilitySource) *GetAvailability {
	return &GetAvailability{src: src}
}

func (uc *GetAvailability) Execute(ctx context.Context, eventID string) ([]TierAvailability, error) {
	tiers, err := uc.src.ListTiers(ctx, eventID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(tiers))
	for i, t := range tiers {
		ids[i] = t.TicketTypeID
	}
	remaining, err := uc.src.Remaining(ctx, eventID, ids)
	if err != nil {
		return nil, fmt.Errorf("doc ton kho redis: %w", err)
	}
	out := make([]TierAvailability, len(tiers))
	for i, t := range tiers {
		out[i] = TierAvailability{
			TicketTypeID: t.TicketTypeID,
			Name:         t.Name,
			Availability: domain.LevelOf(remaining[t.TicketTypeID], t.Capacity),
		}
	}
	return out, nil
}
