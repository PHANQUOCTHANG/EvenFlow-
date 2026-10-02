/**
 * Test cho `useServerCountdown` — AC-4 (spec-plan EVF-1803 §4), BR-O2.
 *
 * ==> TEST QUAN TRONG NHAT CUA CA SLICE nam o describe
 *     "tab bi background" ben duoi. Doc comment o do truoc khi sua gi.
 *
 * BR-O2: "Dong ho dem nguoc lay `expires_at` tu server; client khong tu tinh."
 * He qua ky thuat: moi nhip phai tinh LAI `expiresAt - now()`. Neu cai dat kieu
 * `remaining -= 1000` moi nhip thi khi tab bi background browser tiet lưu
 * `setInterval` xuong con ~1 lan/phut, so nhip chay it hon thoi gian thuc ->
 * dong ho hien cho khach NHIEU thoi gian hon so ho thuc su con. Khach tin vao
 * con so do, bam thanh toan muon, mat ve. Day la loi mat tien, khong phai loi
 * hien thi.
 *
 * Cach bat loi do: lam cho DONG HO TREO TUONG va SO NHIP KHONG KHOP nhau —
 * nhay `Date.now()` that nhieu (vi.setSystemTime) nhung chi cho timer chay
 * dung mot nhip (vi.advanceTimersByTime). Cai dat tinh lai se ra dung thoi
 * gian thuc; cai dat giam dan se ra con so gan nguyen ven -> test do.
 *
 * Luu y ve fake timer: `vi.setSystemTime` doi `Date.now()` va dich moc dao han
 * cua cac timer dang cho theo cung mot luong (hanh vi cua @sinonjs/fake-timers),
 * nen sau khi nhay dong ho phai `advanceTimersByTime` them moi co nhip chay.
 */
import { act, renderHook } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useServerCountdown } from "./use-server-countdown";

const START = Date.UTC(2026, 0, 15, 3, 0, 0);
const TEN_MIN = 600_000;

/**
 * Component tham do: in ket qua hook ra attribute de doc duoc ca trong markup
 * prerender (`renderToStaticMarkup`) — noi KHONG co effect nao chay.
 */
