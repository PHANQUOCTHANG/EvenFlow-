package app

import (
	"context"

	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// GetPurchaseLimit tra han muc mua cua mot khach (BR-O4) cho trang chon ve.
//
// UI KHONG duoc tu doan so luong toi da: "mac dinh 4" la default cua he thong, con so ve da mua
// phu thuoc lich su ma UI khong biet. Day chi la goi y hien thi -- lop chan that nam o CreateHold
// va trong transaction cua PersistHold.
type GetPurchaseLimit struct {
	src port.PurchaseLimitSource
}

func NewGetPurchaseLimit(src port.PurchaseLimitSource) *GetPurchaseLimit {
	return &GetPurchaseLimit{src: src}
}

func (uc *GetPurchaseLimit) Execute(ctx context.Context, eventID, identityID string) (port.PurchaseLimit, error) {
	l, err := uc.src.GetPurchaseLimit(ctx, eventID, identityID)
	if err != nil {
		return port.PurchaseLimit{}, err
	}
	l.MaxSelectable = domain.MaxSelectable(l.MaxPerOrder, l.MaxPerIdentity, l.Used)
	if l.HasActiveHold {
		// BR-O3: dang co hold thi moi lan dat giu chi nhan lai hold do, khong tao don moi.
		// Cho chon so luong luc nay la hua mot dieu he thong se khong lam.
		l.MaxSelectable = 0
	}
	return l, nil
}
