/**
 * Test cho `TicketTierCard` — AC-4..AC-7 (spec-plan EVF-1802/1804 §4), BR-O1, BR-O3, BR-O4.
 *
 * ==> HAI TEST QUAN TRONG NHAT: describe "ton kho" va describe "gioi han mua".
 *     Doc phan giai thich truoc khi sua bat ky assertion nao o do.
 *
 * 1) BR-O1 — KHONG BAO GIO KHANG DINH TON KHO CHINH XAC (docs/01 dong 84).
 *    "Redis lech so KHONG THE gay oversell — chi co the gay 'bao het ve som', va job
 *    doi soat se sua lai." Cong handoff §3: "so luong ve public chi nen la
 *    snapshot/nhan khai quat, khong query DB theo moi luot xem."
 *
 *    Nghia la mot con so tren the vé LUON co the sai, va sai theo chieu lam khach tin
 *    rang con nhieu hon/it hon thuc te. `availability` vi vay la NHAN ROI RAC
 *    (`available | limited | sold_out`), va component KHONG co prop so ve con lai.
 *    Test o day doi khang theo hai huong:
 *      - nhan `"limited"` phai KHONG chua bat ky chu so nao ("Sap het", khong phai
 *        "Con 3 ve"). Kiem bang cach bo chuoi gia ra khoi text roi quet `/\d/`.
 *      - truyen them prop so luong bang cast (nhu mot caller "tot bung" se lam) va
 *        bat buoc con so do KHONG duoc xuat hien o dau.
 *
 *    Va: thieu `availability` thi KHONG duoc khang dinh con ve. "Khong biet" khac
 *    "con ve" — handoff §5 nguyen tac 3 doi ghi "dang cap nhat".
 *
 * 2) BR-O4 — GIOI HAN MUA LA CUA SERVER (docs/01 dong 87). Gioi han thuc =
 *    `min(max_per_order, max_per_identity - da mua)`. So hang thu hai phu thuoc LICH
 *    SU MUA cua nguoi nay tren moi don da PAID — UI khong co du lieu do va khong bao
 *    gio tu tinh duoc. "Mac dinh 4" trong docs/01 la default cua HE THONG, khong phai
 *    thu UI duoc phep doan.
 *
 *    Vi the `maxSelectable` KHONG CO DEFAULT: thieu prop -> KHONG render dieu khien
 *    chon so luong. Neu component tu lay 4, mot khach da mua 3 ve se duoc moi chon
 *    them 4, bam thanh toan, va server tu choi — khach mat suat admit (TTL 15 phut,
 *    BR-Q7) vi mot con so UI tu bia.
 *
 *    Cach test "khong co dieu khien" khong dua vao cau chu: bam HET moi nut duoc
 *    render roi assert `onQuantityChange` chua he duoc goi, DONG THOI assert khong co
 *    `role="spinbutton"`/`input[type=number]`/nut tang-giam nao.
 *
 * 3) BR-O3 (docs/01 dong 86, sua 2026-10-08) — "Mot khach chi co 1 hold dang hoat dong
 *    moi su kien. Goi tao hold khi da co hold con han -> tra ve dung hold dang co." Khach
 *    dang giu 2 ve hang A ma bam sang hang B thi nhan lai CHINH 2 ve hang A, khong doi duoc
 *    cho toi khi thanh toan hoac het han. Khong noi truoc thi khach chon B roi nhan ve A ma
 *    khong hieu vi sao. Canh bao phai la TEXT (handoff §5 nguyen tac 7: trang thai khong
 *    duoc phan biet chi bang mau), va KHONG duoc noi "huy" -- ban cu cua BR-O3 noi hold cu
 *    bi huy, dieu chua tung xay ra o backend.
 */
