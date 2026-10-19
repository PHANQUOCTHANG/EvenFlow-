package httpadapter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
	httpadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/http"
	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

type fakeTicketRepo struct {
	orders  map[string]port.OrderIssuanceInfo
	tickets map[string][]domain.Ticket // by order_id
	byJTI   map[string]domain.Ticket
}

func newFakeTicketRepo() *fakeTicketRepo {
	return &fakeTicketRepo{
		orders:  make(map[string]port.OrderIssuanceInfo),
		tickets: make(map[string][]domain.Ticket),
		byJTI:   make(map[string]domain.Ticket),
	}
}

func (f *fakeTicketRepo) GetOrderForIssuance(_ context.Context, orderID string) (port.OrderIssuanceInfo, error) {
	o, ok := f.orders[orderID]
	if !ok {
		return port.OrderIssuanceInfo{}, domain.ErrOrderNotFound
	}
	return o, nil
}

func (f *fakeTicketRepo) IssueTicketsTx(_ context.Context, order port.OrderIssuanceInfo, tickets []domain.Ticket, _ string) (bool, error) {
	if o, ok := f.orders[order.OrderID]; ok {
		if o.Status == "ISSUED" && len(f.tickets[order.OrderID]) > 0 {
			return true, nil
		}
		if o.Status != "PAID" {
			return false, domain.ErrOrderNotPaid
		}
		o.Status = "ISSUED"
		f.orders[order.OrderID] = o
	}
	f.tickets[order.OrderID] = append(f.tickets[order.OrderID], tickets...)
	for _, t := range tickets {
		f.byJTI[t.JTI] = t
	}
	return false, nil
}

func (f *fakeTicketRepo) GetTicketsByOrderID(_ context.Context, orderID string) ([]domain.Ticket, error) {
	return f.tickets[orderID], nil
}

func (f *fakeTicketRepo) GetTicketByJTI(_ context.Context, jti string) (domain.Ticket, error) {
	t, ok := f.byJTI[jti]
	if !ok {
		return domain.Ticket{}, domain.ErrTicketNotFound
	}
	return t, nil
}

func (f *fakeTicketRepo) UpdateTicketStatus(_ context.Context, ticketID string, current, next domain.TicketStatus, checkedInAt *time.Time) error {
	for jti, t := range f.byJTI {
		if t.ID == ticketID {
			if t.Status != current {
				return domain.ErrTicketAlreadyUsed
			}
			t.Status = next
			t.CheckedInAt = checkedInAt
			f.byJTI[jti] = t
			return nil
		}
	}
	return domain.ErrTicketNotFound
}

func setupTicketMux(repo *fakeTicketRepo, signer port.QRSigner) *http.ServeMux {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	issuer := app.NewIssueTicketsUseCase(repo, signer, log)
	verifier := app.NewVerifyTicketUseCase(repo, signer, log)

	h := httpadapter.New(nil, log).WithTickets(issuer, verifier, repo)
	mux := http.NewServeMux()
	noopMiddleware := func(next http.Handler) http.Handler { return next }
	h.Routes(mux, noopMiddleware)
	return mux
}

func TestTicketHandler_IssueTickets_Success(t *testing.T) {
	signer, _ := authx.NewQREngine([]byte("super-secret-test-key-32-bytes!"))
	repo := newFakeTicketRepo()
	orderID := "22222222-2222-2222-2222-222222222222"
	repo.orders[orderID] = port.OrderIssuanceInfo{
		OrderID:    orderID,
		EventID:    "11111111-1111-1111-1111-111111111111",
		IdentityID: "33333333-3333-3333-3333-333333333333",
		Status:     "PAID",
		Items: []port.OrderIssuanceItem{
			{TicketTypeID: "44444444-4444-4444-4444-444444444444", Quantity: 2, UnitCents: 100000},
		},
	}

	mux := setupTicketMux(repo, signer)

	req := httptest.NewRequest("POST", "/v1/orders/"+orderID+"/issue-tickets", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("ky vong 200 OK, got: %d, body: %s", w.Code, w.Body.String())
	}

	var res port.IssueTicketsResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal response that bai: %v", err)
	}

	if len(res.Tickets) != 2 {
		t.Fatalf("ky vong 2 ve phat hanh, got: %d", len(res.Tickets))
	}
	if res.AlreadyIssued {
		t.Errorf("ky vong already_issued la false")
	}

	// Kiem tra endpoint GET /v1/orders/{orderID}/tickets
	getReq := httptest.NewRequest("GET", "/v1/orders/"+orderID+"/tickets", nil)
	getW := httptest.NewRecorder()
	mux.ServeHTTP(getW, getReq)

	if getW.Code != http.StatusOK {
		t.Fatalf("ky vong 200 OK cho GET tickets, got: %d", getW.Code)
	}

	// Kiem tra endpoint POST /v1/tickets/verify voi ve vua sinh
	verifyBody, _ := json.Marshal(map[string]string{
		"qr_payload": res.Tickets[0].QRPayload,
	})
	vReq := httptest.NewRequest("POST", "/v1/tickets/verify", bytes.NewReader(verifyBody))
	vW := httptest.NewRecorder()
	mux.ServeHTTP(vW, vReq)

	if vW.Code != http.StatusOK {
		t.Fatalf("ky vong 200 OK cho verify ve hop le, got: %d, body: %s", vW.Code, vW.Body.String())
	}
}

