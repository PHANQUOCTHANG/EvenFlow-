/** ops-warroom fix round 1 (R-5, X-7): an expired session (401 / AUTH_REQUIRED) in the war room, its controls and the
 * organizer sales dashboard shows the sign-in prompt with a link, never "Không có quyền" or a bare retry. */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { SalesDashboard } from "@/components/organizer/sales-dashboard";
import { ApiError } from "@/lib/api-client";
import { OpsServiceError, getOpsWarRoom, setAdmitOverride, type OpsWarRoomSnapshot } from "@/lib/ops-service";
import { getRealtimeSales, type OrganizerEvent } from "@/lib/organizer-service";

import { OpsWarRoom } from "./ops-war-room";

vi.mock("@/lib/ops-service", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/ops-service")>();
  return { ...actual, getOpsWarRoom: vi.fn(), setAdmitOverride: vi.fn() };
});
vi.mock("@/lib/organizer-service", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/organizer-service")>();
  return { ...actual, getRealtimeSales: vi.fn() };
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const EVENT_ID = "evt-1";
const SIGN_IN_TEXT = "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại.";

function snapshot(): OpsWarRoomSnapshot {
  return {
    eventId: EVENT_ID,
    eventTitle: "Rock Fest",
    saleState: "ON_SALE",
    generatedAt: new Date().toISOString(),
    funnel: { lobby: 1, queued: 2, admitted: 3, holding: 0, paid: 0, issued: 0 },
    admission: {
      ratePerSec: 5, mode: "AUTO", queueDepth: 2, overrideRatePerSec: null, paused: false,
      soldOut: false, stockZero: false, leaderPresent: true,
    },
    checkout: null,
    inventory: [],
    ai: { mode: "NORMAL", manualOverride: false, chatQueueLagSeconds: 0, budgetUsedRatio: 0 },
    sources: { waitingroom: "ok", ticketing: "ok", ai: "ok" },
    isSample: false,
  };
}

function expectSignInPrompt() {
  expect(screen.getByText(SIGN_IN_TEXT)).toBeTruthy();
  const link = screen.getByRole("link", { name: "Đăng nhập" });
  expect(link.getAttribute("href")).toBe("/auth/sign-in");
  expect(screen.queryByText("Không có quyền")).toBeNull();
}

describe("war room sign-in prompt on 401", () => {
  it("initial load 401 shows the sign-in prompt", () => {
    vi.mocked(getOpsWarRoom).mockReturnValue(new Promise(() => {}));
    render(<OpsWarRoom eventId={EVENT_ID} initialError={{ status: 401, message: "x" }} />);
    expectSignInPrompt();
  });

  it("a 401 from polling shows the sign-in prompt", async () => {
    vi.mocked(getOpsWarRoom).mockRejectedValue(new OpsServiceError(401));
    render(<OpsWarRoom eventId={EVENT_ID} />);
    await waitFor(() => expect(screen.queryByText(SIGN_IN_TEXT)).not.toBeNull());
    expectSignInPrompt();
  });

  it("an AUTH_REQUIRED write shows the sign-in prompt instead of retry", async () => {
    vi.mocked(getOpsWarRoom).mockReturnValue(new Promise(() => {}));
    vi.mocked(setAdmitOverride).mockRejectedValue(new ApiError("AUTH_REQUIRED", 401, {}, "login"));
    render(<OpsWarRoom eventId={EVENT_ID} initialSnapshot={snapshot()} />);
    fireEvent.change(screen.getByLabelText("Tốc độ admit (khách/giây)"), { target: { value: "5" } });
    fireEvent.change(screen.getByLabelText("Lý do (bắt buộc, 3–200 ký tự)"), { target: { value: "abc" } });
    fireEvent.click(screen.getByRole("button", { name: "Ghi đè admit rate" }));
    await waitFor(() => expect(screen.queryByText(SIGN_IN_TEXT)).not.toBeNull());
    expectSignInPrompt();
    expect(screen.queryByText("Chưa gửi được lệnh")).toBeNull();
  });
});

describe("sales dashboard sign-in prompt on AUTH_REQUIRED", () => {
  it("shows the sign-in prompt with a link", async () => {
    vi.mocked(getRealtimeSales).mockRejectedValue(new ApiError("AUTH_REQUIRED", 401, {}, "x"));
    const event: OrganizerEvent = {
      id: EVENT_ID, slug: "rock", title: "Rock Fest", description: "", venue: "v",
      startsAt: "2026-12-05T08:30:00Z", saleStartAt: "2026-09-01T09:00:00Z", state: "ON_SALE",
      organizerId: "org-1", tiers: [], createdAt: "2026-08-01T00:00:00Z", updatedAt: "2026-09-01T00:00:00Z",
    };
    render(<SalesDashboard event={event} />);
    await waitFor(() => expect(screen.queryByText(SIGN_IN_TEXT)).not.toBeNull());
    expectSignInPrompt();
  });
});
