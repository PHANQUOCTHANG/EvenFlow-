package app_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/payment/internal/adapter/gateway"
	"github.com/eventflow/eventflow/services/payment/internal/app"
	"github.com/eventflow/eventflow/services/payment/internal/domain"
)

// fakeClock cho phep tua thoi gian de test chinh xac BR-O2
type fakeClock struct {
	now time.Time
}

func (f *fakeClock) Now() time.Time {
	return f.now
}

type fakeIDGen struct {
	nextID string
}

func (f *fakeIDGen) NewID() string {
	if f.nextID != "" {
		return f.nextID
	}
	return "pay_test_uuid_123"
}

// memoryRepo gia lap luu tru in-memory
type memoryRepo struct {
	orders   map[string]domain.OrderView
	payments map[string]domain.Payment
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		orders:   make(map[string]domain.OrderView),
		payments: make(map[string]domain.Payment),
	}
}

func (m *memoryRepo) GetOrder(_ context.Context, orderID string) (domain.OrderView, error) {
	o, exists := m.orders[orderID]
	if !exists {
		return domain.OrderView{}, domain.ErrOrderNotFound
	}
	return o, nil
}

func (m *memoryRepo) CreatePaymentAndMarkPending(_ context.Context, p domain.Payment) error {
	o, exists := m.orders[p.OrderID]
	if !exists {
		return domain.ErrOrderNotFound
	}
	// Chuyen trang thai sang PAYMENT_PENDING trong transaction
	o.Status = "PAYMENT_PENDING"
	m.orders[p.OrderID] = o
	m.payments[p.ID] = p
	return nil
}

func (m *memoryRepo) GetPayment(_ context.Context, paymentID string) (domain.Payment, error) {
	p, exists := m.payments[paymentID]
	if !exists {
		return domain.Payment{}, errors.New("payment not found")
	}
	return p, nil
}

func (m *memoryRepo) GetPaymentByOrder(_ context.Context, orderID string) (domain.Payment, error) {
	for _, p := range m.payments {
		if p.OrderID == orderID {
			return p, nil
		}
	}
	return domain.Payment{}, errors.New("payment not found")
}

func (m *memoryRepo) ProcessPaymentSuccessTx(_ context.Context, params domain.ProcessWebhookParams) (domain.WebhookProcessResult, error) {
	o, exists := m.orders[params.OrderID]
	if !exists {
		return domain.WebhookProcessResult{}, domain.ErrOrderNotFound
	}
	o.Status = "PAID"
	m.orders[o.ID] = o
	return domain.WebhookProcessResult{
		Success:       true,
		OrderID:       o.ID,
		ProviderTxnID: params.ProviderTxnID,
		Status:        domain.StatusSucceeded,
	}, nil
}

func (m *memoryRepo) ProcessPaymentFailedTx(_ context.Context, params domain.ProcessWebhookParams) (domain.WebhookProcessResult, error) {
	return domain.WebhookProcessResult{
		Success:       false,
		OrderID:       params.OrderID,
		ProviderTxnID: params.ProviderTxnID,
		Status:        domain.StatusFailed,
	}, nil
}

func (m *memoryRepo) SavePaymentInbox(_ context.Context, item domain.PaymentInboxItem) (int64, error) {
	return 1, nil
}

func (m *memoryRepo) GetPendingPaymentInbox(_ context.Context, limit int, maxAttempts int) ([]domain.PaymentInboxItem, error) {
	return nil, nil
}

func (m *memoryRepo) MarkPaymentInboxProcessed(_ context.Context, id int64, processedAt time.Time) error {
	return nil
}

func (m *memoryRepo) IncrementPaymentInboxAttempts(_ context.Context, id int64) error {
	return nil
}

func (m *memoryRepo) GetStalePaymentInbox(_ context.Context, olderThan time.Duration, limit int) ([]domain.StaleInboxRecord, error) {
	return nil, nil
}

