package httpx

import (
	"context"
	"net/http"
	"time"
)

// Check kiem tra mot phu thuoc; tra ve loi neu chua san sang.
type Check func(ctx context.Context) error

const readyTimeout = 2 * time.Second

// Health dang ky /healthz va /readyz len mux.
//
//   - /healthz (liveness): luon 200 khi process con chay. KHONG kiem phu thuoc:
//     mat Redis thi pod khong nen bi giet va khoi dong lai giua dot mo ban.
//   - /readyz (readiness): 200 khi moi check deu qua, 503 neu co check loi. Pod
//     mat phu thuoc thi bi rut khoi load balancer nhung van song.
func Health(mux *http.ServeMux, checks ...Check) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readyTimeout)
		defer cancel()
		for _, c := range checks {
			if err := c(ctx); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	})
}
