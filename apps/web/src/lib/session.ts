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
}

export type SessionPersist = "local" | "session";

/** Su kien window phat ra khi phien doi trong cung tab (tab khac dung `storage`). */
export const SESSION_EVENT = "ef:session";

const K_ACCESS = "ef_token";
const K_REFRESH = "ef_refresh";
const K_USER = "ef_user";

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
    return { accessToken, refreshToken: s.getItem(K_REFRESH) || null, user };
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
  // Khong de sot ban sao cu o kho con lai.
  wipe(target);
  wipe(other);
  if (target) {
    try {
      target.setItem(K_ACCESS, session.accessToken);
      if (session.refreshToken) target.setItem(K_REFRESH, session.refreshToken);
      target.setItem(K_USER, JSON.stringify(session.user));
    } catch {
      /* day bo nho / bi chan: bo qua */
    }
  }
  notify();
}

export function clearSession(): void {
  wipe(store("local"));
  wipe(store("session"));
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
}

/** Dung Session tu phan hoi identity; `prev` giu lai email khi refresh khong tra email. */
export function sessionFromIdentity(res: IdentityTokens, prev?: SessionUser | null): Session | null {
  const accessToken = res.access_token ?? res.token;
  if (!accessToken) return null;
  const email = res.email ?? prev?.email;
  return {
    accessToken,
    refreshToken: res.refresh_token ?? null,
    user: {
      id: res.id ?? prev?.id ?? "",
      role: (res.role as Role) ?? prev?.role ?? "buyer",
      ...(email ? { email } : {}),
    },
  };
}
