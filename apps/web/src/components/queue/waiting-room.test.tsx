import { render, screen, act } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { WaitingRoom } from "./waiting-room";

const mockPush = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: mockPush,
  }),
}));

vi.mock("@/hooks/use-server-time-sync", () => ({
  useServerTimeSync: () => ({
    offsetMs: 0,
    synced: true,
    isLoading: false,
    error: null,
    getServerNow: () => Date.now(),
  }),
}));

function jsonResponse(data: unknown, init: { status?: number; headers?: Record<string, string> } = {}) {
  return new Response(JSON.stringify(data), {
    status: init.status ?? 200,
    headers: { "Content-Type": "application/json", ...init.headers },
  });
}

describe("WaitingRoom container (EV-182 AC-6, AC-8)", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    mockPush.mockClear();
    window.sessionStorage.clear();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    window.sessionStorage.clear();
  });

  it("chưa có token: tự động gọi joinQueue và lưu sessionStorage", async () => {
    vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        jsonResponse({
          state: "QUEUED",
          queue_token: "new-token-123",
          rank: 200,
          is_new: true,
          poll_after_ms: 5000,
        })
      )
      .mockResolvedValueOnce(
        jsonResponse({
          state: "QUEUED",
          rank: 200,
          poll_after_ms: 5000,
        })
      );

    render(<WaitingRoom eventId="evt-1" eventTitle="Concert Tri Âm" />);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(window.sessionStorage.getItem("ef:queue:token:evt-1")).toBe("new-token-123");
    expect(screen.getByText("Concert Tri Âm")).toBeInTheDocument();
    expect(screen.getByText("200")).toBeInTheDocument();
  });

  it("đã có token trong sessionStorage: không gọi joinQueue nữa", async () => {
    window.sessionStorage.setItem("ef:queue:token:evt-1", "saved-token-456");

    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      jsonResponse({
        state: "QUEUED",
        rank: 90,
        poll_after_ms: 5000,
      })
    );

    render(<WaitingRoom eventId="evt-1" />);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url] = fetchMock.mock.calls[0];
    expect(String(url)).toContain("/queue/status");
    expect(screen.getByText("90")).toBeInTheDocument();
  });

  it("AC-6: khi state = ADMITTED, tự động redirect sang /checkout/[eventId]", async () => {
    window.sessionStorage.setItem("ef:queue:token:evt-1", "tok-admit");

    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      jsonResponse({
        state: "ADMITTED",
        rank: 0,
        poll_after_ms: 3000,
        expires_at: 1800000000000,
      })
    );

    render(<WaitingRoom eventId="evt-1" autoRedirect={true} />);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(mockPush).toHaveBeenCalledWith("/checkout/evt-1");
    expect(screen.getByText(/Đã đến lượt bạn mua vé!/)).toBeInTheDocument();
  });

  it("EV-146: join lúc hết vé hẳn → hiện màn Hết vé, không có token và không poll status", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      jsonResponse({ state: "SOLD_OUT", queue_token: "", rank: -1, is_new: false, poll_after_ms: 30000 })
    );

    render(<WaitingRoom eventId="evt-1" eventTitle="Concert Tri Âm" />);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000);
    });

    expect(screen.getAllByText("Đã hết vé").length).toBeGreaterThan(0);
    expect(window.sessionStorage.getItem("ef:queue:token:evt-1")).toBeNull();
    expect(fetchMock).toHaveBeenCalledTimes(1); // chỉ có join, không có status
  });

  it("EV-137: token cũ bị status từ chối 401 → tự vào lại hàng đúng một lần, không nháy màn lỗi", async () => {
    window.sessionStorage.setItem("ef:queue:token:evt-1", "0123456789abcdef0123456789abcdef");

    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(jsonResponse({ type: "/problems/queue-token-invalid" }, { status: 401 }))
      .mockResolvedValueOnce(
        jsonResponse({ state: "QUEUED", queue_token: "q1.moi.chuky", rank: 7, is_new: false, poll_after_ms: 5000 })
      )
      .mockResolvedValueOnce(jsonResponse({ state: "QUEUED", rank: 7, poll_after_ms: 5000 }));

    render(<WaitingRoom eventId="evt-1" />);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(fetchMock.mock.calls.map(([u]) => String(u).split("/queue/")[1].split("?")[0])).toEqual([
      "status",
      "join",
      "status",
    ]);
    expect(window.sessionStorage.getItem("ef:queue:token:evt-1")).toBe("q1.moi.chuky");
    expect(screen.queryByText("Phiên xếp hàng không hợp lệ")).not.toBeInTheDocument();
    expect(screen.getByText("7")).toBeInTheDocument();
  });

  it("EV-137: vào lại rồi vẫn 401 thì hiện màn lỗi, không lặp vô hạn", async () => {
    window.sessionStorage.setItem("ef:queue:token:evt-1", "token-cu");

    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(jsonResponse({}, { status: 401 }))
      .mockResolvedValueOnce(
        jsonResponse({ state: "QUEUED", queue_token: "q1.moi.chuky", rank: 7, is_new: false, poll_after_ms: 5000 })
      )
      .mockResolvedValueOnce(jsonResponse({}, { status: 401 }));

    render(<WaitingRoom eventId="evt-1" />);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });

    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(screen.getByText("Phiên xếp hàng không hợp lệ")).toBeInTheDocument();
  });

  it("joinQueue trả về 401 thì hiển thị cảnh báo đăng nhập", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      jsonResponse({ detail: "can dang nhap" }, { status: 401 })
    );

    render(<WaitingRoom eventId="evt-1" />);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(screen.getByText(/Bạn cần đăng nhập và xác thực OTP/)).toBeInTheDocument();
  });

  it("render nút hỗ trợ AI và mở được ChatPanel khi click (EV-185, EVF-115)", async () => {
    window.sessionStorage.setItem("ef:queue:token:evt-1", "saved-token-456");
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      jsonResponse({
        state: "QUEUED",
        rank: 50,
        poll_after_ms: 5000,
      })
    );

    render(<WaitingRoom eventId="evt-1" eventTitle="Sự kiện Tri Âm" />);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    const aiBtn = screen.getByRole("button", { name: /Mở trợ lý ảo AI/ });
    expect(aiBtn).toBeInTheDocument();

    // Click mở panel
    await act(async () => {
      aiBtn.click();
    });

    expect(screen.getByRole("region", { name: "Bảng điều khiển trợ lý AI" })).toBeInTheDocument();
    expect(screen.getByText("Trợ lý EventFlow")).toBeInTheDocument();
  });
});
