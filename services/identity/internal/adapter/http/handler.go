package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/eventflow/eventflow/libs/go/httpx"
	"github.com/eventflow/eventflow/services/identity/internal/app"
	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type AuthHandler struct {
	registerUC    app.RegisterUseCase
	loginUC       app.LoginUseCase
	vneidUC       app.VNeIDUseCase
	forgotPwdUC   app.ForgotPasswordUseCase
	googleLoginUC app.GoogleLoginUseCase
	sessions      SessionManager
	devices       domain.DeviceRegistrar
	redisClient   *redis.Client
}

// WithDevices bật API đăng ký thiết bị (EV-113).
func (h *AuthHandler) WithDevices(d domain.DeviceRegistrar) *AuthHandler {
	h.devices = d
	return h
}

// Loại lỗi RFC 7807 của API thiết bị.
const (
	ProblemInvalidFingerprint = "/problems/invalid-fingerprint"
	ProblemTooManyDevices     = "/problems/too-many-devices"
)

// deviceFrom dựng DeviceInfo từ fingerprint client gửi; nil nếu client không gửi.
// IP lấy từ X-Forwarded-For do gateway đặt (gateway xoá giá trị client gửi).
func deviceFrom(r *http.Request, fingerprint string) *domain.DeviceInfo {
	if fingerprint == "" {
		return nil
	}
	d := newDevice(r, fingerprint)
	return &d
}

func newDevice(r *http.Request, fingerprint string) domain.DeviceInfo {
	ip := r.RemoteAddr
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ip, _, _ = strings.Cut(xff, ",")
	}
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	return domain.DeviceInfo{Fingerprint: fingerprint, IP: strings.TrimSpace(ip)}
}

// writeDeviceError trả true nếu err là lỗi thiết bị đã được ghi ra response.
func writeDeviceError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, domain.ErrInvalidFingerprint):
		httpx.WriteProblemType(w, http.StatusBadRequest, ProblemInvalidFingerprint, "device fingerprint không hợp lệ")
	case errors.Is(err, domain.ErrTooManyDevices):
		httpx.WriteProblemType(w, http.StatusTooManyRequests, ProblemTooManyDevices, "tài khoản đã gắn quá nhiều thiết bị")
	default:
		return false
	}
	return true
}

// RegisterDevice gắn thiết bị với người đang đăng nhập; idempotent theo fingerprint.
func (h *AuthHandler) RegisterDevice(w http.ResponseWriter, r *http.Request) {
	caller, ok := CallerFromContext(r.Context())
	if !ok || h.devices == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "Chưa đăng nhập")
		return
	}
	var req RegisterDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid Request", "Dữ liệu JSON không hợp lệ")
		return
	}
	id, err := h.devices.Register(r.Context(), caller.ID, newDevice(r, req.Fingerprint))
	if err != nil {
		if !writeDeviceError(w, err) {
			slog.ErrorContext(r.Context(), "dang ky thiet bi that bai", "err", err)
			writeError(w, http.StatusInternalServerError, "Server Error", "Lỗi hệ thống nội bộ")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"device_id": id.String()})
}

// SessionManager là phần của app.SessionService mà handler cần (EV-112).
type SessionManager interface {
	Refresh(ctx context.Context, rawRefresh string, device *domain.DeviceInfo) (*domain.TokenPair, *domain.Identity, error)
	Logout(ctx context.Context, rawRefresh string) error
	LogoutAll(ctx context.Context, identityID uuid.UUID) error
}

func NewAuthHandler(registerUC app.RegisterUseCase, loginUC app.LoginUseCase, vneidUC app.VNeIDUseCase, forgotPwdUC app.ForgotPasswordUseCase, googleLoginUC app.GoogleLoginUseCase, sessions SessionManager, redisClient *redis.Client) *AuthHandler {
	return &AuthHandler{
		registerUC:    registerUC,
		loginUC:       loginUC,
		vneidUC:       vneidUC,
		forgotPwdUC:   forgotPwdUC,
		googleLoginUC: googleLoginUC,
		sessions:      sessions,
		redisClient:   redisClient,
	}
}

