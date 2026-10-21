/** AC-5 (W-4): addTier succeeded on the server but its response was lost. The retry must match the unsaved tier to the
 *  reloaded server tier (not already referenced, same price, trim + case-insensitive name) instead of POSTing again, and
 *  must not end in a 422 duplicate-name loop. No unique match -> POST as new and show the server's 422 detail. */
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "@/lib/api-client";
import type { EventLifecycleState, OrganizerEvent, TicketTierConfig } from "@/lib/organizer-service";

import EditEventPage from "./page";

const mocks = vi.hoisted(() => ({
  push: vi.fn(),
  getById: vi.fn(),
  update: vi.fn(),
  addTier: vi.fn(),
  updateQuota: vi.fn(),
  submit: vi.fn(),
}));

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: mocks.push }) }));

vi.mock("@/lib/organizer-service", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/organizer-service")>()),
  getOrganizerEventById: mocks.getById,
  updateOrganizerEvent: mocks.update,
  addTier: mocks.addTier,
  updateTierQuota: mocks.updateQuota,
  submitOrganizerEvent: mocks.submit,
}));

vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({
    user: { id: "u-1", role: "organizer" },
    loading: false,
    isAuthenticated: true,
    hasRole: (roles: string[]) => roles.includes("organizer"),
  }),
}));

const EVT = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";

const event = (state: EventLifecycleState = "DRAFT"): OrganizerEvent => ({
  id: EVT,
  slug: "rock-fest",
  title: "Rock Fest",
  description: "Mô tả",
  venue: "Mỹ Đình",
  startsAt: new Date(Date.now() + 10 * 24 * 3600 * 1000).toISOString(),
  saleStartAt: new Date(Date.now() + 5 * 24 * 3600 * 1000).toISOString(),
  state,
  organizerId: "org-1",
  tiers: [{ id: "tier-a", name: "GA", price: 650000, quota: 100 }],
  createdAt: "",
  updatedAt: "",
});

async function renderPage() {
  const ui = await EditEventPage({ params: Promise.resolve({ eventId: EVT }) });
  return render(ui);
}

const next = () => fireEvent.click(screen.getByRole("button", { name: "Tiếp tục →" }));
const back = () => fireEvent.click(screen.getByRole("button", { name: "← Quay lại" }));
const save = () => fireEvent.click(screen.getByRole("button", { name: "Lưu nháp" }));
const addTierButton = () => fireEvent.click(screen.getByRole("button", { name: "+ Thêm hạng vé" }));
const addedNames = () => mocks.addTier.mock.calls.map((c) => (c[1] as TicketTierConfig).name);

/** Server state: the tier whose create response was lost exists server-side from the first addTier call on. */
function lostResponseServer(base: OrganizerEvent, serverName?: (n: string) => string) {
  const state: { onServer: TicketTierConfig | null } = { onServer: null };
  mocks.update.mockResolvedValue(base);
  mocks.addTier.mockImplementationOnce(async (_e: string, t: TicketTierConfig) => {
    state.onServer = { ...t, id: "srv-new-1", name: serverName ? serverName(t.name) : t.name };
    throw new Error("mất mạng");
  });
  mocks.getById.mockImplementation(async () => ({
    ...base,
    tiers: state.onServer ? [...base.tiers, state.onServer] : base.tiers,
  }));
  return state;
}

async function firstSaveFails() {
  await renderPage();
  await screen.findByText("Chỉnh sửa: Rock Fest");
  next();
  addTierButton();
  next();
  next();
  save();
  await screen.findByText(/mất mạng/);
}

beforeEach(() => {
  Object.values(mocks).forEach((m) => m.mockReset());
});

