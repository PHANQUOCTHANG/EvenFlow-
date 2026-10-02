/**
 * Test cho Input — AC-4.
 *
 * Nguyen tac handoff §6: label LUON hien, khong bao gio dung placeholder thay label
 * (placeholder mat khi user bat dau nhap -> mat ngu canh, fail WCAG 3.3.2).
 *
 * Truong hop am quan trong: khong co `error` thi TUYET DOI khong duoc co
 * aria-invalid — neu co, screen reader bao loi ca khi form con trang.
 */
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { Input } from "./input";

function describedByIds(el: HTMLElement): string[] {
  return (el.getAttribute("aria-describedby") ?? "").split(" ").filter(Boolean);
}

describe("Input — label luon hien va lien ket (AC-4)", () => {
  it("getByLabelText tim duoc input qua nhan", () => {
    render(<Input label="Email" />);
    const input = screen.getByLabelText("Email");
    expect(input.tagName).toBe("INPUT");
  });

  it("id truyen vao thi label[for] khop dung id do", () => {
    const { container } = render(<Input label="Email" id="email-nguoi-mua" />);
    const label = container.querySelector("label");
    expect(label).toHaveAttribute("for", "email-nguoi-mua");
    expect(screen.getByLabelText("Email")).toHaveAttribute("id", "email-nguoi-mua");
  });

  it("khong truyen id thi id sinh tu dong van khop label", () => {
    const { container } = render(<Input label="Email" />);
    const input = screen.getByLabelText("Email");
    const label = container.querySelector("label");

    expect(input.id).toBeTruthy();
    expect(label).toHaveAttribute("for", input.id);
  });

  it("KHONG dung placeholder thay label: van co <label> va ten doc duoc la label", () => {
    const { container } = render(<Input label="Email" placeholder="ban@vi-du.com" />);
    const input = screen.getByLabelText("Email");

    expect(container.querySelector("label")).not.toBeNull();
    expect(input).toHaveAccessibleName("Email");
    expect(input).toHaveAttribute("placeholder", "ban@vi-du.com");
  });
});

