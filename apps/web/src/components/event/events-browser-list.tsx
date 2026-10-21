"use client";

import { useEffect, useState } from "react";

import { EventListView } from "@/components/event/event-list-view";
import { getAllPublicEvents } from "@/lib/event-service";
import type { EventDetailSnapshot } from "@/lib/types/event";

/** Tai danh sach su kien o TRINH DUYET. Dung cho vo trang /events duoc prerender luc `next build`
 * (luc do khong goi backend), de nguoi truy cap dau tien van thay danh sach that (plan 8, m-R3-3). */
export function EventsBrowserList() {
  const [state, setState] = useState<{ status: "loading" } | { status: "error" } | { status: "ok"; events: EventDetailSnapshot[] }>({
    status: "loading",
  });

  useEffect(() => {
    let cancelled = false;
    getAllPublicEvents()
      .then((events) => {
        if (!cancelled) setState({ status: "ok", events });
      })
      .catch(() => {
        if (!cancelled) setState({ status: "error" });
      });
    return () => {
      cancelled = true;
    };
  }, []);

  if (state.status === "loading") {
    return (
      <p role="status" aria-busy="true" className="text-body-lg text-fg-muted" lang="vi">
        Đang tải danh sách sự kiện...
      </p>
    );
  }
  return <EventListView events={state.status === "ok" ? state.events : []} error={state.status === "error"} />;
}
