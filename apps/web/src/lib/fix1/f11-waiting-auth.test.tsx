/** Regression F-11 (C-5, X-9): AUTH_REQUIRED in the waiting room is a sign-in prompt, not a network error; polling stops. */
import { act, render, renderHook, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { WaitingRoom } from "@/components/queue/waiting-room";
import { useQueueStatus } from "@/hooks/use-queue-status";
import { saveQueueToken } from "@/lib/queue-token";
import { installFetch, json, problem, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock("@/hooks/use-server-time-sync", () => ({
  useServerTimeSync: () => ({ offsetMs: 0, synced: true, isLoading: false, error: null, getServerNow: () => Date.now() }),
}));
vi.mock("@/components/assistant/chat-panel", () => ({ ChatPanel: () => null }));

beforeEach(setupEnv);
afterEach(resetEnv);

const handler = (joinStatus: string) => (c: { path: string }) =>
  c.path === "/api/v1/auth/refresh"
    ? problem(401, "/problems/unauthorized")
    : c.path.endsWith("/queue/join") || c.path.endsWith("/queue/status")
      ? problem(401, joinStatus)
      : json({});

describe("F-11", () => {
  it("join: refresh rejected -> signInRequired message with sign-in link, not 'lỗi mạng'", async () => {
    seedSession("old", "r1");
    installFetch(handler("/problems/token-expired"));
    await act(async () => {
      render(<WaitingRoom eventId="e1" />);
    });
    await act(async () => {
      await Promise.resolve();
    });
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Bạn cần đăng nhập trước khi vào phòng chờ");
    expect(alert.textContent).not.toContain("lỗi mạng");
    expect(screen.getByRole("link", { name: /đăng nhập/i })).toHaveAttribute("href", "/auth/sign-in");
  });

  it("poll: refresh rejected -> terminal sign-in prompt and no further polling", async () => {
    seedSession("old", "r1");
    saveQueueToken("e1", "q-tok");
    const { calls } = installFetch(handler("/problems/token-expired"));
    await act(async () => {
      render(<WaitingRoom eventId="e1" />);
    });
    await act(async () => {
      await new Promise((r) => setTimeout(r, 50));
    });
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Bạn cần đăng nhập trước khi vào phòng chờ");
    expect(screen.getByRole("link", { name: /đăng nhập/i })).toBeInTheDocument();
    const statusCalls = calls.filter((c) => c.path.endsWith("/queue/status")).length;
    await act(async () => {
      await new Promise((r) => setTimeout(r, 100));
    });
    expect(calls.filter((c) => c.path.endsWith("/queue/status")).length).toBe(statusCalls);
  });

  it("hook: AUTH_REQUIRED becomes terminal error and transport idle (not reconnecting)", async () => {
    seedSession("old", "r1");
    installFetch(handler("/problems/token-expired"));
    const { result } = renderHook(() => useQueueStatus({ eventId: "e1", token: "q-tok" }));
    await act(async () => {
      await new Promise((r) => setTimeout(r, 50));
    });
    expect(result.current.error?.code).toBe("AUTH_REQUIRED");
    expect(result.current.transport).toBe("idle");
    expect(result.current.connection).not.toBe("reconnecting");
  });
});
