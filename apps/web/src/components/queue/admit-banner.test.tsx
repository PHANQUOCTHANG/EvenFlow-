import { render, screen, fireEvent } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { AdmitBanner } from "./admit-banner";

describe("AdmitBanner (EV-182 AC-6)", () => {
  it("render vai trò alert để screen reader thông báo ngay", () => {
    render(<AdmitBanner />);
    const alert = screen.getByRole("alert");
    expect(alert).toBeInTheDocument();
    expect(screen.getByText(/Đã đến lượt bạn mua vé!/)).toBeInTheDocument();
  });

  it("gọi onProceed khi người dùng bấm nút", () => {
    const onProceed = vi.fn();
    render(<AdmitBanner onProceed={onProceed} />);

    const btn = screen.getByRole("button", { name: /Tiến hành chọn vé/ });
    fireEvent.click(btn);
    expect(onProceed).toHaveBeenCalledTimes(1);
  });

  it("render link sang checkoutUrl nếu không truyền onProceed", () => {
    render(<AdmitBanner checkoutUrl="/checkout/evt-1" />);

    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "/checkout/evt-1");
    expect(screen.getByRole("button", { name: /Tiến hành chọn vé/ })).toBeInTheDocument();
  });

  it("render đồng hồ đếm ngược suất mua khi có expiresAt", () => {
    render(<AdmitBanner expiresAt={1800000000000} />);
    expect(screen.getAllByText(/Suất mua của bạn còn/)[0]).toBeInTheDocument();
  });
});
