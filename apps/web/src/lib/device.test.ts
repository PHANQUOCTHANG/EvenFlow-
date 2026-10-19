import { beforeEach, describe, expect, it, vi } from "vitest";
import { getDeviceFingerprint } from "./device";

describe("getDeviceFingerprint", () => {
  beforeEach(() => {
    window.localStorage.clear();
    vi.restoreAllMocks();
  });

  it("sinh id du dai va giu nguyen qua cac lan goi", () => {
    const a = getDeviceFingerprint();
    expect(a.length).toBeGreaterThanOrEqual(8);
    expect(getDeviceFingerprint()).toBe(a);
    expect(window.localStorage.getItem("ef:device-id")).toBe(a);
  });

  it("dung lai id da luu", () => {
    window.localStorage.setItem("ef:device-id", "may-da-luu-12345");
    expect(getDeviceFingerprint()).toBe("may-da-luu-12345");
  });

  it("bo qua gia tri luu qua ngan", () => {
    window.localStorage.setItem("ef:device-id", "abc");
    expect(getDeviceFingerprint().length).toBeGreaterThanOrEqual(8);
  });

  it("van tra id khi localStorage nem loi", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    const id = getDeviceFingerprint();
    expect(id.length).toBeGreaterThanOrEqual(8);
    expect(getDeviceFingerprint()).toBe(id);
  });
});
