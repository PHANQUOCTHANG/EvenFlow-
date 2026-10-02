/**
 * Test cho Button — AC-3.
 *
 * Hai bug that hay xay ra, nen kiem ky truong hop am:
 *   1. `loading` chi doi hinh thuc ma van cho click -> nguoi dung bam 2 lan =
 *      2 don hang. Test: disabled VA loading deu KHONG goi onClick.
 *   2. prop `className` ghi de class goc -> mat toan bo style variant.
 *   3. Spinner trong nut co aria-label -> nhan do nhap vao accessible name cua nut
 *      ("Dang tai Mua ve" thay vi "Mua ve"). Vi vay test loading query theo NAME
 *      chu khong query role="status" cua spinner.
 *
 * Chieu cao 44/48px (AC-3) khong kiem duoc o jsdom (khong co layout engine)
 * -> xem traceability.md.
 */
import { createRef } from "react";

import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { Button, type ButtonSize, type ButtonVariant } from "./button";

const VARIANTS: ButtonVariant[] = ["primary", "secondary", "tertiary", "destructive"];
const SIZES: ButtonSize[] = ["sm", "md", "lg"];

describe("Button — ma tran variant x size", () => {
  for (const variant of VARIANTS) {
    for (const size of SIZES) {
      it(`render duoc voi variant="${variant}" size="${size}"`, () => {
        render(
          <Button variant={variant} size={size}>
            Mua vé
          </Button>,
        );
        const btn = screen.getByRole("button", { name: "Mua vé" });
        expect(btn).toBeInTheDocument();
        expect(btn).toBeEnabled();
      });
    }
  }

  it("moi variant cho ra class khac nhau (variant thuc su co tac dung)", () => {
    const classes = VARIANTS.map((variant) => {
      const view = render(<Button variant={variant}>Mua vé</Button>);
      const cls = screen.getByRole("button").getAttribute("class") ?? "";
      view.unmount();
      return cls;
    });
    expect(new Set(classes).size).toBe(VARIANTS.length);
  });

  it("moi size cho ra class khac nhau", () => {
    const classes = SIZES.map((size) => {
      const view = render(<Button size={size}>Mua vé</Button>);
      const cls = screen.getByRole("button").getAttribute("class") ?? "";
      view.unmount();
      return cls;
    });
    expect(new Set(classes).size).toBe(SIZES.length);
  });
});

