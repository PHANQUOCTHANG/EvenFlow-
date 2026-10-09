import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import WaitingRoomPage, { generateMetadata } from "./page";

vi.mock("@/components/queue/waiting-room", () => ({
  WaitingRoom: ({ eventId, eventTitle }: { eventId: string; eventTitle?: string }) => (
    <div data-testid="waiting-room">
      <span>Event: {eventId}</span>
      <span>Title: {eventTitle}</span>
    </div>
  ),
}));

vi.mock("@/lib/event-service", () => ({
  getEventById: vi.fn(async (id: string) => {
    if (id === "evt-1") {
      return {
        id: "evt-1",
        title: "EvenFlow Grand Concert",
      };
    }
    return null;
  }),
}));

describe("WaitingRoomPage (EV-182)", () => {
  it("render WaitingRoom component với thông tin sự kiện tìm được", async () => {
    const params = Promise.resolve({ eventId: "evt-1" });
    const Component = await WaitingRoomPage({ params });
    render(Component);

    expect(screen.getByTestId("waiting-room")).toBeInTheDocument();
    expect(screen.getByText("Event: evt-1")).toBeInTheDocument();
    expect(screen.getByText("Title: EvenFlow Grand Concert")).toBeInTheDocument();
  });

  it("generateMetadata tạo title chính xác và noindex", async () => {
    const params = Promise.resolve({ eventId: "evt-1" });
    const meta = await generateMetadata({ params });

    expect(meta.title).toBe("Phòng chờ: EvenFlow Grand Concert | EvenFlow");
    expect(meta.robots).toEqual({ index: false, follow: false });
  });

  it("generateMetadata fallback khi không tìm thấy event", async () => {
    const params = Promise.resolve({ eventId: "unknown-id" });
    const meta = await generateMetadata({ params });

    expect(meta.title).toBe("Phòng chờ mua vé | EvenFlow");
  });
});
