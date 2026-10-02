/**
 * Test cho `formatMoney` — AC-1 (spec-plan EVF-1802/1804 §4), §2.4.
 *
 * File RIENG voi `format.test.ts`: `formatVnd` la ham dinh dang mot loai tien,
 * `formatMoney` la ham QUYET DINH loai tien nao duoc in ra. Hai trach nhiem khac
 * nhau, va cai thu hai moi la cho sai tien.
 *
 * Vi sao ham nay ton tai (spec §2.4): DESIGN.md chot dinh dang `1.250.000 đ`, nen
 * `formatVnd` hard-code hau to `đ`. Neu server tra `currency: "USD"` ma UI van in
 * `đ` thi do khong phai loi trinh bay — do la **noi sai so tien voi khach**. Ba
 * rang buoc doi khang duoc chot o day:
 *
 *   1. KHONG BIA KY HIEU. Tien khong phai VND chi duoc in SO + MA tien te. Khong
 *      duoc tu suy ra `$`, `¥`, `€`... (spec §7: can ky hieu rieng thi DUNG, hoi).
 *      Test kiem ca ky hieu cua dung tien te dang duoc in (USD -> khong co `$`).
 *
 *   2. THIEU `currency` KHONG DUOC THANH VND. Day la loi de xay ra nhat: `currency
 *      ?? "VND"`. Mot gia 50 USD in thanh "50 đ" thi khach thay re hon 25 nghin lan.
 *      Thieu nguon -> `"—"` (handoff §5 nguyen tac 3: thieu du lieu thi khong hien).
 *
 *   3. KHONG BAO GIO "NaN". Gia tri khong huu han -> `"—"`, o MOI loai tien te,
 *      khong phai "NaN đ" hay "NaN USD".
 *
 * Cach test: khong hard-code dau phan cach nghin cho tien te KHONG phai VND (spec
 * chi noi "so da nhom", khong chot dau). Test chot "cac chu so dung thu tu" bang
 * cach bo moi ky tu khong phai chu so roi so sanh — cach nay dung voi `1,250,000`
 * hay `1.250.000` deu duoc, nhung van bat duoc viec in sai so.
 */
import { describe, expect, it } from "vitest";

import { formatVnd } from "./format";
import { formatMoney } from "./money";

/** Chi lay cac chu so, de so sanh gia tri khong phu thuoc dau phan cach nghin. */
function digits(value: string): string {
  return value.replace(/\D/g, "");
}

/** Moi ky hieu tien te ma ham KHONG duoc tu bia ra (spec §2.4 va §7). */
const INVENTED_SYMBOLS = ["$", "¥", "€", "£", "₫", "₩", "₹"];

describe("formatMoney — VND (AC-1)", () => {
  it("(1250000, 'VND') -> '1.250.000 đ'", () => {
    expect(formatMoney(1250000, "VND")).toBe("1.250.000 đ");
  });

  it("(0, 'VND') -> '0 đ' (khong phai '—': 0 dong la mot gia hop le, ve mien phi)", () => {
    expect(formatMoney(0, "VND")).toBe("0 đ");
  });

  it("VND di qua dung `formatVnd`, khong tu cai dat lai cach nhom chu so", () => {
    for (const amount of [0, 1, 999, 1000, 1250000, 123456789]) {
      expect(formatMoney(amount, "VND")).toBe(formatVnd(amount));
    }
  });
});

describe("formatMoney — tien te khac VND: so + MA tien te, khong bia ky hieu (AC-1)", () => {
  it("(1250000, 'USD') KHONG chua 'đ'", () => {
    expect(formatMoney(1250000, "USD")).not.toContain("đ");
  });

  it("(1250000, 'USD') co chua ma tien te 'USD'", () => {
    expect(formatMoney(1250000, "USD")).toContain("USD");
  });

  it("(1250000, 'USD') giu dung cac chu so cua so tien", () => {
    expect(digits(formatMoney(1250000, "USD"))).toBe("1250000");
  });

  it("(1250000, 'USD') co nhom chu so (khong in lien mot khoi 7 chu so)", () => {
    // "So da nhom" theo spec §2.4. Khong chot DAU phan cach, chi chot la co nhom:
    // mot khoi >= 5 chu so lien tiep nghia la khong nhom gi ca.
    expect(formatMoney(1250000, "USD")).not.toMatch(/\d{5}/);
  });

  it("KHONG bia ky hieu tien te cho bat ky ma nao", () => {
    for (const currency of ["USD", "JPY", "EUR", "GBP", "KRW", "SGD"]) {
      const out = formatMoney(1250000, currency);

      expect(out).toContain(currency);
      expect(out).not.toContain("đ");
      for (const symbol of INVENTED_SYMBOLS) {
        expect(out).not.toContain(symbol);
      }
    }
  });

  it("(0, 'USD') van la mot gia hop le, khong phai '—'", () => {
    const out = formatMoney(0, "USD");

    expect(out).toContain("USD");
    expect(out).toContain("0");
    expect(out).not.toBe("—");
  });

  it("ma tien te la cua chinh caller, khong bi doi sang ma khac", () => {
    expect(formatMoney(1250000, "JPY")).not.toContain("USD");
    expect(formatMoney(1250000, "JPY")).not.toContain("VND");
  });
});

