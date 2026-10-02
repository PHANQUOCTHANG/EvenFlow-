/**
 * Test cho SkipLink — AC-3.
 *
 * Bug kinh dien: an skip link bang `display: none` / `visibility: hidden` cho "gon".
 * Lam vay thi ban phim KHONG BAO GIO toi duoc no -> component ton tai nhung vo dung,
 * va khong co test nao do lam DOM van co the <a>. Vi vay o day kiem:
 *   - no nam trong accessibility tree (getByRole tim ra duoc)
 *   - Tab lan dau thi no nhan focus (tuc la focusable that)
 *   - khong co inline style display:none / visibility:hidden, khong aria-hidden,
 *     khong hidden, khong tabindex="-1"
 * Lop ao `sr-only`/`focus:not-sr-only` cua Tailwind khong chay trong jsdom nen
 * khong kiem duoc bang getComputedStyle — xem traceability.md.
 */
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { SkipLink } from "./skip-link";

const SKIP_LABEL = "Bỏ qua điều hướng, tới nội dung chính";

function skipLink(): HTMLElement {
  return screen.getByRole("link", { name: SKIP_LABEL });
}

describe("SkipLink — hop dong copy va href", () => {
  it("render link voi dung noi dung tieng Viet theo hop dong", () => {
    render(<SkipLink />);
    expect(skipLink()).toBeInTheDocument();
    expect(skipLink()).toHaveAccessibleName(SKIP_LABEL);
  });

  it('mac dinh tro tới "#main" (targetId mac dinh "main")', () => {
    render(<SkipLink />);
    expect(skipLink()).toHaveAttribute("href", "#main");
  });

  it('targetId="main" cho ra dung href "#main"', () => {
    render(<SkipLink targetId="main" />);
    expect(skipLink()).toHaveAttribute("href", "#main");
  });

  it("targetId khac duoc ton trong", () => {
    render(<SkipLink targetId="noi-dung-chinh" />);
    expect(skipLink()).toHaveAttribute("href", "#noi-dung-chinh");
  });

  it("targetId khong duoc nhan doi dau '#'", () => {
    render(<SkipLink targetId="main" />);
    expect(skipLink().getAttribute("href")).not.toContain("##");
  });

  it("la the <a> that (khong phai <button> gia link)", () => {
    const { container } = render(<SkipLink />);
    const anchor = container.querySelector("a");
    expect(anchor).not.toBeNull();
    expect(anchor).toBe(skipLink());
  });
});

describe("SkipLink — AC-3 ban phim toi duoc", () => {
  it("nam trong accessibility tree (khong bi aria-hidden / hidden)", () => {
    render(<SkipLink />);
    const link = skipLink();
    expect(link).not.toHaveAttribute("aria-hidden", "true");
    expect(link).not.toHaveAttribute("hidden");
  });

  it('KHONG bi lay ra khoi tab order bang tabindex="-1"', () => {
    render(<SkipLink />);
    expect(skipLink()).not.toHaveAttribute("tabindex", "-1");
  });

  it("KHONG bi an bang inline display:none hay visibility:hidden", () => {
    render(<SkipLink />);
    const link = skipLink();
    expect(link.style.display).not.toBe("none");
    expect(link.style.visibility).not.toBe("hidden");
    expect(window.getComputedStyle(link).display).not.toBe("none");
    expect(window.getComputedStyle(link).visibility).not.toBe("hidden");
  });

  it("Tab lan dau thi SkipLink nhan focus (truoc moi phan tu khac)", async () => {
    const user = userEvent.setup();
    render(
      <>
        <SkipLink />
        <button type="button">Mua vé</button>
        <main id="main" tabIndex={-1}>
          Noi dung
        </main>
      </>,
    );

    await user.tab();

    expect(skipLink()).toHaveFocus();
    expect(document.activeElement).toBe(skipLink());
  });

  it("focus duoc truc tiep bang .focus()", () => {
    render(<SkipLink />);
    const link = skipLink();
    link.focus();
    expect(link).toHaveFocus();
  });
});
