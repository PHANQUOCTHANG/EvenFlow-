package domain

import "errors"

var (
	// ErrInvalidRecipient bao loi khi dia chi email hoac so dien thoai nguoi nhan khong hop le.
	ErrInvalidRecipient = errors.New("domain: invalid recipient email or phone")

	// ErrEmptyContent bao loi khi noi dung tieu de hoac than tin nhan bi trong.
	ErrEmptyContent = errors.New("domain: empty notification subject or body")

	// ErrNotificationNotFound bao loi khi khong tim thay ban ghi thong bao.
	ErrNotificationNotFound = errors.New("domain: notification not found")

	// ErrDuplicateNotification bao loi khi thong bao da duoc xu ly truoc do.
	ErrDuplicateNotification = errors.New("domain: duplicate notification already processed")

	// ErrSenderFailed bao loi khi adapter gui email/sms gap su co.
	ErrSenderFailed = errors.New("domain: notification sender dispatch failed")

	// ErrInvalidEventPayload bao loi khi du lieu event payload bi thieu truong bat buoc.
	ErrInvalidEventPayload = errors.New("domain: invalid event payload")
)
