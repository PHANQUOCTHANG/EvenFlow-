import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { connectSse, createSseParser, type SseMessage } from "./sse";

function controlledStream() {
  let ctrl!: ReadableStreamDefaultController<Uint8Array>;
  const stream = new ReadableStream<Uint8Array>({
    start(c) {
      ctrl = c;
    },
  });
  const enc = new TextEncoder();
  return {
    stream,
    send: (s: string) => ctrl.enqueue(enc.encode(s)),
    end: () => ctrl.close(),
    fail: (e: unknown) => ctrl.error(e),
  };
}

describe("createSseParser", () => {
  function collect() {
    const out: SseMessage[] = [];
    return { out, parser: createSseParser((m) => out.push(m)) };
  }

  it("event + data, dispatch khi gap dong trong", () => {
    const { out, parser } = collect();
    parser.push('event: position\ndata: {"rank":5}\n\n');
    expect(out).toEqual([{ event: "position", data: '{"rank":5}', id: undefined }]);
  });

  it("chua co dong trong thi CHUA dispatch", () => {
    const { out, parser } = collect();
    parser.push("event: position\ndata: x\n");
    expect(out).toHaveLength(0);
    parser.push("\n");
    expect(out).toHaveLength(1);
  });

  it("su kien bi cat giua nhieu chunk", () => {
    const { out, parser } = collect();
    parser.push("eve");
    parser.push("nt: position\nda");
    parser.push("ta: 1\n\n");
    expect(out).toEqual([{ event: "position", data: "1", id: undefined }]);
  });

  it("data nhieu dong duoc noi bang \\n; khong co event -> 'message'", () => {
    const { out, parser } = collect();
    parser.push("data: a\ndata: b\n\n");
    expect(out).toEqual([{ event: "message", data: "a\nb", id: undefined }]);
  });

  it("CRLF va CR, ke ca \\r\\n bi cat doi giua hai chunk, khong dispatch som", () => {
    const { out, parser } = collect();
    parser.push("data: a\r");
    parser.push("\ndata: b\r\n\r\n");
    parser.push("data: c\r\rdata: d\n\n");
    expect(out.map((m) => m.data)).toEqual(["a\nb", "c", "d"]);
  });

  it("bo qua comment/heartbeat va field la; ghi nhan id", () => {
    const { out, parser } = collect();
    parser.push(": ping\nretry: 10\nid: 7\nfoo\ndata\n\n");
    expect(out).toEqual([{ event: "message", data: "", id: "7" }]);
  });

  it("dong trong khong co data -> khong dispatch", () => {
    const { out, parser } = collect();
    parser.push("event: x\n\n");
    expect(out).toHaveLength(0);
  });

  it("flush bo su kien dang do", () => {
    const { out, parser } = collect();
    parser.push("data: half");
    parser.flush();
    parser.push("\n\n");
    expect(out).toHaveLength(0);
  });
});

