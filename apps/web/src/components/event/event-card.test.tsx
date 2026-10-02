/**
 * Test cho `EventCard` — AC-2, AC-3 (spec-plan EVF-1802/1804 §4), BR-E1..E3, §3.
 *
 * ==> TEST QUAN TRONG NHAT o describe "sau trang thai cho ra sau thong diep".
 *
 * Vi sao: handoff §11.2 chi liet 4 nhan (`scheduled / on-sale / sold-out /
 * cancelled`), nhung state machine o `docs/01-nghiep-vu.md` dong 22-23 co SAU
 * trang thai cong khai: `SCHEDULED -> ON_SALE -> SOLD_OUT | CLOSED -> COMPLETED`
 * va nhanh `CANCELLED (hoan tien toan bo)`. `docs/01` la nguon chuan.
 *
 * Ba trang thai de bi gop lam mot la cho sai nghiep vu nang nhat:
 *   - `SOLD_OUT`  = het ve. Khach nen vao waitlist.
 *   - `CLOSED`    = het GIO ban, ve co the VAN CON. Khach khong con cach mua.
 *   - `COMPLETED` = su kien DA DIEN RA. Khach den noi thi khong co gi.
 * Gop ba cai nay thanh "Khong con ve" la noi sai voi khach o hai trong ba truong
 * hop. Vi vay test duoi day khong hard-code cau chu (spec khong chot cau chu) ma
 * chot "sau trang thai cho ra sau phan CHU khac nhau doi mot, va deu khong rong".
 *
 * `CANCELLED` la truong hop duy nhat spec BAT BUOC noi ra mot y cu the: duoc hoan
 * tien toan bo (docs/01 dong 23). Khach bi huy su kien ma khong duoc biet minh co
 * lay lai tien hay khong la thiet hai that -> test regex rong cho y nay.
 *
 * Hai "khong bia du lieu" di kem:
 *   - Thieu `href` -> KHONG render link. Mot `<a>` tro vao `undefined` la link
 *     chet; khach bam vao va di den trang loi.
 *   - Thieu `imageUrl` -> KHONG render `<img src="">`. Trinh duyet coi `src=""`
 *     la tro ve chinh trang hien tai -> tai lai ca trang.
 *
 * Skeleton (AC-3) duoc test theo huong doi khang: skeleton cua SAU trang thai phai
 * cho ra text y nhu nhau. Neu mot nhan trang thai nao lo ro ri ra khi dang
 * `loading`, sau chuoi do khong con bang nhau nua va test do.
 */
import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { EventCard, type EventCardProps, type EventPublicState } from "./event-card";

const STATES: EventPublicState[] = [
  "SCHEDULED",
  "ON_SALE",
  "SOLD_OUT",
  "CLOSED",
  "COMPLETED",
  "CANCELLED",
];

/** Bon trang thai "khong ban duoc nua": phai giam nhan nhung VAN hien thi. */
const BLOCKED_STATES: EventPublicState[] = ["SOLD_OUT", "CLOSED", "COMPLETED", "CANCELLED"];

const TITLE = "Dem nhac Mua Thu";
const VENUE = "Nha hat Lon";

/** Props co dinh cho moi phep so sanh giua cac trang thai: chi `state` duoc khac. */
const BASE: EventCardProps = {
  state: "ON_SALE",
  title: TITLE,
  venue: VENUE,
  startsAt: Date.UTC(2026, 10, 20, 12, 0, 0),
};

function norm(value: string | null | undefined): string {
  return (value ?? "").replace(/\s+/g, " ").trim();
}

/** Text node + moi `aria-label`: toan bo chu ma nguoi dung hoac screen reader nhan duoc. */
function allText(container: HTMLElement): string {
  const labels = Array.from(container.querySelectorAll("[aria-label]"))
    .map((el) => el.getAttribute("aria-label") ?? "")
    .join(" ");
  return norm(`${container.textContent ?? ""} ${labels}`);
}

