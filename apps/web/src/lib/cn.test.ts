/**
 * Unit test cho helper cn() — AC-1 / plan muc 3.
 *
 * cn() la nen tang cua moi component: neu no bo mat class nguoi dung truyen vao
 * thi prop `className` cua ca component library se bi ghi de im lang (AC-3).
 */
import { describe, expect, it } from "vitest";

import { cn } from "./cn";

describe("cn", () => {
  it("noi nhieu class bang dung mot dau cach", () => {
    expect(cn("a", "b", "c")).toBe("a b c");
  });

  it("bo gia tri false / null / undefined", () => {
    expect(cn("a", false, null, undefined, "b")).toBe("a b");
  });

  it("bo chuoi rong", () => {
    expect(cn("a", "", "b")).toBe("a b");
  });

  it("khong co phan nao hop le thi tra ve chuoi rong", () => {
    expect(cn()).toBe("");
    expect(cn(false, null, undefined, "")).toBe("");
  });

  it("giu dung thu tu tham so", () => {
    const out = cn("btn btn-primary", "mt-4");
    expect(out.startsWith("btn btn-primary")).toBe(true);
    expect(out.endsWith("mt-4")).toBe(true);
  });

  it("khong ghi de: ca class goc va class nguoi dung deu con lai", () => {
    const parts = cn("base", undefined, "custom").split(" ");
    expect(parts).toContain("base");
    expect(parts).toContain("custom");
  });

  it("ket qua luon la string, khong bao gio undefined", () => {
    expect(typeof cn(undefined)).toBe("string");
    expect(typeof cn("x")).toBe("string");
  });
});
