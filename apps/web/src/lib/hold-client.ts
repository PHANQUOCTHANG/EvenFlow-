/** Client API tao hold ve va quan ly khoa idempotency (EVF-113, EV-183, BR-O2, BR-O5).
 *
 * Tuan thu tuyet doi:
 * 1. BR-O5: Bat buoc header `Idempotency-Key` (UUID) cho moi thao tac POST hold.
 * 2. BR-O2: Lay expires_at tu server; client khong tu tinh.
 * 3. BR-O3: 1 hold dang hoat dong tren moi su kien.
 * 4. Error mapping dung ma loi backend Go.
 */

import { ApiError, apiFetch, readProblem } from "@/lib/api-client";
import {
  forgetKey,
  IDEMPOTENCY_IN_PROGRESS,
  IN_PROGRESS_MAX_ATTEMPTS,
  isDefinitiveStatus,
  retryAfterMs,
  sleep,
  stableKey,
} from "@/lib/idempotency";

export interface CreateHoldParams {
  eventId: string;
  ticketTypeId: string;
  quantity: number;
  queueToken: string;
  idempotencyKey?: string;
}

export interface HoldResult {
  orderId: string;
  holdId: string;
  expiresAt: string; // ISO-8601 string from server (BR-O2)
  ticketTypeId: string;
  quantity: number;
}

export type HoldClientErrorType =
  | "MISSING_IDEMPOTENCY_KEY"
  | "NOT_ADMITTED" // 403 (chua duoc admit)
  | "FORBIDDEN" // 403 /problems/forbidden: khong du quyen (khac chua duoc admit)
  | "QUEUE_TOKEN_INVALID" // 401/403 queue-token-expired|invalid|mismatch: quay lai phong cho (AC-10)
  | "SOLD_OUT" // 409
  | "PER_IDENTITY_LIMIT" // 422 /problems/per-identity-limit
  | "QUANTITY_NOT_ALLOWED" // 422 /problems/quantity-not-allowed
  | "LIMIT_EXCEEDED" // 422 khac
  | "AUTH_REQUIRED" // phien het han, refresh that bai
  | "AMBIGUOUS" // network failure / timeout / 503 (BR-O5)
  | "UNKNOWN";

export class HoldClientError extends Error {
  readonly type: HoldClientErrorType;
  /** Cung gia tri voi `type`, de goi chung voi ApiError (`code`). */
  readonly code: HoldClientErrorType;
  readonly status?: number;

  constructor(message: string, type: HoldClientErrorType, status?: number) {
    super(message);
    this.name = "HoldClientError";
    this.type = type;
    this.code = type;
    this.status = status;
  }
}

/** Sinh Idempotency-Key UUID v4 an toan cho ca browser va Node environment. */
export function generateIdempotencyKey(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  // Fallback RFC4122 v4 UUID
  return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    const v = c === "x" ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
}

const HOLD_STORAGE_PREFIX = "evenflow_hold_";

/** Luu active hold vao sessionStorage cho mot su kien. */
export function saveActiveHold(eventId: string, hold: HoldResult): void {
  if (typeof window === "undefined" || !window.sessionStorage) return;
  try {
    sessionStorage.setItem(`${HOLD_STORAGE_PREFIX}${eventId}`, JSON.stringify(hold));
  } catch {
    // sessionStorage day hoac bi chan
  }
}

/** Lay active hold tu sessionStorage cho mot su kien. */
export function getActiveHold(eventId: string): HoldResult | null {
  if (typeof window === "undefined" || !window.sessionStorage) return null;
  try {
    const raw = sessionStorage.getItem(`${HOLD_STORAGE_PREFIX}${eventId}`);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as HoldResult;
    if (parsed && parsed.orderId && parsed.holdId && parsed.expiresAt) {
      return parsed;
    }
    return null;
  } catch {
    return null;
  }
}

/** Xoa active hold khoi sessionStorage khi het han hoac hoan tat thanh toan. */
export function clearActiveHold(eventId: string): void {
  if (typeof window === "undefined" || !window.sessionStorage) return;
  try {
    sessionStorage.removeItem(`${HOLD_STORAGE_PREFIX}${eventId}`);
  } catch {
    // ignore
  }
}

