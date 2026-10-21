import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as i18nProvider from "@/components/i18n/locale-provider";
import * as i18nMessages from "@/lib/i18n/messages";

import { EventActionPanel } from "./event-action-panel";

// Mock next/navigation
const mockPush = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: mockPush,
  }),
}));

describe("EventActionPanel (EVF-111, BR-Q5, BR-O2)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("SCHEDULED va chua toi gio mo ban: hien thi countdown va nut disabled", () => {
    // 1 gio nua moi mo ban
    const futureTime = new Date(Date.now() + 3600_000).toISOString();

    render(
      <EventActionPanel
        eventId="evt-1"
        state="SCHEDULED"
        saleStartAt={futureTime}
      />,
    );

    expect(screen.getByText("Thời gian mở bán")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /Chưa tới giờ mở bán/i }),
    ).toBeDisabled();
  });

  it("ON_SALE: nut mo ban duoc kich hoat va bam vao se chay jitter", async () => {
    const onJoinQueue = vi.fn();
    const pastTime = new Date(Date.now() - 3600_000).toISOString();

    render(
      <EventActionPanel
        eventId="evt-1"
        state="ON_SALE"
        saleStartAt={pastTime}
        onJoinQueue={onJoinQueue}
        jitterOptions={{ minJitterMs: 0, maxJitterMs: 0 }}
      />,
    );

    const button = screen.getByRole("button", { name: /Vào phòng chờ mua vé/i });
    expect(button).toBeEnabled();

    fireEvent.click(button);

    await waitFor(() => {
      expect(onJoinQueue).toHaveBeenCalledWith("evt-1");
    });
  });

  it("dieu huong qua router.push khi khong truyen onJoinQueue", async () => {
    const pastTime = new Date(Date.now() - 3600_000).toISOString();

    render(
      <EventActionPanel
        eventId="evt-abc"
        state="ON_SALE"
        saleStartAt={pastTime}
        jitterOptions={{ minJitterMs: 0, maxJitterMs: 0 }}
      />,
    );

    const button = screen.getByRole("button", { name: /Vào phòng chờ mua vé/i });
    fireEvent.click(button);

    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith("/waiting/evt-abc");
    });
  });

  it("SOLD_OUT: nut bi vo hieu hoa voi thong bao het ve", () => {
    render(
      <EventActionPanel
        eventId="evt-1"
        state="SOLD_OUT"
        saleStartAt="2026-10-10T10:00:00+07:00"
      />,
    );

    const button = screen.getByRole("button", { name: /Đã hết vé/i });
    expect(button).toBeDisabled();
  });

  it("CANCELLED: nut bi vo hieu hoa voi thong bao su kien da huy", () => {
    render(
      <EventActionPanel
        eventId="evt-1"
        state="CANCELLED"
        saleStartAt="2026-10-10T10:00:00+07:00"
      />,
    );

    const button = screen.getByRole("button", { name: /Sự kiện đã huỷ/i });
    expect(button).toBeDisabled();
  });

  it("CLOSED: nut bi vo hieu hoa voi thong bao da dong ban", () => {
    render(
      <EventActionPanel
        eventId="evt-1"
        state="CLOSED"
        saleStartAt="2026-10-10T10:00:00+07:00"
      />,
    );

    const button = screen.getByRole("button", { name: /Đã đóng bán/i });
    expect(button).toBeDisabled();
  });
});

describe("EventActionPanel — chi ON_SALE moi co nut vao phong cho (AC-16)", () => {
  it.each(["DRAFT", "PENDING_REVIEW", "REJECTED", "COMPLETED", "CLOSED", "CANCELLED", "SOLD_OUT"] as const)(
    "%s: khong co nut vao phong cho, chi nut vo hieu hoa",
    (state) => {
      render(<EventActionPanel eventId="evt-1" state={state} saleStartAt="2026-10-10T10:00:00+07:00" />);
      const buttons = screen.getAllByRole("button");
      expect(buttons).toHaveLength(1);
      expect(buttons[0]).toBeDisabled();
      expect(screen.queryByRole("button", { name: /phòng chờ/i })).not.toBeInTheDocument();
    },
  );
});

// ---------------------------------------------------------------------------
// EV-188 (AC-8; plan §6 M2): EventActionPanel dich vi/en; phan tu goc co lang = locale
// (nam trong <main lang="vi">).
// ---------------------------------------------------------------------------
const LOCALE_KEY = "eventflow-locale";

