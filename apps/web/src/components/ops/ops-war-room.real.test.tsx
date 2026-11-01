/**
 * ops-warroom AC-23, AC-24, AC-25 (UI), AC-21 (no healthy indicator): OpsWarRoom on real data, driven through the screen.
 *
 * ASSUMPTIONS (plan WP4):
 *  - The controls live inside the existing region named "Điều khiển" (section aria-labelledby the h2).
 *  - Rate input = role spinbutton named /rate|tốc độ/i; reason inputs = role textbox named /lý do/i (one shared field or
 *    one per action; the tests fill every visible reason field). Buttons: "Ghi đè admit rate", "Tạm dừng khẩn cấp",
 *    "Bỏ ghi đè" / "Tiếp tục tự động" (only when MANUAL), confirmation button named /xác nhận/i.
 *  - Success => element role "status" naming the new state, then getOpsWarRoom is called (immediate refresh).
 *  - Write errors are thrown with a `.code` (INVALID|FORBIDDEN|NOT_FOUND|UNAVAILABLE) as in lib/ops-service OpsWriteError.
 *  - Client validation failure is exposed as role "alert" or aria-invalid on the field, with no service call.
 */
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  clearAdmitOverride,
  getOpsWarRoom,
  setAdmitOverride,
  type OpsWarRoomSnapshot,
} from "@/lib/ops-service";

import { OpsWarRoom } from "./ops-war-room";

vi.mock("@/lib/ops-service", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/ops-service")>();
  return { ...actual, getOpsWarRoom: vi.fn(), setAdmitOverride: vi.fn(), clearAdmitOverride: vi.fn() };
});
const mockGet = vi.mocked(getOpsWarRoom);
const mockSet = vi.mocked(setAdmitOverride);
const mockClear = vi.mocked(clearAdmitOverride);

const EV = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";

function snap(over: Partial<OpsWarRoomSnapshot> = {}, adm: Partial<OpsWarRoomSnapshot["admission"]> = {}): OpsWarRoomSnapshot {
  return {
    eventId: EV, eventTitle: "Rock Fest", saleState: "ON_SALE", generatedAt: new Date().toISOString(),
    funnel: { lobby: 11, queued: 4210, admitted: 350, holding: 5, paid: 33, issued: 30 },
    admission: {
      ratePerSec: 50, mode: "AUTO", queueDepth: 4210, overrideRatePerSec: null, paused: false, soldOut: false,
      stockZero: false, leaderPresent: true, ...adm,
    },
    inventory: [{ tierId: "a", name: "GA", quota: 100, held: 4, paid: 30, issued: 25, available: 41 }],
    ai: { mode: "NORMAL", manualOverride: false, chatQueueLagSeconds: 7, budgetUsedRatio: 0.35 },
    checkout: { p99Ms: 420, errorRate: 0.004, dbPoolUsage: 0.61, pods: 3 },
    sources: { waitingroom: "ok", ticketing: "ok", ai: "ok" },
    isSample: false,
    ...over,
  };
}

const ok = (active: boolean, rate: number | null, audit: "recorded" | "failed" = "recorded") => ({
  active, ratePerSec: rate, paused: active && rate === 0, audit,
});
const writeError = (code: string) => Object.assign(new Error("server-detail-must-not-show"), { code });

function controls() {
  return screen.getByRole("region", { name: "Điều khiển" });
}
function fillReasons(text: string) {
  for (const el of within(controls()).queryAllByRole("textbox", { name: /lý do/i })) fireEvent.change(el, { target: { value: text } });
}
function setRate(value: string) {
  fireEvent.change(within(controls()).getByRole("spinbutton", { name: /rate|tốc độ/i }), { target: { value } });
}
const btn = (name: RegExp | string) => within(controls()).getByRole("button", { name });
const queryBtn = (name: RegExp | string) => within(controls()).queryByRole("button", { name });

function mount(s: OpsWarRoomSnapshot) {
  mockGet.mockResolvedValue(s);
  return render(<OpsWarRoom eventId={EV} initialSnapshot={s} />);
}

beforeEach(() => {
  mockGet.mockReset();
  mockSet.mockReset();
  mockClear.mockReset();
});
afterEach(cleanup);

describe("sample label (AC-23)", () => {
  it("never on the real path, only when isSample is true", () => {
    const { unmount } = mount(snap());
    expect(screen.queryByText("Dữ liệu mẫu")).toBeNull();
    unmount();
    mount(snap({ isSample: true }));
    expect(screen.getByText("Dữ liệu mẫu")).toBeInTheDocument();
  });
});