function Probe({
  expiresAt,
  offsetMs,
}: {
  expiresAt: number | string | null | undefined;
  offsetMs?: number;
}) {
  const { remainingMs, expired, ready } = useServerCountdown(expiresAt, offsetMs);
  return (
    <span
      data-ready={String(ready)}
      data-expired={String(expired)}
      data-remaining={String(remainingMs)}
    />
  );
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(START);
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("useServerCountdown — gia tri ban dau (AC-4)", () => {
  it("expiresAt = now + 10 phut -> remainingMs 600_000, expired false", () => {
    const { result } = renderHook(() => useServerCountdown(START + TEN_MIN));

    expect(result.current.remainingMs).toBe(TEN_MIN);
    expect(result.current.expired).toBe(false);
  });

  it("nhan expiresAt dang ISO string", () => {
    const iso = new Date(START + TEN_MIN).toISOString();
    const { result } = renderHook(() => useServerCountdown(iso));

    expect(result.current.remainingMs).toBe(TEN_MIN);
    expect(result.current.expired).toBe(false);
  });

  it("nhan expiresAt dang epoch ms (number)", () => {
    const { result } = renderHook(() => useServerCountdown(START + 90_000));

    expect(result.current.remainingMs).toBe(90_000);
  });

  it("tra ve dung hinh dang { remainingMs: number, expired: boolean }", () => {
    const { result } = renderHook(() => useServerCountdown(START + TEN_MIN));

    expect(typeof result.current.remainingMs).toBe("number");
    expect(typeof result.current.expired).toBe("boolean");
  });
});

describe("useServerCountdown — dem nguoc theo thoi gian troi (AC-4)", () => {
  it("troi 1 giay -> con 599_000 (co tick, khong dung yen)", () => {
    const { result } = renderHook(() => useServerCountdown(START + TEN_MIN));

    act(() => {
      vi.advanceTimersByTime(1_000);
    });

    expect(result.current.remainingMs).toBe(599_000);
    expect(result.current.expired).toBe(false);
  });

  it("troi 5 phut -> con khoang 300_000", () => {
    const { result } = renderHook(() => useServerCountdown(START + TEN_MIN));

    act(() => {
      vi.advanceTimersByTime(300_000);
    });

    expect(result.current.remainingMs).toBeGreaterThanOrEqual(299_000);
    expect(result.current.remainingMs).toBeLessThanOrEqual(300_000);
  });

  it("troi het 10 phut -> remainingMs 0 va expired true", () => {
    const { result } = renderHook(() => useServerCountdown(START + TEN_MIN));

    act(() => {
      vi.advanceTimersByTime(TEN_MIN + 1_000);
    });

    expect(result.current.remainingMs).toBe(0);
    expect(result.current.expired).toBe(true);
  });

  it("remainingMs khong bao gio am sau khi qua han", () => {
    const { result } = renderHook(() => useServerCountdown(START + 2_000));

    act(() => {
      vi.advanceTimersByTime(60_000);
    });

    expect(result.current.remainingMs).toBe(0);
  });
});

describe("useServerCountdown — tab bi background: tinh LAI tu moc, KHONG giam dan (AC-4, BR-O2)", () => {
  it("nhay dong ho 300s nhung chi chay 1 nhip -> remainingMs theo thoi gian THUC, khong theo so nhip", () => {
    const { result } = renderHook(() => useServerCountdown(START + TEN_MIN));
    expect(result.current.remainingMs).toBe(TEN_MIN);

    // Tab bi background: dong ho treo tuong nhay 300 giay, nhung browser chi
    // cho interval chay dung mot nhip (~1 giay) khi tab duoc danh thuc.
    act(() => {
      vi.setSystemTime(START + 300_000);
      vi.advanceTimersByTime(1_000);
    });

    // Thoi gian thuc da troi: 301s -> phai con ~299s.
    // Cai dat giam dan se bao con ~599s (chi tru di 1 nhip) -> sai 5 phut.
    expect(result.current.remainingMs).toBeGreaterThanOrEqual(298_000);
    expect(result.current.remainingMs).toBeLessThanOrEqual(300_000);
    expect(result.current.remainingMs).toBeLessThan(400_000);
  });

  it("nhay dong ho 9 phut, 1 nhip -> con ~1 phut, van chua expired", () => {
    const { result } = renderHook(() => useServerCountdown(START + TEN_MIN));

    act(() => {
      vi.setSystemTime(START + 540_000);
      vi.advanceTimersByTime(1_000);
    });

    expect(result.current.remainingMs).toBeGreaterThanOrEqual(58_000);
    expect(result.current.remainingMs).toBeLessThanOrEqual(60_000);
    expect(result.current.expired).toBe(false);
  });

  it("nhay dong ho qua han, 1 nhip -> expired true ngay (khong cho du so nhip)", () => {
    const { result } = renderHook(() => useServerCountdown(START + TEN_MIN));

    act(() => {
      vi.setSystemTime(START + TEN_MIN + 30_000);
      vi.advanceTimersByTime(1_000);
    });

    expect(result.current.expired).toBe(true);
    expect(result.current.remainingMs).toBe(0);
  });

  it("nhieu lan nhay dong ho lien tiep van khong tich luy sai so", () => {
    const { result } = renderHook(() => useServerCountdown(START + TEN_MIN));

    for (const jump of [100_000, 200_000, 300_000]) {
      act(() => {
        vi.setSystemTime(START + jump);
        vi.advanceTimersByTime(1_000);
      });
    }

    // Lan nhay cuoi dat dong ho ve START+300_000, cong 1 nhip 1s -> troi 301s.
    expect(result.current.remainingMs).toBeGreaterThanOrEqual(296_000);
    expect(result.current.remainingMs).toBeLessThanOrEqual(300_000);
  });
});

describe("useServerCountdown — moc da qua: expired ngay lan render dau (AC-4)", () => {
  it("expiresAt trong qua khu -> remainingMs 0, expired true, KHONG can tick", () => {
    const { result } = renderHook(() => useServerCountdown(START - 60_000));

    // Khong he goi advanceTimersByTime.
    expect(result.current.remainingMs).toBe(0);
    expect(result.current.expired).toBe(true);
  });

  it("expiresAt dung bang now -> da het han", () => {
    const { result } = renderHook(() => useServerCountdown(START));

    expect(result.current.remainingMs).toBe(0);
    expect(result.current.expired).toBe(true);
  });

  it("expiresAt dang ISO trong qua khu cung expired ngay", () => {
    const iso = new Date(START - 1_000).toISOString();
    const { result } = renderHook(() => useServerCountdown(iso));

    expect(result.current.expired).toBe(true);
  });
});

describe("useServerCountdown — chua co moc thi KHONG phai het han (AC-4, hop dong §5)", () => {
  it("expiresAt null -> remainingMs 0 nhung expired FALSE", () => {
    const { result } = renderHook(() => useServerCountdown(null));

    expect(result.current.remainingMs).toBe(0);
    expect(result.current.expired).toBe(false);
  });

  it("expiresAt undefined -> remainingMs 0 nhung expired FALSE", () => {
    const { result } = renderHook(() => useServerCountdown(undefined));

    expect(result.current.remainingMs).toBe(0);
    expect(result.current.expired).toBe(false);
  });

  it("string khong parse duoc -> giong null: remainingMs 0, expired FALSE", () => {
    // Day la khoang trong nguy hiem ve TIEN: neu coi `NaN` la "da het han" thi
    // server tra mot chuoi rac se lam nguoi dung bi bao mat hold trong khi ho
    // VAN con giu ghe. Khong biet moc != het moc.
    const { result } = renderHook(() => useServerCountdown("soon"));

    expect(result.current.remainingMs).toBe(0);
    expect(result.current.expired).toBe(false);
  });

  it("string rong cung khong phai het han", () => {
    const { result } = renderHook(() => useServerCountdown(""));

    expect(result.current.remainingMs).toBe(0);
    expect(result.current.expired).toBe(false);
  });

  it("string khong parse duoc + thoi gian troi van khong thanh expired", () => {
    const { result } = renderHook(() => useServerCountdown("not-a-date"));

    act(() => {
      vi.advanceTimersByTime(600_000);
    });

    expect(result.current.expired).toBe(false);
    expect(Number.isFinite(result.current.remainingMs)).toBe(true);
    expect(result.current.remainingMs).toBe(0);
  });

  it("null + thoi gian troi van khong tu chuyen thanh expired", () => {
    const { result } = renderHook(() => useServerCountdown(null));

    act(() => {
      vi.advanceTimersByTime(600_000);
    });

    expect(result.current.expired).toBe(false);
    expect(result.current.remainingMs).toBe(0);
  });
});

describe("useServerCountdown — offsetMs (AC-4)", () => {
  it("server nhanh hon client 60s -> con it hon 60s so voi khong offset", () => {
    const { result } = renderHook(() => useServerCountdown(START + TEN_MIN, 60_000));

    expect(result.current.remainingMs).toBe(TEN_MIN - 60_000);
  });

  it("server cham hon client 60s -> con nhieu hon 60s", () => {
    const { result } = renderHook(() => useServerCountdown(START + TEN_MIN, -60_000));

    expect(result.current.remainingMs).toBe(TEN_MIN + 60_000);
  });

  it("offset lam moc da qua -> expired true ngay", () => {
    const { result } = renderHook(() => useServerCountdown(START + 30_000, 60_000));

    expect(result.current.remainingMs).toBe(0);
    expect(result.current.expired).toBe(true);
  });

  it("offset van duoc ap dung sau khi tick", () => {
    const { result } = renderHook(() => useServerCountdown(START + TEN_MIN, 60_000));

    act(() => {
      vi.advanceTimersByTime(10_000);
    });

    expect(result.current.remainingMs).toBe(TEN_MIN - 70_000);
  });

  it("offset khong huu hien -> coi nhu 0, KHONG ra NaN (AC-3)", () => {
    const { result } = renderHook(() => useServerCountdown(START + TEN_MIN, Number.NaN));

    expect(Number.isFinite(result.current.remainingMs)).toBe(true);
    expect(result.current.remainingMs).toBe(TEN_MIN);
  });
});

describe("useServerCountdown — don dep timer (AC-4)", () => {
  it("clear interval khi unmount", () => {
    const baseline = vi.getTimerCount();
    const { unmount } = renderHook(() => useServerCountdown(START + TEN_MIN));

    expect(vi.getTimerCount()).toBeGreaterThan(baseline);

    unmount();

    expect(vi.getTimerCount()).toBe(baseline);
  });

  it("dung chay khi da het han, khong de interval song mai", () => {
    const baseline = vi.getTimerCount();
    const { result } = renderHook(() => useServerCountdown(START + 2_000));

    act(() => {
      vi.advanceTimersByTime(10_000);
    });

    expect(result.current.expired).toBe(true);
    expect(vi.getTimerCount()).toBe(baseline);
  });

  it("doi expiresAt sang moc moi thi dem lai theo moc moi", () => {
    const { result, rerender } = renderHook(
      ({ exp }: { exp: number }) => useServerCountdown(exp),
      { initialProps: { exp: START + TEN_MIN } },
    );

    expect(result.current.remainingMs).toBe(TEN_MIN);

    rerender({ exp: START + 120_000 });

    expect(result.current.remainingMs).toBe(120_000);
    expect(result.current.expired).toBe(false);
  });
});

/**
 * `ready` — vi sao can (major M1).
 *
 * Moi route cua app la `○ (Static)`, tuc HTML duoc PRERENDER luc build. Neu hook
 * doc `Date.now()` ngay trong than render thi con so cua THOI DIEM BUILD bi dong
 * bang vao HTML: khach mo trang ba ngay sau se thay "Da het thoi gian giu ve" o
 * first paint roi moi nhay ve so dung. Va vi lan render hydrate phai cho ra output
 * y het server, gia tri khoi tao buoc phai xac dinh.
 *
 * Hop dong: truoc khi mount LUON la { remainingMs: 0, expired: false, ready: false }
 * — ke ca khi moc da qua. `ready === false` nghia la "CHUA BIET", khac "con 0" va
 * khac "da het han".
 */
describe("useServerCountdown — truoc khi mount (prerender tinh): ready false (AC-4, M1)", () => {
  it("moc tuong lai: markup prerender khong chua so nao cua luc build", () => {
    const markup = renderToStaticMarkup(<Probe expiresAt={START + TEN_MIN} />);

    expect(markup).toContain('data-ready="false"');
    expect(markup).toContain('data-expired="false"');
    expect(markup).toContain('data-remaining="0"');
  });

  it("moc DA QUA: prerender van KHONG duoc noi la het han", () => {
    const markup = renderToStaticMarkup(<Probe expiresAt={START - 600_000} />);

    // Day la bug that: HTML tinh bi dong bang trang thai "het han" cua luc build.
    expect(markup).toContain('data-expired="false"');
    expect(markup).toContain('data-ready="false"');
  });

  it("khong co moc: prerender cung la chua biet", () => {
    const markup = renderToStaticMarkup(<Probe expiresAt={null} />);

    expect(markup).toContain('data-ready="false"');
    expect(markup).toContain('data-expired="false"');
    expect(markup).toContain('data-remaining="0"');
  });

  it("offsetMs khong lam thay doi gia tri truoc mount", () => {
    const markup = renderToStaticMarkup(
      <Probe expiresAt={START + TEN_MIN} offsetMs={60_000} />,
    );

    expect(markup).toContain('data-ready="false"');
    expect(markup).toContain('data-remaining="0"');
  });
});

describe("useServerCountdown — ready sau khi mount (AC-4)", () => {
  it("moc tuong lai hop le -> ready true", () => {
    const { result } = renderHook(() => useServerCountdown(START + TEN_MIN));

    expect(result.current.ready).toBe(true);
    expect(result.current.remainingMs).toBe(TEN_MIN);
    expect(result.current.expired).toBe(false);
  });

  it("moc da qua -> ready true VA expired true (biet roi, va biet la het)", () => {
    const { result } = renderHook(() => useServerCountdown(START - 60_000));

    expect(result.current.ready).toBe(true);
    expect(result.current.expired).toBe(true);
    expect(result.current.remainingMs).toBe(0);
  });

  it("ready la boolean trong moi truong hop", () => {
    const { result } = renderHook(() => useServerCountdown(START + TEN_MIN));

    expect(typeof result.current.ready).toBe("boolean");
  });

  it.each([
    [null, "null"],
    [undefined, "undefined"],
    ["", "chuoi rong"],
    ["soon", "chuoi rac"],
    ["2026-10-03", "chuoi khong co mui gio"],
    ["2026-10-03T09:00:00", "chuoi co gio nhung khong co offset"],
    [Number.NaN, "NaN"],
  ])("moc khong doc duoc (%s) -> ready FALSE va expired FALSE", (value, _label) => {
    const { result } = renderHook(() =>
      useServerCountdown(value as number | string | null | undefined),
    );

    // ready false = "chua biet moc", KHONG duoc hieu la het han.
    expect(result.current.ready).toBe(false);
    expect(result.current.expired).toBe(false);
    expect(result.current.remainingMs).toBe(0);
  });

  it("ready khong bao gio true cung luc voi moc khong doc duoc, du thoi gian troi", () => {
    const { result } = renderHook(() => useServerCountdown("khong-phai-ngay"));

    act(() => {
      vi.advanceTimersByTime(600_000);
    });

    expect(result.current.ready).toBe(false);
    expect(result.current.expired).toBe(false);
  });

  it("doi tu moc khong doc duoc sang moc hop le -> ready chuyen thanh true", () => {
    const { result, rerender } = renderHook(
      ({ exp }: { exp: number | string | null }) => useServerCountdown(exp),
      { initialProps: { exp: null as number | string | null } },
    );
    expect(result.current.ready).toBe(false);

    rerender({ exp: START + TEN_MIN });

    expect(result.current.ready).toBe(true);
    expect(result.current.remainingMs).toBe(TEN_MIN);
  });
});

describe("useServerCountdown — doi moc khong duoc loe mot frame 'het han' (minor m6)", () => {
  it("tu moc da qua sang moc tuong lai: expired false NGAY tai lan render do", () => {
    const { result, rerender } = renderHook(
      ({ exp }: { exp: number }) => useServerCountdown(exp),
      { initialProps: { exp: START - 60_000 } },
    );
    expect(result.current.expired).toBe(true);

    // Hold moi vua tao, con nguyen 10 phut. Neu gia tri duoc tinh trong effect
    // thay vi trong than render thi o day con mot frame hien "da het thoi gian
    // giu ve" tren mot hold VAN con 10 phut.
    rerender({ exp: START + TEN_MIN });

    expect(result.current.expired).toBe(false);
    expect(result.current.remainingMs).toBe(TEN_MIN);
    expect(result.current.ready).toBe(true);
  });

  it("tu hold cu (con 10s) sang hold moi (con 10 phut) khong di qua trang thai het han", () => {
    const { result, rerender } = renderHook(
      ({ exp }: { exp: number }) => useServerCountdown(exp),
      { initialProps: { exp: START + 10_000 } },
    );

    rerender({ exp: START + TEN_MIN });

    expect(result.current.expired).toBe(false);
    expect(result.current.remainingMs).toBe(TEN_MIN);
  });
});

describe("useServerCountdown — hop dong: chi nhan moc tuyet doi (BR-O2)", () => {
  it("chu ky ham khong co tham so thu ba kieu durationMs", () => {
    // BR-O2: hook KHONG duoc nhan durationMs. TypeScript la chot chinh (truyen
    // tham so thu 3 se fail `npm run typecheck`); day la chot phu o runtime.
    expect(useServerCountdown.length).toBeLessThanOrEqual(2);
  });
});
