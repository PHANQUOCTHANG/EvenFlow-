/** Client API tao hold ve va quan ly khoa idempotency (EVF-113, EV-183, BR-O2, BR-O5).
 *
 * Tuan thu tuyet doi:
 * 1. BR-O5: Bat buoc header `Idempotency-Key` (UUID) cho moi thao tac POST hold.
 * 2. BR-O2: Lay expires_at tu server; client khong tu tinh.
 * 3. BR-O3: 1 hold dang hoat dong tren moi su kien.
 * 4. Error mapping dung ma loi backend Go.
 */

export interface CreateHoldParams {
  eventId: string;
  ticketTypeId: string;
  quantity: number;
  queueToken: string;
  identityId?: string;
  idempotencyKey?: string;
  apiBaseUrl?: string;
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
  | "NOT_ADMITTED" // 403
  | "SOLD_OUT" // 409
  | "LIMIT_EXCEEDED" // 422
  | "AMBIGUOUS" // network failure / timeout / 503 (BR-O5)
  | "UNKNOWN";

export class HoldClientError extends Error {
  readonly type: HoldClientErrorType;
  readonly status?: number;

  constructor(message: string, type: HoldClientErrorType, status?: number) {
    super(message);
    this.name = "HoldClientError";
    this.type = type;
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

/** Goi API backend POST /v1/events/{id}/holds. */
export async function requestHold(params: CreateHoldParams): Promise<HoldResult> {
  const {
    eventId,
    ticketTypeId,
    quantity,
    queueToken,
    identityId = "guest-user",
    idempotencyKey = generateIdempotencyKey(),
    apiBaseUrl = "",
  } = params;

  if (!idempotencyKey) {
    throw new HoldClientError(
      "Thiếu Idempotency-Key bắt buộc (BR-O5)",
      "MISSING_IDEMPOTENCY_KEY",
      400,
    );
  }

  const url = `${apiBaseUrl}/v1/events/${encodeURIComponent(eventId)}/holds`;

  let response: Response;
  try {
    response = await fetch(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Idempotency-Key": idempotencyKey,
        "X-Queue-Token": queueToken,
        "X-Identity-Id": identityId,
      },
      body: JSON.stringify({
        ticket_type_id: ticketTypeId,
        quantity,
      }),
    });
  } catch {
    // BR-O5: Loi mang hoac timeout khi goi API ghi -> khong the xac dinh backend da nhan va ghi chua.
    // Chuyen ve trang thai AMBIGUOUS de bao ve nguoi dung va kho ve.
    throw new HoldClientError(
      "Không nhận được phản hồi từ máy chủ khi tạo hold",
      "AMBIGUOUS",
    );
  }

  if (response.ok) {
    const data = await response.json();
    return {
      orderId: data.order_id,
      holdId: data.hold_id,
      expiresAt: data.expires_at,
      ticketTypeId,
      quantity,
    };
  }

  // Xu ly cac ma loi nghiep vu
  switch (response.status) {
    case 403:
      throw new HoldClientError(
        "Bạn chưa được cấp quyền mua vé, vui lòng quay lại phòng chờ",
        "NOT_ADMITTED",
        403,
      );
    case 409:
      throw new HoldClientError(
        "Rất tiếc, hạng vé này vừa hết",
        "SOLD_OUT",
        409,
      );
    case 422:
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
