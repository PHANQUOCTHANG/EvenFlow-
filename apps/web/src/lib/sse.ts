/** Client SSE doc qua `fetch` + `ReadableStream` (EV-182 AC-2, AC-4).
 *
 * Vi sao KHONG dung `EventSource` goc: endpoint `/queue/stream` cua waitingroom bat buoc header
 * `X-Queue-Token`, ma `EventSource` khong gui duoc header tuy chinh. Dua token len query string
 * thi token se nam trong access log cua moi proxy/CDN tren duong di — khong chap nhan duoc.
 *
 * Reconnect tu viet thay vi de trinh duyet tu lo: `EventSource` reconnect theo nhip co dinh
 * (~3s) cho moi client, nen khi mot pod chet, toan bo client cua pod do quay lai CUNG LUC. O day
 * dung `backoffDelay` (luy thua + jitter) va co tran so lan thu; vuot tran thi ket thuc de hook
 * roi ve polling, khong retry mai mai. */

import { backoffDelay, type BackoffOptions } from "@/lib/backoff";

export interface SseMessage {
  event: string;
  data: string;
  id?: string;
}

/** Ly do stream ket thuc.
 *  - `closed`: caller tu dong (`close()`).
 *  - `ended`: server dong stream binh thuong (vd: sau khi ADMITTED).
 *  - `gave-up`: loi lien tiep vuot `maxRetries`.
 *  - `fatal`: loi khong nen thu lai (4xx, trinh duyet khong ho tro streaming). */
export type SseEndReason = "closed" | "ended" | "gave-up" | "fatal";

export interface SseParser {
  push(chunk: string): void;
  /** Bo phan su kien dang do khi stream dong (dung theo dac ta HTML: su kien chua co dong trong
   *  ket thuc thi khong dispatch). */
  flush(): void;
}

/** Parser dong `text/event-stream` toi gian: ho tro `event`, `data` nhieu dong, `id`, comment
 *  (`:`), va ca ba kieu xuong dong `\n`, `\r\n`, `\r` — ke ca khi `\r\n` bi cat doi giua hai chunk. */
export function createSseParser(onMessage: (message: SseMessage) => void): SseParser {
  let buffer = "";
  let eventName = "";
  let dataLines: string[] = [];
  let lastId: string | undefined;

  function dispatch() {
    if (dataLines.length > 0) {
      onMessage({ event: eventName || "message", data: dataLines.join("\n"), id: lastId });
    }
    eventName = "";
    dataLines = [];
  }

  function handleLine(line: string) {
    if (line === "") {
      dispatch();
      return;
    }
    if (line.startsWith(":")) return; // comment / heartbeat

    const colon = line.indexOf(":");
    const field = colon === -1 ? line : line.slice(0, colon);
    let value = colon === -1 ? "" : line.slice(colon + 1);
    if (value.startsWith(" ")) value = value.slice(1);

    switch (field) {
      case "event":
        eventName = value;
        break;
      case "data":
        dataLines.push(value);
        break;
      case "id":
        lastId = value;
        break;
      default:
        // `retry` va field la: bo qua. Nhip reconnect do backoffDelay quyet, khong do server stream.
        break;
    }
  }

  return {
    push(chunk: string) {
      buffer += chunk;
      // `\r` o cuoi co the la nua dau cua `\r\n` — giu lai cho chunk sau.
      const cut = buffer.endsWith("\r") ? buffer.length - 1 : buffer.length;
      const pending = buffer.slice(cut);
      const lines = buffer.slice(0, cut).replace(/\r\n?/g, "\n").split("\n");
      buffer = (lines.pop() ?? "") + pending;
      for (const line of lines) handleLine(line);
    },
    flush() {
      buffer = "";
      eventName = "";
      dataLines = [];
    },
  };
}

class SseHttpError extends Error {
  constructor(
    public readonly status: number,
    public readonly retryAfterMs: number | null,
  ) {
    super(`SSE HTTP ${status}`);
    this.name = "SseHttpError";
  }
}

function retryAfterMs(header: string | null): number | null {
  if (header === null) return null;
  const seconds = Number(header);
  return Number.isFinite(seconds) && seconds > 0 ? Math.floor(seconds * 1000) : null;
}

export interface SseConnectOptions {
  url: string;
  headers?: Record<string, string>;
  onMessage: (message: SseMessage) => void;
  onOpen?: () => void;
  /** Goi truoc moi lan cho de reconnect. */
  onRetry?: (attempt: number, delayMs: number, error: unknown) => void;
  /** So lan thu lai LIEN TIEP toi da truoc khi bo cuoc. Mac dinh 5. */
  maxRetries?: number;
  backoff?: BackoffOptions;
}

export interface SseConnection {
  close(): void;
  done: Promise<SseEndReason>;
}

export const DEFAULT_SSE_MAX_RETRIES = 5;

export function connectSse(options: SseConnectOptions): SseConnection {
  const controller = new AbortController();
  const maxRetries = options.maxRetries ?? DEFAULT_SSE_MAX_RETRIES;
  let closed = false;

  const wait = (ms: number) =>
    new Promise<void>((resolve) => {
      const timer = setTimeout(resolve, ms);
      controller.signal.addEventListener(
        "abort",
        () => {
          clearTimeout(timer);
          resolve();
        },
        { once: true },
      );
    });

  async function run(): Promise<SseEndReason> {
    let failures = 0;

    while (!closed) {
      try {
        const res = await fetch(options.url, {
          headers: { Accept: "text/event-stream", ...options.headers },
          credentials: "include",
          cache: "no-store",
          signal: controller.signal,
        });

        if (!res.ok) {
          const retryable = res.status >= 500 || res.status === 429;
          if (!retryable) return "fatal";
          throw new SseHttpError(res.status, retryAfterMs(res.headers.get("Retry-After")));
        }
        if (!res.body) return "fatal";

        failures = 0;
        options.onOpen?.();

        const reader = res.body.getReader();
        // Khong tin rang moi tang (proxy, polyfill, mock) deu noi abort vao body stream: huy
        // reader tuong minh de `close()` luon ket thuc duoc vong doc, khong de ro ri ket noi.
        controller.signal.addEventListener(
          "abort",
          () => {
            reader.cancel().catch(() => undefined);
          },
          { once: true },
        );
        const decoder = new TextDecoder();
        const parser = createSseParser(options.onMessage);

        for (;;) {
          const { done, value } = await reader.read();
          if (done) break;
          parser.push(decoder.decode(value, { stream: true }));
        }
        parser.push(decoder.decode());
        parser.flush();
        return closed ? "closed" : "ended";
      } catch (error) {
        if (closed || controller.signal.aborted) return "closed";

        failures += 1;
        if (failures > maxRetries) return "gave-up";

        let delay = backoffDelay(failures, options.backoff);
        if (error instanceof SseHttpError && error.retryAfterMs !== null) {
          delay = Math.max(delay, error.retryAfterMs);
        }
        options.onRetry?.(failures, delay, error);
        await wait(delay);
      }
    }

    return "closed";
  }

  const done = run();

  return {
    close() {
      closed = true;
      controller.abort();
    },
    done,
  };
}
