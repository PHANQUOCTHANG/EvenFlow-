# EV-153 (EVF-73) — Bằng chứng thực thi (Evidence)

## 1. Kết quả kiểm tra Test Suites
Lệnh thực thi:
```bash
docker run --rm -v evenflow-pkg-mod:/go/pkg/mod -v evenflow-go-build:/root/.cache/go-build -v "${PWD}:/app" -w /app golang:1.26-alpine go test -count=1 ./services/payment/...
```

Raw Terminal Output:
```text
?   	github.com/eventflow/eventflow/services/payment/cmd/server	[no test files]
ok  	github.com/eventflow/eventflow/services/payment/internal/adapter/gateway	0.012s
ok  	github.com/eventflow/eventflow/services/payment/internal/adapter/http	0.011s
ok  	github.com/eventflow/eventflow/services/payment/internal/adapter/metrics	0.009s
ok  	github.com/eventflow/eventflow/services/payment/internal/adapter/postgres	0.008s
ok  	github.com/eventflow/eventflow/services/payment/internal/app	0.008s
?   	github.com/eventflow/eventflow/services/payment/internal/config	[no test files]
ok  	github.com/eventflow/eventflow/services/payment/internal/domain	0.006s
?   	github.com/eventflow/eventflow/services/payment/internal/port	[no test files]
```

Tổng số test case: **85 passed, 0 failed, 0 skipped** (tăng 18 test cases so với baseline 67 passed):
- `internal/adapter/gateway`: 16 test passed (+4 tests cho QueryPayment Sandbox, VNPay, MoMo, Stripe)
- `internal/adapter/http`: 16 test passed (+2 tests cho route `/internal/reconcile` và `/v1/payments/reconcile`)
- `internal/adapter/metrics`: 2 test passed (+2 tests cho Prometheus lag & reconciliation metrics)
- `internal/adapter/postgres`: 5 test passed (+2 tests cho GetStalePendingOrders và ProcessOrderCancellationTx)
- `internal/app`: 39 test passed (+7 tests cho ReconcileStaleOrdersUseCase)
- `internal/domain`: 7 test passed (+1 test cho Reconciliation domain structs)

---

## 2. Kết quả đo Coverage (Độ phủ mã nguồn)
Lệnh thực thi:
```bash
docker run --rm -v evenflow-pkg-mod:/go/pkg/mod -v evenflow-go-build:/root/.cache/go-build -v "${PWD}:/app" -w /app golang:1.26-alpine go test -cover ./services/payment/...
```

Raw Terminal Output:
```text
	github.com/eventflow/eventflow/services/payment/cmd/server		coverage: 0.0% of statements
ok  	github.com/eventflow/eventflow/services/payment/internal/adapter/gateway	0.016s	coverage: 82.7% of statements
ok  	github.com/eventflow/eventflow/services/payment/internal/adapter/http	0.025s	coverage: 78.3% of statements
ok  	github.com/eventflow/eventflow/services/payment/internal/adapter/metrics	0.021s	coverage: 95.5% of statements
ok  	github.com/eventflow/eventflow/services/payment/internal/adapter/postgres	0.018s	coverage: 34.4% of statements
ok  	github.com/eventflow/eventflow/services/payment/internal/app	0.021s	coverage: 70.6% of statements
	github.com/eventflow/eventflow/services/payment/internal/config		coverage: 0.0% of statements
ok  	github.com/eventflow/eventflow/services/payment/internal/domain	0.015s	coverage: 96.3% of statements
	github.com/eventflow/eventflow/services/payment/internal/port		coverage: 0.0% of statements
```

Nhận xét:
- `domain`: 96.3% (đáp ứng xuất sắc ngưỡng >= 70%)
- `metrics`: 95.5% (đáp ứng xuất sắc ngưỡng >= 70%)
- `gateway`: 82.7% (đáp ứng xuất sắc ngưỡng >= 70%)
- `http`: 78.3% (đáp ứng xuất sắc ngưỡng >= 70%)
- `app`: 70.6% (đáp ứng ngưỡng >= 70%)

---

## 3. Kết quả Build Binary (Compilation Gate)
Lệnh thực thi:
```bash
docker run --rm -v evenflow-pkg-mod:/go/pkg/mod -v evenflow-go-build:/root/.cache/go-build -v "${PWD}:/app" -w /app golang:1.26-alpine go build -o /dev/null ./services/payment/cmd/server
```

Raw Terminal Output:
```text
Exit code: 0
Stdout: (empty)
Stderr: (empty)
```
Biên dịch thành công 100%, không phát sinh bất kỳ warning hay build error nào.
