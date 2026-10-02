/**
 * Test cho Spinner — AC-8.
 *
 * Spinner dung DOC LAP phai co role="status" + ten doc duoc, khong co thi screen
 * reader im lang.
 *
 * Nhung khi Spinner nam BEN TRONG mot control da tu thong bao trang thai (Button
 * loading co aria-busy), aria-label cua spinner se NHAP vao accessible name cua
 * control do -> ten nut thanh "Dang tai Mua ve". Vi vay co prop `decorative`:
 * khong role, khong aria-label, aria-hidden="true".
 */
import { readFileSync, readdirSync, statSync } from "node:fs";
import path from "node:path";

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Spinner, type SpinnerSize } from "./spinner";

const SIZES: SpinnerSize[] = ["sm", "md", "lg"];

describe("Spinner", () => {
  it('co role="status" de screen reader thong bao trang thai', () => {
    render(<Spinner />);
    expect(screen.getByRole("status")).toBeInTheDocument();
  });

  it("mac dinh KHONG phai decorative: co role va co nhan", () => {
    const { container } = render(<Spinner />);
    const svg = container.querySelector("svg");

    expect(svg).toHaveAttribute("role", "status");
    expect(svg).toHaveAttribute("aria-label", "Đang tải");
    expect(svg).not.toHaveAttribute("aria-hidden", "true");
  });

  it('aria-label mac dinh la "Đang tải"', () => {
    render(<Spinner />);
    expect(screen.getByRole("status")).toHaveAccessibleName("Đang tải");
  });

  it("label truyen vao ghi de duoc nhan mac dinh", () => {
    render(<Spinner label="Đang xử lý thanh toán" />);
    expect(screen.getByRole("status")).toHaveAccessibleName("Đang xử lý thanh toán");
    expect(screen.queryByLabelText("Đang tải")).not.toBeInTheDocument();
  });

  for (const size of SIZES) {
    it(`render duoc voi size="${size}"`, () => {
      render(<Spinner size={size} />);
      expect(screen.getByRole("status")).toBeInTheDocument();
    });
  }

  it("ba size cho ra ba ket qua khac nhau (size thuc su co tac dung)", () => {
    const classes = SIZES.map((size) => {
      const view = render(<Spinner size={size} />);
      const cls = screen.getByRole("status").getAttribute("class") ?? "";
      view.unmount();
      return cls;
    });

    expect(new Set(classes).size).toBe(SIZES.length);
  });

  it("spread rest xuong phan tu goc", () => {
    render(<Spinner data-testid="spinner-cho" />);
    expect(screen.getByTestId("spinner-cho")).toBe(screen.getByRole("status"));
  });

  it("co khai bao prefers-reduced-motion trong source de tat animation", () => {
    const srcRoot = path.resolve(process.cwd(), "src");

    const files: string[] = [];
    const walk = (dir: string): void => {
      for (const entry of readdirSync(dir)) {
        const full = path.join(dir, entry);
        if (statSync(full).isDirectory()) walk(full);
        else if (/\.(css|ts|tsx)$/.test(entry)) files.push(full);
      }
    };
    walk(srcRoot);

    const hits = files.filter((f) => readFileSync(f, "utf8").includes("prefers-reduced-motion"));
    expect(hits.length).toBeGreaterThan(0);
  });
});

describe("Spinner — decorative (chong lam ban accessible name)", () => {
  it("decorative thi KHONG co role=status", () => {
    render(<Spinner decorative />);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it('decorative thi co aria-hidden="true" va KHONG co aria-label', () => {
    const { container } = render(<Spinner decorative />);
    const svg = container.querySelector("svg");

    expect(svg).not.toBeNull();
    expect(svg).toHaveAttribute("aria-hidden", "true");
    expect(svg).not.toHaveAttribute("aria-label");
  });

  it("decorative van render svg (chi bao thi giac khong mat)", () => {
    const { container } = render(<Spinner decorative />);
    expect(container.querySelector("svg")).not.toBeNull();
  });

  it("decorative bo qua ca label truyen vao (khong de lot aria-label)", () => {
    const { container } = render(<Spinner decorative label="Đang tải" />);

    expect(container.querySelector("svg")).not.toHaveAttribute("aria-label");
    expect(screen.queryByLabelText("Đang tải")).not.toBeInTheDocument();
  });

  it("decorative van nhan prop size", () => {
    const classes = SIZES.map((size) => {
      const view = render(<Spinner decorative size={size} />);
      const cls = view.container.querySelector("svg")?.getAttribute("class") ?? "";
      view.unmount();
      return cls;
    });

    expect(new Set(classes).size).toBe(SIZES.length);
  });

  /**
   * Ly do ton tai cua prop `decorative`. Day la bug that da xay ra o Button loading:
   * accessible name cua nut bi spinner noi them vao.
   */
  it("trong <button>: decorative giu nguyen accessible name cua nut", () => {
    render(
      <button type="button">
        Mua vé
        <Spinner decorative />
      </button>,
    );

    expect(screen.getByRole("button")).toHaveAccessibleName("Mua vé");
    expect(screen.getByRole("button", { name: "Mua vé" })).toBeInTheDocument();
  });

  it("doi chung: KHONG decorative thi accessible name cua nut bi lam ban", () => {
    render(
      <button type="button">
        Mua vé
        <Spinner />
      </button>,
    );

    const btn = screen.getByRole("button");
    expect(btn).not.toHaveAccessibleName("Mua vé");
    expect(btn.textContent).toContain("Mua vé");
  });
});