/** Bo chu so va dau cham cau -> con phan CHU, de so sanh nhan khong phu thuoc ngay/gio. */
function words(value: string | null | undefined): string {
  return norm(value)
    .replace(/[\d:/.,-]+/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

/** Phan chu RIENG cua trang thai: bo ten + dia diem (giong nhau o moi trang thai). */
function stateWords(state: EventPublicState): string {
  const { container, unmount } = render(<EventCard {...BASE} state={state} />);
  const text = words(allText(container))
    .replace(words(TITLE), " ")
    .replace(words(VENUE), " ")
    .replace(/\s+/g, " ")
    .trim();
  unmount();
  return text;
}

function anchors(container: HTMLElement): HTMLAnchorElement[] {
  return Array.from(container.querySelectorAll("a"));
}

describe("EventCard — sau trang thai cho ra sau thong diep khac nhau (AC-2, docs/01 dong 22-23)", () => {
  it("render duoc ca sau trang thai cua docs/01 ma khong vo", () => {
    for (const state of STATES) {
      const { container, unmount } = render(<EventCard {...BASE} state={state} />);

      expect(allText(container)).toContain(TITLE);
      unmount();
    }
  });

  it("moi trang thai co phan chu rieng, khong rong (nhan la TEXT, khong chi mau)", () => {
    for (const state of STATES) {
      expect(stateWords(state).length).toBeGreaterThan(2);
    }
  });

  it("SOLD_OUT, CLOSED va COMPLETED cho ra BA thong diep khac nhau (het ve / het gio / da xong)", () => {
    const soldOut = stateWords("SOLD_OUT");
    const closed = stateWords("CLOSED");
    const completed = stateWords("COMPLETED");

    expect(soldOut).not.toBe(closed);
    expect(closed).not.toBe(completed);
    expect(soldOut).not.toBe(completed);
    expect(new Set([soldOut, closed, completed]).size).toBe(3);
  });

  it("ca sau trang thai khac nhau doi mot (khong trang thai nao bi gop)", () => {
    const texts = STATES.map(stateWords);

    expect(new Set(texts).size).toBe(STATES.length);
  });

  it("CANCELLED noi ro duoc hoan tien (docs/01 dong 23: 'hoan tien toan bo')", () => {
    const { container } = render(<EventCard {...BASE} state="CANCELLED" />);
    const text = allText(container);

    expect(text).toMatch(/ho[àa]n\s*(l[ạa]i\s*)?(ti[ềe]n|100)/i);
  });

  it("CANCELLED noi ro pham vi hoan la TOAN BO, khong de khach doan", () => {
    const { container } = render(<EventCard {...BASE} state="CANCELLED" />);

    expect(allText(container)).toMatch(/to[àa]n b[ộo]|100\s*%|đ[ầa]y đ[ủu]|t[ấa]t c[ảa]/i);
  });

  it("CANCELLED noi QUYEN duoc hoan, KHONG noi la DA hoan xong (docs/01 dong 78)", () => {
    // `docs/01` dong 78: `ISSUED --organizer huy su kien--> REFUNDING --> REFUNDED`.
    // Hoan tien la mot QUA TRINH co trang thai, khong phai mot su kien tuc thi. Mot
    // the su kien noi "da hoan tien" trong khi don con o `REFUNDING` la khang dinh
    // sai ve tien: khach kiem tai khoan khong thay gi va mo tranh chap.
    const { container } = render(<EventCard {...BASE} state="CANCELLED" />);

    expect(allText(container)).not.toMatch(/đ[ãa]\s*(đ[ưu][ợo]c\s*)?ho[àa]n\s*(l[ạa]i\s*)?ti[ềe]n/i);
  });

  it("trang thai con ban duoc KHONG noi gi ve hoan tien (khong canh bao sai)", () => {
    for (const state of ["SCHEDULED", "ON_SALE"] as EventPublicState[]) {
      const { container, unmount } = render(<EventCard {...BASE} state={state} />);

      expect(allText(container)).not.toMatch(/ho[àa]n\s*(l[ạa]i\s*)?ti[ềe]n/i);
      unmount();
    }
  });
});

describe("EventCard — bon trang thai chan van phai HIEN THI (AC-2, DESIGN.md giu bo cuc)", () => {
  it("SOLD_OUT / CLOSED / COMPLETED / CANCELLED van hien ten su kien", () => {
    for (const state of BLOCKED_STATES) {
      const { container, unmount } = render(<EventCard {...BASE} state={state} />);

      expect(allText(container)).toContain(TITLE);
      unmount();
    }
  });

  it("the khong bi an khoi DOM va khong bi an khoi screen reader", () => {
    for (const state of BLOCKED_STATES) {
      const { container, unmount } = render(<EventCard {...BASE} state={state} />);
      const root = container.firstElementChild as HTMLElement | null;

      expect(root).not.toBeNull();
      expect(root?.hasAttribute("hidden")).toBe(false);
      expect(root?.getAttribute("aria-hidden")).not.toBe("true");
      expect(root?.closest('[aria-hidden="true"]')).toBeNull();
      unmount();
    }
  });
});

/**
 * `SOLD_OUT` KHONG phai mot chieu — BR-E3 + BR-O1.
 *
 * BR-E3 (docs/01 dong 30): "Sau khi `ON_SALE`, cam giam `quota` cua hang ve; chi duoc
 * TANG (phat su kien `inventory.increased`)." BR-O1 (dong 84): "Redis lech so khong the
 * gay oversell — chi co the gay 'bao het ve som', va JOB DOI SOAT SE SUA LAI."
 *
 * Hai dieu nay cong lai: mot su kien dang `SOLD_OUT` co the quay ve `ON_SALE` ngay
 * trong phien cua khach — vi Organizer tang quota, hoac vi lan bao het ve do la bao
 * som va doi soat vua sua lai. Neu component nho rang "da tung thay SOLD_OUT" va khoa
 * nhan do lai, khach dang nhin mot su kien MO BAN ma van thay "Het ve" — ta tu tay
 * danh mat chinh cai doanh so vua mo lai.
 */
describe("EventCard — trang thai phai doi duoc theo prop, SOLD_OUT khong mot chieu (AC-2, BR-E3, BR-O1)", () => {
  /** Toan bo phan CHU cua mot the vua render moi, dung lam moc so sanh. */
  function freshWords(state: EventPublicState): string {
    const { container, unmount } = render(<EventCard {...BASE} state={state} />);
    const text = words(allText(container));
    unmount();
    return text;
  }

  it("SOLD_OUT -> rerender ON_SALE: noi dung doi het theo, khong giu nhan cu", () => {
    const { container, rerender } = render(<EventCard {...BASE} state="SOLD_OUT" />);
    expect(words(allText(container))).toBe(freshWords("SOLD_OUT"));

    rerender(<EventCard {...BASE} state="ON_SALE" />);

    expect(words(allText(container))).toBe(freshWords("ON_SALE"));
    expect(words(allText(container))).not.toBe(freshWords("SOLD_OUT"));
  });

  it("ON_SALE -> SOLD_OUT -> ON_SALE: ve dung trang thai ban dau", () => {
    const { container, rerender } = render(<EventCard {...BASE} state="ON_SALE" />);
    rerender(<EventCard {...BASE} state="SOLD_OUT" />);
    rerender(<EventCard {...BASE} state="ON_SALE" />);

    expect(words(allText(container))).toBe(freshWords("ON_SALE"));
  });

  it("tu SOLD_OUT doi sang BAT KY trang thai nao cung duoc (khong trang thai nao bi dinh)", () => {
    for (const state of STATES) {
      const { container, rerender, unmount } = render(
        <EventCard {...BASE} state="SOLD_OUT" />,
      );

      rerender(<EventCard {...BASE} state={state} />);
      expect(words(allText(container))).toBe(freshWords(state));

      unmount();
    }
  });
});

describe("EventCard — dung MOT link chinh, khong bao gio link chet (AC-2, handoff §11.2)", () => {
  it("co `href` -> dung mot link", () => {
    const { container } = render(<EventCard {...BASE} href="/su-kien/dem-nhac-mua-thu" />);

    expect(anchors(container)).toHaveLength(1);
  });

  it("link tro dung dia chi duoc truyen", () => {
    const { container } = render(<EventCard {...BASE} href="/su-kien/dem-nhac-mua-thu" />);

    expect(anchors(container)[0]).toHaveAttribute("href", "/su-kien/dem-nhac-mua-thu");
  });

  it("the KHONG phai mot link boc ca khoi (click target khong mo ho)", () => {
    const { container } = render(<EventCard {...BASE} href="/su-kien/x" />);
    const root = container.firstElementChild as HTMLElement;

    expect(root.tagName).not.toBe("A");
    // Link phai la mot phan cua the, khong phai chinh the: text trong link it hon
    // text ca the (neu bang nhau thi link da boc het, click vao dau cung dieu huong).
    expect(norm(anchors(container)[0]?.textContent).length).toBeLessThan(
      norm(root.textContent).length,
    );
  });

  it("link co ten doc duoc (khong phai anchor rong)", () => {
    const { container } = render(<EventCard {...BASE} href="/su-kien/x" />);
    const link = anchors(container)[0];
    const name = norm(link.getAttribute("aria-label") ?? link.textContent);

    expect(name.length).toBeGreaterThan(0);
  });

  it("thieu `href` -> KHONG co link nao (khong link chet)", () => {
    const { container, queryByRole } = render(<EventCard {...BASE} />);

    expect(queryByRole("link")).toBeNull();
    expect(anchors(container)).toHaveLength(0);
  });

  it("thieu `href` o MOI trang thai deu khong co link", () => {
    for (const state of STATES) {
      const { container, unmount } = render(<EventCard {...BASE} state={state} />);

      expect(anchors(container)).toHaveLength(0);
      unmount();
    }
  });

  it("khong bao gio render `<a>` thieu href hoac href rong", () => {
    for (const state of STATES) {
      const { container, unmount } = render(
        <EventCard {...BASE} state={state} href="/su-kien/x" />,
      );

      for (const anchor of anchors(container)) {
        expect(norm(anchor.getAttribute("href"))).not.toBe("");
        expect(anchor.hasAttribute("href")).toBe(true);
      }
      unmount();
    }
  });
});

describe("EventCard — vung anh: khong bao gio `<img src=''>` (AC-2)", () => {
  it("co `imageUrl` -> render anh voi src do", () => {
    const { container } = render(<EventCard {...BASE} imageUrl="/img/mua-thu.jpg" />);
    const images = Array.from(container.querySelectorAll("img"));

    expect(images).toHaveLength(1);
    expect(images[0]).toHaveAttribute("src", "/img/mua-thu.jpg");
  });

  it("co `imageUrl` -> anh co alt (rong hay khong deu duoc, nhung phai co thuoc tinh)", () => {
    const { container } = render(<EventCard {...BASE} imageUrl="/img/mua-thu.jpg" />);

    expect(container.querySelector("img")?.hasAttribute("alt")).toBe(true);
  });

  it("thieu `imageUrl` -> KHONG co `<img>` nao", () => {
    const { container } = render(<EventCard {...BASE} />);

    expect(container.querySelectorAll("img")).toHaveLength(0);
  });

  it("thieu `imageUrl` -> co placeholder va placeholder bi an khoi screen reader", () => {
    const { container } = render(<EventCard {...BASE} />);

    expect(container.querySelectorAll('[aria-hidden="true"]').length).toBeGreaterThan(0);
  });

  it("`imageUrl` rong hoac toan khoang trang cung khong duoc thanh `<img src=''>`", () => {
    for (const imageUrl of ["", "   "]) {
      const { container, unmount } = render(<EventCard {...BASE} imageUrl={imageUrl} />);

      for (const image of Array.from(container.querySelectorAll("img"))) {
        expect(norm(image.getAttribute("src")).length).toBeGreaterThan(0);
      }
      unmount();
    }
  });

  it("moi `<img>` o moi trang thai deu co src khong rong", () => {
    for (const state of STATES) {
      const { container, unmount } = render(
        <EventCard {...BASE} state={state} imageUrl="/img/mua-thu.jpg" />,
      );

      for (const image of Array.from(container.querySelectorAll("img"))) {
        expect(norm(image.getAttribute("src")).length).toBeGreaterThan(0);
      }
      unmount();
    }
  });
});

describe("EventCard — thoi diem bat dau: khong bao gio ro ri 'NaN'/'Invalid Date' (AC-2)", () => {
  it("nhan epoch ms", () => {
    const { container } = render(<EventCard {...BASE} startsAt={Date.UTC(2026, 10, 20, 12)} />);
    const text = allText(container);

    expect(text).not.toContain("NaN");
    expect(text).not.toContain("Invalid Date");
  });

  it("nhan chuoi ISO CO mui gio", () => {
    const iso = new Date(Date.UTC(2026, 10, 20, 12)).toISOString();
    const { container } = render(<EventCard {...BASE} startsAt={iso} />);
    const text = allText(container);

    expect(text).not.toContain("NaN");
    expect(text).not.toContain("Invalid Date");
  });

  it("moc khong doc duoc -> khong hien 'NaN'/'Invalid Date', va van hien ten su kien", () => {
    const { container } = render(<EventCard {...BASE} startsAt="khong-phai-ngay" />);
    const text = allText(container);

    expect(text).not.toContain("NaN");
    expect(text).not.toContain("Invalid Date");
    expect(text).toContain(TITLE);
  });

  it("thieu `startsAt` -> khong bia ngay va van render duoc", () => {
    const { container } = render(
      <EventCard state="ON_SALE" title={TITLE} venue={VENUE} />,
    );
    const text = allText(container);

    expect(text).toContain(TITLE);
    expect(text).not.toContain("NaN");
    expect(text).not.toContain("Invalid Date");
  });
});

describe("EventCard — skeleton khi `loading` (AC-3)", () => {
  it("co `aria-busy=\"true\"`", () => {
    const { container } = render(<EventCard {...BASE} loading />);

    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull();
  });

  it("KHONG render ten su kien hay dia diem", () => {
    const { container } = render(<EventCard {...BASE} loading />);
    const text = allText(container);

    expect(text).not.toContain(TITLE);
    expect(text).not.toContain(VENUE);
  });

  it("KHONG render link, du co `href`", () => {
    const { container, queryByRole } = render(<EventCard {...BASE} loading href="/su-kien/x" />);

    expect(queryByRole("link")).toBeNull();
    expect(anchors(container)).toHaveLength(0);
  });

  it("KHONG render anh, du co `imageUrl`", () => {
    const { container } = render(
      <EventCard {...BASE} loading imageUrl="/img/mua-thu.jpg" />,
    );

    expect(container.querySelectorAll("img")).toHaveLength(0);
  });

  it("KHONG ro ri nhan trang thai: skeleton cua ca sau trang thai cho ra text y nhau", () => {
    const texts = STATES.map((state) => {
      const { container, unmount } = render(<EventCard {...BASE} loading state={state} />);
      const text = allText(container);
      unmount();
      return text;
    });

    expect(new Set(texts).size).toBe(1);
  });

  it("KHONG ro ri nhan trang thai: khong chuoi nao cua trang thai that xuat hien", () => {
    const real = STATES.map(stateWords);

    const { container } = render(<EventCard {...BASE} loading state="CANCELLED" />);
    const skeleton = words(allText(container));

    for (const text of real) {
      expect(skeleton).not.toContain(text);
    }
  });

  it("KHONG render gia / so tien nao trong skeleton", () => {
    const { container } = render(<EventCard {...BASE} loading />);
    const text = allText(container);

    // Chot bang "khong co CHU SO nao": moi so tien deu phai co chu so, nen rang buoc nay
    // bao trum va khong phu thuoc hau to. KHONG assert `not.toContain("đ")`: copy skeleton
    // co the la "đang tai" va `đ` o do khong phai hau to tien te. Cung KHONG dung regex co
    // ranh gioi tu: `textContent` noi cac node lien nhau nen moi ranh gioi deu khong dang tin.
    expect(text).not.toMatch(/\d/);
  });

  it("thoat `loading` thi noi dung that quay lai", () => {
    const { container, rerender } = render(<EventCard {...BASE} loading href="/su-kien/x" />);
    expect(allText(container)).not.toContain(TITLE);

    rerender(<EventCard {...BASE} href="/su-kien/x" />);

    expect(allText(container)).toContain(TITLE);
    expect(anchors(container)).toHaveLength(1);
    expect(container.querySelector('[aria-busy="true"]')).toBeNull();
  });

  it("khong `loading` thi KHONG dat `aria-busy=\"true\"`", () => {
    const { container } = render(<EventCard {...BASE} />);

    expect(container.querySelector('[aria-busy="true"]')).toBeNull();
  });
});
