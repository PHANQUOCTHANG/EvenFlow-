/**
 * Test cho `ServerExpiryCountdown` — AC-5 (spec-plan EVF-1803 §4), BR-Q7, BR-O2.
 *
 * Ba thu de sai nhat o component nay:
 *
 *   1. GOP HAI DONG HO CHECKOUT. BR-Q7: suat admit TTL 15 phut, giu ghe 10 phut.
 *      Neu hai dong ho hien cung mot nhan ("Con lai 09:30") thi khach khong biet
 *      con so do thuoc cai nao, tuong con 15 phut trong khi ghe chi giu 10 ->
 *      mat ve. Vi vay `variant` bat buoc va MOI variant phai cho ra nhan KHAC
 *      nhau. Test o day KHONG hard-code cau chu cua nhan (spec khong chot cau
 *      chu); no chot "ba nhan khac nhau doi mot va deu khong rong".
 *
 *   2. TU BIA NGUONG CANH BAO. `warningThresholdMs` khong co default (spec §7:
 *      nguong canh bao la policy nghiep vu, khong phai quyet dinh cua component).
 *      Khong truyen -> khong duoc co trang thai canh bao o bat ky moc nao.
 *
 *   3. ARIA-LIVE SPAM. Vung live doi noi dung moi giay se lam screen reader doc
 *      lien tuc, khong dung duoc. Chi duoc thong bao khi VAO canh bao va khi HET
 *      HAN. Test: chay nhieu giay roi assert noi dung vung live KHONG doi, dong
 *      thoi assert phan chu so VAN doi (de chac chan component khong bi dong bang).
 *
 * Trang thai canh bao duoc kiem qua tin hieu CO THE QUAN SAT DUOC (noi dung vung
 * aria-live) chu khong qua class CSS — mau sac se duoc review bang mat va bang
 * tokens.css.
 */
import { act, render } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ServerExpiryCountdown, type CountdownVariant } from "./server-expiry-countdown";

const START = Date.UTC(2026, 0, 15, 3, 0, 0);
const VARIANTS: CountdownVariant[] = ["sale-start", "admission", "hold"];

/** Chuan hoa khoang trang de so sanh text. */
function norm(value: string | null | undefined): string {
  return (value ?? "").replace(/\s+/g, " ").trim();
}

