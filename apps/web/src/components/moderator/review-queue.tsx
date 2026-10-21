"use client";

/** Hàng đợi thẩm định của Moderator (EV-121, handoff §10.15).
 *
 * Gồm hồ sơ AI chuyển người duyệt (NEEDS_HUMAN) và hồ sơ AI đã từ chối — BR-E5: mọi quyết định AI
 * đều phải lật ngược được, nên Moderator cần thấy cả những hồ sơ AI tự loại. Không hiện dữ liệu cá
 * nhân của người mua; điểm AI chỉ là một tín hiệu, luôn đi kèm lý do. */

import Link from "next/link";
import { useCallback, useEffect, useId, useMemo, useState } from "react";

import { Alert } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Spinner } from "@/components/ui/spinner";
import { ApiError } from "@/lib/api-client";
import { formatDateTimeVn, spellDuration } from "@/lib/format";
import { getModerationQueue, type QueueItem } from "@/lib/moderation-service";

import { VERDICT_LABELS, foldVi, percent } from "./labels";

type Filter = "all" | "needs_human" | "ai_rejected";

type LoadState =
  | { status: "loading" }
  | { status: "forbidden" }
  | { status: "error"; message: string }
  | { status: "ready"; items: QueueItem[] };

const FILTERS: { key: Filter; label: string; match: (i: QueueItem) => boolean }[] = [
  { key: "all", label: "Tất cả", match: () => true },
  { key: "needs_human", label: "Cần người duyệt", match: (i) => i.aiVerdict === "NEEDS_HUMAN" },
  { key: "ai_rejected", label: "AI đã từ chối", match: (i) => i.aiVerdict === "AUTO_REJECTED" },
];

