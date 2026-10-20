/** Dinh danh thiet bi gui kem khi dang nhap (EV-113 / EV-137).
 *
 * Backend gan phien dang nhap voi mot thiet bi va rang buoc queue token voi thiet bi do: chia queue
 * token cho may khac thi khong dung duoc. Gia tri nay do client tu sinh (UUID ngau nhien, giu o
 * localStorage) -- la tin hieu cho antibot, khong phai bang chung. Backend chi luu HMAC cua no. */

const KEY = "ef:device-id";

function storage(): Storage | null {
  try {
    return typeof window === "undefined" ? null : (window.localStorage ?? null);
  } catch {
    // localStorage co the nem (che do rieng tu, cookie bi chan).
    return null;
  }
}

function randomId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  // Trinh duyet cu: du dai (>= 8 ky tu) de backend chap nhan, du ngau nhien cho muc dich phan biet may.
  return `dev-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`;
}

let memoryId: string | null = null;

/** Tra id thiet bi on dinh qua cac lan dang nhap; neu khong luu duoc thi on dinh trong phien tab. */
export function getDeviceFingerprint(): string {
  const store = storage();
  try {
    const existing = store?.getItem(KEY);
    if (existing && existing.length >= 8) return existing;
    const id = randomId();
    store?.setItem(KEY, id);
    memoryId = id;
    return id;
  } catch {
    memoryId ??= randomId();
    return memoryId;
  }
}
