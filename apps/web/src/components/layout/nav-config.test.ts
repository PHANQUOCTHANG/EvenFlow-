/**
 * Test cho nav-config — AC-4 (isActive) va AC-5 (khong link gia).
 *
 * `isActive` la noi bug thuc su nam. Hai cach cai sai pho bien:
 *   1. `pathname.startsWith(href)` cho MOI href -> href "/" khop moi trang,
 *      nen link "Trang chu" luon sang. Spec §5: href "/" khop CHINH XAC.
 *   2. `pathname.startsWith(href)` khong kem dau "/" -> "/organizer/events"
 *      khop ca "/organizer/events-archive" (trang khac han).
 * Ca hai duoc kiem truc tiep o duoi.
 *
 * Phan kiem danh sach nav co y khong hard-code nhan tieng Viet cu the: spec §5
 * chi chot HINH DANG cua NavItem va ten 3 hang so, khong chot noi dung tung item.
 * Test o day kiem hinh dang + bat bien, de implementer con duoc chon noi dung.
 *
 * Luu y (spec §2.2): 3 hang so nay la CAU HINH HIEN THI, khong phai ma tran quyen.
 */
import { describe, expect, it } from "vitest";

import { OPS_NAV, ORGANIZER_NAV, PUBLIC_NAV, isActive, type NavItem } from "./nav-config";

const CONFIGS: Array<[string, NavItem[]]> = [
  ["PUBLIC_NAV", PUBLIC_NAV],
  ["ORGANIZER_NAV", ORGANIZER_NAV],
  ["OPS_NAV", OPS_NAV],
];

describe("nav-config — hinh dang danh sach nav", () => {
  for (const [name, items] of CONFIGS) {
    describe(name, () => {
      it("la mang va khong rong", () => {
        expect(Array.isArray(items)).toBe(true);
        expect(items.length).toBeGreaterThan(0);
      });

      it("moi item co href bat dau bang '/'", () => {
        for (const item of items) {
          expect(typeof item.href).toBe("string");
          expect(item.href.startsWith("/")).toBe(true);
        }
      });

      it("moi item co label khong rong", () => {
        for (const item of items) {
          expect(typeof item.label).toBe("string");
          expect(item.label.trim().length).toBeGreaterThan(0);
        }
      });

      it("moi item co ready la boolean (khong undefined, khong string)", () => {
        for (const item of items) {
          expect(typeof item.ready).toBe("boolean");
        }
      });

      it("href khong trung nhau trong cung danh sach", () => {
        const hrefs = items.map((item) => item.href);
        expect(new Set(hrefs).size).toBe(hrefs.length);
      });

      it("label khong trung nhau trong cung danh sach", () => {
        const labels = items.map((item) => item.label);
        expect(new Set(labels).size).toBe(labels.length);
      });

      it("khong co href rong hay href '#' (spec §2.1 khong link gia)", () => {
        for (const item of items) {
          expect(item.href).not.toBe("");
          expect(item.href).not.toBe("#");
        }
      });
    });
  }
});

describe("nav-config — AC-5 co that su dung co ready: false", () => {
  it("ton tai it nhat mot item ready: false (hien chua co route nao ngoai trang chu)", () => {
    const all = [...PUBLIC_NAV, ...ORGANIZER_NAV, ...OPS_NAV];
    expect(all.some((item) => item.ready === false)).toBe(true);
  });
});

describe("isActive — AC-4 khop chinh xac cho route goc", () => {
  it('href "/" active khi pathname la "/"', () => {
    expect(isActive("/", "/")).toBe(true);
  });

  it('href "/" KHONG active khi pathname la "/organizer" (bug startsWith)', () => {
    expect(isActive("/organizer", "/")).toBe(false);
  });

  it('href "/" KHONG active khi pathname la "/events/abc"', () => {
    expect(isActive("/events/abc", "/")).toBe(false);
  });
});

describe("isActive — AC-4 khop chinh xac va khop route con", () => {
  it("khop chinh xac", () => {
    expect(isActive("/organizer/events", "/organizer/events")).toBe(true);
  });

  it("route con van lam sang item cha", () => {
    expect(isActive("/organizer/events/123", "/organizer/events")).toBe(true);
  });

  it("route con nhieu tang van lam sang item cha", () => {
    expect(isActive("/organizer/events/123/tickets", "/organizer/events")).toBe(true);
  });

  it("tien to KHONG theo bien segment thi khong active (bug thieu dau '/')", () => {
    expect(isActive("/organizer/events-archive", "/organizer/events")).toBe(false);
  });

  it("pathname la cha cua href thi khong active", () => {
    expect(isActive("/organizer", "/organizer/events")).toBe(false);
  });

  it("hai nhanh khac nhau thi khong active", () => {
    expect(isActive("/ops/queues", "/organizer/events")).toBe(false);
  });

  it("item cha /organizer active khi dang o /organizer/events", () => {
    expect(isActive("/organizer/events", "/organizer")).toBe(true);
  });
});
