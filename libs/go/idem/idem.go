// Package idem hien thuc BR-O5: moi API ghi bat buoc co header Idempotency-Key; goi lai cung
// key trong 24 gio tra DUNG response cu, khong chay lai nghiep vu.
//
// Dung cho tung route ghi (khong gan vao chuoi middleware toan cuc vi GET khong can):
//
//	mux.Handle("POST /v1/events/{eventID}/holds",
//		idem.Middleware(idem.NewPostgresStore(pool), idem.WithIdentity(identityOf))(h))
//
// # Hop dong
//
//   - Thieu hoac sai dinh dang header: 400.
//   - Cung key + cung noi dung + da xong: phat lai y nguyen status va body, kem header
//     Idempotent-Replayed: true. Handler KHONG chay lai.
//   - Cung key dang duoc xu ly boi request khac: 409, kem Retry-After. Handler khong chay.
//   - Cung key nhung noi dung hoac nguoi goi khac: 422. Khong bao gio tra response cua nguoi khac.
//   - Response 5xx va panic khong duoc nho: key bi xoa de client thu lai that su chay lai.
//
// # Gioi han (doc truoc khi dung)
//
//   - Phan hoi duoc dem het trong bo nho roi moi tra ve, nen KHONG gan len route stream (SSE).
//   - Key luu rieng, KHONG cung transaction voi nghiep vu. Neu tien trinh chet giua luc handler
//     xong va luc ghi key, sau 30 giay (lease) request khac se chay lai handler. An toan voi
//     tao hold nho BR-O3; API co tac dung phu khong idempotent (thanh toan) phai tu co khoa
//     phia ben thu ba.
//   - Dinh danh nguoi goi do WithIdentity cung cap. Truoc khi co JWT (giai doan 3) do la header
//     client tu khai, nen chua phai ranh gioi bao mat.
package idem

import (
	"context"
	"time"
)

const (
	// Header la ten header mang khoa idempotency.
	Header = "Idempotency-Key"
	// ReplayedHeader duoc dat tren response phat lai.
	ReplayedHeader = "Idempotent-Replayed"

	// DefaultTTL: BR-O5 yeu cau giu response 24 gio.
	DefaultTTL = 24 * time.Hour
	// DefaultLease la thoi gian toi da mot request duoc coi la "dang xu ly". Qua han thi request
	// khac duoc tiep quan (tien trinh cu co the da chet).
	DefaultLease = 30 * time.Second
	// DefaultMaxBody gioi han body doc de tinh hash.
	DefaultMaxBody = 1 << 20
)

// State la ket qua cua Store.Begin.
type State int

const (
	// Acquired: khoa thuoc ve request nay, hay chay handler.
	Acquired State = iota
	// Replay: da co response, phat lai.
	Replay
	// InProgress: request khac dang xu ly cung khoa.
	InProgress
	// Mismatch: khoa da dung cho request khac (noi dung hoac nguoi goi khac).
	Mismatch
)

// Request dinh danh mot lan goi.
type Request struct {
	Key      string
	Endpoint string // "<METHOD> <path>"
	Identity string // co the rong
	Hash     string // SHA-256 cua (method, path, query, identity, body)
}

// Response la phan can phat lai.
type Response struct {
	Status      int
	ContentType string
	Body        []byte
}

// Outcome la ket qua Store.Begin.
type Outcome struct {
	State    State
	Token    string   // Acquired: quyen so huu, truyen lai cho Complete/Abandon
	Response Response // Replay
}

// Store luu trang thai khoa. Hai request cung (Key, Endpoint) chi mot request nhan Acquired.
type Store interface {
	// Begin gianh khoa. Khoa het han (qua ttl) hoac dang xu ly qua lease duoc giao lai.
	Begin(ctx context.Context, req Request, ttl, lease time.Duration) (Outcome, error)
	// Complete luu response. Khong lam gi (tra ErrLeaseLost) neu khoa da bi tiep quan.
	Complete(ctx context.Context, req Request, token string, res Response) error
	// Abandon xoa khoa cua request nay de client thu lai duoc. Khong xoa khoa cua nguoi khac.
	Abandon(ctx context.Context, req Request, token string) error
}
