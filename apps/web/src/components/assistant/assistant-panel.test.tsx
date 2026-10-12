/**
 * Test cho `AssistantPanel` — AC-1..AC-9 (spec-plan EVF-1806 §4), BR-A1..A7,
 * handoff §10.11, §11.7, §5.
 *
 * Viet TRUOC khi co cai dat, chi doc spec. Khong doc code component.
 *
 * ==> BA TEST QUAN TRONG NHAT:
 *     - describe "ranh gioi tham quyen" (AC-1, BR-A4): tro ly KHONG co cho cho so lieu.
 *       Props khong co `rank`/`eta`/ton kho (chot bang `@ts-expect-error`), va panel
 *       luon noi ro: khong giu ve, khong doi don, so lieu xem o bang trang thai.
 *     - describe "khong bao loi ky thuat" (§2.5, AC goc Jira): o SAVING/FAQ_ONLY/OFF,
 *       tin `failed` va het luot, copy KHONG duoc chua `lỗi|error|429|rate limit|Gemini|
 *       timeout|server|quota|token`. Khach trong phong cho khong can biet Gemini la gi.
 *     - describe "text la du lieu" (AC-2, BR-A7): `<script>` hay `[link](…)` trong `text`
 *       phai hien NGUYEN VAN, khong thanh phan tu DOM.
 *
 * Cach test noi dung chu: spec KHONG chot cau tieng Viet, nen test chot HANH VI (cai gi
 * phai/khong duoc xuat hien, control nao bi disabled) va chi dung regex rong ma §4 neu
 * truc tiep. Khong assert nguyen cau.
 *
 * Gio `HH:mm` tu `sentAt`: khong gia dinh mui gio cua may chay test — chi assert "co MOT
 * chuoi dang HH:mm" khi `sentAt` hop le, va KHONG co khi `sentAt` thieu/rac.
 */
import { fireEvent, render, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  AssistantPanel,
  type AssistantAnswerKind,
  type AssistantFaq,
  type AssistantMessage,
  type AssistantMode,
  type AssistantPanelProps,
} from "./assistant-panel";

const MODES: AssistantMode[] = ["NORMAL", "SAVING", "FAQ_ONLY", "OFF"];
const MODES_WITH_CHAT: AssistantMode[] = ["NORMAL", "SAVING", "FAQ_ONLY"];
const KINDS: AssistantAnswerKind[] = ["ai", "faq", "out_of_scope", "fallback"];

/** Moc co dinh; chi dung de kiem "co gio" / "khong co gio", khong kiem gia tri. */
const SENT_AT = Date.UTC(2026, 0, 15, 3, 5, 0);
// Khong dung `\b`: textContent noi cac node lien nhau ("Bạn10:05Còn") va chu truoc so la
// word char nen `\b` khong co ranh gioi. Dung lookaround tren CHU SO.
const HHMM = /(?<!\d)\d{2}:\d{2}(?!\d)/;

const SUPPORT_HREF = "https://example.test/ho-tro";

const FAQS: AssistantFaq[] = [
  { id: "faq-open", label: "Mở bán lúc nào?" },
  { id: "faq-pay", label: "Thanh toán bằng gì?" },
  { id: "faq-ticket", label: "Nhận vé ở đâu?" },
];

const USER_MSG: AssistantMessage = {
  id: "u1",
  role: "user",
  text: "Còn vé không?",
};

const ASSISTANT_MSG: AssistantMessage = {
  id: "a1",
  role: "assistant",
  text: "Xin xem bảng trạng thái.",
};

const PENDING_MSG: AssistantMessage = {
  id: "u-pending",
  role: "user",
  text: "Mở bán lúc mấy giờ?",
  status: "pending",
};

const FAILED_MSG: AssistantMessage = {
  id: "u-failed",
  role: "user",
  text: "Mở bán lúc mấy giờ?",
  status: "failed",
};

/**
 * §2.5 — tu ky thuat bi cam trong copy nhin thay. "quota" tieng Anh bi cam, "lượt"
 * tieng Viet thi duoc. Quet case-insensitive tren toan textContent cua panel.
 */
const FORBIDDEN = /lỗi|error|429|rate limit|gemini|timeout|server|\bquota\b|token/i;

function forbidden(textContent: string | null | undefined): boolean {
  return FORBIDDEN.test(textContent ?? "");
}

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

function accName(el: Element): string {
  return norm(el.getAttribute("aria-label") ?? el.textContent);
}

function isDisabled(el: Element | null): boolean {
  if (!el) return false;
  return (
    (el as HTMLButtonElement | HTMLTextAreaElement).disabled === true ||
    el.getAttribute("aria-disabled") === "true"
  );
}

function root(container: HTMLElement): HTMLElement {
  const el = container.querySelector<HTMLElement>('[data-slot="assistant-panel"]');
  if (!el) throw new Error("khong tim thay goc [data-slot=assistant-panel]");
  return el;
}

function textarea(container: HTMLElement): HTMLTextAreaElement | null {
  return container.querySelector("textarea");
}

function submitButton(container: HTMLElement): HTMLButtonElement | null {
  return container.querySelector<HTMLButtonElement>('button[type="submit"]');
}

function faqGroup(container: HTMLElement): HTMLElement | null {
  return Array.from(container.querySelectorAll<HTMLElement>('[role="group"]')).find(
    (el) => el.getAttribute("aria-label") === "Câu hỏi thường gặp",
  ) ?? null;
}

function faqChips(container: HTMLElement): HTMLButtonElement[] {
  const group = faqGroup(container);
  if (!group) return [];
  return Array.from(group.querySelectorAll<HTMLButtonElement>("button"));
}

function messages(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>('[data-slot="message"]'));
}

function statusNodes(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>('[role="status"]'));
}

/** Go chu vao textarea roi bam Gui. */
function typeAndSubmit(container: HTMLElement, value: string): void {
  const input = textarea(container);
  if (!input) throw new Error("khong co textarea");
  fireEvent.change(input, { target: { value } });
  const button = submitButton(container);
  if (!button) throw new Error("khong co nut Gui");
  fireEvent.click(button);
}

/** Go chu vao textarea roi nhan Enter (co/khong Shift). */
function typeAndEnter(container: HTMLElement, value: string, shiftKey = false): void {
  const input = textarea(container);
  if (!input) throw new Error("khong co textarea");
  fireEvent.change(input, { target: { value } });
  fireEvent.keyDown(input, { key: "Enter", code: "Enter", shiftKey });
}

function renderPanel(props: Partial<AssistantPanelProps> & { mode: AssistantMode }) {
  return render(<AssistantPanel messages={[]} {...props} />);
}

afterEach(() => {
  vi.restoreAllMocks();
});

/* -------------------------------------------------------------------------- */
/* AC-1                                                                        */
/* -------------------------------------------------------------------------- */

/**
 * Chot KIEU — chay o `npm run typecheck`, khong chay runtime.
 * BR-A4: tro ly khong co tham quyen => props KHONG duoc co cho nhet so lieu.
 */
type ForbiddenPropKeys =
  | "rank"
  | "eta"
  | "etaSeconds"
  | "available"
  | "stock"
  | "inventory"
  | "orderStatus";
type LeakedKeys = Extract<keyof AssistantPanelProps, ForbiddenPropKeys>;
// Neu mot key cam lot vao props, `LeakedKeys` khac `never` va dong duoi do typecheck.
const NO_LEAKED_KEYS: [LeakedKeys] extends [never] ? true : never = true;

// Ham nay KHONG BAO GIO duoc goi; chi de typecheck bat `rank`.
function typeLevelRankMustBeRejected(): void {
  // @ts-expect-error -- `rank` khong ton tai trong AssistantPanelProps (AC-1, BR-A4)
  void (<AssistantPanel mode="NORMAL" messages={[]} rank={1} />);
}
void typeLevelRankMustBeRejected;

