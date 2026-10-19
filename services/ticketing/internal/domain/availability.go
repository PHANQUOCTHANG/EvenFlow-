package domain

import "errors"

// ErrHoldNotCancellable: hold da gan voi don dang thanh toan / da thanh toan, khach khong tu huy duoc.
var ErrHoldNotCancellable = errors.New("lenh giu ghe khong the huy o trang thai hien tai")

// AvailabilityLevel la muc con ve cong khai cua mot hang ve. Cong khai cho moi nguoi nen KHONG lo so
// ve chinh xac: bot khong do duoc nhip ban, va khong cho phep suy ra thong luong tu so lieu nay.
type AvailabilityLevel string

const (
	AvailabilityAvailable AvailabilityLevel = "AVAILABLE"
	AvailabilityFewLeft   AvailabilityLevel = "FEW_LEFT"
	AvailabilitySoldOut   AvailabilityLevel = "SOLD_OUT"
)

// fewLeftPercent: con tu 10% suc chua tro xuong thi bao "sap het".
const fewLeftPercent = 10

// LevelOf doi so ve con (available) tren suc chua (capacity) thanh muc cong khai. available am hoac
// capacity <= 0 duoc coi la het (tin phia an toan).
func LevelOf(available, capacity int) AvailabilityLevel {
	switch {
	case available <= 0 || capacity <= 0:
		return AvailabilitySoldOut
	case available*100 <= capacity*fewLeftPercent:
		return AvailabilityFewLeft
	default:
		return AvailabilityAvailable
	}
}
