/**
 * Test cho OpsWarRoom — màn hình war room của Ops (EV-187, spec §3 AC-1..AC-7, AC-9, AC-11;
 * plan §1.3 + §6).
 *
 * Dữ liệu tiêm qua props `initialSnapshot` / `initialError`; polling đi qua getOpsWarRoom
 * thật của hook nên ở đây mock riêng hàm đó (giữ nguyên các hàm thuần + OpsServiceError).
 * Lỗi OpsServiceError lấy từ service thật (__setOpsSampleOverride) vì hợp đồng không chốt
 * chữ ký constructor.
 *
 * Bộ số liệu cố ý khác nhau từng ô để biết ô nào đang hiện:
 *   waitingroom: lobby 911, queued 4.210, admitted 733, rate 37, queueDepth 3.890
 *   ticketing:   holding 644, paid 555, issued 466; bảng tồn kho (Hạng A, Hạng B)
 *   ai:          lag 17 giây, ngân sách 63%
 */
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { renderToString } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  getOpsWarRoom,
  type DegradationMode,
  type OpsWarRoomSnapshot,
  type SourceHealth,
} from "@/lib/ops-service";

import { OpsWarRoom } from "./ops-war-room";

vi.mock("@/lib/ops-service", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/ops-service")>();
  return { ...actual, getOpsWarRoom: vi.fn() };
});

const mockGet = vi.mocked(getOpsWarRoom);

const EVENT_ID = "evt-tech-summit-2026";
const TITLE = "Vietnam Tech Summit 2026 — Kỷ Nguyên AI & Cloud Scale";
const BASE_MS = Date.parse("2026-10-13T03:00:00.000Z"); // 10:00:00 giờ Việt Nam

const AI_COPY: Record<DegradationMode, string> = {
  NORMAL: "Trợ lý trả lời đầy đủ.",
  SAVING: "Trợ lý trả lời ngắn gọn hơn để tiết kiệm tài nguyên.",
  FAQ_ONLY: "Trợ lý chỉ trả lời các câu hỏi thường gặp.",
  OFF: "Trợ lý tạm tắt, người mua được hướng dẫn liên hệ CSKH.",
};
const MODES: DegradationMode[] = ["NORMAL", "SAVING", "FAQ_ONLY", "OFF"];

function makeSnapshot(overrides: Partial<OpsWarRoomSnapshot> = {}): OpsWarRoomSnapshot {
  return {
    eventId: EVENT_ID,
    eventTitle: TITLE,
    saleState: "ON_SALE",
    generatedAt: new Date(Date.now()).toISOString(),
    funnel: { lobby: 911, queued: 4210, admitted: 733, holding: 644, paid: 555, issued: 466 },
    admission: { ratePerSec: 37, mode: "AUTO", queueDepth: 3890 },
    inventory: [
      { tierId: "a", name: "Hạng A", quota: 1000, held: 21, paid: 102, issued: 93, available: 784 },
      { tierId: "b", name: "Hạng B", quota: 150, held: 13, paid: 47, issued: 38, available: 52 },
    ],
    ai: { mode: "NORMAL", manualOverride: false, chatQueueLagSeconds: 17, budgetUsedRatio: 0.63 },
    sources: { waitingroom: "ok", ticketing: "ok", ai: "ok" },
    isSample: true,
    ...overrides,
  };
}

function withSources(partial: Partial<Record<keyof OpsWarRoomSnapshot["sources"], SourceHealth>>) {
  return makeSnapshot({ sources: { waitingroom: "ok", ticketing: "ok", ai: "ok", ...partial } });
}

/** Lỗi OpsServiceError thật với status mong muốn. */
async function realOpsError(status: number): Promise<Error> {
  const actual = await vi.importActual<typeof import("@/lib/ops-service")>("@/lib/ops-service");
  if (status === 404) {
    try {
      await actual.getOpsWarRoom("evt-khong-ton-tai-ev187");
    } catch (err) {
      return err as Error;
    }
  }
  actual.__setOpsSampleOverride(EVENT_ID, { error: status });
  try {
    await actual.getOpsWarRoom(EVENT_ID);
  } catch (err) {
    return err as Error;
  } finally {
    actual.__setOpsSampleOverride(EVENT_ID, null);
  }
  throw new Error(`Ky vong loi ${status}`);
}

function escapeRe(s: string) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/** Có ô nào hiện đúng giá trị `text` (vd "4.210", "63%") trong text node trực tiếp của nó. */
function hasValue(text: string): boolean {
  const re = new RegExp(`(^|[^\\d.,])${escapeRe(text)}([.,]0+)?(?![\\d]|[.,]\\d)`);
  return Array.from(document.body.querySelectorAll("*")).some((el) => {
    const own = Array.from(el.childNodes)
      .filter((n) => n.nodeType === Node.TEXT_NODE)
      .map((n) => n.textContent ?? "")
      .join("");
    return re.test(own);
  });
}

/** Có phần tử nào mà text node trực tiếp chứa nguyên văn `text`. */
function hasOwnText(text: string): boolean {
  return Array.from(document.body.querySelectorAll("*")).some((el) =>
    Array.from(el.childNodes)
      .filter((n) => n.nodeType === Node.TEXT_NODE)
      .map((n) => n.textContent ?? "")
      .join("")
      .includes(text),
  );
}