describe("AssistantPanel — ranh gioi tham quyen, khong co cho cho so lieu (AC-1)", () => {
  it("props khong co key rank/eta/etaSeconds/available/stock/inventory/orderStatus (chot bang typecheck) (AC-1)", () => {
    expect(NO_LEAKED_KEYS).toBe(true);
  });

  it.each(MODES_WITH_CHAT)(
    "mode %s co khoi authority-notice voi role=note (AC-1)",
    (mode) => {
      const { container } = renderPanel({ mode });
      const notice = container.querySelector('[data-slot="authority-notice"]');

      expect(notice).not.toBeNull();
      expect(notice?.getAttribute("role")).toBe("note");
    },
  );

  it.each(MODES_WITH_CHAT)(
    "mode %s: authority-notice noi ro KHONG giu ve, KHONG doi don, xem bang trang thai chinh thuc (AC-1)",
    (mode) => {
      const { container } = renderPanel({ mode });
      const notice = container.querySelector<HTMLElement>('[data-slot="authority-notice"]');
      const text = norm(notice?.textContent);

      expect(text).toMatch(/không.*giữ vé/i);
      expect(text).toMatch(/không.*đơn/i);
      expect(text).toMatch(/bảng trạng thái|trạng thái chính thức/i);
    },
  );

  it.each(MODES)('mode %s: goc co data-authoritative="false" (AC-1)', (mode) => {
    const { container } = renderPanel({ mode });

    expect(root(container).getAttribute("data-authoritative")).toBe("false");
  });

  it.each(MODES)("mode %s: nhan nhin thay 'AI hỗ trợ' (AC-1)", (mode) => {
    const { container } = renderPanel({ mode });

    expect(norm(root(container).textContent)).toContain("AI hỗ trợ");
  });

  it('goc la role="region" voi aria-label "Trợ lý sự kiện" (AC-1)', () => {
    const { getByRole } = renderPanel({ mode: "NORMAL" });
    const region = getByRole("region", { name: "Trợ lý sự kiện" });

    expect(region.getAttribute("data-slot")).toBe("assistant-panel");
  });

  it("messages=[] render duoc, khong throw, khong co tin nao trong log (AC-1)", () => {
    expect(() => renderPanel({ mode: "NORMAL", messages: [] }).unmount()).not.toThrow();

    const { container, getByRole } = renderPanel({ mode: "NORMAL", messages: [] });
    const log = getByRole("log");

    expect(log).toBeInTheDocument();
    expect(messages(container)).toHaveLength(0);
  });

  it("messages=[] van co chu (trang thai chao mung), khong lo 'undefined'/'NaN' (AC-1)", () => {
    const { container } = renderPanel({ mode: "NORMAL", messages: [] });
    const text = allText(container);

    expect(text.length).toBeGreaterThan(3);
    expect(text).not.toContain("undefined");
    expect(text).not.toContain("NaN");
    expect(text).not.toContain("null");
  });

  it("panel khong tu them con so nao ngoai text cua tin nhan va han muc (AC-1, BR-A4)", () => {
    // Khong truyen quota, tin nhan khong co so -> UI khong duoc co bat ky chu so nao
    // (khong duoc tu bia rank/ETA/ton kho).
    const { container } = renderPanel({
      mode: "NORMAL",
      messages: [USER_MSG, ASSISTANT_MSG],
      faqs: FAQS,
    });

    expect(allText(container)).not.toMatch(/\d/);
  });
});

/* -------------------------------------------------------------------------- */
/* AC-2                                                                        */
/* -------------------------------------------------------------------------- */

describe("AssistantPanel — render tin nhan, text la du lieu (AC-2, BR-A7)", () => {
  it("hai tin xuat hien trong role=log, dung thu tu mang, dung data-role (AC-2)", () => {
    const { getByRole } = renderPanel({
      mode: "NORMAL",
      messages: [USER_MSG, ASSISTANT_MSG],
    });
    const log = getByRole("log");
    const items = Array.from(log.querySelectorAll<HTMLElement>('[data-slot="message"]'));

    expect(items).toHaveLength(2);
    expect(items[0]?.getAttribute("data-role")).toBe("user");
    expect(norm(items[0]?.textContent)).toContain(USER_MSG.text);
    expect(items[1]?.getAttribute("data-role")).toBe("assistant");
    expect(norm(items[1]?.textContent)).toContain(ASSISTANT_MSG.text);
  });

  it("thu tu hien thi dao theo mang: assistant truoc user thi hien assistant truoc (AC-2)", () => {
    const { container } = renderPanel({
      mode: "NORMAL",
      messages: [ASSISTANT_MSG, USER_MSG],
    });
    const items = messages(container);

    expect(items.map((el) => el.getAttribute("data-role"))).toEqual(["assistant", "user"]);
  });

  it("text chua HTML/markdown/script hien NGUYEN VAN, khong tao phan tu b/a/script (AC-2, BR-A7)", () => {
    const raw = "<b>đậm</b> [link](http://x) <script>alert(1)</script>";
    const { container } = renderPanel({
      mode: "NORMAL",
      messages: [{ id: "a-raw", role: "assistant", text: raw }],
    });
    const log = container.querySelector<HTMLElement>('[role="log"]');

    expect(log).not.toBeNull();
    expect(log?.querySelector("b, a, script")).toBeNull();
    expect(log?.textContent ?? "").toContain(raw);
  });

  it("text cua tin nguoi dung cung hien nguyen van, khong parse (AC-2, BR-A7)", () => {
    const raw = "<img src=x onerror=alert(1)> **đậm**";
    const { container } = renderPanel({
      mode: "NORMAL",
      messages: [{ id: "u-raw", role: "user", text: raw }],
    });
    const log = container.querySelector<HTMLElement>('[role="log"]');

    expect(log?.querySelector("img, strong, b")).toBeNull();
    expect(log?.textContent ?? "").toContain(raw);
  });

  it.each(KINDS)("tin tro ly kind=%s co data-kind dung (AC-2)", (kind) => {
    const { container } = renderPanel({
      mode: "NORMAL",
      messages: [{ id: `a-${kind}`, role: "assistant", text: "Nội dung", kind }],
    });
    const [item] = messages(container);

    expect(item?.getAttribute("data-kind")).toBe(kind);
  });

  it("4 kind cho ra 4 nhan chu KHAC NHAU, khong trung, khong rong (AC-2, handoff §11.7)", () => {
    const sameText = "Nội dung giống nhau";
    const { container } = renderPanel({
      mode: "NORMAL",
      messages: KINDS.map((kind) => ({
        id: `a-${kind}`,
        role: "assistant" as const,
        text: sameText,
        kind,
      })),
    });
    const labels = messages(container).map((el) =>
      norm(allText(el).replace(sameText, " ")),
    );

    expect(labels).toHaveLength(4);
    for (const label of labels) {
      expect(label.length).toBeGreaterThan(0);
    }
    expect(new Set(labels).size).toBe(4);
  });

  it('tin tro ly KHONG co kind -> data-kind="ai" (AC-2)', () => {
    const { container } = renderPanel({
      mode: "NORMAL",
      messages: [ASSISTANT_MSG],
    });
    const [item] = messages(container);

    expect(item?.getAttribute("data-kind")).toBe("ai");
  });

  it("tin thieu kind hien cung nhan voi kind='ai' (khong co nhan thu 5) (AC-2)", () => {
    const text = "Nội dung";
    const explicit = renderPanel({
      mode: "NORMAL",
      messages: [{ id: "a1", role: "assistant", text, kind: "ai" }],
    });
    const explicitLabel = norm(allText(messages(explicit.container)[0] as HTMLElement));
    explicit.unmount();

    const implicit = renderPanel({
      mode: "NORMAL",
      messages: [{ id: "a1", role: "assistant", text }],
    });
    const implicitLabel = norm(allText(messages(implicit.container)[0] as HTMLElement));

    expect(implicitLabel).toBe(explicitLabel);
  });

  it("sentAt epoch ms hop le -> hien gio dang HH:mm (AC-2)", () => {
    const { container } = renderPanel({
      mode: "NORMAL",
      messages: [{ ...USER_MSG, sentAt: SENT_AT }],
    });

    expect(allText(container)).toMatch(HHMM);
  });

  it("sentAt ISO-8601 co mui gio -> hien gio dang HH:mm (AC-2)", () => {
    const { container } = renderPanel({
      mode: "NORMAL",
      messages: [{ ...USER_MSG, sentAt: new Date(SENT_AT).toISOString() }],
    });

    expect(allText(container)).toMatch(HHMM);
  });

  it("sentAt thieu -> KHONG hien gio (AC-2)", () => {
    const { container } = renderPanel({ mode: "NORMAL", messages: [USER_MSG] });

    expect(allText(container)).not.toMatch(HHMM);
  });

  it("sentAt khong parse duoc -> KHONG hien gio, khong throw, khong lo 'Invalid Date'/'NaN' (AC-2)", () => {
    // server-time canh bao chuoi khong co mui gio o dev; o day la co y, khong de rac stderr.
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    expect(() =>
      renderPanel({
        mode: "NORMAL",
        messages: [{ ...USER_MSG, sentAt: "khong-phai-ngay" }],
      }).unmount(),
    ).not.toThrow();

    const { container } = renderPanel({
      mode: "NORMAL",
      messages: [{ ...USER_MSG, sentAt: "khong-phai-ngay" }],
    });
    const text = allText(container);

    expect(text).not.toMatch(HHMM);
    expect(text).not.toContain("Invalid Date");
    expect(text).not.toContain("NaN");
    expect(warn).toHaveBeenCalled();
  });

  it("sentAt la so khong huu hien (NaN) -> KHONG hien gio, khong throw (AC-2)", () => {
    const { container } = renderPanel({
      mode: "NORMAL",
      messages: [{ ...USER_MSG, sentAt: Number.NaN }],
    });
    const text = allText(container);

    expect(text).not.toMatch(HHMM);
    expect(text).not.toContain("NaN");
  });

  it('log co aria-live="polite" va aria-label "Hội thoại với trợ lý" (AC-2)', () => {
    const { getByRole } = renderPanel({
      mode: "NORMAL",
      messages: [USER_MSG, ASSISTANT_MSG],
    });
    const log = getByRole("log");

    expect(log.getAttribute("aria-live")).toBe("polite");
    expect(log.getAttribute("aria-label")).toBe("Hội thoại với trợ lý");
  });

  it('KHONG co aria-live="assertive" o bat ky dau trong panel (AC-2)', () => {
    const { container } = renderPanel({
      mode: "NORMAL",
      messages: [USER_MSG, ASSISTANT_MSG, PENDING_MSG],
      faqs: FAQS,
      quotaRemaining: 3,
      quotaLimit: 10,
    });

    expect(container.querySelector('[aria-live="assertive"]')).toBeNull();
  });
});

