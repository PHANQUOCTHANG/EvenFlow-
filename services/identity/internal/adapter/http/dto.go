package http

import (
	"time"

	"github.com/google/uuid"
)

// DTO cho yêu cầu Đăng ký
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// DTO cho yêu cầu Đăng nhập
type LoginRequest struct {
	Identifier string `json:"identifier"` // Chấp nhận Email hoặc Phone
	Password   string `json:"password"`
}

// DTO trả về thông tin người dùng
type IdentityResponse struct {
	ID        uuid.UUID  `json:"id"`
	Email     *string    `json:"email,omitempty"`
	Role      string     `json:"role"`
	CreatedAt time.Time  `json:"created_at"`
}

// Cấu trúc lỗi chuẩn RFC 7807 (Problem Details for HTTP APIs)
type ErrorResponse struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}
