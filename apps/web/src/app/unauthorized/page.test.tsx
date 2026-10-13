/**
 * Test cho trang /unauthorized — EV-188 code review M1 (plan §7).
 *
 * Trang chua dich (tieng Viet) va KHONG nam trong shell nao co <main lang="vi">, nen goc trang phai
 * tu khai bao lang="vi"; neu khong, khi nguoi dung chon English (<html lang="en">) screen reader doc
 * tieng Viet bang giong Anh (WCAG 3.1.2).
 */
import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import UnauthorizedPage from "./page";

vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({ user: null, logout: vi.fn() }),
}));

beforeEach(() => {
  document.documentElement.setAttribute("lang", "en");
});

afterEach(() => {
  document.documentElement.setAttribute("lang", "vi");
});

describe("UnauthorizedPage — lang (EV-188 M1)", () => {
  it('phan tu goc cua trang co lang="vi"', () => {
    const { container } = render(<UnauthorizedPage />);
    expect(container.firstElementChild).toHaveAttribute("lang", "vi");
  });

  it("html lang = en: tieu de 403 co ngon ngu hieu luc la vi", () => {
    render(<UnauthorizedPage />);
    const heading = screen.getByRole("heading", { level: 1, name: /403/ });
    expect(heading.closest("[lang]")?.getAttribute("lang")).toBe("vi");
  });
});
