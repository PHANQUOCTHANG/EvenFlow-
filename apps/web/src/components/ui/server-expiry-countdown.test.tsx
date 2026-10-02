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
