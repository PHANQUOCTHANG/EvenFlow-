import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { installFetch, json, problem, resetEnv, setupEnv } from "@/test-utils/fetch-mock";

import EventsListPage, { revalidate } from "./page";

function wireEvent(id: string, title: string, over: Record<string, unknown> = {}) {
  return {
    id,
    slug: id,
    title,
    description: "",
    venue: "Hà Nội",
    starts_at: "2026-12-01T12:00:00Z",
    sale_start_at: "2026-11-01T03:00:00Z",
    status: "ON_SALE",
    max_per_order: 4,
    tiers: [],
    ...over,
  };
}

beforeEach(setupEnv);
afterEach(() => {
  resetEnv();
  delete process.env.NEXT_PHASE;
});

describe("EventsListPage (EVF-111, AC-17)", () => {
  it("giu ISR revalidate = 60", () => {
    expect(revalidate).toBe(60);
  });

  it("render tieu de chinh va danh sach the su kien tu API", async () => {
    installFetch(() =>
      json({ events: [wireEvent("a", "EvenFlow Grand Concert 2026"), wireEvent("b", "Vietnam Tech Summit 2026")] }),
    );
    const jsx = await EventsListPage();
    render(jsx);

    expect(screen.getByRole("heading", { level: 1, name: /Sự kiện nổi bật/i })).toBeInTheDocument();
    expect(screen.getByText(/EvenFlow Grand Concert 2026/i)).toBeInTheDocument();
    expect(screen.getByText(/Vietnam Tech Summit 2026/i)).toBeInTheDocument();
  });

  it("danh sach rong -> empty state", async () => {
    installFetch(() => json({ events: [] }));
    render(await EventsListPage());
    expect(screen.getByText(/chưa có sự kiện nào/i)).toBeInTheDocument();
  });

  it("backend loi -> error state, KHONG hien du lieu mau", async () => {
    installFetch(() => problem(502, "/problems/upstream-unavailable"));
    render(await EventsListPage());
    expect(screen.getByRole("alert")).toHaveTextContent(/Không tải được danh sách/);
    expect(screen.queryByText(/Grand Concert/i)).not.toBeInTheDocument();
  });

  it("luc next build: KHONG goi backend; vo trang tai danh sach o trinh duyet", async () => {
    process.env.NEXT_PHASE = "phase-production-build";
    const { calls } = installFetch(() => json({ events: [wireEvent("a", "Sự kiện tải ở trình duyệt")] }));
    const jsx = await EventsListPage();
    expect(calls).toHaveLength(0);
    // Trinh duyet khong co NEXT_PHASE (bien khong NEXT_PUBLIC_ khong duoc inline vao bundle client).
    delete process.env.NEXT_PHASE;
    render(jsx);
    expect(screen.getByRole("heading", { level: 1, name: /Sự kiện nổi bật/i })).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText(/Sự kiện tải ở trình duyệt/)).toBeInTheDocument());
    expect(calls).toHaveLength(1);
  });
});
