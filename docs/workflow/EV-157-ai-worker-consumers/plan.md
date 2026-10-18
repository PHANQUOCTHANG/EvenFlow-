# EV-157 — Kế hoạch thực thi (Plan)

Đầu vào: `spec.md` (AC-1..AC-11), `baseline.md`. Chạy lệnh trong `services/ai-worker` bằng `uv` 0.9.30.

## 1. Hợp đồng chốt trước

```python
# app/mq/protocol.py  -- THUAN, khong import aio-pika
ATTEMPT_HEADER = "x-ef-attempt"; ERROR_HEADER = "x-ef-error"; MAX_ATTEMPTS = 3
class PermanentError(Exception): ...              # errors.Is(ErrPermanent) ben Go
class Action(enum.Enum): ACK, RETRY, DEAD_LETTER
def attempt_of(headers: Mapping[str, Any] | None) -> int
def decide(err: BaseException | None, attempt: int, max_attempts: int = MAX_ATTEMPTS) -> Action
def forward_headers(headers, attempt: int, error: str | None = None) -> dict[str, Any]

# app/mq/topology.py
@dataclass(frozen=True) class Topology: exchanges, queues, bindings
def load_ai_topology(path: Path) -> Topology          # chi lay ai.*; thieu file / sai JSON -> loi
async def declare(channel, topo: Topology) -> None    # exchanges -> queues -> bindings

# app/mq/runtime.py
Handler = Callable[[Delivery], Awaitable[None]]
@dataclass(frozen=True) class Delivery: body, headers, message_id, routing_key, attempt, timestamp
@dataclass(frozen=True) class PoolSpec: key: str; concurrency: int; handler: Handler
class ConsumerRuntime:
    def __init__(self, connection, specs: list[PoolSpec], metrics: Metrics, *, grace_seconds: float)
    async def start(self) -> None
    async def stop(self) -> None
    def in_flight(self, key: str) -> int
async def process(msg, spec, forwarder, metrics, stopping: Callable[[], bool]) -> str  # tra outcome

# app/pipeline/results.py
class RedisResultSink: async def publish(self, job_id: str, payload: dict) -> None

# app/gemini/client.py (them)
class GeminiClient(api_key=None, model=None, *, sdk=None, timeout_seconds=10.0, embed_model=..., on_rate_limited=None)
    async def generate(prompt, *, max_output_tokens) -> Generation   # 429 -> RateLimited
    async def embed(text) -> list[float]
class RateLimited(Exception)                          # chat doi thanh Retry
def mock_embedding(text: str, dims: int = 64) -> list[float]
```

Metric tên cố định (khớp `deploy/observability`): `ai_queue_lag_seconds{queue}`, `ai_queue_depth{queue}`,
`ai_jobs_total{queue,outcome}`, `ai_job_duration_seconds{queue}`, `ai_gemini_429_total`. Metric gom vào
`app/metrics.py` để runtime và `main.py` dùng chung một registry.

## 2. Thứ tự

| Bước | Nội dung | Test đi kèm |
|---|---|---|
| 1 | `config.py`: thêm `gemini_timeout_seconds`, `gemini_embed_model`, `shutdown_grace_seconds`, `ai_topology_file`, validator | `tests/test_config.py` |
| 2 | `mq/protocol.py` | `tests/mq/test_protocol.py` — bảng như test Go `decide`/`attemptOf`/`copyHeaders` |
| 3 | `mq/topology.py` | `tests/mq/test_topology.py` — đọc file thật của repo, chỉ `ai.*`, đếm đúng 3 exchange / 9 queue / 12 binding; `declare` với channel giả theo thứ tự |
| 4 | `metrics.py`, `logging.py` | `tests/test_logging.py` — JSON, `trace_id` từ `traceparent`, không có trường câu hỏi |
| 5 | `mq/runtime.py` | `tests/mq/test_runtime.py` — `process` với message giả: ack / retry / DLQ / requeue khi publish lỗi / requeue khi đang tắt / lag ghi đúng |
| 6 | `pipeline/results.py` | `tests/test_results.py` — fake Redis ghi SET trước PUBLISH, TTL 300 |
| 7 | `gemini/client.py` | `tests/test_gemini_client.py` — SDK giả: token thật, timeout, 429 → `RateLimited` + counter, embed mock tất định |
| 8 | `consumers/chat.py` adapter, `consumers/unsupported.py` | `tests/test_chat_handler.py` — `RateLimited` → `Retry`, job hết hạn → ack |
| 9 | `main.py`: lifespan thay `on_event`, dựng 4 pool, sampler lag | `tests/test_main.py` — `build_specs` đúng số consumer, pool 0 bị bỏ |
| 10 | Tích hợp: `tests/integration/` (RabbitMQ 3.13 + Redis 7) | AC-1, AC-4, AC-5, AC-8 trên broker thật |
| 11 | `pyproject.toml` (thêm `httpx`, marker), `uv lock`; Dockerfile copy `definitions.json`; compose env; `integration.yml` thêm bước Python | — |
| 12 | evidence, traceability, code-review, challenge | — |

## 3. File thay đổi

Thêm: `app/mq/{__init__,protocol,topology,runtime}.py`, `app/metrics.py`, `app/logging.py`,
`app/pipeline/results.py`, `app/consumers/unsupported.py`, `tests/**`.
Sửa: `app/main.py`, `app/config.py`, `app/gemini/client.py`, `app/consumers/chat.py`, `pyproject.toml`, `uv.lock`,
`deploy/docker/ai-worker.Dockerfile`, `deploy/compose/docker-compose.yml` (env), `.github/workflows/integration.yml`.
Không sửa: `libs/go/**`, `deploy/rabbitmq/definitions.json`, `services/event/**`.

## 4. Rủi ro

| Rủi ro | Xử lý |
|---|---|
| `aio-pika` trả header số dưới dạng `int` Python; Go gửi int32 | `attempt_of` nhận mọi số nguyên + chuỗi, giống Go |
| Đặt `x-ef-attempt` từ Python thành int64 (`l`) thay vì int32 | Go `attemptOf` nhận cả int64 → tương thích hai chiều; ghi rõ trong test tích hợp |
| Test AC-1 chập chờn trên Docker Desktop | Ngưỡng 5 giây theo Jira; đo lag lớn nhất, chạy lặp 3 lần trong evidence |