/** Text node trực tiếp của từng phần tử. KHÔNG dùng textContent của phần tử cha cho số liệu:
 *  textContent nối các ô liền nhau không có dấu cách (`<td>17</td><td>63%</td>` → "1763%"). */
function ownTexts(): string[] {
  return Array.from(document.body.querySelectorAll("*")).map((el) =>
    Array.from(el.childNodes)
      .filter((n) => n.nodeType === Node.TEXT_NODE)
      .map((n) => n.textContent ?? "")
      .join(""),
  );
}

function anyOwnText(re: RegExp): boolean {
  return ownTexts().some((t) => re.test(t));
}

/** Có phần tử nào mà textContent (đã trim) khớp `re` — dùng khi số và đơn vị có thể ở hai con. */
function anyElementText(re: RegExp): boolean {
  return Array.from(document.body.querySelectorAll("*")).some((el) =>
    re.test((el.textContent ?? "").trim()),
  );
}

function hasPercent(n: number): boolean {
  return anyOwnText(new RegExp(`(^|[^\\d])${n}\\s?%`));
}

function liveRegions(role: "alert" | "status"): HTMLElement[] {
  return screen.queryAllByRole(role);
}

function hasBanner(role: "alert" | "status", re: RegExp): boolean {
  return liveRegions(role).some((el) => re.test(el.textContent ?? ""));
}

const STALE_WORD = /(^|[\s(·:–-])cũ([\s).,:·]|$)/i;
const HEALTH_CLAIM = /hệ thống[^.]{0,30}bình thường|tất cả[^.]{0,30}bình thường/i;

function renderWarRoom(props: {
  initialSnapshot?: OpsWarRoomSnapshot | null;
  initialError?: { status: number; message: string } | null;
}) {
  return render(
    <OpsWarRoom
      eventId={EVENT_ID}
      initialSnapshot={props.initialSnapshot ?? null}
      initialError={props.initialError ?? undefined}
    />,
  );
}

async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

async function flush() {
  await advance(0);
}

function inventoryTable(): HTMLElement {
  const table = screen.getAllByRole("table").find((t) => (t.textContent ?? "").includes("Hạng A"));
  if (!table) throw new Error("Khong tim thay bang ton kho");
  return table;
}

function tierRow(name: string): HTMLElement {
  const row = within(inventoryTable())
    .getAllByRole("row")
    .find((r) => (r.textContent ?? "").includes(name));
  if (!row) throw new Error(`Khong tim thay hang ${name}`);
  return row;
}

function controlButtons(): HTMLElement[] {
  return screen.getAllByRole("button", { name: /ghi đè|tạm dừng/i });
}

function refreshButton(): HTMLElement {
  return screen.getAllByRole("button", { name: /làm mới/i })[0];
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(BASE_MS);
  mockGet.mockReset();
  mockGet.mockImplementation(async () => makeSnapshot());
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("OpsWarRoom — AC-1 một màn hình đủ số liệu", () => {
  it("h1 là tên sự kiện, có trạng thái bán", () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    expect(screen.getByRole("heading", { level: 1, name: TITLE })).toBeInTheDocument();
    expect(document.body.textContent).toMatch(/ON_SALE|đang (mở )?bán/i);
  });

  it("phễu 6 bước hiện số tuyệt đối (định dạng dấu chấm ngăn nghìn)", () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    for (const v of ["911", "4.210", "733", "644", "555", "466"]) {
      expect(hasValue(v), `phễu ${v}`).toBe(true);
    }
  });

  it("tuyển sinh: admit rate kèm đơn vị khách/giây, chế độ AUTO, độ sâu hàng chờ", () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    expect(hasValue("37")).toBe(true);
    expect(screen.getAllByText(/khách\/giây/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/\bAUTO\b/).length).toBeGreaterThan(0);
    expect(hasValue("3.890")).toBe(true);
  });

  it("chế độ MANUAL được hiện khi admission.mode = MANUAL", () => {
    renderWarRoom({
      initialSnapshot: makeSnapshot({ admission: { ratePerSec: 37, mode: "MANUAL", queueDepth: 3890 } }),
    });
    expect(screen.getAllByText(/\bMANUAL\b/).length).toBeGreaterThan(0);
  });

  it("bảng tồn kho có đủ cột quota, đang giữ, đã thanh toán, đã phát hành, còn lại và số liệu từng hạng", () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    const headers = within(inventoryTable())
      .getAllByRole("columnheader")
      .map((h) => (h.textContent ?? "").trim());

    // So từng Ô với đúng cột của nó. Không so textContent của cả hàng: textContent nối
    // các ô không có dấu cách (`<td>1.000</td><td>21</td>` → "1.00021").
    const cells = Array.from(tierRow("Hạng A").children).map((c) => (c.textContent ?? "").trim());
    expect(cells).toHaveLength(headers.length);

    const expected: Array<[RegExp, string]> = [
      [/quota/i, "1.000"],
      [/đang giữ/i, "21"],
      [/đã thanh toán/i, "102"],
      [/đã phát hành/i, "93"],
      [/còn lại/i, "784"],
    ];
    for (const [re, value] of expected) {
      const idx = headers.findIndex((h) => re.test(h));
      expect(idx, `thiếu cột ${re}`).toBeGreaterThanOrEqual(0);
      // Cho phép hậu tố chữ trong cùng ô (vd "784 vé"), không cho dính thêm chữ số.
      expect(cells[idx], `cột ${re}`).toMatch(new RegExp(`^${escapeRe(value)}(\\D.*)?$`));
    }
  });

  it("AI: nấc suy biến, độ trễ chat (giây), % ngân sách token", () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    expect(screen.getAllByText(/\bNORMAL\b/).length).toBeGreaterThan(0);
    expect(hasValue("17")).toBe(true);
    // Có biên trái: "117 giây" không được tính là "17 giây". Xét từng phần tử (ô nhỏ nhất chứa
    // cả số và đơn vị), không xét textContent nối liền của cả trang.
    expect(anyElementText(/(^|[^\d.,])17(,0)?\s*giây/)).toBe(true);
    expect(hasPercent(63)).toBe(true);
  });

  it("mỗi khối có dòng phụ 'làm mới mỗi 2 giây' (m-7)", () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    expect(screen.getAllByText(/làm mới mỗi 2 giây/i).length).toBeGreaterThan(0);
  });
});

