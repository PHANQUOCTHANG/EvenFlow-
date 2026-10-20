package authx

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Queue token (EV-137): chung minh "nguoi nay, tren thiet bi nay, duoc xep hang o su kien nay".
//
// Phong cho cap luc join; phong cho (status/stream) va ticketing (tao hold) cung xac minh bang
// cung mot secret. Token KHONG phai quyen vao mua ve: quyen do nam o ZSET `admitted` theo jti. Token
// chi rang buoc jti do voi dung nguoi, thiet bi va su kien, nen chia token cho tai khoan khac
// hay dung o su kien khac deu vo hieu.
const (
	queueTokenPrefix = "q1."

	// queueMACDomain tach HMAC cua queue token khoi HMAC cua QR ve dung chung secret.
	queueMACDomain = "eventflow.queue-token.v1\x00"
)

var (
	// ErrInvalidQueueToken gop moi loi xac minh (sai dinh dang, sai chu ky, thieu claim).
	ErrInvalidQueueToken = errors.New("authx: queue token khong hop le")
	// ErrExpiredQueueToken: token dung chu ky nhung het han; client nen join lai (idempotent theo BR-Q3).
	ErrExpiredQueueToken = errors.New("authx: queue token da het han")
)

// QueueClaims la noi dung queue token.
type QueueClaims struct {
	EventID   string    `json:"eid"`
	Subject   string    `json:"sub"` // identity_id
	DeviceID  string    `json:"did"` // rong neu phong cho khong bat buoc thiet bi
	JTI       string    `json:"jti"` // thanh vien trong hang cho / ZSET admitted
	IssuedAt  time.Time `json:"-"`
	ExpiresAt time.Time `json:"-"`
}

type queuePayload struct {
	QueueClaims
	IAT int64 `json:"iat"`
	EXP int64 `json:"exp"`
}

// SignQueueToken ky claims. IssuedAt va ExpiresAt bat buoc.
func SignQueueToken(secret []byte, c QueueClaims) (string, error) {
	if len(secret) < minSecretLen {
		return "", ErrInvalidSecret
	}
	if c.EventID == "" || c.Subject == "" || c.JTI == "" || c.IssuedAt.IsZero() || !c.ExpiresAt.After(c.IssuedAt) {
		return "", fmt.Errorf("authx: queue claims thieu eid/sub/jti hoac han khong hop le")
	}
	raw, err := json.Marshal(queuePayload{QueueClaims: c, IAT: c.IssuedAt.Unix(), EXP: c.ExpiresAt.Unix()})
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return queueTokenPrefix + payload + "." + base64.RawURLEncoding.EncodeToString(queueMAC(secret, payload)), nil
}

// VerifyQueueToken xac minh chu ky va han dung tai thoi diem now. Khong kiem eid/sub/did: noi goi
// phai doi chieu voi su kien va nguoi dang goi (xem QueueClaims.Matches).
func VerifyQueueToken(secret []byte, token string, now time.Time) (*QueueClaims, error) {
	if len(secret) < minSecretLen {
		return nil, ErrInvalidSecret
	}
	rest, ok := strings.CutPrefix(token, queueTokenPrefix)
	if !ok {
		return nil, ErrInvalidQueueToken
	}
	payload, sig, ok := strings.Cut(rest, ".")
	if !ok || payload == "" || sig == "" || strings.Contains(sig, ".") {
		return nil, ErrInvalidQueueToken
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(got, queueMAC(secret, payload)) {
		return nil, ErrInvalidQueueToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, ErrInvalidQueueToken
	}
	var p queuePayload
	if err := json.Unmarshal(raw, &p); err != nil || p.EventID == "" || p.Subject == "" || p.JTI == "" || p.EXP == 0 {
		return nil, ErrInvalidQueueToken
	}
	p.IssuedAt, p.ExpiresAt = time.Unix(p.IAT, 0), time.Unix(p.EXP, 0)
	if !now.Before(p.ExpiresAt) {
		return nil, ErrExpiredQueueToken
	}
	return &p.QueueClaims, nil
}

// Matches kiem token co thuoc ve dung su kien, dung nguoi va (neu token gan thiet bi) dung thiet bi
// dang goi khong. deviceID rong trong khi token co thiet bi thi KHONG khop.
func (c *QueueClaims) Matches(eventID, subject, deviceID string) bool {
	if c.EventID != eventID || c.Subject != subject {
		return false
	}
	return c.DeviceID == "" || c.DeviceID == deviceID
}

func queueMAC(secret []byte, payload string) []byte {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(queueMACDomain))
	m.Write([]byte(payload))
	return m.Sum(nil)
}
