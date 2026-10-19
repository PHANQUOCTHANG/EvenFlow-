package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/google/uuid"
)

// ActionRefreshTokenReuse ghi vao audit_logs khi mot refresh token da xoay bi dung lai.
const ActionRefreshTokenReuse = "REFRESH_TOKEN_REUSE"

// SessionService cap, xoay va thu hoi phien dang nhap (EV-112).
//
//   - Access token: JWT HS256 15 phut (authx), gateway tu xac minh, khong goi ve identity.
//   - Refresh token: chuoi ngau nhien 256 bit, chi luu hash, xoay vong moi lan dung, song
//     toi da 30 ngay ke tu luc dang nhap. Dung lai token cu -> thu hoi ca ho.
//
// Thu hoi co hieu luc voi refresh ngay lap tuc; access token da cap van dung duoc toi khi
// het han (<= 15 phut). Do la danh doi co chu dich: gateway khong phai tra DB moi request.
type SessionService struct {
	secret     []byte
	identities domain.IdentityRepository
	tokens     domain.RefreshTokenRepository
	audit      domain.AuditRepository
	devices    domain.DeviceRegistrar
	now        func() time.Time
}

// WithDevices cho phep Refresh gan / doi thiet bi cua phien (EV-113).
func (s *SessionService) WithDevices(d domain.DeviceRegistrar) *SessionService {
	s.devices = d
	return s
}

// NewSessionService tao service. audit co the nil (khong ghi audit khi phat hien tai su dung).
func NewSessionService(secret []byte, identities domain.IdentityRepository, tokens domain.RefreshTokenRepository, audit domain.AuditRepository) *SessionService {
	return &SessionService{
		secret:     secret,
		identities: identities,
		tokens:     tokens,
		audit:      audit,
		now:        time.Now,
	}
}

// WithClock thay dong ho (cho test).
func (s *SessionService) WithClock(now func() time.Time) *SessionService {
	s.now = now
	return s
}

// GenerateTokens mo mot phien moi (mot ho refresh token moi) cho identity vua xac thuc.
func (s *SessionService) GenerateTokens(ctx context.Context, identity *domain.Identity) (*domain.TokenPair, error) {
	now := s.now()
	raw, hash, err := domain.NewRefreshTokenValue()
	if err != nil {
		return nil, fmt.Errorf("sinh refresh token: %w", err)
	}
	var deviceID *uuid.UUID
	if id, ok := domain.DeviceFromContext(ctx); ok {
		deviceID = &id
	}
	rt := &domain.RefreshToken{
		DeviceID:   deviceID,
		ID:         uuid.New(),
		IdentityID: identity.ID,
		FamilyID:   uuid.New(),
		TokenHash:  hash,
		ExpiresAt:  now.Add(domain.RefreshTokenTTL),
		CreatedAt:  now,
	}
	if err := s.tokens.Create(ctx, rt); err != nil {
		return nil, fmt.Errorf("luu refresh token: %w", err)
	}
	return s.pair(identity, raw, rt.ExpiresAt, deviceID, now)
}

