package httpx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// NewServer tao http.Server voi cac timeout an toan.
//
// Co y KHONG dat WriteTimeout: mot so endpoint la SSE (phong cho), ket noi song
// hang chuc phut va WriteTimeout se cat ngang chung.
func NewServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

// Run lang nghe tren srv.Addr cho toi khi ctx bi huy, roi tat em trong grace.
func Run(ctx context.Context, srv *http.Server, grace time.Duration) error {
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("lang nghe %s: %w", srv.Addr, err)
	}
	return Serve(ctx, srv, ln, grace)
}

// Serve phuc vu tren ln. Khi ctx bi huy, ngung nhan ket noi moi va cho cac
// request dang chay xong toi da grace.
//
// Tra ve nil khi tat em thanh cong. Cat ngang giua mot transaction tao hold la
// cach nhanh nhat de tao ra ve bi khoa, nen grace phai du dai (20 giay).
func Serve(ctx context.Context, srv *http.Server, ln net.Listener, grace time.Duration) error {
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case err := <-errCh:
		// Server dung truoc khi ctx bi huy: day la loi that.
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	shCtx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()
	if err := srv.Shutdown(shCtx); err != nil {
		_ = srv.Close() // het gio cho: dong cuong buc de khong treo process
		return fmt.Errorf("tat em that bai: %w", err)
	}
	if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
