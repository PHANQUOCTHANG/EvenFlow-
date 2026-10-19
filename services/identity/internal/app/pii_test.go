package app_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/eventflow/eventflow/libs/go/pii"
	"github.com/eventflow/eventflow/services/identity/internal/app"
)

// captureLogs chuyen slog mac dinh vao buffer (qua bo loc PII nhu main) trong mot test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(pii.NewRedactingHandler(slog.NewJSONHandler(&buf, nil))))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// EV-115: OTP khong bao gio vao log; khong co kenh gui that thi chi kenh dev nhan.
func TestRegister_OTPKhongVaoLogChiDiKenhDev(t *testing.T) {
	logs := captureLogs(t)
	var gotID, gotOTP string
	redisClient := newFakeRedis()
	uc := app.NewRegisterUseCase(newFakeRepo(), &fakePasswordHasher{}, redisClient, nil,
		app.WithDevOTPSink(func(id, otp string) { gotID, gotOTP = id, otp }))

	if err := uc.RequestOTP(context.Background(), app.RequestOTPInput{Identifier: "0912345678"}); err != nil {
		t.Fatal(err)
	}
	stored := redisClient.otpStore["0912345678"]
	if gotOTP == "" || gotOTP != stored || gotID != "0912345678" {
		t.Fatalf("kenh dev nhan (%q,%q), OTP that %q", gotID, gotOTP, stored)
	}
	if strings.Contains(logs.String(), stored) || strings.Contains(logs.String(), "0912345678") {
		t.Fatalf("log lo OTP hoac SDT: %s", logs)
	}
}

func TestRegister_KhongCoKenhDevThiKhongInOTP(t *testing.T) {
	logs := captureLogs(t)
	redisClient := newFakeRedis()
	uc := app.NewRegisterUseCase(newFakeRepo(), &fakePasswordHasher{}, redisClient, nil)
	if err := uc.RequestOTP(context.Background(), app.RequestOTPInput{Identifier: "0912345678"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.String(), redisClient.otpStore["0912345678"]) {
		t.Fatal("production (khong co kenh dev) van in OTP")
	}
}

func TestRegister_EmailOTPLogDaCheEmail(t *testing.T) {
	logs := captureLogs(t)
	notifier := &fakeNotifier{}
	uc := app.NewRegisterUseCase(newFakeRepo(), &fakePasswordHasher{}, newFakeRedis(), notifier)
	if err := uc.RequestOTP(context.Background(), app.RequestOTPInput{Identifier: "nguyenvana@gmail.com"}); err != nil {
		t.Fatal(err)
	}
	if out := logs.String(); strings.Contains(out, "nguyenvana@") || !strings.Contains(out, "n***@gmail.com") {
		t.Fatalf("log email phai bi che: %s", out)
	}
}

func TestForgotPassword_OTPKhongVaoLog(t *testing.T) {
	logs := captureLogs(t)
	repo := newFakeRepo()
	phone := "0912345678"
	repo.identities[phone] = nil // ton tai: fake chi kiem key
	var gotOTP string
	redisClient := newFakeRedis()
	uc := app.NewForgotPasswordUseCase(repo, &fakePasswordHasher{}, redisClient, nil,
		app.WithResetDevOTPSink(func(_, otp string) { gotOTP = otp }))
	if err := uc.RequestResetOTP(context.Background(), phone); err != nil {
		t.Fatal(err)
	}
	if gotOTP == "" || strings.Contains(logs.String(), gotOTP) {
		t.Fatalf("OTP dat lai mat khau: kenh dev %q, log %s", gotOTP, logs)
	}
}

func TestRegister_SoDienThoaiKhongHopLeBiTuChoi(t *testing.T) {
	uc := app.NewRegisterUseCase(newFakeRepo(), &fakePasswordHasher{}, newFakeRedis(), nil)
	for _, in := range []string{"0123456789", "091234567", "84 1 2"} {
		if err := uc.RequestOTP(context.Background(), app.RequestOTPInput{Identifier: in}); err != app.ErrInvalidIdentifier {
			t.Fatalf("%q: muon ErrInvalidIdentifier, nhan %v", in, err)
		}
	}
}
