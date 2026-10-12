/**
 * Test cho `DomainGallery` — AC-10 (spec-plan EVF-1806 §4).
 *
 * Showcase phai trinh bay `AssistantPanel` o ca 4 nac suy bien (NORMAL / SAVING /
 * FAQ_ONLY / OFF) bang du lieu mau, de reviewer nhin duoc copy cua tung nac ma khong
 * can backend. Test chi bam vao `data-slot` / `data-mode` da chot o spec §3, khong
 * assert cau chu cua section.
 *
 * Gallery dung `useEffect` de dat moc demo cho cac dong ho -> render trong `act`.
 * `window.matchMedia` da duoc stub o `vitest.setup.ts`.
 */
import { act, render, type RenderResult } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { DomainGallery } from "./domain-gallery";

const ASSISTANT_MODES = ["NORMAL", "SAVING", "FAQ_ONLY", "OFF"] as const;

function renderGallery(): RenderResult {
  let view: RenderResult | undefined;
  act(() => {
    view = render(<DomainGallery />);
  });
  if (!view) throw new Error("render khong tra ve ket qua");
  return view;
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(Date.UTC(2026, 0, 15, 3, 0, 0));
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("DomainGallery — section Trợ lý AI (AC-10)", () => {
  it("render duoc khong throw (AC-10)", () => {
    expect(() => renderGallery()).not.toThrow();
  });

  it("co heading chua 'Trợ lý' (AC-10)", () => {
    const { getAllByRole } = renderGallery();
    const headings = getAllByRole("heading").filter((el) => /Trợ lý/.test(el.textContent ?? ""));

    expect(headings.length).toBeGreaterThan(0);
  });

  it.each(ASSISTANT_MODES)(
    'co it nhat mot [data-slot="assistant-panel"][data-mode="%s"] (AC-10)',
    (mode) => {
      const { container } = renderGallery();
      const panels = container.querySelectorAll(
        `[data-slot="assistant-panel"][data-mode="${mode}"]`,
      );

      expect(panels.length).toBeGreaterThan(0);
    },
  );

  it("moi panel tro ly trong showcase deu mang data-authoritative=false (AC-10, BR-A4)", () => {
    const { container } = renderGallery();
    const panels = Array.from(container.querySelectorAll('[data-slot="assistant-panel"]'));

    expect(panels.length).toBeGreaterThanOrEqual(ASSISTANT_MODES.length);
    for (const panel of panels) {
      expect(panel.getAttribute("data-authoritative")).toBe("false");
    }
  });

  it("cac section cu van con (khong sua section cu, chi them) (AC-10, §5 rui ro c)", () => {
    const { getAllByRole } = renderGallery();
    const titles = getAllByRole("heading").map((el) => el.textContent ?? "");

    expect(titles.some((title) => /Phòng chờ ảo/.test(title))).toBe(true);
  });
});
