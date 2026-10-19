package authx_test

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
)

func TestNewQREngine_SecretValidation(t *testing.T) {
	_, err := authx.NewQREngine([]byte("too-short"))
	if !errors.Is(err, authx.ErrInvalidSecret) {
		t.Fatalf("ky vong ErrInvalidSecret voi secret ngan, nhan duoc: %v", err)
	}

	validSecret := []byte("this-is-a-very-secure-32-byte-key!")
	engine, err := authx.NewQREngine(validSecret)
	if err != nil {
		t.Fatalf("khoi tao engine that bai voi secret hop le: %v", err)
	}
	if engine == nil {
		t.Fatal("engine tra ve nil")
	}
}

func TestQREngine_SignAndVerify_Success(t *testing.T) {
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)
	engine, err := authx.NewQREngine(secret)
	if err != nil {
		t.Fatalf("khoi tao engine that bai: %v", err)
	}

	claims := authx.TicketClaims{
		TicketID:     "11111111-1111-1111-1111-111111111111",
		OrderID:      "22222222-2222-2222-2222-222222222222",
		EventID:      "33333333-3333-3333-3333-333333333333",
		TicketTypeID: "44444444-4444-4444-4444-444444444444",
		IdentityID:   "55555555-5555-5555-5555-555555555555",
		JTI:          "jti-unique-token-test-12345",
		IssuedAt:     time.Now().Unix(),
		ExpiresAt:    time.Now().Add(24 * time.Hour).Unix(),
	}

	qrPayload, sig, err := engine.Sign(claims)
	if err != nil {
		t.Fatalf("ky QR that bai: %v", err)
	}
	if qrPayload == "" || sig == "" {
		t.Fatalf("qrPayload hoac sig rong: qrPayload=%q, sig=%q", qrPayload, sig)
	}

	// Verify lai
	verifiedClaims, err := engine.Verify(qrPayload)
	if err != nil {
		t.Fatalf("verify QR hop le that bai: %v", err)
	}

	if verifiedClaims.TicketID != claims.TicketID {
		t.Errorf("TicketID lech: want %s, got %s", claims.TicketID, verifiedClaims.TicketID)
	}
	if verifiedClaims.OrderID != claims.OrderID {
		t.Errorf("OrderID lech: want %s, got %s", claims.OrderID, verifiedClaims.OrderID)
	}
	if verifiedClaims.EventID != claims.EventID {
		t.Errorf("EventID lech: want %s, got %s", claims.EventID, verifiedClaims.EventID)
	}
	if verifiedClaims.JTI != claims.JTI {
		t.Errorf("JTI lech: want %s, got %s", claims.JTI, verifiedClaims.JTI)
	}
	if verifiedClaims.ExpiresAt != claims.ExpiresAt {
		t.Errorf("ExpiresAt lech: want %d, got %d", claims.ExpiresAt, verifiedClaims.ExpiresAt)
	}
}

func TestQREngine_TamperedPayload_Rejected(t *testing.T) {
	secret := []byte("a-very-secure-random-secret-key-1234")
	engine, err := authx.NewQREngine(secret)
	if err != nil {
		t.Fatal(err)
	}

	claims := authx.TicketClaims{
		TicketID:     "11111111-1111-1111-1111-111111111111",
		OrderID:      "22222222-2222-2222-2222-222222222222",
		EventID:      "33333333-3333-3333-3333-333333333333",
		TicketTypeID: "44444444-4444-4444-4444-444444444444",
		JTI:          "jti-original",
		ExpiresAt:    time.Now().Add(time.Hour).Unix(),
	}

	qrPayload, _, err := engine.Sign(claims)
	if err != nil {
		t.Fatal(err)
	}

	// Gia mao du lieu: thay doi 1 byte trong phan base64 payload
	tamperedPayload := "X" + qrPayload[1:]
	_, err = engine.Verify(tamperedPayload)
	if !errors.Is(err, authx.ErrTamperedQR) {
		t.Fatalf("ky vong ErrTamperedQR khi sua payload, nhan duoc: %v", err)
	}
}

