// Package pii bao ve du lieu ca nhan (EVF-15 / EV-115): ma hoa cot, bam co khoa de tra cuu, va
// loc PII khoi log.
package pii

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// keyVersion la byte dau cua moi ban ma. Doi khoa thi tang version va giu khoa cu de giai ma
// du lieu cu; chua can nen moi co mot khoa.
const keyVersion byte = 1

var (
	// ErrInvalidKey: khoa khong phai base64 cua dung 32 byte.
	ErrInvalidKey = errors.New("pii: khoa phai la base64 cua 32 byte")
	// ErrDecrypt gop moi loi giai ma (sai khoa, sai AAD, ban ma bi sua, sai version).
	ErrDecrypt = errors.New("pii: khong giai ma duoc")
)

// Protector ma hoa (AES-256-GCM) va bam co khoa (HMAC-SHA256) gia tri PII.
//
//   - Encrypt: de doc lai (vd gui SMS). Moi lan ma hoa mot nonce ngau nhien nen cung gia tri
//     cho ra ban ma khac nhau; khong dung de tra cuu.
//   - Hash: de tra cuu va rang buoc UNIQUE. Co khoa (khac SHA-256 tran) nen lo DB khong do nguoc
//     duoc so dien thoai bang cach bam het ~10^9 so.
type Protector struct {
	aead    cipher.AEAD
	hashKey []byte
}

// NewProtector tao Protector tu hai khoa base64 (32 byte moi khoa). Hai khoa phai khac nhau.
func NewProtector(encKeyB64, hashKeyB64 string) (*Protector, error) {
	encKey, err := decodeKey(encKeyB64)
	if err != nil {
		return nil, fmt.Errorf("khoa ma hoa: %w", err)
	}
	hashKey, err := decodeKey(hashKeyB64)
	if err != nil {
		return nil, fmt.Errorf("khoa bam: %w", err)
	}
	if hmac.Equal(encKey, hashKey) {
		return nil, errors.New("pii: khoa ma hoa va khoa bam phai khac nhau")
	}
	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Protector{aead: aead, hashKey: hashKey}, nil
}

func decodeKey(s string) ([]byte, error) {
	k, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(k) != 32 {
		return nil, ErrInvalidKey
	}
	return k, nil
}

// Encrypt ma hoa plaintext. aad gan ban ma voi ngu canh (vd "identities.phone_enc:<id>"):
// chep ban ma sang dong khac thi giai ma that bai.
// Dinh dang: version(1) || nonce(12) || ciphertext+tag.
func (p *Protector) Encrypt(plaintext, aad string) ([]byte, error) {
	nonce := make([]byte, p.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := make([]byte, 0, 1+len(nonce)+len(plaintext)+p.aead.Overhead())
	out = append(out, keyVersion)
	out = append(out, nonce...)
	return p.aead.Seal(out, nonce, []byte(plaintext), []byte(aad)), nil
}

// Decrypt giai ma ban ma cua Encrypt voi cung aad.
func (p *Protector) Decrypt(ciphertext []byte, aad string) (string, error) {
	ns := p.aead.NonceSize()
	if len(ciphertext) < 1+ns+p.aead.Overhead() || ciphertext[0] != keyVersion {
		return "", ErrDecrypt
	}
	nonce, body := ciphertext[1:1+ns], ciphertext[1+ns:]
	plain, err := p.aead.Open(nil, nonce, body, []byte(aad))
	if err != nil {
		return "", ErrDecrypt
	}
	return string(plain), nil
}

// Hash tra HMAC-SHA256 dang hex (64 ky tu). Cung dau vao luon cho cung ket qua.
func (p *Protector) Hash(value string) string {
	m := hmac.New(sha256.New, p.hashKey)
	m.Write([]byte(value))
	return hex.EncodeToString(m.Sum(nil))
}

// NormalizePhoneVN dua so di dong Viet Nam ve mot dang duy nhat "84xxxxxxxxx" de "0912..." va
// "+84 912..." la cung mot nguoi. Khong phai so hop le thi tra ok=false.
func NormalizePhoneVN(s string) (string, bool) {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' || r == ' ' || r == '-' || r == '.' || r == '(' || r == ')':
		default:
			return "", false
		}
	}
	d := b.String()
	if strings.HasPrefix(d, "0") {
		d = "84" + d[1:]
	}
	if len(d) != 11 || !strings.HasPrefix(d, "84") || !strings.ContainsRune("35789", rune(d[2])) {
		return "", false
	}
	return d, true
}