describe("Input — error / helpText (AC-4)", () => {
  it("KHONG co error thi KHONG co aria-invalid (truong hop am)", () => {
    render(<Input label="Email" />);
    expect(screen.getByLabelText("Email")).not.toHaveAttribute("aria-invalid");
  });

  it("KHONG co error va KHONG co help thi khong co aria-describedby", () => {
    render(<Input label="Email" />);
    expect(screen.getByLabelText("Email")).not.toHaveAttribute("aria-describedby");
  });

  it("co error thi aria-invalid bang true", () => {
    render(<Input label="Email" error="Email không hợp lệ" />);
    expect(screen.getByLabelText("Email")).toHaveAttribute("aria-invalid", "true");
  });

  it("co error thi aria-describedby tro toi phan tu loi co role alert", () => {
    render(<Input label="Email" error="Email không hợp lệ" />);
    const input = screen.getByLabelText("Email");
    const alert = screen.getByRole("alert");

    expect(alert).toHaveTextContent("Email không hợp lệ");
    expect(describedByIds(input)).toContain(alert.id);
  });

  it("chi co helpText: describedby tro toi help va KHONG co aria-invalid", () => {
    render(<Input label="Email" helpText="Dùng để nhận vé điện tử" />);
    const input = screen.getByLabelText("Email");
    const help = screen.getByText("Dùng để nhận vé điện tử");

    expect(describedByIds(input)).toContain(help.id);
    expect(input).not.toHaveAttribute("aria-invalid");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("co ca help va error: describedby chua ca hai id", () => {
    render(<Input label="Email" helpText="Dùng để nhận vé điện tử" error="Email không hợp lệ" />);
    const input = screen.getByLabelText("Email");
    const help = screen.getByText("Dùng để nhận vé điện tử");
    const alert = screen.getByRole("alert");

    const ids = describedByIds(input);
    expect(ids).toContain(help.id);
    expect(ids).toContain(alert.id);
  });
});

describe("Input — type", () => {
  it("mac dinh type la text", () => {
    render(<Input label="Họ tên" />);
    expect(screen.getByLabelText("Họ tên")).toHaveAttribute("type", "text");
  });

  it("type email xuong DOM dung", () => {
    render(<Input label="Email" type="email" />);
    expect(screen.getByLabelText("Email")).toHaveAttribute("type", "email");
  });

  it("type otp: inputMode numeric + autoComplete one-time-code + maxLength mac dinh 6", () => {
    render(<Input label="Mã OTP" type="otp" />);
    const input = screen.getByLabelText("Mã OTP");

    expect(input).toHaveAttribute("inputmode", "numeric");
    expect(input).toHaveAttribute("autocomplete", "one-time-code");
    expect(input).toHaveAttribute("maxlength", "6");
  });

  it("type otp voi otpLength=4 thi maxLength bang 4", () => {
    render(<Input label="Mã OTP" type="otp" otpLength={4} />);
    expect(screen.getByLabelText("Mã OTP")).toHaveAttribute("maxlength", "4");
  });

  it("type otp khong render ra type=otp (khong phai type HTML hop le)", () => {
    render(<Input label="Mã OTP" type="otp" />);
    expect(screen.getByLabelText("Mã OTP")).not.toHaveAttribute("type", "otp");
  });
});

describe("Input — type=password, nut hien/an mat khau (AC-4)", () => {
  it("mac dinh input la password va nut co aria-label Hien mat khau, aria-pressed false", () => {
    render(<Input label="Mật khẩu" type="password" />);

    expect(screen.getByLabelText("Mật khẩu")).toHaveAttribute("type", "password");
    const toggle = screen.getByRole("button", { name: "Hiện mật khẩu" });
    expect(toggle).toHaveAttribute("aria-pressed", "false");
  });

  it("bam nut: input thanh text, aria-label doi thanh An mat khau, aria-pressed true", async () => {
    const user = userEvent.setup();
    render(<Input label="Mật khẩu" type="password" />);

    await user.click(screen.getByRole("button", { name: "Hiện mật khẩu" }));

    expect(screen.getByLabelText("Mật khẩu")).toHaveAttribute("type", "text");
    const toggle = screen.getByRole("button", { name: "Ẩn mật khẩu" });
    expect(toggle).toHaveAttribute("aria-pressed", "true");
  });

  it("bam hai lan thi tro lai password", async () => {
    const user = userEvent.setup();
    render(<Input label="Mật khẩu" type="password" />);

    await user.click(screen.getByRole("button", { name: "Hiện mật khẩu" }));
    await user.click(screen.getByRole("button", { name: "Ẩn mật khẩu" }));

    expect(screen.getByLabelText("Mật khẩu")).toHaveAttribute("type", "password");
    expect(screen.getByRole("button", { name: "Hiện mật khẩu" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
  });

  it("type khac password thi KHONG co nut toggle (truong hop am)", () => {
    render(<Input label="Email" type="email" />);
    expect(screen.queryByRole("button", { name: "Hiện mật khẩu" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Ẩn mật khẩu" })).not.toBeInTheDocument();
  });
});

describe("Input — trang thai & hop dong chung", () => {
  it("disabled thi input bi disable", () => {
    render(<Input label="Email" disabled />);
    expect(screen.getByLabelText("Email")).toBeDisabled();
  });

  it("nhap duoc va onChange duoc goi", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Input label="Email" onChange={onChange} />);

    await user.type(screen.getByLabelText("Email"), "a@b.vn");

    expect(screen.getByLabelText("Email")).toHaveValue("a@b.vn");
    expect(onChange).toHaveBeenCalled();
  });

  it("focus duoc bang ban phim (AC-9)", async () => {
    const user = userEvent.setup();
    render(<Input label="Email" />);

    await user.tab();

    expect(screen.getByLabelText("Email")).toHaveFocus();
  });

  it("spread rest xuong input", () => {
    render(<Input label="Email" name="email" required />);
    const input = screen.getByLabelText("Email");
    expect(input).toHaveAttribute("name", "email");
    expect(input).toBeRequired();
  });
});
