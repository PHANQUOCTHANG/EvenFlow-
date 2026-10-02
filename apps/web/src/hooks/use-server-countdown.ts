"use client";

import { useCallback, useEffect, useState } from "react";

import { serverNow, toEpochMs } from "@/lib/server-time";

export interface ServerCountdown {
  remainingMs: number;
  expired: boolean;
  /** false cho den khi component da mount tren browser.
   *
   *  Truoc moc do chua the biet con lai bao nhieu: moi route trong app nay deu la `○ (Static)`
   *  nen HTML duoc prerender LUC BUILD. Neu doc `Date.now()` ngay trong than render thi con so
   *  cua thoi diem BUILD bi dong bang vao HTML — khach mo trang ba ngay sau se thay "da het
   *  thoi gian giu ve" o first paint roi moi nhay ve so dung sau khi hydrate. Va vi lan render
   *  hydrate phai cho ra output y het server, gia tri khoi tao buoc phai la gia tri XAC DINH.
   *
   *  `ready === false` nghia la "chua biet", KHAC "con 0" va KHAC "da het han" — cung mot
   *  nguyen tac voi `expiresAt` null o duoi. */
  ready: boolean;
}

/** 250ms: du min de so giay doi dung luc, va van re. */
const TICK_MS = 250;

const UNKNOWN: ServerCountdown = { remainingMs: 0, expired: false, ready: false };

/** Dem nguoc toi mot MOC tuyet doi cua server (BR-O2).
 *
 * Hook nay co y KHONG nhan `durationMs`, va gia tri duoc TINH TRONG LUC RENDER tu
 * `expiresAt - serverNow()` chu khong giam dan mot bien dem.
 *
 * Ly do khong phai hoc thuat: browser tiet lieu `setInterval` o tab background xuong con
 * khoang 1 lan/phut. Mot hien thuc giam dan se chay CHAM hon thuc te va hien cho khach
 * nhieu thoi gian hon so ho thuc su con lai — khach mat ve ma tuong con kip. Tinh lai tu
 * moc thi tab background bao lau cung khong anh huong.
 *
 * Tinh trong luc render (thay vi luu vao state roi cap nhat trong effect) con tranh mot loi
 * nua: khi `expiresAt` doi sang hold moi, state cu se con hien mot frame "da het thoi gian
 * giu ve" tren mot hold van con 10 phut.
 *
 * `expiresAt` null/undefined/khong parse duoc nghia la CHUA BIET thoi han, nen `expired` la
 * false — khac han voi da het han. Bao "het han" cho mot thoi han chua biet la noi voi khach
 * rang ho da mat mot hold ma ho van dang giu. */
export function useServerCountdown(
  expiresAt: number | string | null | undefined,
  offsetMs: number = 0,
): ServerCountdown {
  const target = toEpochMs(expiresAt);

  // Gia tri khoi tao phai xac dinh de SSR va lan hydrate khop nhau. Cung pattern voi
  // use-theme.ts, vi cung mot ly do.
  const [mounted, setMounted] = useState(false);
  useEffect(() => {
    setMounted(true);
  }, []);

  const read = useCallback((): ServerCountdown => {
    if (!mounted || target === null) return UNKNOWN;

    const remaining = target - serverNow(offsetMs);
    return remaining <= 0
      ? { remainingMs: 0, expired: true, ready: true }
      : { remainingMs: remaining, expired: false, ready: true };
  }, [mounted, target, offsetMs]);

  // Chi dung de buoc render lai theo nhip; gia tri that duoc tinh o `read()` ben duoi.
  const [, setTick] = useState(0);

  useEffect(() => {
    if (!mounted || target === null || read().expired) return;

    const id = setInterval(() => {
      setTick((value) => value + 1);
      if (read().expired) clearInterval(id);
    }, TICK_MS);

    return () => clearInterval(id);
  }, [mounted, read, target]);

  return read();
}
