package domain

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
)

// RefreshTokenTTL la tuoi tho toi da cua mot phien (EV-112). Moi lan refresh cap token moi
// nhung KHONG keo dai han cua ca ho token: phien het han sau 30 ngay ke tu luc dang nhap.
const RefreshTokenTTL = 30 * 24 * time.Hour

var (
	// ErrRefreshTokenInvalid: token khong ton tai, het han, hoac da bi thu hoi (logout).
	ErrRefreshTokenInvalid = errors.New("refresh token khong hop le hoac da het han")
	// ErrRefreshTokenReused: token DA DUOC XOAY roi lai duoc dung lan nua. Day la dau hieu token
	// bi danh cap: ca ho token bi thu hoi, nguoi dung phai dang nhap lai.
	ErrRefreshTokenReused = errors.New("refresh token da duoc su dung, phien bi thu hoi")
)

// Ly do thu hoi, ghi vao refresh_tokens.revoked_reason.
const (
	RevokeReasonRotated   = "rotated"
	RevokeReasonReuse     = "reuse_detected"
	RevokeReasonLogout    = "logout"
	RevokeReasonLogoutAll = "logout_all"
	RevokeReasonPassword  = "password_reset"
)

// RefreshToken la mot ban ghi trong bang refresh_tokens. Chi luu HASH cua token: lo bang
// cung khong dung duoc token.
type RefreshToken struct {
	ID         uuid.UUID
	IdentityID uuid.UUID
	FamilyID   uuid.UUID // moi lan dang nhap mot ho; xoay vong giu nguyen ho
	TokenHash  string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	RevokedAt  *time.Time
}

// TokenPair la ket qua mot lan dang nhap hoac refresh.
type TokenPair struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
}

// RefreshTokenRepository luu va xoay refresh token. Rotate phai nguyen tu: hai request dung
// cung mot token cung luc thi dung mot request thang, request kia thay ErrRefreshTokenReused.
type RefreshTokenRepository interface {
	Create(ctx context.Context, t *RefreshToken) error
	// Rotate khoa ban ghi co hash oldHash, thu hoi no va chen next cung ho, cung identity.
	// Tra ve ban ghi cu. Token da thu hoi -> thu hoi ca ho va tra ErrRefreshTokenReused;
	// khong ton tai hoac het han -> ErrRefreshTokenInvalid.
	Rotate(ctx context.Context, oldHash string, next *RefreshToken, now time.Time) (*RefreshToken, error)
	// RevokeFamilyByHash thu hoi ca ho chua token co hash nay (logout). Khong thay thi khong loi.
	RevokeFamilyByHash(ctx context.Context, hash, reason string, now time.Time) error
	// RevokeAllForIdentity thu hoi moi phien con hieu luc cua identity.
	RevokeAllForIdentity(ctx context.Context, identityID uuid.UUID, reason string, now time.Time) error
}

// NewRefreshTokenValue sinh refresh token ngau nhien 256 bit (base64url), tra ca gia tri va hash.
func NewRefreshTokenValue() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, HashRefreshToken(raw), nil
}

// HashRefreshToken bam SHA-256. Token da co 256 bit ngau nhien nen khong can salt/argon2.
func HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// TokenGenerator cap cap token cho mot identity vua xac thuc xong (mat khau, Google, VNeID).
// Moi lan goi mo mot phien (ho refresh token) moi.
type TokenGenerator interface {
	GenerateTokens(ctx context.Context, identity *Identity) (*TokenPair, error)
}

// SessionRevoker thu hoi moi phien cua mot identity (vd sau khi doi mat khau).
type SessionRevoker interface {
	RevokeAllForIdentity(ctx context.Context, identityID uuid.UUID, reason string, now time.Time) error
}