import { fireEvent, render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import {
  TicketTierCard,
  type TicketTierCardProps,
  type TierAvailability,
} from "./ticket-tier-card";

const AVAILABILITIES: TierAvailability[] = ["available", "limited", "sold_out"];

const NAME = "Hang Thuong";

/** Gia co chu so 1/5/0 — tranh trung voi so luong dung trong test (2, 3). */
const BASE: TicketTierCardProps = {
  name: NAME,
  unitAmount: 500000,
  currency: "VND",
};

const PRICE_TEXT = "500.000 đ";

/** Ten nut tang / giam so luong — chap nhan nhieu cach dat chu khac nhau.
 *  KHONG dua "them" vao INCREASE: mot nut "Them vao don" se bi nhan dien nham.
 *  Xem muc "khop long" trong traceability.md neu nut duoc dat ten khac. */
const INCREASE = /t[ăa]ng|c[ộo]ng|increase|plus|^\+$/i;
const DECREASE = /gi[ảa]m|b[ớo]t|tr[ừu]|decrease|minus|^[-−–]$/i;

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

/**
 * Noi dung cua TUNG text node rieng le (cong moi `aria-label`).
 *
 * `textContent` cua ca cay noi cac node lien nhau KHONG chen khoang trang: "500.000 USD"
 * dung canh "Chon hang nay" cho ra "...500.000 USDChon hang nay...". Tren chuoi gop do,
 * moi rang buoc ranh gioi tu deu sai: `/\bUSD\b/` truot vi `D` va `C` deu la word char.
 * Mot text node thi luon la mot doan lien tuc, nen khong co hien tuong dinh chu.
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

/** So tien mang hau to VND. `(?!\p{L})` loai "2 đơn", "đã", "đang cập nhật". */
const VND_MONEY = /\d[\d.,]*\s*đ(?!\p{L})/u;

/** Chu ma tro ho tro THUC SU doc: bo qua moi nhanh `aria-hidden="true"`. */
function atText(root: Element): string {
  if (root.getAttribute?.("aria-hidden") === "true") return "";

  let out = "";
  for (const node of Array.from(root.childNodes)) {
    if (node.nodeType === 3) {
      out += node.textContent ?? "";
    } else if (node.nodeType === 1) {
      const el = node as Element;
      out += ` ${atText(el)}`;
      const label = el.getAttribute("aria-label");
      if (label) out += ` ${label}`;
    }
  }
  return norm(out);
}

function buttons(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>('button, [role="button"]'));
}

/** Ten doc duoc cua mot control: `aria-label` neu co, neu khong thi text. */
function accName(el: Element): string {
  return norm(el.getAttribute("aria-label") ?? el.textContent);
}

function findButton(container: HTMLElement, pattern: RegExp): HTMLElement | undefined {
  return buttons(container).find((button) => pattern.test(accName(button)));
}

/**
 * Moi widget co the dung de doi so luong, bat ke cach cai dat:
 * spinbutton, input so, hoac nut tang/giam.
 */
function quantityWidgets(container: HTMLElement): Element[] {
  return [
    ...Array.from(container.querySelectorAll('[role="spinbutton"]')),
    ...Array.from(container.querySelectorAll('input[type="number"]')),
    ...buttons(container).filter(
      (button) => INCREASE.test(accName(button)) || DECREASE.test(accName(button)),
    ),
  ];
}

/** Bam het moi nut duoc render — dung de chung minh KHONG co duong nao doi so luong. */
function clickEveryButton(container: HTMLElement): void {
  for (const button of buttons(container)) {
    fireEvent.click(button);
  }
}

