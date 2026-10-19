package pii_test

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/eventflow/eventflow/libs/go/pii"
)

func key(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func newProtector(t *testing.T) *pii.Protector {
	t.Helper()
	p, err := pii.NewProtector(key(t), key(t))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestProtector_MaHoaGiaiMaDungAAD(t *testing.T) {
	p := newProtector(t)
	ct, err := p.Encrypt("84912345678", "identities.phone_enc:a")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ct, []byte("84912345678")) {
		t.Fatal("ban ma chua plaintext")
	}
	got, err := p.Decrypt(ct, "identities.phone_enc:a")
	if err != nil || got != "84912345678" {
		t.Fatalf("giai ma: %q %v", got, err)
	}

	ct2, _ := p.Encrypt("84912345678", "identities.phone_enc:a")
	if bytes.Equal(ct, ct2) {
		t.Fatal("hai lan ma hoa phai khac nhau (nonce ngau nhien)")
	}
}

func TestProtector_TuChoiBanMaSaiNguCanh(t *testing.T) {
	p := newProtector(t)
	ct, _ := p.Encrypt("84912345678", "identities.phone_enc:a")

	if _, err := p.Decrypt(ct, "identities.phone_enc:b"); !errors.Is(err, pii.ErrDecrypt) {
		t.Fatalf("chep ban ma sang dong khac phai that bai, nhan %v", err)
	}
	tampered := append([]byte{}, ct...)
	tampered[len(tampered)-1] ^= 1
	if _, err := p.Decrypt(tampered, "identities.phone_enc:a"); !errors.Is(err, pii.ErrDecrypt) {
		t.Fatal("ban ma bi sua phai that bai")
	}
	other := newProtector(t)
	if _, err := other.Decrypt(ct, "identities.phone_enc:a"); !errors.Is(err, pii.ErrDecrypt) {
		t.Fatal("sai khoa phai that bai")
	}
	if _, err := p.Decrypt([]byte{1, 2, 3}, "x"); !errors.Is(err, pii.ErrDecrypt) {
		t.Fatal("ban ma ngan phai that bai")
	}
}

func TestProtector_HashCoDinhVaPhuThuocKhoa(t *testing.T) {
	p, other := newProtector(t), newProtector(t)
	h := p.Hash("84912345678")
	if len(h) != 64 || h != p.Hash("84912345678") {
		t.Fatalf("hash phai co dinh, 64 hex: %q", h)
	}
	if h == other.Hash("84912345678") {
		t.Fatal("khoa khac phai cho hash khac (khong do nguoc bang bang tra)")
	}
}

func TestProtector_KhoaHong(t *testing.T) {
	k := key(t)
	for name, c := range map[string][2]string{
		"khong base64": {"!!!", k},
		"ngan":         {base64.StdEncoding.EncodeToString([]byte("short")), k},
		"trung khoa":   {k, k},
	} {
		if _, err := pii.NewProtector(c[0], c[1]); err == nil {
			t.Fatalf("%s: phai loi", name)
		}
	}
}

func TestNormalizePhoneVN(t *testing.T) {
	ok := map[string]string{
		"0912345678":      "84912345678",
		"84912345678":     "84912345678",
		"+84 912 345 678": "84912345678",
		"0389-999-000":    "84389999000",
	}
	for in, want := range ok {
		if got, valid := pii.NormalizePhoneVN(in); !valid || got != want {
			t.Fatalf("%q -> %q %v, muon %q", in, got, valid, want)
		}
	}
	for _, in := range []string{"", "0123456789", "091234567", "user@example.com", "09123456789"} {
		if _, valid := pii.NormalizePhoneVN(in); valid {
			t.Fatalf("%q phai khong hop le", in)
		}
	}
}

func logTo(buf *bytes.Buffer) *slog.Logger {
	return slog.New(pii.NewRedactingHandler(slog.NewJSONHandler(buf, nil)))
}

func TestRedact_ChePIIVaBiMatTrongLog(t *testing.T) {
	var buf bytes.Buffer
	log := logTo(&buf)
	log.Info("ket noi postgres://eventflow:S3cret@db:5432/eventflow that bai",
		"database_url", "postgres://eventflow:S3cret@db:5432/eventflow",
		"password", "hunter2",
		"refresh_token", "abc",
		"otp", "123456",
		"Authorization", "Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl",
		"email", "nguyenvana@gmail.com",
		"identifier", "0912345678",
		"err", fmt.Errorf("gui mail toi nguyenvana@gmail.com: dial tcp"),
		"note", "lien he 0912345678 hoac token eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl",
		"cccd_number", "001099001122",
		slog.Group("req", "phone", "84912345678", "route", "POST /api/v1/auth/login"),
		"status", 200,
	)
	out := buf.String()
	for _, leak := range []string{"S3cret", "hunter2", "123456\"", "\"abc\"", "nguyenvana@", "0912345678", "84912345678", "eyJhbGciOiJIUzI1NiJ9", "001099001122"} {
		if strings.Contains(out, leak) {
			t.Fatalf("log con lo %q:\n%s", leak, out)
		}
	}
	for _, keep := range []string{"n***@gmail.com", "POST /api/v1/auth/login", `"status":200`, "*******78", "eventflow:***@db"} {
		if !strings.Contains(out, keep) {
			t.Fatalf("log mat thong tin can giu %q:\n%s", keep, out)
		}
	}
}

func TestRedact_WithAttrsCungDuocChe(t *testing.T) {
	var buf bytes.Buffer
	logTo(&buf).With("email", "a.b@example.com", "token", "t").Info("x")
	if out := buf.String(); strings.Contains(out, "a.b@") || strings.Contains(out, `"token":"t"`) {
		t.Fatalf("With() lot PII: %s", out)
	}
}

func TestMask(t *testing.T) {
	if got := pii.Mask("0912345678"); got != "0*******78" {
		t.Fatalf("mask phone = %q", got)
	}
	if got := pii.Mask("user@example.com"); got != "u***@example.com" {
		t.Fatalf("mask email = %q", got)
	}
}