/**
 * Chuan hoa ma tien te — hop dong duoc chot SAU spec (spec §2.4 chi viet
 * `currency === "VND"`, nen `"vnd"` se roi sang nhanh "tien te khac" va in
 * `1.250.000 vnd`: dung ky thuat, sai nghiep vu).
 *
 * Ly do phai chuan hoa: ma tien te den tu JSON cua server va tu query string, hai
 * nguon khong cung quy uoc chu hoa. Mot don VND bi in `1.250.000 vnd` thi khach con
 * doc duoc, nhung mot don VND roi vao nhanh "khong phai VND" nghia la logic tien te
 * da re sai huong — va lan sau co the re sai o cho nguy hiem hon.
 */
describe("formatMoney — chuan hoa ma tien te truoc khi so sanh (AC-1, §2.4)", () => {
  it("'vnd' / 'Vnd' / ' VND ' cho ket qua Y HET 'VND'", () => {
    const expected = formatMoney(1250000, "VND");

    for (const currency of ["vnd", "Vnd", " VND ", "vND", "\tVND\n"]) {
      expect(formatMoney(1250000, currency)).toBe(expected);
    }
  });

  it("'vnd' in dung hau to 'đ' (khong roi sang nhanh ma tien te)", () => {
    expect(formatMoney(1250000, "vnd")).toBe("1.250.000 đ");
    expect(formatMoney(1250000, "vnd")).not.toContain("vnd");
  });

  it("'usd' in ma tien te VIET HOA", () => {
    const out = formatMoney(1250000, "usd");

    expect(out).toContain("USD");
    expect(out).not.toContain("usd");
    expect(out).not.toContain("đ");
  });

  it("' usd ' cho ket qua y het 'USD'", () => {
    expect(formatMoney(1250000, " usd ")).toBe(formatMoney(1250000, "USD"));
  });

  it("currency toan khoang trang -> coi nhu THIEU, tra '—'", () => {
    for (const currency of ["   ", "\t", "\n", " "]) {
      expect(formatMoney(1250000, currency)).toBe("—");
    }
  });
});

describe("formatMoney — thieu currency KHONG duoc mac dinh VND (AC-1, §2.4)", () => {
  it("currency = undefined -> '—'", () => {
    expect(formatMoney(1250000, undefined)).toBe("—");
  });

  it("currency = null -> '—'", () => {
    expect(formatMoney(1250000, null)).toBe("—");
  });

  it("currency = '' -> '—'", () => {
    expect(formatMoney(1250000, "")).toBe("—");
  });

  it("thieu currency: KHONG in 'đ' va KHONG in con so nao (so tran la lap lung)", () => {
    for (const currency of [undefined, null, ""]) {
      const out = formatMoney(1250000, currency);

      expect(out).not.toContain("đ");
      expect(out).not.toContain("VND");
      expect(out).not.toMatch(/\d/);
    }
  });

  it("thieu currency voi so tien 0 cung '—' (khong tron '0 đ')", () => {
    expect(formatMoney(0, undefined)).toBe("—");
  });
});

describe("formatMoney — gia tri khong huu han: '—', khong bao gio 'NaN' (AC-1)", () => {
  const BAD_AMOUNTS = [Number.NaN, Number.POSITIVE_INFINITY, Number.NEGATIVE_INFINITY];

  it("NaN / Infinity / -Infinity voi VND -> '—'", () => {
    for (const amount of BAD_AMOUNTS) {
      expect(formatMoney(amount, "VND")).toBe("—");
    }
  });

  it("NaN / Infinity / -Infinity voi USD -> '—' (khong phai 'NaN USD')", () => {
    for (const amount of BAD_AMOUNTS) {
      expect(formatMoney(amount, "USD")).toBe("—");
    }
  });

  it("khong bao gio xuat hien chuoi 'NaN' hay 'Infinity' trong ket qua", () => {
    for (const amount of BAD_AMOUNTS) {
      for (const currency of ["VND", "USD", "", undefined, null]) {
        const out = formatMoney(amount, currency);

        expect(out).not.toContain("NaN");
        expect(out).not.toContain("Infinity");
      }
    }
  });

  it("so am van ra mot chuoi co chu so, khong ra 'NaN'", () => {
    // Spec khong chot cach trinh bay so am (xem traceability muc "khong test duoc"),
    // nen chi chot phan BAT BUOC: khong duoc ro ri 'NaN' va khong duoc mat dau am.
    const out = formatMoney(-1250000, "VND");

    expect(out).not.toContain("NaN");
    expect(digits(out)).toBe("1250000");
    expect(out).toContain("-");
  });
});
