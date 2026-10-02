// @vitest-environment node
/**
 * Test cho `instrumentation` — AC-2b (spec muc 4), contract C1.1 + C1.1b.
 *
 * Moi truong `node` chu KHONG jsdom (C1.3 dung ly do, khac trieu chung): file nay noi ve
 * TIN HIEU TIEN TRINH. jsdom khong mo hinh hoa `process.on` / `process.exit` / vong lap su kien
 * cua Node, nen chay o jsdom thi test se do vi ha tang chu khong vi hanh vi.
 *
 * HOP DONG DUOC KIEM (nguyen van C1.1b — "DU BA VIEC, DUNG THU TU"):
 *
 *   1. `setReady(false)`  -> `/api/readyz` tra 503 NGAY LAP TUC
 *   2. cho `DRAIN_MS = 5000`
 *   3. `process.exit(0)`
 *
 * Diem song con cua AC-2b la THU TU, khong phai tung buoc roi rac. Neu `setReady(false)` chay
 * sau cua so drain thi suot 5 giay do `/api/readyz` van tra 200, endpoint khong bi rut, va
 * kubelet tiep tuc day traffic vao pod sap thoat — dung thu hong ma AC-2b sinh ra de chong.
 * Vi vay test khong chi assert "co goi exit" ma assert readiness DA la `false` tai dung thoi
 * diem `process.exit` duoc goi.
 *
 * CACH LAP TIN HIEU. Test emit `SIGTERM` tren `process` (tin hieu MO PHONG), khong gui tin hieu
 * that cua he dieu hanh. Xem phan "khong chung minh duoc" trong bao cao: may chay la Windows,
 * khong co POSIX signal, nen bang chung cho duong tin hieu that chi co o P4.5 trong cluster.
 *
 * AN TOAN CUA RUNNER — hai thu bat buoc, khong phai phong thu thua:
 *
 *   - `process.exit` luon bi spy. Mot test de `process.exit(0)` chay that se keo sap ca runner
 *     va 1067 test dang xanh bien thanh "khong ket luan duoc".
 *   - Listener `SIGTERM` co san (cua vitest / tinypool) duoc luu bang `rawListeners` roi don
 *     sach truoc moi test va hoan nguyen sau moi test. Nho vay `process.emit("SIGTERM")` chi
 *     chay dung handler dang kiem, khong vo tinh goi co che tat cua runner. Dung `rawListeners`
 *     chu khong `listeners` de giu nguyen tinh `once` cua cac listener goc khi gan lai.
 *
 * MOI TEST NAP LAI MODULE. `instrumentation.ts` giu co chong-goi-hai-lan o tam module, nen neu
 * dung chung mot instance thi test thu hai tro di se chay voi co da bat va khong con kiem duoc gi.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { MockInstance } from "vitest";

import { setReady } from "./lib/readiness";

/** Chot trong C1.1b. Test KHONG import hang so nay tu ban hien thuc: neu ai do doi 5000 thanh
 * 500 thi test phai do, chu khong duoc doi theo. */
const DRAIN_MS = 5_000;

/** Spy cua `process.exit`. Mac dinh khong lam gi — tien trinh phai song qua het test. */
let exitSpy: MockInstance<(code?: string | number | null) => never>;

/** Listener SIGTERM co tu truoc, de gan lai o cleanup. */
let originalSigtermListeners: Array<(...args: unknown[]) => void> = [];

/** Gia tri NEXT_RUNTIME co tu truoc, de gan lai o cleanup. */
let originalNextRuntime: string | undefined;

/** Nap mot instance `instrumentation` HOAN TOAN MOI cung voi dung instance `readiness` ma no
 * dung, de assert readiness doc tu cung mot nguon voi ban hien thuc. */
async function loadFreshInstrumentation() {
  vi.resetModules();

  const readiness = await import("./lib/readiness");
  const instrumentation = await import("./instrumentation");

  return { register: instrumentation.register, isReady: readiness.isReady };
}

/** Gui SIGTERM mo phong. Tach ra mot ham de moi test doc duoc y dinh thay vi doc `process.emit`. */
function sendSigterm(): void {
  process.emit("SIGTERM");
}