describe("OpsWarRoom — AC-2 độ tươi ≤ 5s", () => {
  it("hiện 'cập nhật lúc' theo giờ Việt Nam (formatTimeVn) và không cảnh báo khi dữ liệu mới", async () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    await flush();
    expect(document.body.textContent).toMatch(/cập nhật lúc:?\s*10:00:00/i);
    expect(hasBanner("alert", /dữ liệu cũ/i)).toBe(false);
  });

  it("dữ liệu quá 5s: banner 'dữ liệu cũ' role alert, số liệu vẫn giữ", async () => {
    const old = makeSnapshot({ generatedAt: new Date(BASE_MS - 10_000).toISOString() });
    mockGet.mockImplementation(async () => old);

    renderWarRoom({ initialSnapshot: old });
    await flush();
    await advance(1000);

    expect(hasBanner("alert", /dữ liệu cũ/i)).toBe(true);
    expect(hasValue("4.210")).toBe(true);
    expect(hasValue("644")).toBe(true);
  });

  it("vượt mốc 5s khi request treo thì chuyển sang cảnh báo cũ", async () => {
    mockGet.mockImplementation(() => new Promise<OpsWarRoomSnapshot>(() => {}));

    renderWarRoom({ initialSnapshot: makeSnapshot() });
    await flush();
    expect(hasBanner("alert", /dữ liệu cũ/i)).toBe(false);

    await advance(6000);
    expect(hasBanner("alert", /dữ liệu cũ/i)).toBe(true);
    expect(hasValue("911")).toBe(true);
  });

  it("lần làm mới gần nhất lỗi: cảnh báo cũ, KHÔNG xoá số liệu cuối", async () => {
    mockGet.mockRejectedValue(new Error("mất kết nối"));

    renderWarRoom({ initialSnapshot: makeSnapshot() });
    await advance(2000);

    expect(mockGet).toHaveBeenCalled();
    expect(hasBanner("alert", /dữ liệu cũ/i)).toBe(true);
    for (const v of ["911", "4.210", "733", "644", "555", "466"]) {
      expect(hasValue(v), v).toBe(true);
    }
  });

  it("tự làm mới mỗi 2s qua getOpsWarRoom(eventId)", async () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    await advance(2000);
    expect(mockGet).toHaveBeenCalledTimes(1);
    expect(mockGet).toHaveBeenCalledWith(EVENT_ID);
    await advance(2000);
    expect(mockGet).toHaveBeenCalledTimes(2);
  });

  it("số mới từ lần làm mới tự động được hiển thị", async () => {
    mockGet.mockImplementation(async () =>
      makeSnapshot({ funnel: { lobby: 1, queued: 8888, admitted: 3, holding: 4, paid: 5, issued: 6 } }),
    );
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    await advance(2000);
    expect(hasValue("8.888")).toBe(true);
  });
});

