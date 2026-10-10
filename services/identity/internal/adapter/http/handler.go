package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/eventflow/eventflow/services/identity/internal/app"
	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/redis/go-redis/v9"
)

type AuthHandler struct {
	registerUC  app.RegisterUseCase
	loginUC     app.LoginUseCase
	vneidUC     app.VNeIDUseCase
	forgotPwdUC app.ForgotPasswordUseCase
	redisClient *redis.Client
}

func NewAuthHandler(registerUC app.RegisterUseCase, loginUC app.LoginUseCase, vneidUC app.VNeIDUseCase, forgotPwdUC app.ForgotPasswordUseCase, redisClient *redis.Client) *AuthHandler {
	return &AuthHandler{
		registerUC:  registerUC,
		loginUC:     loginUC,
		vneidUC:     vneidUC,
		forgotPwdUC: forgotPwdUC,
		redisClient: redisClient,
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

func (h *AuthHandler) RequestOTP(w http.ResponseWriter, r *http.Request) {
	var req RequestOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid Request", "Dữ liệu JSON không hợp lệ")
		return
	}

	err := h.registerUC.RequestOTP(r.Context(), app.RequestOTPInput{
		Identifier: req.Identifier,
	})

	if err != nil {
		switch {
		case errors.Is(err, app.ErrInvalidIdentifier):
			writeError(w, http.StatusBadRequest, "Validation Error", err.Error())
		case errors.Is(err, app.ErrRateLimited):
			writeError(w, http.StatusTooManyRequests, "Rate Limited", err.Error())
		case errors.Is(err, domain.ErrIdentityExists):
			writeError(w, http.StatusConflict, "Conflict", "Email hoặc số điện thoại này đã được đăng ký.")
		default:
			writeError(w, http.StatusInternalServerError, "Server Error", "Lỗi hệ thống nội bộ")
		}
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *AuthHandler) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	var req VerifyOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid Request", "Dữ liệu JSON không hợp lệ")
		return
	}

	identity, err := h.registerUC.VerifyOTP(r.Context(), app.VerifyOTPInput{
		Identifier: req.Identifier,
		Password:   req.Password,
		OTP:        req.OTP,
	})

	if err != nil {
		switch {
		case errors.Is(err, app.ErrInvalidOTP), errors.Is(err, app.ErrPasswordTooShort):
			writeError(w, http.StatusBadRequest, "Validation Error", err.Error())
		case errors.Is(err, domain.ErrIdentityExists):
			writeError(w, http.StatusConflict, "Conflict", "Tài khoản này đã tồn tại")
		default:
			if err.Error() == "nhập sai quá nhiều lần, mã OTP đã bị vô hiệu hóa. Vui lòng lấy mã mới" {
				writeError(w, http.StatusTooManyRequests, "Rate Limited", err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "Server Error", "Lỗi hệ thống nội bộ")
			}
		}
		return
	}

	var email *string
	if identity.Email != nil {
		email = identity.Email
	}

	resp := IdentityResponse{
		ID:        identity.ID,
		Email:     email,
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

	output, err := h.loginUC.Execute(r.Context(), app.LoginInput{
		Identifier: req.Identifier,
		Password:   req.Password,
	})

	if err != nil {
		switch {
		case errors.Is(err, app.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, "Validation Error", err.Error())
		case errors.Is(err, app.ErrInvalidCredentials):
			writeError(w, http.StatusUnauthorized, "Unauthorized", "Email/Số điện thoại hoặc mật khẩu không chính xác")
		default:
			writeError(w, http.StatusInternalServerError, "Server Error", "Lỗi hệ thống nội bộ")
		}
		return
	}

	resp := IdentityResponse{
		ID:        output.Identity.ID,
		Email:     output.Identity.Email,
		Role:      string(output.Identity.Role),
		Token:     output.Token,
		CreatedAt: output.Identity.CreatedAt,
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---- FORGOT PASSWORD ----

func (h *AuthHandler) RequestResetOTP(w http.ResponseWriter, r *http.Request) {
	var req RequestResetOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid Request", "Dữ liệu JSON không hợp lệ")
		return
	}

	err := h.forgotPwdUC.RequestResetOTP(r.Context(), req.Identifier)

	if err != nil {
		switch {
		case errors.Is(err, app.ErrInvalidIdentifier):
			writeError(w, http.StatusBadRequest, "Validation Error", err.Error())
		case errors.Is(err, app.ErrRateLimited):
			writeError(w, http.StatusTooManyRequests, "Rate Limited", err.Error())
		default:
			if err.Error() == "tài khoản không tồn tại" {
				writeError(w, http.StatusNotFound, "Not Found", err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "Server Error", "Lỗi hệ thống nội bộ")
			}
		}
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid Request", "Dữ liệu JSON không hợp lệ")
		return
	}

	err := h.forgotPwdUC.ResetPassword(r.Context(), app.ResetPasswordInput{
		Identifier:  req.Identifier,
		OTP:         req.OTP,
		NewPassword: req.NewPassword,
	})

	if err != nil {
		switch {
		case errors.Is(err, app.ErrInvalidOTP), errors.Is(err, app.ErrPasswordTooShort):
			writeError(w, http.StatusBadRequest, "Validation Error", err.Error())
		default:
			if err.Error() == "nhập sai quá nhiều lần, mã OTP đã bị vô hiệu hóa. Vui lòng lấy mã mới" {
				writeError(w, http.StatusTooManyRequests, "Rate Limited", err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "Server Error", "Lỗi hệ thống nội bộ")
			}
		}
		return
	}

	w.WriteHeader(http.StatusOK)
}

// ---- VNEID MOCK ----

func (h *AuthHandler) VNeIDCallback(w http.ResponseWriter, r *http.Request) {
	var input app.VNeIDCallbackInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid Request", "Dữ liệu JSON không hợp lệ")
		return
	}

	if err := h.vneidUC.HandleCallback(r.Context(), input); err != nil {
		writeError(w, http.StatusInternalServerError, "Callback Error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AuthHandler) VNeIDListenSSE(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "session_id required", http.StatusBadRequest)
		return
	}

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	rc := w.(http.Flusher)

	// Subscribe to Redis channel
	ctx := r.Context()
	sub := h.redisClient.Subscribe(ctx, "vneid_session:"+sessionID)
	defer sub.Close()

	ch := sub.Channel()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			// Push to client
			data := "data: " + msg.Payload + "\n\n"
			w.Write([]byte(data))
			rc.Flush()
			return // Kết thúc stream sau khi có phản hồi
		}
	}
}
