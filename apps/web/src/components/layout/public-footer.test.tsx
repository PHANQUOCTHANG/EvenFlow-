/**
 * Test cho PublicFooter — AC-2 (landmark contentinfo) va AC-5 (khong link gia).
 *
 * Hop dong da chot: moi shell chi co DUNG MOT <nav>, va no nam o header.
 * Vi vay footer khong duoc tu mo thêm mot <nav> — neu co, PublicShell se co 2
 * landmark navigation va AC-2 doc sai.
 */
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { PublicFooter } from "./public-footer";

describe("PublicFooter — AC-2 landmark", () => {
  it("render landmark contentinfo", () => {
    render(<PublicFooter />);
    expect(screen.getByRole("contentinfo")).toBeInTheDocument();
  });

  it("la the <footer> that", () => {
    const { container } = render(<PublicFooter />);
    const footer = container.querySelector("footer");
    expect(footer).not.toBeNull();
    expect(footer).toBe(screen.getByRole("contentinfo"));
  });

  it("co noi dung chu, khong phai footer rong", () => {
    render(<PublicFooter />);
    expect((screen.getByRole("contentinfo").textContent ?? "").trim().length).toBeGreaterThan(0);
  });

  it("KHONG tu mo thêm <nav> (chi header co nav)", () => {
    const { container } = render(<PublicFooter />);
    expect(container.querySelectorAll("nav")).toHaveLength(0);
    expect(screen.queryAllByRole("navigation")).toHaveLength(0);
  });

  it("KHONG render <main> hay <header>", () => {
    const { container } = render(<PublicFooter />);
    expect(container.querySelectorAll("main")).toHaveLength(0);
    expect(container.querySelectorAll("header")).toHaveLength(0);
  });
});

describe("PublicFooter — AC-5 khong link gia", () => {
  it('moi <a> deu co href khong rong va khac "#"', () => {
    const { container } = render(<PublicFooter />);

    for (const anchor of Array.from(container.querySelectorAll("a"))) {
      const href = anchor.getAttribute("href");
      expect(href).toBeTruthy();
      expect(href).not.toBe("#");
    }
  });

  it("moi <a> deu co accessible name (khong link rong)", () => {
    const { container } = render(<PublicFooter />);

    for (const anchor of Array.from(container.querySelectorAll("a"))) {
      expect((anchor.textContent ?? "").trim().length).toBeGreaterThan(0);
    }
  });
});
