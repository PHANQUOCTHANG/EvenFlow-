"use client";

import {
  useEffect,
  useId,
  useState,
  type FormEvent,
  type KeyboardEvent,
} from "react";

import { Alert, Button } from "@/components/ui";
import { cn } from "@/lib/cn";
import { toEpochMs } from "@/lib/server-time";

/** Bon nac suy bien cua ai-worker (docs/02 muc 3, BR-A6). Giu dung ten backend.
 *
 *  BAT BUOC, khong co default: neu caller quen truyen ma UI tu coi la NORMAL thi dung luc
 *  he thong can ngan sach, UI lai mo cua cho khach goi Gemini. */
export type AssistantMode = "NORMAL" | "SAVING" | "FAQ_ONLY" | "OFF";

/** Loai cau tra loi (handoff muc 11.7): khach phai biet minh dang doc gi. */
export type AssistantAnswerKind = "ai" | "faq" | "out_of_scope" | "fallback";

/** pending = job dang chay (BR-A1: khong co gi dong bo), failed = job loi hoac qua TTL. */
export type AssistantMessageStatus = "pending" | "done" | "failed";

export interface AssistantMessage {
  id: string;
  role: "user" | "assistant";
  text: string;
  /** Chi co nghia voi role "assistant". Thieu -> "ai". */
  kind?: AssistantAnswerKind;
  /** Chi co nghia voi role "user". Thieu -> "done". */
  status?: AssistantMessageStatus;
  /** Epoch ms hoac ISO-8601 CO mui gio. Thieu hoac khong doc duoc -> khong hien gio. */
  sentAt?: number | string;
}

export interface AssistantFaq {
  id: string;
  label: string;
}

/** KHONG co prop rank / eta / ton kho / trang thai don — co y (BR-A4, BR-A5, handoff muc 10.11).
 *
 *  Tro ly khong co tham quyen. Neu backend co so thuc (qua tool-call) thi no nam trong `text`
 *  cua tin nhan; UI khong co cho de tu them mot con so nao. So lieu chinh thuc o bang trang
 *  thai hang cho, khong o day. */
export interface AssistantPanelProps {
  mode: AssistantMode;
  messages: AssistantMessage[];
  faqs?: AssistantFaq[];
  /** BR-A2: 10 cau / 15 phut. Chi hien so khi CA HAI la so nguyen >= 0. */
  quotaRemaining?: number;
  quotaLimit?: number;
  /** Nhan text da trim. Thieu -> o nhap bi khoa. */
  onSend?: (text: string) => void;
  onSelectFaq?: (faqId: string) => void;
  onRetry?: (messageId: string) => void;
  /** Co -> hien nut dong, va Escape trong panel se dong. */
  onClose?: () => void;
  /** Kenh CSKH. Thieu -> cau chu chung, khong co link. */
  supportHref?: string;
  className?: string;
}

/** Nhan cho tung loai cau tra loi. Bon nhan phai KHAC nhau (AC-2). */
const KIND_LABEL: Record<AssistantAnswerKind, string> = {
  ai: "Trợ lý AI",
  faq: "Câu trả lời mẫu",
  out_of_scope: "Ngoài phạm vi hỗ trợ",
  fallback: "Trả lời dự phòng",
};

/** Thong bao suy bien. KHONG duoc chua tu ky thuat (spec muc 2.5): khach trong phong cho
 *  khong can biet Gemini hay 429 la gi — ho can biet con hoi duoc gi va hoi o dau. */
const MODE_NOTICE: Partial<Record<AssistantMode, { title: string; body: string }>> = {
  SAVING: {
    title: "Trợ lý đang ở chế độ tiết kiệm",
    body: "Lúc này trợ lý chỉ trả lời các câu hỏi thường gặp. Câu hỏi tự do tạm thời chưa nhận.",
  },
  FAQ_ONLY: {
    title: "Trợ lý chỉ trả lời câu hỏi thường gặp",
    body: "Trợ lý vẫn trả lời các câu hỏi thường gặp. Câu hỏi tự do tạm thời chưa nhận.",
  },
  OFF: {
    title: "Trợ lý tạm không khả dụng",
    body: "Trợ lý đang tạm ngưng. Thông tin hàng chờ và vé vẫn hiển thị đầy đủ ở bảng trạng thái chính thức.",
  },
};

