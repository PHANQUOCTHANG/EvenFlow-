// Package httpadapter phoi bay phong cho qua REST + SSE.
package httpadapter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

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
}

func New(store QueueStore, bots BotGuard, log *slog.Logger) *Handler {
	return &Handler{store: store, bots: bots, log: log}
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
	identityID, ok := identityFrom(r)
	if !ok {
		problem(w, http.StatusUnauthorized, "can dang nhap va xac thuc OTP truoc khi xep hang")
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
			problem(w, http.StatusForbidden, "can xac minh them truoc khi vao hang cho")
			return
		case !allow:
			problem(w, http.StatusForbidden, "yeu cau bi tu choi")
			return
		}
	}

	res, err := h.store.Join(r.Context(), eventID, identityID, newToken())
	if err != nil {
		h.log.Error("join that bai", "event", eventID, "err", err)
		problem(w, http.StatusServiceUnavailable, "phong cho dang qua tai, vui long thu lai")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"state":         res.State,
		"queue_token":   res.Token,
		"rank":          res.Rank,
		"is_new":        res.IsNew,
		"poll_after_ms": domain.PollInterval(res.State, res.Rank).Milliseconds(),
	})
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("eventID")
	token := r.Header.Get("X-Queue-Token")
	if token == "" {
		problem(w, http.StatusBadRequest, "thieu X-Queue-Token")
		return
	}

	pos, err := h.store.Status(r.Context(), eventID, token)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "khong doc duoc trang thai hang cho")
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
	writeJSON(w, http.StatusOK, pos)
}

// stream day trang thai qua SSE cho nhom SAP toi luot.
//
// SSE giu mot ket noi mo cho moi khach, nen chi danh cho nhom gan luot. Nhom o
// xa van dung polling gian -- 300.000 ket noi dong thoi tat ca deu SSE la cach
// chac chan nhat de het file descriptor.
func (h *Handler) stream(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("eventID")
	token := r.Header.Get("X-Queue-Token")
	if token == "" {
		problem(w, http.StatusBadRequest, "thieu X-Queue-Token")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		problem(w, http.StatusInternalServerError, "streaming khong duoc ho tro")
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
			fmt.Fprintf(w, "event: position\ndata: %s\n\n", b)
			flusher.Flush()

			if pos.State == domain.StateAdmitted || pos.State == domain.StateExpired {
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

// identityFrom lay identity da duoc gateway xac thuc va dat vao context.
func identityFrom(r *http.Request) (string, bool) {
	if v, ok := r.Context().Value(ctxKeyIdentity).(string); ok && v != "" {
		return v, true
	}
	// Fallback cho local dev khi chay khong co gateway.
	if v := r.Header.Get("X-Identity-Id"); v != "" {
		return v, true
	}
	return "", false
}

type ctxKey string

const ctxKeyIdentity ctxKey = "identity_id"

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// problem tra loi theo RFC 7807.
func problem(w http.ResponseWriter, code int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "about:blank",
		"title":  http.StatusText(code),
		"status": code,
		"detail": detail,
	})
}
