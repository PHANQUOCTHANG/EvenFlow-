import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { TicketTierSnapshot } from "@/lib/types/event";
import { TICKET_TIERS_HEADING_ID, TicketTierList } from "./ticket-tier-list";

describe("TicketTierList (BR-A4, EVF-111)", () => {
  const sampleTiers: TicketTierSnapshot[] = [
    {
      id: "t1",
      name: "Standard Ticket",
      price: 500000,
      currency: "VND",
      availability: "AVAILABLE",
    },
    {
      id: "t2",
      name: "VIP Ticket",
      price: 1500000,
      currency: "VND",
      availability: "FEW_LEFT",
    },
    {
      id: "t3",
      name: "VVIP Ticket",
      price: 3000000,
      currency: "VND",
      availability: "SOLD_OUT",
    },
  ];

  it("render danh sach hang ve voi day du ten va gia tien da format", () => {
    render(<TicketTierList tiers={sampleTiers} />);

    expect(screen.getByText("Standard Ticket")).toBeInTheDocument();
    expect(screen.getByText("VIP Ticket")).toBeInTheDocument();
    expect(screen.getByText("VVIP Ticket")).toBeInTheDocument();

    expect(screen.getByText("500.000 đ")).toBeInTheDocument();
    expect(screen.getByText("1.500.000 đ")).toBeInTheDocument();
    expect(screen.getByText("3.000.000 đ")).toBeInTheDocument();
  });

  it("tuan thu BR-A4: hien thi nhan khoang va khong de lo so luong quota", () => {
    render(<TicketTierList tiers={sampleTiers} />);

    expect(screen.getByText("Còn vé")).toBeInTheDocument();
    expect(screen.getByText("Sắp hết vé")).toBeInTheDocument();
    expect(screen.getByText("Hết vé")).toBeInTheDocument();

    // Khong co so luong con lai
    expect(screen.queryByText(/còn \d+/i)).not.toBeInTheDocument();
  });

  it("availability UNKNOWN: hien nhan trung tinh, khong khang dinh con/het ve", () => {
    render(
      <TicketTierList
        tiers={[{ id: "t9", name: "Hang chua ro", price: 100000, currency: "VND", availability: "UNKNOWN" }]}
      />,
    );
    expect(screen.getByText("Chưa rõ tình trạng")).toBeInTheDocument();
    expect(screen.queryByText("Còn vé")).not.toBeInTheDocument();
    expect(screen.queryByText("Hết vé")).not.toBeInTheDocument();
  });

  it("render thong bao khi danh sach trong", () => {
    render(<TicketTierList tiers={[]} />);
    expect(
      screen.getByText(/Chưa có thông tin hạng vé cho sự kiện này/i),
    ).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// EV-188 AC-3 (axe incomplete `aria-valid-attr-value`): trang chi tiet su kien dat
// <section aria-labelledby="ticket-tiers-heading">; id do phai nam tren heading cua danh sach.
// ---------------------------------------------------------------------------
describe("TicketTierList — heading lam ten cho section (EV-188 I1)", () => {
  const tiers: TicketTierSnapshot[] = [
    { id: "t1", name: "Standard Ticket", price: 500000, currency: "VND", availability: "AVAILABLE" },
  ];

  it('co hang ve: heading cap 2 "Các hạng vé" mang id="ticket-tiers-heading"', () => {
    render(<TicketTierList tiers={tiers} />);
    const heading = screen.getByRole("heading", { level: 2, name: "Các hạng vé" });
    expect(heading).toHaveAttribute("id", "ticket-tiers-heading");
  });

  it("khong co hang ve: khong render id ticket-tiers-heading (section khong duoc tro toi no)", () => {
    const { container } = render(<TicketTierList tiers={[]} />);
    expect(container.querySelector("#ticket-tiers-heading")).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// EV-188 code review m3 (plan §7): TicketTierList nhan `headingId` (mac dinh
// TICKET_TIERS_HEADING_ID = "ticket-tiers-heading"); trang [slug] dung chung hang so.
// ---------------------------------------------------------------------------
describe("TicketTierList — prop headingId (EV-188 m3)", () => {
  const tiers: TicketTierSnapshot[] = [
    { id: "t1", name: "Standard Ticket", price: 500000, currency: "VND", availability: "AVAILABLE" },
  ];

  it('hang so TICKET_TIERS_HEADING_ID = "ticket-tiers-heading" va la gia tri mac dinh', () => {
    expect(TICKET_TIERS_HEADING_ID).toBe("ticket-tiers-heading");
    render(<TicketTierList tiers={tiers} />);
    expect(screen.getByRole("heading", { level: 2 })).toHaveAttribute("id", TICKET_TIERS_HEADING_ID);
  });

  it("headingId tuy chinh duoc dat len heading, khong con id mac dinh", () => {
    const { container } = render(<TicketTierList tiers={tiers} headingId="hang-ve-rieng" />);
    expect(screen.getByRole("heading", { level: 2, name: "Các hạng vé" })).toHaveAttribute("id", "hang-ve-rieng");
    expect(container.querySelector(`#${TICKET_TIERS_HEADING_ID}`)).toBeNull();
  });

  it("khong co hang ve: khong render id tuy chinh", () => {
    const { container } = render(<TicketTierList tiers={[]} headingId="hang-ve-rieng" />);
    expect(container.querySelector("#hang-ve-rieng")).toBeNull();
  });
});
