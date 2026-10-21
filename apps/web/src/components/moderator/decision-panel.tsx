"use client";

/** Quyết định của Moderator (EV-121, handoff §10.16, BR-E5).
 *
 * - Từ chối luôn cần lý do; duyệt sự kiện đang REJECTED là lật ngược quyết định trước đó (của AI
 *   hoặc Moderator khác) nên cũng cần lý do. Server kiểm lại cả hai (422) — UI chỉ để báo sớm.
 * - Mọi quyết định đi qua một bước xác nhận; không có thao tác "theo AI" một chạm: AI không phải
 *   người quyết.
 * - 409: Moderator khác vừa quyết -> báo rõ và để trang tải lại hồ sơ. */

import { useId, useState } from "react";

import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { FieldError, FieldHelp } from "@/components/ui/field";
import { ApiError } from "@/lib/api-client";
import {
  MODERATION_PROBLEM,
  approveEvent,
  isDecidable,
  isOverride,
  rejectEvent,
  type ModerationEvent,
} from "@/lib/moderation-service";
import type { EventLifecycleState } from "@/lib/organizer-service";

type Mode = "idle" | "approve" | "reject";

export interface DecisionPanelProps {
  eventId: string;
  status: EventLifecycleState;
  onDecided: (event: ModerationEvent, decision: "approved" | "rejected") => void;
  /** Hồ sơ vừa bị người khác quyết (409): trang tải lại ngầm. */
  onConflict: () => void;
}

export function DecisionPanel({ eventId, status, onDecided, onConflict }: DecisionPanelProps) {
  const [mode, setMode] = useState<Mode>("idle");
  const [text, setText] = useState("");
  const [fieldError, setFieldError] = useState<string | null>(null);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const textId = useId();

  const override = isOverride(status);
  const reasonRequired = mode === "reject" || (mode === "approve" && override);

  if (!isDecidable(status)) {
    return (
      <Alert variant="info" title="Hồ sơ đã được xử lý">
        Sự kiện đang ở trạng thái {status}; không còn quyết định kiểm duyệt nào cần đưa ra.
      </Alert>
    );
  }

  const start = (m: Mode) => {
    setMode(m);
    setText("");
    setFieldError(null);
    setSubmitError(null);
  };

  const submit = async () => {
    const value = text.trim();
    if (reasonRequired && value === "") {
      setFieldError("Cần nhập lý do cho quyết định này.");
      return;
    }
    setFieldError(null);
    setSubmitError(null);
    setSubmitting(true);
    try {
      const evt = mode === "approve" ? await approveEvent(eventId, value) : await rejectEvent(eventId, value);
      onDecided(evt, mode === "approve" ? "approved" : "rejected");
    } catch (err) {
      if (err instanceof ApiError && err.type === MODERATION_PROBLEM.overrideReasonRequired) {
        setFieldError("Lật ngược quyết định trước đó bắt buộc ghi lý do.");
      } else if (err instanceof ApiError && err.status === 409) {
        setSubmitError("Moderator khác vừa quyết định hồ sơ này. Đang tải lại hồ sơ mới nhất.");
        setMode("idle");
        onConflict();
      } else if (err instanceof ApiError && err.status === 403) {
        setSubmitError("Tài khoản này không có quyền Moderator.");
      } else if (err instanceof ApiError && err.status === 422) {
        setFieldError(err.detail ?? "Quyết định không hợp lệ với trạng thái hiện tại của sự kiện.");
      } else {
        setSubmitError(`Không gửi được quyết định: ${err instanceof Error ? err.message : "lỗi không xác định"}.`);
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="space-y-md">
      {submitError ? (
        <Alert variant="error" title="Chưa ghi được quyết định">
          {submitError}
        </Alert>
      ) : null}

      {mode === "idle" ? (
        <div className="flex flex-wrap gap-sm">
          {override ? (
            <Button onClick={() => start("approve")}>Lật ngược — duyệt sự kiện</Button>
          ) : (
            <>
              <Button onClick={() => start("approve")}>Duyệt</Button>
              <Button variant="destructive" onClick={() => start("reject")}>
                Từ chối
              </Button>
            </>
          )}
        </div>
      ) : (
        <Alert
          variant="warning"
          title={
            mode === "reject" ? "Xác nhận từ chối hồ sơ" : override ? "Xác nhận lật ngược quyết định" : "Xác nhận duyệt hồ sơ"
          }
        >
          <div className="space-y-sm">
            <p>
              {mode === "reject"
                ? "Sự kiện sẽ chuyển sang bị từ chối; ban tổ chức thấy lý do và có thể sửa rồi nộp lại."
                : override
                  ? "Sự kiện đang bị từ chối. Duyệt lúc này là lật ngược quyết định trước đó và được ghi nhật ký."
                  : "Sự kiện sẽ được lên lịch mở bán theo cấu hình của ban tổ chức."}
            </p>
            <label htmlFor={textId} className="block text-body-sm font-semibold text-fg">
              {reasonRequired ? "Lý do (bắt buộc)" : "Ghi chú cho quyết định (không bắt buộc)"}
            </label>
            <textarea
              id={textId}
              rows={3}
              value={text}
              onChange={(e) => setText(e.target.value)}
              aria-invalid={fieldError ? true : undefined}
              aria-describedby={fieldError ? `${textId}-error` : `${textId}-help`}
              className="w-full rounded-control border border-border bg-surface p-sm text-body-sm text-fg ef-focus-ring focus:outline-none"
            />
            {fieldError ? (
              <FieldError id={`${textId}-error`}>{fieldError}</FieldError>
            ) : (
              <FieldHelp id={`${textId}-help`}>Ghi vào nhật ký kiểm duyệt cùng tên tài khoản của bạn.</FieldHelp>
            )}
            <div className="flex flex-wrap gap-sm">
              <Button
                variant={mode === "reject" ? "destructive" : "primary"}
                loading={submitting}
                onClick={() => void submit()}
              >
                {mode === "reject" ? "Xác nhận từ chối" : "Xác nhận duyệt"}
              </Button>
              <Button variant="secondary" disabled={submitting} onClick={() => start("idle")}>
                Quay lại
              </Button>
            </div>
          </div>
        </Alert>
      )}
    </div>
  );
}
