/**
 * Test cho tu dien i18n — EV-188 AC-5 (ha tang, en du khoa) va AC-8 (pham vi dich).
 *
 * Hop dong (plan §1.1 + §6):
 *   - `vi` la nguon; `en: Record<MessageKey, string>` phai co DUNG cac khoa cua `vi`
 *     (TypeScript chan khi viet literal; test nay chan ca truong hop ep kieu / spread).
 *   - Gia tri `vi` TRUNG NGUYEN VAN chuoi dang co trong component (test hien co giu nguyen).
 *   - `translate(locale, key, params)`: thay `{ten}` bang params.ten; thieu khoa o en -> roi ve vi;
 *     locale la -> vi.
 *
 * Test KHONG chot ten khoa (tru `skipLink.label` va `language.group` da ghi trong plan): khoa
 * duoc tim theo GIA TRI tieng Viet, de implementer con duoc chon cach dat ten.
 */
import { describe, expect, it } from "vitest";

import {
  DEFAULT_LOCALE,
  LOCALE_STORAGE_KEY,
  LOCALES,
  MESSAGES,
  en,
  isLocale,
  translate,
  vi as viMessages,
  type Locale,
  type MessageKey,
} from "./messages";

const VI_KEYS = Object.keys(viMessages).sort() as MessageKey[];

/** Moi khoa co gia tri vi dung bang `text`. */
function keysWithViValue(text: string): MessageKey[] {
  return VI_KEYS.filter((k) => viMessages[k] === text);
}

/** Ten cac tham so `{ten}` trong mot chuoi. */
function placeholders(text: string): string[] {
  return Array.from(text.matchAll(/\{(\w+)\}/g), (m) => m[1]).sort();
}

/** Chuoi tieng Viet dang co trong component thuoc pham vi AC-8 (doi chieu ma nguon tai 809e942).
 *  Moi chuoi phai la gia tri cua it nhat mot khoa vi — nguyen van, ke ca dau cau. */
const EXISTING_VI_STRINGS = [
  // components/layout/skip-link.tsx
  "Bỏ qua điều hướng, tới nội dung chính",
  // components/layout/public-header.tsx
  "Mở menu điều hướng",
  "Điều hướng chính",
  // components/layout/public-footer.tsx
  "EventFlow — nền tảng bán vé sự kiện chịu tải đột biến.",
  "Dự án học tập. Mọi số liệu trong giao diện là dữ liệu mẫu.",
  // components/layout/nav-config.ts (PUBLIC_NAV) + nav-link.tsx (badge)
  "Trang chủ",
  "Sự kiện",
  "Sắp có",
  // components/theme/theme-toggle.tsx
  "Chuyển giao diện sáng/tối",
  "Sáng",
  "Tối",
  // components/event/event-action-panel.tsx
  "Đã hết vé",
  "Sự kiện đã huỷ",
  "Đã đóng bán",
  "Đang điều phối lượt vào phòng chờ...",
  "Vui lòng giữ nguyên màn hình...",
  "Thời gian mở bán",
  "Chưa tới giờ mở bán",
  "Vào phòng chờ mua vé",
];

describe("messages — hang so (AC-5)", () => {
  it('LOCALES dung ["vi", "en"], mac dinh la "vi"', () => {
    expect([...LOCALES]).toEqual(["vi", "en"]);
    expect(DEFAULT_LOCALE).toBe("vi");
  });

  it('khoa localStorage la "eventflow-locale"', () => {
    expect(LOCALE_STORAGE_KEY).toBe("eventflow-locale");
  });

  it("MESSAGES tro dung tu dien vi va en", () => {
    expect(MESSAGES.vi).toEqual(viMessages);
    expect(MESSAGES.en).toEqual(en);
  });
});

describe("messages — en du khoa cua vi (AC-5)", () => {
  it("tu dien vi khong rong", () => {
    expect(VI_KEYS.length).toBeGreaterThan(0);
  });

  it("en co DUNG tap khoa cua vi: khong thieu, khong thua", () => {
    expect(Object.keys(en).sort()).toEqual(VI_KEYS);
  });

  it("khong co chuoi rong (hoac chi khoang trang) o vi", () => {
    const empty = VI_KEYS.filter((k) => typeof viMessages[k] !== "string" || viMessages[k].trim() === "");
    expect(empty).toEqual([]);
  });

  it("khong co chuoi rong (hoac chi khoang trang) o en", () => {
    const empty = VI_KEYS.filter((k) => typeof en[k] !== "string" || en[k].trim() === "");
    expect(empty).toEqual([]);
  });

  it("moi khoa co cung tap tham so {ten} o vi va en", () => {
    const mismatched = VI_KEYS.filter(
      (k) => placeholders(viMessages[k]).join(",") !== placeholders(en[k]).join(","),
    );
    expect(mismatched).toEqual([]);
  });

  it('khoa dat dang phang "nhom.ten"', () => {
    const bad = VI_KEYS.filter((k) => !/^[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)+$/.test(k));
    expect(bad).toEqual([]);
  });
});