func (m *memoryRepo) GetStalePendingOrders(_ context.Context, olderThan time.Duration, limit int) ([]domain.StaleOrderInfo, error) {
	return nil, nil
}

func (m *memoryRepo) ProcessOrderCancellationTx(_ context.Context, params domain.CancelOrderParams) (domain.CancelOrderResult, error) {
	return domain.CancelOrderResult{OrderID: params.OrderID, Status: "CANCELLED"}, nil
}

func setupTestEnvironment() (*memoryRepo, *gateway.Registry, *fakeClock, *fakeIDGen, *slog.Logger) {
	repo := newMemoryRepo()
	reg := gateway.NewRegistry(domain.ProviderSandbox, []string{"sandbox", "vnpay"})
	reg.Register(gateway.NewSandboxGateway(gateway.SandboxModeSuccess))
	reg.Register(gateway.NewVNPayGateway(gateway.VNPayConfig{
		TmnCode:    "TMN",
		HashSecret: "SECRET",
	}))

	clock := &fakeClock{now: time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)}
	idGen := &fakeIDGen{nextID: "pay_test_001"}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	return repo, reg, clock, idGen, logger
}

func TestInitiatePayment_Success(t *testing.T) {
	repo, reg, clock, idGen, logger := setupTestEnvironment()

	// Tao order HELD con han 10 phut
	repo.orders["ord-100"] = domain.OrderView{
		ID:         "ord-100",
		EventID:    "ev-rock",
		IdentityID: "usr-1",
		Status:     "HELD",
		TotalCents: 250000,
		Currency:   "VND",
		ExpiresAt:  clock.Now().Add(10 * time.Minute),
	}

	uc := app.NewInitiatePaymentUseCase(repo, reg, clock, idGen, logger)

	in := app.InitiatePaymentInput{
		EventID:     "ev-rock",
		OrderID:     "ord-100",
		HoldID:      "hld-1",
		AmountCents: 250000,
		Currency:    "VND",
		Method:      domain.ProviderSandbox,
		BuyerName:   "Nguyen Van A",
		BuyerEmail:  "buyer@example.com",
		ReturnURL:   "/checkout/result",
	}

	out, err := uc.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.PaymentID != "pay_test_001" {
		t.Errorf("expected paymentID pay_test_001, got %s", out.PaymentID)
	}
	if out.OrderID != "ord-100" {
		t.Errorf("expected orderID ord-100, got %s", out.OrderID)
	}
	if out.RedirectURL == "" {
		t.Errorf("expected non-empty redirectURL")
	}

	// Kiem tra trang thai don hang da duoc chuyen sang PAYMENT_PENDING
	updatedOrder, _ := repo.GetOrder(context.Background(), "ord-100")
	if updatedOrder.Status != "PAYMENT_PENDING" {
		t.Errorf("expected order status PAYMENT_PENDING, got %s", updatedOrder.Status)
	}

	// Kiem tra ban ghi payment da duoc tao
	createdPayment, pErr := repo.GetPayment(context.Background(), "pay_test_001")
	if pErr != nil {
		t.Fatalf("payment record was not saved: %v", pErr)
	}
	if createdPayment.Status != domain.StatusPending {
		t.Errorf("expected payment status PENDING, got %s", createdPayment.Status)
	}
	if createdPayment.AmountCents != 250000 {
		t.Errorf("expected payment amount 250000, got %d", createdPayment.AmountCents)
	}
}

