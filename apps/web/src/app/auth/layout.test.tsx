/**
 * Test cho app/auth/layout.tsx — EV-188 code review M1 (plan §7).
 *
 * Trang /auth/** (dang nhap, dang ky, quen mat khau) chua dich va khong nam trong shell nao co
 * <main lang="vi">. Layout boc noi dung trong phan tu lang="vi" — KHONG doi giao dien: khong them
 * landmark, khong them chu, children render nguyen ven.
 */
import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import AuthLayout from "./layout";

const CHILD = "Đăng nhập";

beforeEach(() => {
  document.documentElement.setAttribute("lang", "en");
});

afterEach(() => {
  document.documentElement.setAttribute("lang", "vi");
});

describe("AuthLayout — lang (EV-188 M1)", () => {
  it("html lang = en: noi dung con co ngon ngu hieu luc la vi", () => {
    render(
      <AuthLayout>
        <h1>{CHILD}</h1>
      </AuthLayout>,
    );
    const heading = screen.getByRole("heading", { level: 1, name: CHILD });
    const owner = heading.closest("[lang]");
    expect(owner).not.toBe(document.documentElement);
    expect(owner?.getAttribute("lang")).toBe("vi");
  });

  it("khong doi giao dien: khong them landmark, khong them chu", () => {
    const { container } = render(
      <AuthLayout>
        <p>{CHILD}</p>
      </AuthLayout>,
    );
    expect(container.querySelectorAll("main, header, footer, nav")).toHaveLength(0);
    expect((container.textContent ?? "").trim()).toBe(CHILD);
  });
});
