// Package domain chua nghiep vu thuan tuy cua phong cho.
//
// Khong import HTTP, Redis hay bat ky ha tang nao -- nho vay moi quy tac o day
// deu test duoc bang unit test chay trong vai mili giay.
package domain

import "time"

// State la trang thai mot phien xep hang.
type State string

const (
	StateLobby    State = "LOBBY"    // truoc T0, chua co so thu tu (BR-Q1)
	StateQueued   State = "QUEUED"   // da co rank
	StateAdmitted State = "ADMITTED" // duoc vao mua ve
	StateExpired  State = "EXPIRED"  // het TTL admit ma chua mua (BR-Q7)
	StateUnknown  State = "UNKNOWN"
)

// Position la trang thai tra ve cho client moi lan hoi.
type Position struct {
	State       State  `json:"state"`
	Rank        int64  `json:"rank"`          // -1 khi con o LOBBY
	QueueDepth  int64  `json:"queue_depth"`
	AdmitRate   float64 `json:"admit_rate"`   // nguoi/giay
	EtaSeconds  int64  `json:"eta_seconds"`   // -1 khi chua uoc luong duoc
	PollAfterMs int64  `json:"poll_after_ms"` // server quyet dinh (BR-Q4)
	ExpiresAt   int64  `json:"expires_at,omitempty"`
}

// Cac nguong polling. Day la mot trong nhung con so anh huong lon nhat toi tai
// he thong: 300.000 nguoi poll moi 3 giay la 100k rps, con poll moi 30 giay chi
// la 10k rps. Nen khach o xa luot duoc gian ra, khach gan luot duoc uu tien.
const (
	pollNear   = 3 * time.Second  // rank < 500  -- sap toi luot
	pollMid    = 10 * time.Second // rank < 10000
	pollFar    = 30 * time.Second // con lai
	pollLobby  = 15 * time.Second // truoc T0: chi cho toi gio, khong voi
	minAdmitRate = 0.01
)

// PollInterval quyet dinh khi nao client duoc hoi lai.
//
// Client KHONG duoc tu chon chu ky poll: bot se chon 100ms. Server la noi ra
// quyet dinh, va gateway se chan nhung ai poll day hon muc da cap.
func PollInterval(state State, rank int64) time.Duration {
	switch state {
	case StateLobby:
		return pollLobby
	case StateAdmitted, StateExpired:
		return pollNear
	case StateQueued:
		switch {
		case rank <= 0 || rank < 500:
			return pollNear
		case rank < 10000:
			return pollMid
		default:
			return pollFar
		}
	default:
		return pollFar
	}
}

// EstimateETA uoc luong thoi gian con phai cho.
//
// Tra ve -1 khi chua du co so -- tha khong noi gi con hon dua ra mot con so sai.
// Khach mat long tin vi ETA nhay loan hon la vi khong co ETA.
func EstimateETA(rank int64, admitRate float64) int64 {
	if rank <= 0 || admitRate < minAdmitRate {
		return -1
	}
	eta := float64(rank) / admitRate
	// Lam tron len boi 15 giay: ETA nhay tung giay trong khi hang cho bien dong
	// chi lam khach lo lang. Lam tron cho cam giac on dinh.
	const round = 15
	return ((int64(eta) + round - 1) / round) * round
}

// Build gop cac manh lai thanh cau tra loi hoan chinh cho client.
func Build(state State, rank, depth int64, admitRate float64, expiresAt int64) Position {
	return Position{
		State:       state,
		Rank:        rank,
		QueueDepth:  depth,
		AdmitRate:   admitRate,
		EtaSeconds:  EstimateETA(rank, admitRate),
		PollAfterMs: PollInterval(state, rank).Milliseconds(),
		ExpiresAt:   expiresAt,
	}
}
