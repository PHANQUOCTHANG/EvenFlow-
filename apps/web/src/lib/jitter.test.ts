import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  calculateJitterMs,
  DEFAULT_MAX_JITTER_MS,
  DEFAULT_MIN_JITTER_MS,
  type JitterProgress,
  sleepWithJitter,
} from "./jitter";

describe("jitter (BR-Q5, EVF-61)", () => {
  describe("calculateJitterMs", () => {
    it("tra ve gia tri mac dinh nam trong khoang [0, 5000]ms", () => {
      for (let i = 0; i < 100; i++) {
        const val = calculateJitterMs();
        expect(val).toBeGreaterThanOrEqual(DEFAULT_MIN_JITTER_MS);
        expect(val).toBeLessThanOrEqual(DEFAULT_MAX_JITTER_MS);
      }
    });

    it("tuan thu ham random tuy bien", () => {
      expect(calculateJitterMs({ randomFn: () => 0 })).toBe(0);
      expect(calculateJitterMs({ randomFn: () => 0.5 })).toBe(2500);
      expect(calculateJitterMs({ randomFn: () => 1 })).toBe(5000);
    });

    it("ho tro khoang min/max tuy chinh", () => {
      const val = calculateJitterMs({
        minJitterMs: 1000,
        maxJitterMs: 3000,
        randomFn: () => 0.5,
      });
      expect(val).toBe(2000);
    });

    it("xu ly khi min === max", () => {
      expect(calculateJitterMs({ minJitterMs: 2000, maxJitterMs: 2000 })).toBe(2000);
    });

    it("xu ly min > max an toan (clamp max len bang min)", () => {
      expect(calculateJitterMs({ minJitterMs: 3000, maxJitterMs: 1000 })).toBe(3000);
    });
  });

  describe("sleepWithJitter", () => {
    beforeEach(() => {
      vi.useFakeTimers();
    });

    afterEach(() => {
      vi.useRealTimers();
    });

    it("giai quyet tuc thi neu jitter = 0", async () => {
      const progress: JitterProgress[] = [];
      const promise = sleepWithJitter((p) => progress.push(p), {
        randomFn: () => 0,
        minJitterMs: 0,
        maxJitterMs: 0,
      });

      const total = await promise;
      expect(total).toBe(0);
      expect(progress.length).toBe(1);
      expect(progress[0]).toEqual({ remainingMs: 0, totalJitterMs: 0 });
    });

    it("cap nhat tien trinh va resolve dung tong thoi gian", async () => {
      const progress: JitterProgress[] = [];
      const promise = sleepWithJitter((p) => progress.push(p), {
        randomFn: () => 0.2, // 1000ms
      });

      // Khoi tao: 1000ms
      expect(progress.length).toBe(1);
      expect(progress[0].remainingMs).toBe(1000);
      expect(progress[0].totalJitterMs).toBe(1000);

      // Chay timer 500ms
      vi.advanceTimersByTime(500);
      expect(progress.length).toBeGreaterThan(1);

      // Chay het timer
      vi.advanceTimersByTime(600);
      const total = await promise;
      expect(total).toBe(1000);
      expect(progress[progress.length - 1].remainingMs).toBe(0);
    });
  });
});