/* -------------------------------------------------------------------------- */
/* AC-3                                                                        */
/* -------------------------------------------------------------------------- */

describe("AssistantPanel — gui cau hoi mo (AC-3, BR-A1)", () => {
  it("go co khoang trang hai dau roi bam Gui -> onSend dung 1 lan voi text da trim (AC-3)", () => {
    const onSend = vi.fn();
    const { container } = renderPanel({ mode: "NORMAL", onSend });

    typeAndSubmit(container, "  Mở bán lúc mấy giờ?  ");

    expect(onSend).toHaveBeenCalledTimes(1);
    expect(onSend).toHaveBeenCalledWith("Mở bán lúc mấy giờ?");
  });

  it("sau khi gui, o nhap duoc xoa trong (AC-3)", () => {
    const onSend = vi.fn();
    const { container } = renderPanel({ mode: "NORMAL", onSend });

    typeAndSubmit(container, "  Mở bán lúc mấy giờ?  ");

    expect(textarea(container)?.value).toBe("");
  });

  it("nut Gui la <button type=submit> co ten 'Gửi' (AC-3)", () => {
    const { getByRole } = renderPanel({ mode: "NORMAL", onSend: vi.fn() });
    const button = getByRole("button", { name: "Gửi" });

    expect(button.tagName).toBe("BUTTON");
    expect(button.getAttribute("type")).toBe("submit");
  });

  it("Enter (khong Shift) trong textarea gui nhu bam nut (AC-3)", () => {
    const onSend = vi.fn();
    const { container } = renderPanel({ mode: "NORMAL", onSend });

    typeAndEnter(container, "Mở bán lúc mấy giờ?");

    expect(onSend).toHaveBeenCalledTimes(1);
    expect(onSend).toHaveBeenCalledWith("Mở bán lúc mấy giờ?");
  });

  it("Enter cung xoa trong o nhap sau khi gui (AC-3)", () => {
    const { container } = renderPanel({ mode: "NORMAL", onSend: vi.fn() });

    typeAndEnter(container, "Mở bán lúc mấy giờ?");

    expect(textarea(container)?.value).toBe("");
  });

  it("Shift+Enter KHONG gui (xuong dong) (AC-3)", () => {
    const onSend = vi.fn();
    const { container } = renderPanel({ mode: "NORMAL", onSend });

    typeAndEnter(container, "Mở bán lúc mấy giờ?", true);

    expect(onSend).not.toHaveBeenCalled();
  });

  it("Shift+Enter giu nguyen noi dung da go (AC-3)", () => {
    const { container } = renderPanel({ mode: "NORMAL", onSend: vi.fn() });

    typeAndEnter(container, "Mở bán lúc mấy giờ?", true);

    expect(textarea(container)?.value).toBe("Mở bán lúc mấy giờ?");
  });

  it("o nhap rong -> bam Gui KHONG goi onSend (AC-3)", () => {
    const onSend = vi.fn();
    const { container } = renderPanel({ mode: "NORMAL", onSend });

    typeAndSubmit(container, "");

    expect(onSend).not.toHaveBeenCalled();
  });

  it("o nhap chi khoang trang -> bam Gui KHONG goi onSend (AC-3)", () => {
    const onSend = vi.fn();
    const { container } = renderPanel({ mode: "NORMAL", onSend });

    typeAndSubmit(container, "   \n\t  ");

    expect(onSend).not.toHaveBeenCalled();
  });

  it("o nhap chi khoang trang -> Enter KHONG goi onSend (AC-3)", () => {
    const onSend = vi.fn();
    const { container } = renderPanel({ mode: "NORMAL", onSend });

    typeAndEnter(container, "   ");

    expect(onSend).not.toHaveBeenCalled();
  });

  it("khong truyen onSend -> textarea va nut Gui bi disabled, khong throw (AC-3)", () => {
    expect(() => renderPanel({ mode: "NORMAL" }).unmount()).not.toThrow();

    const { container } = renderPanel({ mode: "NORMAL" });

    expect(textarea(container)).not.toBeNull();
    expect(isDisabled(textarea(container))).toBe(true);
    expect(submitButton(container)).not.toBeNull();
    expect(isDisabled(submitButton(container))).toBe(true);
  });

  it("khong truyen onSend -> bam Gui / Enter khong throw (AC-3)", () => {
    const { container } = renderPanel({ mode: "NORMAL" });

    expect(() => {
      fireEvent.click(submitButton(container) as HTMLButtonElement);
      fireEvent.keyDown(textarea(container) as HTMLTextAreaElement, { key: "Enter" });
    }).not.toThrow();
  });

  it("textarea co <label> nhin thay 'Câu hỏi của bạn' gan dung (AC-3, handoff §6)", () => {
    const { getByLabelText, container } = renderPanel({ mode: "NORMAL", onSend: vi.fn() });
    const input = getByLabelText("Câu hỏi của bạn");

    expect(input.tagName).toBe("TEXTAREA");
    const label = Array.from(container.querySelectorAll("label")).find((el) =>
      norm(el.textContent).includes("Câu hỏi của bạn"),
    );
    expect(label).toBeDefined();
  });

  it("KHONG dung placeholder lam label (AC-3, handoff §6)", () => {
    const { container } = renderPanel({ mode: "NORMAL", onSend: vi.fn() });
    const input = textarea(container);

    // Placeholder co the co nhung khong duoc la nguon ten duy nhat: phai co <label> that.
    const hasRealLabel =
      (input?.id && container.querySelector(`label[for="${input.id}"]`) !== null) ||
      input?.closest("label") !== null ||
      (input?.getAttribute("aria-labelledby") !== null &&
        document.getElementById(input?.getAttribute("aria-labelledby") ?? "") !== null);

    expect(hasRealLabel).toBe(true);
  });
});

/* -------------------------------------------------------------------------- */
/* AC-4                                                                        */
/* -------------------------------------------------------------------------- */