describe("OpsWarRoom — AC-3 không tô xanh giả", () => {
  it("isSample = true: nhãn 'Dữ liệu mẫu' role status, nằm trước các khối số liệu", () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    const label = liveRegions("status").find((el) => /Dữ liệu mẫu/i.test(el.textContent ?? ""));
    expect(label).toBeDefined();
    expect(
      label!.compareDocumentPosition(inventoryTable()) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("isSample = false: không có nhãn 'Dữ liệu mẫu'", () => {
    renderWarRoom({ initialSnapshot: makeSnapshot({ isSample: false }) });
    expect(screen.queryAllByText(/Dữ liệu mẫu/i)).toHaveLength(0);
  });

  it("waitingroom unavailable: lobby/queued/admitted + tuyển sinh hiện 'Không có dữ liệu'", () => {
    renderWarRoom({ initialSnapshot: withSources({ waitingroom: "unavailable" }) });
    expect(screen.getAllByText(/Không có dữ liệu/).length).toBeGreaterThan(0);
    for (const v of ["911", "4.210", "733", "37", "3.890"]) {
      expect(hasValue(v), `waitingroom ${v}`).toBe(false);
    }
    // Ô của nguồn khác vẫn hiện.
    expect(hasValue("644")).toBe(true);
    expect(hasValue("17")).toBe(true);
  });

  it("ticketing unavailable: holding/paid/issued + bảng tồn kho hiện 'Không có dữ liệu'", () => {
    renderWarRoom({ initialSnapshot: withSources({ ticketing: "unavailable" }) });
    expect(screen.getAllByText(/Không có dữ liệu/).length).toBeGreaterThan(0);
    for (const v of ["644", "555", "466", "102", "93", "784", "47"]) {
      expect(hasValue(v), `ticketing ${v}`).toBe(false);
    }
    expect(hasValue("911")).toBe(true);
  });

  it("ai unavailable: khối AI hiện 'Không có dữ liệu', không hiện nấc/độ trễ/ngân sách", () => {
    renderWarRoom({
      initialSnapshot: makeSnapshot({
        ai: { mode: "SAVING", manualOverride: false, chatQueueLagSeconds: 17, budgetUsedRatio: 0.63 },
        sources: { waitingroom: "ok", ticketing: "ok", ai: "unavailable" },
      }),
    });
    expect(screen.getAllByText(/Không có dữ liệu/).length).toBeGreaterThan(0);
    expect(hasValue("17")).toBe(false);
    expect(hasPercent(63)).toBe(false);
    expect(screen.queryAllByText(/\bSAVING\b/)).toHaveLength(0);
    expect(hasOwnText(AI_COPY.SAVING)).toBe(false);
    expect(hasValue("911")).toBe(true);
  });

  it("mọi nguồn ok: không có nhãn 'cũ' và không có 'Không có dữ liệu'", () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    expect(anyOwnText(STALE_WORD)).toBe(false);
    expect(screen.queryAllByText(/Không có dữ liệu/)).toHaveLength(0);
  });

  it.each(["waitingroom", "ticketing", "ai"] as const)(
    "nguồn %s stale: ô vẫn hiện số nhưng gắn nhãn 'cũ'",
    (source) => {
      renderWarRoom({ initialSnapshot: withSources({ [source]: "stale" }) });
      expect(anyOwnText(STALE_WORD)).toBe(true);
      const sample = { waitingroom: "4.210", ticketing: "644", ai: "17" }[source];
      expect(hasValue(sample)).toBe(true);
    },
  );

  it.each([
    ["waitingroom", "unavailable"],
    ["ticketing", "unavailable"],
    ["ai", "unavailable"],
    ["waitingroom", "stale"],
    ["ai", "stale"],
  ] as const)("nguồn %s = %s: không có câu 'hệ thống bình thường'", (source, health) => {
    renderWarRoom({ initialSnapshot: withSources({ [source]: health }) });
    expect(HEALTH_CLAIM.test(document.body.textContent ?? "")).toBe(false);
  });
});

describe("OpsWarRoom — AC-4 bất biến tồn kho BR-O1", () => {
  it("held + paid + issued > quota: hàng bị đánh dấu bằng chữ + banner 'lệch tồn kho' role alert", () => {
    const snap = makeSnapshot({
      inventory: [
        { tierId: "a", name: "Hạng A", quota: 100, held: 21, paid: 102, issued: 93, available: 0 },
        { tierId: "b", name: "Hạng B", quota: 150, held: 13, paid: 47, issued: 38, available: 52 },
      ],
    });
    renderWarRoom({ initialSnapshot: snap });

    expect(hasBanner("alert", /lệch tồn kho/i)).toBe(true);
    const bad = tierRow("Hạng A").textContent ?? "";
    const ok = tierRow("Hạng B").textContent ?? "";
    const MARK = /vượt|âm|lỗi|lệch|sai/i;
    expect(bad).toMatch(MARK);
    expect(ok).not.toMatch(MARK);
  });

  it("available < 0: hàng bị đánh dấu bằng chữ + banner 'lệch tồn kho'", () => {
    const snap = makeSnapshot({
      inventory: [
        { tierId: "a", name: "Hạng A", quota: 1000, held: 21, paid: 102, issued: 93, available: 784 },
        { tierId: "b", name: "Hạng B", quota: 150, held: 13, paid: 47, issued: 38, available: -5 },
      ],
    });
    renderWarRoom({ initialSnapshot: snap });

    expect(hasBanner("alert", /lệch tồn kho/i)).toBe(true);
    expect(tierRow("Hạng B").textContent).toMatch(/vượt|âm|lỗi|lệch|sai/i);
    expect(tierRow("Hạng A").textContent).not.toMatch(/vượt|âm|lỗi|lệch|sai/i);
  });

  it("tồn kho hợp lệ: không có cảnh báo lệch tồn kho", () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    expect(screen.queryAllByText(/lệch tồn kho/i)).toHaveLength(0);
  });

  it.each(["unavailable", "stale"] as const)(
    "ticketing %s: KHÔNG kiểm/không báo BR-O1 dù số liệu vi phạm (M-3)",
    (health) => {
      renderWarRoom({
        initialSnapshot: makeSnapshot({
          inventory: [
            { tierId: "a", name: "Hạng A", quota: 10, held: 21, paid: 102, issued: 93, available: -206 },
          ],
          sources: { waitingroom: "ok", ticketing: health, ai: "ok" },
        }),
      });
      expect(screen.queryAllByText(/lệch tồn kho/i)).toHaveLength(0);
    },
  );
});

