/** Thu tin hieu hanh vi cho anti-bot (EV-173, antibot-phase1 contracts 1, payload v1).
 *
 * Collector chay tren trang su kien (noi khach bam "vao phong cho"): chi dem su kien, giu khoang cach thoi gian
 * giua cac su kien ROI RAC (pointerdown, keydown, touchstart, scroll dau tien cua mot dot), KHONG giu gia tri
 * phim, toa do hay van ban. Luc bam join, snapshot v1 duoc ghi vao sessionStorage `ef:ab:{eventId}`; phong cho
 * gui no khi join va moi lan join lai, va chi xoa khi khach roi su kien (checkout xong, het ve, roi hang).
 *
 * Tin hieu do client gui nen khong dang tin (Q-1): server van con R1-server, R4D, R4N, R5-UA, R7 va co chan. */

import { getDeviceFingerprint } from "./device";

const STORAGE_PREFIX = "ef:ab:";
const MAX_COUNT = 10000;
const MAX_ITEMS = 32;
const MAX_INTERVAL_MS = 60000;
const MAX_RT_MS = 600000;
const SAMPLE_MS = 100; // pointermove / scroll dem toi da 1 lan moi 100 ms
const SCROLL_BURST_GAP_MS = 500; // scroll dau tien sau >= 500 ms yen lang la su kien roi rac
const BUCKET_MS = 50;
const MAX_BUCKET = 40;
const RT_BUCKET_MS = 250;
const SQ_MIN_EVENTS = 12;
const SQ_MIN_BUCKETS = 4;
const TAP_DEDUP_MS = 50; // mot cu cham sinh ca pointerdown va touchstart: chi tinh mot

/** Snapshot v1 (contracts 1). Moi so la so nguyen. */
export interface SignalSnapshot {
  v: 1;
  rt: number;
  pe: number;
  se: number;
  ke: number;
  iv: number[];
  sq?: string;
  sn?: number;
  sb?: number;
  fp?: string;
  wd: boolean;
  gl: boolean;
  fc: number;
  tz?: string;
  tzo: number;
}

type Kind = "p" | "k" | "s";

interface Discrete {
  kind: Kind;
  bucket: number;
}

interface CollectorState {
  start: number;
  pe: number;
  se: number;
  ke: number;
  lastDiscreteAt: number | null;
  iv: number[];
  events: Discrete[];
  totalDiscrete: number;
  lastMoveSampleAt: number | null;
  lastScrollSampleAt: number | null;
  lastScrollAt: number | null;
  lastPointerDownAt: number | null;
  lastTouchStartAt: number | null;
}

function now(): number {
  return typeof performance !== "undefined" && typeof performance.now === "function" ? performance.now() : Date.now();
}

function freshState(): CollectorState {
  return {
    start: now(),
    pe: 0,
    se: 0,
    ke: 0,
    lastDiscreteAt: null,
    iv: [],
    events: [],
    totalDiscrete: 0,
    lastMoveSampleAt: null,
    lastScrollSampleAt: null,
    lastScrollAt: null,
    lastPointerDownAt: null,
    lastTouchStartAt: null,
  };
}

let state: CollectorState = freshState();

const clamp = (n: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, n));
const inc = (n: number) => Math.min(MAX_COUNT, n + 1);

function pushCapped<T>(list: T[], item: T) {
  list.push(item);
  if (list.length > MAX_ITEMS) list.splice(0, list.length - MAX_ITEMS);
}

function recordDiscrete(kind: Kind, t: number) {
  const s = state;
  const since = s.lastDiscreteAt ?? s.start;
  const interval = clamp(Math.round(t - since), 0, MAX_INTERVAL_MS);
  if (s.lastDiscreteAt !== null) pushCapped(s.iv, interval);
  pushCapped(s.events, { kind, bucket: Math.min(MAX_BUCKET, Math.floor(interval / BUCKET_MS)) });
  s.totalDiscrete += 1;
  s.lastDiscreteAt = t;
}

