package port

import "time"

// SellableEvent la su kien ticketing phai phuc vu: dang ban, het ve tam thoi (con hold co the het
// han tra kho), hoac sap mo ban trong khoang nhin truoc.
type SellableEvent struct {
	ID          string
	SaleStartAt time.Time
}
