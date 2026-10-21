import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import WaitingRoomPage, { metadata } from "./page";

// WP2 (AC-18): trang la vo server mong, su kien duoc tai phia client trong WaitingRoomPageClient.
vi.mock("@/components/queue/waiting-room-page-client", () => ({
  WaitingRoomPageClient: ({ eventId }: { eventId: string }) => (
    <div data-testid="waiting-room-client">Event: {eventId}</div>
  ),
}));

describe("WaitingRoomPage (EV-182, AC-18)", () => {
  it("render client component voi eventId tu URL", async () => {
    const Component = await WaitingRoomPage({ params: Promise.resolve({ eventId: "evt-1" }) });
    render(Component);

    expect(screen.getByTestId("waiting-room-client")).toBeInTheDocument();
    expect(screen.getByText("Event: evt-1")).toBeInTheDocument();
  });

  it("metadata tinh co noindex", () => {
    expect(metadata.title).toBe("Phòng chờ mua vé | EvenFlow");
    expect(metadata.robots).toEqual({ index: false, follow: false });
  });
});
