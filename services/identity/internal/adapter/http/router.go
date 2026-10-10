package http

import (
	"net/http"
)

// RegisterRoutes cấu hình các đường dẫn API cho dịch vụ Identity
func RegisterRoutes(mux *http.ServeMux, authHandler *AuthHandler) {
	// Group /api/v1/auth
	mux.HandleFunc("POST /api/v1/auth/register/request-otp", authHandler.RequestOTP)
	mux.HandleFunc("POST /api/v1/auth/register/verify", authHandler.VerifyOTP)
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("POST /api/v1/auth/vneid/callback", authHandler.VNeIDCallback)
	mux.HandleFunc("GET /api/v1/auth/vneid/sse", authHandler.VNeIDListenSSE)
}
