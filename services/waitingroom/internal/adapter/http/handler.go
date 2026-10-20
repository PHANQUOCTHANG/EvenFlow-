// Package httpadapter phoi bay phong cho qua REST + SSE.
package httpadapter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
	"github.com/eventflow/eventflow/libs/go/httpx"
	"github.com/eventflow/eventflow/services/waitingroom/internal/domain"
)

// QueueStore la phan hang cho ma handler can.
type QueueStore interface {
	Join(ctx context.Context, eventID, identityID, newToken string) (JoinResult, error)
	Status(ctx context.Context, eventID, token string) (domain.Position, error)
}

type JoinResult struct {
	State domain.State
	Token string
	Rank  int64
	IsNew bool
}

// BotGuard cham diem rui ro truoc khi cho ghi danh (E8).
type BotGuard interface {
	Check(ctx context.Context, eventID, identityID string, signals map[string]any) (allow bool, challenge bool, err error)
}

type Handler struct {
	store QueueStore
	bots  BotGuard
	log   *slog.Logger

	secret        []byte // ky / xac minh queue token (EV-137); rong -> tu choi moi yeu cau co token
	requireDevice bool
	now           func() time.Time
}

// WithQueueTokens bat queue token ky HMAC (EV-137). requireDevice: join bi tu choi neu phien chua gan
// thiet bi, de token luon rang buoc duoc voi thiet bi.
func (h *Handler) WithQueueTokens(secret []byte, requireDevice bool) *Handler {
	h.secret, h.requireDevice = secret, requireDevice
	return h
}

// queueTokenTTL la han queue token. Bang han phien hang (24 gio, xem NewQueue o main): het han thi
// client join lai, ket qua idempotent theo BR-Q3 nen khong mat cho.
const queueTokenTTL = 24 * time.Hour

func New(store QueueStore, bots BotGuard, log *slog.Logger) *Handler {
	return &Handler{store: store, bots: bots, log: log, now: time.Now}
}

func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/events/{eventID}/queue/join", h.join)
	mux.HandleFunc("GET /v1/events/{eventID}/queue/status", h.status)
	mux.HandleFunc("GET /v1/events/{eventID}/queue/stream", h.stream)
}

type joinRequest struct {
	Signals map[string]any `json:"signals"` // tin hieu hanh vi cho anti-bot
}

func (h *Handler) join(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("eventID")
	p, ok := authx.PrincipalFromHeaders(r.Header)
	if !ok {
		httpx.WriteProblem(w, http.StatusUnauthorized, "can dang nhap truoc khi xep hang")
		return
	}
	// BR-Q3: token xep hang gan voi identity DA xac thuc OTP. Chan tai khoan ao ngay cua vao.
	if !p.Verified {
		httpx.WriteProblemType(w, http.StatusForbidden, ProblemOTPRequired, "can xac thuc OTP truoc khi xep hang")
		return
	}
	identityID := p.ID
	if h.secret == nil {
		httpx.WriteProblem(w, http.StatusServiceUnavailable, "phong cho chua cau hinh queue token")
		return
	}
	if h.requireDevice && p.DeviceID == "" {
		httpx.WriteProblemType(w, http.StatusForbidden, ProblemDeviceRequired, "can gan thiet bi cho phien dang nhap truoc khi xep hang")
		return
	}

	var req joinRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	if h.bots != nil {
		allow, challenge, err := h.bots.Check(r.Context(), eventID, identityID, req.Signals)
		switch {
		case err != nil:
			// Anti-bot hong thi KHONG chan khach that. Fail-open o day la lua chon
			// co y: chan nham nguoi that trong dot mo ban ton kem hon nhieu so voi
			// de lot vai bot trong vai phut.
			h.log.Error("antibot loi, tam thoi cho qua", "err", err)
		case challenge:
			httpx.WriteProblem(w, http.StatusForbidden, "can xac minh them truoc khi vao hang cho")
			return
		case !allow:
			httpx.WriteProblem(w, http.StatusForbidden, "yeu cau bi tu choi")
			return
		}
	}

	res, err := h.store.Join(r.Context(), eventID, identityID, newToken())
	if err != nil {
		h.log.Error("join that bai", "event", eventID, "err", err)
		httpx.WriteProblem(w, http.StatusServiceUnavailable, "phong cho dang qua tai, vui long thu lai")
		return
	}

	// Het ve han (EV-146, BR-Q6): khach moi khong duoc ghi danh nen khong co token. 200 chu khong phai loi:
	// day la ket qua nghiep vu binh thuong, client hien man het ve.
	if res.State == domain.StateSoldOut {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"state":         domain.StateSoldOut,
			"queue_token":   "",
			"rank":          -1,
			"is_new":        false,
			"poll_after_ms": domain.PollInterval(domain.StateSoldOut, -1).Milliseconds(),
		})
		return
	}

	// res.Token la jti (thanh vien trong hang). Client nhan token KY gan jti voi nguoi + thiet bi + su kien.
	now := h.now()
	signed, err := authx.SignQueueToken(h.secret, authx.QueueClaims{
		EventID: eventID, Subject: identityID, DeviceID: p.DeviceID, JTI: res.Token,
		IssuedAt: now, ExpiresAt: now.Add(queueTokenTTL),
	})
	if err != nil {
		h.log.Error("ky queue token that bai", "event", eventID, "err", err)
		httpx.WriteProblem(w, http.StatusInternalServerError, "khong cap duoc queue token")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"state":         res.State,
		"queue_token":   signed,
		"rank":          res.Rank,
		"is_new":        res.IsNew,
		"poll_after_ms": domain.PollInterval(res.State, res.Rank).Milliseconds(),
	})
}

