package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/eventflow/eventflow/services/identity/internal/app"
	"github.com/eventflow/eventflow/services/identity/internal/domain"
)

type AuthHandler struct {
	registerUC app.RegisterUseCase
	loginUC    app.LoginUseCase
}

func NewAuthHandler(registerUC app.RegisterUseCase, loginUC app.LoginUseCase) *AuthHandler {
	return &AuthHandler{
		registerUC: registerUC,
		loginUC:    loginUC,
	}
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, title string, detail string) {
	errResp := ErrorResponse{
		Type:   "about:blank",
		Title:  title,
		Status: status,
		Detail: detail,
	}
	writeJSON(w, status, errResp)
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid Request", "Dữ liệu JSON không hợp lệ")
		return
	}

	identity, err := h.registerUC.Execute(r.Context(), app.RegisterInput{
		Email:    req.Email,
		Password: req.Password,
	})

	if err != nil {
		switch {
		case errors.Is(err, app.ErrInvalidEmail), errors.Is(err, app.ErrPasswordTooShort):
			writeError(w, http.StatusBadRequest, "Validation Error", err.Error())
		case errors.Is(err, domain.ErrIdentityExists):
			writeError(w, http.StatusConflict, "Conflict", "Email này đã được sử dụng")
		default:
			writeError(w, http.StatusInternalServerError, "Server Error", "Lỗi hệ thống nội bộ")
		}
		return
	}

	resp := IdentityResponse{
		ID:        identity.ID,
		Email:     identity.Email,
		Role:      string(identity.Role),
		CreatedAt: identity.CreatedAt,
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid Request", "Dữ liệu JSON không hợp lệ")
		return
	}

	identity, err := h.loginUC.Execute(r.Context(), app.LoginInput{
		Identifier: req.Identifier,
		Password:   req.Password,
	})

	if err != nil {
		switch {
		case errors.Is(err, app.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, "Validation Error", err.Error())
		case errors.Is(err, app.ErrInvalidCredentials):
			// 401 Unauthorized cho mọi trường hợp sai thông tin
			writeError(w, http.StatusUnauthorized, "Unauthorized", "Email/Số điện thoại hoặc mật khẩu không chính xác")
		default:
			writeError(w, http.StatusInternalServerError, "Server Error", "Lỗi hệ thống nội bộ")
		}
		return
	}

	resp := IdentityResponse{
		ID:        identity.ID,
		Email:     identity.Email,
		Role:      string(identity.Role),
		CreatedAt: identity.CreatedAt,
	}
	writeJSON(w, http.StatusOK, resp)
}
