package domain_test

// Test viet tu nghiep vu (docs/01-nghiep-vu.md §2.2), khong chep lai logic code:
//   - BR-Q1: LOBBY truoc T0 khong co so thu tu -> khong co ETA, nhip poll khong phu thuoc rank.
//   - BR-Q4: poll_after_ms do server quyet theo rank: xa -> 30s, gan luot -> 3s.
//   - Hop dong Position: eta_seconds = -1 khi chua uoc luong duoc, khong bao gio am kieu khac.

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/waitingroom/internal/domain"
)

const (
	nearCadence = 3 * time.Second  // BR-Q4: gan luot
	farCadence  = 30 * time.Second // BR-Q4: rank xa
)

func TestPollInterval_QueuedNearTurnPollsEvery3s(t *testing.T) {
	// BR-Q4: nguoi dung dau hang (sap toi luot) phai duoc hoi lai nhanh nhat.
	for _, rank := range []int64{0, 1, 2, 10} {
		if got := domain.PollInterval(domain.StateQueued, rank); got != nearCadence {
			t.Errorf("rank %d (gan luot): poll = %v, muon %v", rank, got, nearCadence)
		}
	}
}

func TestPollInterval_QueuedFarRankPollsEvery30s(t *testing.T) {
	// BR-Q4: rank xa (hang tram nghin, hang trieu) -> 30s de giam tai poll.
	for _, rank := range []int64{100_000, 300_000, 3_000_000, math.MaxInt64} {
		if got := domain.PollInterval(domain.StateQueued, rank); got != farCadence {
			t.Errorf("rank %d (xa): poll = %v, muon %v", rank, got, farCadence)
		}
	}
}

func TestPollInterval_QueuedIsMonotonicNonDecreasingInRank(t *testing.T) {
	// BR-Q4: cang xa luot thi khong bao gio duoc poll day hon nguoi gan luot hon.
	prev := domain.PollInterval(domain.StateQueued, 1)
	for rank := int64(2); rank <= 50_000; rank++ {
		cur := domain.PollInterval(domain.StateQueued, rank)
		if cur < prev {
			t.Fatalf("poll giam khi rank tang: rank %d -> %v, rank %d -> %v", rank-1, prev, rank, cur)
		}
		prev = cur
	}
}

func TestPollInterval_AlwaysBoundedByServerCadence(t *testing.T) {
	// BR-Q4: server luon cap mot nhip trong [3s, 30s] cho moi trang thai -- khong bao gio
	// tra 0 (client se poll lien tuc) va khong bat ai cho lau hon nhip xa nhat.
	states := []domain.State{
		domain.StateLobby, domain.StateQueued, domain.StateAdmitted,
		domain.StateExpired, domain.StateUnknown, domain.State("KHONG_HOP_LE"),
	}
	ranks := []int64{-1, 0, 1, 499, 500, 9_999, 10_000, 1_000_000}
	for _, s := range states {
		for _, r := range ranks {
			got := domain.PollInterval(s, r)
			if got < nearCadence || got > farCadence {
				t.Errorf("state %s rank %d: poll = %v nam ngoai [%v, %v]", s, r, got, nearCadence, farCadence)
			}
		}
	}
}

func TestPollInterval_LobbyDoesNotDependOnRankAndIsNotFastLane(t *testing.T) {
	// BR-Q1: LOBBY khong co so thu tu, vao som hay muon deu nhu nhau -> nhip poll
	// khong duoc phu thuoc rank, va khong co ly do cap nhip "gan luot" (3s) cho ai o LOBBY.
	base := domain.PollInterval(domain.StateLobby, -1)
	if base <= nearCadence {
		t.Errorf("LOBBY poll = %v, khong duoc nhanh bang nhip gan luot %v", base, nearCadence)
	}
	for _, r := range []int64{0, 1, 42, 1_000_000} {
		if got := domain.PollInterval(domain.StateLobby, r); got != base {
			t.Errorf("LOBBY rank %d: poll = %v, khac voi %v -- LOBBY khong duoc co loi the theo rank", r, got, base)
		}
	}
}

func TestEstimateETA_ZeroOrNegativeAdmitRateDoesNotPanic(t *testing.T) {
	// admit_rate = 0 (vd Ops tam dung) khong duoc chia cho 0 / panic; chua uoc luong
	// duoc thi tra -1 theo hop dong cua Position.
	for _, rate := range []float64{0, -1, -0.5, 0.000001} {
		got := domain.EstimateETA(1_000, rate)
		if got != -1 {
			t.Errorf("rate %v: ETA = %d, muon -1 (chua uoc luong duoc)", rate, got)
		}
	}
}

func TestEstimateETA_NoRankMeansNoETA(t *testing.T) {
	// BR-Q1: o LOBBY chua co rank (-1) -> khong duoc dua ra ETA.
	for _, rank := range []int64{-1, 0, -100} {
		if got := domain.EstimateETA(rank, 50); got != -1 {
			t.Errorf("rank %d: ETA = %d, muon -1", rank, got)
		}
	}
}

func TestEstimateETA_NeverNegativeExceptSentinel(t *testing.T) {
	ranks := []int64{-5, -1, 0, 1, 7, 500, 10_000, 3_000_000}
	rates := []float64{-10, 0, 0.01, 0.5, 1, 5, 100, 500, 1e9, math.Inf(1)}
	for _, r := range ranks {
		for _, a := range rates {
			got := domain.EstimateETA(r, a)
			if got < -1 {
				t.Errorf("rank %d rate %v: ETA = %d am (chi -1 la gia tri 'chua biet')", r, a, got)
			}
			if r > 0 && a >= 1 && got < 0 {
				t.Errorf("rank %d rate %v: du co so ma ETA = %d", r, a, got)
			}
		}
	}
}

