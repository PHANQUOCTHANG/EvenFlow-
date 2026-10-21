import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { WaitingRoom } from "./waiting-room";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
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

function problemResponse(status: number, type: string) {
  return new Response(JSON.stringify({ type, title: "t", status, detail: "chi tiet" }), {
    status,
    headers: { "Content-Type": "application/problem+json" },
  });
}

describe("WaitingRoom — loi join co the hanh dong (AC-9)", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    window.sessionStorage.clear();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    window.sessionStorage.clear();
  });

  async function renderWithJoinError(type: string) {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(problemResponse(403, type));
    render(<WaitingRoom eventId="evt-1" />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
  }

  it("403 otp-required: huong dan xac thuc OTP + link dang nhap lai", async () => {
    await renderWithJoinError("/problems/otp-required");

    expect(screen.getByText("Cần xác thực OTP")).toBeInTheDocument();
    expect(screen.getByText(/chưa xác thực OTP nên chưa thể vào phòng chờ/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Xác thực OTP và đăng nhập lại" })).toHaveAttribute(
      "href",
      "/auth/sign-in",
    );
  });

  it("403 device-required: huong dan dang nhap lai tren thiet bi nay", async () => {
    await renderWithJoinError("/problems/device-required");

    expect(screen.getByText("Cần đăng nhập lại trên thiết bị này")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Đăng nhập lại" })).toHaveAttribute("href", "/auth/sign-in");
  });

  it("403 chung (forbidden) van la thong bao chung, khong co link OTP", async () => {
    await renderWithJoinError("/problems/forbidden");

    expect(screen.getByText(/Yêu cầu cần xác minh thêm/)).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /Đăng nhập lại|Xác thực OTP/ })).toBeNull();
  });
});
