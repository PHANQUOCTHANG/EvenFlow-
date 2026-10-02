/**
 * Test cho Select — AC-5.
 *
 * Diem de lam sai: dung placeholder lam label. Placeholder cua select la
 * <option value="" disabled> — no la gia tri goi y, KHONG thay the <label>.
 */
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { Select, type SelectOption } from "./select";

const OPTIONS: SelectOption[] = [
  { value: "standard", label: "Vé thường" },
  { value: "vip", label: "Vé VIP" },
  { value: "sold", label: "Vé hạng A (hết)", disabled: true },
];

function describedByIds(el: HTMLElement): string[] {
  return (el.getAttribute("aria-describedby") ?? "").split(" ").filter(Boolean);
}

describe("Select — label va option", () => {
  it("label lien ket voi <select>", () => {
    render(<Select label="Loại vé" options={OPTIONS} />);
    const select = screen.getByLabelText("Loại vé");
    expect(select.tagName).toBe("SELECT");
  });

  it("khong truyen id thi id sinh tu dong van khop label", () => {
    const { container } = render(<Select label="Loại vé" options={OPTIONS} />);
    const select = screen.getByLabelText("Loại vé");
    expect(select.id).toBeTruthy();
    expect(container.querySelector("label")).toHaveAttribute("for", select.id);
  });

  it("render day du option tu prop", () => {
    render(<Select label="Loại vé" options={OPTIONS} />);
    const options = screen.getAllByRole("option");
    expect(options).toHaveLength(OPTIONS.length);
    expect(options.map((o) => o.textContent)).toEqual([
      "Vé thường",
      "Vé VIP",
      "Vé hạng A (hết)",
    ]);
  });

  it("option co disabled: true thi bi disable, cac option khac thi khong", () => {
    render(<Select label="Loại vé" options={OPTIONS} />);
    expect(screen.getByRole("option", { name: "Vé hạng A (hết)" })).toBeDisabled();
    expect(screen.getByRole("option", { name: "Vé VIP" })).toBeEnabled();
  });

  it("chon option thi gia tri doi va onChange duoc goi", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Select label="Loại vé" options={OPTIONS} onChange={onChange} />);

    const select = screen.getByLabelText("Loại vé");
    await user.selectOptions(select, "vip");

    expect(select).toHaveValue("vip");
    expect(onChange).toHaveBeenCalled();
  });
});

describe("Select — placeholder (AC-5)", () => {
  it("placeholder la <option value=\"\"> va bi disabled", () => {
    render(<Select label="Loại vé" options={OPTIONS} placeholder="Chọn loại vé" />);
    const ph = screen.getByRole("option", { name: "Chọn loại vé" });

    expect(ph).toBeDisabled();
    expect(ph).toHaveValue("");
  });

  it("placeholder KHONG thay the label: van co <label> va ten doc duoc la label", () => {
    const { container } = render(
      <Select label="Loại vé" options={OPTIONS} placeholder="Chọn loại vé" />,
    );
    const select = screen.getByLabelText("Loại vé");

    expect(container.querySelector("label")).not.toBeNull();
    expect(select).toHaveAccessibleName("Loại vé");
  });

  it("khong truyen placeholder thi chi co dung so option cua prop", () => {
    render(<Select label="Loại vé" options={OPTIONS} />);
    expect(screen.getAllByRole("option")).toHaveLength(OPTIONS.length);
  });
});

describe("Select — error / helpText", () => {
  it("KHONG co error thi KHONG co aria-invalid (truong hop am)", () => {
    render(<Select label="Loại vé" options={OPTIONS} />);
    expect(screen.getByLabelText("Loại vé")).not.toHaveAttribute("aria-invalid");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("co error thi aria-invalid true + describedby tro vao phan tu role=alert", () => {
    render(<Select label="Loại vé" options={OPTIONS} error="Vui lòng chọn loại vé" />);
    const select = screen.getByLabelText("Loại vé");
    const alert = screen.getByRole("alert");

    expect(select).toHaveAttribute("aria-invalid", "true");
    expect(alert).toHaveTextContent("Vui lòng chọn loại vé");
    expect(describedByIds(select)).toContain(alert.id);
  });

  it("chi co helpText: describedby tro vao help, khong co aria-invalid", () => {
    render(<Select label="Loại vé" options={OPTIONS} helpText="Mỗi đơn chỉ chọn một loại" />);
    const select = screen.getByLabelText("Loại vé");
    const help = screen.getByText("Mỗi đơn chỉ chọn một loại");

    expect(describedByIds(select)).toContain(help.id);
    expect(select).not.toHaveAttribute("aria-invalid");
  });
});

describe("Select — trang thai & hop dong chung", () => {
  it("disabled thi select bi disable", () => {
    render(<Select label="Loại vé" options={OPTIONS} disabled />);
    expect(screen.getByLabelText("Loại vé")).toBeDisabled();
  });

  it("focus duoc bang ban phim (AC-9)", async () => {
    const user = userEvent.setup();
    render(<Select label="Loại vé" options={OPTIONS} />);

    await user.tab();

    expect(screen.getByLabelText("Loại vé")).toHaveFocus();
  });

  it("spread rest xuong <select>", () => {
    render(<Select label="Loại vé" options={OPTIONS} name="ticket-type" required />);
    const select = screen.getByLabelText("Loại vé");
    expect(select).toHaveAttribute("name", "ticket-type");
    expect(select).toBeRequired();
  });
});
