/**
 * Test cho `OrderSummary` — AC-8, AC-9, AC-10 (spec-plan EVF-1802/1804 §4),
 * BR-O1..O5, BR-Q7, handoff §11.6.
 *
 * ==> HAI TEST QUAN TRONG NHAT:
 *     - describe "fees" (AC-8): KHONG duoc render dong phi bang 0.
 *     - describe "ambiguous" (AC-9): KHONG duoc noi thanh cong, khong duoc noi that
 *       bai, khong duoc moi bam lai.
 *
 * 1) KHONG BIA TONG (handoff §11.6: "Display fees/taxes only if supplied. Do not …
 *    invent totals."). Dong "Phi: 0 đ" KHONG phai la hien thi trung tinh — do la mot
 *    KHANG DINH rang don nay khong co phi, ma UI khong he biet dieu do: phi do server
 *    tinh va co the chua tinh xong. Khach doc "0 đ" roi bi tru them 20 nghin o buoc
 *    sau la mot cam giac bi lua, va la mot tranh chap thuc. Vi vay: thieu `fees` ->
 *    khong co DONG phi nao, va `total === subtotal`.
 *    Cung ly do: `items` rong -> KHONG duoc hien "0 đ" nhu mot tong hop le.
 *    Va: tron nhieu `currency` trong `items` -> hien LOI, khong duoc cong lan; cong
 *    100.000 VND voi 50.000 USD ra 150.000 la mot con so vo nghia nhung trong rat that.
 *
 * 2) TRANG THAI "KHONG RO KET QUA" LA TRANG THAI THAT (BR-O5, docs/01 dong 88: moi API
 *    ghi bat buoc `Idempotency-Key`, "goi lai cung key trong 24h tra dung response
 *    cu"). Idempotency ton tai CHINH VI request co the khong biet ket qua. Khi do:
 *      - noi "da giu duoc ve" la co the sai -> khach tuong minh an chac roi roi di mat.
 *      - noi "that bai" cung co the sai -> khach di mua lai o cho khac trong khi ve
 *        cua minh dang bi giu, roi het han vo ich.
 *      - moi bam lai la te nhat: bam lai sinh key MOI -> hai hold cho cung mot khach,
 *        va BR-O3 noi hold moi HUY hold cu, nen ket qua phu thuoc thu tu toi dich.
 *    Test doi khang: truyen `primaryActionLabel` + `onPrimaryAction` vao trang thai
 *    `ambiguous` roi bam HET moi nut va bat buoc `onPrimaryAction` khong he duoc goi.
 *
 * 3) BR-Q7 / handoff §3 — HAI DONG HO CHECKOUT KHONG DUOC LAN. Suat admit co TTL 15
 *    phut, hold ve 10 phut. `OrderSummary` chi duoc dung variant `"hold"`. Test khong
 *    hard-code cau chu: no render `ServerExpiryCountdown` cua ca hai variant de LAY ra
 *    nhan thuc te, roi bat buoc text cua summary chua nhan `hold` va KHONG chua nhan
 *    `admission`.
 *
 * Luu y ky thuat: `expiresAt` dang chuoi BAT BUOC co mui gio (`toEpochMs` tu choi
 * chuoi khong co `Z`/offset), nen moi moc o day la epoch ms hoac `toISOString()`.
 */
import { act, fireEvent, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ServerExpiryCountdown, type CountdownVariant } from "../ui/server-expiry-countdown";
import {
  OrderSummary,
  type HoldState,
  type OrderSummaryItem,
  type OrderSummaryProps,
} from "./order-summary";

const START = Date.UTC(2026, 0, 15, 3, 0, 0);
const HOLD_EXPIRES_AT = START + 570_000; // con 09:30

const HOLD_STATES: HoldState[] = [
  "idle",
  "creating",
  "active",
  "expired",
  "sold_out",
  "ambiguous",
];

const ITEM_A: OrderSummaryItem = {
  tierName: "Hang A",
  quantity: 2,
  unitAmount: 100000,
  currency: "VND",
};

const ITEM_B: OrderSummaryItem = {
  tierName: "Hang B",
  quantity: 1,
  unitAmount: 250000,
  currency: "VND",
};

const ITEMS = [ITEM_A, ITEM_B];

const UNIT_A = "100.000 đ";
const LINE_A = "200.000 đ"; // 100.000 x 2
const LINE_B = "250.000 đ";
const SUBTOTAL = "450.000 đ"; // 200.000 + 250.000
const FEES = 30000;
const FEES_TEXT = "30.000 đ";
const TOTAL_WITH_FEES = "480.000 đ";

const PRIMARY_LABEL = "Thanh toan ngay";

const BASE: OrderSummaryProps = {
  items: ITEMS,
  holdState: "idle",
};

function norm(value: string | null | undefined): string {
  return (value ?? "").replace(/\s+/g, " ").trim();
}

/** Text node + moi `aria-label`: toan bo chu nguoi dung hoac screen reader nhan duoc. */
function allText(container: HTMLElement): string {
  const labels = Array.from(container.querySelectorAll("[aria-label]"))
    .map((el) => el.getAttribute("aria-label") ?? "")
    .join(" ");
  return norm(`${container.textContent ?? ""} ${labels}`);
}

