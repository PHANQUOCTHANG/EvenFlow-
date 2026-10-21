import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { installFetch, json, problem, resetEnv, setupEnv } from "@/test-utils/fetch-mock";

import { EventsBrowserList } from "./events-browser-list";

function wireEvent(id: string, title: string) {
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
  };
}

beforeEach(setupEnv);
afterEach(resetEnv);

describe("EventsBrowserList (AC-17, vo trang luc build)", () => {
  it("dang tai -> hien status, sau do hien danh sach", async () => {
    installFetch(() => json({ events: [wireEvent("a", "Đêm nhạc A")] }));
    render(<EventsBrowserList />);
    expect(screen.getByRole("status")).toHaveTextContent(/Đang tải/);
    await waitFor(() => expect(screen.getByText("Đêm nhạc A")).toBeInTheDocument());
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("danh sach rong -> empty state rieng, khong phai loi", async () => {
    installFetch(() => json({ events: [] }));
    render(<EventsBrowserList />);
    await waitFor(() => expect(screen.getByText(/chưa có sự kiện nào/i)).toBeInTheDocument());
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("loi -> error state co nut thu lai; thu lai thanh cong thi hien danh sach", async () => {
    let fail = true;
    const { calls } = installFetch(() =>
      fail ? problem(502, "/problems/upstream-unavailable") : json({ events: [wireEvent("b", "Đêm nhạc B")] }),
    );
    render(<EventsBrowserList />);
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent(/Không tải được danh sách/));
    expect(screen.queryByText(/Đêm nhạc/)).not.toBeInTheDocument();

    fail = false;
    fireEvent.click(screen.getByRole("button", { name: /thử lại/i }));
    await waitFor(() => expect(screen.getByText("Đêm nhạc B")).toBeInTheDocument());
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(calls).toHaveLength(2);
  });
});
