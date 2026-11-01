"use client";

/** Khu "Điều khiển" của war room (EVF-60, ops-warroom AC-24, AC-25).
 *
 * - Ghi đè admit rate (số nguyên 0..10000), tạm dừng khẩn cấp (có bước xác nhận, gửi rate 0), bỏ ghi đè
 *   (chỉ khi MANUAL). Mọi lệnh bắt buộc lý do 3..200 ký tự; kiểm tra ở client trước khi gọi API.
 * - Không đổi trạng thái lạc quan: chỉ báo trạng thái mới sau khi server xác nhận, rồi làm mới ngay.
 * - Thông báo lỗi là câu cố định phía client, không bao giờ hiện `detail` của server. */

import { useId, useState } from "react";

import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  MAX_OVERRIDE_RATE,
  clearAdmitOverride,
  isValidOverrideRate,
  normalizeOverrideReason,
  setAdmitOverride,
  type AdmitOverrideResult,
  type OpsWarRoomSnapshot,
} from "@/lib/ops-service";

type Command = { kind: "set"; rate: number } | { kind: "pause" } | { kind: "clear" };

type Outcome =
  | { kind: "success"; command: Command; result: AdmitOverrideResult }
  | { kind: "error"; code: string; command: Command; reason: string };

interface FieldErrors {
  rate?: string;
  reason?: string;
}

const RATE_ERROR = `Tốc độ phải là số nguyên từ 0 đến ${MAX_OVERRIDE_RATE}.`;
const REASON_ERROR = "Lý do phải dài từ 3 đến 200 ký tự.";

function parseRate(raw: string): number | null {
  const t = raw.trim();
  if (!/^\d+$/.test(t)) return null;
  const n = Number(t);
  return isValidOverrideRate(n) ? n : null;
}

function errorCode(err: unknown): string {
  const code = (err as { code?: unknown } | null)?.code;
  return typeof code === "string" ? code : "UNAVAILABLE";
}

function successText(command: Command, result: AdmitOverrideResult): string {
  if (!result.active) {
    return "Đã bỏ ghi đè: hàng chờ trở lại chế độ tự động. Tự động có thể hiện 0/s vài giây trước khi tăng lại.";
  }
  if (result.paused) return "Đã tạm dừng khẩn cấp: admit rate 0 khách/giây.";
  const rate = result.ratePerSec ?? (command.kind === "set" ? command.rate : 0);
  return `Đã ghi đè admit rate: ${rate} khách/giây.`;
}

function ErrorOutcome({ code, onRetry, busy }: { code: string; onRetry: () => void; busy: boolean }) {
  if (code === "FORBIDDEN") {
    return (
      <Alert variant="error" title="Không có quyền">
        Tài khoản này không được phép điều khiển admit rate.
      </Alert>
    );
  }
  if (code === "NOT_FOUND") {
    return (
      <Alert variant="error" title="Sự kiện chưa có phòng chờ">
        Chưa thể điều khiển cho tới khi phòng chờ được cấu hình.
      </Alert>
    );
  }
  if (code === "INVALID") {
    return (
      <Alert variant="error" title="Dữ liệu chưa hợp lệ">
        {RATE_ERROR} {REASON_ERROR}
      </Alert>
    );
  }
  return (
    <Alert variant="error" title="Chưa gửi được lệnh">
      <div className="space-y-sm">
        <p>Trạng thái hiện tại không đổi. Kiểm tra kết nối rồi gửi lại.</p>
        <Button variant="secondary" size="sm" onClick={onRetry} disabled={busy}>
          Thử lại
        </Button>
      </div>
    </Alert>
  );
}

export interface OpsControlsPanelProps {
  eventId: string;
  snapshot: OpsWarRoomSnapshot;
  /** Làm mới số liệu ngay sau một lệnh thành công. */
  onApplied: () => void | Promise<void>;
}