/** Bo moi chu so va dau hai cham -> con lai phan CHU (nhan + text thay the). */
function words(value: string | null | undefined): string {
  return norm(value)
    .replace(/[\d:]+/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

/**
 * Toan bo chu ma tro ho tro co the doc duoc: text node + moi `aria-label`.
 * Text thay the cho vung chu so co the lam bang sr-only text HAY aria-label,
 * ca hai deu hop le nen test khong ep mot cach cai dat.
 */
function allText(container: HTMLElement): string {
  const labels = Array.from(container.querySelectorAll("[aria-label]"))
    .map((el) => el.getAttribute("aria-label") ?? "")
    .join(" ");
  return norm(`${container.textContent ?? ""} ${labels}`);
}

/** Vung aria-live cua component (noi duy nhat quan sat duoc trang thai canh bao). */
function liveRegion(container: HTMLElement): Element | null {
  return container.querySelector("[aria-live]");
}

function liveText(container: HTMLElement): string {
  return norm(liveRegion(container)?.textContent);
}

/**
 * `data-state` = "running" | "warning" | "expired": tin hieu trang thai khong
 * phu thuoc cau chu va khong phai class CSS, dung de chot ngưỡng canh bao.
 */
function dataState(container: HTMLElement): string | undefined {
  return (
    container.querySelector("[data-state]")?.getAttribute("data-state") ?? undefined
  );
}

/**
 * Chu ma tro ho tro THUC SU doc: bo qua moi nhanh `aria-hidden="true"`.
 * Dung de chot "nhan chi duoc doc DUNG MOT LAN".
 */
function atText(root: Element): string {
  if (root.getAttribute?.("aria-hidden") === "true") return "";

  let out = "";
  for (const node of Array.from(root.childNodes)) {
    if (node.nodeType === 3) {
      out += node.textContent ?? "";
    } else if (node.nodeType === 1) {
      out += ` ${atText(node as Element)}`;
    }
  }
  return norm(out);
}

/**
 * Co cum tu >= `minWords` nao bi lap lai trong chuoi khong?
 * Bug that da xay ra: nhan vua hien thi vua nam trong cau sr-only nen screen
 * reader doc "Suat mua cua ban con. Suat mua cua ban con 9 phut 30 giay".
 * Kiem theo cach nay khong phu thuoc cau chu cu the cua nhan.
 */
function hasRepeatedPhrase(text: string, minWords = 3): boolean {
  const parts = text.split(" ").filter(Boolean);

  for (let len = Math.floor(parts.length / 2); len >= minWords; len -= 1) {
    for (let i = 0; i + len <= parts.length; i += 1) {
      const phrase = parts.slice(i, i + len).join(" ");
      if (parts.slice(i + len).join(" ").includes(phrase)) return true;
    }
  }
  return false;
}

/** Node trong cung chua dung chuoi dong ho dang mm:ss hoac h:mm:ss. */
function clockNode(container: HTMLElement): Element | undefined {
  const matches = Array.from(container.querySelectorAll("*")).filter((el) =>
    /^\d{1,2}:\d{2}(:\d{2})?$/.test(norm(el.textContent)),
  );
  return matches[matches.length - 1];
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(START);
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("ServerExpiryCountdown — hien thi so con lai (AC-5)", () => {
  it("expiresAt = now + 9 phut 30 giay -> hien 09:30", () => {
    const { container } = render(
      <ServerExpiryCountdown variant="hold" expiresAt={START + 570_000} />,
    );

    expect(norm(container.textContent)).toContain("09:30");
  });

  it("nhan expiresAt dang ISO string", () => {
    const iso = new Date(START + 570_000).toISOString();
    const { container } = render(<ServerExpiryCountdown variant="hold" expiresAt={iso} />);

    expect(norm(container.textContent)).toContain("09:30");
  });

  it("dem nguoc theo thoi gian troi", () => {
    const { container } = render(
      <ServerExpiryCountdown variant="hold" expiresAt={START + 570_000} />,
    );

    act(() => {
      vi.advanceTimersByTime(10_000);
    });

    expect(norm(container.textContent)).toContain("09:20");
  });

  it("ap dung offsetMs khi so sanh voi gio server", () => {
    const { container } = render(
      <ServerExpiryCountdown variant="hold" expiresAt={START + 180_000} offsetMs={60_000} />,
    );

    expect(norm(container.textContent)).toContain("02:00");
  });

  it("tab bi background: nhay dong ho nhieu, chi 1 nhip -> so hien theo thoi gian THUC (BR-O2)", () => {
    const { container } = render(
      <ServerExpiryCountdown variant="hold" expiresAt={START + 600_000} />,
    );
    expect(norm(container.textContent)).toContain("10:00");

    act(() => {
      vi.setSystemTime(START + 300_000);
      vi.advanceTimersByTime(1_000);
    });

    // Troi 301s -> con 04:59. Cai dat giam dan se van hien ~09:59.
    expect(norm(container.textContent)).toContain("04:59");
    expect(norm(container.textContent)).not.toContain("09:59");
  });
});

describe("ServerExpiryCountdown — moi variant mot nhan rieng, khong the lan (AC-5, BR-Q7)", () => {
  it("ba variant cho ra ba nhan khac nhau doi mot", () => {
    const labels = VARIANTS.map((variant) => {
      const { container, unmount } = render(
        <ServerExpiryCountdown variant={variant} expiresAt={START + 570_000} />,
      );
      const label = words(container.textContent);
      unmount();
      return label;
    });

    expect(new Set(labels).size).toBe(VARIANTS.length);
    expect(labels[0]).not.toBe(labels[1]);
    expect(labels[1]).not.toBe(labels[2]);
    expect(labels[0]).not.toBe(labels[2]);
  });

  it("moi variant co phan chu khong rong (khong chi co chu so tron)", () => {
    for (const variant of VARIANTS) {
      const { container, unmount } = render(
        <ServerExpiryCountdown variant={variant} expiresAt={START + 570_000} />,
      );

      expect(words(container.textContent).length).toBeGreaterThan(3);

      unmount();
    }
  });

  it("hai dong ho checkout (admission vs hold) khac nhan nhau", () => {
    const admission = render(
      <ServerExpiryCountdown variant="admission" expiresAt={START + 570_000} />,
    );
    const admissionLabel = words(admission.container.textContent);
    admission.unmount();

    const hold = render(<ServerExpiryCountdown variant="hold" expiresAt={START + 570_000} />);
    const holdLabel = words(hold.container.textContent);
    hold.unmount();

    expect(admissionLabel).not.toBe(holdLabel);
    expect(admissionLabel.length).toBeGreaterThan(3);
    expect(holdLabel.length).toBeGreaterThan(3);
  });
});

describe("ServerExpiryCountdown — tiep can voi screen reader (AC-5)", () => {
  it("vung chu so bi aria-hidden (khong doc tung ky tu 09:30)", () => {
    const { container } = render(
      <ServerExpiryCountdown variant="hold" expiresAt={START + 570_000} />,
    );

    const node = clockNode(container);
    expect(node).toBeDefined();
    expect(node?.closest('[aria-hidden="true"]')).not.toBeNull();
  });

  it("co text thay the dang chu, co nhac so PHUT", () => {
    const { container } = render(
      <ServerExpiryCountdown variant="hold" expiresAt={START + 570_000} />,
    );

    expect(allText(container)).toMatch(/9\s*phút/i);
  });

  it("text thay the doi theo so phut con lai", () => {
    const { container } = render(
      <ServerExpiryCountdown variant="hold" expiresAt={START + 570_000} />,
    );
    expect(allText(container)).toMatch(/9\s*phút/i);

    act(() => {
      vi.advanceTimersByTime(300_000);
    });

    expect(allText(container)).toMatch(/4\s*phút/i);
  });

  it("co vung aria-live (polite hoac assertive)", () => {
    const { container } = render(
      <ServerExpiryCountdown variant="hold" expiresAt={START + 570_000} />,
    );

    const live = liveRegion(container);
    expect(live).not.toBeNull();
    expect(["polite", "assertive"]).toContain(live?.getAttribute("aria-live"));
  });

  it("vung aria-live KHONG doi moi giay (khong spam screen reader)", () => {
    const { container } = render(
      <ServerExpiryCountdown variant="hold" expiresAt={START + 570_000} />,
    );

    const before = liveText(container);
    const textBefore = norm(container.textContent);

    act(() => {
      vi.advanceTimersByTime(5_000);
    });

    // Chu so phai doi (component van chay) nhung vung live thi khong.
    expect(norm(container.textContent)).not.toBe(textBefore);
    expect(liveText(container)).toBe(before);
  });
});

describe("ServerExpiryCountdown — nguong canh bao do caller quyet dinh (AC-5)", () => {
  it("vao duoi nguong -> vung aria-live thong bao (noi dung doi va khong rong)", () => {
    const { container } = render(
      <ServerExpiryCountdown
        variant="hold"
        expiresAt={START + 180_000}
        warningThresholdMs={120_000}
      />,
    );

    const before = liveText(container);

    act(() => {
      vi.advanceTimersByTime(80_000); // con 100s < 120s
    });

    const after = liveText(container);
    expect(after).not.toBe(before);
    expect(after.length).toBeGreaterThan(0);
  });

  it("con tren nguong thi chua thong bao gi moi", () => {
    const { container } = render(
      <ServerExpiryCountdown
        variant="hold"
        expiresAt={START + 180_000}
        warningThresholdMs={120_000}
      />,
    );

    const before = liveText(container);

    act(() => {
      vi.advanceTimersByTime(30_000); // con 150s > 120s
    });

    expect(liveText(container)).toBe(before);
  });

  it("KHONG truyen warningThresholdMs -> khong co canh bao o bat ky moc nao (khong tu bia default)", () => {
    const { container } = render(
      <ServerExpiryCountdown variant="hold" expiresAt={START + 180_000} />,
    );

    const before = liveText(container);

    act(() => {
      vi.advanceTimersByTime(60_000); // con 120s
    });
    expect(liveText(container)).toBe(before);

    act(() => {
      vi.advanceTimersByTime(60_000); // con 60s
    });
    expect(liveText(container)).toBe(before);

    act(() => {
      vi.advanceTimersByTime(59_000); // con 1s, van chua het han
    });
    expect(liveText(container)).toBe(before);
  });
});

describe("ServerExpiryCountdown — data-state: chot nguong khong qua cau chu (AC-5)", () => {
  it("con nhieu thoi gian -> running", () => {
    const { container } = render(
      <ServerExpiryCountdown
        variant="hold"
        expiresAt={START + 570_000}
        warningThresholdMs={120_000}
      />,
    );

    expect(dataState(container)).toBe("running");
  });

  it("remainingMs <= warningThresholdMs -> warning", () => {
    const { container } = render(
      <ServerExpiryCountdown
        variant="hold"
        expiresAt={START + 120_000}
        warningThresholdMs={120_000}
      />,
    );

    expect(dataState(container)).toBe("warning");
  });

  it("chay tu running sang warning dung tai moc nguong", () => {
    const { container } = render(
      <ServerExpiryCountdown
        variant="hold"
        expiresAt={START + 180_000}
        warningThresholdMs={120_000}
      />,
    );
    expect(dataState(container)).toBe("running");

    act(() => {
      vi.advanceTimersByTime(59_000); // con 121s
    });
    expect(dataState(container)).toBe("running");

    act(() => {
      vi.advanceTimersByTime(1_000); // con 120s = nguong
    });
    expect(dataState(container)).toBe("warning");
  });

  it("het han -> expired (khong con la warning)", () => {
    const { container } = render(
      <ServerExpiryCountdown
        variant="hold"
        expiresAt={START - 1_000}
        warningThresholdMs={120_000}
      />,
    );

    expect(dataState(container)).toBe("expired");
  });

  it("KHONG truyen nguong -> khong bao gio warning, chi running roi expired", () => {
    const { container } = render(
      <ServerExpiryCountdown variant="hold" expiresAt={START + 10_000} />,
    );
    expect(dataState(container)).toBe("running");

    act(() => {
      vi.advanceTimersByTime(9_000); // con 1s
    });
    expect(dataState(container)).toBe("running");

    act(() => {
      vi.advanceTimersByTime(2_000);
    });
    expect(dataState(container)).toBe("expired");
  });
});

/**
 * Trang thai `unknown` — vi sao can (major M1).
 *
 * Moi route la `○ (Static)` nen HTML duoc prerender luc build. Component khong
 * duoc hien con so nao truoc khi biet gio thuc cua may khach: so cua luc build
 * se bi dong bang vao HTML. Va `00:00` la lua chon TOI nhat cho luc "chua biet"
 * vi no trong y nhu da het han — dung lam khach tuong minh mat ve. Vi vay
 * `--:--` + `data-state="unknown"`.
 */
describe("ServerExpiryCountdown — truoc khi biet moc: unknown + '--:--' (AC-5, M1)", () => {
  it("markup prerender la unknown, hien '--:--', KHONG hien so cua luc build", () => {
    const markup = renderToStaticMarkup(
      <ServerExpiryCountdown variant="hold" expiresAt={START + 570_000} />,
    );

    expect(markup).toContain('data-state="unknown"');
    expect(markup).toContain("--:--");
    expect(markup).not.toContain("09:30");
  });

  it("markup prerender cua moc DA QUA cung khong noi la het han", () => {
    const markup = renderToStaticMarkup(
      <ServerExpiryCountdown variant="hold" expiresAt={START - 600_000} />,
    );

    expect(markup).toContain('data-state="unknown"');
    expect(markup).not.toContain('data-state="expired"');
    expect(markup).toContain("--:--");
  });

  it("moc khong doc duoc (string rac) -> unknown va '--:--', khong phai 00:00", () => {
    const { container } = render(<ServerExpiryCountdown variant="hold" expiresAt="soon" />);

    expect(dataState(container)).toBe("unknown");
    expect(norm(container.textContent)).toContain("--:--");
    expect(norm(container.textContent)).not.toContain("00:00");
  });

  it("chuoi khong co mui gio bi coi la chua biet moc (khong doan lech 7 tieng)", () => {
    const { container } = render(
      <ServerExpiryCountdown variant="hold" expiresAt="2026-10-03T09:00:00" />,
    );

    expect(dataState(container)).toBe("unknown");
  });

  it("unknown KHONG BAO GIO thanh warning, du nguong lon the nao", () => {
    const { container } = render(
      <ServerExpiryCountdown
        variant="hold"
        expiresAt="soon"
        warningThresholdMs={999_999_999}
      />,
    );

    expect(dataState(container)).toBe("unknown");

    act(() => {
      vi.advanceTimersByTime(60_000);
    });

    expect(dataState(container)).toBe("unknown");
  });

  it("unknown khong thong bao gi cho screen reader (vung live rong)", () => {
    const { container } = render(<ServerExpiryCountdown variant="hold" expiresAt="soon" />);

    expect(liveText(container)).toHaveLength(0);
  });

  it("unknown khong hien so am va khong hien 'NaN'", () => {
    const { container } = render(<ServerExpiryCountdown variant="hold" expiresAt="soon" />);
    const text = norm(container.textContent);

    expect(text).not.toMatch(/-\s*\d/);
    expect(text).not.toContain("NaN");
  });
});

describe("ServerExpiryCountdown — thong bao khi het han phai gianh duoc mic (AC-5)", () => {
  // Mat hold = khach MAT VE. `polite` bi xep sau moi thu AT dang doc (vi du khach
  // dang go so the), nen su kien nay phai la `role="alert"` + `assertive`.
  // Luc chi moi CANH BAO thi van `polite` de khong cat loi khach.

  it("het han -> vung thong bao la role=alert + aria-live=assertive va co noi dung", () => {
    const { container } = render(
      <ServerExpiryCountdown variant="hold" expiresAt={START - 1_000} />,
    );

    const alert = container.querySelector('[role="alert"]');
    expect(alert).not.toBeNull();
    expect(alert?.getAttribute("aria-live")).toBe("assertive");
    expect(norm(alert?.textContent).length).toBeGreaterThan(0);
  });

  it("canh bao -> van la role=status + aria-live=polite, KHONG phai alert", () => {
    const { container } = render(
      <ServerExpiryCountdown
        variant="hold"
        expiresAt={START + 60_000}
        warningThresholdMs={120_000}
      />,
    );

    expect(dataState(container)).toBe("warning");
    expect(container.querySelector('[role="alert"]')).toBeNull();

    const status = container.querySelector('[role="status"]');
    expect(status).not.toBeNull();
    expect(status?.getAttribute("aria-live")).toBe("polite");
  });

  it("chay tu canh bao sang het han thi doi tu status sang alert", () => {
    const { container } = render(
      <ServerExpiryCountdown
        variant="hold"
        expiresAt={START + 10_000}
        warningThresholdMs={120_000}
      />,
    );
    expect(container.querySelector('[role="alert"]')).toBeNull();

    act(() => {
      vi.advanceTimersByTime(11_000);
    });

    expect(dataState(container)).toBe("expired");
    expect(container.querySelector('[role="alert"]')).not.toBeNull();
  });
});

describe("ServerExpiryCountdown — screen reader chi doc nhan MOT lan (AC-5)", () => {
  it.each(VARIANTS)("variant %s: khong co cum tu nao bi doc lap", (variant) => {
    const { container } = render(
      <ServerExpiryCountdown variant={variant} expiresAt={START + 570_000} />,
    );

    const spoken = words(atText(container));

    expect(spoken.length).toBeGreaterThan(0);
    expect(hasRepeatedPhrase(spoken)).toBe(false);
  });

  it("vung chu so khong di vao noi dung AT doc (da aria-hidden)", () => {
    const { container } = render(
      <ServerExpiryCountdown variant="hold" expiresAt={START + 570_000} />,
    );

    expect(atText(container)).not.toContain("09:30");
    expect(atText(container)).toMatch(/9\s*phút/i);
  });

  it("cau sr-only nhac don vi phut dung mot lan", () => {
    const { container } = render(
      <ServerExpiryCountdown variant="hold" expiresAt={START + 570_000} />,
    );

    const spoken = atText(container);
    expect(spoken.split("phút").length - 1).toBe(1);
  });
});

describe("ServerExpiryCountdown — icon ngu nghia kem van ban (DESIGN.md)", () => {
  // DESIGN.md: "bat buoc ket hop van ban ro rang VA icon ngu nghia".
  // Icon la trang tri cho AT nen phai aria-hidden; chu moi la nguon thong tin.

  it("trang thai dang chay co icon va icon khong bi AT doc", () => {
    const { container } = render(
      <ServerExpiryCountdown variant="hold" expiresAt={START + 570_000} />,
    );

    expect(norm(container.textContent)).toMatch(/[⏱⏳⌛🕒]/u);
    expect(atText(container)).not.toMatch(/[⏱⏳⌛🕒]/u);
  });

  it("trang thai canh bao co icon canh bao rieng va cung bi an khoi AT", () => {
    const { container } = render(
      <ServerExpiryCountdown
        variant="hold"
        expiresAt={START + 60_000}
        warningThresholdMs={120_000}
      />,
    );

    expect(norm(container.textContent)).toMatch(/[⚠❗⏰]/u);
    expect(atText(container)).not.toMatch(/[⚠❗⏰]/u);
  });

  it("thong tin khong bao gio chi nam o icon: bo icon di van con chu", () => {
    const { container } = render(
      <ServerExpiryCountdown variant="hold" expiresAt={START + 570_000} />,
    );

    const withoutIcons = norm(container.textContent).replace(/[\p{Emoji_Presentation}⏱⏳⌛🕒⚠❗⏰]/gu, "");
    expect(words(withoutIcons).length).toBeGreaterThan(3);
  });
});

describe("ServerExpiryCountdown — da het han (AC-5)", () => {
  it("expiresAt trong qua khu -> co text het han, KHONG dem so am", () => {
    const expired = render(<ServerExpiryCountdown variant="hold" expiresAt={START - 60_000} />);
    const expiredText = norm(expired.container.textContent);

    expect(expiredText).not.toMatch(/-\s*\d/);
    expect(expiredText).not.toContain("-00:");
    expect(expiredText.length).toBeGreaterThan(0);
    expired.unmount();

    const running = render(<ServerExpiryCountdown variant="hold" expiresAt={START + 570_000} />);
    expect(norm(running.container.textContent)).not.toBe(expiredText);
  });

  it("het han trong luc dang mount -> vung aria-live thong bao", () => {
    const { container } = render(
      <ServerExpiryCountdown variant="hold" expiresAt={START + 5_000} />,
    );

    const before = liveText(container);

    act(() => {
      vi.advanceTimersByTime(6_000);
    });

    const after = liveText(container);
    expect(after).not.toBe(before);
    expect(after.length).toBeGreaterThan(0);
  });

  it("sau khi het han khong tiep tuc dem xuong so am", () => {
    const { container } = render(
      <ServerExpiryCountdown variant="hold" expiresAt={START + 5_000} />,
    );

    act(() => {
      vi.advanceTimersByTime(120_000);
    });

    expect(norm(container.textContent)).not.toMatch(/-\s*\d/);
  });

  it("het han o moi variant deu khong hien so am", () => {
    for (const variant of VARIANTS) {
      const { container, unmount } = render(
        <ServerExpiryCountdown variant={variant} expiresAt={START - 1_000} />,
      );

      expect(norm(container.textContent)).not.toMatch(/-\s*\d/);

      unmount();
    }
  });
});