describe("TicketTierCard — gia di qua formatMoney, khong bia tien te (AC-4, §2.4)", () => {
  it("VND -> hien dung dinh dang '500.000 đ'", () => {
    const { container } = render(<TicketTierCard {...BASE} />);

    expect(allText(container)).toContain(PRICE_TEXT);
  });

  it("USD -> KHONG in 'đ' va co ma tien te", () => {
    const { container } = render(<TicketTierCard {...BASE} currency="USD" />);
    const text = allText(container);

    // Hai cai bay da gap o day, ca hai deu do `textContent` noi node lien nhau:
    //  1. `not.toContain("đ")` KHONG the pass: "đang cap nhat" cung co ky tu `đ`.
    //  2. `/\bUSD\b/` TRUOT: sau "USD" la chu dau cua phan tu ke ben ("...USDChon..."),
    //     `D` va `C` deu la word char nen khong co ranh gioi tu.
    // Cach dung: kiem tung TEXT NODE, va chi chot "co so tien USD" / "khong so tien đ".
    expect(hasMoneyRun(container, /\d[\d.,]*\s*USD/)).toBe(true);
    expect(hasMoneyRun(container, VND_MONEY)).toBe(false);
    expect(text).toContain("USD");
  });

  it("gia 0 van hien ro la 0, khong an di (ve mien phi la hop le)", () => {
    const { container } = render(<TicketTierCard {...BASE} unitAmount={0} />);

    expect(allText(container)).toContain("0 đ");
  });

  it("gia khong huu han -> khong bao gio ro ri 'NaN'", () => {
    const { container } = render(<TicketTierCard {...BASE} unitAmount={Number.NaN} />);

    expect(allText(container)).not.toContain("NaN");
  });

  it("hien ten hang ve", () => {
    const { container } = render(<TicketTierCard {...BASE} />);

    expect(allText(container)).toContain(NAME);
  });
});

describe("TicketTierCard — ton kho: nhan khai quat, TUYET DOI khong co con so (AC-4, BR-O1)", () => {
  it("ba nhan cho ra ba thong diep khac nhau doi mot, deu khong rong", () => {
    const texts = AVAILABILITIES.map((availability) => {
      const { container, unmount } = render(
        <TicketTierCard {...BASE} availability={availability} />,
      );
      const text = allText(container).replace(NAME, " ").replace(PRICE_TEXT, " ");
      unmount();
      return norm(text);
    });

    expect(new Set(texts).size).toBe(AVAILABILITIES.length);
    for (const text of texts) {
      expect(text.length).toBeGreaterThan(2);
    }
  });

  it("`limited` -> nhan dinh tinh, KHONG chua bat ky chu so nao", () => {
    const { container } = render(<TicketTierCard {...BASE} availability="limited" />);

    // Bo chuoi gia ra khoi text; phan con lai la ten hang + nhan ton kho.
    // Con bat ky chu so nao o day nghia la component dang khang dinh mot con so ve.
    const withoutPrice = allText(container).split(PRICE_TEXT).join(" ");

    expect(withoutPrice).not.toMatch(/\d/);
  });

  it("`available` va `sold_out` cung khong chua con so nao", () => {
    for (const availability of ["available", "sold_out"] as TierAvailability[]) {
      const { container, unmount } = render(
        <TicketTierCard {...BASE} availability={availability} />,
      );

      expect(allText(container).split(PRICE_TEXT).join(" ")).not.toMatch(/\d/);
      unmount();
    }
  });

  it("caller co the lo truyen prop so ve con lai -> con so do KHONG duoc hien o dau", () => {
    // Mo phong dung mot caller "tot bung": server co `remaining` thi truyen vao cho dep.
    // Component khong duoc co prop nay, va kho co cach nao de con so 7 ro ri ra UI.
    const sneaky = {
      remaining: 7,
      remainingCount: 7,
      available: 7,
      stock: 7,
      quotaLeft: 7,
    } as unknown as Partial<TicketTierCardProps>;

    const { container } = render(
      <TicketTierCard {...BASE} availability="limited" {...sneaky} />,
    );

    expect(allText(container)).not.toContain("7");
  });

  it("`sold_out` noi ro la het ve", () => {
    const { container } = render(<TicketTierCard {...BASE} availability="sold_out" />);

    expect(allText(container)).toMatch(/h[ếe]t v[ée]|h[ếe]t ch[ỗo]|s[ốo]ld.?out/i);
  });

  it("thieu `availability` -> hien 'dang cap nhat' (handoff §5 nguyen tac 3)", () => {
    const { container } = render(<TicketTierCard {...BASE} />);

    expect(allText(container)).toMatch(/đang c[ậa]p nh[ậa]t/i);
  });

  it("thieu `availability` -> KHONG khang dinh con ve", () => {
    const { container } = render(<TicketTierCard {...BASE} />);
    const text = allText(container);

    expect(text).not.toMatch(/c[òo]n v[ée]|c[òo]n ch[ỗo]|s[ẵa]n s[àa]ng|available/i);
  });

  it("thieu `availability` khac han `available`: hai text khong duoc giong nhau", () => {
    const unknown = render(<TicketTierCard {...BASE} />);
    const unknownText = allText(unknown.container);
    unknown.unmount();

    const available = render(<TicketTierCard {...BASE} availability="available" />);
    const availableText = allText(available.container);

    expect(unknownText).not.toBe(availableText);
  });
});