beforeEach(() => {
  originalSigtermListeners = process.rawListeners("SIGTERM") as Array<
    (...args: unknown[]) => void
  >;
  process.removeAllListeners("SIGTERM");

  originalNextRuntime = process.env.NEXT_RUNTIME;
  // `register()` chi dang ky o runtime `nodejs` (edge runtime khong co `process.on`/`exit`).
  process.env.NEXT_RUNTIME = "nodejs";

  vi.useFakeTimers();
  exitSpy = vi.spyOn(process, "exit").mockImplementation((() => undefined) as never);
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();

  // Don listener do test tao ra, roi gan lai listener goc. Thieu buoc nay la ro ri tin hieu
  // sang cac file test khac trong cung worker.
  process.removeAllListeners("SIGTERM");
  for (const listener of originalSigtermListeners) {
    process.on("SIGTERM", listener as NodeJS.SignalsListener);
  }
  originalSigtermListeners = [];

  if (originalNextRuntime === undefined) {
    delete process.env.NEXT_RUNTIME;
  } else {
    process.env.NEXT_RUNTIME = originalNextRuntime;
  }

  // Trang thai readiness song qua `vi.resetModules()`, nen phai dua ve mac dinh ready.
  setReady(true);
});

describe("Buoc 1 — SIGTERM ha readiness xuong false NGAY LAP TUC (AC-2b, C1.1b)", () => {
  it("isReady() thanh false ngay o thoi diem nhan tin hieu, chua cho het cua so drain", async () => {
    const { register, isReady } = await loadFreshInstrumentation();
    register();

    expect(isReady()).toBe(true);
    sendSigterm();

    expect(isReady()).toBe(false);
  });

  it("ha readiness xong nhung CHUA thoat tien trinh (drain chua bat dau troi)", async () => {
    const { register, isReady } = await loadFreshInstrumentation();
    register();

    sendSigterm();

    expect(isReady()).toBe(false);
    expect(exitSpy).not.toHaveBeenCalled();
  });

  it("readiness DA la false tai dung thoi diem process.exit duoc goi (dung thu tu 1 -> 3)", async () => {
    const { register, isReady } = await loadFreshInstrumentation();
    const readyWhenExiting: boolean[] = [];
    exitSpy.mockImplementation(((): undefined => {
      readyWhenExiting.push(isReady());
      return undefined;
    }) as never);
    register();

    sendSigterm();
    vi.advanceTimersByTime(DRAIN_MS);

    expect(readyWhenExiting).toEqual([false]);
  });
});

describe("Buoc 2 — tien trinh chi thoat SAU cua so drain 5000ms (AC-2b, C1.1b)", () => {
  it("chua thoat o 4999ms (cua so drain khong bi cat ngan)", async () => {
    const { register } = await loadFreshInstrumentation();
    register();

    sendSigterm();
    vi.advanceTimersByTime(DRAIN_MS - 1);

    expect(exitSpy).not.toHaveBeenCalled();
  });

  it("thoat dung o 5000ms", async () => {
    const { register } = await loadFreshInstrumentation();
    register();

    sendSigterm();
    vi.advanceTimersByTime(DRAIN_MS);

    expect(exitSpy).toHaveBeenCalledTimes(1);
  });

  it("chi thoat MOT lan du dong ho chay tiep rat lau sau cua so drain", async () => {
    const { register } = await loadFreshInstrumentation();
    register();

    sendSigterm();
    vi.advanceTimersByTime(60_000);

    expect(exitSpy).toHaveBeenCalledTimes(1);
  });
});

describe("Buoc 3 — ma thoat la 0 (AC-2b, C1.1b)", () => {
  it("goi process.exit(0)", async () => {
    const { register } = await loadFreshInstrumentation();
    register();

    sendSigterm();
    vi.advanceTimersByTime(DRAIN_MS);

    expect(exitSpy).toHaveBeenCalledWith(0);
  });

  it("ma thoat la so 0, khong phai undefined va khong phai ma loi", async () => {
    const { register } = await loadFreshInstrumentation();
    register();

    sendSigterm();
    vi.advanceTimersByTime(DRAIN_MS);

    const code = exitSpy.mock.calls[0]?.[0];
    expect(code).toBe(0);
    expect(typeof code).toBe("number");
  });
});

