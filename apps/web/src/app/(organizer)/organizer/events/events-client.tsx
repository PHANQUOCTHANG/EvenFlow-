"use client";

import { useCallback, useEffect, useState } from "react";

import { EventList } from "@/components/organizer/event-list";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Spinner } from "@/components/ui/spinner";
import { getOrganizerEvents, type OrganizerEvent } from "@/lib/organizer-service";

import { OrganizerOnly } from "../_lib/organizer-only";

type ListState =
  | { status: "loading" }
  | { status: "error"; message: string }
  | { status: "ready"; events: OrganizerEvent[] };

export function OrganizerEventsClient() {
  const [state, setState] = useState<ListState>({ status: "loading" });
  const [attempt, setAttempt] = useState(0);

  const load = useCallback(async () => {
    setState({ status: "loading" });
    try {
      setState({ status: "ready", events: await getOrganizerEvents() });
    } catch (err) {
      setState({
        status: "error",
        message: err instanceof Error ? err.message : "Không tải được danh sách sự kiện.",
      });
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load, attempt]);

  return (
    <OrganizerOnly>
      <div className="py-md">
        {state.status === "loading" ? (
          <Card className="p-xl text-center space-y-md">
            <Spinner size="lg" className="mx-auto" />
            <p className="text-body-sm text-fg-muted">Đang tải danh sách sự kiện...</p>
          </Card>
        ) : state.status === "error" ? (
          <div className="space-y-md">
            <Alert variant="error" title="Không tải được danh sách sự kiện">
              {state.message}
            </Alert>
            <Button variant="secondary" onClick={() => setAttempt((n) => n + 1)}>
              Thử lại
            </Button>
          </div>
        ) : (
          <EventList events={state.events} />
        )}
      </div>
    </OrganizerOnly>
  );
}