/** Khoa Idempotency-Key on dinh theo lan thu (event + hang ve + so luong): giu qua cac lan nguoi dung thu lai
 *  cho toi khi thanh cong hoac gap loi dinh doat (F-7). */
const HOLD_KEY_PREFIX = "ef:hold:";
function holdKeyStorage(p: { eventId: string; ticketTypeId: string; quantity: number }): string {
  return `${HOLD_KEY_PREFIX}${p.eventId}:${p.ticketTypeId}:${p.quantity}`;
}

const QUEUE_TOKEN_PROBLEMS = new Set([
  "/problems/queue-token-expired",
  "/problems/queue-token-invalid",
  "/problems/queue-token-mismatch",
]);

/** Goi API backend POST /v1/events/{id}/holds. Di qua apiFetch: Bearer tu phien, khong cookie, khong header danh tinh gia. */
export async function requestHold(params: CreateHoldParams): Promise<HoldResult> {
  const { eventId, ticketTypeId, quantity, queueToken } = params;
  const keyStorage = holdKeyStorage(params);
  const callerKey = params.idempotencyKey;
  const idempotencyKey = callerKey ?? stableKey(keyStorage);

  if (!idempotencyKey) {
    throw new HoldClientError(
      "Thiếu Idempotency-Key bắt buộc (BR-O5)",
      "MISSING_IDEMPOTENCY_KEY",
      400,
    );
  }

  let response: Response;
  let attempt = 0;
  for (;;) {
    attempt += 1;
    try {
      response = await apiFetch(`/v1/events/${encodeURIComponent(eventId)}/holds`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Idempotency-Key": idempotencyKey,
          "X-Queue-Token": queueToken,
        },
        body: JSON.stringify({
          ticket_type_id: ticketTypeId,
          quantity,
        }),
      });
    } catch (err) {
      if (err instanceof ApiError && err.code === "AUTH_REQUIRED") {
        throw new HoldClientError("Phiên đăng nhập đã hết hạn", "AUTH_REQUIRED", 401);
      }
      // BR-O5: Loi mang hoac timeout khi goi API ghi -> khong the xac dinh backend da nhan va ghi chua.
      // Giu nguyen khoa de lan thu lai la idempotent.
      throw new HoldClientError(
        "Không nhận được phản hồi từ máy chủ khi tạo hold",
        "AMBIGUOUS",
      );
    }
    // 409 "cung khoa dang duoc xu ly": cho Retry-After roi gui lai CUNG khoa (toi da 3 lan gui).
    if (response.status === 409 && attempt < IN_PROGRESS_MAX_ATTEMPTS) {
      if ((await readProblem(response)).type === IDEMPOTENCY_IN_PROGRESS) {
        await sleep(retryAfterMs(response));
        continue;
      }
    }
    break;
  }

  if (!callerKey) {
    if (response.ok || isDefinitiveStatus(response.status)) {
      if (!(response.status === 409 && (await readProblem(response)).type === IDEMPOTENCY_IN_PROGRESS)) {
        forgetKey(keyStorage);
      }
    }
  }

  // 200 (theo contract) va 201 deu la thanh cong.
  if (response.status === 200 || response.status === 201) {
    const data = await response.json();
    return {
      orderId: data.order_id,
      holdId: data.hold_id,
      expiresAt: data.expires_at,
      ticketTypeId,
      quantity,
    };
  }

  const problemType = (await readProblem(response)).type;

  // Queue token het han/sai: khong phai loi dang nhap -> UI dua nguoi dung ve phong cho (AC-10).
  if ((response.status === 401 || response.status === 403) && problemType && QUEUE_TOKEN_PROBLEMS.has(problemType)) {
    throw new HoldClientError(
      "Mã xếp hàng không còn hiệu lực, vui lòng quay lại phòng chờ",
      "QUEUE_TOKEN_INVALID",
      response.status,
    );
  }

  // 401 khong phai cua queue token (khong refresh duoc / retry van 401): can dang nhap lai.
  if (response.status === 401) {
    throw new HoldClientError("Phiên đăng nhập đã hết hạn", "AUTH_REQUIRED", 401);
  }

  switch (response.status) {
    case 403:
      if (problemType === "/problems/forbidden") {
        throw new HoldClientError("Tài khoản của bạn không có quyền thực hiện thao tác này", "FORBIDDEN", 403);
      }
      throw new HoldClientError(
        "Bạn chưa được cấp quyền mua vé, vui lòng quay lại phòng chờ",
        "NOT_ADMITTED",
        403,
      );
    case 409: {
      // Hai loai 409: "het ve" (nghiep vu) va "cung Idempotency-Key dang duoc xu ly" (BR-O5, vd khach
      // bam dup). Loai sau KHONG phai het ve: yeu cau dau co the van thanh cong.
      if (problemType === IDEMPOTENCY_IN_PROGRESS) {
        throw new HoldClientError(
          "Yêu cầu giữ vé đang được xử lý, vui lòng chờ giây lát",
          "AMBIGUOUS",
          409,
        );
      }
      throw new HoldClientError(
        "Rất tiếc, hạng vé này vừa hết",
        "SOLD_OUT",
        409,
      );
    }
    case 422:
      if (problemType === "/problems/per-identity-limit") {
        throw new HoldClientError("Bạn đã đạt giới hạn số vé cho sự kiện này", "PER_IDENTITY_LIMIT", 422);
      }
      if (problemType === "/problems/quantity-not-allowed") {
        throw new HoldClientError("Số lượng vé không được phép cho một đơn", "QUANTITY_NOT_ALLOWED", 422);
      }
      throw new HoldClientError(
        "Số lượng vé vượt quá giới hạn cho phép",
        "LIMIT_EXCEEDED",
        422,
      );
    case 503:
    case 502:
    case 504:
      // He thong ban / timeout trung gian -> AMBIGUOUS
      throw new HoldClientError(
        "Máy chủ đang bận xử lý hoặc phản hồi chậm",
        "AMBIGUOUS",
        response.status,
      );
    default:
      throw new HoldClientError(
        `Lỗi hệ thống khi giữ vé (HTTP ${response.status})`,
        "UNKNOWN",
        response.status,
      );
  }
}

