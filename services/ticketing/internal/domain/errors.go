// Package domain chua nghiep vu thuan tuy cua ticketing.
package domain

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
)

var (
	// ErrSoldOut: het ve that su (hoac Redis bao het -- tin phia an toan).
	ErrSoldOut = errors.New("het ve")

	// ErrInventoryExhausted: Postgres tu choi vi ton kho khong du. Phan biet voi
	// ErrSoldOut de biet Redis va Postgres dang lech nhau.
	ErrInventoryExhausted = errors.New("ton kho khong du trong co so du lieu")

	// ErrNotAdmitted: chua duoc phong cho tha vao (BR-Q7).
	ErrNotAdmitted = errors.New("chua duoc vao mua ve")

	ErrQuantityNotAllowed = errors.New("so luong khong hop le")   // BR-O4
	ErrPerIdentityLimit   = errors.New("vuot gioi han moi nguoi") // BR-O4
	ErrHoldExpired        = errors.New("lenh giu ghe da het han") // BR-O2
	ErrOrderNotFound      = errors.New("khong tim thay don hang")
	ErrEventNotFound      = errors.New("khong tim thay su kien dang ban")

	// Cac loi ve phat hanh va xac thuc ve (EV-133, BR-O8)
	ErrOrderNotPaid        = errors.New("don hang chua duoc thanh toan de phat hanh ve")
	ErrTicketNotFound      = errors.New("khong tim thay ve")
	ErrTamperedQR          = errors.New("ma QR gia mao hoac bi sua doi")
	ErrTicketAlreadyUsed   = errors.New("ve da duoc su dung truoc do")
	ErrTicketExpired       = errors.New("ve da qua han su dung")
	ErrTicketVoid          = errors.New("ve da bi huy hoac vo hieu hoa")
	ErrTicketRefunded      = errors.New("ve da duoc hoan tien")
	ErrInvalidTicketStatus = errors.New("trang thai ve khong hop le")
)

// NewID sinh dinh danh ngau nhien 128-bit.
func NewID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("khong doc duoc nguon ngau nhien: " + err.Error())
	}
	return hex.EncodeToString(b)
}
