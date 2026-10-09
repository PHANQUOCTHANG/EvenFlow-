package http

import (
	"net/http"
)

// RegisterRoutes cấu hình các đường dẫn API cho dịch vụ Identity
func RegisterRoutes(mux *http.ServeMux, authHandler *AuthHandler) {
	// Group /api/v1/auth
	mux.HandleFunc("POST /api/v1/auth/register", authHandler.Register)
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
}