function onPointerDown() {
  const t = now();
  if (state.lastTouchStartAt !== null && t - state.lastTouchStartAt < TAP_DEDUP_MS) return;
  state.lastPointerDownAt = t;
  state.pe = inc(state.pe);
  recordDiscrete("p", t);
}

function onTouchStart() {
  const t = now();
  if (state.lastPointerDownAt !== null && t - state.lastPointerDownAt < TAP_DEDUP_MS) return;
  state.lastTouchStartAt = t;
  state.pe = inc(state.pe);
  recordDiscrete("p", t);
}

function onPointerMove() {
  const t = now();
  if (state.lastMoveSampleAt !== null && t - state.lastMoveSampleAt < SAMPLE_MS) return;
  state.lastMoveSampleAt = t;
  state.pe = inc(state.pe);
}

function onKeyDown(e?: Event) {
  state.ke = inc(state.ke);
  // X-8 / R-7: OS auto-repeat of a held key fires at a fixed ~30 ms rhythm; it is not a discrete human action
  // and would fill iv with machine-regular intervals (R1). Count it, but keep it out of iv / sq.
  if (e && (e as KeyboardEvent).repeat === true) return;
  recordDiscrete("k", now());
}

function onScroll() {
  const t = now();
  const burstStart = state.lastScrollAt === null || t - state.lastScrollAt >= SCROLL_BURST_GAP_MS;
  state.lastScrollAt = t;
  if (burstStart) recordDiscrete("s", t);
  if (state.lastScrollSampleAt !== null && t - state.lastScrollSampleAt < SAMPLE_MS) return;
  state.lastScrollSampleAt = t;
  state.se = inc(state.se);
}

const LISTENERS: Array<[string, (e?: Event) => void]> = [
  ["pointerdown", onPointerDown],
  ["touchstart", onTouchStart],
  ["pointermove", onPointerMove],
  ["keydown", onKeyDown],
  ["scroll", onScroll],
];

/** Bat dau thu tin hieu (listener passive tren window). Tra ham don dep. */
export function startSignalCollector(eventId: string): () => void {
  void eventId; // snapshot gan voi event luc bam join; collector chi do mot trang
  state = freshState();
  if (typeof window === "undefined") return () => {};
  const opts: AddEventListenerOptions = { passive: true, capture: true };
  for (const [type, fn] of LISTENERS) window.addEventListener(type, fn, opts);
  return () => {
    for (const [type, fn] of LISTENERS) window.removeEventListener(type, fn, opts);
  };
}

// ---- probes (mot lan moi trang) ------------------------------------------------------------------------------

const FONT_LIST = [
  "Arial", "Arial Black", "Calibri", "Cambria", "Comic Sans MS", "Consolas", "Courier New", "Georgia",
  "Helvetica", "Impact", "Lucida Console", "Palatino Linotype", "Segoe UI", "Tahoma", "Times New Roman",
  "Trebuchet MS", "Verdana", "Menlo", "Roboto", "Ubuntu",
];

interface Probes {
  gl: boolean;
  fc: number;
}

let probes: Probes | null = null;

function runProbes(): Probes {
  if (probes) return probes;
  let gl = false;
  let fc = 0;
  try {
    if (typeof document !== "undefined") {
      const c = document.createElement("canvas");
      gl = Boolean(c.getContext("webgl") ?? c.getContext("experimental-webgl"));
      const ctx = document.createElement("canvas").getContext("2d");
      if (ctx) {
        const text = "mmmmmmmmmmlli1WQ";
        const base = ["monospace", "serif", "sans-serif"].map((b) => {
          ctx.font = `72px ${b}`;
          return ctx.measureText(text).width;
        });
        for (const f of FONT_LIST) {
          const found = ["monospace", "serif", "sans-serif"].some((b, i) => {
            ctx.font = `72px "${f}", ${b}`;
            return ctx.measureText(text).width !== base[i];
          });
          if (found) fc += 1;
        }
      }
    }
  } catch {
    // trinh duyet chan canvas: giu gia tri mac dinh
  }
  probes = { gl, fc: clamp(fc, 0, 20) };
  return probes;
}

