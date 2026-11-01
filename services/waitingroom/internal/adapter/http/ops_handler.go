package httpadapter

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
	"github.com/eventflow/eventflow/libs/go/httpx"
	"github.com/eventflow/eventflow/services/waitingroom/internal/app"
)

// Problem type cua Ops API (contracts 3).
const (
	ProblemForbidden        = "/problems/forbidden"
	ProblemNotFound         = "/problems/not-found"
	ProblemInvalidOverride  = "/problems/invalid-override"
	ProblemAuditUnavailable = "/problems/audit-unavailable"
)

// maxOverrideBody gioi han than request ghi de (contracts 3.2).
const maxOverrideBody = 4 << 10

var eventUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// OpsHandler phuc vu Ops API cua phong cho (EVF-60, EVF-117): doc trang thai, dat / bo ghi de admit rate.
// Gateway da chan role khac ops; handler kiem lai role (phong thu hai lop).
type OpsHandler struct {
	state app.OpsStateReader
	ov    *app.OpsOverride
	log   *slog.Logger
	now   func() time.Time
}

func NewOpsHandler(state app.OpsStateReader, ov *app.OpsOverride, log *slog.Logger) *OpsHandler {
	if log == nil {
		log = slog.Default()
	}
	return &OpsHandler{state: state, ov: ov, log: log, now: time.Now}
}

func (h *OpsHandler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/ops/events/{eventID}/queue", h.queue)
	mux.HandleFunc("PUT /v1/ops/events/{eventID}/admit-override", h.setOverride)
	mux.HandleFunc("DELETE /v1/ops/events/{eventID}/admit-override", h.clearOverride)
}

// --- wire types (contracts 3.1 - 3.3) ---

type overrideJSON struct {
	Active     bool     `json:"active"`
	RatePerSec *float64 `json:"rate_per_sec"`
}

type controllerJSON struct {
	LeaderPresent bool  `json:"leader_present"`
	Epoch         int64 `json:"epoch"`
}

type checkoutJSON struct {
	Signal      string   `json:"signal"`
	Pods        int      `json:"pods"`
	P99Ms       *float64 `json:"p99_ms"`
	ErrorRate   *float64 `json:"error_rate"`
	DBPoolUsage *float64 `json:"db_pool_usage"`
}

type queueStateJSON struct {
	EventID         string         `json:"event_id"`
	GeneratedAt     string         `json:"generated_at"`
	Lobby           int64          `json:"lobby"`
	Queued          int64          `json:"queued"`
	AdmittedActive  int64          `json:"admitted_active"`
	AdmitRatePerSec float64        `json:"admit_rate_per_sec"`
	Override        overrideJSON   `json:"override"`
	Paused          bool           `json:"paused"`
	SoldOut         bool           `json:"sold_out"`
	StockZero       bool           `json:"stock_zero"`
	Controller      controllerJSON `json:"controller"`
	Checkout        checkoutJSON   `json:"checkout"`
}

type overrideResultJSON struct {
	EventID   string       `json:"event_id"`
	Override  overrideJSON `json:"override"`
	Paused    bool         `json:"paused"`
	AppliedAt string       `json:"applied_at"`
	Audit     string       `json:"audit"`
}

// authorize: 401 khi thieu danh tinh, 403 khi role khac ops, 400 khi eventID khong phai UUID.
func (h *OpsHandler) authorize(w http.ResponseWriter, r *http.Request) (authx.Principal, string, bool) {
	p, ok := authx.PrincipalFromHeaders(r.Header)
	if !ok {
		httpx.WriteProblem(w, http.StatusUnauthorized, "can dang nhap")
		return authx.Principal{}, "", false
	}
	if p.Role != "ops" {
		httpx.WriteProblemType(w, http.StatusForbidden, ProblemForbidden, "chi Ops duoc dung chuc nang nay")
		return authx.Principal{}, "", false
	}
	eventID := r.PathValue("eventID")
	if !eventUUID.MatchString(eventID) {
		httpx.WriteProblem(w, http.StatusBadRequest, "ma su kien khong hop le")
		return authx.Principal{}, "", false
	}
	return p, eventID, true
}

func (h *OpsHandler) queue(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	_, eventID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	now := h.now()
	st, err := h.state.QueueOpsState(r.Context(), eventID, now)
	switch {
	case errors.Is(err, app.ErrRoomNotConfigured):
		httpx.WriteProblemType(w, http.StatusNotFound, ProblemNotFound, "su kien chua co phong cho")
		return
	case err != nil:
		h.log.Error("doc trang thai phong cho cho Ops that bai", "event", eventID, "err", err)
		httpx.WriteProblem(w, http.StatusServiceUnavailable, "khong doc duoc trang thai phong cho")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, queueStateJSON{
		EventID:         eventID,
		GeneratedAt:     now.UTC().Format(time.RFC3339Nano),
		Lobby:           st.Lobby,
		Queued:          st.Queued,
		AdmittedActive:  st.AdmittedActive,
		AdmitRatePerSec: st.AdmitRatePerSec,
		Override:        toOverrideJSON(st.Override),
		Paused:          st.Override.Paused(),
		SoldOut:         st.SoldOut,
		StockZero:       st.StockZero,
		Controller:      controllerJSON{LeaderPresent: st.Controller.LeaderPresent, Epoch: st.Controller.Epoch},
		Checkout:        toCheckoutJSON(st.Checkout),
	})
}

