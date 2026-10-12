"use client";

import { useCallback, useEffect, useState } from "react";
import {
  type AssistantMessage,
  type AssistantQuota,
  type DegradationMode,
  type FaqItem,
  DEGRADATION_NOTICES,
  getStoredQuota,
  resetStoredQuota,
  sendAssistantMessage,
  streamJobResponse,
} from "@/lib/assistant-client";

export interface UseAssistantOptions {
  eventId: string;
  initialMode?: DegradationMode;
  enableStreaming?: boolean;
}

export interface UseAssistantReturn {
  messages: AssistantMessage[];
  mode: DegradationMode;
  quota: AssistantQuota;
  isPending: boolean;
  isStreaming: boolean;
  error: string | null;
  sendMessage: (question: string) => Promise<void>;
  askFaq: (faq: FaqItem) => Promise<void>;
  setMode: (mode: DegradationMode) => void;
  resetQuota: () => void;
  clearHistory: () => void;
}

const welcomeMessage = (mode: DegradationMode): AssistantMessage => ({
  id: "welcome-msg",
  role: "assistant",
  content:
    "Xin chào! Tôi là Trợ lý EvenFlow. Tôi có thể giải đáp các quy chế phòng chờ, thời gian giữ vé 10:00 và hướng dẫn quy trình thanh toán.",
  createdAt: new Date().toISOString(),
  status: "complete",
  notice: DEGRADATION_NOTICES[mode] ?? undefined,
});

