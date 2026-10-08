/** Client-side Jitter tai T0 (BR-Q5, EVF-61, kien truc Tang 2).
 *
 * Tai thoi diem mo ban (T0), hang tram nghin khach cung bam vao phong cho hoac client tu dong
 * goi join. Neu khong co jitter, he thong phai chiu dinh nhon 300.000 rps trong giay dau tien.
 *
 * Client tu hoan ngau nhien 0..5s sau T0 de trai deu luu luong thanh ~60.000 rps.
 * Nho co che phong cho xao tron (lottery/randomized lobby o BR-Q1), viec hoan 0..5s khong he
 * gay bat loi hay lam giam co hoi mua ve cua khach hang. */

export const DEFAULT_MIN_JITTER_MS = 0;
export const DEFAULT_MAX_JITTER_MS = 5000;

export interface JitterOptions {
  minJitterMs?: number;
  maxJitterMs?: number;
  /** Cho phep truyen ham sinh ngau nhien de kiem thu xac dinh. Mac dinh: Math.random */
  randomFn?: () => number;
}

/** Tinh do tre jitter ngau nhien phan bo deu trong khoang [min, max].
 * Dau vao bat hop le se duoc chuan hoa ve gia tri an toan hop ly. */
export function calculateJitterMs(options: JitterOptions = {}): number {
  const min = Math.max(0, options.minJitterMs ?? DEFAULT_MIN_JITTER_MS);
  const max = Math.max(min, options.maxJitterMs ?? DEFAULT_MAX_JITTER_MS);

  if (min === max) {
    return min;
  }

  const rand = (options.randomFn ?? Math.random)();
  const clampedRand = Math.max(0, Math.min(1, rand));

  return Math.floor(min + clampedRand * (max - min));
}

export interface JitterProgress {
  remainingMs: number;
  totalJitterMs: number;
}

/** Hoan thuc thi kem theo callback cap nhat tien trinh (neu co).
 * Tra ve tong so ms da jitter. */
export function sleepWithJitter(
  onProgress?: (progress: JitterProgress) => void,
  options: JitterOptions = {},
): Promise<number> {
  const totalMs = calculateJitterMs(options);

  if (totalMs <= 0) {
    if (onProgress) {
      onProgress({ remainingMs: 0, totalJitterMs: 0 });
    }
    return Promise.resolve(0);
  }

  return new Promise((resolve) => {
    const startTime = Date.now();
    const intervalMs = 100;

    // Thong bao khoi tao
    if (onProgress) {
      onProgress({ remainingMs: totalMs, totalJitterMs: totalMs });
    }

    const timer = setInterval(() => {
      const elapsed = Date.now() - startTime;
      const remaining = Math.max(0, totalMs - elapsed);

      if (onProgress) {
        onProgress({ remainingMs: remaining, totalJitterMs: totalMs });
      }

      if (remaining <= 0) {
        clearInterval(timer);
        resolve(totalMs);
      }
    }, intervalMs);
  });
}