describe("OpsWarRoom — AC-5 nấc suy biến AI", () => {
  it.each(MODES)("mode %s: hiện đúng nấc đó và câu tác động nguyên văn", (mode) => {
    renderWarRoom({
      initialSnapshot: makeSnapshot({
        ai: { mode, manualOverride: false, chatQueueLagSeconds: 17, budgetUsedRatio: 0.63 },
      }),
    });

    expect(screen.getAllByText(new RegExp(`\\b${mode}\\b`)).length).toBeGreaterThan(0);
    expect(hasOwnText(AI_COPY[mode])).toBe(true);

    for (const other of MODES.filter((m) => m !== mode)) {
      expect(screen.queryAllByText(new RegExp(`\\b${other}\\b`)), other).toHaveLength(0);
      expect(hasOwnText(AI_COPY[other]), other).toBe(false);
    }
  });

  it("manualOverride = true: ghi rõ 'Ops đặt tay'", () => {
    renderWarRoom({
      initialSnapshot: makeSnapshot({
        ai: { mode: "SAVING", manualOverride: true, chatQueueLagSeconds: 17, budgetUsedRatio: 0.63 },
      }),
    });
    expect(screen.getAllByText(/Ops đặt tay/).length).toBeGreaterThan(0);
  });

  it("manualOverride = false: không có 'Ops đặt tay'", () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    expect(screen.queryAllByText(/Ops đặt tay/)).toHaveLength(0);
  });
});

describe("OpsWarRoom — AC-6 điều khiển tách khỏi giám sát", () => {
  it("có khu 'Giám sát' và khu 'Điều khiển' riêng", () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    expect(screen.getAllByRole("heading", { name: /giám sát/i }).length).toBeGreaterThan(0);
    expect(screen.getAllByRole("heading", { name: /điều khiển/i }).length).toBeGreaterThan(0);
  });

  it("nút ghi đè admit rate / tạm dừng bị vô hiệu, kèm giải thích EVF-60", () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    const buttons = controlButtons();
    expect(buttons.length).toBeGreaterThan(0);
    for (const b of buttons) {
      expect(b).toBeDisabled();
    }
    expect(document.body.textContent).toContain("Chưa có API ghi đè (EVF-60)");
  });

  it("bấm nút điều khiển không gửi request nào", async () => {
    const fetchSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    await flush();
    const before = mockGet.mock.calls.length;

    for (const b of controlButtons()) {
      fireEvent.click(b);
    }
    await flush();

    expect(fetchSpy).not.toHaveBeenCalled();
    expect(mockGet).toHaveBeenCalledTimes(before);
  });
});

describe("OpsWarRoom — AC-7 trạng thái đặc biệt", () => {
  it("initialError 403: màn 'Không có quyền', không poll", async () => {
    mockGet.mockRejectedValue(await realOpsError(403));
    renderWarRoom({ initialError: { status: 403, message: "forbidden" } });
    await flush();

    expect(screen.getAllByText(/Không có quyền/i).length).toBeGreaterThan(0);
    const calls = mockGet.mock.calls.length;
    await advance(20_000);
    expect(mockGet.mock.calls.length).toBe(calls);
  });

  it("initialError 404: 'Không tìm thấy sự kiện'", async () => {
    mockGet.mockRejectedValue(await realOpsError(404));
    renderWarRoom({ initialError: { status: 404, message: "not found" } });
    await flush();
    expect(screen.getAllByText(/Không tìm thấy sự kiện/i).length).toBeGreaterThan(0);
  });

  it("initialError khác (500): thông báo lỗi + nút 'Thử lại'; thử lại thành công thì hiện số liệu", async () => {
    mockGet.mockRejectedValue(await realOpsError(500));
    renderWarRoom({ initialError: { status: 500, message: "lỗi máy chủ" } });
    await flush();

    expect(screen.queryAllByText(/Không có quyền/i)).toHaveLength(0);
    const retry = screen.getByRole("button", { name: /thử lại/i });

    mockGet.mockReset();
    mockGet.mockImplementation(async () => makeSnapshot());
    fireEvent.click(retry);
    await flush();

    expect(mockGet).toHaveBeenCalledWith(EVENT_ID);
    expect(hasValue("4.210")).toBe(true);
  });

  it("không có initialSnapshot, lần tải đầu 403: 'Không có quyền'", async () => {
    mockGet.mockRejectedValue(await realOpsError(403));
    renderWarRoom({});
    await flush();
    expect(screen.getAllByText(/Không có quyền/i).length).toBeGreaterThan(0);
  });

  it("không có initialSnapshot, lần tải đầu 404: 'Không tìm thấy sự kiện'", async () => {
    mockGet.mockRejectedValue(await realOpsError(404));
    renderWarRoom({});
    await flush();
    expect(screen.getAllByText(/Không tìm thấy sự kiện/i).length).toBeGreaterThan(0);
  });

  it("không có initialSnapshot, lần tải đầu lỗi khác: nút 'Thử lại'", async () => {
    mockGet.mockRejectedValue(new Error("mạng lỗi"));
    renderWarRoom({});
    await flush();
    expect(screen.getByRole("button", { name: /thử lại/i })).toBeInTheDocument();
  });

  it("403 giữa chừng: chuyển sang màn 'Không có quyền' và dừng poll", async () => {
    mockGet.mockRejectedValue(await realOpsError(403));
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    await advance(2000);

    expect(screen.getAllByText(/Không có quyền/i).length).toBeGreaterThan(0);
    expect(mockGet).toHaveBeenCalledTimes(1);
    await advance(20_000);
    expect(mockGet).toHaveBeenCalledTimes(1);
  });

  it.each(["SCHEDULED", "SOLD_OUT", "CLOSED"] as const)(
    "saleState %s: nhãn 'Không có đợt bán đang diễn ra', vẫn hiện số liệu",
    (saleState) => {
      renderWarRoom({ initialSnapshot: makeSnapshot({ saleState }) });
      expect(screen.getAllByText(/Không có đợt bán đang diễn ra/i).length).toBeGreaterThan(0);
      expect(hasValue("644")).toBe(true);
    },
  );

  it("saleState ON_SALE: không có nhãn 'Không có đợt bán đang diễn ra'", () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    expect(screen.queryAllByText(/Không có đợt bán đang diễn ra/i)).toHaveLength(0);
  });
});