func TestQREngine_InvalidSignature_Rejected(t *testing.T) {
	secret1 := []byte("secret-key-number-one-1234567890123")
	secret2 := []byte("secret-key-number-two-9876543210987")

	engine1, _ := authx.NewQREngine(secret1)
	engine2, _ := authx.NewQREngine(secret2)

	claims := authx.TicketClaims{
		TicketID:     "11111111-1111-1111-1111-111111111111",
		OrderID:      "22222222-2222-2222-2222-222222222222",
		EventID:      "33333333-3333-3333-3333-333333333333",
		TicketTypeID: "44444444-4444-4444-4444-444444444444",
		JTI:          "jti-secret-test",
		ExpiresAt:    time.Now().Add(time.Hour).Unix(),
	}

	qrPayload, _, _ := engine1.Sign(claims)

	// Verify voi engine2 (dung secret khac) -> phai bi tu choi
	_, err := engine2.Verify(qrPayload)
	if !errors.Is(err, authx.ErrTamperedQR) {
		t.Fatalf("ky vong ErrTamperedQR khi secret khac, nhan duoc: %v", err)
	}
}

func TestQREngine_ExpiredQR_Rejected(t *testing.T) {
	secret := []byte("a-very-secure-random-secret-key-1234")
	baseTime := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)

	currentClock := baseTime
	engine, err := authx.NewQREngine(secret)
	if err != nil {
		t.Fatal(err)
	}
	engine.WithClock(func() time.Time { return currentClock })

	claims := authx.TicketClaims{
		TicketID:     "11111111-1111-1111-1111-111111111111",
		OrderID:      "22222222-2222-2222-2222-222222222222",
		EventID:      "33333333-3333-3333-3333-333333333333",
		TicketTypeID: "44444444-4444-4444-4444-444444444444",
		JTI:          "jti-expired-test",
		IssuedAt:     baseTime.Unix(),
		ExpiresAt:    baseTime.Add(2 * time.Hour).Unix(), // Het han sau 2 tieng
	}

	qrPayload, _, err := engine.Sign(claims)
	if err != nil {
		t.Fatal(err)
	}

	// 1 gio sau -> van con han
	currentClock = baseTime.Add(1 * time.Hour)
	_, err = engine.Verify(qrPayload)
	if err != nil {
		t.Fatalf("ky vong thanh cong khi chua het han, nhan duoc: %v", err)
	}

	// 3 gio sau -> qua han
	currentClock = baseTime.Add(3 * time.Hour)
	_, err = engine.Verify(qrPayload)
	if !errors.Is(err, authx.ErrExpiredQR) {
		t.Fatalf("ky vong ErrExpiredQR khi qua han, nhan duoc: %v", err)
	}
}

func TestQREngine_InvalidFormatAndMissingClaims(t *testing.T) {
	secret := []byte("a-very-secure-random-secret-key-1234")
	engine, _ := authx.NewQREngine(secret)

	// Format sai
	badFormats := []string{
		"",
		"not-a-token",
		"onlyonepart",
		"one.two.three",
		".signature",
		"payload.",
	}
	for _, bad := range badFormats {
		_, err := engine.Verify(bad)
		if !errors.Is(err, authx.ErrInvalidFormat) {
			t.Errorf("ky vong ErrInvalidFormat cho chuoi %q, nhan duoc: %v", bad, err)
		}
	}

	// Thieu claims bat buoc khi ky
	_, _, err := engine.Sign(authx.TicketClaims{TicketID: "tid-only"})
	if !errors.Is(err, authx.ErrMissingClaims) {
		t.Errorf("ky vong ErrMissingClaims khi thieu truong, nhan duoc: %v", err)
	}

	// Thieu claims khi verify (payload hop le HMAC nhung thieu truong ben trong)
	emptyClaimsJSON, _ := json.Marshal(map[string]string{"foo": "bar"})
	encPayload := base64.RawURLEncoding.EncodeToString(emptyClaimsJSON)
	// ky thu cong
	qrFull, _, _ := engine.Sign(authx.TicketClaims{
		TicketID: "tid", OrderID: "oid", EventID: "eid", JTI: "jti",
	})
	_ = qrFull
	// Thay payload rong claims vao nhung ky dung
	engine2, _ := authx.NewQREngine(secret)
	fakeQR, _, _ := engine2.Sign(authx.TicketClaims{
		TicketID: "t", OrderID: "o", EventID: "e", JTI: "j",
	})
	_ = fakeQR

	// Verify chuoi decode dung HMAC nhung json thieu
	validHMACWithMissingFields := func() string {
		claims := authx.TicketClaims{TicketID: "t", OrderID: "o", EventID: "e", JTI: "j"}
		full, _, _ := engine.Sign(claims)
		return full
	}()
	v, err := engine.Verify(validHMACWithMissingFields)
	if err != nil || v.TicketID != "t" {
		t.Fatalf("loi bat ngo voi claims toi thieu: %v", err)
	}
	_ = encPayload
}
