/** ops-warroom fix round 2 (X2-5): the reason error names the cause (control character, bidi, length) and nothing is
 * sent. */
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { getOpsWarRoom, setAdmitOverride, type OpsWarRoomSnapshot } from "@/lib/ops-service";

import { OpsWarRoom } from "./ops-war-room";

vi.mock("@/lib/ops-service", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/ops-service")>();
  return { ...actual, getOpsWarRoom: vi.fn(), setAdmitOverride: vi.fn() };
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function snapshot(): OpsWarRoomSnapshot {
  return {
    eventId: "evt-1",
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

const cp = (...codes: number[]) => String.fromCodePoint(...codes);

describe("reason error names the cause (X2-5)", () => {
  it.each([
    ["control", "ab\tcd", "Lý do không được chứa ký tự điều khiển (tab, xuống dòng, ...)."],
    ["NEL", cp(0x85) + "abc", "Lý do không được chứa ký tự điều khiển (tab, xuống dòng, ...)."],
    ["bidi", "ab" + cp(0x202e) + "c", "Lý do không được chứa ký tự điều khiển hướng văn bản (bidi)."],
    ["invisible", cp(0x2060, 0x2060, 0x2060), "Lý do phải dài từ 3 đến 200 ký tự."],
  ])("%s", (_n, reason, message) => {
    vi.mocked(getOpsWarRoom).mockReturnValue(new Promise(() => {}));
    render(<OpsWarRoom eventId="evt-1" initialSnapshot={snapshot()} />);
    fireEvent.change(screen.getByLabelText("Tốc độ admit (khách/giây)"), { target: { value: "5" } });
    fireEvent.change(screen.getByLabelText("Lý do (bắt buộc, 3–200 ký tự)"), { target: { value: reason } });
    fireEvent.click(screen.getByRole("button", { name: "Ghi đè admit rate" }));
    expect(screen.getByText(message)).toBeTruthy();
    expect(setAdmitOverride).not.toHaveBeenCalled();
  });
});
