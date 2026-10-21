package app_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

type stubVerifier struct {
	res port.VerifyTicketResult
	err error
}

func (s stubVerifier) Execute(context.Context, port.VerifyTicketParams) (port.VerifyTicketResult, error) {
	return s.res, s.err
}

// fakeUpdater mo phong UPDATE ... WHERE status = current: dung mot nguoi doi duoc ISSUED -> CHECKED_IN.
type fakeUpdater struct {
	mu     sync.Mutex
	status domain.TicketStatus
	calls  atomic.Int32
	err    error
}

func (f *fakeUpdater) UpdateTicketStatus(_ context.Context, _ string, cur, next domain.TicketStatus, _ *time.Time) error {
	f.calls.Add(1)
	if f.err != nil {
		return f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.status != cur {
		return domain.ErrTicketAlreadyUsed
	}
	f.status = next
	return nil
}

func checkInUC(v stubVerifier, u *fakeUpdater) *app.CheckInTicketUseCase {
	return app.NewCheckInTicketUseCase(v, u, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func okVerify() stubVerifier {
	return stubVerifier{res: port.VerifyTicketResult{Valid: true, Ticket: domain.Ticket{ID: "t-1", OrderID: "o-1", Status: domain.TicketStatusIssued}}}
}

func TestCheckIn_ThanhCongDoiTrangThaiVaGhiGio(t *testing.T) {
	now := time.Date(2026, 10, 30, 19, 0, 0, 0, time.UTC)
	u := &fakeUpdater{status: domain.TicketStatusIssued}
	res, err := checkInUC(okVerify(), u).WithClock(func() time.Time { return now }).
		Execute(context.Background(), port.VerifyTicketParams{QRPayload: "qr"})
	if err != nil || !res.Valid {
		t.Fatalf("Execute = %+v, %v", res, err)
	}
	if res.Ticket.Status != domain.TicketStatusCheckedIn || res.Ticket.CheckedInAt == nil || !res.Ticket.CheckedInAt.Equal(now) {
		t.Errorf("ve sau check-in = %+v", res.Ticket)
	}
	if u.status != domain.TicketStatusCheckedIn {
		t.Errorf("trang thai luu = %s", u.status)
	}
}

// Kiem QR loi (gia mao, het han, da dung, ...) thi KHONG cham vao trang thai ve.
func TestCheckIn_KiemQRLoiThiKhongDoiTrangThai(t *testing.T) {
	for _, verr := range []error{domain.ErrTamperedQR, domain.ErrTicketAlreadyUsed, domain.ErrTicketNotFound, domain.ErrTicketVoid} {
		u := &fakeUpdater{status: domain.TicketStatusIssued}
		_, err := checkInUC(stubVerifier{err: verr}, u).Execute(context.Background(), port.VerifyTicketParams{QRPayload: "qr"})
		if !errors.Is(err, verr) || u.calls.Load() != 0 {
			t.Errorf("%v: err = %v, UpdateTicketStatus goi %d lan", verr, err, u.calls.Load())
		}
	}
}

// 50 may quet cung luc cung mot ve: dung mot may vao duoc, 49 may con lai nhan ErrTicketAlreadyUsed.
func TestCheckIn_QuetDongThoiChiMotMayThanhCong(t *testing.T) {
	u := &fakeUpdater{status: domain.TicketStatusIssued}
	uc := checkInUC(okVerify(), u)
	const scanners = 50
	var wins, used atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < scanners; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := uc.Execute(context.Background(), port.VerifyTicketParams{QRPayload: "qr"})
			switch {
			case err == nil && res.Valid:
				wins.Add(1)
			case errors.Is(err, domain.ErrTicketAlreadyUsed) && !res.Valid:
				used.Add(1)
			default:
				t.Errorf("ket qua bat ngo: %+v, %v", res, err)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 || used.Load() != scanners-1 {
		t.Fatalf("thang %d, bi tu choi %d; muon 1 va %d", wins.Load(), used.Load(), scanners-1)
	}
}

func TestCheckIn_LoiHaTangDuocTraNguyen(t *testing.T) {
	boom := errors.New("db mat ket noi")
	_, err := checkInUC(okVerify(), &fakeUpdater{err: boom}).Execute(context.Background(), port.VerifyTicketParams{QRPayload: "qr"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}