describe("admission panel states (AC-23)", () => {
  it("MANUAL shows the override rate", () => {
    mount(snap({}, { mode: "MANUAL", overrideRatePerSec: 120, ratePerSec: 120 }));
    expect(screen.getAllByText(/120/).length).toBeGreaterThan(0);
  });
  it("paused => prominent text 'ĐANG TẠM DỪNG'", () => {
    mount(snap({}, { mode: "MANUAL", overrideRatePerSec: 0, ratePerSec: 0, paused: true }));
    expect(screen.getByText(/ĐANG TẠM DỪNG/)).toBeInTheDocument();
  });
  it("not paused => no pause banner", () => {
    mount(snap());
    expect(screen.queryByText(/ĐANG TẠM DỪNG/)).toBeNull();
  });
  it("soldOut => 'Hết vé'", () => {
    mount(snap({}, { soldOut: true }));
    expect(screen.getAllByText(/Hết vé/).length).toBeGreaterThan(0);
  });
  it("no leader => 'Không có leader điều phối'", () => {
    mount(snap({}, { leaderPresent: false }));
    expect(screen.getByText(/Không có leader điều phối/)).toBeInTheDocument();
  });
  it("leader present => no leader warning", () => {
    mount(snap());
    expect(screen.queryByText(/Không có leader điều phối/)).toBeNull();
  });
  it("checkout block shows p99, error rate, pool, pods", () => {
    mount(snap());
    const text = document.body.textContent ?? "";
    expect(text).toMatch(/420/);
    expect(text).toMatch(/0[.,]4\s*%/);
    expect(text).toMatch(/61\s*%/);
  });
  it("checkout null => 'Không có tín hiệu'", () => {
    mount(snap({ checkout: null }));
    expect(screen.getByText(/Không có tín hiệu/)).toBeInTheDocument();
  });
  it("a waitingroom source that is not ok never shows a healthy indicator", () => {
    mount(snap({ sources: { waitingroom: "unavailable", ticketing: "ok", ai: "ok" } }));
    expect(screen.queryByText(/hoạt động bình thường|Bình thường|Tất cả nguồn/i)).toBeNull();
    expect(screen.getAllByText(/Không có dữ liệu/).length).toBeGreaterThan(0);
  });
});

describe("override control (AC-24, AC-25)", () => {
  it("is no longer a disabled stub", () => {
    mount(snap());
    expect(btn("Ghi đè admit rate")).toBeEnabled();
    expect(btn("Tạm dừng khẩn cấp")).toBeEnabled();
  });

  it.each([
    ["", "đủ dài"],
    ["-1", "đủ dài"],
    ["10001", "đủ dài"],
    ["1.5", "đủ dài"],
    ["50", ""],
    ["50", "ab"],
    ["50", "x".repeat(201)],
  ])("invalid input rate=%j reason=%j => no service call", async (rate, reason) => {
    mount(snap());
    setRate(rate);
    fillReasons(reason);
    fireEvent.click(btn("Ghi đè admit rate"));
    await act(async () => {});
    expect(mockSet).not.toHaveBeenCalled();
    const invalid =
      within(controls()).queryAllByRole("alert").length > 0 ||
      within(controls()).getAllByRole("textbox").some((e) => e.getAttribute("aria-invalid") === "true") ||
      within(controls()).getAllByRole("spinbutton").some((e) => e.getAttribute("aria-invalid") === "true");
    expect(invalid).toBe(true);
  });

  it("valid override calls the service, then announces the new state and refreshes", async () => {
    let resolve!: (v: ReturnType<typeof ok>) => void;
    mockSet.mockReturnValue(new Promise((r) => (resolve = r)));
    mount(snap());
    setRate("120");
    fillReasons("checkout p99 cao");
    fireEvent.click(btn("Ghi đè admit rate"));
    await waitFor(() => expect(mockSet).toHaveBeenCalledWith(EV, 120, "checkout p99 cao"));
    // in flight: no optimistic state, buttons locked, no refresh yet
    expect(screen.queryAllByRole("status").some((e) => /120/.test(e.textContent ?? ""))).toBe(false);
    expect(btn("Ghi đè admit rate")).toBeDisabled();
    expect(btn("Tạm dừng khẩn cấp")).toBeDisabled();
    expect(mockGet).not.toHaveBeenCalled();
    await act(async () => resolve(ok(true, 120)));
    await waitFor(() => expect(screen.getAllByRole("status").some((e) => /120/.test(e.textContent ?? ""))).toBe(true));
    await waitFor(() => expect(mockGet).toHaveBeenCalled());
  });
});

