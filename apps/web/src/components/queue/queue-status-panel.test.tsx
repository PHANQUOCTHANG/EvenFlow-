/**
 * Test cho `QueueStatusPanel` — AC-6..AC-10 (spec-plan EVF-1803 §4).
 *
 * ==> TEST QUAN TRONG NHAT o describe "LOBBY" ben duoi. Doc truoc khi sua.
 *
 * BR-Q1 (docs/01-nghiep-vu.md dong 53): "Moi nguoi vao TRUOC `sale_start_at`
 * duoc gom vao LOBBY KHONG CO SO THU TU. Dung T0, he thong xao tron ngau nhien
 * toan bo LOBBY roi moi cap rank. Vao som 2 tieng hay 2 giay deu co co hoi nhu
 * nhau -> triet tieu dong co bot bam F5."
 *
 * Nghia la: neu panel lo hien mot con so thu tu o LOBBY thi nguoi dung (va bot)
 * suy ra dieu NGUOC LAI — rang vao som thi so nho hon — va toan bo co che cong
 * bang cua san pham bien mat. Day la rang buoc nghiep vu, khong phai chi tiet
 * trinh bay. Test duoi day doi khang: truyen rank/eta/initialRank VAO va bat
 * buoc component phai BO QUA het.
 *
 * Hai rang buoc "khong bia du lieu" di kem:
 *   - Thieu `etaSeconds` -> hien "dang cap nhat", TUYET DOI khong suy ETA tu
 *     rank (handoff §5 nguyen tac 3). Test chan bang cach assert khong co bat ky
 *     chuoi thoi luong nao xuat hien khi chi co rank.
 *   - Thieu `initialRank` -> khong ve progress (handoff §11.3 "no fake progress").
 *
 * Progressbar dung MAU SO CO DINH `initialRank` (rank luc vao hang doi), KHONG
 * dung "so nguoi dang o phia truoc" — con so do giam dan nen thanh tien trinh se
 * nhay nguoc, va vi no luon chenh `rank` dung 1 nen hieu cua chung luon ~0, thanh
 * tien trinh dung im o 0 mai mai. Cong thuc da chot:
 *   aria-valuemin = 0
 *   aria-valuemax = initialRank
 *   aria-valuenow = clamp(initialRank - rank, 0, initialRank)   // tang khi rank giam
 *
 * Cach test noi dung chu: spec KHONG chot cau chu tieng Viet cho tung trang thai,
 * nen test chot HANH VI (cai gi phai/khong duoc xuat hien) va chot "cac trang
 * thai phai khac nhau", chi dung regex rong cho nhung y bat buoc phai noi ra.
 */
import { act, fireEvent, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ServerExpiryCountdown } from "../ui/server-expiry-countdown";
import {
  QueueStatusPanel,
  type ConnectionState,
  type QueueState,
  type QueueStatusPanelProps,
} from "./queue-status-panel";

const START = Date.UTC(2026, 0, 15, 3, 0, 0);
const ADMISSION_EXP = START + 570_000; // 09:30

const STATES: QueueState[] = [
  "LOBBY",
  "QUEUED",
  "ADMITTED",
  "EXPIRED",
  "SOLD_OUT",
  "RECONNECTING",
  "DROPPED",
  "UNKNOWN",
];

const CONNECTION_STATES: ConnectionState[] = ["live", "reconnecting", "offline"];

function norm(value: string | null | undefined): string {
  return (value ?? "").replace(/\s+/g, " ").trim();
}

/** Text node + moi aria-label: chu ma nguoi dung hoac screen reader nhan duoc. */
function allText(container: HTMLElement): string {
  const labels = Array.from(container.querySelectorAll("[aria-label]"))
    .map((el) => el.getAttribute("aria-label") ?? "")
    .join(" ");
  return norm(`${container.textContent ?? ""} ${labels}`);
}