describe("messages — gia tri vi trung nguyen van chuoi hien co (AC-8, AC-10)", () => {
  it.each(EXISTING_VI_STRINGS)("co khoa vi = %j", (text) => {
    expect(keysWithViValue(text).length).toBeGreaterThan(0);
  });

  it("chuoi da chuyen vao tu dien thuc su duoc dich sang en (khac vi)", () => {
    const untranslated = EXISTING_VI_STRINGS.filter((text) =>
      keysWithViValue(text).some((k) => en[k] === viMessages[k]),
    );
    expect(untranslated).toEqual([]);
  });

  it('skipLink.label la chuoi skip link hien co', () => {
    expect(viMessages["skipLink.label" as MessageKey]).toBe("Bỏ qua điều hướng, tới nội dung chính");
  });

  it('language.group: "Ngôn ngữ" / "Language" (plan §6 M1)', () => {
    expect(viMessages["language.group" as MessageKey]).toBe("Ngôn ngữ");
    expect(en["language.group" as MessageKey]).toBe("Language");
  });

  it("panel jitter co chuoi dem nguoc co tham so (Con ...s (giam tai dot bien))", () => {
    const withParams = VI_KEYS.filter((k) => placeholders(viMessages[k]).length > 0);
    expect(withParams.some((k) => /^Còn \{\w+\}s \(giảm tải đột biến\)$/.test(viMessages[k]))).toBe(true);
  });
});

describe("translate", () => {
  it("vi tra dung gia tri vi, en tra dung gia tri en cho moi khoa", () => {
    for (const k of VI_KEYS) {
      if (placeholders(viMessages[k]).length > 0) continue;
      expect(translate("vi", k)).toBe(viMessages[k]);
      expect(translate("en", k)).toBe(en[k]);
    }
  });

  it("thay {ten} bang params (chuoi va so), o ca vi va en", () => {
    const withParams = VI_KEYS.filter((k) => placeholders(viMessages[k]).length > 0);
    expect(withParams.length, "can it nhat mot khoa co tham so de kiem interpolation").toBeGreaterThan(0);

    for (const k of withParams) {
      const names = placeholders(viMessages[k]);
      const params: Record<string, string | number> = {};
      names.forEach((name, i) => {
        params[name] = i % 2 === 0 ? 4.5 : `GT${i}`;
      });

      for (const locale of LOCALES) {
        const out = translate(locale, k, params);
        expect(out, `${locale}/${k} con sot {…}`).not.toMatch(/\{\w+\}/);
        for (const name of names) {
          expect(out).toContain(String(params[name]));
        }
      }
    }
  });

  it("thay MOI lan xuat hien cua cung mot tham so, phan con lai giu nguyen", () => {
    const k = VI_KEYS.find((key) => placeholders(viMessages[key]).length > 0);
    expect(k).toBeDefined();
    const key = k as MessageKey;
    const names = placeholders(viMessages[key]);
    const params = Object.fromEntries(names.map((n) => [n, "X"]));
    const expected = viMessages[key].replace(/\{(\w+)\}/g, "X");
    expect(translate("vi", key, params)).toBe(expected);
  });

  it("params thua (khong co trong chuoi) bi bo qua", () => {
    const k = VI_KEYS.find((key) => placeholders(viMessages[key]).length === 0) as MessageKey;
    expect(translate("vi", k, { khongDung: "zzz" })).toBe(viMessages[k]);
  });

  it("thieu khoa o en -> roi ve vi", () => {
    const k = VI_KEYS.find((key) => placeholders(viMessages[key]).length === 0) as MessageKey;
    const dict = MESSAGES.en as Record<string, string>;
    const saved = dict[k];
    delete dict[k];
    try {
      expect(translate("en", k)).toBe(viMessages[k]);
    } finally {
      dict[k] = saved;
    }
    expect(translate("en", k)).toBe(en[k]);
  });

  it("locale la -> dung vi", () => {
    for (const k of VI_KEYS) {
      if (placeholders(viMessages[k]).length > 0) continue;
      expect(translate("fr" as Locale, k)).toBe(viMessages[k]);
      expect(translate(undefined as unknown as Locale, k)).toBe(viMessages[k]);
    }
  });
});

describe("isLocale", () => {
  it.each(["vi", "en"])("%j la locale hop le", (value) => {
    expect(isLocale(value)).toBe(true);
  });

  it.each([["fr"], ["VI"], ["EN"], ["en-US"], [""], [" vi"], [null], [undefined], [1], [{}], [["vi"]]])(
    "%j KHONG phai locale",
    (value) => {
      expect(isLocale(value)).toBe(false);
    },
  );
});

// ---------------------------------------------------------------------------
// EV-188 code review m4, m5 (plan §7).
// ---------------------------------------------------------------------------
describe("translate — chi dung tham so la thuoc tinh RIENG cua params (EV-188 m5)", () => {
  function keyWithParam(): { key: MessageKey; name: string } {
    const key = VI_KEYS.find((k) => placeholders(viMessages[k]).length > 0) as MessageKey;
    return { key, name: placeholders(viMessages[key])[0] };
  }

  it("thuoc tinh ke thua tu prototype KHONG duoc thay vao chuoi", () => {
    const { key, name } = keyWithParam();
    const inherited = Object.create({ [name]: "KE-THUA" }) as Record<string, string>;
    const out = translate("vi", key, inherited);
    expect(out).not.toContain("KE-THUA");
    expect(out).toContain(`{${name}}`);
  });

  it("thuoc tinh cua Object.prototype (toString, constructor...) khong duoc coi la tham so", () => {
    const { key } = keyWithParam();
    const out = translate("vi", key, {});
    expect(out).not.toMatch(/function|\[object/);
    expect(out).toBe(viMessages[key]);
  });

  it("thieu tham so: giu nguyen {ten} (khong ra 'undefined')", () => {
    const { key, name } = keyWithParam();
    const out = translate("en", key, {});
    expect(out).not.toContain("undefined");
    expect(out).toContain(`{${name}}`);
  });
});

describe('ban dich en cua skip link (EV-188 m4)', () => {
  it('skipLink.label en = "Skip to main content"', () => {
    expect(en["skipLink.label" as MessageKey]).toBe("Skip to main content");
  });
});