describe("AssistantPanel — mot job mot luc (AC-4, BR-A1)", () => {
  function renderWithPending() {
    const onSend = vi.fn();
    const onSelectFaq = vi.fn();
    const view = renderPanel({
      mode: "NORMAL",
      messages: [USER_MSG, ASSISTANT_MSG, PENDING_MSG],
      faqs: FAQS,
      onSend,
      onSelectFaq,
    });
    return { ...view, onSend, onSelectFaq };
  }

  it("co tin pending -> textarea disabled (AC-4)", () => {
    const { container } = renderWithPending();

    expect(isDisabled(textarea(container))).toBe(true);
  });

  it("co tin pending -> nut Gui disabled (AC-4)", () => {
    const { container } = renderWithPending();

    expect(isDisabled(submitButton(container))).toBe(true);
  });

  it("co tin pending -> MOI chip FAQ disabled (AC-4)", () => {
    const { container } = renderWithPending();
    const chips = faqChips(container);

    expect(chips).toHaveLength(FAQS.length);
    for (const chip of chips) {
      expect(isDisabled(chip)).toBe(true);
    }
  });

  it("co tin pending -> bam Gui / Enter KHONG goi onSend (AC-4)", () => {
    const { container, onSend } = renderWithPending();

    typeAndSubmit(container, "Câu hỏi mới");
    typeAndEnter(container, "Câu hỏi mới");

    expect(onSend).not.toHaveBeenCalled();
  });

  it("co tin pending -> bam chip KHONG goi onSelectFaq (AC-4)", () => {
    const { container, onSelectFaq } = renderWithPending();

    for (const chip of faqChips(container)) {
      fireEvent.click(chip);
    }

    expect(onSelectFaq).not.toHaveBeenCalled();
  });

  it("pending la tin o GIUA mang cung chan gui (bat ky tin pending nao) (AC-4)", () => {
    const onSend = vi.fn();
    const { container } = renderPanel({
      mode: "NORMAL",
      messages: [PENDING_MSG, ASSISTANT_MSG, USER_MSG],
      onSend,
    });

    expect(isDisabled(textarea(container))).toBe(true);
    typeAndSubmit(container, "Câu hỏi mới");
    expect(onSend).not.toHaveBeenCalled();
  });

  it('tin pending co data-status="pending" va text "đang xử lý" (AC-4)', () => {
    const { container } = renderWithPending();
    const pending = messages(container).find(
      (el) => el.getAttribute("data-status") === "pending",
    );

    expect(pending).toBeDefined();
    expect(norm(pending?.textContent)).toMatch(/đang xử lý/i);
  });

  it("KHONG co spinner gia streaming: khong co role=progressbar (AC-4, handoff §11.7)", () => {
    const { container } = renderWithPending();

    expect(container.querySelector('[role="progressbar"]')).toBeNull();
  });

  it("tin nguoi dung KHONG co status -> data-status la 'done' (mac dinh) (AC-4)", () => {
    const { container } = renderPanel({ mode: "NORMAL", messages: [USER_MSG] });
    const [item] = messages(container);

    expect(item?.getAttribute("data-status")).toBe("done");
  });

  it("tin done KHONG hien 'đang xử lý' (AC-4)", () => {
    const { container } = renderPanel({
      mode: "NORMAL",
      messages: [USER_MSG, ASSISTANT_MSG],
    });

    expect(allText(container)).not.toMatch(/đang xử lý/i);
  });

  it("het pending (rerender) -> gui lai duoc (AC-4)", () => {
    const onSend = vi.fn();
    const { container, rerender } = render(
      <AssistantPanel mode="NORMAL" messages={[PENDING_MSG]} onSend={onSend} />,
    );
    expect(isDisabled(textarea(container))).toBe(true);

    rerender(
      <AssistantPanel
        mode="NORMAL"
        messages={[{ ...PENDING_MSG, status: "done" }, ASSISTANT_MSG]}
        onSend={onSend}
      />,
    );

    expect(isDisabled(textarea(container))).toBe(false);
    expect(isDisabled(submitButton(container))).toBe(false);
    typeAndSubmit(container, "Câu hỏi tiếp");
    expect(onSend).toHaveBeenCalledTimes(1);
    expect(onSend).toHaveBeenCalledWith("Câu hỏi tiếp");
  });

  it("het pending (rerender) -> chip FAQ bam lai duoc (AC-4)", () => {
    const onSelectFaq = vi.fn();
    const { container, rerender } = render(
      <AssistantPanel
        mode="NORMAL"
        messages={[PENDING_MSG]}
        faqs={FAQS}
        onSelectFaq={onSelectFaq}
      />,
    );

    rerender(
      <AssistantPanel
        mode="NORMAL"
        messages={[{ ...PENDING_MSG, status: "done" }]}
        faqs={FAQS}
        onSelectFaq={onSelectFaq}
      />,
    );

    fireEvent.click(faqChips(container)[0] as HTMLButtonElement);
    expect(onSelectFaq).toHaveBeenCalledTimes(1);
    expect(onSelectFaq).toHaveBeenCalledWith(FAQS[0]?.id);
  });
});

/* -------------------------------------------------------------------------- */
/* AC-5                                                                        */
/* -------------------------------------------------------------------------- */

describe("AssistantPanel — tin that bai va gui lai (AC-5)", () => {
  it('tin failed co data-status="failed" (AC-5)', () => {
    const { container } = renderPanel({
      mode: "NORMAL",
      messages: [FAILED_MSG],
      onRetry: vi.fn(),
    });
    const [item] = messages(container);

    expect(item?.getAttribute("data-status")).toBe("failed");
  });

  it("co onRetry -> tin failed co nut 'Gửi lại' NAM TRONG tin do (AC-5)", () => {
    const { container } = renderPanel({
      mode: "NORMAL",
      messages: [USER_MSG, FAILED_MSG],
      onRetry: vi.fn(),
    });
    const failed = messages(container).find(
      (el) => el.getAttribute("data-status") === "failed",
    ) as HTMLElement;

    const retry = within(failed).getByRole("button", { name: "Gửi lại" });
    expect(retry.tagName).toBe("BUTTON");
  });

  it("bam 'Gửi lại' -> onRetry(message.id) dung 1 lan (AC-5)", () => {
    const onRetry = vi.fn();
    const { getByRole } = renderPanel({
      mode: "NORMAL",
      messages: [FAILED_MSG],
      onRetry,
    });

    fireEvent.click(getByRole("button", { name: "Gửi lại" }));

    expect(onRetry).toHaveBeenCalledTimes(1);
    expect(onRetry).toHaveBeenCalledWith(FAILED_MSG.id);
  });

  it("'Gửi lại' truyen DUNG id cua tin failed, khong phai tin khac (AC-5)", () => {
    const onRetry = vi.fn();
    const { getByRole } = renderPanel({
      mode: "NORMAL",
      messages: [USER_MSG, ASSISTANT_MSG, { ...FAILED_MSG, id: "u-second-failed" }],
      onRetry,
    });

    fireEvent.click(getByRole("button", { name: "Gửi lại" }));

    expect(onRetry).toHaveBeenCalledWith("u-second-failed");
  });

  it("tin done KHONG co nut 'Gửi lại' du co onRetry (AC-5)", () => {
    const { queryByRole } = renderPanel({
      mode: "NORMAL",
      messages: [USER_MSG, ASSISTANT_MSG],
      onRetry: vi.fn(),
    });

    expect(queryByRole("button", { name: "Gửi lại" })).toBeNull();
  });

  it("khong co onRetry -> khong co nut 'Gửi lại' (AC-5)", () => {
    const { queryByRole } = renderPanel({ mode: "NORMAL", messages: [FAILED_MSG] });

    expect(queryByRole("button", { name: "Gửi lại" })).toBeNull();
  });

  it("khong co onRetry -> van hien trang thai that bai bang CHU (khac tin done) (AC-5)", () => {
    const failed = renderPanel({ mode: "NORMAL", messages: [FAILED_MSG] });
    const failedText = norm(allText(messages(failed.container)[0] as HTMLElement));
    failed.unmount();

    const done = renderPanel({ mode: "NORMAL", messages: [{ ...FAILED_MSG, status: "done" }] });
    const doneText = norm(allText(messages(done.container)[0] as HTMLElement));

    expect(failedText).not.toBe(doneText);
    expect(failedText.replace(FAILED_MSG.text, "").trim().length).toBeGreaterThan(0);
  });

  it("copy trang thai failed KHONG chua tu ky thuat (co onRetry) (AC-5, §2.5)", () => {
    const { container } = renderPanel({
      mode: "NORMAL",
      messages: [FAILED_MSG],
      onRetry: vi.fn(),
    });

    expect(forbidden(allText(root(container)))).toBe(false);
  });

  it("copy trang thai failed KHONG chua tu ky thuat (khong co onRetry) (AC-5, §2.5)", () => {
    const { container } = renderPanel({ mode: "NORMAL", messages: [FAILED_MSG] });

    expect(forbidden(allText(root(container)))).toBe(false);
  });

  it("tin failed KHONG chan gui tin moi (khong phai pending) (AC-5, BR-A1)", () => {
    const onSend = vi.fn();
    const { container } = renderPanel({
      mode: "NORMAL",
      messages: [FAILED_MSG],
      onSend,
      onRetry: vi.fn(),
    });

    expect(isDisabled(textarea(container))).toBe(false);
    typeAndSubmit(container, "Hỏi lại");
    expect(onSend).toHaveBeenCalledTimes(1);
  });
});

/* -------------------------------------------------------------------------- */
/* AC-6                                                                        */
/* -------------------------------------------------------------------------- */

