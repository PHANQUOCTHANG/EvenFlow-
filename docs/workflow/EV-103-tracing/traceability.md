# EV-103 — Truy vết AC → test

Unit: `libs/go/otelx/otelx_test.go` (`O:`), `services/gateway/internal/proxy/trace_test.go` (`G:`). Tích hợp Postgres: `services/payment/internal/worker/outbox_relay_integration_test.go` (`P:`).
Script: `deploy/scripts/demo.sh` bước 10.

| AC | Test |
|---|---|
| AC-1 | G: `NoiTraceSangUpstream`, `KhoiTaoTraceKhiClientKhongGui` · O: `InjectHTTP_GhiTraceparentCuaSpanHienTai` |
| AC-2 | O: `Traceparent_LuuRoiKhoiPhuc` (gồm `""`, mã request cũ, chuỗi hỏng) |
| AC-3 | P: `Relay_NoiTiepTraceTuOutbox` |
| AC-4 | demo bước 10: tự truy vấn Jaeger, đòi trace ≥ 4 service |
