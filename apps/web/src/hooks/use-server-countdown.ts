"use client";

import { useCallback, useEffect, useState } from "react";

import { serverNow, toEpochMs } from "@/lib/server-time";

export interface ServerCountdown {
  remainingMs: number;
  expired: boolean;
}

/** 250ms: du min de so giay doi dung luc, va van re. */
const TICK_MS = 250;

/** Dem nguoc toi mot MOC tuyet doi cua server (BR-O2).
 *
 * Hook nay co y KHONG nhan `durationMs`, va moi tick TINH LAI `expiresAt - serverNow()`
 * chu khong giam dan mot bien dem.
 *
 * Ly do khong phai hoc thuat: browser tiet lieu `setInterval` o tab background xuong con
 * khoang 1 lan/phut. Mot hien thuc giam dan se chay CHAM hon thuc te va hien cho khach
 * nhieu thoi gian hon so ho thuc su con lai — khach mat ve ma tuong con kip. Tinh lai tu
 * moc thi tab background bao lau cung khong anh huong: lan tick ke tiep tra ve dung so con lai.
 *
 * `expiresAt` null/undefined nghia la CHUA BIET thoi han, nen `expired` la false — khac han
 * voi da het han. Bao "het han" cho mot thoi han chua biet la noi voi khach rang ho da mat
 * mot hold ma ho van dang giu. */
export function useServerCountdown(
  expiresAt: number | string | null | undefined,
  offsetMs: number = 0,
): ServerCountdown {
  const target = toEpochMs(expiresAt);

  const read = useCallback((): ServerCountdown => {
    if (target === null) return { remainingMs: 0, expired: false };

    const remaining = target - serverNow(offsetMs);
    return remaining <= 0
      ? { remainingMs: 0, expired: true }
      : { remainingMs: remaining, expired: false };
  }, [target, offsetMs]);

  // Khoi tao lazy: moc da qua thi `expired` dung ngay o lan render DAU, khong cho tick.
  const [state, setState] = useState<ServerCountdown>(read);

  useEffect(() => {
    const first = read();
    setState(first);

    // Khong co moc, hoac da het han: khong can interval nao.
    if (target === null || first.expired) return;

    const id = setInterval(() => {
      const next = read();
      setState(next);
      if (next.expired) clearInterval(id);
    }, TICK_MS);

    return () => clearInterval(id);
  }, [read, target]);

  return state;
}