describe("AssistantPanel — FAQ chips (AC-6)", () => {
  it("3 faqs -> nhom role=group ten 'Câu hỏi thường gặp' co dung 3 button (AC-6)", () => {
    const { getByRole } = renderPanel({ mode: "NORMAL", faqs: FAQS, onSelectFaq: vi.fn() });
    const group = getByRole("group", { name: "Câu hỏi thường gặp" });
    const chips = within(group).getAllByRole("button");

    expect(chips).toHaveLength(3);
  });

  it("moi chip la <button type=button> voi label tuong ung, dung thu tu (AC-6)", () => {
    const { container } = renderPanel({ mode: "NORMAL", faqs: FAQS, onSelectFaq: vi.fn() });
    const chips = faqChips(container);

    expect(chips.map((chip) => accName(chip))).toEqual(FAQS.map((faq) => faq.label));
    for (const chip of chips) {
      expect(chip.tagName).toBe("BUTTON");
      expect(chip.getAttribute("type")).toBe("button");
    }
  });

  it("bam chip thu 2 -> onSelectFaq(faqs[1].id) dung 1 lan (AC-6)", () => {
    const onSelectFaq = vi.fn();
    const { container } = renderPanel({ mode: "NORMAL", faqs: FAQS, onSelectFaq });

    fireEvent.click(faqChips(container)[1] as HTMLButtonElement);

    expect(onSelectFaq).toHaveBeenCalledTimes(1);
    expect(onSelectFaq).toHaveBeenCalledWith(FAQS[1]?.id);
  });

  it("bam chip KHONG goi onSend (chip la FAQ, khong phai cau hoi mo) (AC-6)", () => {
    const onSend = vi.fn();
    const { container } = renderPanel({
      mode: "NORMAL",
      faqs: FAQS,
      onSend,
      onSelectFaq: vi.fn(),
    });

    fireEvent.click(faqChips(container)[0] as HTMLButtonElement);

    expect(onSend).not.toHaveBeenCalled();
  });

  it("faqs rong -> KHONG render nhom (khong co group rong) (AC-6)", () => {
    const { container } = renderPanel({ mode: "NORMAL", faqs: [], onSelectFaq: vi.fn() });

    expect(faqGroup(container)).toBeNull();
  });

  it("faqs thieu -> KHONG render nhom (AC-6)", () => {
    const { container } = renderPanel({ mode: "NORMAL", onSelectFaq: vi.fn() });

    expect(faqGroup(container)).toBeNull();
  });

  it.each(["FAQ_ONLY", "SAVING"] as AssistantMode[])(
    "mode %s -> chip VAN bam duoc va goi onSelectFaq (AC-6, BR-A6)",
    (mode) => {
      const onSelectFaq = vi.fn();
      const { container } = renderPanel({ mode, faqs: FAQS, onSelectFaq });
      const chips = faqChips(container);

      expect(chips).toHaveLength(FAQS.length);
      for (const chip of chips) {
        expect(isDisabled(chip)).toBe(false);
      }

      fireEvent.click(chips[2] as HTMLButtonElement);
      expect(onSelectFaq).toHaveBeenCalledTimes(1);
      expect(onSelectFaq).toHaveBeenCalledWith(FAQS[2]?.id);
    },
  );

  it("quotaRemaining=0 -> chip VAN bam duoc (AC-6, BR-A2)", () => {
    const onSelectFaq = vi.fn();
    const { container } = renderPanel({
      mode: "NORMAL",
      faqs: FAQS,
      quotaRemaining: 0,
      quotaLimit: 10,
      onSelectFaq,
      onSend: vi.fn(),
    });
    const chips = faqChips(container);

    for (const chip of chips) {
      expect(isDisabled(chip)).toBe(false);
    }
    fireEvent.click(chips[0] as HTMLButtonElement);
    expect(onSelectFaq).toHaveBeenCalledTimes(1);
    expect(onSelectFaq).toHaveBeenCalledWith(FAQS[0]?.id);
  });

  it("khong truyen onSelectFaq -> bam chip khong throw (AC-6)", () => {
    const { container } = renderPanel({ mode: "NORMAL", faqs: FAQS });

    expect(() => {
      for (const chip of faqChips(container)) fireEvent.click(chip);
    }).not.toThrow();
  });
});

/* -------------------------------------------------------------------------- */
/* AC-7                                                                        */
/* -------------------------------------------------------------------------- */

describe("AssistantPanel — han muc (AC-7, BR-A2)", () => {
  it('quotaRemaining=3, quotaLimit=10 -> data-slot="quota" hien "3/10" kem chu "lượt" (AC-7)', () => {
    const { container } = renderPanel({
      mode: "NORMAL",
      quotaRemaining: 3,
      quotaLimit: 10,
      onSend: vi.fn(),
    });
    const quota = container.querySelector<HTMLElement>('[data-slot="quota"]');

    expect(quota).not.toBeNull();
    expect(norm(quota?.textContent)).toMatch(/3\s*\/\s*10/);
    expect(norm(quota?.textContent)).toMatch(/lượt/i);
  });

  it("quotaRemaining=0, quotaLimit=10 -> van hien so '0/10' (0 la so hop le) (AC-7)", () => {
    const { container } = renderPanel({
      mode: "NORMAL",
      quotaRemaining: 0,
      quotaLimit: 10,
      onSend: vi.fn(),
    });
    const quota = container.querySelector<HTMLElement>('[data-slot="quota"]');

    expect(quota).not.toBeNull();
    expect(norm(quota?.textContent)).toMatch(/0\s*\/\s*10/);
  });

  it.each<[string, Partial<AssistantPanelProps>]>([
    ["chi co quotaRemaining", { quotaRemaining: 3 }],
    ["chi co quotaLimit", { quotaLimit: 10 }],
    ["quotaRemaining = -1", { quotaRemaining: -1, quotaLimit: 10 }],
    ["quotaLimit = -1", { quotaRemaining: 3, quotaLimit: -1 }],
    ["quotaRemaining = 2.5", { quotaRemaining: 2.5, quotaLimit: 10 }],
    ["quotaLimit = 2.5", { quotaRemaining: 3, quotaLimit: 2.5 }],
    ["quotaRemaining = NaN", { quotaRemaining: Number.NaN, quotaLimit: 10 }],
    ["quotaLimit = NaN", { quotaRemaining: 3, quotaLimit: Number.NaN }],
    ["quotaLimit = Infinity", { quotaRemaining: 3, quotaLimit: Number.POSITIVE_INFINITY }],
  ])("%s -> KHONG render quota, khong lo NaN (AC-7)", (_label, props) => {
    const { container } = renderPanel({ mode: "NORMAL", onSend: vi.fn(), ...props });

    expect(container.querySelector('[data-slot="quota"]')).toBeNull();
    expect(allText(container)).not.toContain("NaN");
    expect(allText(container)).not.toContain("Infinity");
  });

  it("khong truyen quota -> khong render quota (AC-7)", () => {
    const { container } = renderPanel({ mode: "NORMAL", onSend: vi.fn() });

    expect(container.querySelector('[data-slot="quota"]')).toBeNull();
  });

  it("quotaRemaining=0 + NORMAL -> textarea va Gui disabled (AC-7)", () => {
    const { container } = renderPanel({
      mode: "NORMAL",
      quotaRemaining: 0,
      quotaLimit: 10,
      onSend: vi.fn(),
    });

    expect(isDisabled(textarea(container))).toBe(true);
    expect(isDisabled(submitButton(container))).toBe(true);
  });

  it("quotaRemaining=0 + NORMAL -> bam Gui / Enter KHONG goi onSend (AC-7)", () => {
    const onSend = vi.fn();
    const { container } = renderPanel({
      mode: "NORMAL",
      quotaRemaining: 0,
      quotaLimit: 10,
      onSend,
    });

    typeAndSubmit(container, "Còn hỏi được không?");
    typeAndEnter(container, "Còn hỏi được không?");

    expect(onSend).not.toHaveBeenCalled();
  });

  it("quotaRemaining=0 -> co thong bao het luot va nhac kenh CSKH (AC-7)", () => {
    const { container } = renderPanel({
      mode: "NORMAL",
      quotaRemaining: 0,
      quotaLimit: 10,
      onSend: vi.fn(),
    });
    const text = allText(root(container));

    expect(text).toMatch(/hết lượt|đã dùng hết/i);
    expect(text).toMatch(/hỗ trợ|CSKH|chăm sóc/i);
  });

  it("quotaRemaining=0 -> thong bao het luot dung role=status (AC-7)", () => {
    const { container } = renderPanel({
      mode: "NORMAL",
      quotaRemaining: 0,
      quotaLimit: 10,
      onSend: vi.fn(),
    });
    const matching = statusNodes(container).filter((el) =>
      /hết lượt|đã dùng hết/i.test(norm(el.textContent)),
    );

    expect(matching.length).toBeGreaterThan(0);
  });

  it("quotaRemaining=0 + supportHref -> co <a href=supportHref> (AC-7)", () => {
    const { container } = renderPanel({
      mode: "NORMAL",
      quotaRemaining: 0,
      quotaLimit: 10,
      supportHref: SUPPORT_HREF,
      onSend: vi.fn(),
    });
    const link = container.querySelector(`a[href="${SUPPORT_HREF}"]`);

    expect(link).not.toBeNull();
    expect(accName(link as Element).length).toBeGreaterThan(0);
  });

  it("quotaRemaining=0 KHONG co supportHref -> khong co <a> nao (AC-7, handoff §5)", () => {
    const { container } = renderPanel({
      mode: "NORMAL",
      quotaRemaining: 0,
      quotaLimit: 10,
      onSend: vi.fn(),
    });

    expect(container.querySelectorAll("a")).toHaveLength(0);
  });

  it("quotaRemaining=0 -> thong bao het luot KHONG chua tu ky thuat (AC-7, §2.5)", () => {
    const { container } = renderPanel({
      mode: "NORMAL",
      quotaRemaining: 0,
      quotaLimit: 10,
      faqs: FAQS,
      supportHref: SUPPORT_HREF,
      onSend: vi.fn(),
    });

    expect(forbidden(allText(root(container)))).toBe(false);
  });

  it("quotaRemaining > 0 -> KHONG co thong bao het luot, gui duoc (AC-7)", () => {
    const onSend = vi.fn();
    const { container } = renderPanel({
      mode: "NORMAL",
      quotaRemaining: 3,
      quotaLimit: 10,
      onSend,
    });

    expect(allText(container)).not.toMatch(/hết lượt|đã dùng hết/i);
    typeAndSubmit(container, "Mở bán lúc mấy giờ?");
    expect(onSend).toHaveBeenCalledTimes(1);
  });

  it("quotaRemaining=0 nhung quotaLimit thieu -> van chan gui (0 la so thuc) nhung khong hien so (AC-7, BR-A2)", () => {
    const onSend = vi.fn();
    const { container } = renderPanel({ mode: "NORMAL", quotaRemaining: 0, onSend });

    expect(container.querySelector('[data-slot="quota"]')).toBeNull();
    expect(isDisabled(textarea(container))).toBe(true);
    typeAndSubmit(container, "Còn hỏi được không?");
    expect(onSend).not.toHaveBeenCalled();
  });
});

