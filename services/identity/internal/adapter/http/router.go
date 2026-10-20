package http

import (
	"net/http"

	"github.com/eventflow/eventflow/services/identity/internal/domain"
)

// RegisterRoutes cấu hình các đường dẫn API cho dịch vụ Identity
func RegisterRoutes(mux *http.ServeMux, authHandler *AuthHandler, jwtSecret string, auditRepo domain.AuditRepository) {
	// Group /api/v1/auth (Public)
	mux.HandleFunc("POST /api/v1/auth/register/request-otp", authHandler.RequestOTP)
	mux.HandleFunc("POST /api/v1/auth/register/verify", authHandler.VerifyOTP)
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("POST /api/v1/auth/google", authHandler.GoogleLogin)
	mux.HandleFunc("POST /api/v1/auth/forgot-password/request-otp", authHandler.RequestResetOTP)
	mux.HandleFunc("POST /api/v1/auth/forgot-password/reset", authHandler.ResetPassword)
	mux.HandleFunc("POST /api/v1/auth/vneid/callback", authHandler.VNeIDCallback)
	mux.HandleFunc("GET /api/v1/auth/vneid/sse", authHandler.VNeIDListenSSE)
	mux.HandleFunc("POST /api/v1/auth/refresh", authHandler.Refresh)
	mux.HandleFunc("POST /api/v1/auth/logout", authHandler.Logout)

	// Protected routes
	authMiddleware := Authenticate(jwtSecret)
	mux.Handle("GET /api/v1/auth/me", authMiddleware(http.HandlerFunc(authHandler.Me)))
	mux.Handle("POST /api/v1/auth/devices", authMiddleware(http.HandlerFunc(authHandler.RegisterDevice)))
	mux.Handle("POST /api/v1/auth/logout-all", authMiddleware(http.HandlerFunc(authHandler.LogoutAll)))
}

