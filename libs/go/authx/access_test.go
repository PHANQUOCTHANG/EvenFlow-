package authx_test

import (
	"errors"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
	"github.com/golang-jwt/jwt/v5"
)

var accessSecret = []byte("access-secret-for-tests-32-bytes!")

func validClaims(now time.Time) authx.AccessClaims {
	return authx.AccessClaims{
		Subject:  "7f6c2a8e-0000-4000-8000-000000000001",
		Role:     "buyer",
		ID:       "jti-1",
		IssuedAt: now,
	}
}

func TestAccessToken_KyRoiXacMinhDuocDungClaims(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tok, err := authx.SignAccessToken(accessSecret, validClaims(now))
	if err != nil {
		t.Fatalf("ky token: %v", err)
	}

	got, err := authx.VerifyAccessToken(accessSecret, tok, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("xac minh token hop le: %v", err)
	}
	if got.Subject != "7f6c2a8e-0000-4000-8000-000000000001" || got.Role != "buyer" || got.ID != "jti-1" {
		t.Fatalf("claims sai: %+v", got)
	}
	if want := now.Add(authx.AccessTokenTTL); !got.ExpiresAt.Equal(want) {
		t.Fatalf("exp = %v, muon %v (mac dinh 15 phut)", got.ExpiresAt, want)
	}
}

func TestAccessToken_CoXacThucOTPDiTheoToken(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	for _, verified := range []bool{true, false} {
		c := validClaims(now)
		c.Verified = verified
		tok, _ := authx.SignAccessToken(accessSecret, c)
		got, err := authx.VerifyAccessToken(accessSecret, tok, now)
		if err != nil || got.Verified != verified {
			t.Fatalf("verified=%v: nhan %+v %v", verified, got, err)
		}
	}
}

func TestAccessToken_HetHanTraLoiRieng(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tok, _ := authx.SignAccessToken(accessSecret, validClaims(now))

	_, err := authx.VerifyAccessToken(accessSecret, tok, now.Add(authx.AccessTokenTTL+time.Minute))
	if !errors.Is(err, authx.ErrExpiredAccessToken) {
		t.Fatalf("token qua 15 phut phai bao het han, nhan %v", err)
	}
}

func TestAccessToken_TuChoiTokenGiaMao(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	good, _ := authx.SignAccessToken(accessSecret, validClaims(now))
	otherSecret, _ := authx.SignAccessToken([]byte("another-secret-that-is-long-enough"), validClaims(now))

	// Token ky dung secret nhung sai issuer/audience/typ: vd mot JWT cu cua identity (24 gio).
	legacy, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "7f6c2a8e-0000-4000-8000-000000000001", "role": "ops",
		"iat": now.Unix(), "exp": now.Add(24 * time.Hour).Unix(),
	}).SignedString(accessSecret)

	// alg=none: thu vien phai tu choi.
	none, _ := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"iss": authx.AccessIssuer, "aud": authx.AccessAudience, "typ": "access",
		"sub": "x", "role": "ops", "iat": now.Unix(), "exp": now.Add(time.Minute).Unix(),
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)

	// Dung HS512 thay HS256.
	hs512, _ := jwt.NewWithClaims(jwt.SigningMethodHS512, jwt.MapClaims{
		"iss": authx.AccessIssuer, "aud": authx.AccessAudience, "typ": "access",
		"sub": "x", "role": "ops", "iat": now.Unix(), "exp": now.Add(time.Minute).Unix(),
	}).SignedString(accessSecret)

	// Sai typ (vd token muc dich khac cung secret).
	wrongTyp, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": authx.AccessIssuer, "aud": authx.AccessAudience, "typ": "refresh",
		"sub": "x", "role": "ops", "iat": now.Unix(), "exp": now.Add(time.Minute).Unix(),
	}).SignedString(accessSecret)

	cases := map[string]string{
		"khac secret":      otherSecret,
		"jwt cu khong iss": legacy,
		"alg none":         none,
		"alg HS512":        hs512,
		"typ khac access":  wrongTyp,
		"sua payload":      good[:len(good)-4] + "AAAA",
		"chuoi rac":        "not-a-jwt",
		"rong":             "",
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := authx.VerifyAccessToken(accessSecret, tok, now); !errors.Is(err, authx.ErrInvalidAccessToken) {
				t.Fatalf("phai tu choi, nhan %v", err)
			}
		})
	}
}

func TestAccessToken_SecretNganBiTuChoi(t *testing.T) {
	if _, err := authx.SignAccessToken([]byte("short"), validClaims(time.Now())); !errors.Is(err, authx.ErrInvalidSecret) {
		t.Fatalf("secret < 16 byte phai bi tu choi, nhan %v", err)
	}
	if _, err := authx.VerifyAccessToken([]byte("short"), "x", time.Now()); !errors.Is(err, authx.ErrInvalidSecret) {
		t.Fatalf("secret < 16 byte phai bi tu choi, nhan %v", err)
	}
}

func TestAccessToken_ThieuClaimBatBuocKhongKy(t *testing.T) {
	c := validClaims(time.Now())
	c.Role = ""
	if _, err := authx.SignAccessToken(accessSecret, c); err == nil {
		t.Fatal("thieu role phai bao loi")
	}
}