/* -------------------------------------------------------------------------- */
/* AC-8                                                                        */
/* -------------------------------------------------------------------------- */

describe("AssistantPanel — 4 nac suy bien (AC-8, BR-A6)", () => {
  const DEGRADED = /câu hỏi thường gặp|không khả dụng|tạm ngưng|tiết kiệm/i;

  it.each(MODES)("mode %s: goc co data-mode dung bang mode (AC-8)", (mode) => {
    const { container } = renderPanel({ mode });

    expect(root(container).getAttribute("data-mode")).toBe(mode);
  });

  it("NORMAL: KHONG co thong bao suy bien nao (AC-8)", () => {
    const { container } = renderPanel({ mode: "NORMAL", faqs: FAQS, onSend: vi.fn() });
    const degraded = statusNodes(container).filter((el) =>
      /câu hỏi thường gặp|không khả dụng|tạm ngưng/i.test(norm(el.textContent)),
    );

    expect(degraded).toHaveLength(0);
  });

  it("NORMAL: gui duoc (AC-8)", () => {
    const onSend = vi.fn();
    const { container } = renderPanel({ mode: "NORMAL", onSend });

    expect(isDisabled(textarea(container))).toBe(false);
    expect(isDisabled(submitButton(container))).toBe(false);
    typeAndSubmit(container, "Mở bán lúc mấy giờ?");
    expect(onSend).toHaveBeenCalledTimes(1);
  });

  it.each(["SAVING", "FAQ_ONLY"] as AssistantMode[])(
    "mode %s: textarea va Gui disabled, bam khong goi onSend (AC-8)",
    (mode) => {
      const onSend = vi.fn();
      const { container } = renderPanel({ mode, onSend });

      expect(isDisabled(textarea(container))).toBe(true);
      expect(isDisabled(submitButton(container))).toBe(true);

      typeAndSubmit(container, "Mở bán lúc mấy giờ?");
      typeAndEnter(container, "Mở bán lúc mấy giờ?");
      expect(onSend).not.toHaveBeenCalled();
    },
  );

  it.each(["SAVING", "FAQ_ONLY"] as AssistantMode[])(
    "mode %s: co thong bao role=status nhac 'câu hỏi thường gặp' (AC-8)",
    (mode) => {
      const { container } = renderPanel({ mode, faqs: FAQS });
      const matching = statusNodes(container).filter((el) =>
        /câu hỏi thường gặp/i.test(norm(el.textContent)),
      );

      expect(matching.length).toBeGreaterThan(0);
    },
  );

  it("SAVING: thong bao nhac 'tiết kiệm' (docs/02 §3) (AC-8)", () => {
    const { container } = renderPanel({ mode: "SAVING", faqs: FAQS });
    const text = statusNodes(container)
      .map((el) => norm(el.textContent))
      .join(" ");

    expect(text).toMatch(/tiết kiệm/i);
  });

  it.each(["SAVING", "FAQ_ONLY"] as AssistantMode[])(
    "mode %s: chip FAQ van bam duoc (AC-8, BR-A6)",
    (mode) => {
      const onSelectFaq = vi.fn();
      const { container } = renderPanel({ mode, faqs: FAQS, onSelectFaq });

      fireEvent.click(faqChips(container)[0] as HTMLButtonElement);
      expect(onSelectFaq).toHaveBeenCalledTimes(1);
    },
  );

  it.each(["SAVING", "FAQ_ONLY"] as AssistantMode[])(
    "mode %s: van con role=log va authority-notice (chi chan cau hoi mo) (AC-8)",
    (mode) => {
      const { container } = renderPanel({ mode, messages: [USER_MSG, ASSISTANT_MSG] });

      expect(container.querySelector('[role="log"]')).not.toBeNull();
      expect(container.querySelector('[data-slot="authority-notice"]')).not.toBeNull();
      expect(messages(container)).toHaveLength(2);
    },
  );

  it.each(["SAVING", "FAQ_ONLY", "OFF"] as AssistantMode[])(
    "mode %s: copy KHONG chua tu ky thuat (AC-8, §2.5 — AC goc Jira)",
    (mode) => {
      const { container } = renderPanel({
        mode,
        faqs: FAQS,
        messages: [USER_MSG, ASSISTANT_MSG],
        supportHref: SUPPORT_HREF,
        onSend: vi.fn(),
        onSelectFaq: vi.fn(),
      });

      expect(forbidden(allText(root(container)))).toBe(false);
    },
  );

  it.each(["SAVING", "FAQ_ONLY", "OFF"] as AssistantMode[])(
    "mode %s: copy KHONG chua tu ky thuat ngay ca khi KHONG co supportHref (AC-8, §2.5)",
    (mode) => {
      const { container } = renderPanel({ mode, faqs: FAQS });

      expect(forbidden(allText(root(container)))).toBe(false);
    },
  );

  it.each(["SAVING", "FAQ_ONLY", "OFF"] as AssistantMode[])(
    "mode %s: thong bao suy bien dung role=status, KHONG dung role=alert (AC-8)",
    (mode) => {
      const { container } = renderPanel({ mode, faqs: FAQS });

      expect(container.querySelector('[role="alert"]')).toBeNull();
      expect(statusNodes(container).length).toBeGreaterThan(0);
    },
  );

  it("OFF: KHONG co role=log (AC-8)", () => {
    const { container } = renderPanel({
      mode: "OFF",
      messages: [USER_MSG, ASSISTANT_MSG],
      faqs: FAQS,
    });

    expect(container.querySelector('[role="log"]')).toBeNull();
    expect(messages(container)).toHaveLength(0);
  });

  it("OFF: KHONG co textarea va nut Gui (AC-8)", () => {
    const { container } = renderPanel({ mode: "OFF", onSend: vi.fn() });

    expect(textarea(container)).toBeNull();
    expect(submitButton(container)).toBeNull();
  });

  it("OFF: KHONG co nhom FAQ du co faqs (AC-8)", () => {
    const { container } = renderPanel({ mode: "OFF", faqs: FAQS, onSelectFaq: vi.fn() });

    expect(faqGroup(container)).toBeNull();
    expect(faqChips(container)).toHaveLength(0);
  });

  it("OFF: co DUNG MOT thong bao 'tạm không khả dụng' (AC-8, docs/02 §3)", () => {
    const { container } = renderPanel({ mode: "OFF", faqs: FAQS });
    const matching = statusNodes(container).filter((el) =>
      /tạm.*không.*khả dụng|tạm ngưng/i.test(norm(el.textContent)),
    );

    expect(matching).toHaveLength(1);
  });

  it("OFF: thong bao nhac kenh CSKH (AC-8)", () => {
    const { container } = renderPanel({ mode: "OFF" });

    expect(allText(root(container))).toMatch(/hỗ trợ|CSKH|chăm sóc/i);
  });

  it("OFF + supportHref -> co <a href=supportHref> (AC-8)", () => {
    const { container } = renderPanel({ mode: "OFF", supportHref: SUPPORT_HREF });

    expect(container.querySelector(`a[href="${SUPPORT_HREF}"]`)).not.toBeNull();
  });

  it("OFF khong co supportHref -> khong co <a> nao (AC-8, handoff §5)", () => {
    const { container } = renderPanel({ mode: "OFF" });

    expect(container.querySelectorAll("a")).toHaveLength(0);
  });

  it('OFF: goc van co data-mode="OFF" va data-authoritative="false" (AC-8)', () => {
    const { container } = renderPanel({ mode: "OFF" });
    const el = root(container);

    expect(el.getAttribute("data-mode")).toBe("OFF");
    expect(el.getAttribute("data-authoritative")).toBe("false");
  });

  it("4 mode cho ra noi dung KHAC NHAU (khong truyen tin chi bang mau) (AC-8)", () => {
    const texts = MODES.map((mode) => {
      const { container, unmount } = renderPanel({ mode, faqs: FAQS });
      const text = allText(container);
      unmount();
      return text;
    });

    expect(new Set(texts).size).toBe(MODES.length);
  });

  it("doi mode luc dang chay (rerender NORMAL -> SAVING -> NORMAL) phan anh dung (AC-8)", () => {
    const onSend = vi.fn();
    const { container, rerender } = render(
      <AssistantPanel mode="NORMAL" messages={[]} onSend={onSend} />,
    );
    expect(isDisabled(textarea(container))).toBe(false);

    rerender(<AssistantPanel mode="SAVING" messages={[]} onSend={onSend} />);
    expect(root(container).getAttribute("data-mode")).toBe("SAVING");
    expect(isDisabled(textarea(container))).toBe(true);
    expect(allText(container)).toMatch(DEGRADED);

    rerender(<AssistantPanel mode="NORMAL" messages={[]} onSend={onSend} />);
    expect(root(container).getAttribute("data-mode")).toBe("NORMAL");
    expect(isDisabled(textarea(container))).toBe(false);
  });
});

