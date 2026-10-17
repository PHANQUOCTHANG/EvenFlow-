# EV-151 — Bằng chứng thực thi (Evidence)

Ngày chạy: 2026-10-17.
Môi trường: Docker Engine 28.4.0 / Linux container Go 1.26 (`golang:1.26-alpine`).
Nhánh: `feature/EV-151-webhook-exactly-once`, base `develop` @ `c241ea152fc1a728883f1155b9c0f0afc837a1a6`.

---

## 1. Static Analysis & Lint

### A. Format check (`gofmt`)
```bash
docker run --rm -v "${PWD}:/app" -w /app golang:1.26-alpine gofmt -l ./services/payment
```
**Output:**
```
(empty - 100% formatted clean)
```

### B. Static analysis (`go vet`)
```bash
docker run --rm -v evenflow-pkg-mod:/go/pkg/mod -v evenflow-go-build:/root/.cache/go-build -v "${PWD}:/app" -w /app golang:1.26-alpine go vet ./services/payment/...
```
**Output:**
```
(exit code 0 - 0 issues found)
```

---

## 2. Test Suites & Coverage

```bash
docker run --rm -v evenflow-pkg-mod:/go/pkg/mod -v evenflow-go-build:/root/.cache/go-build -v "${PWD}:/app" -w /app golang:1.26-alpine go test -v -cover ./services/payment/...
```