/** Huy hold dang giu (DELETE /v1/holds/{holdId}, AC-11). 204 va 404 deu coi la xong va xoa hold luu cuc bo. */
export async function cancelHold(holdId: string): Promise<void> {
  let response: Response;
  try {
    response = await apiFetch(`/v1/holds/${encodeURIComponent(holdId)}`, {
      method: "DELETE",
      headers: { "Idempotency-Key": generateIdempotencyKey() },
    });
  } catch (err) {
    if (err instanceof ApiError && err.code === "AUTH_REQUIRED") {
      throw new HoldClientError("Phiên đăng nhập đã hết hạn", "AUTH_REQUIRED", 401);
    }
    throw new HoldClientError("Không nhận được phản hồi từ máy chủ khi huỷ giữ vé", "AMBIGUOUS");
  }

  if (response.status === 204 || response.status === 404 || response.ok) {
    clearActiveHoldById(holdId);
    return;
  }
  if (response.status === 401) {
    throw new HoldClientError("Phiên đăng nhập đã hết hạn", "AUTH_REQUIRED", 401);
  }
  throw new HoldClientError(`Không thể huỷ giữ vé (HTTP ${response.status})`, "UNKNOWN", response.status);
}

/** Xoa hold luu cuc bo co holdId khop (khong dung toi hold cua su kien khac). */
function clearActiveHoldById(holdId: string): void {
  if (typeof window === "undefined" || !window.sessionStorage) return;
  try {
    for (let i = sessionStorage.length - 1; i >= 0; i--) {
      const key = sessionStorage.key(i);
      if (!key || !key.startsWith(HOLD_STORAGE_PREFIX)) continue;
      const parsed = JSON.parse(sessionStorage.getItem(key) ?? "null") as { holdId?: string } | null;
      if (parsed?.holdId === holdId) sessionStorage.removeItem(key);
    }
  } catch {
    // bo qua loi storage
  }
}
