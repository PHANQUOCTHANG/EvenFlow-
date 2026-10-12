import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  clearQueueToken,
  queueTokenKey,
  readQueueToken,
  saveQueueToken,
} from "./queue-token";

/** Hop dong EV-198 plan 1.1: MOT khoa sessionStorage duy nhat cho queue token cua mot su kien.
 *  Gia tri khoa phai giu nguyen `ef:queue:token:{eventId}` (khoa phong cho dang dung) de khach
 *  dang trong hang khong mat token khi deploy. */
describe("queue-token (EV-198, B1)", () => {
  const eventId = "evt-tech-summit-2026";

  beforeEach(() => {
    sessionStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    sessionStorage.clear();
  });

  describe("queueTokenKey", () => {
    it("trả về khoá dạng ef:queue:token:{eventId}", () => {
      expect(queueTokenKey(eventId)).toBe("ef:queue:token:evt-tech-summit-2026");
    });

    it("khoá khác nhau cho sự kiện khác nhau", () => {
      expect(queueTokenKey("evt-a")).not.toBe(queueTokenKey("evt-b"));
    });
  });

  describe("readQueueToken", () => {
    it("trả về null khi chưa có token", () => {
      expect(readQueueToken(eventId)).toBeNull();
    });

    it("đọc token phòng chờ đã ghi trực tiếp bằng khoá ef:queue:token: (tương thích ngược)", () => {
      sessionStorage.setItem(`ef:queue:token:${eventId}`, "tok-legacy-waiting-room");
      expect(readQueueToken(eventId)).toBe("tok-legacy-waiting-room");
    });

    it("không đọc nhầm token của sự kiện khác", () => {
      sessionStorage.setItem("ef:queue:token:evt-other", "tok-other");
      expect(readQueueToken(eventId)).toBeNull();
    });

    it("trả về null (không ném lỗi) khi sessionStorage.getItem ném lỗi", () => {
      sessionStorage.setItem(`ef:queue:token:${eventId}`, "tok");
      vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
        throw new DOMException("blocked", "SecurityError");
      });
      expect(() => readQueueToken(eventId)).not.toThrow();
      expect(readQueueToken(eventId)).toBeNull();
    });

    it("trả về null khi không có window / sessionStorage (SSR)", () => {
      sessionStorage.setItem(`ef:queue:token:${eventId}`, "tok-ssr");
      vi.stubGlobal("window", undefined);
      vi.stubGlobal("sessionStorage", undefined);
      expect(() => readQueueToken(eventId)).not.toThrow();
      expect(readQueueToken(eventId)).toBeNull();
    });
  });

  describe("saveQueueToken", () => {
    it("lưu token vào sessionStorage đúng khoá ef:queue:token:{eventId}", () => {
      saveQueueToken(eventId, "tok-123");
      expect(sessionStorage.getItem(`ef:queue:token:${eventId}`)).toBe("tok-123");
    });

    it("save rồi read trả lại đúng token", () => {
      saveQueueToken(eventId, "tok-roundtrip");
      expect(readQueueToken(eventId)).toBe("tok-roundtrip");
    });

    it("ghi đè token cũ của cùng sự kiện", () => {
      saveQueueToken(eventId, "tok-old");
      saveQueueToken(eventId, "tok-new");
      expect(readQueueToken(eventId)).toBe("tok-new");
    });

    it("nuốt lỗi khi sessionStorage.setItem ném lỗi (đầy / bị chặn)", () => {
      vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
        throw new DOMException("quota", "QuotaExceededError");
      });
      expect(() => saveQueueToken(eventId, "tok")).not.toThrow();
    });

    it("không ném lỗi khi không có window / sessionStorage (SSR)", () => {
      vi.stubGlobal("window", undefined);
      vi.stubGlobal("sessionStorage", undefined);
      expect(() => saveQueueToken(eventId, "tok")).not.toThrow();
    });
  });

  describe("clearQueueToken", () => {
    it("xoá token của sự kiện, không đụng sự kiện khác", () => {
      saveQueueToken(eventId, "tok-1");
      saveQueueToken("evt-other", "tok-2");

      clearQueueToken(eventId);

      expect(readQueueToken(eventId)).toBeNull();
      expect(sessionStorage.getItem(`ef:queue:token:${eventId}`)).toBeNull();
      expect(readQueueToken("evt-other")).toBe("tok-2");
    });

    it("không ném lỗi khi chưa có token", () => {
      expect(() => clearQueueToken(eventId)).not.toThrow();
    });

    it("nuốt lỗi khi sessionStorage.removeItem ném lỗi", () => {
      vi.spyOn(Storage.prototype, "removeItem").mockImplementation(() => {
        throw new DOMException("blocked", "SecurityError");
      });
      expect(() => clearQueueToken(eventId)).not.toThrow();
    });

    it("không ném lỗi khi không có window / sessionStorage (SSR)", () => {
      vi.stubGlobal("window", undefined);
      vi.stubGlobal("sessionStorage", undefined);
      expect(() => clearQueueToken(eventId)).not.toThrow();
    });
  });
});
