package http_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Prometheus scrape /metrics khong can dang nhap (chi mo trong mang noi bo; gateway khong dinh tuyen toi no).
func TestHTTP_Metrics_KhongCanDangNhap(t *testing.T) {
	router, _ := setupTestServer()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "go_goroutines") {
		t.Fatalf("GET /metrics = %d", rec.Code)
	}
}
