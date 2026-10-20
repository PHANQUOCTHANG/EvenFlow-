package app

import (
	"context"
	"strings"

	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/google/uuid"
)

const (
	minFingerprintLen = 8   // vd. hash 32 ky tu; chuoi qua ngan la gia mao
	maxFingerprintLen = 256 // chan payload lon lam day cot
)

// DeviceService dang ky thiet bi cua identity (EV-113).
type DeviceService struct {
	repo domain.DeviceRepository
}

func NewDeviceService(repo domain.DeviceRepository) *DeviceService {
	return &DeviceService{repo: repo}
}

// Register kiem tra fingerprint roi gan thiet bi voi identity. Idempotent.
func (s *DeviceService) Register(ctx context.Context, identityID uuid.UUID, d domain.DeviceInfo) (uuid.UUID, error) {
	fp := strings.TrimSpace(d.Fingerprint)
	if len(fp) < minFingerprintLen || len(fp) > maxFingerprintLen {
		return uuid.Nil, domain.ErrInvalidFingerprint
	}
	return s.repo.Register(ctx, identityID, fp, d.IP, domain.MaxDevicesPerIdentity)
}
