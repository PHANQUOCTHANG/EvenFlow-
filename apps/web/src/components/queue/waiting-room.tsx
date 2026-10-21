"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { AdmitBanner } from "@/components/queue/admit-banner";
import { QueuePosition } from "@/components/queue/queue-position";
import { QueueStatusPanel } from "@/components/queue/queue-status-panel";
import { ChatPanel } from "@/components/assistant/chat-panel";
import { useT } from "@/components/i18n/locale-provider";
import { Badge, Button, Spinner } from "@/components/ui";
import { useQueueStatus } from "@/hooks/use-queue-status";
import { useServerTimeSync } from "@/hooks/use-server-time-sync";
import { ApiError } from "@/lib/api-client";
import { joinQueue, QueueError } from "@/lib/queue-client";
import { cn } from "@/lib/cn";
import { clearQueueToken, readQueueToken, saveQueueToken } from "@/lib/queue-token";

export interface WaitingRoomProps {
  eventId: string;
  eventTitle?: string;
  autoRedirect?: boolean;
  className?: string;
}

/** Loi join co the hanh dong duoc (AC-9): otp-required / device-required co huong dan rieng. */
type JoinFailure = { kind: "otp" | "device" | "auth" | "generic"; message?: string };

export function WaitingRoom({
  eventId,
  eventTitle,
  autoRedirect = true,
  className,
}: WaitingRoomProps) {
  const router = useRouter();
  const t = useT();
  const { offsetMs } = useServerTimeSync();

  const [token, setToken] = useState<string | null>(null);
  const [isJoining, setIsJoining] = useState(false);
  const [joinError, setJoinError] = useState<JoinFailure | null>(null);
  const [isAssistantOpen, setIsAssistantOpen] = useState(false);
  // Het ve han luc join (EV-146): khong co token de poll, nen trang thai den thang tu ket qua join.
  const [soldOut, setSoldOut] = useState(false);
  // Moi lan tang = vao lai hang (token cu bi tu choi). Idempotent theo BR-Q3 nen khong mat cho.
  const [joinAttempt, setJoinAttempt] = useState(0);
  const redirectedRef = useRef(false);
  const rejoinedRef = useRef(false);

  // 1. Quản lý khôi phục hoặc cấp mới queue token từ sessionStorage
  useEffect(() => {
    let cancelled = false;

    async function initQueueToken() {
      try {
        const savedToken = readQueueToken(eventId);
        if (savedToken) {
          setToken(savedToken);
          return;
        }

        // Chưa có token: gọi join (không jitter thêm vì đã jitter tại event-action-panel)
        setIsJoining(true);
        setJoinError(null);

        const res = await joinQueue(eventId, {}, { jitter: false });
        if (!cancelled) {
          if (res.state === "SOLD_OUT" || !res.queue_token) {
            setSoldOut(true);
            return;
          }
          saveQueueToken(eventId, res.queue_token);
          setToken(res.queue_token);
        }
      } catch (err) {
        if (!cancelled) {
          if (err instanceof ApiError && err.code === "AUTH_REQUIRED") {
            setJoinError({ kind: "auth" });
          } else if (err instanceof QueueError) {
            if (err.code === "OTP_REQUIRED") {
              setJoinError({ kind: "otp" });
            } else if (err.code === "DEVICE_REQUIRED") {
              setJoinError({ kind: "device" });
            } else if (err.status === 401) {
              setJoinError({ kind: "generic", message: "Bạn cần đăng nhập và xác thực OTP trước khi vào phòng chờ." });
            } else if (err.status === 403) {
              setJoinError({ kind: "generic", message: "Yêu cầu cần xác minh thêm trước khi vào phòng chờ." });
            } else {
              setJoinError({
                kind: "generic",
                message: err.message || "Không thể kết nối vào phòng chờ. Vui lòng thử lại.",
              });
            }
          } else {
            setJoinError({ kind: "generic", message: "Đã xảy ra lỗi mạng khi tham gia phòng chờ. Vui lòng thử lại." });
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
  }, [eventId, joinAttempt]);

  // 2. Theo dõi trạng thái xếp hàng thông qua hook tối ưu
  const { snapshot, initialRank, connection, error } = useQueueStatus({
    eventId,
    token,
  });

  // 401 tu status = queue token khong con hop le (het han 24 gio, hoac cap truoc khi doi sang token ky,
  // EV-137). Vao lai hang dung MOT lan; lan hai van loi thi hien man loi de khach tu quyet dinh.
  useEffect(() => {
    if (error?.status === 401 && error.code !== "AUTH_REQUIRED" && !rejoinedRef.current) {
      rejoinedRef.current = true;
      clearQueueToken(eventId);
      setToken(null);
      setJoinAttempt((n) => n + 1);
    }
  }, [error, eventId]);

  const state = soldOut ? "SOLD_OUT" : (snapshot?.state ?? "LOBBY");

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

  // Phien dang nhap het han (join hoac poll): moi dang nhap lai thay vi bao loi mang (F-11).
  if (joinError?.kind === "auth" || error?.code === "AUTH_REQUIRED") {
    return (
      <div
        role="alert"
        className={cn(
          "flex flex-col items-center justify-center p-xl gap-md rounded-container border border-danger bg-danger-soft text-center",
          className
        )}
      >
        <p className="text-body-md text-fg">{t("waiting.signInRequired")}</p>
        <Link
          href="/auth/sign-in"
          className="ef-focus-ring inline-flex items-center justify-center rounded-control border border-transparent bg-primary px-md py-sm text-label-md text-primary-fg hover:bg-primary-hover"
        >
          {t("checkout.signInAction")}
        </Link>
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
        <span className="text-headline-md font-bold text-danger">
          {joinError.kind === "otp"
            ? t("waiting.otpRequiredTitle")
            : joinError.kind === "device"
              ? t("waiting.deviceRequiredTitle")
              : "Không thể vào phòng chờ"}
        </span>
        <p className="text-body-md text-fg">
          {joinError.kind === "otp"
            ? t("waiting.otpRequiredBody")
            : joinError.kind === "device"
              ? t("waiting.deviceRequiredBody")
              : joinError.message}
        </p>
        {joinError.kind !== "generic" ? (
          <Link
            href="/auth/sign-in"
            className="ef-focus-ring inline-flex items-center justify-center rounded-control border border-transparent bg-primary px-md py-sm text-label-md text-primary-fg hover:bg-primary-hover"
          >
            {joinError.kind === "otp" ? t("waiting.otpRequiredAction") : t("waiting.deviceRequiredAction")}
          </Link>
        ) : null}
        <Button
          variant="secondary"
          onClick={() => {
            clearQueueToken(eventId);
            window.location.reload();
          }}
        >
          Thử lại
        </Button>
      </div>
    );
  }

  // Lỗi terminal từ status (401 đang được tự vào lại hàng ở trên, không nháy màn lỗi)
  if (error && !(error.status === 401 && !rejoinedRef.current) && !isJoining) {
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
            clearQueueToken(eventId);
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
        {state === "SOLD_OUT" ? (
          // BR-Q6: hết vé hẳn thì không còn vị trí để hiển thị; dùng nội dung chuẩn của panel trạng thái.
          <QueueStatusPanel state="SOLD_OUT" connectionState={connection} offsetMs={offsetMs} />
        ) : (
          <QueuePosition
            state={state}
            rank={snapshot?.rank ?? null}
            etaSeconds={snapshot?.etaSeconds ?? null}
            initialRank={initialRank}
          />
        )}
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
