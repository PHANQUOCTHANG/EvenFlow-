"use client";

/** War room cho Ops (EV-187, EVF-117, Stitch 10.18): phễu, tuyển sinh, tồn kho, AI trên một màn hình.
 *
 * - Dữ liệu làm mới mỗi 2s qua useOpsWarRoom; quá 5s hoặc lần làm mới lỗi thì hiện cảnh báo
 *   nhưng KHÔNG xoá số liệu cuối (AC-2).
 * - Dữ liệu mẫu phải gắn nhãn rõ (AC-3). Khu "Điều khiển" tách khỏi "Giám sát" (AC-6).
 * - Không dùng Date/locale trong lần render đầu: ageMs là null cho tới khi mount (tránh hydration). */

import Link from "next/link";
import type { ReactNode } from "react";

import { Alert } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Spinner } from "@/components/ui/spinner";
import { useOpsWarRoom } from "@/hooks/use-ops-war-room";
import { formatTimeVn } from "@/lib/format";
import {
  opsErrorStatus,
  saleStateLabel,
  tierInventoryViolation,
  type OpsWarRoomSnapshot,
} from "@/lib/ops-service";

import {
  OpsAdmissionPanel,
  OpsAiPanel,
  OpsControlsPanel,
  OpsFunnelPanel,
  OpsInventoryPanel,
} from "./ops-panels";

export interface OpsWarRoomProps {
  eventId: string;
  initialSnapshot?: OpsWarRoomSnapshot | null;
  /** Lỗi lần tải đầu ở server (object thường, không truyền class qua ranh giới server/client). */
  initialError?: { status: number; message: string };
}

function ErrorScreen({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <Card className="my-lg space-y-sm p-xl text-center">
      <h1 className="text-headline-md font-bold text-fg">{title}</h1>
      {children}
    </Card>
  );
}

function BackLink() {
  return (
    <Link href="/ops/events" className="text-body-md text-primary hover:underline">
      ← Danh sách sự kiện
    </Link>
  );
}

function NoPermission() {
  return (
    <ErrorScreen title="Không có quyền">
      <p className="text-body-md text-fg-muted">Bạn không có quyền xem war room của sự kiện này.</p>
      <BackLink />
    </ErrorScreen>
  );
}

/** Nội dung "Không tìm thấy sự kiện", dùng chung với not-found.tsx của route. */
export function OpsNotFoundView() {
  return (
    <ErrorScreen title="Không tìm thấy sự kiện">
      <p className="text-body-md text-fg-muted">Sự kiện không tồn tại hoặc đã bị gỡ.</p>
      <BackLink />
    </ErrorScreen>
  );
}

export function OpsWarRoom({ eventId, initialSnapshot = null, initialError }: OpsWarRoomProps) {
  // 403/404 ở lần tải đầu: màn tĩnh, không polling (hook không được mount).
  if (!initialSnapshot && initialError?.status === 403) return <NoPermission />;
  if (!initialSnapshot && initialError?.status === 404) return <OpsNotFoundView />;

  return (
    <LiveWarRoom eventId={eventId} initialSnapshot={initialSnapshot} initialError={initialError} />
  );
}

