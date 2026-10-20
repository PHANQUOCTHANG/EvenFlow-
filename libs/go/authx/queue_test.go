package authx_test

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
)

var queueSecret = []byte("queue-secret-for-tests-32-bytes!")

func queueClaims(now time.Time) authx.QueueClaims {
	return authx.QueueClaims{
		EventID: "ev-1", Subject: "7f6c2a8e-0000-4000-8000-000000000001",
		DeviceID: "7f6c2a8e-0000-4000-8000-0000000000d1", JTI: "jti-abc",
		IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	}
}

func TestQueueToken_KyRoiXacMinh(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tok, err := authx.SignQueueToken(queueSecret, queueClaims(now))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tok, "q1.") {
		t.Fatalf("token phai bat dau bang q1.: %q", tok)
	}
	got, err := authx.VerifyQueueToken(queueSecret, tok, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	want := queueClaims(now)
	if got.EventID != want.EventID || got.Subject != want.Subject || got.DeviceID != want.DeviceID || got.JTI != want.JTI {
		t.Fatalf("claims sai: %+v", got)
	}
}

func TestQueueToken_HetHanTraLoiRieng(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tok, _ := authx.SignQueueToken(queueSecret, queueClaims(now))
	if _, err := authx.VerifyQueueToken(queueSecret, tok, now.Add(time.Hour)); !errors.Is(err, authx.ErrExpiredQueueToken) {
		t.Fatalf("het han phai ErrExpiredQueueToken, nhan %v", err)
	}
}

func TestQueueToken_TuChoiTokenGiaMao(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	good, _ := authx.SignQueueToken(queueSecret, queueClaims(now))
	other, _ := authx.SignQueueToken([]byte("another-secret-that-is-long-enough"), queueClaims(now))

	// Doi payload (doi subject) giu nguyen chu ky.
	parts := strings.Split(good, ".")
	forged := queueClaims(now)
	forged.Subject = "7f6c2a8e-0000-4000-8000-000000000002"
	other2, _ := authx.SignQueueToken([]byte("another-secret-that-is-long-enough"), forged)
	forgedPayload := strings.Split(other2, ".")[1]

	cases := map[string]string{
		"khac secret":           other,
		"doi payload":           "q1." + forgedPayload + "." + parts[2],
		"sua chu ky":            good[:len(good)-2] + "AA",
		"thieu chu ky":          "q1." + parts[1],
		"thua phan":             good + ".x",
		"hex cu (truoc EV-137)": "0123456789abcdef0123456789abcdef",
		"rong":                  "",
		"payload rac":           "q1." + base64.RawURLEncoding.EncodeToString([]byte("{}")) + "." + parts[2],
		"khong phai base64":     "q1.!!!.!!!",
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := authx.VerifyQueueToken(queueSecret, tok, now); !errors.Is(err, authx.ErrInvalidQueueToken) {
				t.Fatalf("phai tu choi, nhan %v", err)
			}
		})
	}
}

// Queue token khong the dung nhu access token / QR va nguoc lai (khac domain HMAC).
func TestQueueToken_KhongLanVoiLoaiTokenKhac(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	access, _ := authx.SignAccessToken(queueSecret, validClaims(now))
	if _, err := authx.VerifyQueueToken(queueSecret, access, now); err == nil {
		t.Fatal("access token khong duoc chap nhan la queue token")
	}
	q, _ := authx.SignQueueToken(queueSecret, queueClaims(now))
	if _, err := authx.VerifyAccessToken(queueSecret, q, now); err == nil {
		t.Fatal("queue token khong duoc chap nhan la access token")
	}
}

func TestQueueToken_Matches(t *testing.T) {
	c := queueClaims(time.Unix(1_800_000_000, 0))
	const dev = "7f6c2a8e-0000-4000-8000-0000000000d1"
	if !c.Matches("ev-1", c.Subject, dev) {
		t.Fatal("dung su kien/nguoi/thiet bi phai khop")
	}
	for name, ok := range map[string]bool{
		"su kien khac":   c.Matches("ev-2", c.Subject, dev),
		"nguoi khac":     c.Matches("ev-1", "7f6c2a8e-0000-4000-8000-000000000002", dev),
		"thiet bi khac":  c.Matches("ev-1", c.Subject, "7f6c2a8e-0000-4000-8000-0000000000d2"),
		"thieu thiet bi": c.Matches("ev-1", c.Subject, ""),
	} {
		if ok {
			t.Fatalf("%s: khong duoc khop", name)
		}
	}
	c.DeviceID = ""
	if !c.Matches("ev-1", c.Subject, "") || !c.Matches("ev-1", c.Subject, "bat-ky") {
		t.Fatal("token khong gan thiet bi khop moi thiet bi")
	}
}

func TestQueueToken_KyTuChoiThieuClaim(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	for name, mutate := range map[string]func(*authx.QueueClaims){
		"thieu eid":  func(c *authx.QueueClaims) { c.EventID = "" },
		"thieu sub":  func(c *authx.QueueClaims) { c.Subject = "" },
		"thieu jti":  func(c *authx.QueueClaims) { c.JTI = "" },
		"han <= iat": func(c *authx.QueueClaims) { c.ExpiresAt = c.IssuedAt },
	} {
		c := queueClaims(now)
		mutate(&c)
		if _, err := authx.SignQueueToken(queueSecret, c); err == nil {
			t.Fatalf("%s: phai bao loi", name)
		}
	}
	if _, err := authx.SignQueueToken([]byte("short"), queueClaims(now)); !errors.Is(err, authx.ErrInvalidSecret) {
		t.Fatal("secret ngan phai bi tu choi")
	}
}
