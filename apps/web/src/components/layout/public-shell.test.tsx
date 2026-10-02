/**
 * Test cho PublicShell — AC-2 (banner + main + contentinfo), AC-3 (SkipLink + main),
 * AC-6 (chi mot <nav>).
 *
 * Day la cho de lap bug "hai <main>" nhat: root layout da co cau truc, shell lai
 * boc them Container as="main". AC-2 doi DUNG MOT <main> nen kiem truc tiep bang
 * querySelectorAll.
 */
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { PublicShell } from "./public-shell";

const { pathnameMock } = vi.hoisted(() => ({ pathnameMock: vi.fn(() => "/") }));

vi.mock("next/navigation", () => ({
  usePathname: () => pathnameMock(),
}));

const SKIP_LABEL = "Bỏ qua điều hướng, tới nội dung chính";
const CHILD = "Noi dung trang marketing";

beforeEach(() => {
  pathnameMock.mockReturnValue("/");
  window.localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
});

afterEach(() => {
  vi.clearAllMocks();
  window.localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
});

describe("PublicShell — AC-2 landmark", () => {
  it("co banner, main va contentinfo", () => {
    render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    expect(screen.getByRole("banner")).toBeInTheDocument();
    expect(screen.getByRole("main")).toBeInTheDocument();
    expect(screen.getByRole("contentinfo")).toBeInTheDocument();
  });

  it("render DUNG MOT <main>", () => {
    const { container } = render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    expect(container.querySelectorAll("main")).toHaveLength(1);
  });

  it("render DUNG MOT <nav> (khong nhan doi desktop/mobile)", () => {
    const { container } = render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    expect(container.querySelectorAll("nav")).toHaveLength(1);
    expect(screen.getAllByRole("navigation")).toHaveLength(1);
  });

  it("<nav> nam trong banner, khong nam trong main", () => {
    render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    const nav = screen.getByRole("navigation");
    expect(screen.getByRole("banner")).toContainElement(nav);
    expect(screen.getByRole("main")).not.toContainElement(nav);
  });

  it("thu tu DOM: banner truoc main, main truoc contentinfo", () => {
    const { container } = render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    const html = container.innerHTML;
    expect(html.indexOf("<header")).toBeGreaterThanOrEqual(0);
    expect(html.indexOf("<header")).toBeLessThan(html.indexOf("<main"));
    expect(html.indexOf("<main")).toBeLessThan(html.indexOf("<footer"));
  });
});

describe("PublicShell — children", () => {
  it("render children", () => {
    render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    expect(screen.getByText(CHILD)).toBeInTheDocument();
  });

  it("children nam BEN TRONG <main>", () => {
    render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    expect(screen.getByRole("main")).toContainElement(screen.getByText(CHILD));
  });
});

describe("PublicShell — AC-3 SkipLink va muc tieu #main", () => {
  it('co SkipLink tro tới "#main"', () => {
    render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    expect(screen.getByRole("link", { name: SKIP_LABEL })).toHaveAttribute("href", "#main");
  });

  it('<main> co id="main"', () => {
    render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    expect(screen.getByRole("main")).toHaveAttribute("id", "main");
  });

  it('<main> co tabindex="-1" de nhan duoc focus sau khi nhay', () => {
    render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    expect(screen.getByRole("main")).toHaveAttribute("tabindex", "-1");
  });

  it("href cua SkipLink khop dung id cua <main>", () => {
    render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    const href = screen.getByRole("link", { name: SKIP_LABEL }).getAttribute("href") ?? "";
    expect(href.startsWith("#")).toBe(true);
    expect(document.getElementById(href.slice(1))).toBe(screen.getByRole("main"));
  });

  it("Tab lan dau trong shell thi SkipLink nhan focus", async () => {
    const user = userEvent.setup();
    render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    await user.tab();

    expect(screen.getByRole("link", { name: SKIP_LABEL })).toHaveFocus();
  });
});