describe("TicketTierCard — gioi han mua do SERVER tinh, khong co default 4 (AC-5, BR-O4)", () => {
  it("thieu `maxSelectable` -> KHONG co widget chon so luong nao", () => {
    const { container } = render(
      <TicketTierCard {...BASE} availability="available" onQuantityChange={vi.fn()} />,
    );

    expect(quantityWidgets(container)).toHaveLength(0);
  });

  it("thieu `maxSelectable` -> bam het moi nut cung khong goi `onQuantityChange`", () => {
    const onQuantityChange = vi.fn();
    const { container } = render(
      <TicketTierCard {...BASE} availability="available" onQuantityChange={onQuantityChange} />,
    );

    clickEveryButton(container);

    expect(onQuantityChange).not.toHaveBeenCalled();
  });

  it("thieu `maxSelectable` -> khong he hien so '4' o dau (khong doan default cua he thong)", () => {
    const { container } = render(
      <TicketTierCard {...BASE} availability="available" onQuantityChange={vi.fn()} />,
    );

    expect(allText(container).split(PRICE_TEXT).join(" ")).not.toContain("4");
  });

  it("`maxSelectable={0}` -> KHONG render dieu khien (nguoi nay het suat mua)", () => {
    const onQuantityChange = vi.fn();
    const { container } = render(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={0}
        quantity={0}
        onQuantityChange={onQuantityChange}
      />,
    );

    expect(quantityWidgets(container)).toHaveLength(0);

    clickEveryButton(container);
    expect(onQuantityChange).not.toHaveBeenCalled();
  });

  it("`maxSelectable={0}` -> co giai thich bang chu, khong de khach doan vi sao", () => {
    const withLimit = render(
      <TicketTierCard {...BASE} availability="available" maxSelectable={0} quantity={0} />,
    );
    const withLimitText = allText(withLimit.container);
    withLimit.unmount();

    const withAllowance = render(
      <TicketTierCard {...BASE} availability="available" maxSelectable={3} quantity={0} />,
    );

    expect(withLimitText).not.toBe(allText(withAllowance.container));
    expect(withLimitText.replace(NAME, " ").replace(PRICE_TEXT, " ").trim().length).toBeGreaterThan(2);
  });

  it("`maxSelectable={3}` -> co dieu khien chon so luong", () => {
    const { container } = render(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={3}
        quantity={0}
        onQuantityChange={vi.fn()}
      />,
    );

    expect(quantityWidgets(container).length).toBeGreaterThan(0);
    expect(findButton(container, INCREASE)).toBeDefined();
    expect(findButton(container, DECREASE)).toBeDefined();
  });

  it("bam tang -> goi `onQuantityChange` voi so MOI", () => {
    const onQuantityChange = vi.fn();
    const { container } = render(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={3}
        quantity={1}
        onQuantityChange={onQuantityChange}
      />,
    );

    fireEvent.click(findButton(container, INCREASE) as HTMLElement);

    expect(onQuantityChange).toHaveBeenCalledWith(2);
  });

  it("bam giam -> goi `onQuantityChange` voi so MOI", () => {
    const onQuantityChange = vi.fn();
    const { container } = render(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={3}
        quantity={2}
        onQuantityChange={onQuantityChange}
      />,
    );

    fireEvent.click(findButton(container, DECREASE) as HTMLElement);

    expect(onQuantityChange).toHaveBeenCalledWith(1);
  });

  it("gia tri do prop quyet dinh: bam tang hai lan voi `quantity={1}` co dinh -> ca hai lan deu la 2", () => {
    // Component khong duoc giu so luong trong state rieng: so thuc nam o phia goi
    // (va cuoi cung la o server). Giu ban sao noi bo thi UI va don hang lech nhau.
    const onQuantityChange = vi.fn();
    const { container } = render(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={3}
        quantity={1}
        onQuantityChange={onQuantityChange}
      />,
    );

    const increase = findButton(container, INCREASE) as HTMLElement;
    fireEvent.click(increase);
    fireEvent.click(increase);

    expect(onQuantityChange).toHaveBeenNthCalledWith(1, 2);
    expect(onQuantityChange).toHaveBeenNthCalledWith(2, 2);
  });

  it("o toi da -> nut tang bi disable", () => {
    const { container } = render(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={3}
        quantity={3}
        onQuantityChange={vi.fn()}
      />,
    );

    expect(findButton(container, INCREASE)).toBeDisabled();
  });

  it("o toi da -> bam tang khong goi `onQuantityChange` (khong vuot qua 3)", () => {
    const onQuantityChange = vi.fn();
    const { container } = render(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={3}
        quantity={3}
        onQuantityChange={onQuantityChange}
      />,
    );

    clickEveryButton(container);

    expect(onQuantityChange).not.toHaveBeenCalledWith(4);
    for (const call of onQuantityChange.mock.calls) {
      expect(call[0]).toBeLessThanOrEqual(3);
    }
  });

  it("o 0 -> nut giam bi disable", () => {
    const { container } = render(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={3}
        quantity={0}
        onQuantityChange={vi.fn()}
      />,
    );

    expect(findButton(container, DECREASE)).toBeDisabled();
  });

  it("o 0 -> khong bao gio goi `onQuantityChange` voi so am", () => {
    const onQuantityChange = vi.fn();
    const { container } = render(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={3}
        quantity={0}
        onQuantityChange={onQuantityChange}
      />,
    );

    clickEveryButton(container);

    for (const call of onQuantityChange.mock.calls) {
      expect(call[0]).toBeGreaterThanOrEqual(0);
    }
  });

  it("`quantity` vuot `maxSelectable` (server vua ha gioi han) -> van khong cho tang them", () => {
    const onQuantityChange = vi.fn();
    const { container } = render(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={2}
        quantity={5}
        onQuantityChange={onQuantityChange}
      />,
    );

    clickEveryButton(container);

    for (const call of onQuantityChange.mock.calls) {
      expect(call[0]).toBeLessThanOrEqual(5);
    }
    expect(onQuantityChange).not.toHaveBeenCalledWith(6);
  });
});

