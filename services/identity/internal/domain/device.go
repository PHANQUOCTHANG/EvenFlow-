package domain

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// MaxDevicesPerIdentity la so thiet bi toi da gan voi mot identity (EV-113). Vuot thi tu choi:
// mot tai khoan dung tren hang chuc thiet bi la tin hieu bot / chia se tai khoan.
const MaxDevicesPerIdentity = 5

var (
	// ErrTooManyDevices: identity da du MaxDevicesPerIdentity va day la thiet bi moi.
	ErrTooManyDevices = errors.New("identity da gan qua nhieu thiet bi")
	// ErrInvalidFingerprint: fingerprint rong hoac qua ngan/dai.
	ErrInvalidFingerprint = errors.New("device fingerprint khong hop le")
)

// DeviceInfo la thong tin thiet bi client gui kem khi dang nhap / refresh.
type DeviceInfo struct {
	Fingerprint string
	IP          string // IP ket noi goc (gateway dat X-Forwarded-For); rong neu khong biet
}

// DeviceRepository luu lien ket identity <-> thiet bi trong bang devices.
type DeviceRepository interface {
	// Register tra id thiet bi cua (identity, fingerprint), tao moi neu chua co. Idempotent; cap nhat
	// last_seen/last_ip. Thiet bi moi vuot max -> ErrTooManyDevices. Kiem tra gioi han khong bi dua
	// dong thoi nho khoa theo identity.
	Register(ctx context.Context, identityID uuid.UUID, fingerprint, ip string, max int) (uuid.UUID, error)
}

// DeviceRegistrar la phan use case dang nhap can: dang ky thiet bi cua identity vua xac thuc.
type DeviceRegistrar interface {
	Register(ctx context.Context, identityID uuid.UUID, d DeviceInfo) (uuid.UUID, error)
}

type deviceCtxKey struct{}

// ContextWithDevice gan thiet bi cua phien vao ctx de TokenGenerator dua vao claim "did".
func ContextWithDevice(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, deviceCtxKey{}, id)
}

// DeviceFromContext doc thiet bi da gan bang ContextWithDevice.
func DeviceFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(deviceCtxKey{}).(uuid.UUID)
	return id, ok
}