// tokenResponse đổi TokenPair sang DTO; expires_in tính từ lúc trả lời.
func tokenResponse(p *domain.TokenPair) *TokenResponse {
	if p == nil {
		return nil
	}
	now := time.Now()
	return &TokenResponse{
		Token:            p.AccessToken,
		AccessToken:      p.AccessToken,
		TokenType:        "Bearer",
		ExpiresIn:        int64(p.AccessExpiresAt.Sub(now).Seconds()),
		RefreshToken:     p.RefreshToken,
		RefreshExpiresIn: int64(p.RefreshExpiresAt.Sub(now).Seconds()),
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
		Device:     deviceFrom(r, req.DeviceFingerprint),
	})

	if err != nil {
		switch {
		case writeDeviceError(w, err):
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
		ID:            output.Identity.ID,
		Email:         output.Identity.Email,
		Role:          string(output.Identity.Role),
		CreatedAt:     output.Identity.CreatedAt,
		TokenResponse: tokenResponse(output.Tokens),
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *AuthHandler) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	var req GoogleLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid Request", "Dữ liệu JSON không hợp lệ")
		return
	}

	output, err := h.googleLoginUC.Execute(r.Context(), app.GoogleLoginInput{
		IDToken: req.Credential,
	})

	if err != nil {
		switch {
		case errors.Is(err, app.ErrInvalidGoogleToken):
			writeError(w, http.StatusUnauthorized, "Unauthorized", "Google Token không hợp lệ hoặc đã hết hạn")
		default:
			writeError(w, http.StatusInternalServerError, "Server Error", err.Error())
		}
		return
	}

	resp := IdentityResponse{
		ID:            output.Identity.ID,
		Email:         output.Identity.Email,
		Role:          string(output.Identity.Role),
		CreatedAt:     output.Identity.CreatedAt,
		TokenResponse: tokenResponse(output.Tokens),
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---- SESSION (EV-112) ----

// Refresh đổi refresh token lấy cặp token mới; token cũ bị thu hồi ngay.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "Invalid Request", "Thiếu refresh_token")
		return
	}

	pair, identity, err := h.sessions.Refresh(r.Context(), req.RefreshToken, deviceFrom(r, req.DeviceFingerprint))
	if err != nil {
		switch {
		case writeDeviceError(w, err):
		case errors.Is(err, domain.ErrRefreshTokenReused):
			slog.WarnContext(r.Context(), "refresh token bi dung lai, da thu hoi ca phien")
			writeError(w, http.StatusUnauthorized, "Unauthorized", "Phiên đăng nhập đã bị thu hồi, vui lòng đăng nhập lại")
		case errors.Is(err, domain.ErrRefreshTokenInvalid):
			writeError(w, http.StatusUnauthorized, "Unauthorized", "Refresh token không hợp lệ hoặc đã hết hạn")
		default:
			slog.ErrorContext(r.Context(), "refresh token that bai", "err", err)
			writeError(w, http.StatusInternalServerError, "Server Error", "Lỗi hệ thống nội bộ")
		}
		return
	}

	writeJSON(w, http.StatusOK, IdentityResponse{
		ID:            identity.ID,
		Email:         identity.Email,
		Role:          string(identity.Role),
		CreatedAt:     identity.CreatedAt,
		TokenResponse: tokenResponse(pair),
	})
}

// Logout thu hồi phiên chứa refresh token. Luôn 204 với token lạ để không lộ token nào hợp lệ.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid Request", "Dữ liệu JSON không hợp lệ")
		return
	}
	if err := h.sessions.Logout(r.Context(), req.RefreshToken); err != nil {
		slog.ErrorContext(r.Context(), "logout that bai", "err", err)
		writeError(w, http.StatusInternalServerError, "Server Error", "Lỗi hệ thống nội bộ")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LogoutAll thu hồi mọi phiên của người đang đăng nhập (cần access token).
func (h *AuthHandler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	caller, ok := CallerFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "Chưa đăng nhập")
		return
	}
	if err := h.sessions.LogoutAll(r.Context(), caller.ID); err != nil {
		slog.ErrorContext(r.Context(), "logout-all that bai", "err", err)
		writeError(w, http.StatusInternalServerError, "Server Error", "Lỗi hệ thống nội bộ")
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

// Me trả về thông tin định danh và vai trò của người dùng hiện tại từ context
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	caller, ok := CallerFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "Chưa đăng nhập")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":   caller.ID.String(),
		"role": string(caller.Role),
	})
}

