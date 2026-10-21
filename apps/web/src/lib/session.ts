/** Kho phien dang nhap (access + refresh token + user) o trinh duyet (AC-1, AC-7).
 *
 * Khoa dong bang: `ef_token` (access, giu ten cu de tuong thich nguoc), `ef_refresh`, `ef_user` (JSON).
 * Mac dinh ghi localStorage; "ghi nho toi" tat -> sessionStorage (cung khoa, doc local truoc).
 * Token van o web storage (nguoi dung da duyet cho task nay); an toan XSS la no ky thuat da ghi nhan. */

export type Role = "buyer" | "organizer" | "moderator" | "ops";

export interface SessionUser {
  id: string;
  role: Role;
  email?: string;
}

export interface Session {
  accessToken: string;
  refreshToken: string | null;
  user: SessionUser;
  /** Han access token theo DONG HO MAY KHACH luc nhan (Date.now() + expires_in*1000). Thieu -> dung exp cua JWT. */
  expiresAt?: number;
}

export type SessionPersist = "local" | "session";

/** Su kien window phat ra khi phien doi trong cung tab (tab khac dung `storage`). */
export const SESSION_EVENT = "ef:session";

const K_ACCESS = "ef_token";
const K_REFRESH = "ef_refresh";
const K_USER = "ef_user";
const K_EXPIRES = "ef_expires_at";

function store(kind: SessionPersist): Storage | null {
  if (typeof window === "undefined") return null;
  try {
    return kind === "local" ? window.localStorage : window.sessionStorage;
  } catch {
    // Che do rieng tu / cookie bi chan co the nem khi truy cap storage.
    return null;
  }
}

function readFrom(s: Storage | null): Session | null {
  if (!s) return null;
  try {
    const accessToken = s.getItem(K_ACCESS);
    const rawUser = s.getItem(K_USER);
    if (!accessToken || !rawUser) return null;
    const user = JSON.parse(rawUser) as SessionUser | null;
    if (!user || typeof user !== "object" || typeof user.id !== "string" || typeof user.role !== "string") return null;
    const rawExp = Number(s.getItem(K_EXPIRES));
    return {
      accessToken,
      refreshToken: s.getItem(K_REFRESH) || null,
      user,
      ...(Number.isFinite(rawExp) && rawExp > 0 ? { expiresAt: rawExp } : {}),
    };
  } catch {
    return null;
  }
}

function wipe(s: Storage | null) {
  if (!s) return;
  try {
    s.removeItem(K_ACCESS);
    s.removeItem(K_REFRESH);
    s.removeItem(K_USER);
    s.removeItem(K_EXPIRES);
  } catch {
    /* bo qua */
  }
}

function notify() {
  if (typeof window === "undefined") return;
  window.dispatchEvent(new Event(SESSION_EVENT));
}

/** Doc phien hien tai (localStorage truoc, roi sessionStorage); null khi chua dang nhap hoac du lieu hong. */
export function getSession(): Session | null {
  return readFrom(store("local")) ?? readFrom(store("session"));
}

/** Phien dang nam o kho nao (de refresh giu nguyen che do "ghi nho"). */
export function getSessionPersist(): SessionPersist | null {
  if (readFrom(store("local"))) return "local";
  if (readFrom(store("session"))) return "session";
  return null;
}

