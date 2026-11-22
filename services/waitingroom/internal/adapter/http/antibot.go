package httpadapter

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strings"

	"github.com/eventflow/eventflow/libs/go/antibot"
	"github.com/eventflow/eventflow/libs/go/authx"
	"github.com/eventflow/eventflow/libs/go/httpx"
	"github.com/eventflow/eventflow/services/waitingroom/internal/domain"
)

// ProblemRequestDenied la "type" cua 403 khi anti-bot chan (D6). Thong diep chung chung, khong lo luat nao.
// /problems/challenge-required de danh cho EV-176.
const ProblemRequestDenied = "/problems/request-denied"

// joinWithAntibot cham diem lan join va ap Applied (contracts 2). denied = da ghi 403.
//
//	PASS      -> Join (shadow) hoac JoinLane(n) (enforce); cho da co duoc tra nguyen trang
//	SLOW_LANE -> JoinLane(s, P); store khong co LaneStore -> Join + canh bao
//	BLOCK     -> Revoke (dat co chan) + 403; store khong co LaneStore -> chi 403
//	JoinLane tra BLOCKED (co chan do mot BLOCK dong thoi) -> 403
func (h *Handler) joinWithAntibot(w http.ResponseWriter, r *http.Request, eventID string, p authx.Principal) (JoinResult, bool, error) {
	identityID := p.ID
	in := antibot.Input{
		EventID: eventID, IdentityID: identityID, DeviceID: p.DeviceID,
		UserAgent: truncateUA(r.UserAgent()), ClientIP: clientIP(r), Now: h.now(),
	}
	in.RawSignals, in.BodyErr = readSignals(w, r)
	d := h.bots.Decide(r.Context(), in)

	lanes, hasLanes := h.store.(LaneStore)
	deny := func() (JoinResult, bool, error) {
		httpx.WriteProblemType(w, http.StatusForbidden, ProblemRequestDenied, "yeu cau vao hang cho bi tu choi")
		return JoinResult{}, true, nil
	}
	joinLane := func(lane domain.Lane) (JoinResult, bool, error) {
		res, err := lanes.JoinLane(r.Context(), eventID, identityID, newToken(), lane, h.slowPenalty)
		if err == nil && res.State == domain.StateBlocked {
			return deny()
		}
		return res, false, err
	}

	switch d.Applied {
	case antibot.ActionBlock:
		// Chi ma luat va tag; khong fp / IP / UA tho (AC-15).
		h.log.Info("antibot chan lan join", "event", eventID, "action", d.Action, "score", d.Score,
			"rules", ruleList(d), "tags", strings.Join(d.Tags, ","))
		// STICKY: the first BLOCK already removed the slot and set the flag; revoking again would only cost a
		// script call (and, before R-2, extended the 24 h window).
		if !hasLanes || hasTag(d, antibot.TagSticky) {
			return deny()
		}
		revoked, err := lanes.Revoke(r.Context(), eventID, identityID)
		switch {
		case err != nil:
			h.log.Error("antibot revoke that bai", "event", eventID, "err", err)
		case revoked && h.abMetrics != nil:
			h.abMetrics.SlotRevoked.Inc()
		}
		return deny()
	case antibot.ActionSlowLane:
		if !hasLanes {
			h.log.Warn("antibot slow lane nhung store khong ho tro lane, dung lane thuong", "event", eventID)
			res, err := h.store.Join(r.Context(), eventID, identityID, newToken())
			return res, false, err
		}
		return joinLane(domain.LaneSlow)
	default:
		if !hasLanes || hasTag(d, antibot.TagModeShadow) {
			res, err := h.store.Join(r.Context(), eventID, identityID, newToken())
			return res, false, err
		}
		return joinLane(domain.LaneNormal)
	}
}

// readSignals doc body (toi da 4 KiB) va tach gia tri "signals" tho. Body qua lon / JSON hong khong bao gio lam
// join that bai: anti-bot coi tin hieu la thieu (AC-3).
func readSignals(w http.ResponseWriter, r *http.Request) (json.RawMessage, antibot.InvalidReason) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, antibot.MaxSignalBytes))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return nil, antibot.ReasonTooLarge
		}
		return nil, antibot.ReasonBadJSON
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, ""
	}
	var req joinRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, antibot.ReasonBadJSON
	}
	if len(req.Signals) == 0 {
		return nil, ""
	}
	return req.Signals, ""
}

func truncateUA(ua string) string {
	if len(ua) > maxUAForAntibot {
		return ua[:maxUAForAntibot]
	}
	return ua
}

// clientIP la phan tu dau cua X-Forwarded-For (gateway dat no la client da phan giai); thieu / hong -> zero.
func clientIP(r *http.Request) netip.Addr {
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return netip.Addr{}
	}
	first, _, _ := strings.Cut(xff, ",")
	a, err := netip.ParseAddr(strings.TrimSpace(first))
	if err != nil {
		return netip.Addr{}
	}
	return a.WithZone("")
}

func hasTag(d antibot.Decision, tag string) bool {
	for _, t := range d.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

func ruleList(d antibot.Decision) string {
	s := make([]string, len(d.Fired))
	for i, id := range d.Fired {
		s[i] = string(id)
	}
	return strings.Join(s, ",")
}