describe("Button — thuoc tinh mac dinh", () => {
  it('mac dinh type="button" (khong vo tinh submit form)', () => {
    render(<Button>Huỷ</Button>);
    expect(screen.getByRole("button", { name: "Huỷ" })).toHaveAttribute("type", "button");
  });

  it("type truyen vao duoc ton trong", () => {
    render(<Button type="submit">Gửi</Button>);
    expect(screen.getByRole("button", { name: "Gửi" })).toHaveAttribute("type", "submit");
  });

  it("khong loading thi KHONG co aria-busy", () => {
    render(<Button>Mua vé</Button>);
    expect(screen.getByRole("button")).not.toHaveAttribute("aria-busy", "true");
  });

  it("khong loading thi khong co spinner", () => {
    const { container } = render(<Button>Mua vé</Button>);
    expect(container.querySelector("svg")).toBeNull();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("forwardRef tro toi phan tu <button>", () => {
    const ref = createRef<HTMLButtonElement>();
    render(<Button ref={ref}>Mua vé</Button>);
    expect(ref.current).toBeInstanceOf(HTMLButtonElement);
    expect(ref.current).toBe(screen.getByRole("button"));
  });

  it("spread rest xuong <button>", () => {
    render(
      <Button data-testid="nut-mua" aria-describedby="mo-ta">
        Mua vé
      </Button>,
    );
    expect(screen.getByTestId("nut-mua")).toHaveAttribute("aria-describedby", "mo-ta");
  });

  it("className truyen vao duoc merge, KHONG ghi de class goc (AC-3)", () => {
    render(<Button className="lop-rieng-cua-toi">Mua vé</Button>);
    const btn = screen.getByRole("button");
    expect(btn).toHaveClass("lop-rieng-cua-toi");
    expect(btn.className.trim().split(/\s+/).length).toBeGreaterThan(1);
  });
});

describe("Button — click", () => {
  it("goi onClick khi nut binh thuong", async () => {
    const user = userEvent.setup();
    const onClick = vi.fn();
    render(<Button onClick={onClick}>Mua vé</Button>);

    await user.click(screen.getByRole("button", { name: "Mua vé" }));

    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it("focus duoc bang ban phim va Enter kich hoat (AC-9)", async () => {
    const user = userEvent.setup();
    const onClick = vi.fn();
    render(<Button onClick={onClick}>Mua vé</Button>);

    await user.tab();
    expect(screen.getByRole("button")).toHaveFocus();

    await user.keyboard("{Enter}");
    expect(onClick).toHaveBeenCalledTimes(1);
  });
});

describe("Button — disabled (truong hop am)", () => {
  it("nut bi disable", () => {
    render(<Button disabled>Mua vé</Button>);
    expect(screen.getByRole("button")).toBeDisabled();
  });

  it("KHONG goi onClick khi disabled", () => {
    const onClick = vi.fn();
    render(
      <Button disabled onClick={onClick}>
        Mua vé
      </Button>,
    );

    fireEvent.click(screen.getByRole("button"));

    expect(onClick).not.toHaveBeenCalled();
  });
});

describe("Button — loading (truong hop am + chong layout shift)", () => {
  it('co aria-busy="true"', () => {
    render(<Button loading>Mua vé</Button>);
    expect(screen.getByRole("button")).toHaveAttribute("aria-busy", "true");
  });

  /**
   * Test quan trong nhat cua ca file. Spinner nam BEN TRONG <button> nen neu no co
   * aria-label thi nhan do nhap vao accessible name cua nut: ten nut thanh
   * "Dang tai Mua ve" thay vi "Mua ve" -> screen reader doc sai, va moi
   * getByRole("button", { name: "Mua vé" }) o cho khac trong app se TRUOT dung luc
   * nut vao loading. Query theo NAME (khong theo role status) moi bat duoc.
   */
  it("accessible name KHONG bi Spinner lam ban khi loading", () => {
    render(<Button loading>Mua vé</Button>);

    const btn = screen.getByRole("button", { name: "Mua vé" });
    expect(btn).toHaveAccessibleName("Mua vé");
    expect(btn).toHaveAttribute("aria-busy", "true");
  });

  it("trang thai ban truyen bang aria-busy tren nut, KHONG bang role cua Spinner", () => {
    const { container } = render(<Button loading>Mua vé</Button>);

    // Spinner van ton tai trong DOM (chi bao thi giac)...
    const spinner = container.querySelector("svg");
    expect(spinner).not.toBeNull();

    // ...nhung la trang tri: khong role, khong aria-label, co aria-hidden.
    expect(spinner).not.toHaveAttribute("role", "status");
    expect(spinner).not.toHaveAttribute("aria-label");
    expect(spinner).toHaveAttribute("aria-hidden", "true");
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("children VAN render khi loading (chong layout shift)", () => {
    render(<Button loading>Mua vé</Button>);
    expect(screen.getByRole("button")).toHaveTextContent("Mua vé");
  });

  it("nut bi disable khi loading", () => {
    render(<Button loading>Mua vé</Button>);
    expect(screen.getByRole("button")).toBeDisabled();
  });

  it("KHONG goi onClick khi loading", () => {
    const onClick = vi.fn();
    render(
      <Button loading onClick={onClick}>
        Mua vé
      </Button>,
    );

    fireEvent.click(screen.getByRole("button"));

    expect(onClick).not.toHaveBeenCalled();
  });

  it("loading cung chan ca khi user bam nhieu lan", () => {
    const onClick = vi.fn();
    render(
      <Button loading onClick={onClick}>
        Mua vé
      </Button>,
    );

    const btn = screen.getByRole("button");
    fireEvent.click(btn);
    fireEvent.click(btn);
    fireEvent.click(btn);

    expect(onClick).not.toHaveBeenCalled();
  });
});
