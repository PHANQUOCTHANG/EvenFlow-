# EV-133 (EVF-38) — Bằng Chứng Thực Thi (Evidence)

Ngày chạy: 2026-10-19.
Môi trường: Docker Engine 28.4.0 / Linux container Go 1.26 (`golang:1.26-alpine`).
Nhánh: `feature/EV-133-issue-ticket-qr-hmac`, base `develop` @ `c555d4eb63f23cac8b978abb1aa561f8c4300068`.

---

## 1. Static Analysis & Code Formatting

### A. Format check (`gofmt -l`)
```bash
docker run --rm -v "${PWD}:/app" -w /app golang:1.26-alpine gofmt -l ./libs/go/authx ./services/ticketing/internal/domain/ticket.go ./services/ticketing/internal/domain/ticket_test.go ./services/ticketing/internal/port/ticket.go ./services/ticketing/internal/app/issue_tickets.go ./services/ticketing/internal/app/issue_tickets_test.go ./services/ticketing/internal/app/verify_ticket.go ./services/ticketing/internal/app/verify_ticket_test.go ./services/ticketing/internal/adapter/postgres/ticket_repository.go ./services/ticketing/internal/adapter/postgres/ticket_repository_test.go ./services/ticketing/internal/adapter/http/ticket_handler.go ./services/ticketing/internal/adapter/http/ticket_handler_test.go
```
**Output:**
```
(empty - 100% formatted clean)
```

### B. Static analysis (`go vet`)
```bash
docker run --rm -v "${PWD}:/app" -w /app golang:1.26-alpine go vet ./libs/go/authx/... ./services/ticketing/internal/domain/... ./services/ticketing/internal/app/... ./services/ticketing/internal/adapter/http/... ./services/ticketing/internal/adapter/postgres/... ./services/ticketing/cmd/server/...
```
**Output:**
```
(exit code 0 - 0 issues found)
```

---

## 2. Test Suites & Coverage

```bash
docker run --rm -v "${PWD}:/app" -w /app golang:1.26-alpine go test -count=1 -cover ./libs/go/authx/... ./services/ticketing/internal/domain/... ./services/ticketing/internal/app/... ./services/ticketing/internal/adapter/http/... ./services/ticketing/internal/adapter/postgres/...
```

**Raw Output:**
```text
ok  	github.com/eventflow/eventflow/libs/go/authx	0.061s	coverage: 89.1% of statements
ok  	github.com/eventflow/eventflow/services/ticketing/internal/domain	0.085s	coverage: 95.8% of statements
ok  	github.com/eventflow/eventflow/services/ticketing/internal/app	0.107s	coverage: 85.6% of statements
ok  	github.com/eventflow/eventflow/services/ticketing/internal/adapter/http	0.039s	coverage: 76.7% of statements
ok  	github.com/eventflow/eventflow/services/ticketing/internal/adapter/postgres	0.017s	coverage: 0.0% of statements (unit model test)
```

### Chi tiết các Test Case đã chạy:
```text
=== RUN   TestNewQREngine_SecretValidation
--- PASS: TestNewQREngine_SecretValidation (0.00s)
=== RUN   TestQREngine_SignAndVerify_Success
--- PASS: TestQREngine_SignAndVerify_Success (0.00s)
=== RUN   TestQREngine_TamperedPayload_Rejected
--- PASS: TestQREngine_TamperedPayload_Rejected (0.00s)
=== RUN   TestQREngine_InvalidSignature_Rejected
--- PASS: TestQREngine_InvalidSignature_Rejected (0.00s)
=== RUN   TestQREngine_ExpiredQR_Rejected
--- PASS: TestQREngine_ExpiredQR_Rejected (0.00s)
=== RUN   TestQREngine_InvalidFormatAndMissingClaims
--- PASS: TestQREngine_InvalidFormatAndMissingClaims (0.00s)
=== RUN   TestTicket_StatusValidation
--- PASS: TestTicket_StatusValidation (0.00s)
=== RUN   TestTicket_CanAdmit
--- PASS: TestTicket_CanAdmit (0.00s)
=== RUN   TestTicket_NewJTI
--- PASS: TestTicket_NewJTI (0.00s)
=== RUN   TestIssueTickets_Success_MultipleItems
--- PASS: TestIssueTickets_Success_MultipleItems (0.00s)
=== RUN   TestIssueTickets_OrderNotPaid_Rejected
--- PASS: TestIssueTickets_OrderNotPaid_Rejected (0.00s)
=== RUN   TestIssueTickets_Idempotent_AlreadyIssued
--- PASS: TestIssueTickets_Idempotent_AlreadyIssued (0.00s)
=== RUN   TestIssueTickets_EmptyItems_Rejected
--- PASS: TestIssueTickets_EmptyItems_Rejected (0.00s)
=== RUN   TestVerifyTicket_Success
--- PASS: TestVerifyTicket_Success (0.00s)
=== RUN   TestVerifyTicket_TamperedQR_Rejected
--- PASS: TestVerifyTicket_TamperedQR_Rejected (0.00s)
=== RUN   TestVerifyTicket_ExpiredQR_Rejected
--- PASS: TestVerifyTicket_ExpiredQR_Rejected (0.00s)
=== RUN   TestVerifyTicket_NotFoundInDB_Rejected
--- PASS: TestVerifyTicket_NotFoundInDB_Rejected (0.00s)
=== RUN   TestVerifyTicket_MismatchWithDB_Rejected
--- PASS: TestVerifyTicket_MismatchWithDB_Rejected (0.00s)
=== RUN   TestVerifyTicket_AlreadyCheckedIn_Rejected
--- PASS: TestVerifyTicket_AlreadyCheckedIn_Rejected (0.00s)
=== RUN   TestVerifyTicket_VoidOrRefunded_Rejected
--- PASS: TestVerifyTicket_VoidOrRefunded_Rejected (0.00s)
=== RUN   TestTicketHandler_IssueTickets_Success
--- PASS: TestTicketHandler_IssueTickets_Success (0.00s)
=== RUN   TestTicketHandler_IssueTickets_OrderNotPaid_422
--- PASS: TestTicketHandler_IssueTickets_OrderNotPaid_422 (0.00s)
=== RUN   TestTicketHandler_VerifyTicket_Tampered_400
--- PASS: TestTicketHandler_VerifyTicket_Tampered_400 (0.00s)
=== RUN   TestTicketHandler_VerifyTicket_AlreadyUsed_409
--- PASS: TestTicketHandler_VerifyTicket_AlreadyUsed_409 (0.00s)
=== RUN   TestOutboxTicketIssuedPayload_JSONMarshal
--- PASS: TestOutboxTicketIssuedPayload_JSONMarshal (0.00s)
=== RUN   TestSignalMetrics_ChiTinhYeuCauChamTangDuoi
--- PASS: TestSignalMetrics_ChiTinhYeuCauChamTangDuoi (0.00s)
```

---

## 3. Build Gate

```bash
docker run --rm -v "${PWD}:/app" -w /app golang:1.26-alpine go build -o /tmp/ticketing-server ./services/ticketing/cmd/server
docker run --rm -v "${PWD}:/app" -w /app golang:1.26-alpine go build -o /tmp/ticketing-worker ./services/ticketing/cmd/worker
```
**Output:**
```
(exit code 0 - binaries built successfully)
```