describe("OpsWarRoom — AC-9 làm mới thủ công", () => {
  it("nút 'Làm mới' gọi getOpsWarRoom(eventId) ngay và hiện số mới", async () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    await flush();
    expect(mockGet).not.toHaveBeenCalled();

    mockGet.mockImplementation(async () =>
      makeSnapshot({ funnel: { lobby: 1, queued: 7777, admitted: 3, holding: 4, paid: 5, issued: 6 } }),
    );
    fireEvent.click(refreshButton());
    await flush();

    expect(mockGet).toHaveBeenCalledTimes(1);
    expect(mockGet).toHaveBeenCalledWith(EVENT_ID);
    expect(hasValue("7.777")).toBe(true);
  });
});

describe("OpsWarRoom — AC-11 truy cập được", () => {
  it("bảng tồn kho là <table> có <caption> và header cột", () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    const table = inventoryTable();
    expect(table.tagName).toBe("TABLE");
    const caption = table.querySelector("caption");
    expect(caption).not.toBeNull();
    expect((caption?.textContent ?? "").trim().length).toBeGreaterThan(0);
    expect(within(table).getAllByRole("columnheader").length).toBeGreaterThanOrEqual(5);
  });

  it("cập nhật tự động không cướp focus", async () => {
    let n = 0;
    mockGet.mockImplementation(async () => {
      n += 1;
      return makeSnapshot({
        funnel: { lobby: 900 + n, queued: 4210, admitted: 733, holding: 644, paid: 555, issued: 466 },
      });
    });
    renderWarRoom({ initialSnapshot: makeSnapshot() });

    const btn = refreshButton();
    btn.focus();
    expect(document.activeElement).toBe(btn);

    await advance(2000);
    await advance(2000);
    expect(mockGet).toHaveBeenCalledTimes(2);
    expect(document.activeElement).toBe(btn);
  });

  it("khi không có gì được focus, cập nhật tự động không đặt focus vào đâu", async () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    (document.activeElement as HTMLElement | null)?.blur();
    const before = document.activeElement;

    await advance(2000);
    await advance(2000);
    expect(document.activeElement).toBe(before);
  });

  it("không đặt aria-live lên vùng số liệu (m-8)", () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    for (const el of Array.from(document.querySelectorAll("[aria-live]"))) {
      if (el.getAttribute("aria-live") === "off") continue;
      expect(el.textContent ?? "").not.toContain("4.210");
      expect(el.querySelector("table")).toBeNull();
    }
  });
});

