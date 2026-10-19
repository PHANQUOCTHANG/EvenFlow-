// Package domain chua cac entity, value object va loi nghiep vu thuan tuy cua payment-svc.
//
// Quyen tac kien truc bat bien: domain KHONG import bat ky package ngoai nao
// (khong database/sql, khong net/http, khong redis, khong third-party SDK).
package domain

import (
	"errors"
	"fmt"
)

var (
	// ErrHoldExpired tra ve khi hold/don hang da qua thoi han expires_at (BR-O2).
	ErrHoldExpired = errors.New("phiên giữ vé đã hết hạn (BR-O2)")

	// ErrOrderNotFound tra ve khi ma don hang khong ton tai.
	ErrOrderNotFound = errors.New("không tìm thấy đơn hàng")

	// ErrOrderNotHeld tra ve khi don hang khong o trang thai HELD (khong the thanh toan).
	ErrOrderNotHeld = errors.New("đơn hàng không ở trạng thái giữ vé (HELD)")

	// ErrAmountMismatch tra ve khi so tien yeu cau thanh toan khong khop voi tong tien don hang.
	ErrAmountMismatch = errors.New("số tiền thanh toán không khớp với giá trị đơn hàng")

	// ErrCurrencyMismatch tra ve khi don vi tien te khong khop voi don hang.
	ErrCurrencyMismatch = errors.New("đơn vị tiền tệ không khớp")

	// ErrUnsupportedProvider tra ve khi cong thanh toan khong ton tai trong he thong.
	ErrUnsupportedProvider = errors.New("cổng thanh toán không được hỗ trợ")

	// ErrProviderDisabled tra ve khi cong thanh toan dang bi tam khoa trong cau hinh.
	ErrProviderDisabled = errors.New("cổng thanh toán hiện đang tạm khóa")

	// ErrInvalidInput tra ve khi tham so truyen vao thieu hoac sai dinh dang.
	ErrInvalidInput = errors.New("dữ liệu thanh toán không hợp lệ")

	// ErrPaymentAlreadyProcessed tra ve khi giao dich da hoan tat.
	ErrPaymentAlreadyProcessed = errors.New("giao dịch thanh toán đã được xử lý")

	// ErrGatewayCommunication tra ve khi giao tiep voi cong thanh toan that bai.
	ErrGatewayCommunication = errors.New("lỗi giao tiếp với cổng thanh toán")

	// ErrInvalidSignature tra ve khi chu ky webhook khong hop le hoac bi gia mao.
	ErrInvalidSignature = errors.New("chữ ký số của cổng thanh toán không hợp lệ")

	// ErrReplayDetected tra ve khi timestamp webhook vuot qua cua so thoi gian cho phep (chong replay attack).
	ErrReplayDetected = errors.New("webhook quá cửa sổ thời gian cho phép (phát hiện replay attack)")

	// ErrDuplicateWebhook tra ve khi webhook da duoc xu ly truoc do (BR-O6).
	ErrDuplicateWebhook = errors.New("webhook trùng lặp đã được xử lý (BR-O6)")

	// ErrOrderNotRefundable tra ve khi don hang khong o trang thai co the hoan tien (PAID/ISSUED/REFUNDING).
	ErrOrderNotRefundable = errors.New("đơn hàng không ở trạng thái có thể hoàn tiền")

	// ErrRefundFailed tra ve khi giao dich hoan tien qua cong thanh toan that bai.
	ErrRefundFailed = errors.New("hoàn tiền qua cổng thanh toán thất bại")

	// ErrAlreadyRefunded tra ve khi don hang da duoc hoan tien truoc do.
	ErrAlreadyRefunded = errors.New("đơn hàng đã được hoàn tiền trước đó")
)

// ValidationError mo ta chi tiet loi xac thuc tham so.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("trường %s: %s", e.Field, e.Message)
}
