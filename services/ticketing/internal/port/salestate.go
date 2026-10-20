package port

import "context"

// ===================== Tin hieu het ve (EV-146) =====================

// StockInfo la anh chup ton kho cua mot su kien doc tu Redis (duong nong).
type StockInfo struct {
	// Total la tong ve con cua cac hang ve, cong moi bucket.
	Total int
	// Seeded=false khi it nhat mot hang ve CHUA duoc nap vao Redis. Khi do Total=0 khong co nghia la het
	// ve: chi la chua nap. Noi dung Total phai kiem Seeded truoc.
	Seeded bool
	// ActiveHolds la so hold dang song (chua tra / chua dung). Hold het han se tra kho lai.
	ActiveHolds int
}

// StockReader doc ton kho Redis cua cac hang ve trong mot su kien.
type StockReader interface {
	Stock(ctx context.Context, eventID string, ticketTypeIDs []string) (StockInfo, error)
}

// SaleState la tin hieu ticketing bao cho phong cho (qua meta cua su kien trong Redis).
type SaleState struct {
	// StockZero: kho dang bang 0 nhung con hold co the tra lai. Phong cho ngung tha them nguoi, khach
	// khong thay doi trang thai.
	StockZero bool
	// SoldOut: het ve han (kho 0, khong con hold, on dinh). Khach chua duoc vao nhan SOLD_OUT.
	SoldOut bool
}

// SaleStateSink ghi tin hieu cho phong cho.
type SaleStateSink interface {
	SetSaleState(ctx context.Context, eventID string, s SaleState) error
}
