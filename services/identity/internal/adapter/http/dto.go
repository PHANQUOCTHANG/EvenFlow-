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
	ID        uuid.UUID  `json:"id"`
	Email     *string    `json:"email,omitempty"`
	Role      string     `json:"role"`
	Token     string     `json:"token,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// Cấu trúc lỗi chuẩn RFC 7807 (Problem Details for HTTP APIs)
type ErrorResponse struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}