// ---- digest --------------------------------------------------------------------------------------------------

const FNV_OFFSET = 0xcbf29ce484222325n;
const FNV_PRIME = 0x100000001b3n;
const MASK64 = 0xffffffffffffffffn;

function fnv1a64(s: string): string {
  let h = FNV_OFFSET;
  for (let i = 0; i < s.length; i++) {
    h ^= BigInt(s.charCodeAt(i) & 0xff);
    h = (h * FNV_PRIME) & MASK64;
  }
  return h.toString(16).padStart(16, "0");
}

const TZ_RE = /^[A-Za-z_+\-/0-9]{1,64}$/;
const FP_RE = /^[A-Za-z0-9-]{8,64}$/;

function timeZone(): string | undefined {
  try {
    const tz = Intl.DateTimeFormat().resolvedOptions().timeZone;
    return typeof tz === "string" && TZ_RE.test(tz) ? tz : undefined;
  } catch {
    return undefined;
  }
}

function fingerprint(): string | undefined {
  try {
    const fp = getDeviceFingerprint();
    return FP_RE.test(fp) ? fp : undefined;
  } catch {
    return undefined;
  }
}

function storage(): Storage | null {
  try {
    return typeof window === "undefined" ? null : (window.sessionStorage ?? null);
  } catch {
    return null;
  }
}

/** Ghi lai cu bam join (mot su kien pointer roi rac) roi luu snapshot v1 cho eventId. */
export function snapshotSignals(eventId: string): SignalSnapshot {
  const t = now();
  state.lastPointerDownAt = t;
  state.pe = inc(state.pe);
  recordDiscrete("p", t);

  const rt = clamp(Math.round(t - state.start), 0, MAX_RT_MS);
  const p = runProbes();
  const snap: SignalSnapshot = {
    v: 1,
    rt,
    pe: state.pe,
    se: state.se,
    ke: state.ke,
    iv: [...state.iv],
    wd: typeof navigator !== "undefined" && navigator.webdriver === true,
    gl: p.gl,
    fc: p.fc,
    tzo: clamp(Math.round(-new Date().getTimezoneOffset()), -840, 840),
  };

  const events = state.events;
  const buckets = new Set(events.map((e) => e.bucket));
  if (events.length >= SQ_MIN_EVENTS && buckets.size >= SQ_MIN_BUCKETS) {
    const rtb = Math.min(MAX_BUCKET, Math.floor(rt / RT_BUCKET_MS));
    snap.sq = fnv1a64(`${rtb}|${events.map((e) => `${e.kind}:${e.bucket}`).join(",")}`);
    snap.sn = events.length;
    snap.sb = buckets.size;
  }
  const fp = fingerprint();
  if (fp) snap.fp = fp;
  const tz = timeZone();
  if (tz) snap.tz = tz;

  try {
    storage()?.setItem(STORAGE_PREFIX + eventId, JSON.stringify(snap));
  } catch {
    // sessionStorage day / bi chan: phong cho se gui {"v":1}
  }
  return snap;
}

/** Doc snapshot da luu; null khi chua co hoac hong (phong cho khi do gui {"v":1}). */
export function readSignals(eventId: string): Record<string, unknown> | null {
  try {
    const raw = storage()?.getItem(STORAGE_PREFIX + eventId);
    if (!raw) return null;
    const parsed: unknown = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return null;
    return parsed as Record<string, unknown>;
  } catch {
    return null;
  }
}

/** Xoa snapshot khi khach roi su kien (checkout xong, het ve, roi hang). */
export function clearSignals(eventId: string): void {
  try {
    storage()?.removeItem(STORAGE_PREFIX + eventId);
  } catch {
    // bo qua
  }
}
