"use client";

import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { AdmitBanner } from "@/components/queue/admit-banner";
import { QueuePosition } from "@/components/queue/queue-position";
import { ChatPanel } from "@/components/assistant/chat-panel";
import { Badge, Button, Spinner } from "@/components/ui";
import { useQueueStatus } from "@/hooks/use-queue-status";
import { useServerTimeSync } from "@/hooks/use-server-time-sync";
import { joinQueue, QueueError } from "@/lib/queue-client";
import { cn } from "@/lib/cn";

export interface WaitingRoomProps {
  eventId: string;
  eventTitle?: string;
  autoRedirect?: boolean;
  className?: string;
}

const tokenKey = (eventId: string) => `ef:queue:token:${eventId}`;

export function WaitingRoom({
  eventId,
  eventTitle,
  autoRedirect = true,
  className,
}: WaitingRoomProps) {
  const router = useRouter();
  const { offsetMs } = useServerTimeSync();

  const [token, setToken] = useState<string | null>(null);
  const [isJoining, setIsJoining] = useState(false);
  const [joinError, setJoinError] = useState<string | null>(null);
  const [isAssistantOpen, setIsAssistantOpen] = useState(false);
  const redirectedRef = useRef(false);

  // 1. Quản lý khôi phục hoặc cấp mới queue token từ sessionStorage
  useEffect(() => {
    let cancelled = false;

    async function initQueueToken() {
      try {
        const savedToken = window.sessionStorage.getItem(tokenKey(eventId));
        if (savedToken) {
          setToken(savedToken);
          return;
        }

        // Chưa có token: gọi join (không jitter thêm vì đã jitter tại event-action-panel)
        setIsJoining(true);
        setJoinError(null);

        const res = await joinQueue(eventId, {}, { jitter: false });
        if (!cancelled) {
          window.sessionStorage.setItem(tokenKey(eventId), res.queue_token);
          setToken(res.queue_token);
        }
      } catch (err) {
        if (!cancelled) {
          if (err instanceof QueueError) {
            if (err.status === 401) {
              setJoinError("Bạn cần đăng nhập và xác thực OTP trước khi vào phòng chờ.");
            } else if (err.status === 403) {
              setJoinError("Yêu cầu cần xác minh thêm trước khi vào phòng chờ.");
            } else {
              setJoinError(err.message || "Không thể kết nối vào phòng chờ. Vui lòng thử lại.");
            }
          } else {
            setJoinError("Đã xảy ra lỗi mạng khi tham gia phòng chờ. Vui lòng thử lại.");
          }
        }
      } finally {
        if (!cancelled) {
          setIsJoining(false);
        }
      }
    }

    void initQueueToken();

    return () => {
      cancelled = true;
    };
  }, [eventId]);

  // 2. Theo dõi trạng thái xếp hàng thông qua hook tối ưu
  const { snapshot, initialRank, connection, error } = useQueueStatus({
    eventId,
    token,
  });

  const state = snapshot?.state ?? "LOBBY";

  // 3. Tự động điều hướng sang bước chọn vé khi đã tới lượt (AC-6)
  useEffect(() => {
    if (state === "ADMITTED" && autoRedirect && !redirectedRef.current) {
      redirectedRef.current = true;
      router.push(`/checkout/${eventId}`);
    }
  }, [state, autoRedirect, eventId, router]);

  // Loading ban đầu lúc gọi join
  if (isJoining) {
    return (
      <div className={cn("flex flex-col items-center justify-center p-2xl gap-md", className)}>
        <Spinner size="lg" />
        <p className="text-body-lg text-fg font-medium">Đang đăng ký vào phòng chờ...</p>
      </div>
    );
  }

  // Lỗi khi join
  if (joinError) {
    return (
      <div
        role="alert"
        className={cn(
          "flex flex-col items-center justify-center p-xl gap-md rounded-container border border-danger bg-danger-soft text-center",
          className
        )}
      >
        <span className="text-headline-md font-bold text-danger">Không thể vào phòng chờ</span>
        <p className="text-body-md text-fg">{joinError}</p>
        <Button
          variant="secondary"
          onClick={() => {
            window.sessionStorage.removeItem(tokenKey(eventId));
            window.location.reload();
          }}
        >
          Thử lại
        </Button>
      </div>
    );
  }

  // Lỗi terminal từ status
  if (error) {
    return (
      <div
        role="alert"
        className={cn(
          "flex flex-col items-center justify-center p-xl gap-md rounded-container border border-danger bg-danger-soft text-center",
          className
        )}
      >
        <span className="text-headline-md font-bold text-danger">Phiên xếp hàng không hợp lệ</span>
        <p className="text-body-md text-fg">
          {error.message || "Phiên của bạn đã hết hạn hoặc không tìm thấy. Vui lòng xếp hàng lại."}
        </p>
        <Button
          variant="secondary"
          onClick={() => {
            window.sessionStorage.removeItem(tokenKey(eventId));
            window.location.reload();
          }}
        >
          Xếp hàng lại từ đầu
        </Button>
      </div>
    );
  }

  return (
    <div className={cn("flex flex-col gap-xl w-full max-w-3xl mx-auto py-xl", className)}>
      {/* Tiêu đề & kết nối */}
      <header className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-md border-b border-border pb-md">
        <div>
          <span className="text-label-md text-fg-muted block">Phòng chờ trực tuyến</span>
          <h1 className="text-heading-xl font-bold text-fg">
            {eventTitle || "Sự kiện EvenFlow"}
          </h1>
        </div>

        <div className="flex items-center gap-sm">
          <Button
            variant="secondary"
            size="sm"
            onClick={() => setIsAssistantOpen(true)}
            className="flex items-center gap-xs"
            aria-label="Mở trợ lý ảo AI"
          >
            <span aria-hidden="true">💬</span>
            <span>Hỏi trợ lý AI</span>
          </Button>

          {connection === "reconnecting" && (
            <Badge variant="pending">Đang kết nối lại...</Badge>
          )}
          {connection === "offline" && <Badge variant="failed">Mất kết nối</Badge>}
          {connection === "live" && (
            <Badge variant={state === "ADMITTED" ? "admitted" : "queued"}>
              {state === "ADMITTED" ? "Tới lượt" : "Trực tiếp"}
            </Badge>
          )}
        </div>
      </header>

      {/* Thông báo đang kết nối lại (BR-Q7: mất kết nối không mất chỗ) */}
      {connection === "reconnecting" && (
        <div
          role="status"
          className="p-md rounded-card border border-warning bg-warning-soft text-body-md text-warning-fg"
        >
          Hệ thống đang kết nối lại. Bạn chưa bị mất chỗ trong hàng đợi, vui lòng giữ nguyên tab này.
        </div>
      )}

      {/* Banner khi đã tới lượt (AC-6) */}
      {state === "ADMITTED" && (
        <AdmitBanner
          expiresAt={snapshot?.expiresAt}
          offsetMs={offsetMs}
          checkoutUrl={`/checkout/${eventId}`}
          onProceed={() => router.push(`/checkout/${eventId}`)}
        />
      )}

      {/* Hiển thị vị trí & tiến trình (AC-5) */}
      <section aria-label="Thông tin vị trí hàng đợi">
        <QueuePosition
          state={state}
          rank={snapshot?.rank ?? null}
          etaSeconds={snapshot?.etaSeconds ?? null}
          initialRank={initialRank}
        />
      </section>

      {/* Hướng dẫn an toàn */}
      <footer className="p-md rounded-card bg-surface-subtle text-body-sm text-fg-muted border border-border">
        <p className="font-medium text-fg mb-xs">Lưu ý khi chờ đợi:</p>
        <ul className="list-disc list-inside space-y-1">
          <li>Trang sẽ tự động cập nhật theo nhịp của hệ thống, không cần bấm F5.</li>
          <li>Khi tới lượt, bạn sẽ được tự động chuyển sang trang chọn vé và giữ chỗ.</li>
          <li>Mỗi suất mua vé có thời hạn tối đa 15 phút.</li>
        </ul>
      </footer>

      {/* Drawer Trợ lý AI (EV-185, EVF-115) */}
      <ChatPanel
        eventId={eventId}
        eventTitle={eventTitle}
        isOpen={isAssistantOpen}
        onClose={() => setIsAssistantOpen(false)}
      />
    </div>
  );
}