/* -------------------------------------------------------------------------- */
/* AC-9                                                                        */
/* -------------------------------------------------------------------------- */

describe("AssistantPanel — dong panel va ban phim (AC-9)", () => {
  it("co onClose -> co nut 'Đóng trợ lý' la <button> (AC-9)", () => {
    const { getByRole } = renderPanel({ mode: "NORMAL", onClose: vi.fn() });
    const button = getByRole("button", { name: "Đóng trợ lý" });

    expect(button.tagName).toBe("BUTTON");
  });

  it("bam 'Đóng trợ lý' -> onClose dung 1 lan (AC-9)", () => {
    const onClose = vi.fn();
    const { getByRole } = renderPanel({ mode: "NORMAL", onClose });

    fireEvent.click(getByRole("button", { name: "Đóng trợ lý" }));

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("Escape khi focus trong textarea -> onClose dung 1 lan (AC-9)", () => {
    const onClose = vi.fn();
    const { container } = renderPanel({ mode: "NORMAL", onClose, onSend: vi.fn() });

    fireEvent.keyDown(textarea(container) as HTMLTextAreaElement, {
      key: "Escape",
      code: "Escape",
    });

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("Escape khi focus o chip FAQ (trong panel) -> onClose 1 lan (AC-9)", () => {
    const onClose = vi.fn();
    const { container } = renderPanel({ mode: "NORMAL", onClose, faqs: FAQS });

    fireEvent.keyDown(faqChips(container)[0] as HTMLButtonElement, { key: "Escape" });

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("Escape KHONG goi onSend va khong lam mat noi dung dang go (AC-9)", () => {
    const onSend = vi.fn();
    const { container } = renderPanel({ mode: "NORMAL", onClose: vi.fn(), onSend });
    const input = textarea(container) as HTMLTextAreaElement;

    fireEvent.change(input, { target: { value: "Đang gõ" } });
    fireEvent.keyDown(input, { key: "Escape" });

    expect(onSend).not.toHaveBeenCalled();
    expect(input.value).toBe("Đang gõ");
  });

  it("phim khac (vi du 'a') KHONG goi onClose (AC-9)", () => {
    const onClose = vi.fn();
    const { container } = renderPanel({ mode: "NORMAL", onClose, onSend: vi.fn() });

    fireEvent.keyDown(textarea(container) as HTMLTextAreaElement, { key: "a" });
    fireEvent.keyDown(textarea(container) as HTMLTextAreaElement, { key: "Enter" });

    expect(onClose).not.toHaveBeenCalled();
  });

  it("OFF + onClose -> van co nut dong (khach phai thoat duoc) (AC-9)", () => {
    const onClose = vi.fn();
    const { getByRole } = renderPanel({ mode: "OFF", onClose });

    fireEvent.click(getByRole("button", { name: "Đóng trợ lý" }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("khong co onClose -> KHONG co nut dong (AC-9)", () => {
    const { queryByRole } = renderPanel({ mode: "NORMAL", onSend: vi.fn() });

    expect(queryByRole("button", { name: "Đóng trợ lý" })).toBeNull();
  });

  it("khong co onClose -> Escape khong throw (AC-9)", () => {
    const { container } = renderPanel({ mode: "NORMAL", onSend: vi.fn() });

    expect(() => {
      fireEvent.keyDown(textarea(container) as HTMLTextAreaElement, { key: "Escape" });
      fireEvent.keyDown(root(container), { key: "Escape" });
    }).not.toThrow();
  });

  it("Escape handler khong gan o document: Escape ngoai panel KHONG goi onClose (AC-9, §5 rui ro b)", () => {
    const onClose = vi.fn();
    renderPanel({ mode: "NORMAL", onClose });

    fireEvent.keyDown(document.body, { key: "Escape" });

    expect(onClose).not.toHaveBeenCalled();
  });

  it("moi control tuong tac la phan tu native button/textarea/a (AC-9)", () => {
    const { container } = renderPanel({
      mode: "NORMAL",
      messages: [FAILED_MSG],
      faqs: FAQS,
      quotaRemaining: 0,
      quotaLimit: 10,
      supportHref: SUPPORT_HREF,
      onSend: vi.fn(),
      onSelectFaq: vi.fn(),
      onRetry: vi.fn(),
      onClose: vi.fn(),
    });

    // Khong co div/span gia nut. (React khong gan thuoc tinh `onclick` vao DOM nen khong
    // the bat handler bang selector; role gia la dau hieu con kiem duoc.)
    const fakeControls = Array.from(
      container.querySelectorAll('[role="button"], [role="link"], [role="textbox"]'),
    ).filter((el) => !["BUTTON", "TEXTAREA", "A"].includes(el.tagName));
    expect(fakeControls).toHaveLength(0);

    const focusables = Array.from(container.querySelectorAll("[tabindex]")).filter(
      (el) => el.getAttribute("tabindex") !== "-1",
    );
    for (const el of focusables) {
      expect(["BUTTON", "TEXTAREA", "A"]).toContain(el.tagName);
    }
  });

  it("moi button co ten truy cap duoc (khong co nut chi la icon) (AC-9)", () => {
    const { getAllByRole } = renderPanel({
      mode: "NORMAL",
      messages: [FAILED_MSG],
      faqs: FAQS,
      onSend: vi.fn(),
      onSelectFaq: vi.fn(),
      onRetry: vi.fn(),
      onClose: vi.fn(),
    });

    const buttons = getAllByRole("button");
    expect(buttons.length).toBeGreaterThanOrEqual(FAQS.length + 3); // chips + Gui + Gui lai + Dong
    for (const button of buttons) {
      expect(accName(button).length).toBeGreaterThan(0);
    }
  });

  it("4 nut co ten co dinh deu lay duoc qua getByRole (AC-9)", () => {
    const { getByRole } = renderPanel({
      mode: "NORMAL",
      messages: [FAILED_MSG],
      onSend: vi.fn(),
      onRetry: vi.fn(),
      onClose: vi.fn(),
    });

    expect(getByRole("button", { name: "Gửi" })).toBeInTheDocument();
    expect(getByRole("button", { name: "Gửi lại" })).toBeInTheDocument();
    expect(getByRole("button", { name: "Đóng trợ lý" })).toBeInTheDocument();
    expect(getByRole("textbox", { name: "Câu hỏi của bạn" })).toBeInTheDocument();
  });

  it("className duoc noi vao goc (AC-9, hop dong §3)", () => {
    const { container } = renderPanel({ mode: "NORMAL", className: "test-marker-class" });

    expect(root(container).classList.contains("test-marker-class")).toBe(true);
  });
});

/* -------------------------------------------------------------------------- */
/* Vong review 1 (code-review F-1..F-6, challenge C-1..C-11)                   */
/* -------------------------------------------------------------------------- */

describe("AssistantPanel — Gui lai la cau hoi mo, chiu chung moi chan (AC-5b, BR-A1/A2/A6)", () => {
  function retryButton(container: HTMLElement): HTMLButtonElement | null {
    return (
      Array.from(container.querySelectorAll<HTMLButtonElement>("button")).find(
        (el) => norm(el.textContent) === "Gửi lại",
      ) ?? null
    );
  }

  it.each<[string, Partial<AssistantPanelProps> & { mode: AssistantMode }]>([
    ["dang co tin pending", { mode: "NORMAL", messages: [FAILED_MSG, { ...PENDING_MSG, id: "u2" }] }],
    ["SAVING", { mode: "SAVING", messages: [FAILED_MSG] }],
    ["FAQ_ONLY", { mode: "FAQ_ONLY", messages: [FAILED_MSG] }],
    ["quotaRemaining = 0", { mode: "NORMAL", messages: [FAILED_MSG], quotaRemaining: 0, quotaLimit: 10 }],
  ])("%s -> nut Gui lai disabled, bam KHONG goi onRetry (AC-5b)", (_label, props) => {
    const onRetry = vi.fn();
    const { container } = renderPanel({ onRetry, ...props });
    const button = retryButton(container);

    expect(button).not.toBeNull();
    expect(isDisabled(button)).toBe(true);
    fireEvent.click(button as HTMLButtonElement);
    expect(onRetry).not.toHaveBeenCalled();
  });

  it("NORMAL, khong pending, con luot, KHONG co onSend -> Gui lai van bam duoc (AC-5b)", () => {
    const onRetry = vi.fn();
    const { container } = renderPanel({ mode: "NORMAL", messages: [FAILED_MSG], onRetry });

    fireEvent.click(retryButton(container) as HTMLButtonElement);
    expect(onRetry).toHaveBeenCalledWith(FAILED_MSG.id);
  });
});

describe("AssistantPanel — IME dang ghep chu (Telex/VNI) (AC-3b, AC-9b)", () => {
  it("Enter khi isComposing / keyCode 229 -> KHONG gui, giu nguyen noi dung (AC-3b)", () => {
    const onSend = vi.fn();
    const { container } = renderPanel({ mode: "NORMAL", onSend });
    const input = textarea(container) as HTMLTextAreaElement;

    fireEvent.change(input, { target: { value: "Mở bán lúc mấy giờ" } });
    fireEvent.keyDown(input, { key: "Enter", isComposing: true });
    fireEvent.keyDown(input, { key: "Enter", keyCode: 229 });

    expect(onSend).not.toHaveBeenCalled();
    expect(input.value).toBe("Mở bán lúc mấy giờ");
  });

  it("Enter sau khi ghep xong -> gui binh thuong (AC-3b)", () => {
    const onSend = vi.fn();
    const { container } = renderPanel({ mode: "NORMAL", onSend });

    typeAndEnter(container, "Mở bán lúc mấy giờ?");
    expect(onSend).toHaveBeenCalledTimes(1);
  });

  it("Escape khi isComposing -> KHONG dong panel (AC-9b)", () => {
    const onClose = vi.fn();
    const { container } = renderPanel({ mode: "NORMAL", onClose, onSend: vi.fn() });

    fireEvent.keyDown(textarea(container) as HTMLTextAreaElement, { key: "Escape", isComposing: true });
    expect(onClose).not.toHaveBeenCalled();
  });
});

describe("AssistantPanel — du lieu bien (challenge C-1, C-5..C-7, C-10, C-11)", () => {
  it.each([1e20, 8.64e15 + 1, -8.64e15 - 1])(
    "sentAt = %s (ngoai mien Date) -> KHONG throw, KHONG hien gio (AC-2, C-1)",
    (sentAt) => {
      let container: HTMLElement | undefined;
      expect(() => {
        container = renderPanel({ mode: "NORMAL", messages: [{ ...USER_MSG, sentAt }] }).container;
      }).not.toThrow();
      expect(allText(container as HTMLElement)).not.toMatch(HHMM);
    },
  );

  it.each<[string, number, number]>([
    ["remaining > limit", 11, 10],
    ["limit = 0", 0, 0],
  ])("%s -> KHONG render quota (AC-7, C-6)", (_label, quotaRemaining, quotaLimit) => {
    const { container } = renderPanel({ mode: "NORMAL", onSend: vi.fn(), quotaRemaining, quotaLimit });

    expect(container.querySelector('[data-slot="quota"]')).toBeNull();
  });

  it("quotaRemaining am -> dong cua: textarea va Gui disabled (AC-7, C-5)", () => {
    const { container } = renderPanel({ mode: "NORMAL", onSend: vi.fn(), quotaRemaining: -1, quotaLimit: 10 });

    expect(isDisabled(textarea(container))).toBe(true);
    expect(isDisabled(submitButton(container))).toBe(true);
  });

  it.each(["javascript:alert(1)", " JavaScript:alert(1)", "data:text/html,x", "//evil.test"])(
    "supportHref = %s -> KHONG render link (C-7)",
    (supportHref) => {
      const { container } = renderPanel({ mode: "OFF", supportHref });

      expect(container.querySelector("a")).toBeNull();
      expect(allText(container)).toMatch(/chăm sóc khách hàng/i);
    },
  );

  it.each(["https://example.test/ho-tro", "/ho-tro", "mailto:cskh@example.test", "tel:1900"])(
    "supportHref = %s -> render link (C-7)",
    (supportHref) => {
      const { container } = renderPanel({ mode: "OFF", supportHref });

      expect(container.querySelector("a")?.getAttribute("href")).toBe(supportHref);
    },
  );

  it("kind la luc runtime -> data-kind='ai' va co nhan (C-10)", () => {
    const odd = { ...ASSISTANT_MSG, kind: "la" } as unknown as AssistantMessage;
    const { container } = renderPanel({ mode: "NORMAL", messages: [odd] });
    const [message] = messages(container);

    expect(message?.getAttribute("data-kind")).toBe("ai");
    expect(message?.textContent).toContain("Trợ lý AI");
  });

  it("text chi gom ky tu khong be rong -> KHONG gui (AC-3, C-11)", () => {
    const onSend = vi.fn();
    const { container } = renderPanel({ mode: "NORMAL", onSend });

    typeAndSubmit(container, "\u200B\u200C \uFEFF");
    expect(onSend).not.toHaveBeenCalled();
  });
});

describe("AssistantPanel — copy khop voi man hinh (code-review F-4)", () => {
  it.each<AssistantMode>(["SAVING", "FAQ_ONLY"])(
    "%s -> khong goi y 'Enter de gui' khi o nhap bi khoa, khong hua 'ben duoi' (F-4)",
    (mode) => {
      const { container } = renderPanel({ mode, onSend: vi.fn() });
      const text = allText(container);

      expect(text).not.toMatch(/Enter để gửi/);
      expect(text).not.toMatch(/bên dưới/);
      expect(text).not.toMatch(/đặt câu hỏi/);
    },
  );

  it("NORMAL mo -> co goi y 'Enter de gui' (F-4)", () => {
    const { container } = renderPanel({ mode: "NORMAL", onSend: vi.fn() });

    expect(allText(container)).toMatch(/Enter để gửi/);
  });
});