export function useAssistant({
  eventId,
  initialMode = "NORMAL",
  enableStreaming = true,
}: UseAssistantOptions): UseAssistantReturn {
  const [mode, setModeState] = useState<DegradationMode>(initialMode);
  const [messages, setMessages] = useState<AssistantMessage[]>([welcomeMessage(initialMode)]);
  const [quota, setQuotaState] = useState<AssistantQuota>(() => getStoredQuota(eventId));
  const [isPending, setIsPending] = useState(false);
  const [isStreaming, setIsStreaming] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Đồng bộ quota từ sessionStorage khi mount
  useEffect(() => {
    setQuotaState(getStoredQuota(eventId));
  }, [eventId]);

  // Đồng bộ khi initialMode thay đổi từ props (phục vụ demo / switch nấc Ops)
  useEffect(() => {
    setModeState(initialMode);
    setMessages([welcomeMessage(initialMode)]);
  }, [initialMode]);

  const setMode = useCallback((newMode: DegradationMode) => {
    setModeState(newMode);
    const notice = DEGRADATION_NOTICES[newMode];
    if (notice) {
      setMessages((prev) => [
        ...prev,
        {
          id: `sys-${Date.now()}`,
          role: "system",
          content: notice,
          createdAt: new Date().toISOString(),
          status: "complete",
        },
      ]);
    }
  }, []);

  const resetQuotaAction = useCallback(() => {
    const fresh = resetStoredQuota(eventId);
    setQuotaState(fresh);
  }, [eventId]);

  const clearHistory = useCallback(() => {
    setMessages([welcomeMessage(mode)]);
    setError(null);
  }, [mode]);

  const askFaq = useCallback(
    async (faq: FaqItem) => {
      setError(null);
      const userMsg: AssistantMessage = {
        id: `user-${Date.now()}`,
        role: "user",
        content: faq.question,
        createdAt: new Date().toISOString(),
        status: "complete",
      };

      const assistantMsg: AssistantMessage = {
        id: `faq-${Date.now()}`,
        role: "assistant",
        content: faq.answer,
        createdAt: new Date().toISOString(),
        status: "complete",
        isFaq: true,
        faqId: faq.id,
      };

      setMessages((prev) => [...prev, userMsg, assistantMsg]);
    },
    []
  );

  const sendMessage = useCallback(
    async (text: string) => {
      const trimmed = text.trim();
      if (!trimmed || isPending || isStreaming) return;

      setError(null);
      const userMsgId = `user-${Date.now()}`;
      const userMsg: AssistantMessage = {
        id: userMsgId,
        role: "user",
        content: trimmed,
        createdAt: new Date().toISOString(),
        status: "complete",
      };

      setMessages((prev) => [...prev, userMsg]);
      setIsPending(true);

      try {
        const result = await sendAssistantMessage({
          eventId,
          question: trimmed,
          mode,
        });

        // Cập nhật lại quota sau khi gửi
        setQuotaState(getStoredQuota(eventId));

        // 1. Trả lời ngay (FAQ, Từ chối ngoài phạm vi, hoặc Hết Quota, hoặc Suy biến)
        if (result.immediateAnswer) {
          const immediateMsg: AssistantMessage = {
            id: `assist-${Date.now()}`,
            role: "assistant",
            content: result.immediateAnswer,
            createdAt: new Date().toISOString(),
            status: "complete",
            isFaq: result.isFaq,
            notice: result.notice,
          };
          setMessages((prev) => [...prev, immediateMsg]);
          setIsPending(false);
          return;
        }

        // 2. Chế độ OFF
        if (mode === "OFF") {
          const offMsg: AssistantMessage = {
            id: `assist-${Date.now()}`,
            role: "assistant",
            content: "Trợ lý tạm thời không khả dụng. Bạn vui lòng liên hệ bộ phận CSKH để được hỗ trợ.",
            createdAt: new Date().toISOString(),
            status: "complete",
          };
          setMessages((prev) => [...prev, offMsg]);
          setIsPending(false);
          return;
        }

        // 3. Phản hồi luồng bất đồng bộ (job_id) qua streaming
        if (result.jobId) {
          const assistantMsgId = `assist-${Date.now()}`;
          const initialAssistantMsg: AssistantMessage = {
            id: assistantMsgId,
            role: "assistant",
            content: "",
            createdAt: new Date().toISOString(),
            status: "pending",
            jobId: result.jobId,
          };

          setMessages((prev) => [...prev, initialAssistantMsg]);

          if (enableStreaming) {
            setIsStreaming(true);
            await streamJobResponse(
              result.jobId,
              trimmed,
              (chunk) => {
                setMessages((prev) =>
                  prev.map((m) =>
                    m.id === assistantMsgId
                      ? { ...m, content: m.content + chunk, status: "streaming" }
                      : m
                  )
                );
              },
              (fullText) => {
                setMessages((prev) =>
                  prev.map((m) =>
                    m.id === assistantMsgId
                      ? { ...m, content: fullText, status: "complete" }
                      : m
                  )
                );
                setIsStreaming(false);
                setIsPending(false);
              }
            );
          } else {
            // Không bật streaming: trả luôn full text
            setMessages((prev) =>
              prev.map((m) =>
                m.id === assistantMsgId
                  ? {
                      ...m,
                      content:
                        "Cảm ơn bạn đã hỏi. Thông tin đã được xác nhận từ máy chủ EvenFlow.",
                      status: "complete",
                    }
                  : m
              )
            );
            setIsPending(false);
          }
        }
      } catch {
        const errorMsg: AssistantMessage = {
          id: `err-${Date.now()}`,
          role: "assistant",
          content:
            "Không thể kết nối tới dịch vụ trợ lý. Bạn vui lòng thử lại hoặc tham khảo danh mục FAQ.",
          createdAt: new Date().toISOString(),
          status: "error",
        };
        setMessages((prev) => [...prev, errorMsg]);
        setError("Lỗi kết nối");
        setIsPending(false);
        setIsStreaming(false);
      }
    },
    [eventId, mode, isPending, isStreaming, enableStreaming]
  );

  return {
    messages,
    mode,
    quota,
    isPending,
    isStreaming,
    error,
    sendMessage,
    askFaq,
    setMode,
    resetQuota: resetQuotaAction,
    clearHistory,
  };
}