/** Tu dien + provider gop lai cho cac helper ben duoi. */
const i18n = { ...i18nMessages, ...i18nProvider };

type I18n = typeof i18n;

/** Ban dich en cua chuoi vi (tim khoa theo gia tri). */
function enFor(m: I18n, viText: string): string {
  const keys = (Object.keys(m.vi) as Array<keyof typeof m.vi>).filter((k) => m.vi[k] === viText);
  expect(keys.length, `khong co khoa vi = ${viText}`).toBeGreaterThan(0);
  const values = new Set(keys.map((k) => m.en[k]));
  expect(values.size, `cac khoa vi = ${viText} co ban dich en khac nhau`).toBe(1);
  const [english] = Array.from(values);
  expect(english).not.toBe(viText);
  return english;
}

async function renderEn(m: I18n, ui: React.ReactElement) {
  window.localStorage.setItem(LOCALE_KEY, "en");
  const result = render(<m.LocaleProvider>{ui}</m.LocaleProvider>);
  await waitFor(() => expect(document.documentElement.lang).toBe("en"));
  return result;
}

const pastIso = () => new Date(Date.now() - 3600_000).toISOString();
const futureIso = () => new Date(Date.now() + 3600_000).toISOString();

describe("EventActionPanel — i18n (EV-188 AC-8)", () => {
  afterEach(() => {
    window.localStorage.removeItem(LOCALE_KEY);
    document.documentElement.setAttribute("lang", "vi");
  });

  it.each([["ON_SALE"], ["SOLD_OUT"], ["CANCELLED"], ["CLOSED"], ["SCHEDULED"]] as const)(
    'trang thai %s, khong provider: phan tu goc co lang="vi"',
    (state) => {
      const { container } = render(
        <EventActionPanel eventId="evt-1" state={state} saleStartAt={state === "SCHEDULED" ? futureIso() : pastIso()} />,
      );
      expect(container.firstElementChild).toHaveAttribute("lang", "vi");
    },
  );

  it('locale en + ON_SALE: nut vao phong cho la ban dich en, goc lang="en"', async () => {
    const m = i18n;
    const { container } = await renderEn(m, <EventActionPanel eventId="evt-1" state="ON_SALE" saleStartAt={pastIso()} />);
    expect(await screen.findByRole("button", { name: enFor(m, "Vào phòng chờ mua vé") })).toBeEnabled();
    expect(screen.queryByRole("button", { name: /Vào phòng chờ mua vé/ })).toBeNull();
    expect(container.firstElementChild).toHaveAttribute("lang", "en");
  });

  it.each([
    ["SOLD_OUT", "Đã hết vé"],
    ["CANCELLED", "Sự kiện đã huỷ"],
    ["CLOSED", "Đã đóng bán"],
    ["COMPLETED", "Đã đóng bán"],
  ] as const)("locale en + %s: nut vo hieu hoa hien ban dich en", async (state, viText) => {
    const m = i18n;
    const { container } = await renderEn(
      m,
      <EventActionPanel eventId="evt-1" state={state} saleStartAt="2026-10-10T10:00:00+07:00" />,
    );
    expect(await screen.findByRole("button", { name: enFor(m, viText) })).toBeDisabled();
    expect(container.firstElementChild).toHaveAttribute("lang", "en");
  });

  it("locale en + SCHEDULED chua toi gio: nhan va nut hien ban dich en", async () => {
    const m = i18n;
    await renderEn(m, <EventActionPanel eventId="evt-1" state="SCHEDULED" saleStartAt={futureIso()} />);
    expect(await screen.findByText(enFor(m, "Thời gian mở bán"))).toBeInTheDocument();
    expect(screen.getByRole("button", { name: enFor(m, "Chưa tới giờ mở bán") })).toBeDisabled();
    expect(screen.queryByText("Thời gian mở bán")).toBeNull();
  });

  it("locale en: trang thai jitter hien ban dich en", async () => {
    const m = i18n;
    const onJoinQueue = vi.fn();
    const { container } = await renderEn(
      m,
      <EventActionPanel
        eventId="evt-1"
        state="ON_SALE"
        saleStartAt={pastIso()}
        onJoinQueue={onJoinQueue}
        jitterOptions={{ minJitterMs: 60_000, maxJitterMs: 60_000 }}
      />,
    );

    fireEvent.click(await screen.findByRole("button", { name: enFor(m, "Vào phòng chờ mua vé") }));

    expect(await screen.findByText(enFor(m, "Đang điều phối lượt vào phòng chờ..."))).toBeInTheDocument();
    expect(screen.queryByText("Đang điều phối lượt vào phòng chờ...")).toBeNull();
    expect(container.firstElementChild).toHaveAttribute("lang", "en");
    expect(onJoinQueue).not.toHaveBeenCalled();
  });

  it("doi locale khi dang mo: goc panel doi lang va chu theo", async () => {
    const m = i18n;
    function SwitchToEn() {
      const { setLocale } = m.useLocale();
      return (
        <button type="button" onClick={() => setLocale("en")}>
          to-en
        </button>
      );
    }
    render(
      <m.LocaleProvider>
        <div data-testid="host">
          <EventActionPanel eventId="evt-1" state="SOLD_OUT" saleStartAt="2026-10-10T10:00:00+07:00" />
        </div>
        <SwitchToEn />
      </m.LocaleProvider>,
    );
    const root = () => screen.getByTestId("host").firstElementChild;
    expect(root()).toHaveAttribute("lang", "vi");

    fireEvent.click(screen.getByRole("button", { name: "to-en" }));

    await waitFor(() => expect(root()).toHaveAttribute("lang", "en"));
    expect(screen.getByRole("button", { name: enFor(m, "Đã hết vé") })).toBeDisabled();
  });
});

