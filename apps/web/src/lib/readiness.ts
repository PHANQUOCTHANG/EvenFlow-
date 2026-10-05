/** Trang thai "da san sang nhan traffic chua" + dong ho uptime cua tien trinh web.
 *
 * Day la cho DUY NHAT giu hai thong tin do. Route `/api/readyz` chi dich trang thai nay
 * thanh ma HTTP (200 / 503); no khong tu quyet dinh gi.
 *
 * Module nay la TOGGLE THUAN (C1.1): `setReady(true)` va `setReady(false)` deu co tac dung,
 * goi qua lai bao nhieu lan cung duoc. Tinh "mot chieu" la bat bien cua HE THONG, khong phai
 * cua module: noi DUY NHAT goi `setReady(false)` la handler SIGTERM trong `instrumentation.ts`,
 * va khong noi nao goi `setReady(true)` sau do. Dat latch (`if (!ready) return`) ngay trong day
 * la hien thuc sai bat bien o tang sai — no bien mot ham ba dong thanh thu khong test duoc,
 * trong khi rang buoc thuc te duoc bao ve bang viec chi co mot call site.
 *
 * KHONG cham dependency ngoai (khong fetch, khong redis, khong db): xem spec muc 3.1. */

/** Moc khoi tao module, do bang `Date.now()` chu khong `process.uptime()` hay
 * `performance.now()`.
 *
 * Ly do khong phai phong cach: `uptimeMs()` phai dem theo dung nguon thoi gian ma test va
 * caller quan sat duoc. `process.uptime()` do thoi gian song cua TIEN TRINH, khong phai cua
 * module, nen no van chay khi module duoc nap lai va khong the dat lai duoc tu ben ngoai.
 * `Date.now()` cung la nguon duy nhat chay dung duoi dong ho gia, nen moc khoi tao va moc doc
 * luon cung mot he quy chieu.
 *
 * Moc nay CO Y de o tam module, khong dua len `globalThis` nhu co `ready` ben duoi. C1 dinh
 * nghia `uptimeMs` la "ms ke tu khi MODULE khoi tao", va dua len `globalThis` se lam moc song
 * sot qua `vi.resetModules()` — dung thu ba test da dong bang dang do (nap lai module duoi dong
 * ho gia roi doi `uptimeMs()` bat dau lai tu 0). Hau qua can biet trong van hanh: con so nay la
 * "ms ke tu khi chunk `/api/readyz` duoc nap lan dau", khong phai tuoi cua tien trinh. Khong co
 * probe nao phu thuoc gia tri tuyet doi cua no. */
const startedAtMs = Date.now();

/** Co san sang duoc giu tren `globalThis`, KHONG phai bang `let` o tam module.
 *
 * Day khong phai thoi quen phong thu, day la bat buoc — da kiem bang `next build` that:
 * Next dong goi `instrumentation.ts` va TUNG route handler thanh CAC BUNDLE WEBPACK RIENG.
 * File nay bi nhan ban vao moi bundle, nen mot `let` o tam module se thanh HAI bien khac nhau.
 * Hau qua do trong ban build dau tien (da do lai `.next/server/`):
 *
 *   - Trong bundle cua `/api/readyz` khong co ai GHI `readyFlag`, nen terser chung minh duoc
 *     no luon `true` va **inline** luon: body bi fold thanh `{status:"ready",...}` va **ca
 *     nhanh 503 bi xoa khoi ban build** (`grep -c 503` ra 0).
 *   - Trong bundle cua `instrumentation.ts` khong co ai DOC `readyFlag`, nen loi goi
 *     `setReady(false)` thanh dead code va cung bi xoa (`grep -c setReady` ra 0).
 *
 * Tuc la unit test xanh (vitest khong bundle, no dung dung mot module instance) trong khi
 * production thi `/api/readyz` **luon tra 200** va drain khong bao gio quan sat duoc. Dung
 * dung che do loi ma C1.1b sinh ra de diet: "nhanh 503 lai la code chet trong van hanh".
 *
 * `globalThis` sua dut ca hai: mot tien trinh Node chi co MOT `globalThis`, nen hai bundle
 * dung chung dung mot o nho; va doc/ghi mot property cua `globalThis` la side effect khong
 * phan tich tinh duoc, nen optimizer khong the fold hay xoa. */
const STATE_KEY = "__eventflowWebReadiness__";

type ReadinessState = { ready: boolean };

type GlobalWithReadiness = typeof globalThis & {
  [STATE_KEY]?: ReadinessState;
};

/** Mac dinh READY. Frontend Next khong co buoc khoi tao bat dong bo nao truoc khi phuc vu
 * duoc; mac dinh `false` thi pod KHONG BAO GIO vao endpoints cua Service va Deployment treo
 * mai o `0/N ready`. Viec tach "dang khoi dong" khoi "da treo" da do `startupProbe` lam. */
function state(): ReadinessState {
  const scope = globalThis as GlobalWithReadiness;

  // Kiem HINH DANG chu khong chi kiem ton tai. Dat state tren `globalThis` la cai gia phai
  // tra de optimizer khong fold duoc no (xem comment dau file) — nhung cai gia di kem la mot
  // o nho ai cung ghi duoc. `if (existing)` khong thoi se nhan ca `"ready"` (chuoi), `1`, hay
  // `{}`: khi do `existing.ready` la `undefined`, `isReady()` tra ve mot gia tri khong phai
  // boolean (vi pham chu ky C1.2), readyz tra 503 VINH VIEN, va cong voi `maxUnavailable: 0`
  // thi rollout treo mai trong khi `kubectl describe` chi noi readiness probe fail.
  const existing = scope[STATE_KEY];
  if (typeof existing === "object" && existing !== null && typeof existing.ready === "boolean") {
    return existing;
  }

  const created: ReadinessState = { ready: true };
  scope[STATE_KEY] = created;
  return created;
}

/** Mili-giay ke tu khi module khoi tao. So nguyen, khong am.
 *
 * Chan duoi o 0 vi dong ho he thong co the bi dat lui (NTP step, sua gio tay) — luc do hieu
 * so ra so am, va mot `uptimeMs` am se lam body cua `/api/readyz` vo nghia. */
export function uptimeMs(): number {
  const elapsed = Date.now() - startedAtMs;
  return elapsed > 0 ? elapsed : 0;
}

/** Trang thai hien tai, doc dong bo tai thoi diem goi.
 *
 * Dong bo la bat buoc: `/api/readyz` phai tra loi duoc trong mot nhip probe, va mot API
 * `Promise` o day se moi goi them I/O vao dung cho khong duoc co I/O. */
export function isReady(): boolean {
  // `=== true` chu khong tra thang: chu ky C1.2 chot kieu tra ve la `boolean`, va day la lop
  // chan cuoi neu `state()` bao gio do tra ve thu khong dung hinh dang.
  return state().ready === true;
}

/** Duong DUY NHAT doi trang thai san sang.
 *
 * Trong ung dung, call site duy nhat la handler SIGTERM trong `instrumentation.ts`. */
export function setReady(ready: boolean): void {
  state().ready = ready;
}