/** Bo chu so va dau cham cau -> con phan CHU, de so sanh nhan khong phu thuoc dong ho. */
function words(value: string | null | undefined): string {
  return norm(value)
    .replace(/[\d:/.,%-]+/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

/**
 * Noi dung cua TUNG text node rieng le (cong moi `aria-label`).
 *
 * `textContent` cua ca cay noi cac node lien nhau KHONG chen khoang trang, nen tren
 * chuoi gop:
 *   - `/\bUSD\b/` TRUOT, vi sau "USD" la chu dau cua phan tu ke ben ("...USDChon...")
 *     va `D` voi `C` deu la word char -> khong co ranh gioi tu.
 *   - `/\d\s*đ/` KHOP OAN vao "Chi tiet đơn hàng" neu truoc do co mot chu so.
 * Mot text node luon la mot doan lien tuc, nen khong co hien tuong dinh chu. Moi rang
 * buoc ve "co / khong co SO TIEN" o file nay deu chay tren tung doan, khong tren chuoi gop.
 */
function textRuns(container: HTMLElement): string[] {
  const runs: string[] = [];

  const walk = (node: Node) => {
    if (node.nodeType === 3) {
      const text = norm(node.textContent);
      if (text) runs.push(text);
      return;
    }
    for (const child of Array.from(node.childNodes)) walk(child);
  };
  walk(container);

  for (const el of Array.from(container.querySelectorAll("[aria-label]"))) {
    const label = norm(el.getAttribute("aria-label"));
    if (label) runs.push(label);
  }
  return runs;
}

/** Co doan text nao chua mot SO TIEN khop `pattern` khong? */
function hasMoneyRun(container: HTMLElement, pattern: RegExp): boolean {
  return textRuns(container).some((run) => pattern.test(run));
}

/**
 * Mot SO TIEN bat ky: chu so, roi hau to `đ` hoac mot ma tien te 3 chu in.
 *
 * `(?!\p{L})` CHI dat sau `đ` (de loai "2 đơn", "đã"), KHONG dat sau ma tien te: mot ma
 * nam cuoi text node co the duoc theo ngay bang chu dau cua node ke ben, va lookahead o
 * do se lam ca assertion truot.
 */
const MONEY = /\d[\d.,]*\s*(?:đ(?!\p{L})|[A-Z]{3})/u;

/** Rieng hau to VND: dung de bat viec in `đ` cho tien KHONG phai VND. */
const VND_SUFFIX = /\d[\d.,]*\s*đ(?!\p{L})/u;

function buttons(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>('button, [role="button"]'));
}

function accName(el: Element): string {
  return norm(el.getAttribute("aria-label") ?? el.textContent);
}

/** Nut co phai nut hanh dong chinh duoc truyen vao khong (chap nhan co kem icon). */
function isPrimary(el: Element): boolean {
  return accName(el).includes(PRIMARY_LABEL);
}

function isDisabled(el: Element): boolean {
  return (
    (el as HTMLButtonElement).disabled === true || el.getAttribute("aria-disabled") === "true"
  );
}

function enabledButtons(container: HTMLElement): HTMLElement[] {
  return buttons(container).filter((button) => !isDisabled(button));
}

/** Bam het moi nut — dung de chung minh KHONG co duong nao kich hoat hanh dong chinh. */
function clickEveryButton(container: HTMLElement): void {
  for (const button of buttons(container)) {
    fireEvent.click(button);
  }
}

/** Nhan THUC TE cua `ServerExpiryCountdown` cho mot variant (khong hard-code cau chu). */
function countdownLabel(variant: CountdownVariant): string {
  const { container, unmount } = render(
    <ServerExpiryCountdown variant={variant} expiresAt={HOLD_EXPIRES_AT} />,
  );
  const label = words(container.textContent);
  unmount();
  return label;
}

/** Phan chu RIENG cua mot trang thai hold: bo ten hang + moi chuoi tien. */
function holdWords(state: HoldState, extra?: Partial<OrderSummaryProps>): string {
  const { container, unmount } = render(
    <OrderSummary
      {...BASE}
      holdState={state}
      holdExpiresAt={HOLD_EXPIRES_AT}
      primaryActionLabel={PRIMARY_LABEL}
      {...extra}
    />,
  );
  const text = words(allText(container))
    .replace(words(ITEM_A.tierName), " ")
    .replace(words(ITEM_B.tierName), " ")
    .replace(/\s+/g, " ")
    .trim();
  unmount();
  return text;
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(START);
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("OrderSummary — dong tien cua tung hang ve (AC-8)", () => {
  it("moi dong hien ten hang, so luong, don gia va tong dong", () => {
    const { container } = render(<OrderSummary {...BASE} />);
    const text = allText(container);

    expect(text).toContain(ITEM_A.tierName);
    expect(text).toContain(ITEM_B.tierName);
    expect(text).toContain("2"); // so luong hang A
    expect(text).toContain(UNIT_A); // don gia hang A
    expect(text).toContain(LINE_A); // tong dong hang A
    expect(text).toContain(LINE_B);
  });

  it("so luong cua tung dong den duoc screen reader kem ten hang", () => {
    const { container } = render(
      <OrderSummary items={[{ ...ITEM_A, quantity: 3 }]} holdState="idle" />,
    );

    expect(allText(container)).toContain("3");
    expect(allText(container)).toContain(ITEM_A.tierName);
  });

  it("subtotal = Σ(unitAmount × quantity)", () => {
    const { container } = render(<OrderSummary {...BASE} />);

    expect(allText(container)).toContain(SUBTOTAL);
  });

  it("subtotal dung voi mot dong duy nhat", () => {
    const { container } = render(<OrderSummary items={[ITEM_A]} holdState="idle" />);

    expect(allText(container)).toContain(LINE_A);
  });

  it("moi so tien di qua formatMoney voi dung `currency` cua item (USD -> khong co 'đ')", () => {
    const { container } = render(
      <OrderSummary
        items={[
          { tierName: "Hang A", quantity: 2, unitAmount: 100, currency: "USD" },
          { tierName: "Hang B", quantity: 1, unitAmount: 50, currency: "USD" },
        ]}
        holdState="idle"
      />,
    );
    const text = allText(container);

    // Dieu phai bao ve: KHONG mot so tien nao duoc mang hau to `đ` khi currency la USD.
    // Kiem tren tung text node (xem `textRuns`): tren chuoi gop thi `/\bUSD\b/` truot vi
    // "USD" bi chu dau cua phan tu ke ben noi vao ("...500.000 USDChon..."), va ky tu `đ`
    // cua "đơn hàng"/"đã" thi khong phai hau to tien te.
    expect(hasMoneyRun(container, /\d[\d.,]*\s*USD/)).toBe(true);
    expect(hasMoneyRun(container, VND_SUFFIX)).toBe(false);
    expect(text).toContain("USD");
  });

  it("don gia khong huu han -> khong bao gio ro ri 'NaN'", () => {
    const { container } = render(
      <OrderSummary
        items={[{ tierName: "Hang A", quantity: 2, unitAmount: Number.NaN, currency: "VND" }]}
        holdState="idle"
      />,
    );

    expect(allText(container)).not.toContain("NaN");
  });
});

describe("OrderSummary — phi/thue CHI hien khi duoc truyen (AC-8, handoff §11.6)", () => {
  it("thieu `fees` -> KHONG render dong phi nao", () => {
    const { container } = render(<OrderSummary {...BASE} />);

    // KHONG dung `\b` sau chu co dau: `\b` trong JS la ASCII, nen /ph[íi]\b/ khong
    // bao gio khop "Phí " va assertion phu dinh se thanh vo nghia (luon pass).
    expect(allText(container)).not.toMatch(/ph[íi]|fee/i);
  });

  it("thieu `fees` -> khong bao gio xuat hien 'Phi: 0 đ' hay '0 đ'", () => {
    const { container } = render(<OrderSummary {...BASE} />);

    // Chu y: "200.000 đ" cung chua chuoi "0 đ", nen phai neo vao ranh gioi so:
    // chi bat mot so tien la DUNG so 0 ("0 đ"), khong bat duoi cua so tien khac.
    expect(allText(container)).not.toMatch(/(^|[^.\d])0\s*đ/);
  });

  it("thieu `fees` -> `total === subtotal` (khong co con so thu hai khac)", () => {
    const { container } = render(<OrderSummary {...BASE} />);
    const text = allText(container);

    expect(text).toContain(SUBTOTAL);
    expect(text).not.toContain(TOTAL_WITH_FEES);
  });

  it("co `fees` -> render dong phi va `total = subtotal + fees`", () => {
    const { container } = render(<OrderSummary {...BASE} fees={FEES} />);
    const text = allText(container);

    expect(text).toContain(FEES_TEXT);
    expect(text).toContain(TOTAL_WITH_FEES);
  });

  it("`fees={0}` duoc truyen TUONG MINH thi duoc hien (0 do server khang dinh, khong phai UI doan)", () => {
    const { container } = render(<OrderSummary {...BASE} fees={0} />);

    // Day la su khac biet quyet dinh: "khong truyen" khac "truyen 0".
    expect(allText(container)).toMatch(/ph[íi]|fee/i);
  });

  it("KHONG bao gio render dong thue khi khong co prop thue", () => {
    for (const fees of [undefined, FEES]) {
      const { container, unmount } = render(<OrderSummary {...BASE} fees={fees} />);

      // Khong dung `\b`: tren chuoi gop, "VAT" o cuoi mot node bi chu dau cua node ke ben
      // noi vao nen `/\bVAT\b/` truot — mot assertion PHU DINH truot la luon pass, tuc la
      // vo nghia. Bo ranh gioi thi chi con rui ro khop rong, va huong do thi an toan.
      expect(allText(container)).not.toMatch(/thu[ếe]|VAT|tax/i);
      unmount();
    }
  });
});

describe("OrderSummary — `items` rong: trang thai rong, khong phai tong 0 (AC-8)", () => {
  it("hien mot trang thai rong co chu", () => {
    const { container } = render(<OrderSummary items={[]} holdState="idle" />);

    expect(words(allText(container)).length).toBeGreaterThan(2);
  });

  it("KHONG hien '0 đ' nhu mot tong hop le", () => {
    const { container } = render(<OrderSummary items={[]} holdState="idle" />);

    expect(allText(container)).not.toContain("0 đ");
  });

  it("KHONG hien chuoi tong nao (khong co SO TIEN nao ca)", () => {
    const { container } = render(<OrderSummary items={[]} holdState="idle" />);

    // Khong co don nao thi khong co tong nao: khong duoc co bat ky so tien nao tren
    // man hinh, ke ca `0 đ`. Quet tung text node, va chi quet SO TIEN chu khong quet
    // ky tu `đ` cua chu thuong tieng Viet.
    expect(hasMoneyRun(container, MONEY)).toBe(false);
  });

  it("`items` rong khac han `items` co dong (khong dung chung mot layout tong 0)", () => {
    const empty = render(<OrderSummary items={[]} holdState="idle" />);
    const emptyText = allText(empty.container);
    empty.unmount();

    const filled = render(<OrderSummary {...BASE} />);

    expect(emptyText).not.toBe(allText(filled.container));
  });
});

describe("OrderSummary — tron nhieu currency: bao LOI, khong cong lan (AC-8)", () => {
  const MIXED: OrderSummaryItem[] = [
    { tierName: "Hang A", quantity: 1, unitAmount: 100000, currency: "VND" },
    { tierName: "Hang B", quantity: 1, unitAmount: 50000, currency: "USD" },
  ];

  it("KHONG hien tong cong lan (150.000 / 150000 khong duoc xuat hien)", () => {
    const { container } = render(<OrderSummary items={MIXED} holdState="idle" />);
    const text = allText(container);

    expect(text).not.toContain("150.000");
    expect(text).not.toContain("150000");
  });

  it("co thong bao loi den duoc tro ho tro (role alert/status hoac chu ro rang)", () => {
    const { container } = render(<OrderSummary items={MIXED} holdState="idle" />);
    const signalled =
      container.querySelector('[role="alert"]') !== null ||
      container.querySelector('[role="status"]') !== null ||
      /l[ỗo]i|kh[ôo]ng (th[ểe]|h[ợo]p l[ệe])|kh[áa]c nhau|kh[ôo]ng kh[ớo]p/i.test(
        allText(container),
      );

    expect(signalled).toBe(true);
  });

  it("tung dong van hien dung tien te cua rieng no (khong mat thong tin, chi khong cong)", () => {
    const { container } = render(<OrderSummary items={MIXED} holdState="idle" />);
    const text = allText(container);

    expect(text).toContain("100.000 đ");
    expect(text).toContain("USD");
  });

  it("cung mot currency o moi item thi KHONG bao loi (test phan biet duoc hai nhanh)", () => {
    const { container } = render(<OrderSummary {...BASE} />);

    expect(allText(container)).toContain(SUBTOTAL);
    expect(allText(container)).not.toMatch(/l[ỗo]i/i);
  });
});

describe("OrderSummary — sau trang thai hold deu duoc phu va khac nhau (AC-9)", () => {
  it("render duoc ca sau trang thai ma khong vo", () => {
    for (const state of HOLD_STATES) {
      const { container, unmount } = render(
        <OrderSummary
          {...BASE}
          holdState={state}
          holdExpiresAt={HOLD_EXPIRES_AT}
          primaryActionLabel={PRIMARY_LABEL}
        />,
      );

      expect(allText(container)).toContain(ITEM_A.tierName);
      unmount();
    }
  });

  it("moi trang thai co phan chu rieng, khong rong", () => {
    for (const state of HOLD_STATES) {
      expect(holdWords(state).length).toBeGreaterThan(2);
    }
  });

  it("`ambiguous` KHONG bi gop voi `expired` (hai thong diep khac nhau)", () => {
    expect(holdWords("ambiguous")).not.toBe(holdWords("expired"));
  });

  it("`ambiguous` cung khac `creating` va khac `active`", () => {
    const ambiguous = holdWords("ambiguous");

    expect(ambiguous).not.toBe(holdWords("creating"));
    expect(ambiguous).not.toBe(holdWords("active"));
  });

  it("`expired` khac `sold_out` (tra kho vi het gio khac het ve)", () => {
    expect(holdWords("expired")).not.toBe(holdWords("sold_out"));
  });
});

describe("OrderSummary — `creating`: dang xu ly, chua giu duoc gi (AC-9)", () => {
  it("nut chinh bi disable", () => {
    const { container } = render(
      <OrderSummary
        {...BASE}
        holdState="creating"
        primaryActionLabel={PRIMARY_LABEL}
        onPrimaryAction={vi.fn()}
      />,
    );
    const primary = buttons(container).find(isPrimary);

    expect(primary).toBeDefined();
    expect(isDisabled(primary as Element)).toBe(true);
  });

  it("bam nut chinh khong goi `onPrimaryAction` (khong tao hold thu hai)", () => {
    const onPrimaryAction = vi.fn();
    const { container } = render(
      <OrderSummary
        {...BASE}
        holdState="creating"
        primaryActionLabel={PRIMARY_LABEL}
        onPrimaryAction={onPrimaryAction}
      />,
    );

    clickEveryButton(container);

    expect(onPrimaryAction).not.toHaveBeenCalled();
  });

  it("co chi bao dang xu ly den duoc tro ho tro", () => {
    const { container } = render(
      <OrderSummary {...BASE} holdState="creating" primaryActionLabel={PRIMARY_LABEL} />,
    );
    const indicated =
      container.querySelector('[aria-busy="true"]') !== null ||
      container.querySelector('[role="status"]') !== null ||
      container.querySelector("[aria-live]") !== null;

    expect(indicated).toBe(true);
  });

  it("KHONG noi la da giu duoc ve", () => {
    const { container } = render(
      <OrderSummary {...BASE} holdState="creating" primaryActionLabel={PRIMARY_LABEL} />,
    );

    expect(allText(container)).not.toMatch(/đ[ãa] gi[ữu]|gi[ữu] (v[ée] )?th[àa]nh c[ôo]ng/i);
  });

  it("KHONG render dong ho giu ve (chua co hold thi chua co moc het han)", () => {
    const { container } = render(
      <OrderSummary
        {...BASE}
        holdState="creating"
        holdExpiresAt={HOLD_EXPIRES_AT}
        primaryActionLabel={PRIMARY_LABEL}
      />,
    );

    expect(allText(container)).not.toContain("09:30");
  });
});

describe("OrderSummary — `active`: dung DONG HO GIU VE, khong phai dong ho suat mua (AC-9, BR-Q7)", () => {
  it("text chua nhan cua variant 'hold'", () => {
    const holdLabel = countdownLabel("hold");

    const { container } = render(
      <OrderSummary
        {...BASE}
        holdState="active"
        holdExpiresAt={HOLD_EXPIRES_AT}
        primaryActionLabel={PRIMARY_LABEL}
      />,
    );

    expect(holdLabel.length).toBeGreaterThan(3);
    expect(words(allText(container))).toContain(holdLabel);
  });

  it("text KHONG chua nhan cua variant 'admission' (hai dong ho khong duoc lan)", () => {
    const admissionLabel = countdownLabel("admission");

    const { container } = render(
      <OrderSummary
        {...BASE}
        holdState="active"
        holdExpiresAt={HOLD_EXPIRES_AT}
        primaryActionLabel={PRIMARY_LABEL}
      />,
    );

    expect(admissionLabel.length).toBeGreaterThan(3);
    expect(words(allText(container))).not.toContain(admissionLabel);
  });

  it("text KHONG chua nhan cua variant 'sale-start'", () => {
    const saleStartLabel = countdownLabel("sale-start");

    const { container } = render(
      <OrderSummary
        {...BASE}
        holdState="active"
        holdExpiresAt={HOLD_EXPIRES_AT}
        primaryActionLabel={PRIMARY_LABEL}
      />,
    );

    expect(words(allText(container))).not.toContain(saleStartLabel);
  });

  it("dung dung `holdExpiresAt` cua prop (09:30), khong tu tinh 10 phut (BR-O2)", () => {
    const { container } = render(
      <OrderSummary
        {...BASE}
        holdState="active"
        holdExpiresAt={HOLD_EXPIRES_AT}
        primaryActionLabel={PRIMARY_LABEL}
      />,
    );
    const text = allText(container);

    expect(text).toContain("09:30");
    expect(text).not.toContain("10:00");
  });

  it("nhan `holdExpiresAt` dang chuoi ISO CO mui gio", () => {
    const iso = new Date(HOLD_EXPIRES_AT).toISOString();
    const { container } = render(
      <OrderSummary
        {...BASE}
        holdState="active"
        holdExpiresAt={iso}
        primaryActionLabel={PRIMARY_LABEL}
      />,
    );

    expect(allText(container)).toContain("09:30");
  });

  it("dem nguoc theo thoi gian troi that", () => {
    const { container } = render(
      <OrderSummary
        {...BASE}
        holdState="active"
        holdExpiresAt={HOLD_EXPIRES_AT}
        primaryActionLabel={PRIMARY_LABEL}
      />,
    );

    act(() => {
      vi.advanceTimersByTime(10_000);
    });

    expect(allText(container)).toContain("09:20");
  });

  it("dong ho KHONG reset khi rerender (handoff §11.6: 'Do not reset timer on rerender')", () => {
    const { container, rerender } = render(
      <OrderSummary
        {...BASE}
        holdState="active"
        holdExpiresAt={HOLD_EXPIRES_AT}
        primaryActionLabel={PRIMARY_LABEL}
      />,
    );

    act(() => {
      vi.advanceTimersByTime(60_000);
    });
    expect(allText(container)).toContain("08:30");

    rerender(
      <OrderSummary
        {...BASE}
        holdState="active"
        holdExpiresAt={HOLD_EXPIRES_AT}
        primaryActionLabel={PRIMARY_LABEL}
        fees={FEES}
      />,
    );

    expect(allText(container)).toContain("08:30");
    expect(allText(container)).not.toContain("09:30");
  });

  it("chuyen `offsetMs` xuong dong ho (lech gio may khach khong lam sai so con lai)", () => {
    const { container } = render(
      <OrderSummary
        {...BASE}
        holdState="active"
        holdExpiresAt={START + 180_000}
        offsetMs={60_000}
        primaryActionLabel={PRIMARY_LABEL}
      />,
    );

    expect(allText(container)).toContain("02:00");
  });

  it("chuyen `holdWarningThresholdMs` xuong dong ho (nguong la policy cua caller)", () => {
    const { container } = render(
      <OrderSummary
        {...BASE}
        holdState="active"
        holdExpiresAt={START + 60_000}
        holdWarningThresholdMs={120_000}
        primaryActionLabel={PRIMARY_LABEL}
      />,
    );

    expect(container.querySelector('[data-state="warning"]')).not.toBeNull();
  });

  it("KHONG truyen nguong -> khong co trang thai canh bao nao", () => {
    const { container } = render(
      <OrderSummary
        {...BASE}
        holdState="active"
        holdExpiresAt={START + 5_000}
        primaryActionLabel={PRIMARY_LABEL}
      />,
    );

    expect(container.querySelector('[data-state="warning"]')).toBeNull();
  });

  it("thieu `holdExpiresAt` -> khong bia moc, khong hien 00:00 nhu da het han", () => {
    const { container } = render(
      <OrderSummary {...BASE} holdState="active" primaryActionLabel={PRIMARY_LABEL} />,
    );

    expect(allText(container)).not.toContain("00:00");
    expect(allText(container)).not.toContain("NaN");
  });

  it("nut chinh dung duoc va goi `onPrimaryAction`", () => {
    const onPrimaryAction = vi.fn();
    const { container } = render(
      <OrderSummary
        {...BASE}
        holdState="active"
        holdExpiresAt={HOLD_EXPIRES_AT}
        primaryActionLabel={PRIMARY_LABEL}
        onPrimaryAction={onPrimaryAction}
      />,
    );
    const primary = buttons(container).find(isPrimary);

    expect(primary).toBeDefined();
    fireEvent.click(primary as HTMLElement);

    expect(onPrimaryAction).toHaveBeenCalledTimes(1);
  });
});

describe("OrderSummary — `expired`: ve da tra kho, phai chon lai (AC-9)", () => {
  it("noi ro ve da duoc tra lai kho / khong con giu", () => {
    const { container } = render(
      <OrderSummary
        {...BASE}
        holdState="expired"
        holdExpiresAt={START - 1_000}
        primaryActionLabel={PRIMARY_LABEL}
      />,
    );

    expect(allText(container)).toMatch(/tr[ảa] (l[ạa]i |v[ềe] )?kho|kh[ôo]ng c[òo]n gi[ữu]|đ[ãa] đ[ưu][ợo]c tr[ảa]/i);
  });

  it("noi ro phai chon lai", () => {
    const { container } = render(
      <OrderSummary
        {...BASE}
        holdState="expired"
        holdExpiresAt={START - 1_000}
        primaryActionLabel={PRIMARY_LABEL}
      />,
    );

    expect(allText(container)).toMatch(/ch[ọo]n l[ạa]i|ch[ọo]n v[ée] l[ạa]i|b[ắa]t đ[ầa]u l[ạa]i/i);
  });

  it("nut chinh KHONG con la nut thanh toan duoc truyen vao", () => {
    const { container } = render(
      <OrderSummary
        {...BASE}
        holdState="expired"
        holdExpiresAt={START - 1_000}
        primaryActionLabel={PRIMARY_LABEL}
        onPrimaryAction={vi.fn()}
      />,
    );

    for (const button of enabledButtons(container)) {
      expect(accName(button)).not.toContain(PRIMARY_LABEL);
    }
  });

  it("KHONG moi thanh toan bang chu", () => {
    const { container } = render(
      <OrderSummary
        {...BASE}
        holdState="expired"
        holdExpiresAt={START - 1_000}
        primaryActionLabel={PRIMARY_LABEL}
      />,
    );

    for (const button of enabledButtons(container)) {
      expect(accName(button)).not.toMatch(/thanh to[áa]n/i);
    }
  });
});

describe("OrderSummary — `sold_out`: khong moi thanh toan (AC-9, BR-Q6)", () => {
  it("khong co nut thanh toan dung duoc", () => {
    const { container } = render(
      <OrderSummary
        {...BASE}
        holdState="sold_out"
        primaryActionLabel={PRIMARY_LABEL}
        onPrimaryAction={vi.fn()}
      />,
    );

    for (const button of enabledButtons(container)) {
      expect(accName(button)).not.toMatch(/thanh to[áa]n/i);
      expect(accName(button)).not.toContain(PRIMARY_LABEL);
    }
  });

  it("noi ro la het ve", () => {
    const { container } = render(
      <OrderSummary {...BASE} holdState="sold_out" primaryActionLabel={PRIMARY_LABEL} />,
    );

    expect(allText(container)).toMatch(/h[ếe]t v[ée]|h[ếe]t ch[ỗo]|s[ốo]ld.?out/i);
  });

  it("KHONG render dong ho giu ve", () => {
    const { container } = render(
      <OrderSummary
        {...BASE}
        holdState="sold_out"
        holdExpiresAt={HOLD_EXPIRES_AT}
        primaryActionLabel={PRIMARY_LABEL}
      />,
    );

    expect(allText(container)).not.toContain("09:30");
  });
});

describe("OrderSummary — `ambiguous`: khong khang dinh gi, khong moi bam lai (AC-9, BR-O5)", () => {
  function renderAmbiguous(onPrimaryAction = vi.fn()) {
    const result = render(
      <OrderSummary
        {...BASE}
        holdState="ambiguous"
        holdExpiresAt={HOLD_EXPIRES_AT}
        primaryActionLabel={PRIMARY_LABEL}
        onPrimaryAction={onPrimaryAction}
      />,
    );
    return { ...result, onPrimaryAction };
  }

  it("KHONG noi thanh cong", () => {
    const { container } = renderAmbiguous();

    expect(allText(container)).not.toMatch(
      /th[àa]nh c[ôo]ng|đ[ãa] gi[ữu]|đ[ãa] thanh to[áa]n|đ[ãa] x[áa]c nh[ậa]n/i,
    );
  });

  it("KHONG noi that bai", () => {
    const { container } = renderAmbiguous();

    expect(allText(container)).not.toMatch(
      /th[ấa]t b[ạa]i|kh[ôo]ng th[àa]nh c[ôo]ng|l[ỗo]i|b[ịi] t[ừu] ch[ốo]i|hu[ỷy]|h[ủu]y/i,
    );
  });

  it("KHONG render nut nao se tao hold moi (bam het moi nut cung khong goi `onPrimaryAction`)", () => {
    const { container, onPrimaryAction } = renderAmbiguous();

    clickEveryButton(container);

    expect(onPrimaryAction).not.toHaveBeenCalled();
  });

  it("KHONG co nut nao mang nghia thu lai / tao lai / thanh toan", () => {
    const { container } = renderAmbiguous();

    for (const button of buttons(container)) {
      expect(accName(button)).not.toMatch(
        /th[ửu] l[ạa]i|l[àa]m l[ạa]i|t[ạa]o l[ạa]i|gi[ữu] l[ạa]i|thanh to[áa]n|ti[ếe]p t[ụu]c|retry/i,
      );
    }
  });

  it("KHONG render nut chinh duoc truyen vao (du caller van truyen label)", () => {
    const { container } = renderAmbiguous();

    for (const button of buttons(container)) {
      expect(accName(button)).not.toContain(PRIMARY_LABEL);
    }
  });

  it("moi khach CHO he thong xac nhan", () => {
    const { container } = renderAmbiguous();

    expect(allText(container)).toMatch(/ch[ờo]|đang x[áa]c nh[ậa]n|đang ki[ểe]m tra|đang x[ửu] l[ýy]/i);
  });

  it("thong bao den duoc tro ho tro (co vung live hoac role status/alert)", () => {
    const { container } = renderAmbiguous();
    const announced =
      container.querySelector("[aria-live]") !== null ||
      container.querySelector('[role="status"]') !== null ||
      container.querySelector('[role="alert"]') !== null;

    expect(announced).toBe(true);
  });

  it("KHONG render dong ho giu ve (chua biet co hold hay khong)", () => {
    const { container } = renderAmbiguous();

    expect(allText(container)).not.toContain("09:30");
  });
});

describe("OrderSummary — `idle` (AC-9)", () => {
  it("KHONG render dong ho giu ve khi chua co hold", () => {
    const { container } = render(
      <OrderSummary {...BASE} holdExpiresAt={HOLD_EXPIRES_AT} holdState="idle" />,
    );

    expect(allText(container)).not.toContain("09:30");
  });

  it("KHONG noi la da giu duoc ve", () => {
    const { container } = render(<OrderSummary {...BASE} holdState="idle" />);

    expect(allText(container)).not.toMatch(/đ[ãa] gi[ữu]|gi[ữu] (v[ée] )?th[àa]nh c[ôo]ng/i);
  });

  it("nut chinh dung duoc de bat dau giu ve", () => {
    const onPrimaryAction = vi.fn();
    const { container } = render(
      <OrderSummary
        {...BASE}
        holdState="idle"
        primaryActionLabel={PRIMARY_LABEL}
        onPrimaryAction={onPrimaryAction}
      />,
    );
    const primary = buttons(container).find(isPrimary);

    expect(primary).toBeDefined();
    fireEvent.click(primary as HTMLElement);

    expect(onPrimaryAction).toHaveBeenCalledTimes(1);
  });
});

/**
 * `sold_out` KHONG phai mot chieu — BR-E3 + BR-O1.
 *
 * BR-O1 (docs/01 dong 84): Redis lech so "chi co the gay 'bao het ve som', va JOB DOI
 * SOAT SE SUA LAI". BR-E3 (dong 30): sau `ON_SALE` quota chi duoc TANG.
 *
 * Nen `holdState` co the di tu `sold_out` ve `idle` ngay trong phien. Neu summary khoa
 * nut chinh lai vinh vien sau mot lan thay `sold_out`, khach khong mua duoc so ve ma
 * doi soat vua tra lai kho.
 */
describe("OrderSummary — sold_out khong mot chieu, nut chinh phai quay lai (AC-9, BR-E3, BR-O1)", () => {
  it("sold_out -> rerender idle: nut chinh quay lai va bam duoc", () => {
    const onPrimaryAction = vi.fn();
    const { container, rerender } = render(
      <OrderSummary
        {...BASE}
        holdState="sold_out"
        primaryActionLabel={PRIMARY_LABEL}
        onPrimaryAction={onPrimaryAction}
      />,
    );
    expect(enabledButtons(container).filter(isPrimary)).toHaveLength(0);

    rerender(
      <OrderSummary
        {...BASE}
        holdState="idle"
        primaryActionLabel={PRIMARY_LABEL}
        onPrimaryAction={onPrimaryAction}
      />,
    );

    const primary = buttons(container).find(isPrimary);
    expect(primary).toBeDefined();

    fireEvent.click(primary as HTMLElement);
    expect(onPrimaryAction).toHaveBeenCalledTimes(1);
  });

  it("sold_out -> rerender idle: khong giu lai thong diep 'het ve'", () => {
    const { container, rerender } = render(
      <OrderSummary {...BASE} holdState="sold_out" primaryActionLabel={PRIMARY_LABEL} />,
    );

    rerender(<OrderSummary {...BASE} holdState="idle" primaryActionLabel={PRIMARY_LABEL} />);

    expect(allText(container)).not.toMatch(/h[ếe]t v[ée]|h[ếe]t ch[ỗo]|s[ốo]ld.?out/i);
  });

  it("idle -> sold_out -> idle: ve dung trang thai ban dau", () => {
    const reference = render(
      <OrderSummary {...BASE} holdState="idle" primaryActionLabel={PRIMARY_LABEL} />,
    );
    const referenceText = allText(reference.container);
    reference.unmount();

    const { container, rerender } = render(
      <OrderSummary {...BASE} holdState="idle" primaryActionLabel={PRIMARY_LABEL} />,
    );
    rerender(<OrderSummary {...BASE} holdState="sold_out" primaryActionLabel={PRIMARY_LABEL} />);
    rerender(<OrderSummary {...BASE} holdState="idle" primaryActionLabel={PRIMARY_LABEL} />);

    expect(allText(container)).toBe(referenceText);
  });
});

/**
 * AC-10 ("nut chinh LUON HIEN THI") gap AC-8 ("`items` rong -> trang thai rong").
 *
 * Hai AC noi nguoc nhau neu doc "luon hien thi" la "luon bam duoc". Hop dong da chot:
 * "luon hien thi" la rang buoc BO CUC mobile — nut khong duoc bi giau sau phan thu gon —
 * chu khong phai loi moi bam. Chua chon ve nao thi khong co gi de giu, nen nut PHAI
 * render va PHAI disabled. Mot nut bam duoc o day se tao hold rong hoac mot request
 * chac chan bi server tu choi, trong khi khach da tieu mat mot phan suat admit 15 phut.
 */
describe("OrderSummary — `items` rong: nut chinh van render nhung disabled (AC-8 ∩ AC-10)", () => {
  it("nut chinh VAN render khi `items` rong (khong bi giau o mobile)", () => {
    const { container } = render(
      <OrderSummary
        items={[]}
        holdState="idle"
        primaryActionLabel={PRIMARY_LABEL}
        onPrimaryAction={vi.fn()}
      />,
    );

    expect(buttons(container).find(isPrimary)).toBeDefined();
  });

  it("nut chinh bi DISABLED khi `items` rong", () => {
    const { container } = render(
      <OrderSummary
        items={[]}
        holdState="idle"
        primaryActionLabel={PRIMARY_LABEL}
        onPrimaryAction={vi.fn()}
      />,
    );

    expect(isDisabled(buttons(container).find(isPrimary) as Element)).toBe(true);
  });

  it("bam het moi nut khi `items` rong cung khong goi `onPrimaryAction`", () => {
    const onPrimaryAction = vi.fn();
    const { container } = render(
      <OrderSummary
        items={[]}
        holdState="idle"
        primaryActionLabel={PRIMARY_LABEL}
        onPrimaryAction={onPrimaryAction}
      />,
    );

    clickEveryButton(container);

    expect(onPrimaryAction).not.toHaveBeenCalled();
  });

  it("co `items` tro lai -> nut chinh bam duoc (khong khoa vinh vien)", () => {
    const onPrimaryAction = vi.fn();
    const { container, rerender } = render(
      <OrderSummary
        items={[]}
        holdState="idle"
        primaryActionLabel={PRIMARY_LABEL}
        onPrimaryAction={onPrimaryAction}
      />,
    );

    rerender(
      <OrderSummary
        items={ITEMS}
        holdState="idle"
        primaryActionLabel={PRIMARY_LABEL}
        onPrimaryAction={onPrimaryAction}
      />,
    );

    fireEvent.click(buttons(container).find(isPrimary) as HTMLElement);

    expect(onPrimaryAction).toHaveBeenCalledTimes(1);
  });
});

describe("OrderSummary — mobile: thu gon duoc nhung nut chinh luon o day (AC-10, handoff §11.6)", () => {
  /** Nut dieu khien vung mo rong (dau hieu duy nhat khong phu thuoc CSS). */
  function toggle(container: HTMLElement): HTMLElement | undefined {
    return Array.from(container.querySelectorAll<HTMLElement>("[aria-expanded]"))[0];
  }

  function primary(container: HTMLElement): HTMLElement | undefined {
    return buttons(container).find(isPrimary);
  }

  function renderSummary() {
    return render(
      <OrderSummary
        {...BASE}
        holdState="active"
        holdExpiresAt={HOLD_EXPIRES_AT}
        primaryActionLabel={PRIMARY_LABEL}
        onPrimaryAction={vi.fn()}
      />,
    );
  }

  it("co dieu khien mo rong voi `aria-expanded`", () => {
    const { container } = renderSummary();

    expect(toggle(container)).toBeDefined();
    expect(["true", "false"]).toContain(toggle(container)?.getAttribute("aria-expanded"));
  });

  it("`aria-controls` tro vao mot phan tu CO THUC trong DOM", () => {
    const { container } = renderSummary();
    const controls = toggle(container)?.getAttribute("aria-controls");

    expect(controls).toBeTruthy();
    expect(document.getElementById(controls as string)).not.toBeNull();
  });

  it("dieu khien mo rong co ten doc duoc", () => {
    const { container } = renderSummary();

    expect(accName(toggle(container) as Element).length).toBeGreaterThan(0);
  });

  it("bam dieu khien thi `aria-expanded` doi gia tri", () => {
    const { container } = renderSummary();
    const before = toggle(container)?.getAttribute("aria-expanded");

    fireEvent.click(toggle(container) as HTMLElement);

    expect(toggle(container)?.getAttribute("aria-expanded")).not.toBe(before);
  });

  it("nut chinh co mat o CA hai trang thai thu gon va mo rong", () => {
    const { container } = renderSummary();

    expect(primary(container)).toBeDefined();

    fireEvent.click(toggle(container) as HTMLElement);
    expect(primary(container)).toBeDefined();

    fireEvent.click(toggle(container) as HTMLElement);
    expect(primary(container)).toBeDefined();
  });

  it("nut chinh van bam duoc khi dang thu gon", () => {
    const onPrimaryAction = vi.fn();
    const { container } = render(
      <OrderSummary
        {...BASE}
        holdState="active"
        holdExpiresAt={HOLD_EXPIRES_AT}
        primaryActionLabel={PRIMARY_LABEL}
        onPrimaryAction={onPrimaryAction}
      />,
    );
    const toggleButton = toggle(container) as HTMLElement;

    if (toggleButton.getAttribute("aria-expanded") === "true") {
      fireEvent.click(toggleButton);
    }

    fireEvent.click(primary(container) as HTMLElement);

    expect(onPrimaryAction).toHaveBeenCalledTimes(1);
  });

  it("dong ho giu ve khong bi reset khi thu gon/mo rong", () => {
    const { container } = renderSummary();

    act(() => {
      vi.advanceTimersByTime(30_000);
    });
    expect(allText(container)).toContain("09:00");

    fireEvent.click(toggle(container) as HTMLElement);

    expect(allText(container)).not.toContain("09:30");
  });
});