describe("OpsWarRoom — bổ sung sau code review (plan §7)", () => {
  it("ratePerSec và chatQueueLagSeconds hiển thị bằng formatDecimal (12.7 → '12,7') (M-1)", () => {
    renderWarRoom({
      initialSnapshot: makeSnapshot({
        admission: { ratePerSec: 12.7, mode: "AUTO", queueDepth: 3890 },
        ai: { mode: "NORMAL", manualOverride: false, chatQueueLagSeconds: 4.5, budgetUsedRatio: 0.63 },
      }),
    });
    expect(anyOwnText(/(^|[^\d.,])12,7(?![\d])/)).toBe(true);
    expect(anyElementText(/(^|[^\d.,])4,5\s*giây/)).toBe(true);
    expect(anyOwnText(/12\.7/)).toBe(false);
    expect(anyOwnText(/4\.5/)).toBe(false);
  });

  it("ratePerSec nguyên vẫn có một chữ số thập phân (50 → '50,0')", () => {
    renderWarRoom({
      initialSnapshot: makeSnapshot({ admission: { ratePerSec: 50, mode: "AUTO", queueDepth: 3890 } }),
    });
    expect(anyOwnText(/(^|[^\d.,])50,0(?![\d])/)).toBe(true);
  });

  it("nút 'Làm mới' khi đang làm mới: aria-busy='true', KHÔNG disabled, giữ focus, bấm lại không gọi thêm (m-2)", async () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    await flush();

    let resolve!: (s: OpsWarRoomSnapshot) => void;
    mockGet.mockImplementation(
      () =>
        new Promise<OpsWarRoomSnapshot>((res) => {
          resolve = res;
        }),
    );

    const btn = refreshButton();
    btn.focus();
    fireEvent.click(btn);
    await flush();

    const busy = refreshButton();
    expect(busy).toHaveAttribute("aria-busy", "true");
    expect(busy).not.toBeDisabled();
    expect(document.activeElement).toBe(busy);
    expect(busy).toBe(btn);

    fireEvent.click(busy);
    await flush();
    expect(mockGet).toHaveBeenCalledTimes(1);

    await act(async () => {
      resolve(makeSnapshot());
    });
    await flush();
    expect(refreshButton()).not.toHaveAttribute("aria-busy", "true");
    expect(document.activeElement).toBe(refreshButton());
  });

  it("'cập nhật lúc …' nằm cùng khối với nhãn 'Dữ liệu mẫu' (m-5)", () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    const label = liveRegions("status").find((el) => /Dữ liệu mẫu/i.test(el.textContent ?? ""));
    expect(label).toBeDefined();
    const container = label!.parentElement!;
    expect(container.textContent ?? "").toMatch(/cập nhật lúc:?\s*10:00:00/i);
  });

  it("nút điều khiển vô hiệu có aria-describedby trỏ tới đoạn chứa 'EVF-60' (m-9)", () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    for (const b of controlButtons()) {
      const ids = (b.getAttribute("aria-describedby") ?? "").split(/\s+/).filter(Boolean);
      expect(ids.length, b.textContent ?? "").toBeGreaterThan(0);
      const described = ids.map((id) => document.getElementById(id)?.textContent ?? "").join(" ");
      expect(described).toContain("EVF-60");
    }
  });

  it("HTML server (renderToString) với generatedAt cũ: không có tuổi dữ liệu, không banner 'Dữ liệu cũ' (t-2)", () => {
    const old = makeSnapshot({ generatedAt: new Date(BASE_MS - 10 * 60_000).toISOString() });

    vi.setSystemTime(BASE_MS);
    const htmlA = renderToString(<OpsWarRoom eventId={EVENT_ID} initialSnapshot={old} />);
    vi.setSystemTime(BASE_MS + 60 * 60_000);
    const htmlB = renderToString(<OpsWarRoom eventId={EVENT_ID} initialSnapshot={old} />);

    expect(htmlA).not.toMatch(/dữ liệu cũ/i);
    // Không phụ thuộc đồng hồ lúc render: tuổi dữ liệu chỉ có sau khi mount ở client.
    expect(htmlB).toBe(htmlA);
    expect(htmlA).toContain("Dữ liệu mẫu");
  });
});

describe("OpsWarRoom — giá trị lạ không lộ ra UI (plan §8 C-3, C-4)", () => {
  function dashCount(): number {
    return ownTexts().filter((t) => t.trim() === "—").length;
  }

  it("budgetUsedRatio không hữu hạn → '—', không có NaN/Infinity", () => {
    renderWarRoom({ initialSnapshot: makeSnapshot() });
    const normal = dashCount();
    cleanup();

    renderWarRoom({
      initialSnapshot: makeSnapshot({
        ai: { mode: "NORMAL", manualOverride: false, chatQueueLagSeconds: 17, budgetUsedRatio: Number.NaN },
      }),
    });
    expect(document.body.textContent).not.toMatch(/NaN|Infinity/);
    expect(dashCount()).toBeGreaterThan(normal);
    expect(hasPercent(63)).toBe(false);
  });

  it("budgetUsedRatio = Infinity cũng → '—'", () => {
    renderWarRoom({
      initialSnapshot: makeSnapshot({
        ai: {
          mode: "NORMAL",
          manualOverride: false,
          chatQueueLagSeconds: 17,
          budgetUsedRatio: Number.POSITIVE_INFINITY,
        },
      }),
    });
    expect(document.body.textContent).not.toMatch(/NaN|Infinity/);
  });

  it("saleState lạ → nhãn 'Không rõ trạng thái', không lộ mã thô", () => {
    renderWarRoom({
      initialSnapshot: makeSnapshot({ saleState: "PAUSED_X" as unknown as OpsWarRoomSnapshot["saleState"] }),
    });
    expect(screen.getAllByText(/Không rõ trạng thái/).length).toBeGreaterThan(0);
    expect(document.body.textContent).not.toContain("PAUSED_X");
  });

  it("ai.mode lạ → hiện mã kèm 'Không rõ tác động tới người mua.'", () => {
    renderWarRoom({
      initialSnapshot: makeSnapshot({
        ai: {
          mode: "TURBO_X" as unknown as DegradationMode,
          manualOverride: false,
          chatQueueLagSeconds: 17,
          budgetUsedRatio: 0.63,
        },
      }),
    });
    expect(screen.getAllByText(/\bTURBO_X\b/).length).toBeGreaterThan(0);
    expect(hasOwnText("Không rõ tác động tới người mua.")).toBe(true);
    for (const m of MODES) {
      expect(hasOwnText(AI_COPY[m]), m).toBe(false);
    }
  });

  it("inventory rỗng → một hàng 'Chưa có hạng vé.' trong bảng", () => {
    renderWarRoom({ initialSnapshot: makeSnapshot({ inventory: [] }) });
    const table = screen.getAllByRole("table").find((t) => (t.textContent ?? "").includes("Chưa có hạng vé."));
    expect(table).toBeDefined();
    const rows = within(table!).getAllByRole("row").filter((r) => (r.textContent ?? "").includes("Chưa có hạng vé."));
    expect(rows).toHaveLength(1);
    expect(screen.queryAllByText(/lệch tồn kho/i)).toHaveLength(0);
  });

  it("tier có số không hữu hạn: hàng ghi 'Dữ liệu không hợp lệ' + banner lệch tồn kho (C-4)", () => {
    renderWarRoom({
      initialSnapshot: makeSnapshot({
        inventory: [
          { tierId: "a", name: "Hạng A", quota: 1000, held: 21, paid: Number.NaN, issued: 93, available: 784 },
          { tierId: "b", name: "Hạng B", quota: 150, held: 13, paid: 47, issued: 38, available: 52 },
        ],
      }),
    });
    expect(tierRow("Hạng A").textContent).toMatch(/Dữ liệu không hợp lệ/);
    expect(tierRow("Hạng B").textContent).not.toMatch(/Dữ liệu không hợp lệ/);
    expect(hasBanner("alert", /lệch tồn kho/i)).toBe(true);
    expect(document.body.textContent).not.toMatch(/NaN/);
  });
});

