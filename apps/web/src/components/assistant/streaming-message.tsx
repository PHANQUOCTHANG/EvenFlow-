"use client";

import React from "react";
import type { AssistantMessage } from "@/lib/assistant-client";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/cn";

export interface StreamingMessageProps {
  message: AssistantMessage;
  isLatestAssistant?: boolean;
  className?: string;
}

export function StreamingMessage({
  message,
  isLatestAssistant = false,
  className,
}: StreamingMessageProps) {
  const isUser = message.role === "user";
  const isSystem = message.role === "system";
  const isStreaming = message.status === "streaming";
  const isPending = message.status === "pending";

  if (isSystem) {
    return (
      <div
        role="status"
        className={cn(
          "w-full text-center py-xs px-md my-xs rounded-card bg-surface-subtle text-fg-muted text-body-sm border border-border",
          className
        )}
      >
        <span>{message.content}</span>
      </div>
    );
  }

  return (
    <div
      className={cn(
        "flex flex-col gap-xs w-full",
        isUser ? "items-end" : "items-start",
        className
      )}
      data-testid={`message-${message.id}`}
    >
      {/* Header nguồn thông tin / vai trò */}
      <div className="flex items-center gap-xs px-xs">
        <span className="text-label-sm text-fg-muted font-medium">
          {isUser ? "Bạn" : "Trợ lý EvenFlow"}
        </span>

        {!isUser && (
          <>
            {message.isFaq ? (
              <Badge variant="neutral" className="text-[10px] px-xxs py-0">
                FAQ
              </Badge>
            ) : (
              <Badge variant="queued" className="text-[10px] px-xxs py-0">
                AI hỗ trợ
              </Badge>
            )}
          </>
        )}
      </div>

      {/* Bubble tin nhắn */}
      <div
        className={cn(
          "relative max-w-[85%] rounded-container px-md py-sm text-body-md transition-all shadow-xs",
          isUser
            ? "bg-primary text-primary-fg rounded-tr-none"
            : "bg-surface-raised border border-border text-fg rounded-tl-none"
        )}
      >
        {isPending ? (
          <div
            role="status"
            aria-label="Đang xử lý câu hỏi..."
            className="flex items-center gap-xs py-xxs text-fg-muted"
          >
            <span className="inline-block w-2 h-2 rounded-full bg-primary animate-ping" />
            <span className="text-body-sm italic">Đang xử lý câu trả lời...</span>
          </div>
        ) : (
          <div className="whitespace-pre-wrap break-words leading-relaxed">
            {message.content}
            {isStreaming && (
              <span
                role="presentation"
                aria-hidden="true"
                className="inline-block w-1.5 h-4 ml-0.5 align-middle bg-primary animate-pulse"
              />
            )}
          </div>
        )}

        {/* Thông báo notice kèm theo nếu có */}
        {message.notice && (
          <div className="mt-xs pt-xs border-t border-border/50 text-label-sm text-fg-muted italic">
            {message.notice}
          </div>
        )}
      </div>

      {/* Ranh giới nguồn thông tin cho assistant (Stitch 10.11) */}
      {!isUser && isLatestAssistant && !isPending && (
        <span className="text-[11px] text-fg-muted px-xs">
          Nguồn: Quy chế EvenFlow & thông tin xác nhận từ máy chủ.
        </span>
      )}
    </div>
  );
}