export function setSession(session: Session, opts: { persist?: SessionPersist } = {}): void {
  const persist = opts.persist ?? "local";
  const target = store(persist);
  const other = store(persist === "local" ? "session" : "local");
  let written = !target;
  if (target) {
    // Ghi gia tri moi TRUOC; neu setItem nem giua chung thi khoi phuc gia tri cu (khong de mat phien).
    const keys = [K_ACCESS, K_REFRESH, K_USER, K_EXPIRES];
    const snapshot = new Map<string, string | null>();
    try {
      for (const k of keys) snapshot.set(k, target.getItem(k));
    } catch {
      /* bo qua */
    }
    try {
      target.setItem(K_ACCESS, session.accessToken);
      if (session.refreshToken) target.setItem(K_REFRESH, session.refreshToken);
      else target.removeItem(K_REFRESH);
      target.setItem(K_USER, JSON.stringify(session.user));
      if (session.expiresAt) target.setItem(K_EXPIRES, String(session.expiresAt));
      else target.removeItem(K_EXPIRES);
      written = true;
    } catch {
      // Day bo nho / bi chan: khoi phuc ban cu, KHONG xoa kho con lai.
      for (const [k, v] of snapshot) {
        try {
          if (v === null) target.removeItem(k);
          else target.setItem(k, v);
        } catch {
          /* bo qua */
        }
      }
    }
  }
  // Chi xoa ban sao cu o kho con lai sau khi ghi thanh cong.
  if (written && target) wipe(other);
  notify();
}

/** Tang moi lan dang xuat/xoa phien: refresh dang bay phai bo ket qua neu epoch doi (F-12). */
let sessionEpoch = 0;
export function getSessionEpoch(): number {
  return sessionEpoch;
}

/** Refresh token tho tu ca hai kho (khong phu thuoc ef_user con nguyen ven) - de logout van thu hoi duoc (F-15). */
export function getStoredRefreshToken(): string | null {
  for (const kind of ["local", "session"] as const) {
    try {
      const v = store(kind)?.getItem(K_REFRESH);
      if (v) return v;
    } catch {
      /* bo qua */
    }
  }
  return null;
}

/** Tien to khoa sessionStorage cua luong mua ve; xoa khi dang xuat de nguoi dung sau khong doc duoc (F-17). */
const BUY_FLOW_PREFIXES = ["ef:queue:token:", "evenflow_hold_", "evenflow_order_", "ef:issue:", "ef:pay:", "ef:hold:"];

export function clearBuyFlowStorage(): void {
  try {
    if (typeof window === "undefined") return;
    const ss = window.sessionStorage;
    for (let i = ss.length - 1; i >= 0; i--) {
      const key = ss.key(i);
      if (key && BUY_FLOW_PREFIXES.some((p) => key.startsWith(p))) ss.removeItem(key);
    }
  } catch {
    /* bo qua */
  }
}

/** Xoa phien. Mac dinh CHI xoa khoa dang nhap (het han/refresh hong khong duoc lam mat khoa idempotency va hold dang
 *  mua - G-4); chi dang xuat chu dong moi truyen `{ buyFlow: true }`. */
export function clearSession(opts: { buyFlow?: boolean } = {}): void {
  sessionEpoch += 1;
  wipe(store("local"));
  wipe(store("session"));
  if (opts.buyFlow) clearBuyFlowStorage();
  notify();
}

/** Phan hoi IdentityResponse (login / refresh / google). `token` la alias cu cua `access_token`. */
export interface IdentityTokens {
  id: string;
  email?: string;
  role: string;
  access_token?: string;
  token?: string;
  refresh_token?: string;
  expires_in?: number;
}

/** Dung Session tu phan hoi identity; `prev` giu lai email khi refresh khong tra email. */
export function sessionFromIdentity(res: IdentityTokens, prev?: SessionUser | null): Session | null {
  const accessToken = res.access_token ?? res.token;
  if (!accessToken) return null;
  const email = res.email ?? prev?.email;
  const expiresAt =
    typeof res.expires_in === "number" && res.expires_in > 0 ? Date.now() + res.expires_in * 1000 : undefined;
  return {
    accessToken,
    ...(expiresAt ? { expiresAt } : {}),
    refreshToken: res.refresh_token ?? null,
    user: {
      id: res.id ?? prev?.id ?? "",
      role: (res.role as Role) ?? prev?.role ?? "buyer",
      ...(email ? { email } : {}),
    },
  };
}
