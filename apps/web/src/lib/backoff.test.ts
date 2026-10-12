import { describe, expect, it } from "vitest";

import { backoffDelay } from "./backoff";

const noJitter = { random: () => 0 };

describe("backoffDelay — AC-2 lui theo cap so nhan", () => {
  it("1s, 2s, 4s, 8s ... khi khong co jitter", () => {
    expect([1, 2, 3, 4].map((n) => backoffDelay(n, noJitter))).toEqual([1000, 2000, 4000, 8000]);
  });

  it("tran 30s, khong tang mai", () => {
    expect(backoffDelay(6, noJitter)).toBe(30_000);
    expect(backoffDelay(50, noJitter)).toBe(30_000);
  });

  it("attempt cuc lon khong tran thanh Infinity/NaN", () => {
    const d = backoffDelay(Number.MAX_SAFE_INTEGER, noJitter);
    expect(Number.isFinite(d)).toBe(true);
    expect(d).toBe(30_000);
  });

  it("attempt khong hop le (0, am, NaN) duoc coi nhu lan 1 — KHONG BAO GIO tra 0", () => {
    for (const bad of [0, -3, Number.NaN]) {
      expect(backoffDelay(bad, noJitter)).toBe(1000);
    }
  });

  it("jitter cong them trong [0, jitterMs)", () => {
    expect(backoffDelay(1, { random: () => 0.999 })).toBe(1499);
    expect(backoffDelay(1, { random: () => 0.5, jitterMs: 1000 })).toBe(1500);
  });

  it("jitter that phan tan cac client (khong cung mot gia tri)", () => {
    const seen = new Set<number>();
    for (let i = 0; i < 50; i++) seen.add(backoffDelay(1));
    expect(seen.size).toBeGreaterThan(1);
    for (const d of seen) {
      expect(d).toBeGreaterThanOrEqual(1000);
      expect(d).toBeLessThan(1500);
    }
  });

  it("tuy chinh baseMs / maxMs", () => {
    expect(backoffDelay(3, { ...noJitter, baseMs: 500, maxMs: 1500 })).toBe(1500);
    expect(backoffDelay(2, { ...noJitter, baseMs: 500 })).toBe(1000);
  });
});