function isValidQuota(value: number | undefined): value is number {
  return typeof value === "number" && Number.isInteger(value) && value >= 0;
}

/** Ky tu khong be rong: `trim()` khong xoa, nen mot chuoi chi gom chung van "khong rong". */
const INVISIBLE_CHARS = /[\u200B-\u200D\u2060\uFEFF]/g;

/** Chi nhan link CSKH an toan; `javascript:` va scheme la thi coi nhu khong co link. */
const SAFE_HREF = /^(https?:|mailto:|tel:|\/(?!\/)|#)/i;

function safeSupportHref(href: string | undefined): string | undefined {
  return href && SAFE_HREF.test(href.trim()) ? href : undefined;
}

/** Epoch ms ngoai mien cua Date (vd 1e20) lam `toISOString()` throw -> coi nhu khong co gio. */
function toValidDate(epoch: number | null): Date | null {
  if (epoch === null) return null;
  const date = new Date(epoch);
  return Number.isNaN(date.getTime()) ? null : date;
}

/** Enter/Escape trong luc IME (Telex/VNI) dang ghep chu la de chot chu, khong phai lenh. */
function isComposing(event: KeyboardEvent<HTMLElement>): boolean {
  return event.nativeEvent.isComposing || event.keyCode === 229;
}

/** HH:mm theo gio may nguoi xem. Chi goi SAU khi mount (xem `mounted`). */
function formatHm(date: Date): string {
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function SupportHint({ href }: { href?: string }) {
  if (!href) {
    return <span>Bạn có thể liên hệ bộ phận chăm sóc khách hàng để được hỗ trợ thêm.</span>;
  }
  return (
    <span>
      Bạn có thể{" "}
      <a href={href} className="ef-focus-ring font-medium text-primary underline">
        liên hệ bộ phận chăm sóc khách hàng
      </a>{" "}
      để được hỗ trợ thêm.
    </span>
  );
}

export function AssistantPanel({
  mode,
  messages,
  faqs,
  quotaRemaining,
  quotaLimit,
  onSend,
  onSelectFaq,
  onRetry,
  onClose,
  supportHref: rawSupportHref,
  className,
}: AssistantPanelProps) {
  const supportHref = safeSupportHref(rawSupportHref);
  const [draft, setDraft] = useState("");
  // Gio hien thi theo may nguoi xem -> chi render sau mount, tranh hydration mismatch
  // giua server (UTC) va client (ICT).
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);

  const inputId = useId();

  const hasPending = messages.some((m) => m.role === "user" && m.status === "pending");
  // Chi hien so khi cap so hop ly: limit > 0 va remaining khong vuot limit (khong bia "11/10").
  const showQuota =
    isValidQuota(quotaRemaining) &&
    isValidQuota(quotaLimit) &&
    quotaLimit > 0 &&
    quotaRemaining <= quotaLimit;
  // `quotaRemaining <= 0` tu no da du de chan, khong can `quotaLimit` (BR-A2). So am la sai
  // nhung phai dong cua (fail closed), khong mo cho goi them.
  const quotaExhausted = typeof quotaRemaining === "number" && quotaRemaining <= 0;

  // Gui lai cung la mot cau hoi mo (tao job moi): chiu chung moi chan tru viec co `onSend`.
  const retryAllowed = mode === "NORMAL" && !hasPending && !quotaExhausted;
  // Cau hoi mo: chi o NORMAL, chua co job dang chay, con luot, va co noi de gui.
  const openQuestionsAllowed = retryAllowed && Boolean(onSend);
  // FAQ: khong ton Gemini nen KHONG bi chan boi suy bien hay het luot (BR-A2, BR-A6),
  // chi bi chan khi dang co job (mot nguoi mot job) hoac khong co handler.
  const faqAllowed = !hasPending && Boolean(onSelectFaq);

  const notice = MODE_NOTICE[mode];

  function submit() {
    const text = draft.trim();
    if (text.replace(INVISIBLE_CHARS, "").length === 0) return;
    if (!openQuestionsAllowed || text.length === 0) return;
    onSend?.(text);
    setDraft("");
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    submit();
  }

  function handleTextareaKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === "Enter" && !event.shiftKey && !isComposing(event)) {
      event.preventDefault();
      submit();
    }
  }

  /** Gan o goc panel, KHONG gan document: khong nuot Escape cua dialog khac tren trang. */
  function handlePanelKeyDown(event: KeyboardEvent<HTMLElement>) {
    if (event.key === "Escape" && onClose && !isComposing(event) && !event.defaultPrevented) {
      event.stopPropagation();
      onClose();
    }
  }

  return (
    <section
      role="region"
      aria-label="Trợ lý sự kiện"
      data-slot="assistant-panel"
      data-mode={mode}
      data-authoritative="false"
      onKeyDown={handlePanelKeyDown}
      className={cn(
        "flex flex-col gap-md rounded-card border border-dashed border-border-strong bg-surface p-md shadow-card",
        className,
      )}
    >
      <header className="flex items-start justify-between gap-sm">
        <div className="flex flex-col gap-xs">
          <h2 className="text-headline-sm text-fg">Trợ lý sự kiện</h2>
          {/* Nhan "AI ho tro" la chu, khong chi la mau (handoff muc 10.11 + muc 5 nguyen tac 7). */}
          <span className="inline-flex w-fit items-center gap-xs rounded-full bg-info-soft px-sm py-xs text-label-md text-primary">
            <span aria-hidden="true">✦</span> AI hỗ trợ · chỉ để tham khảo
          </span>
        </div>
        {onClose ? (
          <Button variant="tertiary" size="sm" aria-label="Đóng trợ lý" onClick={onClose}>
            <span aria-hidden="true">✕</span>
          </Button>
        ) : null}
      </header>

      {mode === "OFF" ? (
        <Alert variant="info" title={MODE_NOTICE.OFF?.title}>
          <p>{MODE_NOTICE.OFF?.body}</p>
          <p className="mt-xs">
            <SupportHint href={supportHref} />
          </p>
        </Alert>
      ) : (
        <>
          <p
            role="note"
            data-slot="authority-notice"
            className="rounded-control border border-border bg-surface-subtle px-sm py-xs text-body-sm text-fg-muted"
          >
            Trợ lý chỉ giải thích quy định và hướng dẫn thao tác. Trợ lý <strong>không giữ vé</strong>,{" "}
            <strong>không thay đổi đơn hàng</strong>. Số thứ tự, thời gian chờ và tình trạng vé chỉ
            lấy ở <strong>bảng trạng thái chính thức</strong>.
          </p>

          {notice ? (
            <Alert variant="info" title={notice.title}>
              {notice.body}
            </Alert>
          ) : null}

          <ol
            role="log"
            aria-live="polite"
            aria-label="Hội thoại với trợ lý"
            className="flex flex-col gap-sm"
          >
            {messages.length === 0 ? (
              <li className="text-body-md text-fg-muted">
                {mode === "NORMAL"
                  ? "Xin chào! Hãy chọn một câu hỏi thường gặp hoặc đặt câu hỏi về quy định mua vé."
                  : "Xin chào! Hãy chọn một câu hỏi thường gặp để được trả lời."}
              </li>
            ) : null}
            {messages.map((message) => (
              <MessageItem
                key={message.id}
                message={message}
                mounted={mounted}
                onRetry={onRetry}
                retryAllowed={retryAllowed}
              />
            ))}
          </ol>

          {faqs && faqs.length > 0 ? (
            <div role="group" aria-label="Câu hỏi thường gặp" className="flex flex-wrap gap-xs">
              {faqs.map((faq) => (
                <button
                  key={faq.id}
                  type="button"
                  disabled={!faqAllowed}
                  onClick={() => onSelectFaq?.(faq.id)}
                  className={cn(
                    "ef-focus-ring rounded-full border border-border-strong bg-surface px-sm py-xs text-label-md text-fg",
                    "hover:bg-surface-subtle disabled:cursor-not-allowed disabled:opacity-60",
                  )}
                >
                  {faq.label}
                </button>
              ))}
            </div>
          ) : null}

          {quotaExhausted ? (
            <Alert variant="info" title="Bạn đã dùng hết lượt hỏi">
              Trợ lý vẫn trả lời các câu hỏi thường gặp. <SupportHint href={supportHref} />
            </Alert>
          ) : null}

          <form onSubmit={handleSubmit} className="flex flex-col gap-xs">
            <label htmlFor={inputId} className="text-label-md text-fg">
              Câu hỏi của bạn
            </label>
            <div className="flex items-end gap-sm">
              <textarea
                id={inputId}
                value={draft}
                rows={2}
                disabled={!openQuestionsAllowed}
                onChange={(event) => setDraft(event.target.value)}
                onKeyDown={handleTextareaKeyDown}
                aria-describedby={showQuota ? `${inputId}-quota` : undefined}
                className={cn(
                  "ef-focus-ring min-h-11 w-full resize-y rounded-control border border-border-strong bg-surface px-sm py-xs",
                  "text-body-lg text-fg disabled:cursor-not-allowed disabled:opacity-60",
                )}
              />
              <Button type="submit" size="md" disabled={!openQuestionsAllowed}>
                Gửi
              </Button>
            </div>
            <div className="flex flex-wrap justify-between gap-xs text-body-sm text-fg-muted">
              {openQuestionsAllowed ? <span>Enter để gửi, Shift+Enter để xuống dòng.</span> : null}
              {showQuota ? (
                <span id={`${inputId}-quota`} data-slot="quota" className="tabular-nums">
                  Còn {quotaRemaining}/{quotaLimit} lượt hỏi
                </span>
              ) : null}
            </div>
          </form>
        </>
      )}
    </section>
  );
}

