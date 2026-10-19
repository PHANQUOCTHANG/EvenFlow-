package http

import (
	"time"

	"github.com/google/uuid"
)

// DTO cho yêu cầu lấy OTP
type RequestOTPRequest struct {
	Identifier string `json:"identifier"`
}

// DTO cho yêu cầu xác nhận OTP và tạo tài khoản
type VerifyOTPRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
	OTP        string `json:"otp"`
}

// DTO cho yêu cầu Đăng nhập
type LoginRequest struct {
	Identifier string `json:"identifier"` // Chấp nhận Email hoặc Phone
	Password   string `json:"password"`
}

type GoogleLoginRequest struct {
	Credential string `json:"credential"` // id_token do Google cấp
}

type RequestResetOTPRequest struct {
	Identifier string `json:"identifier"`
}

type ResetPasswordRequest struct {
	Identifier  string `json:"identifier"`
	OTP         string `json:"otp"`
	NewPassword string `json:"newPassword"`
}

// DTO trả về thông tin người dùng
type IdentityResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     *string   `json:"email,omitempty"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	*TokenResponse
}

// TokenResponse là cặp token của một phiên (EV-112). Token giữ lại cho client cũ: cùng giá
// trị với AccessToken.
type TokenResponse struct {
	Token            string `json:"token"`
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int64  `json:"expires_in"`
	RefreshToken     string `json:"refresh_token"`
	RefreshExpiresIn int64  `json:"refresh_expires_in"`
}

// RefreshRequest dùng cho /auth/refresh và /auth/logout.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Cấu trúc lỗi chuẩn RFC 7807 (Problem Details for HTTP APIs)
type ErrorResponse struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}
