/**
 * Test cho Badge — AC-7.
 *
 * Nguyen tac quan trong nhat: badge KHONG BAO GIO truyen tin chi bang mau
 * (handoff §5 nguyen tac 7). Moi badge phai co text doc duoc.
 */
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Badge, type BadgeVariant } from "./badge";

/** 9 trang thai nghiep vu EventFlow + neutral (default theo plan muc 3). */
const VARIANTS: BadgeVariant[] = [
  "lobby",
  "queued",
  "admitted",
  "holding",
  "paid",
  "pending",
  "expired",
  "soldout",
  "failed",
  "neutral",
];

const BUSINESS_VARIANTS: BadgeVariant[] = [
  "lobby",
  "queued",
  "admitted",
  "holding",
  "paid",
  "pending",
  "expired",
  "soldout",
  "failed",
];

describe("Badge", () => {
  for (const variant of VARIANTS) {
    it(`variant "${variant}" render text doc duoc, khong chi mau`, () => {
      render(<Badge variant={variant}>{`Trạng thái ${variant}`}</Badge>);
      const el = screen.getByText(`Trạng thái ${variant}`);
      expect(el).toBeInTheDocument();
      expect(el.textContent?.trim().length ?? 0).toBeGreaterThan(0);
    });
  }

  it("phu du 9 trang thai nghiep vu theo AC-7", () => {
    for (const variant of BUSINESS_VARIANTS) {
      const view = render(<Badge variant={variant}>{variant}</Badge>);
      expect(screen.getByText(variant)).toBeInTheDocument();
      view.unmount();
    }
    expect(BUSINESS_VARIANTS).toHaveLength(9);
  });

  it("render ra <span> (inline, nhung trong dong van ban)", () => {
    render(<Badge>Đã thanh toán</Badge>);
    expect(screen.getByText("Đã thanh toán").tagName).toBe("SPAN");
  });

  it("khong truyen variant thi van render text (mac dinh neutral)", () => {
    render(<Badge>Chưa rõ</Badge>);
    expect(screen.getByText("Chưa rõ")).toBeInTheDocument();
  });

  it("spread rest xuong phan tu goc", () => {
    render(
      <Badge data-testid="badge-trang-thai" title="Đang giữ vé">
        Đang giữ
      </Badge>,
    );
    const el = screen.getByTestId("badge-trang-thai");
    expect(el).toHaveAttribute("title", "Đang giữ vé");
    expect(el).toHaveTextContent("Đang giữ");
  });

  it("className truyen vao duoc merge, khong ghi de class goc", () => {
    render(<Badge className="lop-rieng-cua-toi">Đã vào</Badge>);
    const el = screen.getByText("Đã vào");
    expect(el).toHaveClass("lop-rieng-cua-toi");
    expect(el.className.trim().split(/\s+/).length).toBeGreaterThan(1);
  });
});