describe("OpsWarRoom — nút 'Thử lại' (plan §8 C-5)", () => {
  it("đang thử lại: aria-busy='true', KHÔNG disabled, giữ focus", async () => {
    mockGet.mockRejectedValue(await realOpsError(500));
    renderWarRoom({ initialError: { status: 500, message: "lỗi máy chủ" } });
    await flush();

    mockGet.mockReset();
    mockGet.mockImplementation(() => new Promise<OpsWarRoomSnapshot>(() => {}));

    const retry = screen.getByRole("button", { name: /thử lại/i });
    retry.focus();
    fireEvent.click(retry);
    await flush();

    const busy = screen.getByRole("button", { name: /thử lại/i });
    expect(busy).toBe(retry);
    expect(busy).toHaveAttribute("aria-busy", "true");
    expect(busy).not.toBeDisabled();
    expect(document.activeElement).toBe(busy);

    fireEvent.click(busy);
    await flush();
    expect(mockGet).toHaveBeenCalledTimes(1);
  });
});

describe("OpsWarRoom — mutant sống (plan §8 C-6)", () => {
  /** Phần tử nhỏ nhất chứa `value` mà không chứa các giá trị của bước phễu khác. */
  function cellHasStaleLabel(value: string, others: string[]): boolean {
    return Array.from(document.body.querySelectorAll("*")).some((el) => {
      const t = el.textContent ?? "";
      return t.includes(value) && t.includes("(cũ)") && others.every((o) => !t.includes(o));
    });
  }

  const FUNNEL = ["911", "4.210", "733", "644", "555", "466"];

  it("M46: waitingroom stale → ô lobby/queued/admitted có '(cũ)', ô của ticketing thì không", () => {
    renderWarRoom({ initialSnapshot: withSources({ waitingroom: "stale" }) });
    for (const v of ["911", "4.210", "733"]) {
      expect(cellHasStaleLabel(v, FUNNEL.filter((o) => o !== v)), v).toBe(true);
    }
    for (const v of ["644", "555", "466"]) {
      expect(cellHasStaleLabel(v, FUNNEL.filter((o) => o !== v)), v).toBe(false);
    }
  });

  it("M46: ticketing stale → ô holding/paid/issued có '(cũ)', ô của waitingroom thì không", () => {
    renderWarRoom({ initialSnapshot: withSources({ ticketing: "stale" }) });
    for (const v of ["644", "555", "466"]) {
      expect(cellHasStaleLabel(v, FUNNEL.filter((o) => o !== v)), v).toBe(true);
    }
    for (const v of ["911", "4.210", "733"]) {
      expect(cellHasStaleLabel(v, FUNNEL.filter((o) => o !== v)), v).toBe(false);
    }
  });

  it("M17: ticketing stale + số vi phạm → hàng KHÔNG có badge vi phạm", () => {
    renderWarRoom({
      initialSnapshot: makeSnapshot({
        inventory: [
          { tierId: "a", name: "Hạng A", quota: 10, held: 21, paid: 102, issued: 93, available: -206 },
        ],
        sources: { waitingroom: "ok", ticketing: "stale", ai: "ok" },
      }),
    });
    expect(tierRow("Hạng A").textContent).not.toMatch(/vượt|âm|lỗi|lệch|sai|không hợp lệ/i);
  });

  it("M47: initialError 403 → không có request nào từ client", async () => {
    renderWarRoom({ initialError: { status: 403, message: "forbidden" } });
    await flush();
    await advance(20_000);
    expect(mockGet).not.toHaveBeenCalled();
    expect(screen.getAllByText(/Không có quyền/i).length).toBeGreaterThan(0);
  });
});
