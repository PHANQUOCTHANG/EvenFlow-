"use client";

/** Bảng trạng thái kiểm duyệt & lý do AI / Moderator (EV-186, EVF-116, Stitch 10.14).
 *
 * Nghiệp vụ bắt buộc:
 * - BR-E4: Phân tích 4 trục: Nội dung cấm, Sai lệch/lừa đảo, Bản quyền hình ảnh, Thông tin thiếu.
 *          Rủi ro < 0.3: AUTO_APPROVED; 0.3–0.8: NEEDS_HUMAN; >= 0.8: AUTO_REJECTED.
 * - BR-E5: Mọi quyết định ghi audit_log kèm lý do minh bạch, Moderator có thể lật ngược.
 */

import Link from "next/link";
import { useState } from "react";

import { Alert } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { cn } from "@/lib/cn";
import {
  canCancelEvent,
  canEditEvent,
  canOpenEventEditor,
  canSubmitEvent,
  type OrganizerEvent,
} from "@/lib/organizer-service";

import { LIFECYCLE_BADGE_CONFIG } from "./event-list";

export interface ModerationStatusCardProps {
  event: OrganizerEvent;
  /** Nộp duyệt (DRAFT/REJECTED -> PENDING_REVIEW). */
  onResubmit?: (eventId: string) => Promise<void>;
  /** Huỷ sự kiện (cần xác nhận). */
  onCancel?: (eventId: string) => Promise<void>;
  className?: string;
}