func TestTicketHandler_IssueTickets_OrderNotPaid_422(t *testing.T) {
	signer, _ := authx.NewQREngine([]byte("super-secret-test-key-32-bytes!"))
	repo := newFakeTicketRepo()
	orderID := "22222222-2222-2222-2222-222222222222"
	repo.orders[orderID] = port.OrderIssuanceInfo{
		OrderID: orderID,
		Status:  "HELD",
		Items:   []port.OrderIssuanceItem{{TicketTypeID: "t-1", Quantity: 1}},
	}

	mux := setupTicketMux(repo, signer)

	req := httptest.NewRequest("POST", "/v1/orders/"+orderID+"/issue-tickets", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("ky vong 422 Unprocessable Entity, got: %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error body that bai: %v", err)
	}
	if body["type"] != httpadapter.ProblemOrderNotPaid {
		t.Errorf("Problem.Type khong dung: got %v, want %s", body["type"], httpadapter.ProblemOrderNotPaid)
	}
}

func TestTicketHandler_VerifyTicket_Tampered_400(t *testing.T) {
	signer, _ := authx.NewQREngine([]byte("super-secret-test-key-32-bytes!"))
	repo := newFakeTicketRepo()
	mux := setupTicketMux(repo, signer)

	verifyBody, _ := json.Marshal(map[string]string{
		"qr_payload": "fake.invalid-signature",
	})
	vReq := httptest.NewRequest("POST", "/v1/tickets/verify", bytes.NewReader(verifyBody))
	vW := httptest.NewRecorder()
	mux.ServeHTTP(vW, vReq)

	if vW.Code != http.StatusBadRequest {
		t.Fatalf("ky vong 400 Bad Request cho QR tampered, got: %d", vW.Code)
	}
}

func TestTicketHandler_VerifyTicket_AlreadyUsed_409(t *testing.T) {
	signer, _ := authx.NewQREngine([]byte("super-secret-test-key-32-bytes!"))
	repo := newFakeTicketRepo()

	ticketID := "tid-1"
	jti := "jti-used-test"
	claims := authx.TicketClaims{
		TicketID: ticketID,
		OrderID:  "oid-1",
		EventID:  "eid-1",
		JTI:      jti,
	}
	qrPayload, sig, _ := signer.Sign(claims)

	checkedInAt := time.Now().Add(-10 * time.Minute)
	repo.byJTI[jti] = domain.Ticket{
		ID:          ticketID,
		OrderID:     "oid-1",
		JTI:         jti,
		QRSig:       sig,
		Status:      domain.TicketStatusCheckedIn,
		CheckedInAt: &checkedInAt,
	}

	mux := setupTicketMux(repo, signer)

	verifyBody, _ := json.Marshal(map[string]string{
		"qr_payload": qrPayload,
	})
	vReq := httptest.NewRequest("POST", "/v1/tickets/verify", bytes.NewReader(verifyBody))
	vW := httptest.NewRecorder()
	mux.ServeHTTP(vW, vReq)

	if vW.Code != http.StatusConflict {
		t.Fatalf("ky vong 409 Conflict cho ve da dung, got: %d", vW.Code)
	}
}
