# EV-171 — Kế hoạch

## Hợp đồng HTTP

| Method + path | Trả về |
|---|---|
| `POST /v1/events/{event_id}/assistant/messages` body `{"question": str, "queue_token"?: str}` | **202** `{"status":"queued","job_id","stream_url","poll_url","quota":{used,max,reset_at}}` · **200** `{"status":"quota_exceeded","answer","suggestions":[{intent,answer}],"quota"}` · **200** `{"status":"unavailable","answer"}` (nấc OFF) · 401 / 422 RFC 7807 |
| `GET /v1/assistant/messages/{job_id}` | 200 `{"status":"done", ...payload EV-157}` · 202 `{"status":"pending"}` · 404 (không phải chủ / không tồn tại) |
| `GET /v1/assistant/messages/{job_id}/stream` | `text/event-stream`: `event: answer` / `event: timeout`, heartbeat `: ping` |

## Hợp đồng code

```python
# app/api/quota.py
@dataclass(frozen=True) class QuotaDecision: allowed: bool; used: int; limit: int; reset_at_ms: int
class AssistantQuota:  # Redis + scripts/quota.lua
    async def take(self, event_id: str, identity: str, member: str, now_ms: int) -> QuotaDecision
# app/mq/publisher.py
class JobPublisher: async def publish(self, key: str, body: dict, *, message_id: str) -> None
# app/api/assistant.py
@dataclass class AssistantDeps: quota, publish, redis, mode: Callable[[], Mode], stream_timeout_s=65.0
router = APIRouter()
```

## Thứ tự

| Bước | Nội dung | Test |
|---|---|---|
| 1 | `quota.lua` + `AssistantQuota` | tích hợp Redis: 10 qua, 11 chặn; cửa sổ trượt; 50 lần song song chỉ 10 qua |
| 2 | `JobPublisher` | tích hợp: message vào `ai.chat.q` đúng property |
| 3 | Router POST / GET / SSE | unit (TestClient + fake): 202, quota, OFF, 401, 422, 404 chủ khác, pending/done |
| 4 | Nối vào `main.py` (lifespan) | `test_main.py` route có trong app |
| 5 | Đầu-cuối: POST → consumer chat → GET / SSE | tích hợp; đo p95 POST < 50 ms |
| 6 | evidence, traceability, đột biến | — |
