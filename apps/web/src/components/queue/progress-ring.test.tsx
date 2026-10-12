import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { ProgressRing } from "./progress-ring";

describe("ProgressRing (EV-182 AC-5)", () => {
  it("không render nếu thiếu initialRank hoặc initialRank <= 0 (no fake progress)", () => {
    const { container: c1 } = render(<ProgressRing rank={50} initialRank={null} />);
    expect(c1).toBeEmptyDOMElement();

    const { container: c2 } = render(<ProgressRing rank={50} initialRank={0} />);
    expect(c2).toBeEmptyDOMElement();

    const { container: c3 } = render(<ProgressRing rank={50} initialRank={-10} />);
    expect(c3).toBeEmptyDOMElement();
  });

  it("không render nếu rank không hợp lệ (null, âm)", () => {
    const { container: c1 } = render(<ProgressRing rank={null} initialRank={100} />);
    expect(c1).toBeEmptyDOMElement();

    const { container: c2 } = render(<ProgressRing rank={-1} initialRank={100} />);
    expect(c2).toBeEmptyDOMElement();
  });

  it("render progressbar với đầy đủ ARIA attributes", () => {
    render(<ProgressRing rank={40} initialRank={100} />);
    const bar = screen.getByRole("progressbar");
    expect(bar).toBeInTheDocument();
    expect(bar).toHaveAttribute("aria-valuemin", "0");
    expect(bar).toHaveAttribute("aria-valuemax", "100");
    expect(bar).toHaveAttribute("aria-valuenow", "60");
    expect(bar).toHaveAttribute(
      "aria-valuetext",
      "Đã tiến 60 trong 100 người xếp trước bạn (60%)"
    );
    expect(screen.getByText("60%")).toBeInTheDocument();
  });

  it("khi rank = initialRank thì tiến trình là 0%", () => {
    render(<ProgressRing rank={100} initialRank={100} />);
    const bar = screen.getByRole("progressbar");
    expect(bar).toHaveAttribute("aria-valuenow", "0");
    expect(screen.getByText("0%")).toBeInTheDocument();
  });

  it("khi rank = 0 thì tiến trình là 100%", () => {
    render(<ProgressRing rank={0} initialRank={100} />);
    const bar = screen.getByRole("progressbar");
    expect(bar).toHaveAttribute("aria-valuenow", "100");
    expect(screen.getByText("100%")).toBeInTheDocument();
  });

  it("tiến trình không vượt quá 100% nếu rank < 0 (đã kẹp chặn)", () => {
    render(<ProgressRing rank={0} initialRank={50} />);
    expect(screen.getByText("100%")).toBeInTheDocument();
  });
});