describe("TicketTierCard — dieu khien so luong phai tiep can duoc (AC-5)", () => {
  it("nut tang va nut giam deu co ten doc duoc, khong rong", () => {
    const { container } = render(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={3}
        quantity={1}
        onQuantityChange={vi.fn()}
      />,
    );

    expect(accName(findButton(container, INCREASE) as HTMLElement).length).toBeGreaterThan(0);
    expect(accName(findButton(container, DECREASE) as HTMLElement).length).toBeGreaterThan(0);
  });

  it("gia tri hien tai den duoc screen reader (khong bi nhet trong nhanh aria-hidden)", () => {
    const { container } = render(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={3}
        quantity={2}
        onQuantityChange={vi.fn()}
      />,
    );

    const spinbutton = container.querySelector('[role="spinbutton"]');
    const announced =
      spinbutton?.getAttribute("aria-valuenow") === "2" ||
      atText(container).split(PRICE_TEXT).join(" ").includes("2");

    expect(announced).toBe(true);
  });

  it("gia tri hien tai doi theo prop `quantity`", () => {
    const { container, rerender } = render(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={3}
        quantity={2}
        onQuantityChange={vi.fn()}
      />,
    );
    const before = allText(container).split(PRICE_TEXT).join(" ");

    rerender(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={3}
        quantity={3}
        onQuantityChange={vi.fn()}
      />,
    );

    expect(allText(container).split(PRICE_TEXT).join(" ")).not.toBe(before);
  });
});

