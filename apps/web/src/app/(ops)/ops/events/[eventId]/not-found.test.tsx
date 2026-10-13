/**
 * Test cho not-found của war room (EV-187, plan §6 M-2): "Không tìm thấy sự kiện" + link về /ops/events.
 */
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import OpsEventNotFound from "./not-found";

describe("OpsEventNotFound (/ops/events/[eventId] not-found)", () => {
  it("hiện 'Không tìm thấy sự kiện'", () => {
    render(<OpsEventNotFound />);
    expect(screen.getAllByText(/Không tìm thấy sự kiện/i).length).toBeGreaterThan(0);
  });

  it("có link quay về /ops/events", () => {
    const { container } = render(<OpsEventNotFound />);
    expect(container.querySelector('a[href="/ops/events"]')).not.toBeNull();
  });
});
