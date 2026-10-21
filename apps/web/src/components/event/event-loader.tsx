"use client";

import { notFound } from "next/navigation";
import type { ReactNode } from "react";

import { useT } from "@/components/i18n/locale-provider";
import { Alert, Button, Spinner } from "@/components/ui";
import { useEvent } from "@/hooks/use-event";
import type { EventDetailSnapshot } from "@/lib/types/event";

export interface EventLoaderProps {
  eventId: string;
  children: (event: EventDetailSnapshot) => ReactNode;
}

/** Tai su kien theo UUID o trinh duyet cho cac trang mua ve (AC-18):
 *  404 -> notFound(); loi khac -> trang thai loi + thu lai. */
export function EventLoader({ eventId, children }: EventLoaderProps) {
  const t = useT();
  const state = useEvent(eventId);

  if (state.status === "not_found") {
    notFound();
  }

  if (state.status === "error") {
    return (
      <div className="mx-auto max-w-content py-md">
        <Alert variant="error" title={t("eventLoader.errorTitle")}>
          <p className="mb-sm">{t("eventLoader.errorBody")}</p>
          <Button variant="secondary" onClick={state.retry}>
            {t("eventLoader.retry")}
          </Button>
        </Alert>
      </div>
    );
  }

  if (state.status === "loading") {
    return (
      <div role="status" className="flex flex-col items-center justify-center gap-md p-2xl">
        <Spinner size="lg" />
        <p className="text-body-md text-fg-muted">{t("eventLoader.loading")}</p>
      </div>
    );
  }

  return <>{children(state.event)}</>;
}
