import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { TicketTierSnapshot } from "@/lib/event-service";
import { TicketTierList } from "./ticket-tier-list";

describe("TicketTierList (BR-A4, EVF-111)", () => {
  const sampleTiers: TicketTierSnapshot[] = [
    {
      id: "t1",
      name: "Standard Ticket",
      price: 500000,
      description: "Vé phổ thông",
      availability: "AVAILABLE",
    },
    {
      id: "t2",
      name: "VIP Ticket",
      price: 1500000,
      availability: "FEW_LEFT",
    },
    {
      id: "t3",
      name: "VVIP Ticket",
      price: 3000000,
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

  it("render thong bao khi danh sach trong", () => {
    render(<TicketTierList tiers={[]} />);
    expect(
      screen.getByText(/Chưa có thông tin hạng vé cho sự kiện này/i),
    ).toBeInTheDocument();
  });
});