describe("connectSse", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("gui header (X-Queue-Token) va Accept, goi onOpen, chuyen tiep message, ket thuc 'ended'", async () => {
    const s = controlledStream();
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(s.stream));
    const onMessage = vi.fn();
    const onOpen = vi.fn();

    const conn = connectSse({ url: "/stream", headers: { "X-Queue-Token": "tok" }, onMessage, onOpen });
    await vi.advanceTimersByTimeAsync(0);
    expect(onOpen).toHaveBeenCalledTimes(1);

    const headers = fetchMock.mock.calls[0][1]?.headers as Record<string, string>;
    expect(headers["X-Queue-Token"]).toBe("tok");
    expect(headers.Accept).toBe("text/event-stream");

    s.send("event: position\ndata: 1\n\n");
    await vi.advanceTimersByTimeAsync(0);
    expect(onMessage).toHaveBeenCalledWith({ event: "position", data: "1", id: undefined });

    s.end();
    await expect(conn.done).resolves.toBe("ended");
    expect(fetchMock).toHaveBeenCalledTimes(1); // server dong binh thuong -> KHONG tu mo lai
  });

  it("AC-2: loi mang -> reconnect theo backoff 1s, 2s (khong reconnect ngay)", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0);
    const s = controlledStream();
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockRejectedValueOnce(new TypeError("offline"))
      .mockRejectedValueOnce(new TypeError("offline"))
      .mockResolvedValueOnce(new Response(s.stream));
    const onRetry = vi.fn();

    const conn = connectSse({ url: "/stream", onMessage: vi.fn(), onRetry });
    await vi.advanceTimersByTimeAsync(0);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(onRetry).toHaveBeenLastCalledWith(1, 1000, expect.any(TypeError));

    await vi.advanceTimersByTimeAsync(999);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(onRetry).toHaveBeenLastCalledWith(2, 2000, expect.any(TypeError));

    await vi.advanceTimersByTimeAsync(2000);
    expect(fetchMock).toHaveBeenCalledTimes(3);

    conn.close();
    await expect(conn.done).resolves.toBe("closed");
  });

  it("AC-2: vuot maxRetries -> 'gave-up', so request bi chan tren", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0);
    const fetchMock = vi.spyOn(globalThis, "fetch").mockRejectedValue(new TypeError("offline"));

    const conn = connectSse({ url: "/stream", onMessage: vi.fn(), maxRetries: 2 });
    await vi.advanceTimersByTimeAsync(60_000);
    await expect(conn.done).resolves.toBe("gave-up");
    expect(fetchMock).toHaveBeenCalledTimes(3); // 1 lan dau + 2 lan thu lai
  });

  it("503 kem Retry-After dai hon backoff -> cho theo Retry-After", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0);
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(null, { status: 503, headers: { "Retry-After": "5" } }))
      .mockResolvedValueOnce(new Response(controlledStream().stream));
    const onRetry = vi.fn();

    const conn = connectSse({ url: "/stream", onMessage: vi.fn(), onRetry });
    await vi.advanceTimersByTimeAsync(0);
    expect(onRetry).toHaveBeenCalledWith(1, 5000, expect.anything());
    await vi.advanceTimersByTimeAsync(4999);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    conn.close();
    await conn.done;
  });

  it("4xx -> 'fatal', khong thu lai", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(null, { status: 400 }));
    const conn = connectSse({ url: "/stream", onMessage: vi.fn() });
    await expect(conn.done).resolves.toBe("fatal");
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("200 nhung khong co body (khong ho tro streaming) -> 'fatal'", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(null, { status: 200 }));
    const conn = connectSse({ url: "/stream", onMessage: vi.fn() });
    await expect(conn.done).resolves.toBe("fatal");
  });

  it("stream dut giua chung -> reconnect, bo dem loi da reset sau lan mo thanh cong", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0);
    const first = controlledStream();
    const second = controlledStream();
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(first.stream))
      .mockResolvedValueOnce(new Response(second.stream));
    const onRetry = vi.fn();
    const onOpen = vi.fn();

    const conn = connectSse({ url: "/stream", onMessage: vi.fn(), onRetry, onOpen });
    await vi.advanceTimersByTimeAsync(0);
    first.fail(new TypeError("reset"));
    await vi.advanceTimersByTimeAsync(0);
    expect(onRetry).toHaveBeenCalledWith(1, 1000, expect.anything());
    await vi.advanceTimersByTimeAsync(1000);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(onOpen).toHaveBeenCalledTimes(2);
    conn.close();
    await expect(conn.done).resolves.toBe("closed");
  });

  it("close() trong luc dang cho backoff -> 'closed' ngay, khong goi them fetch", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockRejectedValue(new TypeError("offline"));
    const conn = connectSse({ url: "/stream", onMessage: vi.fn() });
    await vi.advanceTimersByTimeAsync(0);
    conn.close();
    await expect(conn.done).resolves.toBe("closed");
    await vi.advanceTimersByTimeAsync(60_000);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("close() khi dang doc stream -> 'closed'", async () => {
    const s = controlledStream();
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(s.stream));
    const conn = connectSse({ url: "/stream", onMessage: vi.fn() });
    await vi.advanceTimersByTimeAsync(0);
    conn.close();
    await expect(conn.done).resolves.toBe("closed");
  });
});