describe("AC-5 lost addTier response", () => {
  it("W-4: retry matches the reloaded server tier: addTier called once in total, no quota PUT, navigates to review-status", async () => {
    const base = event();
    lostResponseServer(base);
    mocks.addTier.mockImplementation(async (_e: string, t: object) => ({ ...t, id: "srv-new-2" }));
    await firstSaveFails();
    save();
    await waitFor(() => expect(mocks.push).toHaveBeenCalledWith(`/organizer/events/${EVT}/review-status`));
    expect(addedNames()).toEqual(["Hạng vé #2"]);
    expect(mocks.updateQuota).not.toHaveBeenCalled();
  });

  it("matched tier with a different quota: only the quota is sent, to the server id; still one addTier", async () => {
    const base = event();
    lostResponseServer(base);
    mocks.addTier.mockImplementation(async (_e: string, t: object) => ({ ...t, id: "srv-new-2" }));
    mocks.updateQuota.mockResolvedValue({});
    await firstSaveFails();
    back();
    back();
    const quotas = screen.getAllByLabelText(/Số lượng vé \(Quota\)/) as HTMLInputElement[];
    fireEvent.change(quotas[1], { target: { value: "300" } });
    next();
    next();
    save();
    await waitFor(() => expect(mocks.push).toHaveBeenCalled());
    expect(addedNames()).toEqual(["Hạng vé #2"]);
    expect(mocks.updateQuota).toHaveBeenCalledWith(EVT, "srv-new-1", 300);
  });

  it("trim + case-insensitive: server stored the name as ' HẠNG VÉ #2 ' -> still matched, no second POST", async () => {
    const base = event();
    lostResponseServer(base, (n) => ` ${n.toUpperCase()} `);
    mocks.addTier.mockImplementation(async (_e: string, t: object) => ({ ...t, id: "srv-new-2" }));
    await firstSaveFails();
    save();
    await waitFor(() => expect(mocks.push).toHaveBeenCalled());
    expect(addedNames()).toEqual(["Hạng vé #2"]);
  });

  it("any 422 from addTier triggers the same lookup: server tier visible only after the failure -> matched, no error shown", async () => {
    const base = event();
    const st = lostResponseServer(base);
    // 2nd save: the first reload (before POST) is stale (replica lag); the POST answers 422 duplicate; reload then shows it.
    let stale = true;
    mocks.getById.mockImplementation(async () => ({
      ...base,
      tiers: st.onServer && !stale ? [...base.tiers, st.onServer] : base.tiers,
    }));
    mocks.addTier.mockImplementation(async () => {
      stale = false;
      throw new ApiError("HTTP_422", 422, { detail: "tên hạng vé đã tồn tại" });
    });
    await firstSaveFails();
    stale = true;
    save();
    await waitFor(() => expect(mocks.push).toHaveBeenCalled());
    expect(screen.queryByText(/đã tồn tại/)).toBeNull();
    expect(mocks.addTier.mock.calls.length).toBeLessThanOrEqual(2);
    expect(mocks.updateQuota).not.toHaveBeenCalled();
  });

  it("an UNRELATED existing tier with the same name but a different price is NOT matched: POST as new, show the server 422 detail", async () => {
    const base = event(); // existing "GA" at 650000; the new tier defaults to another price
    mocks.update.mockResolvedValue(base);
    mocks.getById.mockResolvedValue(base);
    mocks.addTier.mockRejectedValue(new ApiError("HTTP_422", 422, { detail: "tên hạng vé trùng" }));
    await renderPage();
    await screen.findByText("Chỉnh sửa: Rock Fest");
    next();
    addTierButton();
    const names = screen.getAllByLabelText(/Tên hạng vé/) as HTMLInputElement[];
    fireEvent.change(names[1], { target: { value: "ga" } });
    next();
    next();
    save();
    expect(await screen.findByText(/tên hạng vé trùng/)).toBeDefined();
    expect(addedNames()).toEqual(["ga"]);
    expect(mocks.updateQuota).not.toHaveBeenCalled();
    expect(mocks.push).not.toHaveBeenCalled();
  });

  it("GUARD: the organizer renamed the lost tier before retrying: no match, POSTed under the new name, 422 detail shown", async () => {
    const base = event();
    lostResponseServer(base);
    mocks.addTier.mockRejectedValue(new ApiError("HTTP_422", 422, { detail: "tên hạng vé trùng" }));
    await firstSaveFails();
    back();
    back();
    const names = screen.getAllByLabelText(/Tên hạng vé/) as HTMLInputElement[];
    fireEvent.change(names[1], { target: { value: "Hạng mới" } });
    next();
    next();
    save();
    expect(await screen.findByText(/tên hạng vé trùng/)).toBeDefined();
    expect(addedNames()).toEqual(["Hạng vé #2", "Hạng mới"]);
    expect(mocks.push).not.toHaveBeenCalled();
  });

  it("GUARD (F-3): without any lost response a normal save POSTs the new tier exactly once", async () => {
    const base = event();
    mocks.update.mockResolvedValue(base);
    mocks.getById.mockResolvedValue(base);
    mocks.addTier.mockImplementation(async (_e: string, t: object) => ({ ...t, id: "srv-new-1" }));
    await renderPage();
    await screen.findByText("Chỉnh sửa: Rock Fest");
    next();
    addTierButton();
    next();
    next();
    save();
    await waitFor(() => expect(mocks.push).toHaveBeenCalled());
    expect(addedNames()).toEqual(["Hạng vé #2"]);
  });
});
