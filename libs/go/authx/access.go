package authx

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Hop dong access token (EV-112). Identity ky, gateway xac minh: ca hai phai dung CUNG mot
// dinh nghia, nen no nam o day thay vi trong tung service.
const (
	// AccessTokenTTL la tuoi tho access token. Ngan de token bi lo chi dung duoc it phut;
	// phien dai han nam o refresh token (thu hoi duoc).
	AccessTokenTTL = 15 * time.Minute

	// AccessIssuer va AccessAudience co dinh: token ky cho muc dich khac (vd QR ve) khong
	// bao gio duoc chap nhan la access token.
	AccessIssuer   = "eventflow-identity"
	AccessAudience = "eventflow-api"

	// accessType tach access token khoi moi JWT khac co the ky cung secret.
	accessType = "access"

	// accessLeeway bu lech dong ho giua identity va gateway.
	accessLeeway = 30 * time.Second

	// minSecretLen giong nguong cua NewQREngine.
	minSecretLen = 16
)

var (
	// ErrInvalidAccessToken gop moi loi xac minh (sai chu ky, sai alg, sai issuer, thieu claim).
	// Khong tach nho cho client: ke tan cong khong can biet token hong o dau.
	ErrInvalidAccessToken = errors.New("authx: access token khong hop le")
	// ErrExpiredAccessToken tach rieng de client biet nen goi /auth/refresh thay vi dang nhap lai.
	ErrExpiredAccessToken = errors.New("authx: access token da het han")
)

// AccessClaims la phan danh tinh mang trong access token.
type AccessClaims struct {
	Subject   string    // identity_id (UUID)
	Role      string    // buyer | organizer | moderator | ops
	ID        string    // jti: duy nhat moi token, de truy vet
	Verified  bool      // ver: identity da xac thuc OTP/IdP (BR-Q3)
	DeviceID  string    // did: thiet bi cua phien (EV-113); rong neu phien chua gan thiet bi
	IssuedAt  time.Time // iat
	ExpiresAt time.Time // exp
}

type accessJWT struct {
	Role     string `json:"role"`
	Type     string `json:"typ"`
	Verified bool   `json:"ver,omitempty"`
	DeviceID string `json:"did,omitempty"`
	jwt.RegisteredClaims
}

// SignAccessToken ky access token HS256. ExpiresAt rong thi lay IssuedAt + AccessTokenTTL.
func SignAccessToken(secret []byte, c AccessClaims) (string, error) {
	if len(secret) < minSecretLen {
		return "", ErrInvalidSecret
	}
	if c.Subject == "" || c.Role == "" || c.ID == "" || c.IssuedAt.IsZero() {
		return "", fmt.Errorf("authx: access claims thieu sub/role/jti/iat")
	}
	exp := c.ExpiresAt
	if exp.IsZero() {
		exp = c.IssuedAt.Add(AccessTokenTTL)
	}
	claims := accessJWT{
		Role:     c.Role,
		Type:     accessType,
		Verified: c.Verified,
		DeviceID: c.DeviceID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    AccessIssuer,
			Subject:   c.Subject,
			Audience:  jwt.ClaimStrings{AccessAudience},
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(c.IssuedAt),
			ID:        c.ID,
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

// VerifyAccessToken xac minh chu ky, thuat toan (chi HS256), issuer, audience, typ va han dung
// tai thoi diem now. Tra ErrExpiredAccessToken neu chi sai moi han dung.
func VerifyAccessToken(secret []byte, token string, now time.Time) (*AccessClaims, error) {
	if len(secret) < minSecretLen {
		return nil, ErrInvalidSecret
	}
	var claims accessJWT
	parsed, err := jwt.ParseWithClaims(token, &claims,
		func(*jwt.Token) (any, error) { return secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(AccessIssuer),
		jwt.WithAudience(AccessAudience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(accessLeeway),
		jwt.WithTimeFunc(func() time.Time { return now }),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredAccessToken
		}
		return nil, ErrInvalidAccessToken
	}
	if !parsed.Valid || claims.Type != accessType || claims.Subject == "" || claims.Role == "" {
		return nil, ErrInvalidAccessToken
	}
	out := &AccessClaims{
		Subject:   claims.Subject,
		Role:      claims.Role,
		ID:        claims.ID,
		Verified:  claims.Verified,
		DeviceID:  claims.DeviceID,
		ExpiresAt: claims.ExpiresAt.Time,
	}
	if claims.IssuedAt != nil {
		out.IssuedAt = claims.IssuedAt.Time
	}
	return out, nil
}