// ---------------------------------------------------------------------------
// EV-188 challenge F1 (major): o trang thai SCHEDULED, ServerExpiryCountdown co nhan tieng Viet co dinh
// ("Mở bán sau", cau sr-only, vung live). Khi goc panel la lang="en", moi doan chu tieng Viet trong panel
// phai co to tien [lang] gan nhat la "vi" (WCAG 3.1.2), ke ca chu sr-only ma screen reader doc.
// ---------------------------------------------------------------------------
/** Ky tu chi tieng Viet moi co (dau thanh / chu dac biet) — khong co trong chuoi tieng Anh. */
const VIETNAMESE_CHARS = /[ăâđêôơưàáảãạằắẳẵặầấẩẫậèéẻẽẹềếểễệìíỉĩịòóỏõọồốổỗộờớởỡợùúủũụừứửữựỳýỷỹỵ]/i;

/** Moi text node co chu tieng Viet trong `root` kem `lang` cua to tien [lang] gan nhat. */
function vietnameseTextLangs(root: Element): Array<{ text: string; lang: string | null }> {
  const out: Array<{ text: string; lang: string | null }> = [];
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    const text = (node.textContent ?? "").trim();
    if (!text || !VIETNAMESE_CHARS.test(text)) continue;
    out.push({ text, lang: node.parentElement?.closest("[lang]")?.getAttribute("lang") ?? null });
  }
  return out;
}

describe("EventActionPanel — SCHEDULED + en: chu tieng Viet cua dong ho nam trong lang='vi' (EV-188 challenge F1)", () => {
  afterEach(() => {
    window.localStorage.removeItem(LOCALE_KEY);
    document.documentElement.setAttribute("lang", "vi");
  });

  it("goc panel lang='en', dong ho van hien, va moi doan chu tieng Viet co lang hieu luc 'vi'", async () => {
    const m = i18n;
    const { container } = await renderEn(
      m,
      <EventActionPanel eventId="evt-1" state="SCHEDULED" saleStartAt={futureIso()} />,
    );
    await screen.findByText(enFor(m, "Thời gian mở bán"));
    const root = container.firstElementChild as HTMLElement;
    expect(root).toHaveAttribute("lang", "en");

    // Dong ho dem nguoc thuc su co mat (khong de test pass rong vi countdown bien mat).
    await waitFor(() => expect(root.textContent ?? "").toMatch(/\d+:\d{2}(:\d{2})?/));

    const offenders = vietnameseTextLangs(root).filter((entry) => entry.lang !== "vi");
    expect(offenders).toEqual([]);
  });

  it("mac dinh (vi): khong co doan chu tieng Viet nao bi khai bao ngon ngu khac", async () => {
    const { container } = render(<EventActionPanel eventId="evt-1" state="SCHEDULED" saleStartAt={futureIso()} />);
    const root = container.firstElementChild as HTMLElement;
    await waitFor(() => expect(root.textContent ?? "").toMatch(/\d+:\d{2}(:\d{2})?/));
    const entries = vietnameseTextLangs(root);
    expect(entries.length).toBeGreaterThan(0);
    expect(entries.filter((entry) => entry.lang !== "vi")).toEqual([]);
  });
});
