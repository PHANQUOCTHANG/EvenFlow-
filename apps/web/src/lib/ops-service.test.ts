/**
 * Test cho cac helper thuan cua ops-service (EV-187): hang so do tuoi, BR-O1 (tierInventoryViolation),
 * tuoi du lieu (snapshotAgeMs), nhan trang thai ban.
 *
 * ops-warroom (test change request): cac test du lieu mau cu (SAMPLE_EVENTS, __setOpsSampleOverride, isSample = true,
 * so lieu mau tat dinh) da duoc thay bang lib/ops-service.contract.test.ts (du lieu that qua apiFetch).
 */
import { describe, expect, it } from "vitest";

import {
  OPS_POLL_INTERVAL_MS,
  STALE_AFTER_MS,
  SALE_STATE_LABEL,
  saleStateLabel,
  snapshotAgeMs,
  tierInventoryViolation,
  type OpsTierInventory,
} from "./ops-service";

function tier(overrides: Partial<OpsTierInventory> = {}): OpsTierInventory {
  return {
    tierId: "t1",
    name: "Hạng thử",
    quota: 100,
    held: 10,
    paid: 20,
    issued: 30,
    available: 40,
    ...overrides,
  };
}

describe("ops-service — hằng số độ tươi (AC-2)", () => {
  it("STALE_AFTER_MS = 5000 (dữ liệu trễ không quá 5s)", () => {
    expect(STALE_AFTER_MS).toBe(5_000);
  });

  it("OPS_POLL_INTERVAL_MS = 2000 (làm mới mỗi 2s)", () => {
    expect(OPS_POLL_INTERVAL_MS).toBe(2_000);
  });
});

describe("tierInventoryViolation — bất biến BR-O1 (AC-4)", () => {
  it("hạng vé hợp lệ trả null", () => {
    expect(tierInventoryViolation(tier())).toBeNull();
  });

  it("held + paid + issued đúng bằng quota, available = 0 vẫn hợp lệ", () => {
    expect(
      tierInventoryViolation(tier({ quota: 60, held: 10, paid: 20, issued: 30, available: 0 })),
    ).toBeNull();
  });

  it("held + paid + issued > quota trả OVER_QUOTA", () => {
    expect(
      tierInventoryViolation(tier({ quota: 59, held: 10, paid: 20, issued: 30, available: 0 })),
    ).toBe("OVER_QUOTA");
  });

  it("available < 0 (tổng chưa vượt quota) trả NEGATIVE_AVAILABLE", () => {
    expect(
      tierInventoryViolation(tier({ quota: 100, held: 10, paid: 20, issued: 30, available: -1 })),
    ).toBe("NEGATIVE_AVAILABLE");
  });

  it("vi phạm cả hai điều kiện thì trả OVER_QUOTA (plan §6 m-6)", () => {
    expect(
      tierInventoryViolation(tier({ quota: 10, held: 10, paid: 20, issued: 30, available: -50 })),
    ).toBe("OVER_QUOTA");
  });
});

describe("snapshotAgeMs — tuổi dữ liệu tính từ generatedAt (AC-2)", () => {
  const GEN = "2026-10-13T03:00:00.000Z";
  const GEN_MS = Date.parse(GEN);

  it("bằng nowMs - generatedAt", () => {
    expect(snapshotAgeMs({ generatedAt: GEN }, GEN_MS + 1234)).toBe(1234);
  });

  it("bằng 0 khi now trùng generatedAt", () => {
    expect(snapshotAgeMs({ generatedAt: GEN }, GEN_MS)).toBe(0);
  });

  it("vượt STALE_AFTER_MS khi trễ hơn 5s", () => {
    expect(snapshotAgeMs({ generatedAt: GEN }, GEN_MS + 5001)).toBeGreaterThan(STALE_AFTER_MS);
  });

  it("generatedAt hỏng trả Infinity (coi là cũ, không coi là mới)", () => {
    expect(snapshotAgeMs({ generatedAt: "khong-phai-ngay" }, GEN_MS)).toBe(Infinity);
  });

  it("generatedAt rỗng trả Infinity", () => {
    expect(snapshotAgeMs({ generatedAt: "" }, GEN_MS)).toBe(Infinity);
  });
});

describe("tierInventoryViolation — INVALID_DATA (plan §8 C-4)", () => {
  it.each(["quota", "held", "paid", "issued", "available"] as const)(
    "%s không hữu hạn (NaN) trả INVALID_DATA",
    (field) => {
      expect(tierInventoryViolation(tier({ [field]: Number.NaN }))).toBe("INVALID_DATA");
    },
  );

  it("Infinity cũng là INVALID_DATA", () => {
    expect(tierInventoryViolation(tier({ held: Number.POSITIVE_INFINITY }))).toBe("INVALID_DATA");
    expect(tierInventoryViolation(tier({ available: Number.NEGATIVE_INFINITY }))).toBe("INVALID_DATA");
  });

  it("INVALID_DATA ưu tiên trước OVER_QUOTA", () => {
    expect(
      tierInventoryViolation(tier({ quota: 10, held: 100, paid: 100, issued: Number.NaN, available: -5 })),
    ).toBe("INVALID_DATA");
  });
});

describe("saleStateLabel — nhãn trạng thái bán", () => {
  it.each(Object.entries(SALE_STATE_LABEL))("%s => %s", (state, label) => {
    expect(saleStateLabel(state)).toBe(label);
  });

  it("giá trị lạ => 'Không rõ trạng thái'", () => {
    expect(saleStateLabel("WHATEVER")).toBe("Không rõ trạng thái");
  });
});
