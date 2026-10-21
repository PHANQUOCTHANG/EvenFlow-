import { fireEvent, render, screen } from "@testing-library/react";
import { Component, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installFetch, json, problem, resetEnv, setupEnv } from "@/test-utils/fetch-mock";
import { EventLoader } from "./event-loader";

const NOT_FOUND = "NEXT_NOT_FOUND";

vi.mock("next/navigation", () => ({
  notFound: vi.fn(() => {
    throw new Error("NEXT_NOT_FOUND");
  }),
}));

class Boundary extends Component<{ children: ReactNode }, { error: string | null }> {
  state = { error: null as string | null };
  static getDerivedStateFromError(e: Error) {
    return { error: e.message };
  }
  render() {
    return this.state.error ? <div data-testid="boundary">{this.state.error}</div> : this.props.children;
  }
}

const EVT = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";
const WIRE = {
  id: EVT,
  slug: "s",
  title: "Vietnam Tech Summit 2026",
  description: "",
  venue: "Hà Nội",
  starts_at: "2026-12-05T01:30:00Z",
  sale_start_at: "2026-09-01T02:00:00Z",
  status: "SCHEDULED",
  max_per_order: 4,
  tiers: [],
};

function renderLoader() {
  return render(
    <Boundary>
      <EventLoader eventId={EVT}>{(e) => <p data-testid="ready">{e.title}</p>}</EventLoader>
    </Boundary>,
  );
}

describe("EventLoader (AC-18)", () => {
  beforeEach(() => {
    setupEnv();
    // Lam im loi React in ra khi error boundary bat notFound().
    vi.spyOn(console, "error").mockImplementation(() => undefined);
  });
  afterEach(() => {
    resetEnv();
    vi.restoreAllMocks();
  });

  it("tai su kien theo UUID qua GET /api/v1/events/{id} o trinh duyet roi render con", async () => {
    const { calls } = installFetch(() => json(WIRE));
    renderLoader();

    expect(screen.getByText("Đang tải thông tin sự kiện...")).toBeInTheDocument();
    expect(await screen.findByTestId("ready")).toHaveTextContent("Vietnam Tech Summit 2026");
    expect(calls[0].path).toBe(`/api/v1/events/${EVT}`);
  });

  it("404 -> notFound() cua next/navigation", async () => {
    installFetch(() => problem(404, "/problems/not-found"));
    renderLoader();

    expect(await screen.findByTestId("boundary")).toHaveTextContent(NOT_FOUND);
  });

  it("loi mang/5xx -> trang thai loi co nut thu lai, thu lai thanh cong thi hien su kien", async () => {
    let fail = true;
    installFetch(() => (fail ? problem(503, "/problems/upstream-unavailable") : json(WIRE)));
    renderLoader();

    expect(await screen.findByText("Không tải được sự kiện")).toBeInTheDocument();
    expect(screen.queryByTestId("boundary")).toBeNull();

    fail = false;
    fireEvent.click(screen.getByRole("button", { name: "Thử lại" }));
    expect(await screen.findByTestId("ready")).toBeInTheDocument();
  });
});
