"use client";

/** Danh sách sự kiện cho war room (ops-warroom AC-26). Tải ở client: token nằm ở trình duyệt. */

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";

import { Alert } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Spinner } from "@/components/ui/spinner";
import { listOpsEvents, saleStateLabel, type OpsEventListItem } from "@/lib/ops-service";

type ListState =
  | { status: "loading" }
  | { status: "error" }
  | { status: "ready"; events: OpsEventListItem[] };

export function OpsEventsClient() {
  const [state, setState] = useState<ListState>({ status: "loading" });
  const [attempt, setAttempt] = useState(0);

  const load = useCallback(async () => {
    setState({ status: "loading" });
    try {
      setState({ status: "ready", events: await listOpsEvents() });
    } catch {
      setState({ status: "error" });
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load, attempt]);

  if (state.status === "loading") {
    return (
      <div className="flex items-center gap-sm py-md text-fg-muted">
        <Spinner size="md" />
        <span>Đang tải danh sách sự kiện</span>
      </div>
    );
  }

  if (state.status === "error") {
    return (
      <div className="space-y-md">
        <Alert variant="error" title="Không tải được danh sách sự kiện">
          Kiểm tra kết nối rồi thử lại.
        </Alert>
        <Button variant="secondary" onClick={() => setAttempt((n) => n + 1)}>
          Thử lại
        </Button>
      </div>
    );
  }

  if (state.events.length === 0) {
    return <p className="text-body-md text-fg-muted">Chưa có sự kiện nào đang hoặc sắp mở bán</p>;
  }

  return (
    <ul className="space-y-sm">
      {state.events.map((event) => (
        <li key={event.eventId}>
          <Card className="p-0">
            <Link
              href={`/ops/events/${event.eventId}`}
              className="ef-focus-ring flex items-center justify-between gap-md rounded-card p-md hover:bg-surface-subtle"
            >
              <span className="font-semibold text-fg">{event.eventTitle}</span>
              <Badge variant={event.saleState === "ON_SALE" ? "admitted" : "neutral"}>
                {saleStateLabel(event.saleState)}
              </Badge>
            </Link>
          </Card>
        </li>
      ))}
    </ul>
  );
}