func TestInitiatePayment_HoldExpired_Returns409(t *testing.T) {
	repo, reg, clock, idGen, logger := setupTestEnvironment()

	// Tao order HELD da het han cach day 1 giay (BR-O2)
	repo.orders["ord-expired"] = domain.OrderView{
		ID:         "ord-expired",
		EventID:    "ev-rock",
		Status:     "HELD",
		TotalCents: 200000,
		Currency:   "VND",
		ExpiresAt:  clock.Now().Add(-1 * time.Second),
	}

	uc := app.NewInitiatePaymentUseCase(repo, reg, clock, idGen, logger)

	in := app.InitiatePaymentInput{
		EventID:     "ev-rock",
		OrderID:     "ord-expired",
		AmountCents: 200000,
		Method:      domain.ProviderSandbox,
	}

	_, err := uc.Execute(context.Background(), in)
	if !errors.Is(err, domain.ErrHoldExpired) {
		t.Fatalf("expected ErrHoldExpired (BR-O2), got: %v", err)
	}

	// Don hang van phai giu nguyen, khong bi doi sang PAYMENT_PENDING
	order, _ := repo.GetOrder(context.Background(), "ord-expired")
	if order.Status != "HELD" {
		t.Errorf("order status should still be HELD, got %s", order.Status)
	}
}

func TestInitiatePayment_OrderNotHeld(t *testing.T) {
	repo, reg, clock, idGen, logger := setupTestEnvironment()

	repo.orders["ord-cancelled"] = domain.OrderView{
		ID:         "ord-cancelled",
		EventID:    "ev-rock",
		Status:     "CANCELLED",
		TotalCents: 200000,
		ExpiresAt:  clock.Now().Add(10 * time.Minute),
	}

	uc := app.NewInitiatePaymentUseCase(repo, reg, clock, idGen, logger)

	in := app.InitiatePaymentInput{
		EventID:     "ev-rock",
		OrderID:     "ord-cancelled",
		AmountCents: 200000,
		Method:      domain.ProviderSandbox,
	}

	_, err := uc.Execute(context.Background(), in)
	if !errors.Is(err, domain.ErrOrderNotHeld) {
		t.Fatalf("expected ErrOrderNotHeld, got: %v", err)
	}
}

func TestInitiatePayment_AmountMismatch(t *testing.T) {
	repo, reg, clock, idGen, logger := setupTestEnvironment()

	repo.orders["ord-price"] = domain.OrderView{
		ID:         "ord-price",
		EventID:    "ev-rock",
		Status:     "HELD",
		TotalCents: 500000,
		Currency:   "VND",
		ExpiresAt:  clock.Now().Add(10 * time.Minute),
	}

	uc := app.NewInitiatePaymentUseCase(repo, reg, clock, idGen, logger)

	in := app.InitiatePaymentInput{
		EventID:     "ev-rock",
		OrderID:     "ord-price",
		AmountCents: 300000, // sai lech so tien
		Method:      domain.ProviderSandbox,
	}

	_, err := uc.Execute(context.Background(), in)
	if !errors.Is(err, domain.ErrAmountMismatch) {
		t.Fatalf("expected ErrAmountMismatch, got: %v", err)
	}
}

func TestInitiatePayment_UnsupportedProvider(t *testing.T) {
	repo, reg, clock, idGen, logger := setupTestEnvironment()

	repo.orders["ord-prov"] = domain.OrderView{
		ID:         "ord-prov",
		EventID:    "ev-rock",
		Status:     "HELD",
		TotalCents: 100000,
		ExpiresAt:  clock.Now().Add(10 * time.Minute),
	}

	uc := app.NewInitiatePaymentUseCase(repo, reg, clock, idGen, logger)

	in := app.InitiatePaymentInput{
		EventID:     "ev-rock",
		OrderID:     "ord-prov",
		AmountCents: 100000,
		Method:      domain.Provider("zalopay"),
	}

	_, err := uc.Execute(context.Background(), in)
	if !errors.Is(err, domain.ErrUnsupportedProvider) {
		t.Fatalf("expected ErrUnsupportedProvider, got: %v", err)
	}
}

