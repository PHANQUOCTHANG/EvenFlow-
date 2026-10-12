# EV-182 — Kế hoạch thực thi (Plan)

## 1. Thứ tự thực hiện

| Bước | Nội dung | Lý do / Ràng buộc |
|---|---|---|
| **1** | `lib/backoff.ts` + test | Chính sách lùi dùng chung cho polling, SSE, join (AC-2). Hàm thuần, inject `random` để test xác định |
| **2** | `lib/queue-status.ts` + test | Một lần gọi `/queue/status` → kết quả phân loại (updated / not-modified / throttled), chuẩn hoá `-1` → `null`, kẹp `poll_after_ms` (AC-1) |
| **3** | `lib/sse.ts` + test | Parser SSE + kết nối qua `fetch` (gửi được `X-Queue-Token`), reconnect backoff, giới hạn số lần thử (AC-2, AC-4) |
| **4** | `hooks/use-queue-status.ts` + test | Điều phối polling ↔ SSE, connection state, initialRank, không refetch theo visibility/online (AC-1..AC-4, AC-7) |
| **5** | `ProgressRing`, `QueuePosition`, `AdmitBanner` + test | UI trung thực, a11y (`role=progressbar`, `role=alert`, aria-live chỉ cho tiêu đề) (AC-5, AC-6) |
| **6** | `WaitingRoom` container + route `/waiting/[eventId]` + test | Join/tái sử dụng token, auto-redirect một lần tới `/checkout/[eventId]` (AC-6, AC-8) |
| **7** | Gate: lint, typecheck, test:coverage, build | Thu raw log cho `evidence.md`, cập nhật `traceability.md` |
| **8** | Review đối kháng | `code-review.md` |

## 2. Danh sách File thay đổi

### Thêm mới
- `apps/web/src/lib/backoff.ts` / `backoff.test.ts`
- `apps/web/src/lib/queue-status.ts` / `queue-status.test.ts`
- `apps/web/src/lib/sse.ts` / `sse.test.ts`
- `apps/web/src/hooks/use-queue-status.ts` / `use-queue-status.test.tsx`
- `apps/web/src/components/queue/progress-ring.tsx` / `progress-ring.test.tsx`
- `apps/web/src/components/queue/queue-position.tsx` / `queue-position.test.tsx`
- `apps/web/src/components/queue/admit-banner.tsx` / `admit-banner.test.tsx`
- `apps/web/src/components/queue/waiting-room.tsx` / `waiting-room.test.tsx`
- `apps/web/src/app/(queue)/waiting/[eventId]/page.tsx` / `page.test.tsx`
- `docs/workflow/EV-182/*.md` (6 file hồ sơ)

### Sửa đổi
- Không có file sản phẩm hiện hữu nào bị sửa.

### TUYỆT ĐỐI CẤM SỬA
- `apps/web/src/lib/queue-client.ts` và `queue-client.test.ts` (hợp đồng jitter/poll đã được test).
- `apps/web/src/components/queue/queue-status-panel.tsx` và test của nó.
- `apps/web/src/app/(queue)/layout.tsx` + test (shell không nav).
- Toàn bộ 53 test file baseline; `vitest.config.ts`, `tsconfig.json`, `eslint.config.mjs` (không hạ ngưỡng).
- Mọi mã backend `services/**` (Lua gate, transaction, oversell).

## 3. Hợp đồng API / Type Signatures

```ts
// lib/backoff.ts
export interface BackoffOptions { baseMs?: number; maxMs?: number; jitterMs?: number; random?: () => number }
export function backoffDelay(attempt: number, options?: BackoffOptions): number; // attempt >= 1

// lib/queue-status.ts
export const DEFAULT_POLL_MS = 10_000;
export const MIN_POLL_MS = 1_000;
export interface QueueSnapshot {
  state: QueueState;            // tu queue-status-panel (LOBBY|QUEUED|ADMITTED|EXPIRED|SOLD_OUT|...|UNKNOWN)
  rank: number | null;          // null khi server tra -1 / khong hop le
  etaSeconds: number | null;    // null khi server tra -1 / khong hop le
  pollAfterMs: number;          // da sanitize
  expiresAt: number | null;
}
export type StatusResult =
  | { kind: "updated"; snapshot: QueueSnapshot; etag: string | null; pollAfterMs: number }
  | { kind: "not-modified"; etag: string | null; pollAfterMs: number }
  | { kind: "throttled"; retryAfterMs: number };
export function sanitizePollAfterMs(value: unknown, fallback?: number): number;
export function normalizePosition(raw: unknown): QueueSnapshot;
export function isTerminalState(state: QueueState): boolean;
export function fetchQueueStatus(eventId: string, token: string,
  opts?: { etag?: string | null; signal?: AbortSignal; baseUrl?: string }): Promise<StatusResult>; // throw QueueError

// lib/sse.ts
export interface SseMessage { event: string; data: string; id?: string }
export function createSseParser(onMessage: (m: SseMessage) => void): { push(chunk: string): void; flush(): void };
export type SseEndReason = "closed" | "ended" | "gave-up" | "fatal";
export interface SseConnectOptions {
  url: string; headers?: Record<string, string>;
  onMessage: (m: SseMessage) => void; onOpen?: () => void;
  onRetry?: (attempt: number, delayMs: number, error: unknown) => void;
  maxRetries?: number; backoff?: BackoffOptions;
}
export function connectSse(opts: SseConnectOptions): { close(): void; done: Promise<SseEndReason> };

// hooks/use-queue-status.ts
export type QueueTransport = "idle" | "polling" | "sse";
export interface UseQueueStatusOptions { eventId: string; token: string | null; enableSse?: boolean; sseThresholdMs?: number }
export interface QueueStatusView {
  snapshot: QueueSnapshot | null; initialRank: number | null;
  connection: ConnectionState; transport: QueueTransport;
  lastUpdatedAt: number | null; error: QueueError | null;
}
export function useQueueStatus(options: UseQueueStatusOptions): QueueStatusView;
```