describe("TicketTierCard — het ve / bi disable thi khong cho chon (AC-5)", () => {
  it("`availability=\"sold_out\"` -> khong render dieu khien, khong goi `onQuantityChange`", () => {
    const onQuantityChange = vi.fn();
    const { container } = render(
      <TicketTierCard
        {...BASE}
        availability="sold_out"
        maxSelectable={3}
        quantity={0}
        onQuantityChange={onQuantityChange}
      />,
    );

    expect(quantityWidgets(container)).toHaveLength(0);

    clickEveryButton(container);
    expect(onQuantityChange).not.toHaveBeenCalled();
  });

  it("`disabled` -> khong render dieu khien, khong goi `onQuantityChange`", () => {
    const onQuantityChange = vi.fn();
    const { container } = render(
      <TicketTierCard
        {...BASE}
        availability="available"
        disabled
        maxSelectable={3}
        quantity={0}
        onQuantityChange={onQuantityChange}
      />,
    );

    expect(quantityWidgets(container)).toHaveLength(0);

    clickEveryButton(container);
    expect(onQuantityChange).not.toHaveBeenCalled();
  });

  it("`sold_out` co `maxSelectable` lon van khong mo duong chon", () => {
    const onQuantityChange = vi.fn();
    const { container } = render(
      <TicketTierCard
        {...BASE}
        availability="sold_out"
        maxSelectable={10}
        quantity={0}
        onQuantityChange={onQuantityChange}
      />,
    );

    clickEveryButton(container);

    expect(onQuantityChange).not.toHaveBeenCalled();
  });

  it("`sold_out` van hien ten hang va gia (giu bo cuc, khong an the)", () => {
    const { container } = render(<TicketTierCard {...BASE} availability="sold_out" />);
    const text = allText(container);

    expect(text).toContain(NAME);
    expect(text).toContain(PRICE_TEXT);
  });
});

/**
 * `sold_out` KHONG phai mot chieu — BR-E3 + BR-O1.
 *
 * BR-O1 (docs/01 dong 84): "Redis lech so khong the gay oversell — chi co the gay 'bao
 * het ve som', va JOB DOI SOAT SE SUA LAI." BR-E3 (dong 30): sau `ON_SALE` chi duoc
 * TANG quota (`inventory.increased`).
 *
 * Nghia la `availability` cua mot hang ve co the quay tu `sold_out` ve `available`
 * NGAY TRONG PHIEN cua khach, va day la duong hoi phuc duoc thiet ke san trong nghiep
 * vu, khong phai ngoai le. Neu component nho rang "da tung thay sold_out" roi khoa
 * dieu khien chon lai, khach dang nhin mot hang ve CON VE ma khong mua duoc — ta tu
 * danh mat chinh so ve ma job doi soat vua tra lai.
 */
