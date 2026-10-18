# EV-159 — Kế hoạch

## Hợp đồng

```python
# app/mq/protocol.py
def is_permanent(err: BaseException) -> bool
def decide(err, attempt, max_attempts=MAX_ATTEMPTS) -> Action   # dung is_permanent

# app/metrics.py
DLQ_DEPTH = Gauge("ai_dlq_depth", ...)
DEAD_LETTERED = Counter("ai_dead_lettered_total", ..., ["queue", "reason"])   # permanent | exhausted
```

## Thứ tự

| Bước | Nội dung | Test |
|---|---|---|
| 1 | `is_permanent` + `decide` | `tests/mq/test_protocol.py`: bảng status 400/404/409/422 → vĩnh viễn; 408/425/429/500/503 → tạm; exception giả có `.response.status_code` / `.code`; `json.JSONDecodeError`, `UnicodeDecodeError` |
| 2 | Runtime: counter theo lý do, sampler đo `ai.dlq` | `tests/mq/test_runtime.py`; tích hợp: callback 404 → DLQ ngay, `ai_dlq_depth` > 0 |
| 3 | Lỗi thật của thư viện: `httpx.HTTPStatusError` 404 / 503, `google.genai.errors.ClientError` 400 / 429 | `tests/test_classify_real_errors.py` |
| 4 | `alerts.yml` + `alerts_test.yml` | `promtool check rules`, `promtool test rules` (image `prom/prometheus:v3.5.0`) |
| 5 | `ci.yml`: filter + bước promtool | `test-workflows.sh`, actionlint |
| 6 | evidence, traceability, đột biến | — |