describe("emergency pause (AC-24)", () => {
  it("asks for confirmation and a reason, then sends rate 0", async () => {
    mockSet.mockResolvedValue(ok(true, 0));
    mount(snap());
    fireEvent.click(btn("Tạm dừng khẩn cấp"));
    await act(async () => {});
    expect(mockSet).not.toHaveBeenCalled(); // nothing before confirmation
    fillReasons("su co thanh toan");
    fireEvent.click(screen.getByRole("button", { name: /xác nhận/i }));
    await waitFor(() => expect(mockSet).toHaveBeenCalledWith(EV, 0, "su co thanh toan"));
    await waitFor(() => expect(mockGet).toHaveBeenCalled());
  });

  it("pause without a reason is rejected client-side", async () => {
    mount(snap());
    fireEvent.click(btn("Tạm dừng khẩn cấp"));
    fillReasons("");
    const confirm = screen.queryByRole("button", { name: /xác nhận/i });
    if (confirm) fireEvent.click(confirm);
    await act(async () => {});
    expect(mockSet).not.toHaveBeenCalled();
  });
});

describe("clear override (AC-24)", () => {
  it("not offered while AUTO", () => {
    mount(snap());
    expect(queryBtn(/Bỏ ghi đè|Tiếp tục tự động/)).toBeNull();
  });
  it("offered when MANUAL; calls clear with the reason and refreshes", async () => {
    mockClear.mockResolvedValue(ok(false, null));
    mount(snap({}, { mode: "MANUAL", overrideRatePerSec: 0, paused: true, ratePerSec: 0 }));
    fillReasons("su co da xu ly");
    fireEvent.click(btn(/Bỏ ghi đè|Tiếp tục tự động/));
    const confirm = screen.queryByRole("button", { name: /xác nhận/i });
    if (confirm) {
      fillReasons("su co da xu ly");
      fireEvent.click(confirm);
    }
    await waitFor(() => expect(mockClear).toHaveBeenCalledWith(EV, "su co da xu ly"));
    await waitFor(() => expect(mockGet).toHaveBeenCalled());
  });
});

describe("controls availability (AC-24)", () => {
  it("disabled with a visible reason when the waitingroom source is unavailable", () => {
    mount(snap({ sources: { waitingroom: "unavailable", ticketing: "ok", ai: "ok" } }));
    expect(btn("Ghi đè admit rate")).toBeDisabled();
    expect(btn("Tạm dừng khẩn cấp")).toBeDisabled();
    expect(within(controls()).getByText(/phòng chờ|waitingroom|không khả dụng|không có dữ liệu/i)).toBeInTheDocument();
  });
});

describe("write results (AC-25)", () => {
  async function submitOverride() {
    setRate("5");
    fillReasons("ly do hop le");
    fireEvent.click(btn("Ghi đè admit rate"));
    await waitFor(() => expect(mockSet).toHaveBeenCalled());
  }

  it("audit failed => warning that it was applied but not audited", async () => {
    mockSet.mockResolvedValue(ok(true, 5, "failed"));
    mount(snap());
    await submitOverride();
    expect(await screen.findByText(/Đã áp dụng nhưng chưa ghi được nhật ký kiểm toán/)).toBeInTheDocument();
  });

  it.each([
    ["FORBIDDEN", /Không có quyền/],
    ["NOT_FOUND", /Sự kiện chưa có phòng chờ/],
  ])("%s => fixed client text, never the server detail", async (code, text) => {
    mockSet.mockRejectedValue(writeError(code));
    mount(snap());
    await submitOverride();
    expect(await within(controls()).findByText(text)).toBeInTheDocument();
    expect(document.body.textContent).not.toContain("server-detail-must-not-show");
  });

  it("INVALID => a field error, still no server detail", async () => {
    mockSet.mockRejectedValue(writeError("INVALID"));
    mount(snap());
    await submitOverride();
    await act(async () => {});
    expect(document.body.textContent).not.toContain("server-detail-must-not-show");
    expect(within(controls()).getAllByRole("alert").length).toBeGreaterThan(0);
  });

  it("UNAVAILABLE => error with a retry that re-sends, no optimistic change", async () => {
    mockSet.mockRejectedValueOnce(writeError("UNAVAILABLE")).mockResolvedValueOnce(ok(true, 5));
    mount(snap());
    await submitOverride();
    const retry = await within(controls()).findByRole("button", { name: /thử lại/i });
    expect(mockGet).not.toHaveBeenCalled();
    fireEvent.click(retry);
    await waitFor(() => expect(mockSet).toHaveBeenCalledTimes(2));
  });
});