export function OpsControlsPanel({ eventId, snapshot, onApplied }: OpsControlsPanelProps) {
  const ids = useId();
  const rateId = `${ids}-rate`;
  const reasonId = `${ids}-reason`;
  const rateErrId = `${ids}-rate-error`;
  const reasonErrId = `${ids}-reason-error`;
  const blockedId = `${ids}-blocked`;

  const [rate, setRate] = useState("");
  const [reason, setReason] = useState("");
  const [errors, setErrors] = useState<FieldErrors>({});
  const [confirmPause, setConfirmPause] = useState(false);
  const [busy, setBusy] = useState(false);
  const [outcome, setOutcome] = useState<Outcome | null>(null);

  const blocked = snapshot.sources.waitingroom === "unavailable";
  const disabled = busy || blocked;
  const manual = snapshot.admission.mode === "MANUAL";

  async function run(command: Command, cleanReason: string) {
    setBusy(true);
    setOutcome(null);
    try {
      const result =
        command.kind === "clear"
          ? await clearAdmitOverride(eventId, cleanReason)
          : await setAdmitOverride(eventId, command.kind === "pause" ? 0 : command.rate, cleanReason);
      setOutcome({ kind: "success", command, result });
      setConfirmPause(false);
      void onApplied();
    } catch (err) {
      const code = errorCode(err);
      if (code === "INVALID") setErrors({ rate: command.kind === "set" ? RATE_ERROR : undefined, reason: REASON_ERROR });
      setOutcome({ kind: "error", code, command, reason: cleanReason });
    } finally {
      setBusy(false);
    }
  }

  function validReason(): string | null {
    const r = normalizeOverrideReason(reason);
    return r;
  }

  function submitOverride() {
    const n = parseRate(rate);
    const r = validReason();
    const next: FieldErrors = { rate: n === null ? RATE_ERROR : undefined, reason: r === null ? REASON_ERROR : undefined };
    setErrors(next);
    if (n === null || r === null) return;
    void run({ kind: "set", rate: n }, r);
  }

  function confirmPauseNow() {
    const r = validReason();
    setErrors({ reason: r === null ? REASON_ERROR : undefined });
    if (r === null) return;
    void run({ kind: "pause" }, r);
  }

  function submitClear() {
    const r = validReason();
    setErrors({ reason: r === null ? REASON_ERROR : undefined });
    if (r === null) return;
    void run({ kind: "clear" }, r);
  }

  function retry() {
    if (outcome?.kind === "error") void run(outcome.command, outcome.reason);
  }

  return (
    <Card className="space-y-md">
      {blocked ? (
        <p id={blockedId} className="text-body-md text-warning-fg">
          Nguồn phòng chờ không khả dụng: tạm khóa điều khiển cho tới khi đọc lại được trạng thái.
        </p>
      ) : null}

      <div className="grid grid-cols-1 gap-md md:grid-cols-2">
        <div className="space-y-xs">
          <label htmlFor={rateId} className="text-body-md font-medium text-fg">
            Tốc độ admit (khách/giây)
          </label>
          <input
            id={rateId}
            type="number"
            inputMode="numeric"
            min={0}
            max={MAX_OVERRIDE_RATE}
            step={1}
            value={rate}
            onChange={(e) => setRate(e.target.value)}
            aria-invalid={errors.rate ? true : undefined}
            aria-describedby={errors.rate ? rateErrId : undefined}
            className="ef-focus-ring w-full rounded-control border border-border bg-surface px-md py-sm text-body-md text-fg"
          />
          {errors.rate ? (
            <p id={rateErrId} role="alert" className="text-body-sm text-danger">
              {errors.rate}
            </p>
          ) : null}
        </div>
        <div className="space-y-xs">
          <label htmlFor={reasonId} className="text-body-md font-medium text-fg">
            Lý do (bắt buộc, 3–200 ký tự)
          </label>
          <input
            id={reasonId}
            type="text"
            maxLength={400}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            aria-invalid={errors.reason ? true : undefined}
            aria-describedby={errors.reason ? reasonErrId : undefined}
            className="ef-focus-ring w-full rounded-control border border-border bg-surface px-md py-sm text-body-md text-fg"
          />
          {errors.reason ? (
            <p id={reasonErrId} role="alert" className="text-body-sm text-danger">
              {errors.reason}
            </p>
          ) : null}
        </div>
      </div>

      <div className="flex flex-wrap gap-sm">
        <Button
          variant="secondary"
          onClick={submitOverride}
          disabled={disabled}
          aria-busy={busy ? true : undefined}
          aria-describedby={blocked ? blockedId : undefined}
        >
          Ghi đè admit rate
        </Button>
        <Button
          variant="destructive"
          onClick={() => setConfirmPause(true)}
          disabled={disabled}
          aria-describedby={blocked ? blockedId : undefined}
        >
          Tạm dừng khẩn cấp
        </Button>
        {manual ? (
          <Button
            variant="tertiary"
            onClick={submitClear}
            disabled={disabled}
            aria-describedby={blocked ? blockedId : undefined}
          >
            Bỏ ghi đè / Tiếp tục tự động
          </Button>
        ) : null}
      </div>

      {confirmPause ? (
        <div className="space-y-sm rounded-control border-2 border-danger p-md">
          <p className="text-body-md font-semibold text-fg">
            Tạm dừng khẩn cấp sẽ đặt admit rate về 0: không khách nào được vào mua thêm cho tới khi bỏ ghi đè.
            Nhập lý do ở trên rồi xác nhận.
          </p>
          <div className="flex flex-wrap gap-sm">
            <Button variant="destructive" onClick={confirmPauseNow} disabled={disabled}>
              Xác nhận tạm dừng
            </Button>
            <Button variant="tertiary" onClick={() => setConfirmPause(false)} disabled={busy}>
              Hủy
            </Button>
          </div>
        </div>
      ) : null}

      {outcome?.kind === "success" ? (
        <div className="space-y-sm">
          <Alert variant="success">{successText(outcome.command, outcome.result)}</Alert>
          {outcome.result.audit === "failed" ? (
            <Alert variant="warning" title="Đã áp dụng nhưng chưa ghi được nhật ký kiểm toán">
              Lệnh vẫn có hiệu lực. Báo cho trực ca để bổ sung nhật ký.
            </Alert>
          ) : null}
        </div>
      ) : null}
      {outcome?.kind === "error" ? <ErrorOutcome code={outcome.code} onRetry={retry} busy={busy} /> : null}
    </Card>
  );
}