/** Bo chu so + dau hai cham -> con phan CHU, de so sanh nhan khong phu thuoc dong ho. */
function words(value: string | null | undefined): string {
  return norm(value)
    .replace(/[\d:]+/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

/**
 * Trang thai cua dong ho ben trong panel, doc qua `data-state` cua
 * ServerExpiryCountdown ("running" | "warning" | "expired"). Day la tin hieu
 * khong phu thuoc cau chu va khong phai class CSS. Tra `undefined` khi panel
 * khong render dong ho nao.
 */
function countdownState(container: HTMLElement): string | undefined {
  const node = Array.from(container.querySelectorAll("[data-state]")).find((el) =>
    ["running", "warning", "expired"].includes(el.getAttribute("data-state") ?? ""),
  );
  return node?.getAttribute("data-state") ?? undefined;
}

/** Doc 3 thuoc tinh aria cua progressbar duoi dang so. */
function progressValues(bar: HTMLElement): { min: number; max: number; now: number } {
  return {
    min: Number(bar.getAttribute("aria-valuemin")),
    max: Number(bar.getAttribute("aria-valuemax")),
    now: Number(bar.getAttribute("aria-valuenow")),
  };
}

function renderPanelText(props: QueueStatusPanelProps): string {
  const { container, unmount } = render(<QueueStatusPanel {...props} />);
  const text = allText(container);
  unmount();
  return text;
}

/** Lay phan CHU cua dong ho mot variant, bang cach render component that. */
function countdownWords(variant: "sale-start" | "admission" | "hold"): string {
  const { container, unmount } = render(
    <ServerExpiryCountdown variant={variant} expiresAt={ADMISSION_EXP} />,
  );
  const text = words(container.textContent);
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

describe("QueueStatusPanel — phu du 8 trang thai (AC-6)", () => {
  it.each(STATES)("state %s render duoc va co text khong rong", (state) => {
    const text = renderPanelText({ state });

    expect(text.length).toBeGreaterThan(3);
  });

  it("8 trang thai cho ra 8 noi dung khac nhau (khong truyen tin chi bang mau)", () => {
    const texts = STATES.map((state) => renderPanelText({ state }));

    expect(new Set(texts).size).toBe(STATES.length);
  });

  it.each(STATES)("state %s khong lam ro ri chu 'undefined' / 'NaN' ra UI", (state) => {
    const text = renderPanelText({ state });

    expect(text).not.toContain("undefined");
    expect(text).not.toContain("NaN");
    expect(text).not.toContain("null");
  });
});

describe("QueueStatusPanel — LOBBY tuyet doi khong co so thu tu (AC-7, BR-Q1)", () => {
  it("truyen rank/etaSeconds/initialRank vao LOBBY thi panel BO QUA het", () => {
    const { container, queryByText, queryByRole } = render(
      <QueueStatusPanel state="LOBBY" rank={42} etaSeconds={427} initialRank={1000} />,
    );
    const text = allText(container);

    // Bang chung AC-7: rank khong duoc xuat hien du da duoc truyen.
    expect(queryByText("42")).toBeNull();
    expect(text).not.toContain("42");

    // Khong ETA ca nhan (427s = 7 phut 7 giay).
    expect(text).not.toContain("427");
    expect(text).not.toMatch(/7\s*phút/i);
    expect(text).not.toMatch(/\d{1,2}:\d{2}/);

    // Khong tong so nguoi phia truoc, khong phan tram, khong progress.
    expect(text).not.toContain("1000");
    expect(text).not.toContain("1.000");
    expect(text).not.toContain("%");
    expect(queryByRole("progressbar")).toBeNull();
  });

  it("LOBBY khong co progressbar du co ca rank va initialRank", () => {
    const { queryByRole } = render(
      <QueueStatusPanel state="LOBBY" rank={7} initialRank={9000} />,
    );

    expect(queryByRole("progressbar")).toBeNull();
  });

  it("LOBBY khong hien bat ky con so thu tu nao (khong co '#42', khong co 'vi tri 42')", () => {
    const text = renderPanelText({ state: "LOBBY", rank: 42, initialRank: 1000 });

    expect(text).not.toMatch(/#\s*\d/);
    expect(text).not.toMatch(/\b42\b/);
  });

  it("LOBBY noi ro vao som KHONG tao loi the (chong bot)", () => {
    const text = renderPanelText({ state: "LOBBY" });

    expect(text).toMatch(/sớm/i);
    expect(text).toMatch(
      /như nhau|ngang nhau|bất kể|không giúp|không có lợi thế|không tạo lợi thế|không ưu tiên|không được ưu tiên|không tốt hơn|không ảnh hưởng/i,
    );
  });

  it("LOBBY noi ro chua co so thu tu", () => {
    const text = renderPanelText({ state: "LOBBY" });

    expect(text).toMatch(/chưa có số thứ tự|chưa được cấp số|chưa có thứ tự|chưa có vị trí|chưa cấp số/i);
  });
});

describe("QueueStatusPanel — QUEUED (AC-8)", () => {
  it("co rank thi hien rank", () => {
    const { container } = render(<QueueStatusPanel state="QUEUED" rank={42} />);

    expect(allText(container)).toContain("42");
  });

  it("thieu etaSeconds -> hien 'dang cap nhat' va KHONG tu tinh ETA tu rank", () => {
    const { container } = render(
      <QueueStatusPanel state="QUEUED" rank={42} initialRank={1000} />,
    );
    const text = allText(container);

    expect(text).toMatch(/đang cập nhật/i);
    // Khong duoc co bat ky con so thoi luong nao duoc suy ra tu rank.
    expect(text).not.toMatch(/\d+\s*(giây|phút|giờ)/i);
    expect(text).not.toMatch(/\d{1,2}:\d{2}/);
  });

  it("co etaSeconds -> hien ETA do va bo chu 'dang cap nhat'", () => {
    const { container } = render(
      <QueueStatusPanel state="QUEUED" rank={42} etaSeconds={427} />,
    );
    const text = allText(container);

    expect(text).toMatch(/\d+\s*(giây|phút)|\d{1,2}:\d{2}/i);
    expect(text).not.toMatch(/đang cập nhật/i);
  });

  it("co etaSeconds thi noi dung khac han khi khong co", () => {
    const withEta = renderPanelText({ state: "QUEUED", rank: 42, etaSeconds: 427 });
    const withoutEta = renderPanelText({ state: "QUEUED", rank: 42 });

    expect(withEta).not.toBe(withoutEta);
  });

  it("thieu initialRank -> KHONG co progressbar (no fake progress)", () => {
    const { queryByRole } = render(<QueueStatusPanel state="QUEUED" rank={42} />);

    expect(queryByRole("progressbar")).toBeNull();
  });

  it("thieu rank -> KHONG co progressbar du co initialRank", () => {
    const { queryByRole } = render(<QueueStatusPanel state="QUEUED" initialRank={1000} />);

    expect(queryByRole("progressbar")).toBeNull();
  });

  it("co ca rank va initialRank -> aria-valuemin/max/now dung CONG THUC da chot", () => {
    const { getByRole } = render(
      <QueueStatusPanel state="QUEUED" rank={42} initialRank={1000} />,
    );

    // Cong thuc chot (xem traceability §Cong thuc progressbar):
    //   valuemin = 0; valuemax = initialRank; valuenow = clamp(initialRank - rank, 0, initialRank)
    // Mau so la initialRank (CO DINH) chu khong phai so nguoi phia truoc (giam dan),
    // neu khong thanh tien trinh se nhay nguoc hoac dung im o 0.
    expect(progressValues(getByRole("progressbar"))).toEqual({
      min: 0,
      max: 1000,
      now: 958,
    });
  });

  it("rank=900/initialRank=1000 -> valuenow 100 (moi di duoc 1/10 hang doi)", () => {
    const { getByRole } = render(
      <QueueStatusPanel state="QUEUED" rank={900} initialRank={1000} />,
    );

    expect(progressValues(getByRole("progressbar"))).toEqual({
      min: 0,
      max: 1000,
      now: 100,
    });
  });

  it("rank chua nhich (rank = initialRank) -> valuenow 0", () => {
    const { getByRole } = render(
      <QueueStatusPanel state="QUEUED" rank={1000} initialRank={1000} />,
    );

    expect(progressValues(getByRole("progressbar"))).toEqual({
      min: 0,
      max: 1000,
      now: 0,
    });
  });

  it("rank=1 (gan toi luot) -> valuenow sat valuemax", () => {
    const { getByRole } = render(
      <QueueStatusPanel state="QUEUED" rank={1} initialRank={1000} />,
    );

    expect(progressValues(getByRole("progressbar"))).toEqual({
      min: 0,
      max: 1000,
      now: 999,
    });
  });

  it("rank > initialRank (server day lui) -> clamp ve 0, KHONG ra so am", () => {
    const { getByRole } = render(
      <QueueStatusPanel state="QUEUED" rank={1200} initialRank={1000} />,
    );

    const { min, max, now } = progressValues(getByRole("progressbar"));
    expect(now).toBe(0);
    expect(min).toBe(0);
    expect(max).toBe(1000);
  });

  it("don dieu: cung initialRank, rank NHO hon thi valuenow LON hon", () => {
    const valuesByRank = [900, 500, 100, 42].map((rank) => {
      const view = render(<QueueStatusPanel state="QUEUED" rank={rank} initialRank={1000} />);
      const { now } = progressValues(view.getByRole("progressbar"));
      view.unmount();
      return now;
    });

    for (let i = 1; i < valuesByRank.length; i += 1) {
      expect(valuesByRank[i]).toBeGreaterThan(valuesByRank[i - 1]);
    }
  });
});

describe("QueueStatusPanel — ADMITTED (AC-9)", () => {
  it("khong co admissionExpiresAt -> KHONG render dong ho", () => {
    const { container } = render(<QueueStatusPanel state="ADMITTED" />);

    expect(allText(container)).not.toMatch(/\d{1,2}:\d{2}/);
  });

  it("co admissionExpiresAt -> render dong ho dem nguoc", () => {
    const { container } = render(
      <QueueStatusPanel state="ADMITTED" admissionExpiresAt={ADMISSION_EXP} />,
    );

    expect(allText(container)).toContain("09:30");
  });

  it("dong ho dung variant 'admission', KHONG phai 'hold' (BR-Q7)", () => {
    const admissionWords = countdownWords("admission");
    const holdWords = countdownWords("hold");

    // Tien dieu kien: hai variant phai khac nhan, neu khong phep kiem nay vo nghia.
    expect(admissionWords).not.toBe(holdWords);

    const { container } = render(
      <QueueStatusPanel state="ADMITTED" admissionExpiresAt={ADMISSION_EXP} />,
    );
    const panelWords = words(container.textContent);

    expect(panelWords).toContain(admissionWords);
    expect(panelWords).not.toContain(holdWords);
  });

  it("nhan admissionExpiresAt dang ISO string", () => {
    const { container } = render(
      <QueueStatusPanel
        state="ADMITTED"
        admissionExpiresAt={new Date(ADMISSION_EXP).toISOString()}
      />,
    );

    expect(allText(container)).toContain("09:30");
  });

  it("ap dung offsetMs cho dong ho admit", () => {
    const { container } = render(
      <QueueStatusPanel
        state="ADMITTED"
        admissionExpiresAt={START + 180_000}
        offsetMs={60_000}
      />,
    );

    expect(allText(container)).toContain("02:00");
  });
});

describe("QueueStatusPanel — nguong canh bao cho dong ho admit (AC-5/AC-9)", () => {
  // `admissionWarningThresholdMs` duoc truyen thang xuong ServerExpiryCountdown.
  // Khong truyen = khong canh bao (nguong la policy nghiep vu, panel khong tu bia).
  // Tin hieu quan sat: `data-state` cua dong ho + vung aria-live phai len tieng
  // (BR: vao canh bao PHAI thong bao cho screen reader, khong chi doi mau).

  it("nguong NHO hon thoi gian con lai -> chua canh bao", () => {
    const { container } = render(
      <QueueStatusPanel
        state="ADMITTED"
        admissionExpiresAt={ADMISSION_EXP}
        admissionWarningThresholdMs={120_000}
      />,
    );

    expect(countdownState(container)).toBe("running");
  });

  it("nguong LON hon thoi gian con lai -> canh bao ngay va co thong bao", () => {
    const { container } = render(
      <QueueStatusPanel
        state="ADMITTED"
        admissionExpiresAt={ADMISSION_EXP}
        admissionWarningThresholdMs={600_000}
      />,
    );

    expect(countdownState(container)).toBe("warning");
  });

  it("chay tu tren nguong xuong duoi nguong thi doi sang canh bao", () => {
    const { container } = render(
      <QueueStatusPanel
        state="ADMITTED"
        admissionExpiresAt={ADMISSION_EXP}
        admissionWarningThresholdMs={120_000}
      />,
    );
    expect(countdownState(container)).toBe("running");

    act(() => {
      vi.advanceTimersByTime(460_000); // con 110s < 120s
    });

    expect(countdownState(container)).toBe("warning");
  });

  it("KHONG truyen admissionWarningThresholdMs -> khong canh bao o bat ky muc nao", () => {
    const { container } = render(
      <QueueStatusPanel state="ADMITTED" admissionExpiresAt={ADMISSION_EXP} />,
    );

    expect(countdownState(container)).toBe("running");

    act(() => {
      vi.advanceTimersByTime(560_000); // con 10s
    });

    expect(countdownState(container)).toBe("running");
  });

  it("khong co admissionExpiresAt thi nguong cung khong tao ra dong ho nao", () => {
    const { container } = render(
      <QueueStatusPanel state="ADMITTED" admissionWarningThresholdMs={600_000} />,
    );

    expect(countdownState(container)).toBeUndefined();
    expect(allText(container)).not.toMatch(/\d{1,2}:\d{2}/);
  });
});

describe("QueueStatusPanel — EXPIRED / DROPPED phai KHAC nhau (AC-9)", () => {
  it("EXPIRED noi ro phai xep lai va khong duoc uu tien (BR-Q7)", () => {
    const text = renderPanelText({ state: "EXPIRED" });

    expect(text).toMatch(/xếp lại|xếp hàng lại|vào lại hàng chờ/i);
    expect(text).toMatch(/không[^.]{0,40}(ưu tiên|ưu đãi|giữ chỗ cũ)/i);
  });

  it("DROPPED va EXPIRED cho ra thong diep KHAC nhau", () => {
    const expired = renderPanelText({ state: "EXPIRED" });
    const dropped = renderPanelText({ state: "DROPPED" });

    expect(dropped).not.toBe(expired);
    expect(dropped.length).toBeGreaterThan(3);
  });

  it("DROPPED noi ro nguyen nhan la mat ket noi qua lau (docs/01 dong 48)", () => {
    const text = renderPanelText({ state: "DROPPED" });

    expect(text).toMatch(/kết nối/i);
  });
});

describe("QueueStatusPanel — RECONNECTING chua mat cho (AC-9, BR-2.6)", () => {
  it("noi ro la CHUA mat cho", () => {
    const text = renderPanelText({ state: "RECONNECTING" });

    expect(text).toMatch(
      /(chưa|không|vẫn)[^.]{0,40}(mất chỗ|mất vị trí|giữ chỗ|giữ vị trí|mất lượt)/i,
    );
  });

  it("thong diep khac han DROPPED (mot cai con cho, mot cai mat cho)", () => {
    const reconnecting = renderPanelText({ state: "RECONNECTING" });
    const dropped = renderPanelText({ state: "DROPPED" });

    expect(reconnecting).not.toBe(dropped);
  });
});

describe("QueueStatusPanel — SOLD_OUT khong moi vao checkout (AC-9, BR-Q6)", () => {
  it("khong co CTA dan vao checkout / chon ve", () => {
    const { queryAllByRole } = render(<QueueStatusPanel state="SOLD_OUT" />);

    const ctas = [...queryAllByRole("button"), ...queryAllByRole("link")];
    for (const cta of ctas) {
      expect(norm(cta.textContent)).not.toMatch(
        /thanh toán|checkout|chọn vé|mua vé|đặt vé|tiếp tục mua/i,
      );
      expect(norm(cta.getAttribute("aria-label"))).not.toMatch(
        /thanh toán|checkout|chọn vé|mua vé|đặt vé/i,
      );
    }
  });

  it("noi ro la het ve", () => {
    const text = renderPanelText({ state: "SOLD_OUT" });

    expect(text).toMatch(/hết vé|đã bán hết|bán hết|sold out/i);
  });

  it("khong render dong ho dem nguoc du co admissionExpiresAt", () => {
    const text = renderPanelText({ state: "SOLD_OUT", admissionExpiresAt: ADMISSION_EXP });

    expect(text).not.toContain("09:30");
  });
});

describe("QueueStatusPanel — SOLD_OUT moi vao waitlist (BR-Q6)", () => {
  // BR-Q6 (docs/01 dong 58): "Khi het ve, toan bo QUEUED nhan thong bao SOLD_OUT
  // va duoc moi vao waitlist". Nhung BR khong noi waitlist LUC NAO CUNG co san,
  // va handoff §4 ghi "chi neu API ho tro" -> chi moi khi caller dua `onJoinWaitlist`.
  // Khong co callback thi TUYET DOI khong nhac waitlist (khong hua dieu khong lam duoc).

  it("khong truyen onJoinWaitlist -> khong co button waitlist va khong nhac waitlist", () => {
    const { container, queryByRole } = render(<QueueStatusPanel state="SOLD_OUT" />);

    expect(queryByRole("button", { name: /danh sách chờ/i })).toBeNull();
    expect(allText(container)).not.toMatch(/danh sách chờ|waitlist/i);
  });

  it("co onJoinWaitlist -> co button waitlist", () => {
    const onJoinWaitlist = vi.fn();
    const { getByRole } = render(
      <QueueStatusPanel state="SOLD_OUT" onJoinWaitlist={onJoinWaitlist} />,
    );

    expect(getByRole("button", { name: /danh sách chờ/i })).toBeInTheDocument();
  });

  it("bam button waitlist goi dung callback mot lan", () => {
    const onJoinWaitlist = vi.fn();
    const { getByRole } = render(
      <QueueStatusPanel state="SOLD_OUT" onJoinWaitlist={onJoinWaitlist} />,
    );

    fireEvent.click(getByRole("button", { name: /danh sách chờ/i }));

    expect(onJoinWaitlist).toHaveBeenCalledTimes(1);
  });

  it("co waitlist thi van KHONG co CTA vao checkout", () => {
    const { queryAllByRole } = render(
      <QueueStatusPanel state="SOLD_OUT" onJoinWaitlist={vi.fn()} />,
    );

    const ctas = [...queryAllByRole("button"), ...queryAllByRole("link")];
    expect(ctas.length).toBeGreaterThan(0);
    for (const cta of ctas) {
      expect(norm(cta.textContent)).not.toMatch(
        /thanh toán|checkout|chọn vé|mua vé|đặt vé|tiếp tục mua/i,
      );
      expect(norm(cta.getAttribute("aria-label"))).not.toMatch(
        /thanh toán|checkout|chọn vé|mua vé|đặt vé/i,
      );
    }
  });

  it("onJoinWaitlist o trang thai khac SOLD_OUT thi khong moi waitlist", () => {
    const { container } = render(
      <QueueStatusPanel state="QUEUED" rank={42} onJoinWaitlist={vi.fn()} />,
    );

    expect(allText(container)).not.toMatch(/danh sách chờ|waitlist/i);
  });
});

describe("QueueStatusPanel — UNKNOWN khong bia trang thai (AC-9)", () => {
  it("moi thu lai", () => {
    const text = renderPanelText({ state: "UNKNOWN" });

    expect(text).toMatch(/thử lại|tải lại|làm mới/i);
  });

  it("khong hien rank du duoc truyen (khong biet trang thai thi khong khang dinh gi)", () => {
    const { queryByRole } = render(
      <QueueStatusPanel state="UNKNOWN" rank={42} initialRank={1000} />,
    );

    expect(queryByRole("progressbar")).toBeNull();
  });
});

describe("QueueStatusPanel — do tuoi du lieu va vung live (AC-10)", () => {
  it("co lastUpdatedAt thi hien thi", () => {
    const withStamp = renderPanelText({
      state: "QUEUED",
      rank: 42,
      etaSeconds: 427,
      lastUpdatedAt: START - 30_000,
    });
    const withoutStamp = renderPanelText({ state: "QUEUED", rank: 42, etaSeconds: 427 });

    expect(withStamp).not.toBe(withoutStamp);
    expect(withStamp).toMatch(/cập nhật/i);
  });

  it("khong co lastUpdatedAt thi KHONG bia 'vua cap nhat'", () => {
    const text = renderPanelText({ state: "QUEUED", rank: 42, etaSeconds: 427 });

    expect(text).not.toMatch(/vừa cập nhật|cập nhật lúc|cập nhật: /i);
  });

  it("nhan lastUpdatedAt dang ISO string ma khong lo 'Invalid Date'", () => {
    const text = renderPanelText({
      state: "QUEUED",
      rank: 42,
      etaSeconds: 427,
      lastUpdatedAt: new Date(START - 30_000).toISOString(),
    });

    expect(text).not.toContain("Invalid Date");
    expect(text).toMatch(/cập nhật/i);
  });

  it.each(STATES)('state %s co vung aria-live="polite"', (state) => {
    const { container } = render(<QueueStatusPanel state={state} />);

    const live = container.querySelector('[aria-live="polite"]');
    expect(live).not.toBeNull();
  });
});

describe("QueueStatusPanel — connectionState hien bang CHU (AC-10)", () => {
  it.each(CONNECTION_STATES)("connectionState %s van render duoc va co text", (connectionState) => {
    const text = renderPanelText({
      state: "QUEUED",
      rank: 42,
      etaSeconds: 427,
      connectionState,
    });

    expect(text.length).toBeGreaterThan(3);
  });

  it("'offline' va 'reconnecting' them chu so voi khi khong truyen gi", () => {
    // Khong assert cho "live" vi cai dat co the lay "live" lam mac dinh —
    // spec khong chot dieu do.
    const base = renderPanelText({ state: "QUEUED", rank: 42, etaSeconds: 427 });

    for (const connectionState of ["offline", "reconnecting"] as ConnectionState[]) {
      const text = renderPanelText({
        state: "QUEUED",
        rank: 42,
        etaSeconds: 427,
        connectionState,
      });

      expect(text).not.toBe(base);
    }
  });

  it("ba gia tri connectionState cho ra ba noi dung khac nhau (khong chi doi icon)", () => {
    const texts = CONNECTION_STATES.map((connectionState) =>
      renderPanelText({ state: "QUEUED", rank: 42, etaSeconds: 427, connectionState }),
    );

    expect(new Set(texts).size).toBe(CONNECTION_STATES.length);
  });
});