function MessageItem({
  message,
  mounted,
  onRetry,
  retryAllowed,
}: {
  message: AssistantMessage;
  mounted: boolean;
  onRetry?: (messageId: string) => void;
  retryAllowed: boolean;
}) {
  const isUser = message.role === "user";
  // Kind la luc runtime (du lieu tu backend) -> coi nhu "ai" de khong mat nhan.
  const kind: AssistantAnswerKind | undefined = isUser
    ? undefined
    : message.kind && message.kind in KIND_LABEL
      ? message.kind
      : "ai";
  const status: AssistantMessageStatus | undefined = isUser ? (message.status ?? "done") : undefined;

  const date = mounted ? toValidDate(toEpochMs(message.sentAt)) : null;

  return (
    <li
      data-slot="message"
      data-role={message.role}
      data-kind={kind}
      data-status={status}
      className={cn("flex flex-col gap-xs", isUser ? "items-end" : "items-start")}
    >
      <div
        className={cn(
          "max-w-[85%] rounded-card px-sm py-xs text-body-md",
          isUser
            ? "bg-primary text-primary-fg"
            : "border border-dashed border-border-strong bg-surface-subtle text-fg",
          kind === "out_of_scope" && "italic",
        )}
      >
        <div className="mb-xs flex flex-wrap items-center gap-xs text-label-md opacity-80">
          <span>{isUser ? "Bạn" : KIND_LABEL[kind ?? "ai"]}</span>
          {date ? (
            <time className="tabular-nums" dateTime={date.toISOString()}>
              {formatHm(date)}
            </time>
          ) : null}
        </div>
        {/* Text thuan, React tu escape — noi dung la DU LIEU, khong phai markup (BR-A7). */}
        <p className="whitespace-pre-wrap break-words">{message.text}</p>
      </div>

      {status === "pending" ? (
        <span className="text-body-sm text-fg-muted">Đang xử lý câu hỏi của bạn…</span>
      ) : null}

      {status === "failed" ? (
        <span className="flex flex-wrap items-center gap-xs text-body-sm text-fg-muted">
          <span>Chưa nhận được câu trả lời cho câu hỏi này.</span>
          {onRetry ? (
            <Button
              variant="tertiary"
              size="sm"
              disabled={!retryAllowed}
              onClick={() => onRetry(message.id)}
            >
              Gửi lại
            </Button>
          ) : null}
        </span>
      ) : null}
    </li>
  );
}