function LiveWarRoom({
  eventId,
  initialSnapshot,
  initialError,
}: {
  eventId: string;
  initialSnapshot: OpsWarRoomSnapshot | null;
  initialError?: { status: number; message: string };
}) {
  const { snapshot, error, isRefreshing, ageMs, isStale, refresh } = useOpsWarRoom({
    eventId,
    initialSnapshot,
  });

  const errorStatus = error ? opsErrorStatus(error) : snapshot ? undefined : initialError?.status;

  if (errorStatus === 403) return <NoPermission />;
  if (!snapshot && errorStatus === 404) return <OpsNotFoundView />;

  if (!snapshot) {
    const message = error?.message ?? initialError?.message;
    if (message === undefined) {
      return (
        <div className="flex items-center gap-sm py-xl text-fg-muted">
          <Spinner size="md" />
          <span>Đang tải số liệu war room</span>
        </div>
      );
    }
    return (
      <ErrorScreen title="Không tải được số liệu">
        <p className="text-body-md text-fg-muted">{message}</p>
        <div>
          <Button
            variant="primary"
            aria-busy={isRefreshing ? true : undefined}
            onClick={() => void refresh()}
          >
            Thử lại
          </Button>
        </div>
      </ErrorScreen>
    );
  }

  // Nguồn ticketing không `ok` thì không kiểm BR-O1 (số liệu có thể cũ hoặc không có).
  const violations =
    snapshot.sources.ticketing === "ok"
      ? snapshot.inventory.filter((t) => tierInventoryViolation(t) !== null)
      : [];

  return (
    <div className="mx-auto max-w-6xl space-y-lg py-md">
      <header className="space-y-sm border-b border-border pb-md">
        <BackLink />
        <div className="flex flex-col justify-between gap-md md:flex-row md:items-start">
          <div className="space-y-xs">
            <h1 className="text-headline-xl font-bold text-fg">{snapshot.eventTitle}</h1>
            <div className="flex flex-wrap items-center gap-sm">
              <Badge variant={snapshot.saleState === "ON_SALE" ? "admitted" : "neutral"}>
                {saleStateLabel(snapshot.saleState)}
              </Badge>
              {snapshot.saleState !== "ON_SALE" ? (
                <p className="text-body-md text-fg-muted">Không có đợt bán đang diễn ra</p>
              ) : null}
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-sm">
            <Button
              variant="secondary"
              aria-busy={isRefreshing ? true : undefined}
              onClick={() => void refresh()}
            >
              {isRefreshing ? "Đang làm mới…" : "Làm mới"}
            </Button>
          </div>
        </div>
      </header>

      <div className="space-y-sm">
        {snapshot.isSample ? (
          <Alert variant="info" title="Dữ liệu mẫu">
            Số liệu minh họa, chưa nối với nguồn thật. Không dùng để ra quyết định vận hành.
          </Alert>
        ) : null}
        <div className="flex flex-wrap gap-x-md gap-y-xs text-body-md text-fg-muted">
          <p>Cập nhật lúc {formatTimeVn(snapshot.generatedAt)}</p>
          {ageMs !== null && Number.isFinite(ageMs) ? (
            <p>Tuổi dữ liệu: {Math.floor(ageMs / 1000)} giây</p>
          ) : null}
        </div>
      </div>

      {isStale ? (
        <Alert variant="warning" title="Dữ liệu cũ">
          {error
            ? "Lần làm mới gần nhất không thành công. Đang giữ số liệu lần cuối."
            : "Số liệu đã quá 5 giây chưa được làm mới. Đang giữ số liệu lần cuối."}
        </Alert>
      ) : null}

      {violations.length > 0 ? (
        <Alert variant="warning" title="Lệch tồn kho">
          Hạng vé vi phạm bất biến BR-O1: {violations.map((t) => t.name).join(", ")}.
        </Alert>
      ) : null}

      <section aria-labelledby="ops-monitor-heading" className="space-y-md">
        <h2 id="ops-monitor-heading" className="text-headline-md font-bold text-fg">
          Giám sát
        </h2>
        <OpsFunnelPanel snapshot={snapshot} />
        <div className="grid grid-cols-1 gap-md lg:grid-cols-2">
          <OpsAdmissionPanel snapshot={snapshot} />
          <OpsAiPanel snapshot={snapshot} />
        </div>
        <OpsInventoryPanel snapshot={snapshot} />
      </section>

      <section aria-labelledby="ops-controls-heading" className="space-y-md">
        <h2 id="ops-controls-heading" className="text-headline-md font-bold text-fg">
          Điều khiển
        </h2>
        <OpsControlsPanel />
      </section>
    </div>
  );
}
