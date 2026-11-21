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
import { describe, expect, it, vi } from "vitest";

import {
  formatClock,
  formatCount,
  formatDateTimeVn,
  formatDecimal,
  formatTimeVn,
  formatVnd,
  spellDuration,
} from "./format";

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

/**
 * `spellDuration` la TOAN BO thong tin ve dong ho ma nguoi dung screen reader
 * nghe duoc (vung chu so bi aria-hidden), cong chuoi ETA cua trang thai QUEUED.
 * Truoc do no khong co mot test truc tiep nao: ai "don gian hoa" bo phan giay di
 * thi con 1 phut 59 giay se duoc doc thanh "con 1 phut" — sai 59 giay tren dong
 * ho giu ghe — ma ca bo test van xanh. Bang duoi day la chot do.
 */
describe("spellDuration — doc thanh chu cho screen reader (AC-5)", () => {
  it.each([
    [1_000, "1 giây"],
    [59_000, "59 giây"],
    [60_000, "1 phút"],
    [90_000, "1 phút 30 giây"],
    [119_000, "1 phút 59 giây"],
    [570_000, "9 phút 30 giây"],
    [600_000, "10 phút"],
    [3_600_000, "1 giờ"],
    [3_660_000, "1 giờ 1 phút"],
    [3_661_000, "1 giờ 1 phút 1 giây"],
  ])("spellDuration(%i) = '%s'", (ms, expected) => {
    expect(spellDuration(ms as number)).toBe(expected);
  });

  it("KHONG doc don vi bang 0 (60_000 la '1 phút', khong phai '1 phút 0 giây')", () => {
    expect(spellDuration(60_000)).not.toContain("0 giây");
    expect(spellDuration(3_600_000)).not.toContain("0 phút");
    expect(spellDuration(3_600_000)).not.toContain("0 giây");
    expect(spellDuration(3_660_000)).not.toContain("0 giây");
  });

  it("GIU phan giay: 1 phut 59 giay khong duoc rut gon thanh '1 phút'", () => {
    expect(spellDuration(119_000)).toContain("59");
    expect(spellDuration(119_000)).toContain("giây");
  });

  it("lam tron XUONG giong formatClock (1999ms -> '1 giây')", () => {
    expect(spellDuration(1_999)).toBe("1 giây");
    expect(spellDuration(59_999)).toBe("59 giây");
  });

  it("khop voi formatClock tren cung mot gia tri", () => {
    expect(formatClock(570_000)).toBe("09:30");
    expect(spellDuration(570_000)).toBe("9 phút 30 giây");
  });

  it("duoi 1 giay: 0 va 999 cho ra cung mot chuoi, khong hua them thoi gian", () => {
    expect(spellDuration(999)).toBe(spellDuration(0));
    expect(spellDuration(999)).not.toContain("1 giây");
    expect(spellDuration(0).length).toBeGreaterThan(0);
  });

  it("so am khong lot dau tru ra UI", () => {
    expect(spellDuration(-1_000)).not.toContain("-");
    expect(spellDuration(-600_000)).not.toContain("-");
    expect(() => spellDuration(-1)).not.toThrow();
  });

  it.each([Number.NaN, Number.POSITIVE_INFINITY, Number.NEGATIVE_INFINITY])(
    "gia tri khong huu hien (%s): khong throw, khong lot 'NaN'/'Infinity' ra UI",
    (ms) => {
      expect(() => spellDuration(ms)).not.toThrow();
      const out = spellDuration(ms);
      expect(typeof out).toBe("string");
      expect(out).not.toContain("NaN");
      expect(out).not.toContain("Infinity");
    },
  );
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

/* EV-187 (plan §6 m-1, M-1): formatCount + formatTimeVn cho war room Ops.
 * Ca hai tinh tay, khong toLocale* / ICU, de server va client render giong nhau
 * (tranh loi hydration) va ket qua xac dinh tren moi may. */
describe("formatCount — dấu chấm ngăn nghìn, tính tay (EV-187 m-1)", () => {
  it("số nhỏ hơn 1000 giữ nguyên", () => {
    expect(formatCount(0)).toBe("0");
    expect(formatCount(7)).toBe("7");
    expect(formatCount(999)).toBe("999");
  });

  it("chèn dấu chấm ngăn nghìn", () => {
    expect(formatCount(1000)).toBe("1.000");
    expect(formatCount(4210)).toBe("4.210");
    expect(formatCount(1234567)).toBe("1.234.567");
  });

  it("không dùng dấu phẩy kiểu en-US", () => {
    expect(formatCount(1234567)).not.toContain(",");
  });

  it("giá trị không hữu hạn trả '—'", () => {
    expect(formatCount(Number.NaN)).toBe("—");
    expect(formatCount(Number.POSITIVE_INFINITY)).toBe("—");
    expect(formatCount(Number.NEGATIVE_INFINITY)).toBe("—");
  });
});

describe("formatTimeVn — HH:MM:SS theo UTC+7 tính tay (EV-187 M-1)", () => {
  it("ISO UTC được đổi sang giờ Việt Nam", () => {
    expect(formatTimeVn("2026-10-13T03:00:00.000Z")).toBe("10:00:00");
  });

  it("đệm số 0 cho giờ/phút/giây một chữ số", () => {
    expect(formatTimeVn("2026-10-13T01:02:03Z")).toBe("08:02:03");
  });

  it("qua nửa đêm theo UTC+7 thì quay về 00", () => {
    expect(formatTimeVn("2026-10-13T17:00:00Z")).toBe("00:00:00");
    expect(formatTimeVn("2026-10-13T20:05:09Z")).toBe("03:05:09");
  });

  it("ISO có offset +07:00 giữ đúng giờ địa phương", () => {
    expect(formatTimeVn("2026-10-13T10:15:30+07:00")).toBe("10:15:30");
  });

  it("không phụ thuộc múi giờ của máy (không dùng toLocale*)", () => {
    const spy = vi.spyOn(Date.prototype, "toLocaleTimeString");
    const spyString = vi.spyOn(Date.prototype, "toLocaleString");
    formatTimeVn("2026-10-13T03:00:00.000Z");
    expect(spy).not.toHaveBeenCalled();
    expect(spyString).not.toHaveBeenCalled();
    spy.mockRestore();
    spyString.mockRestore();
  });

  it("ISO hỏng hoặc rỗng trả '--:--:--'", () => {
    expect(formatTimeVn("khong-phai-ngay")).toBe("--:--:--");
    expect(formatTimeVn("")).toBe("--:--:--");
  });
});

/* EV-187 code review vòng 1 (plan §7 M-1, m-1, t-4, t-5). */
describe("formatDecimal — chấm ngăn nghìn, phẩy thập phân (EV-187 §7 M-1)", () => {
  it("một chữ số thập phân mặc định", () => {
    expect(formatDecimal(12.7)).toBe("12,7");
    expect(formatDecimal(5)).toBe("5,0");
  });

  it("làm tròn và chèn dấu chấm ngăn nghìn", () => {
    expect(formatDecimal(1234.56)).toBe("1.234,6");
    expect(formatDecimal(1234567.89)).toBe("1.234.567,9");
  });

  it("tham số digits", () => {
    expect(formatDecimal(3.14159, 2)).toBe("3,14");
    expect(formatDecimal(1234.5, 0)).not.toContain(",");
  });

  it("-0 và giá trị làm tròn về 0 trả '0,0' (không có '-0,0')", () => {
    expect(formatDecimal(-0)).toBe("0,0");
    expect(formatDecimal(0)).toBe("0,0");
    expect(formatDecimal(0.04)).toBe("0,0");
    expect(formatDecimal(-0.04)).toBe("0,0");
  });

  it("số âm giữ dấu trừ", () => {
    expect(formatDecimal(-12.7)).toBe("-12,7");
    expect(formatDecimal(-1234.56)).toBe("-1.234,6");
  });

  it("giá trị không hữu hạn trả '—'", () => {
    expect(formatDecimal(Number.NaN)).toBe("—");
    expect(formatDecimal(Number.POSITIVE_INFINITY)).toBe("—");
    expect(formatDecimal(Number.NEGATIVE_INFINITY)).toBe("—");
  });

  it("không dùng toLocaleString (không phụ thuộc ICU)", () => {
    const spy = vi.spyOn(Number.prototype, "toLocaleString");
    formatDecimal(1234.56);
    expect(spy).not.toHaveBeenCalled();
    spy.mockRestore();
  });
});

describe("formatCount — số âm, -0, số rất lớn (EV-187 §7 M-1, t-4)", () => {
  it("-0 trả '0' (không có '-0')", () => {
    expect(formatCount(-0)).toBe("0");
  });

  it("số âm giữ dấu trừ và ngăn nghìn", () => {
    expect(formatCount(-1234)).toBe("-1.234");
    expect(formatCount(-7)).toBe("-7");
  });

  it("số rất lớn vẫn ngăn nghìn đúng", () => {
    expect(formatCount(1_000_000_000_000)).toBe("1.000.000.000.000");
    expect(formatCount(9_007_199_254_740_991)).toBe("9.007.199.254.740.991");
  });
});

describe("formatTimeVn — chỉ nhận ISO có Z hoặc offset ±HH:MM (EV-187 §7 m-1, t-5)", () => {
  it("ISO không có Z/offset trả '--:--:--' (không đoán theo múi giờ máy)", () => {
    expect(formatTimeVn("2026-10-13T03:00:00")).toBe("--:--:--");
    expect(formatTimeVn("2026-10-13T03:00:00.000")).toBe("--:--:--");
  });

  it("chỉ có ngày trả '--:--:--'", () => {
    expect(formatTimeVn("2026-10-13")).toBe("--:--:--");
  });

  it("offset âm vẫn đổi đúng sang UTC+7", () => {
    expect(formatTimeVn("2026-10-13T03:00:00-05:00")).toBe("15:00:00");
  });
});

/* EV-187 challenge (plan §8 C-3, C-6). */
describe("formatCount / formatDecimal — vượt MAX_SAFE_INTEGER trả '—' (C-3)", () => {
  it("formatCount", () => {
    expect(formatCount(Number.MAX_SAFE_INTEGER + 1)).toBe("—");
    expect(formatCount(-(Number.MAX_SAFE_INTEGER + 1))).toBe("—");
    expect(formatCount(1e21)).toBe("—");
  });

  it("formatDecimal", () => {
    expect(formatDecimal(Number.MAX_SAFE_INTEGER + 1)).toBe("—");
    expect(formatDecimal(-1e20)).toBe("—");
  });

  it("đúng MAX_SAFE_INTEGER vẫn định dạng được", () => {
    expect(formatCount(Number.MAX_SAFE_INTEGER)).toBe("9.007.199.254.740.991");
  });
});

describe("mutant sống M24, M28 (C-6)", () => {
  it("formatCount(-0.5) trả '0' (không '-0')", () => {
    expect(formatCount(-0.5)).toBe("0");
  });

  it("formatTimeVn('xZ') — đúng đuôi Z nhưng ngày hỏng — trả '--:--:--'", () => {
    expect(formatTimeVn("xZ")).toBe("--:--:--");
    expect(formatTimeVn("2026-13-45T99:00:00Z")).toBe("--:--:--");
  });
});

describe("formatDateTimeVn — dd/MM/yyyy HH:mm theo gio Viet Nam (EV-121)", () => {
  it("doi UTC sang UTC+7, qua ngay dung", () => {
    expect(formatDateTimeVn("2026-10-20T17:30:00Z")).toBe("21/10/2026 00:30");
  });

  it("nhan offset khac", () => {
    expect(formatDateTimeVn("2026-10-20T08:00:00+07:00")).toBe("20/10/2026 08:00");
  });

  it("thieu mui gio hoac hong -> ky hieu trong, khong doan", () => {
    expect(formatDateTimeVn("2026-10-20T08:00:00")).toBe("--/--/---- --:--");
    expect(formatDateTimeVn("")).toBe("--/--/---- --:--");
    expect(formatDateTimeVn("rac")).toBe("--/--/---- --:--");
  });
});