// Refresh doi refresh token lay cap token moi. Role doc lai tu DB nen doi role co hieu luc
// tu lan refresh ke tiep. Phien giu thiet bi cu; neu client gui device thi dang ky (hoac doi) thiet bi
// cua phien sang thiet bi do -- cach de client Google/VNeID gan thiet bi sau khi dang nhap.
func (s *SessionService) Refresh(ctx context.Context, rawRefresh string, device *domain.DeviceInfo) (*domain.TokenPair, *domain.Identity, error) {
	if rawRefresh == "" {
		return nil, nil, domain.ErrRefreshTokenInvalid
	}
	now := s.now()
	nextRaw, nextHash, err := domain.NewRefreshTokenValue()
	if err != nil {
		return nil, nil, fmt.Errorf("sinh refresh token: %w", err)
	}
	next := &domain.RefreshToken{ID: uuid.New(), TokenHash: nextHash, CreatedAt: now}

	old, err := s.tokens.Rotate(ctx, domain.HashRefreshToken(rawRefresh), next, now)
	if errors.Is(err, domain.ErrRefreshTokenReused) {
		s.auditReuse(ctx, old, now)
		return nil, nil, err
	}
	if err != nil {
		return nil, nil, err
	}

	identity, err := s.identities.FindByID(ctx, old.IdentityID)
	if err != nil {
		if errors.Is(err, domain.ErrIdentityNotFound) {
			return nil, nil, domain.ErrRefreshTokenInvalid
		}
		return nil, nil, err
	}
	deviceID := old.DeviceID
	if device != nil {
		if s.devices == nil {
			return nil, nil, errors.New("khong cau hinh dang ky thiet bi")
		}
		id, err := s.devices.Register(ctx, identity.ID, *device)
		if err != nil {
			return nil, nil, err
		}
		if deviceID == nil || *deviceID != id {
			if err := s.tokens.SetDevice(ctx, next.ID, id); err != nil {
				return nil, nil, fmt.Errorf("gan thiet bi cho phien: %w", err)
			}
			deviceID = &id
		}
	}
	pair, err := s.pair(identity, nextRaw, next.ExpiresAt, deviceID, now)
	if err != nil {
		return nil, nil, err
	}
	return pair, identity, nil
}

// Logout thu hoi ca ho chua refresh token nay. Token la/khong ton tai cung tra nil: logout
// la idempotent va khong cho ke do tim biet token nao hop le.
func (s *SessionService) Logout(ctx context.Context, rawRefresh string) error {
	if rawRefresh == "" {
		return nil
	}
	return s.tokens.RevokeFamilyByHash(ctx, domain.HashRefreshToken(rawRefresh), domain.RevokeReasonLogout, s.now())
}

// LogoutAll thu hoi moi phien cua identity (dang xuat moi thiet bi).
func (s *SessionService) LogoutAll(ctx context.Context, identityID uuid.UUID) error {
	return s.tokens.RevokeAllForIdentity(ctx, identityID, domain.RevokeReasonLogoutAll, s.now())
}

func (s *SessionService) pair(identity *domain.Identity, rawRefresh string, refreshExp time.Time, deviceID *uuid.UUID, now time.Time) (*domain.TokenPair, error) {
	did := ""
	if deviceID != nil {
		did = deviceID.String()
	}
	access, err := authx.SignAccessToken(s.secret, authx.AccessClaims{
		DeviceID: did,
		Subject:  identity.ID.String(),
		Role:     string(identity.Role),
		ID:       uuid.NewString(),
		Verified: identity.OtpVerifiedAt != nil,
		IssuedAt: now,
	})
	if err != nil {
		return nil, fmt.Errorf("ky access token: %w", err)
	}
	return &domain.TokenPair{
		AccessToken:      access,
		AccessExpiresAt:  now.Add(authx.AccessTokenTTL),
		RefreshToken:     rawRefresh,
		RefreshExpiresAt: refreshExp,
	}, nil
}

func (s *SessionService) auditReuse(ctx context.Context, old *domain.RefreshToken, now time.Time) {
	if s.audit == nil || old == nil {
		return
	}
	_ = s.audit.Save(ctx, &domain.AuditLog{
		ActorType: domain.ActorTypeSystem,
		ActorID:   old.IdentityID.String(),
		Action:    ActionRefreshTokenReuse,
		Subject:   "refresh_token_family:" + old.FamilyID.String(),
		Reason:    "Refresh token da xoay bi dung lai; thu hoi ca ho token",
		Metadata: map[string]any{
			"identity_id": old.IdentityID.String(),
			"family_id":   old.FamilyID.String(),
			"token_id":    old.ID.String(),
		},
		CreatedAt: now,
	})
}
