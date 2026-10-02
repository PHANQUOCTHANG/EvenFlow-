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
 * luon cung mot he quy chieu. */
const startedAtMs = Date.now();

/** Mac dinh READY. Frontend Next khong co buoc khoi tao bat dong bo nao truoc khi phuc vu
 * duoc; mac dinh `false` thi pod KHONG BAO GIO vao endpoints cua Service va Deployment treo
 * mai o `0/N ready`. Viec tach "dang khoi dong" khoi "da treo" da do `startupProbe` lam. */
let readyFlag = true;

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
  return readyFlag;
}

/** Duong DUY NHAT doi trang thai san sang.
 *
 * Trong ung dung, call site duy nhat la handler SIGTERM trong `instrumentation.ts`. */
export function setReady(ready: boolean): void {
  readyFlag = ready;
}
