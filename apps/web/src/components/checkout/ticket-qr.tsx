"use client";

import QRCode from "qrcode";
import { useEffect, useState } from "react";

export interface TicketQrProps {
  /** Payload da ky tu server (qr_payload). Khong bao gio dung du lieu demo. */
  payload: string;
  /** Nhan truy cap duoc (aria-label) cua khoi QR. */
  label: string;
  className?: string;
}

type QrState = { status: "loading" } | { status: "ready"; svg: string } | { status: "error" };

/**
 * Ve QR that, quet duoc, tu `qr_payload` (AC-15). Render SVG phia client (muc sua loi "M").
 * Neu tao that bai thi chi hien payload dang chu nho (du phong), khong bao gio QR gia.
 */
export function TicketQr({ payload, label, className }: TicketQrProps) {
  const [state, setState] = useState<QrState>({ status: "loading" });

  useEffect(() => {
    let cancelled = false;
    setState({ status: "loading" });
    QRCode.toString(payload, { type: "svg", errorCorrectionLevel: "M", margin: 1 })
      .then((svg) => {
        if (!cancelled) setState({ status: "ready", svg });
      })
      .catch(() => {
        if (!cancelled) setState({ status: "error" });
      });
    return () => {
      cancelled = true;
    };
  }, [payload]);

  return (
    <div
      role="img"
      aria-label={label}
      data-qr-payload={payload}
      data-qr-state={state.status}
      className={className}
    >
      {state.status === "ready" ? (
        <div
          aria-hidden="true"
          className="h-32 w-32 [&>svg]:h-full [&>svg]:w-full"
          // SVG do thu vien qrcode sinh tu payload, khong chua HTML nguoi dung.
          dangerouslySetInnerHTML={{ __html: state.svg }}
        />
      ) : state.status === "error" ? (
        <code className="break-all font-mono text-caption text-fg" aria-hidden="true">
          {payload}
        </code>
      ) : null}
    </div>
  );
}
