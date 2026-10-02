import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach, vi } from "vitest";

// jsdom khong implement matchMedia. Stub mot lan de code cham prefers-color-scheme
// khong throw; test nao can gia lap dark thi tu ghi de window.matchMedia cua rieng no.
if (typeof window !== "undefined" && !window.matchMedia) {
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })) as unknown as typeof window.matchMedia;
}

// Don DOM sau moi test de test khong anh huong nhau.
afterEach(() => {
  cleanup();
});
