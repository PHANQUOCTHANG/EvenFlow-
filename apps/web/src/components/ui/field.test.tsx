/**
 * Test cho lop field dung chung — AC-4 / AC-5.
 *
 * useFieldIds + describedBy la logic aria duy nhat dung cho ca Input, Select,
 * Checkbox, Radio. Sai o day thi ca 4 component sai theo, nen test rieng tai goc.
 */
import { render, renderHook, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import {
  FieldError,
  FieldHelp,
  FieldLabel,
  describedBy,
  useFieldIds,
  type FieldIds,
} from "./field";

describe("useFieldIds", () => {
  it("dung id nguoi dung truyen vao lam inputId", () => {
    const { result } = renderHook(() => useFieldIds("email-nguoi-mua"));
    expect(result.current.inputId).toBe("email-nguoi-mua");
  });

  it("sinh id khi khong truyen, ca ba id deu khong rong", () => {
    const { result } = renderHook(() => useFieldIds());
    expect(result.current.inputId).toBeTruthy();
    expect(result.current.helpId).toBeTruthy();
    expect(result.current.errorId).toBeTruthy();
  });

  it("ba id trong cung mot field phai khac nhau", () => {
    const { result } = renderHook(() => useFieldIds("ma-otp"));
    const { inputId, helpId, errorId } = result.current;
    expect(new Set([inputId, helpId, errorId]).size).toBe(3);
  });

  it("hai field khac nhau thi inputId khac nhau (khong trung tren cung trang)", () => {
    const a = renderHook(() => useFieldIds());
    const b = renderHook(() => useFieldIds());
    expect(a.result.current.inputId).not.toBe(b.result.current.inputId);
  });

  it("id on dinh qua nhieu lan render (khong doi sau moi re-render)", () => {
    const { result, rerender } = renderHook(() => useFieldIds());
    const first = result.current.inputId;
    rerender();
    expect(result.current.inputId).toBe(first);
  });
});

describe("describedBy", () => {
  const ids: FieldIds = { inputId: "f", helpId: "f-help", errorId: "f-error" };

  it("khong co help va khong co error -> undefined", () => {
    expect(describedBy(ids, { hasHelp: false, hasError: false })).toBeUndefined();
  });

  it("chi co help -> tro toi help, KHONG chua errorId", () => {
    const out = describedBy(ids, { hasHelp: true, hasError: false });
    expect(out?.split(" ")).toContain("f-help");
    expect(out?.split(" ")).not.toContain("f-error");
  });

  it("chi co error -> tro toi error, KHONG chua helpId", () => {
    const out = describedBy(ids, { hasHelp: false, hasError: true });
    expect(out?.split(" ")).toContain("f-error");
    expect(out?.split(" ")).not.toContain("f-help");
  });

  it("co ca hai -> chua ca hai id", () => {
    const out = describedBy(ids, { hasHelp: true, hasError: true });
    expect(out?.split(" ")).toContain("f-help");
    expect(out?.split(" ")).toContain("f-error");
  });
});

describe("FieldLabel", () => {
  it("render <label> voi htmlFor khop input id", () => {
    const { container } = render(<FieldLabel htmlFor="email">Email</FieldLabel>);
    const label = container.querySelector("label");
    expect(label).not.toBeNull();
    expect(label).toHaveAttribute("for", "email");
    expect(label).toHaveTextContent("Email");
  });

  it("required them dau hieu bat buoc nhung van giu nguyen nhan", () => {
    // Hop dong da chot (plan.md §3): dau * la <span aria-hidden="true">*</span> dat NGOAI
    // <label>, la sibling cua no. Neu * nam TRONG <label> thi label text thanh "Email *"
    // va getByLabelText("Email") tren field required se truot.
    const plain = render(<FieldLabel htmlFor="email">Email</FieldLabel>);
    expect(plain.container.querySelector("label")?.textContent).toBe("Email");
    expect(plain.container.querySelector('[aria-hidden="true"]')).toBeNull();
    plain.unmount();

    const req = render(
      <FieldLabel htmlFor="email" required>
        Email
      </FieldLabel>,
    );

    // textContent cua <label> PHAI dung bang ten field, khong kem dau *.
    expect(req.container.querySelector("label")?.textContent).toBe("Email");

    // Dau * ton tai, aria-hidden, va la sibling ngay sau <label>.
    const marker = req.container.querySelector('label + [aria-hidden="true"]');
    expect(marker).not.toBeNull();
    expect(marker).toHaveTextContent("*");
  });

  it("required van tim duoc input qua nhan khong kem dau * (ly do cua hop dong)", () => {
    render(
      <>
        <FieldLabel htmlFor="email" required>
          Email
        </FieldLabel>
        <input id="email" />
      </>,
    );

    expect(screen.getByLabelText("Email")).toHaveAccessibleName("Email");
  });
});

describe("FieldHelp", () => {
  it("render text help voi dung id de aria-describedby tro vao", () => {
    render(<FieldHelp id="email-help">Dùng email nhận vé điện tử</FieldHelp>);
    const help = screen.getByText("Dùng email nhận vé điện tử");
    expect(help).toHaveAttribute("id", "email-help");
  });

  it("KHONG co role=alert (help khong phai loi)", () => {
    render(<FieldHelp id="email-help">Dùng email nhận vé điện tử</FieldHelp>);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});

describe("FieldError", () => {
  it('co role="alert" de screen reader doc ngay khi loi xuat hien', () => {
    render(<FieldError id="email-error">Email không hợp lệ</FieldError>);
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Email không hợp lệ");
  });

  it("render dung id de aria-describedby tro vao", () => {
    render(<FieldError id="email-error">Email không hợp lệ</FieldError>);
    expect(screen.getByRole("alert")).toHaveAttribute("id", "email-error");
  });
});