describe("TicketTierCard — sold_out khong mot chieu, phai hoi phuc duoc (AC-4, BR-E3, BR-O1)", () => {
  it("sold_out -> rerender available: dieu khien chon so luong quay lai", () => {
    const { container, rerender } = render(
      <TicketTierCard
        {...BASE}
        availability="sold_out"
        maxSelectable={3}
        quantity={0}
        onQuantityChange={vi.fn()}
      />,
    );
    expect(quantityWidgets(container)).toHaveLength(0);

    rerender(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={3}
        quantity={0}
        onQuantityChange={vi.fn()}
      />,
    );

    expect(quantityWidgets(container).length).toBeGreaterThan(0);
  });

  it("sold_out -> rerender available: `onQuantityChange` goi duoc binh thuong", () => {
    const onQuantityChange = vi.fn();
    const { container, rerender } = render(
      <TicketTierCard
        {...BASE}
        availability="sold_out"
        maxSelectable={3}
        quantity={0}
        onQuantityChange={onQuantityChange}
      />,
    );

    rerender(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={3}
        quantity={0}
        onQuantityChange={onQuantityChange}
      />,
    );

    fireEvent.click(findButton(container, INCREASE) as HTMLElement);

    expect(onQuantityChange).toHaveBeenCalledWith(1);
  });

  it("sold_out -> rerender available: nhan ton kho doi theo, khong giu 'het ve'", () => {
    const { container, rerender } = render(
      <TicketTierCard {...BASE} availability="sold_out" />,
    );

    rerender(<TicketTierCard {...BASE} availability="available" />);

    expect(allText(container)).not.toMatch(/h[ếe]t v[ée]|h[ếe]t ch[ỗo]|s[ốo]ld.?out/i);
  });

  it("available -> sold_out -> available: ve dung trang thai ban dau", () => {
    const reference = render(<TicketTierCard {...BASE} availability="available" />);
    const referenceText = allText(reference.container);
    reference.unmount();

    const { container, rerender } = render(
      <TicketTierCard {...BASE} availability="available" />,
    );
    rerender(<TicketTierCard {...BASE} availability="sold_out" />);
    rerender(<TicketTierCard {...BASE} availability="available" />);

    expect(allText(container)).toBe(referenceText);
  });
});

describe("TicketTierCard — dang co hold o hang khac: phai canh bao (AC-6, BR-O3)", () => {
  it("`hasActiveHoldElsewhere` -> canh bao bang CHU noi cach doi duoc hang: thanh toan hoac het han", () => {
    const { container } = render(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={3}
        quantity={0}
        hasActiveHoldElsewhere
        onQuantityChange={vi.fn()}
      />,
    );

    expect(allText(container)).toMatch(/thanh to[áa]n/i);
    expect(allText(container)).toMatch(/h[ếe]t h[ạa]n/i);
  });

  it("canh bao KHONG duoc noi hold dang co se bi huy (BR-O3: bam lai nhan lai dung hold cu)", () => {
    const { container } = render(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={3}
        quantity={0}
        hasActiveHoldElsewhere
        onQuantityChange={vi.fn()}
      />,
    );

    expect(allText(container)).not.toMatch(/hu[ỷyỳ]/i);
    expect(allText(container)).not.toMatch(/tr[ảa] (l[ạa]i )?kho/i);
  });

  it("canh bao noi ro la lien quan den ve dang giu", () => {
    const { container } = render(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={3}
        quantity={0}
        hasActiveHoldElsewhere
        onQuantityChange={vi.fn()}
      />,
    );

    expect(allText(container)).toMatch(/gi[ữu] v[ée]|đang gi[ữu]|hold/i);
  });

  it("KHONG `hasActiveHoldElsewhere` -> khong co canh bao do (test phan biet duoc hai nhanh)", () => {
    const { container } = render(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={3}
        quantity={0}
        onQuantityChange={vi.fn()}
      />,
    );

    expect(allText(container)).not.toMatch(/đang gi[ữu] v[ée] [ởo] h[ạa]ng kh[áa]c/i);
    expect(allText(container)).not.toMatch(/h[ếe]t h[ạa]n/i);
  });

  it("canh bao la text thuc su, khong phai chi mot thuoc tinh mau/data", () => {
    const withWarning = render(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={3}
        quantity={0}
        hasActiveHoldElsewhere
        onQuantityChange={vi.fn()}
      />,
    );
    const withWarningText = allText(withWarning.container);
    withWarning.unmount();

    const without = render(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={3}
        quantity={0}
        onQuantityChange={vi.fn()}
      />,
    );

    // Them canh bao phai LAM DAI phan chu ra, khong chi doi thuoc tinh trinh bay.
    expect(withWarningText.length).toBeGreaterThan(allText(without.container).length);
  });

  it("canh bao den duoc screen reader (khong nam trong nhanh aria-hidden)", () => {
    const { container } = render(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={3}
        quantity={0}
        hasActiveHoldElsewhere
        onQuantityChange={vi.fn()}
      />,
    );

    expect(atText(container)).toMatch(/h[ếe]t h[ạa]n/i);
  });

  it("canh bao khong lam mat dieu khien chon so luong (canh bao, khong phai chan)", () => {
    // AC-6 chi doi CANH BAO, khong doi khoa lua chon. Sau khi BR-O3 duoc sua (2026-10-08)
    // thi chon hang khac khong doi duoc hold -- khoa dieu khien la mot quyet dinh UX rieng,
    // CHUA lam; test nay ghim hanh vi hien tai de quyet dinh do phai doi no co chu y.
    const { container } = render(
      <TicketTierCard
        {...BASE}
        availability="available"
        maxSelectable={3}
        quantity={0}
        hasActiveHoldElsewhere
        onQuantityChange={vi.fn()}
      />,
    );

    expect(quantityWidgets(container).length).toBeGreaterThan(0);
  });
});

