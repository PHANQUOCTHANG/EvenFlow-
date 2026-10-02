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

import {
  OPS_NAV,
  ORGANIZER_NAV,
  PUBLIC_NAV,
  activeHref,
  isActive,
  type NavItem,
} from "./nav-config";

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

/**
 * activeHref — bug M4.
 *
 * `isActive(pathname, href)` tra loi cau hoi VE MOT ITEM, nen no khong the biet
 * trong danh sach con item nao khop sat hon. Khi 2 href long tien to nhau
 * (`/organizer/events` va `/organizer/events/new`), o pathname
 * `/organizer/events/new` thi `isActive` tra TRUE cho ca hai -> hai item cung
 * `aria-current="page"` -> screen reader bao hai "trang hien tai" trong mot nav.
 *
 * `activeHref(items, pathname)` la cau tra loi o CAP DANH SACH: chon item khop
 * DAI NHAT, hoac null. Day moi la thu duoc dung de dat aria-current.
 *
 * Hom nay bug con an vi ca hai item do `ready: false`; EVF-1802 bat `ready: true`
 * la lo ra. Vi vay test o day chot san.
 */
describe("activeHref — AC-4 chon item khop DAI NHAT", () => {
  it("o /organizer/events/new tra ve /organizer/events/new, KHONG phai /organizer/events", () => {
    expect(activeHref(ORGANIZER_NAV, "/organizer/events/new")).toBe("/organizer/events/new");
  });

  it("o /organizer/events tra ve dung /organizer/events", () => {
    expect(activeHref(ORGANIZER_NAV, "/organizer/events")).toBe("/organizer/events");
  });

  it("o route con /organizer/events/123 tra ve item cha /organizer/events", () => {
    expect(activeHref(ORGANIZER_NAV, "/organizer/events/123")).toBe("/organizer/events");
  });

  it('o "/" tra ve "/" trong PUBLIC_NAV', () => {
    expect(activeHref(PUBLIC_NAV, "/")).toBe("/");
  });

  it('o /organizer tra ve null trong PUBLIC_NAV (dac biet KHONG phai "/")', () => {
    const result = activeHref(PUBLIC_NAV, "/organizer");
    expect(result).toBeNull();
    expect(result).not.toBe("/");
  });

  it("danh sach rong tra ve null", () => {
    expect(activeHref([], "/bat-ky")).toBeNull();
  });

  it("pathname khong khop item nao tra ve null", () => {
    expect(activeHref(ORGANIZER_NAV, "/khong-he-ton-tai-o-dau-ca")).toBeNull();
  });

  it("gia tri tra ve luon la mot href co that trong danh sach, hoac null", () => {
    for (const [name, items] of CONFIGS) {
      const hrefs = new Set(items.map((item) => item.href));

      for (const item of items) {
        const result = activeHref(items, item.href);
        expect(result, `${name} @ ${item.href}`).not.toBeNull();
        expect(hrefs.has(result as string), `${name} @ ${item.href}`).toBe(true);
      }
    }
  });

  it("khop chinh xac thang: pathname = href cua item thi tra ve dung href do", () => {
    for (const [name, items] of CONFIGS) {
      for (const item of items) {
        expect(activeHref(items, item.href), `${name} @ ${item.href}`).toBe(item.href);
      }
    }
  });
});

describe("activeHref — ghi lai bug M4: isActive mot minh KHONG du", () => {
  it("o /organizer/events/new, isActive khop NHIEU hon mot item (2 href long tien to)", () => {
    const matched = ORGANIZER_NAV.filter((item) => isActive("/organizer/events/new", item.href));

    // Day la bug: neu dat aria-current theo isActive thi ca 2 item deu sang.
    expect(matched.length).toBeGreaterThan(1);

    // activeHref thi chi tra ve DUNG MOT, va la cai dai nhat.
    expect(activeHref(ORGANIZER_NAV, "/organizer/events/new")).toBe("/organizer/events/new");
  });

  it("khi isActive khop nhieu item, activeHref luon chon href DAI NHAT", () => {
    for (const [name, items] of CONFIGS) {
      for (const item of items) {
        const pathname = item.href;
        const matched = items
          .filter((candidate) => isActive(pathname, candidate.href))
          .map((candidate) => candidate.href);

        expect(matched.length, `${name} @ ${pathname}`).toBeGreaterThan(0);

        const longest = matched.reduce((a, b) => (b.length > a.length ? b : a));
        expect(activeHref(items, pathname), `${name} @ ${pathname}`).toBe(longest);
      }
    }
  });

  it("voi moi pathname lay tu item, dung MOT item duoc coi la active o cap danh sach", () => {
    for (const [name, items] of CONFIGS) {
      for (const item of items) {
        const winner = activeHref(items, item.href);
        const activeCount = items.filter((candidate) => candidate.href === winner).length;

        expect(activeCount, `${name} @ ${item.href}`).toBe(1);
      }
    }
  });

  it("route con sau cung khong lam sang 2 item: /organizer/events/new/abc chon /organizer/events/new", () => {
    expect(activeHref(ORGANIZER_NAV, "/organizer/events/new/abc")).toBe("/organizer/events/new");
  });
});
