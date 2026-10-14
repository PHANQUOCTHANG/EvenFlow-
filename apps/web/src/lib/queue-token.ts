/** Queue token cua phong cho trong sessionStorage (EV-198, B1).
 *
 * Phong cho ghi token, trang giu ve doc lai token do de gui `X-Queue-Token`. Truoc day moi ben
 * tu dat khoa rieng (`ef:queue:token:` va `evenflow_queue_token_`) nen khach DA duoc goi van bi
 * bao "chua duoc cap quyen mua ve". Moi noi doc/ghi token phai di qua module nay.
 *
 * Giu nguyen khoa cu cua phong cho: doi khoa thi khach dang trong hang mat token khi deploy. */

const PREFIX = "ef:queue:token:";

export function queueTokenKey(eventId: string): string {
  return `${PREFIX}${eventId}`;
}

function storage(): Storage | null {
  try {
    return typeof window === "undefined" ? null : (window.sessionStorage ?? null);
  } catch {
    // Truy cap sessionStorage co the nem (third-party cookie bi chan, sandbox iframe).
    return null;
  }
}

export function readQueueToken(eventId: string): string | null {
  try {
    return storage()?.getItem(queueTokenKey(eventId)) ?? null;
  } catch {
    return null;
  }
}

export function saveQueueToken(eventId: string, token: string): void {
  try {
    storage()?.setItem(queueTokenKey(eventId), token);
  } catch {
    // Storage day hoac bi chan: token van con trong state cua phong cho.
  }
}

export function clearQueueToken(eventId: string): void {
  try {
    storage()?.removeItem(queueTokenKey(eventId));
  } catch {
    // bo qua
  }
}