func TestEstimateETA_CloseToRankOverRate(t *testing.T) {
	// ETA phai sat voi thuc te: rank / admit_rate giay, khong lech qua mot buoc lam tron nho.
	cases := []struct {
		rank int64
		rate float64
	}{
		{1, 1}, {100, 10}, {1_000, 5}, {30_000, 500}, {3_000_000, 500}, {7, 0.5},
	}
	for _, c := range cases {
		want := float64(c.rank) / c.rate
		got := float64(domain.EstimateETA(c.rank, c.rate))
		if got < math.Floor(want) || got > want+15 {
			t.Errorf("rank %d rate %v: ETA = %v, muon xap xi %v (trong [floor, +15s])", c.rank, c.rate, got, want)
		}
	}
}

func TestEstimateETA_MonotonicInRankAndRate(t *testing.T) {
	// Cang xa luot thi ETA khong nho hon; admit nhanh hon thi ETA khong lon hon.
	const rate = 20.0
	prev := domain.EstimateETA(1, rate)
	for rank := int64(2); rank <= 20_000; rank++ {
		cur := domain.EstimateETA(rank, rate)
		if cur < prev {
			t.Fatalf("ETA giam khi rank tang: rank %d -> %d, rank %d -> %d", rank-1, prev, rank, cur)
		}
		prev = cur
	}
	const rank = 12_345
	prevRate := domain.EstimateETA(rank, 1)
	for r := 2.0; r <= 500; r++ {
		cur := domain.EstimateETA(rank, r)
		if cur > prevRate {
			t.Fatalf("ETA tang khi admit_rate tang: rate %v -> %d, rate %v -> %d", r-1, prevRate, r, cur)
		}
		prevRate = cur
	}
}

func TestBuild_QueuedFarCustomer(t *testing.T) {
	p := domain.Build(domain.StateQueued, 200_000, 300_000, 100, 0)
	if p.State != domain.StateQueued || p.Rank != 200_000 || p.QueueDepth != 300_000 || p.AdmitRate != 100 {
		t.Fatalf("Build lam sai du lieu dau vao: %+v", p)
	}
	if p.PollAfterMs != farCadence.Milliseconds() {
		t.Errorf("BR-Q4: rank xa poll_after_ms = %d, muon %d", p.PollAfterMs, farCadence.Milliseconds())
	}
	if p.EtaSeconds < 2_000 || p.EtaSeconds > 2_015 {
		t.Errorf("ETA = %d, muon ~2000s (200000 / 100)", p.EtaSeconds)
	}
	if p.ExpiresAt != 0 {
		t.Errorf("QUEUED khong co han admit, ExpiresAt = %d", p.ExpiresAt)
	}
}

func TestBuild_QueuedNearTurnPolls3s(t *testing.T) {
	p := domain.Build(domain.StateQueued, 3, 10, 50, 0)
	if p.PollAfterMs != nearCadence.Milliseconds() {
		t.Errorf("BR-Q4: gan luot poll_after_ms = %d, muon %d", p.PollAfterMs, nearCadence.Milliseconds())
	}
	if p.EtaSeconds < 0 {
		t.Errorf("ETA = %d, du co so de uoc luong", p.EtaSeconds)
	}
}

func TestBuild_LobbyHasNoRankAndNoETA(t *testing.T) {
	// BR-Q1: truoc T0 khach o LOBBY khong co so thu tu, khong co ETA.
	p := domain.Build(domain.StateLobby, -1, 0, 0, 0)
	if p.State != domain.StateLobby {
		t.Fatalf("state = %s", p.State)
	}
	if p.Rank != -1 {
		t.Errorf("LOBBY rank = %d, muon -1", p.Rank)
	}
	if p.EtaSeconds != -1 {
		t.Errorf("LOBBY ETA = %d, muon -1", p.EtaSeconds)
	}
	if p.PollAfterMs <= 0 {
		t.Errorf("poll_after_ms = %d, server phai luon cap mot nhip duong", p.PollAfterMs)
	}
}

func TestBuild_AdmittedKeepsExpiry(t *testing.T) {
	// BR-Q7: suat admit co han -> expires_at phai den tay client nguyen ven.
	exp := time.Date(2026, 10, 8, 20, 15, 0, 0, time.UTC).UnixMilli()
	p := domain.Build(domain.StateAdmitted, 0, 0, 100, exp)
	if p.ExpiresAt != exp {
		t.Errorf("ExpiresAt = %d, muon %d", p.ExpiresAt, exp)
	}
	if p.PollAfterMs <= 0 {
		t.Errorf("poll_after_ms = %d", p.PollAfterMs)
	}
}

func TestPosition_JSONContractForClient(t *testing.T) {
	// BR-Q4: client doc truong `poll_after_ms`; ten truong la hop dong voi frontend.
	b, err := json.Marshal(domain.Build(domain.StateQueued, 42, 100, 5, 0))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"state", "rank", "queue_depth", "admit_rate", "eta_seconds", "poll_after_ms"} {
		if _, ok := m[k]; !ok {
			t.Errorf("JSON thieu truong %q: %s", k, b)
		}
	}
	if _, ok := m["expires_at"]; ok {
		t.Errorf("expires_at phai bi bo khi chua admit: %s", b)
	}
	if m["state"] != string(domain.StateQueued) {
		t.Errorf("state = %v", m["state"])
	}
}