func TestInitiatePayment_InputValidation(t *testing.T) {
	repo, reg, clock, idGen, logger := setupTestEnvironment()
	uc := app.NewInitiatePaymentUseCase(repo, reg, clock, idGen, logger)

	// Missing order_id
	_, err := uc.Execute(context.Background(), app.InitiatePaymentInput{
		EventID:     "ev-1",
		AmountCents: 100,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for missing order_id, got %v", err)
	}

	// Missing event_id
	_, err = uc.Execute(context.Background(), app.InitiatePaymentInput{
		OrderID:     "ord-1",
		AmountCents: 100,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for missing event_id, got %v", err)
	}

	// Amount <= 0
	_, err = uc.Execute(context.Background(), app.InitiatePaymentInput{
		EventID:     "ev-1",
		OrderID:     "ord-1",
		AmountCents: 0,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for amount <= 0, got %v", err)
	}

	// Event mismatch
	repo.orders["ord-mismatch"] = domain.OrderView{
		ID:         "ord-mismatch",
		EventID:    "ev-different",
		Status:     "HELD",
		TotalCents: 100,
		ExpiresAt:  clock.Now().Add(5 * time.Minute),
	}
	_, err = uc.Execute(context.Background(), app.InitiatePaymentInput{
		EventID:     "ev-requested",
		OrderID:     "ord-mismatch",
		AmountCents: 100,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for event mismatch, got %v", err)
	}
}

func TestInitiatePayment_ReentrantPaymentPending(t *testing.T) {
	repo, reg, clock, idGen, logger := setupTestEnvironment()

	// Order already in PAYMENT_PENDING with existing payment
	repo.orders["ord-reentrant"] = domain.OrderView{
		ID:         "ord-reentrant",
		EventID:    "ev-rock",
		Status:     "PAYMENT_PENDING",
		TotalCents: 200000,
		Currency:   "VND",
		ExpiresAt:  clock.Now().Add(5 * time.Minute),
	}
	repo.payments["pay-existing"] = domain.Payment{
		ID:            "pay-existing",
		OrderID:       "ord-reentrant",
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: "sbx_txn_existing",
		AmountCents:   200000,
		Currency:      "VND",
		Status:        domain.StatusPending,
	}

	uc := app.NewInitiatePaymentUseCase(repo, reg, clock, idGen, logger)
	out, err := uc.Execute(context.Background(), app.InitiatePaymentInput{
		EventID:     "ev-rock",
		OrderID:     "ord-reentrant",
		AmountCents: 200000,
		Currency:    "VND",
		Method:      domain.ProviderSandbox,
	})
	if err != nil {
		t.Fatalf("unexpected error on reentrant payment: %v", err)
	}
	if out.PaymentID != "pay-existing" {
		t.Errorf("expected existing payment ID pay-existing, got %s", out.PaymentID)
	}
}

func TestGetPaymentUseCase(t *testing.T) {
	repo, _, _, _, _ := setupTestEnvironment()
	repo.payments["pay-123"] = domain.Payment{
		ID:          "pay-123",
		OrderID:     "ord-1",
		Provider:    domain.ProviderSandbox,
		AmountCents: 100000,
		Status:      domain.StatusPending,
	}

	uc := app.NewGetPaymentUseCase(repo)

	// Valid ID
	p, err := uc.Execute(context.Background(), "pay-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ID != "pay-123" {
		t.Errorf("expected pay-123, got %s", p.ID)
	}

	// Missing ID
	_, err = uc.Execute(context.Background(), "")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty ID, got %v", err)
	}
}

func TestListMethodsUseCase(t *testing.T) {
	_, reg, _, _, _ := setupTestEnvironment()
	uc := app.NewListMethodsUseCase(reg)

	methods := uc.Execute(context.Background())
	if len(methods) == 0 {
		t.Errorf("expected non-empty payment methods")
	}
}

// TestAC_ChayLocalBangMock_Va_DoiCongChiCanCauHinh kiem tra truc tiep 2 tieu chi nghiem thu (AC) trong Jira:
// 1. Chay duoc local hoan toan bang mock, khong can cong that
// 2. Doi cong chi can cau hinh
func TestAC_ChayLocalBangMock_Va_DoiCongChiCanCauHinh(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)}
	idGen := &fakeIDGen{nextID: "pay_ac_test"}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	// =========================================================================
	// AC-1: Chay duoc local hoan toan bang mock, khong can cong that
	// =========================================================================
	repoMock := newMemoryRepo()
	repoMock.orders["ord-mock"] = domain.OrderView{
		ID:         "ord-mock",
		EventID:    "ev-test",
		Status:     "HELD",
		TotalCents: 150000,
		Currency:   "VND",
		ExpiresAt:  clock.Now().Add(10 * time.Minute),
	}

	regMock := gateway.NewRegistry(domain.ProviderSandbox, []string{"sandbox"})
	sbxMock := gateway.NewSandboxGateway(gateway.SandboxModeSuccess)
	regMock.Register(sbxMock)

	ucMock := app.NewInitiatePaymentUseCase(repoMock, regMock, clock, idGen, logger)

	// Khoi tao thanh toan local ma khong can bat ky cong that nao
	outMock, err := ucMock.Execute(context.Background(), app.InitiatePaymentInput{
		EventID:     "ev-test",
		OrderID:     "ord-mock",
		AmountCents: 150000,
		Method:      domain.ProviderSandbox,
	})
	if err != nil {
		t.Fatalf("[AC-1 FAIL] Chay local bang mock that bai: %v", err)
	}
	if outMock.Status != "PENDING" {
		t.Errorf("[AC-1 FAIL] Trang thai mock phai la PENDING, nhan: %s", outMock.Status)
	}
	if outMock.RedirectURL == "" || !strings.Contains(outMock.RedirectURL, "status=SUCCESS") {
		t.Errorf("[AC-1 FAIL] Redirect URL cua sandbox mock khong hop le: %s", outMock.RedirectURL)
	}
	// Kiem tra callback verification cua sandbox mock cung chay hoan toan local 0 network
	cbResult, err := sbxMock.VerifyCallback(context.Background(), map[string]string{
		"provider_txn_id": outMock.ProviderTxnID,
		"status":          "SUCCESS",
	})
	if err != nil || !cbResult.Valid || cbResult.Status != domain.StatusSucceeded {
		t.Errorf("[AC-1 FAIL] Callback verify sandbox mock that bai: valid=%v, err=%v", cbResult.Valid, err)
	}

	// =========================================================================
	// AC-2: Doi cong chi can cau hinh (Config-driven gateway selection)
	// =========================================================================
	// Gia lap he thong thay doi bien moi truong:
	// PAYMENT_DEFAULT_PROVIDER=vnpay
	// PAYMENT_ENABLED_PROVIDERS=sandbox,vnpay,momo
	regConfig1 := gateway.NewRegistry(domain.ProviderVNPay, []string{"sandbox", "vnpay", "momo"})
	regConfig1.Register(gateway.NewSandboxGateway(gateway.SandboxModeSuccess))
	regConfig1.Register(gateway.NewVNPayGateway(gateway.VNPayConfig{
		TmnCode:    "TMN_VNPAY",
		HashSecret: "SECRET_KEY",
		BaseURL:    "https://sandbox.vnpayment.vn/vpcpay.html",
	}))
	regConfig1.Register(gateway.NewMoMoGateway(gateway.MoMoConfig{
		PartnerCode: "MOMO_PARTNER",
		AccessKey:   "ACCESS_KEY",
		SecretKey:   "SECRET_KEY",
	}))

	repoConfig := newMemoryRepo()
	repoConfig.orders["ord-config-1"] = domain.OrderView{
		ID:         "ord-config-1",
		EventID:    "ev-test",
		Status:     "HELD",
		TotalCents: 200000,
		Currency:   "VND",
		ExpiresAt:  clock.Now().Add(10 * time.Minute),
	}

	ucConfig1 := app.NewInitiatePaymentUseCase(repoConfig, regConfig1, clock, idGen, logger)

	// Truong hop 1: Client khong can truyen method (de trong "") -> he thong tu dong chon VNPay do cau hinh default
	outVNPay, err := ucConfig1.Execute(context.Background(), app.InitiatePaymentInput{
		EventID:     "ev-test",
		OrderID:     "ord-config-1",
		AmountCents: 200000,
		Method:      "", // De trong -> he thong chon theo cau hinh defaultProvider
	})
	if err != nil {
		t.Fatalf("[AC-2 FAIL] Khong the doi cong mac dinh qua cau hinh sang VNPay: %v", err)
	}
	if !strings.Contains(outVNPay.RedirectURL, "vnp_TmnCode=TMN_VNPAY") {
		t.Errorf("[AC-2 FAIL] Redirect URL phai sang VNPay theo cau hinh mac dinh, nhan: %s", outVNPay.RedirectURL)
	}

	// Truong hop 2: Doi cau hinh PAYMENT_DEFAULT_PROVIDER sang MoMo (khong sua code)
	regConfig2 := gateway.NewRegistry(domain.ProviderMoMo, []string{"sandbox", "vnpay", "momo"})
	regConfig2.Register(gateway.NewSandboxGateway(gateway.SandboxModeSuccess))
	regConfig2.Register(gateway.NewVNPayGateway(gateway.VNPayConfig{TmnCode: "TMN_VNPAY", HashSecret: "SECRET_KEY"}))
	regConfig2.Register(gateway.NewMoMoGateway(gateway.MoMoConfig{PartnerCode: "MOMO_PARTNER", AccessKey: "ACCESS_KEY", SecretKey: "SECRET_KEY"}))

	repoConfig.orders["ord-config-2"] = domain.OrderView{
		ID:         "ord-config-2",
		EventID:    "ev-test",
		Status:     "HELD",
		TotalCents: 300000,
		Currency:   "VND",
		ExpiresAt:  clock.Now().Add(10 * time.Minute),
	}

	ucConfig2 := app.NewInitiatePaymentUseCase(repoConfig, regConfig2, clock, idGen, logger)
	outMoMo, err := ucConfig2.Execute(context.Background(), app.InitiatePaymentInput{
		EventID:     "ev-test",
		OrderID:     "ord-config-2",
		AmountCents: 300000,
		Method:      "", // De trong -> tu dong sang MoMo
	})
	if err != nil {
		t.Fatalf("[AC-2 FAIL] Khong the doi cong mac dinh sang MoMo qua cau hinh: %v", err)
	}
	if !strings.Contains(outMoMo.RedirectURL, "partnerCode=MOMO_PARTNER") {
		t.Errorf("[AC-2 FAIL] Redirect URL phai sang MoMo theo cau hinh, nhan: %s", outMoMo.RedirectURL)
	}

	// Truong hop 3: Vo hieu hoa cong qua PAYMENT_ENABLED_PROVIDERS (chi bat sandbox)
	regConfigDisabled := gateway.NewRegistry(domain.ProviderSandbox, []string{"sandbox"})
	regConfigDisabled.Register(gateway.NewSandboxGateway(gateway.SandboxModeSuccess))
	regConfigDisabled.Register(gateway.NewVNPayGateway(gateway.VNPayConfig{TmnCode: "TMN_VNPAY", HashSecret: "SECRET_KEY"}))

	repoConfig.orders["ord-config-3"] = domain.OrderView{
		ID:         "ord-config-3",
		EventID:    "ev-test",
		Status:     "HELD",
		TotalCents: 100000,
		Currency:   "VND",
		ExpiresAt:  clock.Now().Add(10 * time.Minute),
	}

	ucDisabled := app.NewInitiatePaymentUseCase(repoConfig, regConfigDisabled, clock, idGen, logger)
	_, err = ucDisabled.Execute(context.Background(), app.InitiatePaymentInput{
		EventID:     "ev-test",
		OrderID:     "ord-config-3",
		AmountCents: 100000,
		Method:      domain.ProviderVNPay, // Yeu cau vnpay nhung cau hinh chi bat sandbox
	})
	if !errors.Is(err, domain.ErrProviderDisabled) {
		t.Errorf("[AC-2 FAIL] Yeu cau cong bi disable phai tra ve ErrProviderDisabled, nhan: %v", err)
	}
}
