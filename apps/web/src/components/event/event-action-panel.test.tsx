import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { EventActionPanel } from "./event-action-panel";

// Mock next/navigation
const mockPush = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: mockPush,
  }),
}));

describe("EventActionPanel (EVF-111, BR-Q5, BR-O2)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("SCHEDULED va chua toi gio mo ban: hien thi countdown va nut disabled", () => {
    // 1 gio nua moi mo ban
    const futureTime = new Date(Date.now() + 3600_000).toISOString();

    render(
      <EventActionPanel
        eventId="evt-1"
        state="SCHEDULED"
        saleStartAt={futureTime}
      />,
    );

    expect(screen.getByText("Thời gian mở bán")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /Chưa tới giờ mở bán/i }),
    ).toBeDisabled();
  });

  it("ON_SALE: nut mo ban duoc kich hoat va bam vao se chay jitter", async () => {
    const onJoinQueue = vi.fn();
    const pastTime = new Date(Date.now() - 3600_000).toISOString();

    render(
      <EventActionPanel
        eventId="evt-1"
        state="ON_SALE"
        saleStartAt={pastTime}
        onJoinQueue={onJoinQueue}
        jitterOptions={{ minJitterMs: 0, maxJitterMs: 0 }}
      />,
    );

    const button = screen.getByRole("button", { name: /Vào phòng chờ mua vé/i });
    expect(button).toBeEnabled();

    fireEvent.click(button);

    await waitFor(() => {
      expect(onJoinQueue).toHaveBeenCalledWith("evt-1");
    });
  });

  it("dieu huong qua router.push khi khong truyen onJoinQueue", async () => {
    const pastTime = new Date(Date.now() - 3600_000).toISOString();

    render(
      <EventActionPanel
        eventId="evt-abc"
        state="ON_SALE"
        saleStartAt={pastTime}
        jitterOptions={{ minJitterMs: 0, maxJitterMs: 0 }}
      />,
    );

    const button = screen.getByRole("button", { name: /Vào phòng chờ mua vé/i });
    fireEvent.click(button);

    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith("/waiting/evt-abc");
    });
  });

  it("SOLD_OUT: nut bi vo hieu hoa voi thong bao het ve", () => {
    render(
      <EventActionPanel
        eventId="evt-1"
        state="SOLD_OUT"
        saleStartAt="2026-10-10T10:00:00+07:00"
      />,
    );

    const button = screen.getByRole("button", { name: /Đã hết vé/i });
    expect(button).toBeDisabled();
  });

  it("CANCELLED: nut bi vo hieu hoa voi thong bao su kien da huy", () => {
    render(
      <EventActionPanel
        eventId="evt-1"
        state="CANCELLED"
        saleStartAt="2026-10-10T10:00:00+07:00"
      />,
    );

    const button = screen.getByRole("button", { name: /Sự kiện đã huỷ/i });
    expect(button).toBeDisabled();
  });

  it("CLOSED: nut bi vo hieu hoa voi thong bao da dong ban", () => {
    render(
      <EventActionPanel
        eventId="evt-1"
        state="CLOSED"
        saleStartAt="2026-10-10T10:00:00+07:00"
      />,
    );

    const button = screen.getByRole("button", { name: /Đã đóng bán/i });
    expect(button).toBeDisabled();
  });
});