// queueClaims xac minh X-Queue-Token: chu ky, han, va token phai thuoc dung su kien, dung nguoi, dung
// thiet bi dang goi (EV-137). Tra false sau khi da ghi loi.
func (h *Handler) queueClaims(w http.ResponseWriter, r *http.Request, eventID string) (*authx.QueueClaims, bool) {
	token := r.Header.Get("X-Queue-Token")
	if token == "" {
		httpx.WriteProblem(w, http.StatusBadRequest, "thieu X-Queue-Token")
		return nil, false
	}
	p, ok := authx.PrincipalFromHeaders(r.Header)
	if !ok {
		httpx.WriteProblem(w, http.StatusUnauthorized, "can dang nhap")
		return nil, false
	}
	if h.secret == nil {
		httpx.WriteProblem(w, http.StatusServiceUnavailable, "phong cho chua cau hinh queue token")
		return nil, false
	}
	claims, err := authx.VerifyQueueToken(h.secret, token, h.now())
	switch {
	case errors.Is(err, authx.ErrExpiredQueueToken):
		httpx.WriteProblemType(w, http.StatusUnauthorized, ProblemQueueTokenExpired, "queue token da het han, hay vao hang cho lai")
		return nil, false
	case err != nil:
		httpx.WriteProblemType(w, http.StatusUnauthorized, ProblemQueueTokenInvalid, "queue token khong hop le")
		return nil, false
	case !claims.Matches(eventID, p.ID, p.DeviceID):
		// Token hop le nhung cua nguoi / thiet bi / su kien khac: token bi chia se hoac dung nham.
		h.log.Warn("queue token khong khop nguoi goi", "event", eventID)
		httpx.WriteProblemType(w, http.StatusForbidden, ProblemQueueTokenMismatch, "queue token khong thuoc ve ban hoac thiet bi nay")
		return nil, false
	}
	return claims, true
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("eventID")
	claims, ok := h.queueClaims(w, r, eventID)
	if !ok {
		return
	}

	pos, err := h.store.Status(r.Context(), eventID, claims.JTI)
	if err != nil {
		httpx.WriteProblem(w, http.StatusServiceUnavailable, "khong doc duoc trang thai hang cho")
		return
	}

	// ETag theo (state, rank lam tron tram): rank nhich vai bac khong dang gui
	// lai ca body cho hang tram nghin client. 304 re hon 200 rat nhieu.
	etag := fmt.Sprintf(`W/"%s-%d"`, pos.State, pos.Rank/100)
	if r.Header.Get("If-None-Match") == etag {
		w.Header().Set("ETag", etag)
		w.Header().Set("X-Poll-After-Ms", strconv.FormatInt(pos.PollAfterMs, 10))
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, pos)
}

// stream day trang thai qua SSE cho nhom SAP toi luot.
//
// SSE giu mot ket noi mo cho moi khach, nen chi danh cho nhom gan luot. Nhom o
// xa van dung polling gian -- 300.000 ket noi dong thoi tat ca deu SSE la cach
// chac chan nhat de het file descriptor.
func (h *Handler) stream(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("eventID")
	claims, ok := h.queueClaims(w, r, eventID)
	if !ok {
		return
	}
	token := claims.JTI
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.WriteProblem(w, http.StatusInternalServerError, "streaming khong duoc ho tro")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ctx := r.Context()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pos, err := h.store.Status(ctx, eventID, token)
			if err != nil {
				return
			}
			b, _ := json.Marshal(pos)
			// Ghi loi = client da ngat ket noi. Dung luon thay vi tiep tuc hoi store moi 2 giay
			// cho mot ket noi da chet.
			if _, err := fmt.Fprintf(w, "event: position\ndata: %s\n\n", b); err != nil {
				return
			}
			flusher.Flush()

			if pos.State == domain.StateAdmitted || pos.State == domain.StateExpired || pos.State == domain.StateSoldOut {
				return
			}
		}
	}
}

func newToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ProblemOTPRequired la "type" RFC 7807 client dung de chuyen khach sang man xac thuc OTP.
const ProblemOTPRequired = "/problems/otp-required"

// Loai loi queue token / thiet bi (EV-137): client dua vao "type" de quyet dinh join lai hay dang nhap lai.
const (
	ProblemDeviceRequired     = "/problems/device-required"
	ProblemQueueTokenInvalid  = "/problems/queue-token-invalid"
	ProblemQueueTokenExpired  = "/problems/queue-token-expired"
	ProblemQueueTokenMismatch = "/problems/queue-token-mismatch"
)
