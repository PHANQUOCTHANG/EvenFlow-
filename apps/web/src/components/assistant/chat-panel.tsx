"use client";

import React, { useEffect, useRef, useState } from "react";
import { Badge, Button, Input } from "@/components/ui";
import { FaqChips } from "@/components/assistant/faq-chips";
import { StreamingMessage } from "@/components/assistant/streaming-message";
import { useAssistant } from "@/hooks/use-assistant";
import {
  CSKH_INFO,
  DEGRADATION_NOTICES,
  type DegradationMode,
} from "@/lib/assistant-client";
import { cn } from "@/lib/cn";

export interface ChatPanelProps {
  eventId: string;
  eventTitle?: string;
  isOpen: boolean;
  onClose: () => void;
  initialMode?: DegradationMode;
  className?: string;
}

export function ChatPanel({
  eventId,
  eventTitle,
  isOpen,
  onClose,
  initialMode = "NORMAL",
  className,
}: ChatPanelProps) {
  const {
    messages,
    mode,
    quota,
    isPending,
    isStreaming,
    sendMessage,
    askFaq,
    setMode,
  } = useAssistant({ eventId, initialMode });

  const [inputVal, setInputVal] = useState("");
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // Cuộn tự động khi có tin nhắn mới hoặc streaming chunk
  useEffect(() => {
    if (isOpen && typeof messagesEndRef.current?.scrollIntoView === "function") {
      messagesEndRef.current.scrollIntoView({ behavior: "smooth" });
    }
  }, [messages, isOpen]);

  // Focus ô input khi mở panel
  useEffect(() => {
    if (isOpen && mode === "NORMAL" && quota.used < quota.max) {
      inputRef.current?.focus();
    }
  }, [isOpen, mode, quota]);

  // Đóng bằng phím ESC
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape" && isOpen) {
        onClose();
      }
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, onClose]);

  if (!isOpen) return null;

  const isQuotaExceeded = quota.used >= quota.max;
  const isInputDisabled =
    mode === "FAQ_ONLY" ||
    mode === "SAVING" ||
    mode === "OFF" ||
    isQuotaExceeded ||
    isPending ||
    isStreaming;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!inputVal.trim() || isInputDisabled) return;
    const text = inputVal;
    setInputVal("");
    void sendMessage(text);
  };

  return (
    <aside
      role="region"
      aria-label="Bảng điều khiển trợ lý AI"
      className={cn(
        "fixed inset-y-0 right-0 z-50 flex flex-col w-full sm:w-[420px] bg-surface border-l border-border shadow-2xl transition-transform duration-300",
        className
      )}
    >
      {/* 1. Header */}
      <header className="flex items-center justify-between px-md py-sm border-b border-border bg-surface-raised">
        <div className="flex flex-col">
          <div className="flex items-center gap-xs">
            <h2 className="text-body-lg font-bold text-fg">Trợ lý EventFlow</h2>
            <Badge variant="queued" className="text-[10px]">
              AI hỗ trợ
            </Badge>
          </div>
          {eventTitle && (
            <span className="text-label-sm text-fg-muted truncate max-w-[260px]">
              {eventTitle}
            </span>
          )}
        </div>

        <div className="flex items-center gap-xs">
          {mode === "NORMAL" && (
            <span
              className="text-label-sm text-fg-muted bg-surface-subtle px-xs py-xxs rounded-full border border-border"
              title="Hạn mức 10 câu hỏi trong 15 phút"
            >
              Lượt: {quota.max - quota.used}/{quota.max}
            </span>
          )}

          {mode === "SAVING" && (
            <Badge variant="pending" className="text-[10px]">
              Tiết kiệm
            </Badge>
          )}

          {mode === "FAQ_ONLY" && (
            <Badge variant="pending" className="text-[10px]">
              Chỉ FAQ
            </Badge>
          )}

          {mode === "OFF" && (
            <Badge variant="failed" className="text-[10px]">
              Tạm ngưng
            </Badge>
          )}

          <Button
            variant="tertiary"
            size="sm"
            onClick={onClose}
            aria-label="Đóng bảng trợ lý"
            className="p-xs h-8 w-8 text-fg-muted hover:text-fg"
          >
            ✕
          </Button>
        </div>
      </header>

      {/* 1.1 Thanh chuyển nấc suy biến (Cho phép test và Ops chuyển nấc trực tiếp) */}
      <div className="flex items-center justify-between px-md py-xs bg-surface-subtle border-b border-border text-label-sm">
        <span className="text-fg-muted font-medium text-[12px]">Nấc suy biến:</span>
        <div className="flex items-center gap-xxs" role="group" aria-label="Chọn nấc suy biến">
          {(["NORMAL", "SAVING", "FAQ_ONLY", "OFF"] as DegradationMode[]).map((m) => (
            <button
              key={m}
              type="button"
              onClick={() => setMode(m)}
              className={cn(
                "px-xs py-xxs text-[11px] rounded-control border transition-all",
                mode === m
                  ? "bg-primary text-primary-fg border-primary font-bold shadow-xs"
                  : "bg-surface text-fg-muted border-border hover:text-fg hover:border-primary/40"
              )}
              aria-pressed={mode === m}
            >
              {m}
            </button>
          ))}
        </div>
      </div>

      {/* 2. Trường hợp nấc suy biến OFF: Ẩn hoàn toàn chat, chỉ hiện kênh CSKH (AC-4) */}
      {mode === "OFF" ? (
        <div className="flex-1 flex flex-col items-center justify-center p-xl gap-md text-center bg-surface-subtle">
          <div className="w-12 h-12 rounded-full bg-danger-soft border border-danger/30 flex items-center justify-center text-danger text-headline-sm font-bold">
            !
          </div>

          <h3 className="text-heading-md font-bold text-fg">
            Trợ lý AI đang tạm ngưng
          </h3>

          <p className="text-body-md text-fg-muted">
            {DEGRADATION_NOTICES.OFF}
          </p>

          <div className="w-full mt-md p-md rounded-container bg-surface border border-border flex flex-col gap-sm text-left">
            <span className="text-label-md font-semibold text-fg">
              Kênh Chăm sóc khách hàng (CSKH):
            </span>
            <div className="flex justify-between text-body-sm">
              <span className="text-fg-muted">Hotline:</span>
              <span className="font-bold text-primary">{CSKH_INFO.hotline}</span>
            </div>
            <div className="flex justify-between text-body-sm">
              <span className="text-fg-muted">Email:</span>
              <span className="font-medium text-fg">{CSKH_INFO.email}</span>
            </div>
            <div className="flex justify-between text-body-sm">
              <span className="text-fg-muted">Thời gian:</span>
              <span className="text-fg">{CSKH_INFO.workingHours}</span>
            </div>
          </div>

          <Button variant="secondary" onClick={onClose} className="mt-md w-full">
            Đóng cửa sổ
          </Button>
        </div>
      ) : (
        <>
          {/* 3. Banners cảnh báo nấc suy biến (SAVING, FAQ_ONLY) & Quota */}
          {mode === "FAQ_ONLY" && (
            <div
              role="alert"
              className="px-md py-xs bg-warning-soft border-b border-warning text-body-sm text-warning-fg"
            >
              {DEGRADATION_NOTICES.FAQ_ONLY}
            </div>
          )}

          {mode === "SAVING" && (
            <div
              role="status"
              className="px-md py-xs bg-surface-subtle border-b border-border text-body-sm text-fg-muted"
            >
              {DEGRADATION_NOTICES.SAVING}
            </div>
          )}

          {mode === "NORMAL" && isQuotaExceeded && (
            <div
              role="status"
              className="px-md py-xs bg-warning-soft border-b border-warning text-body-sm text-warning-fg"
            >
              Bạn đã dùng hết 10 câu hỏi trong phiên 15 phút. Vui lòng tham khảo các gợi ý FAQ bên dưới.
            </div>
          )}

          {/* 4. Vùng lịch sử tin nhắn */}
          <div
            role="log"
            aria-live="polite"
            className="flex-1 overflow-y-auto p-md space-y-md"
          >
            {messages.map((msg, idx) => (
              <StreamingMessage
                key={msg.id}
                message={msg}
                isLatestAssistant={
                  msg.role === "assistant" && idx === messages.length - 1
                }
              />
            ))}
            <div ref={messagesEndRef} />
          </div>

          {/* 5. Gợi ý FAQ chips */}
          <div className="px-md py-xs border-t border-border bg-surface-raised">
            <FaqChips
              disabled={isPending || isStreaming}
              onSelectFaq={(faq) => {
                void askFaq(faq);
              }}
            />
          </div>

          {/* 6. Form gửi câu hỏi */}
          <footer className="p-md border-t border-border bg-surface">
            <form onSubmit={handleSubmit} className="flex gap-xs items-end">
              <Input
                id="assistant-user-input"
                ref={inputRef}
                label="Nội dung câu hỏi"
                value={inputVal}
                onChange={(e) => setInputVal(e.target.value)}
                disabled={isInputDisabled}
                placeholder={
                  mode === "FAQ_ONLY" || mode === "SAVING"
                    ? "Đang ở chế độ FAQ (hãy chọn gợi ý phía trên)"
                    : isQuotaExceeded
                    ? "Đã dùng hết 10 lượt hỏi (chọn FAQ phía trên)"
                    : "Đặt câu hỏi về sự kiện, vé..."
                }
                className="flex-1 text-body-sm"
              />
              <Button
                type="submit"
                variant="primary"
                disabled={isInputDisabled || !inputVal.trim()}
              >
                Gửi
              </Button>
            </form>
          </footer>
        </>
      )}
    </aside>
  );
}
