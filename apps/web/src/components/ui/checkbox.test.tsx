/**
 * Test cho Checkbox — AC-5.
 *
 * `indeterminate` khong phai attribute HTML: phai set qua ref (.indeterminate).
 * Neu ai do viet <input indeterminate> thi React in warning va trang thai khong
 * bao gio hien -> nen o day kiem CA hai: property dung VA attribute khong ton tai.
 *
 * Vi component tu dung ref noi bo cho `indeterminate`, no phai HOP NHAT voi ref
 * ben ngoai truyen vao. Lam sai thi ref ngoai im lang bi bo qua (form library
 * mat input) hoac `indeterminate` ngung hoat dong -> test ca hai huong.
 */
import { createRef } from "react";

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { Checkbox } from "./checkbox";

const LABEL = "Tôi đồng ý điều khoản";

function describedByIds(el: HTMLElement): string[] {
  return (el.getAttribute("aria-describedby") ?? "").split(" ").filter(Boolean);
}

function box(): HTMLInputElement {
  return screen.getByLabelText(LABEL) as HTMLInputElement;
}

describe("Checkbox — label", () => {
  it("label lien ket voi input type=checkbox", () => {
    render(<Checkbox label={LABEL} />);
    expect(box()).toHaveAttribute("type", "checkbox");
  });

  it("khong truyen id thi id sinh tu dong, label van tim duoc input", () => {
    render(<Checkbox label={LABEL} />);
    expect(box().id).toBeTruthy();
  });

  it("bam vao CHU cua label cung tich duoc (vung bam du lon)", async () => {
    const user = userEvent.setup();
    render(<Checkbox label={LABEL} />);

    expect(box()).not.toBeChecked();
    await user.click(screen.getByText(LABEL));
    expect(box()).toBeChecked();
  });

  it("bam truc tiep vao o tich cung doi trang thai", async () => {
    const user = userEvent.setup();
    render(<Checkbox label={LABEL} />);

    await user.click(box());
    expect(box()).toBeChecked();
  });
});

describe("Checkbox — checked / unchecked / indeterminate", () => {
  it("checked=true thi o tich dang duoc tich", () => {
    render(<Checkbox label={LABEL} checked onChange={() => {}} />);
    expect(box()).toBeChecked();
  });

  it("checked=false thi o tich chua duoc tich", () => {
    render(<Checkbox label={LABEL} checked={false} onChange={() => {}} />);
    expect(box()).not.toBeChecked();
  });

  it("indeterminate=true duoc set qua property .indeterminate", () => {
    render(<Checkbox label={LABEL} indeterminate />);
    expect(box().indeterminate).toBe(true);
  });

  it("indeterminate KHONG duoc render thanh attribute HTML", () => {
    render(<Checkbox label={LABEL} indeterminate />);
    expect(box()).not.toHaveAttribute("indeterminate");
  });

  it("indeterminate=false thi property la false", () => {
    render(<Checkbox label={LABEL} indeterminate={false} />);
    expect(box().indeterminate).toBe(false);
  });

  it("khong truyen indeterminate thi property la false", () => {
    render(<Checkbox label={LABEL} />);
    expect(box().indeterminate).toBe(false);
  });
});

describe("Checkbox — forwardRef va hop nhat ref (plan.md §3)", () => {
  it("object ref tro toi dung phan tu <input>", () => {
    const ref = createRef<HTMLInputElement>();
    render(<Checkbox ref={ref} label={LABEL} />);

    expect(ref.current).toBeInstanceOf(HTMLInputElement);
    expect(ref.current).toBe(box());
  });

  it("function ref duoc goi voi dung node input", () => {
    const calls: Array<HTMLInputElement | null> = [];
    render(
      <Checkbox
        ref={(node) => {
          calls.push(node);
        }}
        label={LABEL}
      />,
    );

    expect(calls.length).toBeGreaterThan(0);
    expect(calls[0]).toBe(box());
  });

  it("indeterminate VAN hoat dong khi co ref ngoai truyen vao (ref duoc hop nhat)", () => {
    const ref = createRef<HTMLInputElement>();
    render(<Checkbox ref={ref} label={LABEL} indeterminate />);

    // Ca hai phai dung cung mot luc: ref ngoai KHONG bi ref noi bo lan at,
    // va ref noi bo KHONG bi ref ngoai lan at.
    expect(ref.current).toBe(box());
    expect(box().indeterminate).toBe(true);
    expect(ref.current?.indeterminate).toBe(true);
  });
});

describe("Checkbox — error / helpText", () => {
  it("KHONG co error thi KHONG co aria-invalid (truong hop am)", () => {
    render(<Checkbox label={LABEL} />);
    expect(box()).not.toHaveAttribute("aria-invalid");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("co error thi aria-invalid true + describedby tro vao phan tu role=alert", () => {
    render(<Checkbox label={LABEL} error="Bạn phải đồng ý để tiếp tục" />);
    const alert = screen.getByRole("alert");

    expect(box()).toHaveAttribute("aria-invalid", "true");
    expect(alert).toHaveTextContent("Bạn phải đồng ý để tiếp tục");
    expect(describedByIds(box())).toContain(alert.id);
  });

  it("chi co helpText: describedby tro vao help, khong co aria-invalid", () => {
    render(<Checkbox label={LABEL} helpText="Xem điều khoản ở chân trang" />);
    const help = screen.getByText("Xem điều khoản ở chân trang");

    expect(describedByIds(box())).toContain(help.id);
    expect(box()).not.toHaveAttribute("aria-invalid");
  });
});

describe("Checkbox — disabled & hop dong chung", () => {
  it("disabled thi o tich bi disable", () => {
    render(<Checkbox label={LABEL} disabled />);
    expect(box()).toBeDisabled();
  });

  it("disabled thi bam khong doi trang thai va onChange khong duoc goi", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Checkbox label={LABEL} disabled onChange={onChange} />);

    await user.click(box());

    expect(box()).not.toBeChecked();
    expect(onChange).not.toHaveBeenCalled();
  });

  it("focus duoc bang ban phim (AC-9)", async () => {
    const user = userEvent.setup();
    render(<Checkbox label={LABEL} />);

    await user.tab();

    expect(box()).toHaveFocus();
  });

  it("spread rest xuong <input>", () => {
    render(<Checkbox label={LABEL} name="dong-y" value="yes" />);
    expect(box()).toHaveAttribute("name", "dong-y");
    expect(box()).toHaveAttribute("value", "yes");
  });
});