**Raw Output:**
```
	github.com/eventflow/eventflow/services/payment/cmd/server		coverage: 0.0% of statements
=== RUN   TestGatewayRegistry_ResolveAndDefault
--- PASS: TestGatewayRegistry_ResolveAndDefault (0.00s)
=== RUN   TestGatewayRegistry_ListEnabled
--- PASS: TestGatewayRegistry_ListEnabled (0.00s)
=== RUN   TestMoMoGateway_InitiatePayment
--- PASS: TestMoMoGateway_InitiatePayment (0.00s)
=== RUN   TestMoMoGateway_VerifyCallback
--- PASS: TestMoMoGateway_VerifyCallback (0.00s)
=== RUN   TestSandboxGateway_AutoSuccess
--- PASS: TestSandboxGateway_AutoSuccess (0.00s)
=== RUN   TestSandboxGateway_AutoFail
--- PASS: TestSandboxGateway_AutoFail (0.00s)
=== RUN   TestSandboxGateway_SimulateDelay
--- PASS: TestSandboxGateway_SimulateDelay (0.00s)
=== RUN   TestSandboxGateway_VerifyCallback_SignatureAndTimestamp
--- PASS: TestSandboxGateway_VerifyCallback_SignatureAndTimestamp (0.00s)
=== RUN   TestStripeGateway_InitiatePayment
--- PASS: TestStripeGateway_InitiatePayment (0.00s)
=== RUN   TestStripeGateway_VerifyCallback
--- PASS: TestStripeGateway_VerifyCallback (0.00s)
=== RUN   TestVNPayGateway_InitiatePayment
--- PASS: TestVNPayGateway_InitiatePayment (0.00s)
=== RUN   TestVNPayGateway_VerifyCallback
--- PASS: TestVNPayGateway_VerifyCallback (0.00s)
PASS
coverage: 82.6% of statements
ok  	github.com/eventflow/eventflow/services/payment/internal/adapter/gateway	0.010s	coverage: 82.6% of statements
=== RUN   TestHandler_InitiatePayment_Success
--- PASS: TestHandler_InitiatePayment_Success (0.00s)
=== RUN   TestHandler_InitiatePayment_HoldExpired_409
--- PASS: TestHandler_InitiatePayment_HoldExpired_409 (0.00s)
=== RUN   TestHandler_InitiatePayment_AmountMismatch_422
--- PASS: TestHandler_InitiatePayment_AmountMismatch_422 (0.00s)
=== RUN   TestHandler_InitiatePayment_Validation_400
--- PASS: TestHandler_InitiatePayment_Validation_400 (0.00s)
=== RUN   TestHandler_GetPayment
--- PASS: TestHandler_GetPayment (0.00s)
=== RUN   TestHandler_ListMethods
--- PASS: TestHandler_ListMethods (0.00s)
=== RUN   TestHandler_IdempotencyKey_Integration
--- PASS: TestHandler_IdempotencyKey_Integration (0.00s)
=== RUN   TestHandler_Webhook_Sandbox_Success
--- PASS: TestHandler_Webhook_Sandbox_Success (0.00s)
=== RUN   TestHandler_Webhook_VNPay_GET_Success
--- PASS: TestHandler_Webhook_VNPay_GET_Success (0.00s)
=== RUN   TestHandler_Webhook_Duplicate_200OK
--- PASS: TestHandler_Webhook_Duplicate_200OK (0.00s)
=== RUN   TestHandler_Webhook_InvalidSignature_400
--- PASS: TestHandler_Webhook_InvalidSignature_400 (0.00s)
=== RUN   TestHandler_Webhook_ReplayExpired_400
--- PASS: TestHandler_Webhook_ReplayExpired_400 (0.00s)
=== RUN   TestHandler_Webhook_OrderNotFound_404
--- PASS: TestHandler_Webhook_OrderNotFound_404 (0.00s)
=== RUN   TestHandler_Webhook_ExactlyOnce_HTTP100Concurrent
--- PASS: TestHandler_Webhook_ExactlyOnce_HTTP100Concurrent (0.00s)
PASS
coverage: 71.9% of statements
ok  	github.com/eventflow/eventflow/services/payment/internal/adapter/http	0.036s	coverage: 71.9% of statements
=== RUN   TestMemoryRepository
--- PASS: TestMemoryRepository (0.00s)
=== RUN   TestProcessPaymentSuccessTx_Atomic
--- PASS: TestProcessPaymentSuccessTx_Atomic (0.00s)
=== RUN   TestProcessPaymentFailedTx
--- PASS: TestProcessPaymentFailedTx (0.00s)
PASS
coverage: 31.6% of statements
ok  	github.com/eventflow/eventflow/services/payment/internal/adapter/postgres	0.036s	coverage: 31.6% of statements
=== RUN   TestInitiatePayment_Success
--- PASS: TestInitiatePayment_Success (0.00s)
=== RUN   TestInitiatePayment_HoldExpired_Returns409
--- PASS: TestInitiatePayment_HoldExpired_Returns409 (0.00s)
=== RUN   TestInitiatePayment_OrderNotHeld
--- PASS: TestInitiatePayment_OrderNotHeld (0.00s)
=== RUN   TestInitiatePayment_AmountMismatch
--- PASS: TestInitiatePayment_AmountMismatch (0.00s)
=== RUN   TestInitiatePayment_UnsupportedProvider
--- PASS: TestInitiatePayment_UnsupportedProvider (0.00s)
=== RUN   TestInitiatePayment_InputValidation
--- PASS: TestInitiatePayment_InputValidation (0.00s)
=== RUN   TestInitiatePayment_ReentrantPaymentPending
--- PASS: TestInitiatePayment_ReentrantPaymentPending (0.00s)
=== RUN   TestGetPaymentUseCase
--- PASS: TestGetPaymentUseCase (0.00s)
=== RUN   TestListMethodsUseCase
--- PASS: TestListMethodsUseCase (0.00s)
=== RUN   TestAC_ChayLocalBangMock_Va_DoiCongChiCanCauHinh
--- PASS: TestAC_ChayLocalBangMock_Va_DoiCongChiCanCauHinh (0.00s)
=== RUN   TestProcessWebhook_ExactlyOnce_Concurrent100
--- PASS: TestProcessWebhook_ExactlyOnce_Concurrent100 (0.00s)
=== RUN   TestProcessWebhook_InvalidSignature
--- PASS: TestProcessWebhook_InvalidSignature (0.00s)
=== RUN   TestProcessWebhook_ReplayExpired
--- PASS: TestProcessWebhook_ReplayExpired (0.00s)
=== RUN   TestProcessWebhook_ReplayFutureClockSkew
--- PASS: TestProcessWebhook_ReplayFutureClockSkew (0.00s)
=== RUN   TestProcessWebhook_PaymentFailed
--- PASS: TestProcessWebhook_PaymentFailed (0.00s)
=== RUN   TestProcessWebhook_InputValidation
--- PASS: TestProcessWebhook_InputValidation (0.00s)
PASS
coverage: 81.1% of statements
ok  	github.com/eventflow/eventflow/services/payment/internal/app	0.020s	coverage: 81.1% of statements
	github.com/eventflow/eventflow/services/payment/internal/config		coverage: 0.0% of statements
=== RUN   TestProvider_NormalizeAndValidate
--- PASS: TestProvider_NormalizeAndValidate (0.00s)
=== RUN   TestPaymentStatus_IsTerminal
--- PASS: TestPaymentStatus_IsTerminal (0.00s)
=== RUN   TestOrderView_HoldAndExpiry
--- PASS: TestOrderView_HoldAndExpiry (0.00s)
=== RUN   TestMaskPII
--- PASS: TestMaskPII (0.00s)
=== RUN   TestValidationError
--- PASS: TestValidationError (0.00s)
=== RUN   TestWebhookDomainErrorsAndStructs
--- PASS: TestWebhookDomainErrorsAndStructs (0.00s)
PASS
coverage: 95.7% of statements
ok  	github.com/eventflow/eventflow/services/payment/internal/domain	0.034s	coverage: 95.7% of statements
	github.com/eventflow/eventflow/services/payment/internal/port		coverage: 0.0% of statements
```

### Bảng tổng hợp Coverage
| Module / Package | Coverage đạt được | Ngưỡng yêu cầu | Đánh giá |
|---|---|---|---|
| `internal/domain` | **95.7%** | >= 80% | **VƯỢT NGƯỠNG** |
| `internal/app` | **81.1%** | >= 80% | **VƯỢT NGƯỠNG** |
| `internal/adapter/gateway` | **82.6%** | >= 70% | **VƯỢT NGƯỠNG** |
| `internal/adapter/http` | **71.9%** | >= 70% | **VƯỢT NGƯỠNG** |

---

## 3. Server Build Verification

```bash
docker run --rm -v evenflow-pkg-mod:/go/pkg/mod -v evenflow-go-build:/root/.cache/go-build -v "${PWD}:/app" -w /app/services/payment golang:1.26-alpine go build -v ./cmd/server
```

**Output:**
```
github.com/eventflow/eventflow/services/payment/cmd/server
(exit code 0 - Build thành công 100%)
```
