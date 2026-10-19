// Package authx cung cap cac tien ich ky so va xac thuc cho he thong EvenFlow,
// bao gom HMAC-SHA256 token va QR code bao mat (BR-O8).
package authx

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrInvalidSecret bao khi secret key khong hop le (rong hoac qua ngan).
	ErrInvalidSecret = errors.New("authx: secret key khong hop le (phai co it nhat 16 bytes)")
	// ErrInvalidFormat bao khi chuoi QR khong dung dinh dang <payload>.<signature>.
	ErrInvalidFormat = errors.New("authx: dinh dang ma QR khong hop le")
	// ErrTamperedQR bao khi chu ky HMAC khong khop hoac du lieu bi chinh sua (AC-4).
	ErrTamperedQR = errors.New("authx: chu ky QR khong hop le (du lieu bi sua doi hoac chu ky gia mao)")
	// ErrExpiredQR bao khi ma QR da qua han su dung (AC-4).
	ErrExpiredQR = errors.New("authx: ma QR da qua han su dung")
	// ErrMissingClaims bao khi thieu cac truong bat buoc nhu tid, oid, eid hoac jti.
	ErrMissingClaims = errors.New("authx: thong tin ma QR thieu truong bat buoc")
)

// TicketClaims chua thong tin duoc ma hoa ben trong QR payload (BR-O8).
type TicketClaims struct {
	TicketID     string `json:"tid"`           // UUID cua ve
	OrderID      string `json:"oid"`           // UUID cua don hang
	EventID      string `json:"eid"`           // UUID cua su kien
	TicketTypeID string `json:"tty"`           // UUID cua hang ve
	IdentityID   string `json:"sub,omitempty"` // UUID cua chu ve (neu co)
	JTI          string `json:"jti"`           // Unique Token ID chong replay
	IssuedAt     int64  `json:"iat"`           // Unix timestamp luc phat hanh
	ExpiresAt    int64  `json:"exp,omitempty"` // Unix timestamp het han vao cua (0 neu khong gioi han)
}

// QREngine chiu trach nhiem ky va xac thuc chu ky HMAC-SHA256 cho QR code.
type QREngine struct {
	secret []byte
	clock  func() time.Time
}

// NewQREngine khoi tao QREngine voi secret key toi thieu 16 bytes.
func NewQREngine(secret []byte) (*QREngine, error) {
	if len(secret) < 16 {
		return nil, ErrInvalidSecret
	}
	secCopy := make([]byte, len(secret))
	copy(secCopy, secret)
	return &QREngine{
		secret: secCopy,
		clock:  time.Now,
	}, nil
}

// WithClock cho phep ghi de ham dong ho de ho tro kiem thu thoi gian het han.
func (e *QREngine) WithClock(clock func() time.Time) *QREngine {
	if clock != nil {
		e.clock = clock
	}
	return e
}

// Sign dong goi TicketClaims thanh chuoi base64url va tao chu ky HMAC-SHA256.
// Ket qua tra ve:
// - qrPayload: chuoi gom `<encodedPayload>.<encodedSignature>` dung lam noi dung QR.
// - signature: chuoi chu ky dang base64url dung de luu vao cot qr_sig cua DB.
func (e *QREngine) Sign(claims TicketClaims) (qrPayload string, signature string, err error) {
	if claims.TicketID == "" || claims.OrderID == "" || claims.EventID == "" || claims.JTI == "" {
		return "", "", ErrMissingClaims
	}
	if claims.IssuedAt == 0 {
		claims.IssuedAt = e.clock().Unix()
	}

	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", "", fmt.Errorf("authx: marshal claims that bai: %w", err)
	}

	encodedPayload := base64.RawURLEncoding.EncodeToString(payloadBytes)

	// Tinh toan HMAC-SHA256
	mac := hmac.New(sha256.New, e.secret)
	mac.Write([]byte(encodedPayload))
	sigBytes := mac.Sum(nil)

	encodedSig := base64.RawURLEncoding.EncodeToString(sigBytes)
	fullQR := encodedPayload + "." + encodedSig

	return fullQR, encodedSig, nil
}

// Verify giai ma chuoi QR va kiem tra tinh toan ven cua chu ky HMAC (AC-4, BR-O8).
// Su dung subtle.ConstantTimeCompare de chong timing attacks.
func (e *QREngine) Verify(qrPayload string) (TicketClaims, error) {
	parts := strings.Split(qrPayload, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return TicketClaims{}, ErrInvalidFormat
	}

	encodedPayload := parts[0]
	encodedSig := parts[1]

	sigBytes, err := base64.RawURLEncoding.DecodeString(encodedSig)
	if err != nil {
		return TicketClaims{}, ErrTamperedQR
	}

	// Tinh toan chu ky ky vong
	mac := hmac.New(sha256.New, e.secret)
	mac.Write([]byte(encodedPayload))
	expectedSig := mac.Sum(nil)

	// So sanh bang constant time chong timing attacks
	if subtle.ConstantTimeCompare(sigBytes, expectedSig) != 1 {
		return TicketClaims{}, ErrTamperedQR
	}

	// Giai ma payload
	payloadBytes, err := base64.RawURLEncoding.DecodeString(encodedPayload)
	if err != nil {
		return TicketClaims{}, ErrTamperedQR
	}

	var claims TicketClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return TicketClaims{}, fmt.Errorf("%w: parse json that bai", ErrTamperedQR)
	}

	if claims.TicketID == "" || claims.OrderID == "" || claims.EventID == "" || claims.JTI == "" {
		return TicketClaims{}, ErrMissingClaims
	}

	// Kiem tra han su dung
	if claims.ExpiresAt > 0 && e.clock().Unix() > claims.ExpiresAt {
		return claims, ErrExpiredQR
	}

	return claims, nil
}
