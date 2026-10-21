"use client";

/** Tải một sự kiện của organizer ở phía client (cần Bearer) và hiển thị các trạng thái loading / lỗi / không tìm thấy. */

import { useCallback, useEffect, useState, type ReactNode } from "react";

import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Spinner } from "@/components/ui/spinner";
import { getOrganizerEventById, type OrganizerEvent } from "@/lib/organizer-service";

import { OrganizerOnly } from "./organizer-only";

type LoadState =
  | { status: "loading" }
  | { status: "error"; message: string }
  | { status: "not_found" }
  | { status: "ready"; event: OrganizerEvent };

export interface OrganizerEventViewProps {
  eventId: string;
  children: (event: OrganizerEvent, setEvent: (event: OrganizerEvent) => void) => ReactNode;
}

export function OrganizerEventView({ eventId, children }: OrganizerEventViewProps) {
  const [state, setState] = useState<LoadState>({ status: "loading" });
  const [attempt, setAttempt] = useState(0);

  const load = useCallback(async () => {
    setState({ status: "loading" });
    try {
      const event = await getOrganizerEventById(eventId);
      setState(event ? { status: "ready", event } : { status: "not_found" });
    } catch (err) {
      setState({
        status: "error",
        message: err instanceof Error ? err.message : "Không tải được sự kiện.",
      });
    }
  }, [eventId]);

  useEffect(() => {
    void load();
  }, [load, attempt]);

  const setEvent = useCallback((event: OrganizerEvent) => setState({ status: "ready", event }), []);

  return (
    <OrganizerOnly>
      {state.status === "loading" ? (
        <Card className="p-xl text-center space-y-md my-lg">
          <Spinner size="lg" className="mx-auto" />
          <p className="text-body-sm text-fg-muted">Đang tải sự kiện...</p>
        </Card>
      ) : state.status === "error" ? (
        <div className="space-y-md my-lg">
          <Alert variant="error" title="Không tải được sự kiện">
            {state.message}
          </Alert>
          <Button variant="secondary" onClick={() => setAttempt((n) => n + 1)}>
            Thử lại
          </Button>
        </div>
      ) : state.status === "not_found" ? (
        <Card className="p-xl text-center space-y-sm my-lg">
          <h1 className="text-heading-md font-bold text-fg">Không tìm thấy sự kiện</h1>
          <p className="text-body-sm text-fg-muted">
            Sự kiện với mã &ldquo;{eventId}&rdquo; không tồn tại hoặc bạn không có quyền truy cập.
          </p>
        </Card>
      ) : (
        <div className="py-md">{children(state.event, setEvent)}</div>
      )}
    </OrganizerOnly>
  );
}
