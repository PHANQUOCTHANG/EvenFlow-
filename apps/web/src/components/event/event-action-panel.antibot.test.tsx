import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { EventActionPanel } from "./event-action-panel";

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));

describe("EventActionPanel collects anti-bot signals (EV-173, AC-1)", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    window.sessionStorage.clear();
  });

  it("starts a passive collector on mount", () => {
    const spy = vi.spyOn(window, "addEventListener");
    render(<EventActionPanel eventId="evt-1" state="ON_SALE" saleStartAt={new Date(Date.now() - 3600_000).toISOString()} />);
    const passive = spy.mock.calls.filter(([, , o]) => typeof o === "object" && (o as AddEventListenerOptions).passive);
    expect(passive.length).toBeGreaterThan(0);
  });

  it("the join click writes the v1 snapshot BEFORE the jitter delay", async () => {
    vi.useFakeTimers();
    const onJoinQueue = vi.fn();
    render(
      <EventActionPanel
        eventId="evt-1"
        state="ON_SALE"
        saleStartAt={new Date(Date.now() - 3600_000).toISOString()}
        onJoinQueue={onJoinQueue}
        jitterOptions={{ minJitterMs: 3000, maxJitterMs: 3000 }}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /Vào phòng chờ mua vé/i }));
    // jitter not elapsed yet: no navigation, but the snapshot is already stored
    expect(onJoinQueue).not.toHaveBeenCalled();
    const raw = window.sessionStorage.getItem("ef:ab:evt-1");
    expect(raw).not.toBeNull();
    const snap = JSON.parse(raw as string) as Record<string, number>;
    expect(snap.v).toBe(1);
    expect(snap.pe).toBeGreaterThanOrEqual(1);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(3500);
    });
    expect(onJoinQueue).toHaveBeenCalledWith("evt-1");
  });
});
