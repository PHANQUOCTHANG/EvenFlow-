package httpadapter_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eventflow/eventflow/libs/go/authx"
	httpadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/http"
	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

func setupCheckInMux(repo *fakeTicketRepo, signer port.QRSigner) *http.ServeMux {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	verifier := app.NewVerifyTicketUseCase(repo, signer, log)
	h := httpadapter.New(nil, log).
		WithTickets(app.NewIssueTicketsUseCase(repo, signer, log), verifier, repo).
		WithOrderOwner(allOwner{}).
		WithCheckIn(app.NewCheckInTicketUseCase(verifier, repo, log))
	mux := http.NewServeMux()
	h.Routes(mux, func(next http.Handler) http.Handler { return next })
	return mux
}

func issuedTicketQR(t *testing.T, repo *fakeTicketRepo, signer port.QRSigner) string {
	t.Helper()
	claims := authx.TicketClaims{TicketID: "tid-ci", OrderID: "oid-ci", EventID: "eid", JTI: "jti-ci"}
	qr, sig, err := signer.Sign(claims)
	if err != nil {
		t.Fatal(err)
	}
	repo.byJTI["jti-ci"] = domain.Ticket{ID: "tid-ci", OrderID: "oid-ci", JTI: "jti-ci", QRSig: sig, Status: domain.TicketStatusIssued}
	return qr
}

func postCheckIn(mux *http.ServeMux, role, qr string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"qr_payload": qr})
	r := httptest.NewRequest("POST", "/v1/tickets/check-in", bytes.NewReader(body))
	if role != "" {
		r.Header.Set("X-Identity-Id", "99999999-9999-4999-8999-999999999999")
		r.Header.Set("X-Identity-Role", role)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

// Quet lan 1: 200 va ve sang CHECKED_IN; quet lan 2: 409 ticket-already-used.
func TestCheckInHandler_MotLanDungMotLan(t *testing.T) {
	signer, _ := authx.NewQREngine([]byte("super-secret-test-key-32-bytes!"))
	repo := newFakeTicketRepo()
	mux := setupCheckInMux(repo, signer)
	qr := issuedTicketQR(t, repo, signer)

	w := postCheckIn(mux, "ops", qr)
	if w.Code != http.StatusOK {
		t.Fatalf("lan 1: %d %s", w.Code, w.Body)
	}
	var res struct {
		Valid  bool          `json:"valid"`
		Ticket domain.Ticket `json:"ticket"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil || !res.Valid ||
		res.Ticket.Status != domain.TicketStatusCheckedIn || res.Ticket.CheckedInAt == nil {
		t.Fatalf("body lan 1 = %s (%v)", w.Body, err)
	}

	w = postCheckIn(mux, "ops", qr)
	if w.Code != http.StatusConflict {
		t.Fatalf("lan 2: %d, muon 409; body %s", w.Code, w.Body)
	}
}

// Doi trang thai ve chi danh cho ops; goi thang service khong header danh tinh cung bi chan.
func TestCheckInHandler_ChiOps(t *testing.T) {
	signer, _ := authx.NewQREngine([]byte("super-secret-test-key-32-bytes!"))
	repo := newFakeTicketRepo()
	mux := setupCheckInMux(repo, signer)
	qr := issuedTicketQR(t, repo, signer)

	for _, role := range []string{"", "buyer", "organizer", "moderator"} {
		if w := postCheckIn(mux, role, qr); w.Code != http.StatusForbidden {
			t.Errorf("role %q: %d, muon 403", role, w.Code)
		}
	}
	if repo.byJTI["jti-ci"].Status != domain.TicketStatusIssued {
		t.Error("ve bi doi trang thai du request bi tu choi")
	}
}

func TestCheckInHandler_QRGiaVaBodyHong(t *testing.T) {
	signer, _ := authx.NewQREngine([]byte("super-secret-test-key-32-bytes!"))
	mux := setupCheckInMux(newFakeTicketRepo(), signer)
	if w := postCheckIn(mux, "ops", "fake.invalid"); w.Code != http.StatusBadRequest {
		t.Errorf("QR gia: %d, muon 400", w.Code)
	}
	r := httptest.NewRequest("POST", "/v1/tickets/check-in", bytes.NewReader([]byte("{")))
	r.Header.Set("X-Identity-Id", "99999999-9999-4999-8999-999999999999")
	r.Header.Set("X-Identity-Role", "ops")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("body hong: %d, muon 400", w.Code)
	}
}