export function ModerationStatusCard({
  event,
  onResubmit,
  onCancel,
  className,
}: ModerationStatusCardProps) {
  const [isResubmitting, setIsResubmitting] = useState(false);
  const [resubmitSuccess, setResubmitSuccess] = useState(false);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [confirmingCancel, setConfirmingCancel] = useState(false);
  const [isCancelling, setIsCancelling] = useState(false);
  const [cancelError, setCancelError] = useState<string | null>(null);

  const moderation = event.moderation;
  const isRejected = event.state === "REJECTED";
  const isPending = event.state === "PENDING_REVIEW";
  const isApproved = event.state === "SCHEDULED" || event.state === "ON_SALE";

  const handleResubmit = async () => {
    if (!onResubmit) return;
    setIsResubmitting(true);
    setErrorMsg(null);
    setResubmitSuccess(false);

    try {
      await onResubmit(event.id);
      setResubmitSuccess(true);
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setIsResubmitting(false);
    }
  };

  const handleCancel = async () => {
    if (!onCancel) return;
    setIsCancelling(true);
    setCancelError(null);
    try {
      await onCancel(event.id);
      setConfirmingCancel(false);
    } catch (err) {
      setCancelError(err instanceof Error ? err.message : String(err));
    } finally {
      setIsCancelling(false);
    }
  };

  const statusBadge = LIFECYCLE_BADGE_CONFIG[event.state] ?? { label: event.state, variant: "neutral" as const };

  return (
    <div className={cn("space-y-lg max-w-4xl mx-auto", className)}>
      {/* Thẻ trạng thái chính */}
      <Card className="p-lg space-y-md">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-md border-b border-border pb-md">
          <div>
            <span className="text-body-xs text-fg-muted uppercase tracking-wider font-semibold">
              Trạng thái kiểm duyệt
            </span>
            <h1 className="text-heading-lg font-bold text-fg mt-xs">{event.title}</h1>
            <p className="text-body-sm text-fg-muted mt-xs">
              📍 {event.venue} • Mở bán: {new Date(event.saleStartAt).toLocaleString("vi-VN")}
            </p>
          </div>

          <div className="flex items-center gap-sm">
            {isApproved && (
              <Badge variant="admitted" className="text-body-sm px-md py-xs">
                ✅ Đã duyệt — Sẵn sàng mở bán
              </Badge>
            )}
            {isPending && (
              <Badge variant="pending" className="text-body-sm px-md py-xs">
                ⏳ Đang kiểm duyệt (NEEDS_HUMAN)
              </Badge>
            )}
            {isRejected && (
              <Badge variant="failed" className="text-body-sm px-md py-xs">
                ❌ Bị từ chối kiểm duyệt
              </Badge>
            )}
            {event.state === "DRAFT" && (
              <Badge variant="neutral" className="text-body-sm px-md py-xs">
                📝 Bản nháp
              </Badge>
            )}
            {!isApproved && !isPending && !isRejected && event.state !== "DRAFT" && (
              <Badge variant={statusBadge.variant} className="text-body-sm px-md py-xs">
                {statusBadge.label}
              </Badge>
            )}
          </div>
        </div>

        <p className="text-body-sm text-fg-muted">
          Trạng thái thực từ hệ thống:{" "}
          <strong className="text-fg" data-testid="real-status">
            {event.state}
          </strong>
        </p>

        {/* Thông báo bối cảnh */}
        {isRejected && (
          <Alert variant="error" title="Sự kiện không đạt tiêu chuẩn kiểm duyệt">
            Sự kiện đã bị từ chối kiểm duyệt. Vui lòng chỉnh sửa nội dung và nộp duyệt lại.
          </Alert>
        )}

        {isPending && (
          <Alert variant="warning" title="Đang trong hàng đợi kiểm duyệt thủ công">
            Sự kiện đang chờ kiểm duyệt (PENDING_REVIEW). Bạn sẽ thấy trạng thái cập nhật ở đây khi có kết quả.
          </Alert>
        )}

        {isApproved && (
          <Alert variant="success" title="Sự kiện đã được phê duyệt">
            Sự kiện đã vượt qua kiểm duyệt và sẵn sàng mở bán đúng thời điểm T0.
          </Alert>
        )}

        {resubmitSuccess && (
          <Alert variant="success" title="Đã gửi duyệt lại thành công">
            Hồ sơ cập nhật của sự kiện đã được gửi tới hệ thống kiểm duyệt.
          </Alert>
        )}

        {errorMsg && (
          <Alert variant="error" title="Lỗi gửi duyệt lại">
            {errorMsg}
          </Alert>
        )}

        {/* Chi tiết kiểm duyệt AI: backend chưa cung cấp cho organizer (contracts mục 7) */}
        <Alert variant="info" title="Chi tiết kiểm duyệt AI: dữ liệu mẫu / chưa khả dụng">
          {moderation
            ? "Điểm rủi ro và 4 trục bên dưới là dữ liệu mẫu, chưa nối với nguồn thật. Không dùng để đánh giá kết quả duyệt."
            : "Hệ thống chưa cung cấp điểm rủi ro và phân tích 4 trục cho Ban tổ chức. Chỉ trạng thái duyệt ở trên là dữ liệu thật."}
        </Alert>

        {/* Chi tiết điểm rủi ro tổng hợp */}
        {moderation && (
          <div className="bg-surface-elevated p-md rounded-control space-y-sm">
            <div className="flex items-center justify-between text-body-sm font-semibold">
              <span className="text-fg">Điểm rủi ro tổng hợp (AI Risk Score):</span>
              <span
                className={cn(
                  "text-heading-sm font-bold",
                  moderation.overallRiskScore >= 0.8
                    ? "text-danger"
                    : moderation.overallRiskScore >= 0.3
                      ? "text-warning-fg"
                      : "text-success",
                )}
              >
                {(moderation.overallRiskScore * 100).toFixed(0)}% ({moderation.overallRiskScore} / 1.0)
              </span>
            </div>

            {/* Thanh tiến trình rủi ro */}
            <div
              className="w-full bg-neutral-soft rounded-full h-3 overflow-hidden"
              role="progressbar"
              aria-valuenow={Math.round(moderation.overallRiskScore * 100)}
              aria-valuemin={0}
              aria-valuemax={100}
              aria-label="Thanh tiến trình điểm rủi ro"
            >
              <div
                className={cn(
                  "h-full transition-all duration-500",
                  moderation.overallRiskScore >= 0.8
                    ? "bg-danger"
                    : moderation.overallRiskScore >= 0.3
                      ? "bg-warning"
                      : "bg-success",
                )}
                style={{ width: `${Math.min(100, Math.round(moderation.overallRiskScore * 100))}%` }}
              />
            </div>

            <div className="flex justify-between text-body-xs text-fg-muted pt-xs">
              <span>0% (An toàn - AUTO_APPROVED)</span>
              <span>30% (Cần Moderator rà soát)</span>
              <span>80% (Nguy hiểm - AUTO_REJECTED)</span>
            </div>
          </div>
        )}

        {/* Phân tích 4 trục AI kiểm duyệt (BR-E4) */}
        {moderation?.axes && (
          <div className="space-y-sm pt-sm">
            <h3 className="text-heading-sm font-semibold text-fg">
              Phân tích 4 trục kiểm duyệt tự động (BR-E4)
            </h3>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-sm" role="table" aria-label="Bảng 4 trục kiểm duyệt">
              {moderation.axes.map((axis) => (
                <div
                  key={axis.axis}
                  className={cn(
                    "p-md rounded-control border space-y-xs",
                    axis.passed
                      ? "border-success/40 bg-success-soft/20"
                      : "border-danger/40 bg-danger-soft/20",
                  )}
                >
                  <div className="flex items-center justify-between">
                    <span className="font-semibold text-body-sm text-fg">{axis.title}</span>
                    <Badge variant={axis.passed ? "admitted" : "failed"}>
                      {axis.passed ? "Đạt" : "Cảnh báo"}
                    </Badge>
                  </div>
                  <p className="text-body-xs text-fg-muted">
                    Điểm rủi ro: {(axis.riskScore * 100).toFixed(0)}%
                  </p>
                  {axis.notes && (
                    <p className="text-body-xs text-danger font-medium mt-xs">
                      💬 {axis.notes}
                    </p>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Lý do chi tiết và Audit Log (BR-E5) */}
        {moderation?.reasons && moderation.reasons.length > 0 && (
          <div className="space-y-xs pt-sm">
            <h4 className="text-body-sm font-semibold text-fg">Ghi chú &amp; Lý do thẩm định:</h4>
            <ul className="list-disc list-inside space-y-xs text-body-sm text-fg-muted bg-surface-elevated p-md rounded-control">
              {moderation.reasons.map((reason, idx) => (
                <li key={idx}>{reason}</li>
              ))}
              {moderation.moderatorNotes && (
                <li className="text-warning-fg font-medium">
                  Ghi chú kiểm duyệt viên: {moderation.moderatorNotes}
                </li>
              )}
            </ul>
          </div>
        )}

        {confirmingCancel && (
          <Alert variant="warning" title="Xác nhận huỷ sự kiện">
            <p>Huỷ sự kiện &ldquo;{event.title}&rdquo;? Hành động này không thể hoàn tác.</p>
            <div className="mt-sm flex gap-sm">
              <Button variant="destructive" onClick={handleCancel} disabled={isCancelling}>
                {isCancelling ? "Đang huỷ..." : "Xác nhận huỷ"}
              </Button>
              <Button
                variant="secondary"
                onClick={() => {
                  setConfirmingCancel(false);
                  setCancelError(null);
                }}
                disabled={isCancelling}
              >
                Giữ lại sự kiện
              </Button>
            </div>
          </Alert>
        )}

        {cancelError && (
          <Alert variant="error" title="Lỗi huỷ sự kiện">
            {cancelError}
          </Alert>
        )}

        {/* Các nút hành động */}
        <div className="flex flex-wrap items-center justify-between gap-sm pt-md border-t border-border">
          <Link href="/organizer/events">
            <Button variant="secondary">← Danh sách sự kiện</Button>
          </Link>

          <div className="flex gap-sm">
            {canOpenEventEditor(event.state) && (
              <Link href={`/organizer/events/${event.id}/edit`}>
                <Button variant="secondary">
                  {canEditEvent(event.state) ? "✏️ Chỉnh sửa thông tin" : "✏️ Tăng số lượng vé"}
                </Button>
              </Link>
            )}

            {onCancel && canCancelEvent(event.state) && !confirmingCancel && (
              <Button variant="destructive" onClick={() => setConfirmingCancel(true)}>
                Huỷ sự kiện
              </Button>
            )}

            {onResubmit && canSubmitEvent(event.state) && (
              <Button
                variant="primary"
                onClick={handleResubmit}
                disabled={isResubmitting}
              >
                {isResubmitting ? "Đang gửi..." : "🔄 Nộp duyệt lại"}
              </Button>
            )}
          </div>
        </div>
      </Card>
    </div>
  );
}
