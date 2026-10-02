/**
 * Test cho `formatClock` + `formatVnd` — AC-1, AC-2 (spec-plan EVF-1803 §4).
 *
 * Hai cho de sai nhat, ca hai deu la loi "noi doi nguoi dung":
 *
 *   1. LAM TRON LEN. Con 1500ms ma hien "00:02" la hua them thoi gian khong
 *      ton tai. Dung luc nay la luc khach dang bam thanh toan -> mat ve ma
 *      tuong con kip. AC-1 chot `floor`. Test moc thang vao 1500 / 1999.
 *   2. So am dem len. Het han roi thi phai la "00:00", khong duoc ra "-00:05"
 *      (BR-O2: client khong tu suy dien thoi han, het la het).
 *
 * `formatVnd` KHONG duoc phu thuoc ICU locale data: Node build thieu full-icu
 * se cho ra dau phan cach khac (`1,250,000`). Vi vay test chot CHUOI CHINH XAC
 * `"1.250.000 đ"` (DESIGN.md §Typography muc 2) chu khong chot "co dau cham".
 */
import { describe, expect, it } from "vitest";

import { formatClock, formatVnd } from "./format";

describe("formatClock — dang mm:ss (AC-1)", () => {
  it.each([
    [0, "00:00"],
    [1_000, "00:01"],
    [59_000, "00:59"],
    [60_000, "01:00"],
    [90_000, "01:30"],
    [600_000, "10:00"],
    [3_599_000, "59:59"],
  ])("formatClock(%i) = %s", (ms, expected) => {
    expect(formatClock(ms)).toBe(expected);
  });

  it("luon du 2 chu so cho phut va giay (khong ra '0:1')", () => {
    expect(formatClock(1_000)).toMatch(/^\d{2}:\d{2}$/);
    expect(formatClock(600_000)).toMatch(/^\d{2}:\d{2}$/);
  });
});

describe("formatClock — lam tron XUONG, khong bao gio hien nhieu hon thuc te (AC-1)", () => {
  it("1500ms ra '00:01', KHONG phai '00:02'", () => {
    expect(formatClock(1_500)).toBe("00:01");
  });

  it("1999ms ra '00:01'", () => {
    expect(formatClock(1_999)).toBe("00:01");
  });

  it("999ms ra '00:00' (chua du 1 giay thi khong duoc lam thanh 1 giay)", () => {
    expect(formatClock(999)).toBe("00:00");
  });

  it("59_999ms ra '00:59', KHONG phai '01:00'", () => {
    expect(formatClock(59_999)).toBe("00:59");
  });

  it("599_500ms ra '09:59', KHONG phai '10:00'", () => {
    expect(formatClock(599_500)).toBe("09:59");
  });

  it("3_599_999ms van la mm:ss va ra '59:59', chua nhay sang gio", () => {
    expect(formatClock(3_599_999)).toBe("59:59");
  });
});

describe("formatClock — dang h:mm:ss khi >= 1 gio (AC-1)", () => {
  it.each([
    [3_600_000, "1:00:00"],
    [3_661_000, "1:01:01"],
    [7_322_000, "2:02:02"],
    [36_000_000, "10:00:00"],
  ])("formatClock(%i) = %s", (ms, expected) => {
    expect(formatClock(ms)).toBe(expected);
  });

  it("3_599_999 khong co gio nhung 3_600_000 thi co", () => {
    expect(formatClock(3_599_999)).not.toContain(":00:");
    expect(formatClock(3_600_000).split(":")).toHaveLength(3);
  });

  it("vua qua moc 1 gio van lam tron xuong", () => {
    expect(formatClock(3_600_999)).toBe("1:00:00");
  });
});

describe("formatClock — so am ve 00:00, khong dem len (AC-1)", () => {
  it.each([-1, -999, -1_000, -600_000, -3_600_000])("formatClock(%i) = '00:00'", (ms) => {
    expect(formatClock(ms)).toBe("00:00");
  });

  it("khong bao gio co dau tru trong ket qua", () => {
    expect(formatClock(-600_000)).not.toContain("-");
    expect(formatClock(-1)).not.toContain("-");
  });
});

describe("formatClock — gia tri khong huu hien (AC-1)", () => {
  it.each([
    [Number.NaN, "NaN"],
    [Number.POSITIVE_INFINITY, "Infinity"],
    [Number.NEGATIVE_INFINITY, "-Infinity"],
  ])("tra '00:00' voi %s", (ms) => {
    expect(formatClock(ms as number)).toBe("00:00");
  });

  it("khong throw voi NaN / Infinity", () => {
    expect(() => formatClock(Number.NaN)).not.toThrow();
    expect(() => formatClock(Number.POSITIVE_INFINITY)).not.toThrow();
  });

  it("khong bao gio lot chuoi 'NaN' ra UI", () => {
    expect(formatClock(Number.NaN)).not.toContain("NaN");
    expect(formatClock(Number.POSITIVE_INFINITY)).not.toContain("Infinity");
  });
});

describe("formatVnd — dinh dang tien VND (AC-2)", () => {
  it("1250000 ra dung chuoi '1.250.000 đ' (khong phu thuoc locale ICU)", () => {
    expect(formatVnd(1_250_000)).toBe("1.250.000 đ");
  });

  it.each([
    [0, "0 đ"],
    [999, "999 đ"],
    [1_000, "1.000 đ"],
    [10_000, "10.000 đ"],
    [100_000, "100.000 đ"],
    [1_000_000, "1.000.000 đ"],
    [20_000_000, "20.000.000 đ"],
  ])("formatVnd(%i) = %s", (amount, expected) => {
    expect(formatVnd(amount)).toBe(expected);
  });

  it("khong dung dau phay lam dau phan cach nghin", () => {
    expect(formatVnd(1_250_000)).not.toContain(",");
  });

  it("co hau to 'đ'", () => {
    expect(formatVnd(1_250_000).endsWith("đ")).toBe(true);
  });
});

describe("formatVnd — so am (AC-2)", () => {
  it("co dau tru dung truoc", () => {
    const out = formatVnd(-1_250_000);
    expect(out.startsWith("-")).toBe(true);
    expect(out).toContain("1.250.000");
  });

  it("-1000 van giu dau phan cach nghin", () => {
    expect(formatVnd(-1_000)).toBe("-1.000 đ");
  });
});

describe("formatVnd — gia tri khong huu hien tra '—' (AC-2)", () => {
  it.each([Number.NaN, Number.POSITIVE_INFINITY, Number.NEGATIVE_INFINITY])(
    "tra em dash voi %s",
    (amount) => {
      expect(formatVnd(amount)).toBe("—");
    },
  );

  it("khong throw va KHONG hien 'NaN đ'", () => {
    expect(() => formatVnd(Number.NaN)).not.toThrow();
    expect(formatVnd(Number.NaN)).not.toContain("NaN");
    expect(formatVnd(Number.POSITIVE_INFINITY)).not.toContain("Infinity");
  });
});