export function ReviewQueue() {
  const [state, setState] = useState<LoadState>({ status: "loading" });
  const [attempt, setAttempt] = useState(0);
  const [filter, setFilter] = useState<Filter>("all");
  const [query, setQuery] = useState("");
  const searchId = useId();

  const load = useCallback(async () => {
    setState({ status: "loading" });
    try {
      setState({ status: "ready", items: await getModerationQueue() });
    } catch (err) {
      if (err instanceof ApiError && err.status === 403) {
        setState({ status: "forbidden" });
        return;
      }
      setState({ status: "error", message: err instanceof Error ? err.message : "Không tải được hàng đợi." });
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load, attempt]);

  const items = useMemo(() => (state.status === "ready" ? state.items : []), [state]);
  const visible = useMemo(() => {
    const match = FILTERS.find((f) => f.key === filter)!.match;
    const q = foldVi(query.trim());
    return items.filter((i) => match(i) && (q === "" || foldVi(`${i.title} ${i.venue}`).includes(q)));
  }, [items, filter, query]);

  if (state.status === "loading") {
    return (
      <Card className="p-xl text-center space-y-md my-lg">
        <Spinner size="lg" className="mx-auto" />
        <p className="text-body-sm text-fg-muted">Đang tải hàng đợi thẩm định...</p>
      </Card>
    );
  }
  if (state.status === "forbidden") {
    return (
      <Alert variant="error" title="Không có quyền truy cập" className="my-lg">
        Tài khoản này không có quyền Moderator.
      </Alert>
    );
  }
  if (state.status === "error") {
    return (
      <div className="space-y-md my-lg">
        <Alert variant="error" title="Không tải được hàng đợi">
          {state.message}
        </Alert>
        <Button variant="secondary" onClick={() => setAttempt((n) => n + 1)}>
          Thử lại
        </Button>
      </div>
    );
  }

  const needsHuman = items.filter((i) => i.aiVerdict === "NEEDS_HUMAN").length;
  const aiRejected = items.filter((i) => i.aiVerdict === "AUTO_REJECTED").length;
  const oldest = items
    .map((i) => (i.submittedAt ? Date.parse(i.submittedAt) : NaN))
    .filter((t) => Number.isFinite(t))
    .sort((a, b) => a - b)[0];

  return (
    <div className="space-y-lg py-md">
      <header className="space-y-xs">
        <h1 className="text-heading-lg font-bold text-fg">Hàng đợi thẩm định</h1>
        <p className="text-body-sm text-fg-muted">
          Hồ sơ AI chuyển người duyệt và hồ sơ AI đã từ chối. Kết quả AI chỉ là khuyến nghị — quyết định thuộc về
          Moderator và được ghi nhật ký.
        </p>
      </header>

      <div className="grid gap-md sm:grid-cols-3">
        <Card className="p-md space-y-xs">
          <p className="text-body-sm text-fg-muted">Cần người duyệt</p>
          <p className="text-heading-lg font-bold text-fg" data-testid="kpi-needs-human">
            {needsHuman}
          </p>
        </Card>
        <Card className="p-md space-y-xs">
          <p className="text-body-sm text-fg-muted">AI đã từ chối (có thể lật ngược)</p>
          <p className="text-heading-lg font-bold text-fg" data-testid="kpi-ai-rejected">
            {aiRejected}
          </p>
        </Card>
        <Card className="p-md space-y-xs">
          <p className="text-body-sm text-fg-muted">Hồ sơ chờ lâu nhất</p>
          <p className="text-heading-md font-semibold text-fg" data-testid="kpi-oldest">
            {oldest === undefined ? "—" : spellDuration(Date.now() - oldest)}
          </p>
        </Card>
      </div>

      <div className="flex flex-wrap items-center gap-sm">
        <div role="group" aria-label="Lọc theo kết quả AI" className="flex flex-wrap gap-xs">
          {FILTERS.map((f) => {
            const count = items.filter(f.match).length;
            return (
              <Button
                key={f.key}
                size="sm"
                variant={filter === f.key ? "primary" : "secondary"}
                aria-pressed={filter === f.key}
                onClick={() => setFilter(f.key)}
              >
                {f.label} ({count})
              </Button>
            );
          })}
        </div>
        <label htmlFor={searchId} className="sr-only">
          Tìm hồ sơ theo tên hoặc địa điểm
        </label>
        <input
          id={searchId}
          type="search"
          aria-label="Tìm hồ sơ theo tên hoặc địa điểm"
          placeholder="Tìm theo tên sự kiện hoặc địa điểm..."
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          className="ef-focus-ring h-10 min-w-[16rem] flex-1 rounded-control border border-border-strong bg-surface px-sm text-body-sm text-fg placeholder:text-fg-muted"
        />
      </div>

      {items.length === 0 ? (
        <Card className="p-xl text-center space-y-xs">
          <p className="text-heading-sm font-semibold text-fg">Không còn hồ sơ nào chờ thẩm định</p>
          <p className="text-body-sm text-fg-muted">Hồ sơ mới sẽ xuất hiện khi AI chuyển người duyệt hoặc tự từ chối.</p>
        </Card>
      ) : visible.length === 0 ? (
        <Card className="p-lg text-center">
          <p className="text-body-sm text-fg-muted">Không có hồ sơ khớp bộ lọc hiện tại.</p>
        </Card>
      ) : (
        <ul className="space-y-md" aria-label="Danh sách hồ sơ">
          {visible.map((item) => (
            <li key={item.eventId}>
              <QueueCard item={item} />
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function QueueCard({ item }: { item: QueueItem }) {
  const titleId = useId();
  const rejected = item.aiVerdict === "AUTO_REJECTED";
  return (
    <Card className="p-md space-y-sm">
      <article aria-labelledby={titleId} className="space-y-sm">
        <div className="flex flex-wrap items-start justify-between gap-sm">
          <div className="space-y-xs">
            <h2 id={titleId} className="text-heading-sm font-semibold text-fg">
              {item.title}
            </h2>
            <p className="text-body-sm text-fg-muted">
              {item.venue || "Chưa có địa điểm"} · Bắt đầu {formatDateTimeVn(item.startsAt)}
            </p>
          </div>
          <div className="flex items-center gap-xs">
            {item.aiVerdict ? (
              <Badge variant={rejected ? "failed" : "pending"}>{VERDICT_LABELS[item.aiVerdict]}</Badge>
            ) : null}
            {item.aiScore !== null ? (
              <span className="text-body-sm text-fg-muted" title="Điểm rủi ro AI (cao = rủi ro hơn)">
                Rủi ro AI <strong className="text-fg">{percent(item.aiScore)}%</strong>
              </span>
            ) : null}
          </div>
        </div>

        {item.aiReasons.length > 0 ? (
          <p className="text-body-sm text-fg">
            <span className="text-fg-muted">Lý do AI: </span>
            <span>{item.aiReasons[0]}</span>
          </p>
        ) : null}

        <div className="flex flex-wrap items-center justify-between gap-sm">
          <p className="text-body-sm text-fg-muted">
            Vào hàng đợi {item.submittedAt ? formatDateTimeVn(item.submittedAt) : "—"}
            {rejected ? " · có thể lật ngược quyết định AI" : ""}
          </p>
          <Link
            href={`/moderator/reviews/${item.eventId}`}
            className="ef-focus-ring inline-flex h-10 items-center rounded-control bg-primary px-md text-body-sm font-semibold text-primary-fg hover:bg-primary-hover"
          >
            Mở thẩm định
          </Link>
        </div>
      </article>
    </Card>
  );
}
