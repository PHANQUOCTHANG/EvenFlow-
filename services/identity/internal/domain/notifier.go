package domain

import "context"

type OTPNotifier interface {
	SendOTP(ctx context.Context, to string, otp string) error
}