describe("TicketTierCard — trang thai duoc chon phai den duoc screen reader (AC-7)", () => {
  /** Co phan tu nao dang bao "toi dang duoc chon" theo chuan ARIA khong? */
  function hasSelectedSignal(container: HTMLElement): boolean {
    return (
      container.querySelector('[aria-pressed="true"]') !== null ||
      container.querySelector('[aria-selected="true"]') !== null
    );
  }

  it("`selected` -> co `aria-pressed=\"true\"` HOAC `aria-selected=\"true\"`", () => {
    const { container } = render(
      <TicketTierCard {...BASE} availability="available" selected />,
    );

    expect(hasSelectedSignal(container)).toBe(true);
  });

  it("khong `selected` -> khong co tin hieu da chon nao", () => {
    const { container } = render(<TicketTierCard {...BASE} availability="available" />);

    expect(hasSelectedSignal(container)).toBe(false);
  });

  it("control mang tin hieu do co trang thai ro rang ca khi chua chon (false, khong phai thieu)", () => {
    const selected = render(<TicketTierCard {...BASE} availability="available" selected />);
    const attribute =
      selected.container.querySelector('[aria-pressed="true"]') !== null
        ? "aria-pressed"
        : "aria-selected";
    selected.unmount();

    const unselected = render(<TicketTierCard {...BASE} availability="available" />);

    expect(unselected.container.querySelector(`[${attribute}]`)?.getAttribute(attribute)).toBe(
      "false",
    );
  });

  it("doi `selected` khi rerender thi tin hieu doi theo", () => {
    const { container, rerender } = render(
      <TicketTierCard {...BASE} availability="available" />,
    );
    expect(hasSelectedSignal(container)).toBe(false);

    rerender(<TicketTierCard {...BASE} availability="available" selected />);

    expect(hasSelectedSignal(container)).toBe(true);
  });

  it("`selected` cung hien ra bang chu / cau truc, khong chi mot thuoc tinh ARIA", () => {
    const { container } = render(
      <TicketTierCard {...BASE} availability="available" selected />,
    );
    const marker = container.querySelector('[aria-pressed="true"], [aria-selected="true"]');

    expect(marker).not.toBeNull();
    expect(accName(marker as Element).length).toBeGreaterThan(0);
  });
});