func toOverrideJSON(o app.OverrideState) overrideJSON {
	if !o.Active {
		return overrideJSON{Active: false}
	}
	return overrideJSON{Active: true, RatePerSec: o.RatePerSec}
}

func toCheckoutJSON(c app.CheckoutState) checkoutJSON {
	sig := c.Signal
	if sig == "" {
		sig = app.CheckoutSignalNone
	}
	if sig != app.CheckoutSignalOK {
		return checkoutJSON{Signal: sig}
	}
	return checkoutJSON{Signal: sig, Pods: c.Pods, P99Ms: c.P99Ms, ErrorRate: c.ErrorRate, DBPoolUsage: c.DBPoolUsage}
}

// overrideBody: rate giu dang tho de tu choi chuoi ("12"), null va so le; reason la *string de phan biet thieu.
type overrideBody struct {
	RatePerSec json.RawMessage `json:"rate_per_sec"`
	Reason     *string         `json:"reason"`
}

type clearBody struct {
	Reason *string `json:"reason"`
}

// decodeStrict doc JSON <= 4 KiB, cam truong la va du lieu thua phia sau.
func decodeStrict(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxOverrideBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return false
	}
	return true
}

// parseRate chi nhan so JSON (khong phai chuoi/null). Pham vi va tinh nguyen do app.ValidRate quyet.
func parseRate(raw json.RawMessage) (float64, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || !(raw[0] == '-' || (raw[0] >= '0' && raw[0] <= '9')) {
		return 0, false
	}
	v, err := strconv.ParseFloat(string(raw), 64)
	if err != nil {
		return 0, false
	}
	return v, app.ValidRate(v)
}

func (h *OpsHandler) setOverride(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	p, eventID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	var body overrideBody
	if !decodeStrict(w, r, &body) || body.Reason == nil {
		writeInvalidOverride(w)
		return
	}
	rate, ok := parseRate(body.RatePerSec)
	if !ok {
		writeInvalidOverride(w)
		return
	}
	res, err := h.ov.Set(r.Context(), h.command(r, p, eventID, *body.Reason), rate)
	h.writeResult(w, eventID, res, err)
}

func (h *OpsHandler) clearOverride(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	p, eventID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	var body clearBody
	if !decodeStrict(w, r, &body) || body.Reason == nil {
		writeInvalidOverride(w)
		return
	}
	res, err := h.ov.Clear(r.Context(), h.command(r, p, eventID, *body.Reason))
	h.writeResult(w, eventID, res, err)
}

func (h *OpsHandler) command(r *http.Request, p authx.Principal, eventID, reason string) app.OverrideCommand {
	reqID := httpx.RequestIDFrom(r.Context())
	if reqID == "" {
		reqID = r.Header.Get(httpx.RequestIDHeader)
	}
	return app.OverrideCommand{ActorID: p.ID, EventID: eventID, Reason: reason, RequestID: reqID}
}

func writeInvalidOverride(w http.ResponseWriter) {
	httpx.WriteProblemType(w, http.StatusBadRequest, ProblemInvalidOverride,
		"rate_per_sec phai la so nguyen 0..10000 va reason dai 3..200 ky tu")
}

func (h *OpsHandler) writeResult(w http.ResponseWriter, eventID string, res app.OverrideResult, err error) {
	switch {
	case errors.Is(err, app.ErrInvalidOverride):
		writeInvalidOverride(w)
	case errors.Is(err, app.ErrAuditUnavailable):
		httpx.WriteProblemType(w, http.StatusServiceUnavailable, ProblemAuditUnavailable,
			"chua cau hinh kho nhat ky kiem toan, khong the ghi de")
	case errors.Is(err, app.ErrRoomNotConfigured):
		httpx.WriteProblemType(w, http.StatusNotFound, ProblemNotFound, "su kien chua co phong cho")
	case err != nil:
		h.log.Error("ghi de admit rate that bai", "event", eventID, "err", err)
		httpx.WriteProblem(w, http.StatusServiceUnavailable, "khong ap dung duoc lenh, vui long thu lai")
	default:
		httpx.WriteJSON(w, http.StatusOK, overrideResultJSON{
			EventID:   eventID,
			Override:  overrideJSON{Active: res.Active, RatePerSec: res.RatePerSec},
			Paused:    res.Paused,
			AppliedAt: res.AppliedAt.Format(time.RFC3339),
			Audit:     res.Audit,
		})
	}
}