describe("Tin hieu thu hai trong cua so drain khong lam lai va khong cat ngan (C1.1b)", () => {
  it("tin hieu thu hai o giua cua so khong lam tien trinh thoat som", async () => {
    const { register } = await loadFreshInstrumentation();
    register();

    sendSigterm();
    vi.advanceTimersByTime(2_000);
    sendSigterm();
    // Tong cong moi 4999ms ke tu tin hieu DAU TIEN: van phai chua thoat.
    vi.advanceTimersByTime(DRAIN_MS - 2_000 - 1);

    expect(exitSpy).not.toHaveBeenCalled();
  });

  it("tin hieu thu hai khong keo dai cua so: van thoat o dung 5000ms ke tu tin hieu dau", async () => {
    const { register } = await loadFreshInstrumentation();
    register();

    sendSigterm();
    vi.advanceTimersByTime(2_000);
    sendSigterm();
    vi.advanceTimersByTime(DRAIN_MS - 2_000);

    expect(exitSpy).toHaveBeenCalledTimes(1);
    expect(exitSpy).toHaveBeenCalledWith(0);
  });

  it("ba tin hieu lien tiep chi tao DUNG MOT lan thoat", async () => {
    const { register } = await loadFreshInstrumentation();
    register();

    sendSigterm();
    sendSigterm();
    sendSigterm();
    vi.advanceTimersByTime(60_000);

    expect(exitSpy).toHaveBeenCalledTimes(1);
  });

  it("tin hieu den SAU khi da thoat khong hen them lan thoat nao", async () => {
    const { register, isReady } = await loadFreshInstrumentation();
    register();

    sendSigterm();
    vi.advanceTimersByTime(DRAIN_MS);
    sendSigterm();
    vi.advanceTimersByTime(60_000);

    expect(exitSpy).toHaveBeenCalledTimes(1);
    // Va khong co duong nao dua readiness tro lai true (bat bien mot chieu cua C1.1).
    expect(isReady()).toBe(false);
  });
});

describe("Dang ky handler — mot lan, khong tich luy (C1.1b)", () => {
  it("register() dang ky DUNG MOT listener SIGTERM", async () => {
    const { register } = await loadFreshInstrumentation();

    expect(process.listenerCount("SIGTERM")).toBe(0);
    register();

    expect(process.listenerCount("SIGTERM")).toBe(1);
  });

  it("nap module nhieu lan tra ve cung mot instance, khong tich luy listener", async () => {
    vi.resetModules();

    const first = await import("./instrumentation");
    const second = await import("./instrumentation");

    expect(second).toBe(first);
    first.register();

    expect(process.listenerCount("SIGTERM")).toBe(1);
  });

  it("du register() bi goi nhieu lan, chuoi shutdown van chi chay mot lan va khong ngan hon", async () => {
    const { register, isReady } = await loadFreshInstrumentation();
    register();
    register();

    sendSigterm();
    expect(isReady()).toBe(false);
    vi.advanceTimersByTime(DRAIN_MS - 1);
    expect(exitSpy).not.toHaveBeenCalled();

    vi.advanceTimersByTime(1);
    expect(exitSpy).toHaveBeenCalledTimes(1);
  });

  it("khong dang ky gi o runtime edge (edge khong co process.on / process.exit)", async () => {
    process.env.NEXT_RUNTIME = "edge";

    const { register, isReady } = await loadFreshInstrumentation();
    register();

    expect(process.listenerCount("SIGTERM")).toBe(0);
    expect(isReady()).toBe(true);
    expect(exitSpy).not.toHaveBeenCalled();
  });

  it("khong dang ky gi khi NEXT_RUNTIME khong duoc dat", async () => {
    delete process.env.NEXT_RUNTIME;

    const { register } = await loadFreshInstrumentation();
    register();

    expect(process.listenerCount("SIGTERM")).toBe(0);
    expect(exitSpy).not.toHaveBeenCalled();
  });
});

describe("Khong co tin hieu thi khong xay ra gi (khong side effect luc import)", () => {
  it("chi nap module (chua goi register) khong ha readiness, khong them listener, khong hen exit", async () => {
    const { isReady } = await loadFreshInstrumentation();

    expect(process.listenerCount("SIGTERM")).toBe(0);
    expect(isReady()).toBe(true);

    vi.advanceTimersByTime(60_000);

    expect(exitSpy).not.toHaveBeenCalled();
    expect(isReady()).toBe(true);
  });

  it("da register nhung chua co tin hieu: van ready va khong thoat du dong ho chay 60s", async () => {
    const { register, isReady } = await loadFreshInstrumentation();
    register();

    vi.advanceTimersByTime(60_000);

    expect(isReady()).toBe(true);
    expect(exitSpy).not.toHaveBeenCalled();
  });
});
