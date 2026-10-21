"use client";

/** Thẩm định hồ sơ sự kiện (EV-121, handoff §10.16).
 *
 * Ba vùng tách bạch: nội dung GỐC của ban tổ chức, phân tích AI (chỉ là khuyến nghị) và lịch sử xử
 * lý từ audit. Không trộn câu chữ của AI vào phần nội dung gốc — Moderator phải thấy rõ đâu là thứ
 * ban tổ chức nộp, đâu là điều máy nghĩ. */

import { useCallback, useEffect, useId, useState } from "react";

import { Alert } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Spinner } from "@/components/ui/spinner";
import { ApiError } from "@/lib/api-client";
import { formatDateTimeVn } from "@/lib/format";
import { getModerationDetail, type ModerationDetail } from "@/lib/moderation-service";
import { formatMoney } from "@/lib/money";

import { DecisionPanel } from "./decision-panel";
import { AXIS_LABELS, AXIS_ORDER, VERDICT_LABELS, actorLabel, auditLabel, percent } from "./labels";

type LoadState =
  | { status: "loading" }
  | { status: "not_found" }
  | { status: "forbidden" }
  | { status: "error"; message: string }
  | { status: "ready"; detail: ModerationDetail };

export function ReviewDetail({ eventId }: { eventId: string }) {
  const [state, setState] = useState<LoadState>({ status: "loading" });
  const [attempt, setAttempt] = useState(0);
  const [conflict, setConflict] = useState(false);
  // Giu o day (khong o DecisionPanel): trang thai su kien doi sau quyet dinh, panel ve trang khoa,
  // nhung Moderator van phai thay quyet dinh cua minh da duoc ghi.
  const [decided, setDecided] = useState<"approved" | "rejected" | null>(null);

  /** silent: tải lại sau 409 mà không xoá màn hình (giữ thông báo cho Moderator). */
  const load = useCallback(
    async (silent = false) => {
      if (!silent) setState({ status: "loading" });
      try {
        const detail = await getModerationDetail(eventId);
        setState(detail ? { status: "ready", detail } : { status: "not_found" });
      } catch (err) {
        if (err instanceof ApiError && err.status === 403) {
          setState({ status: "forbidden" });
          return;
        }
        setState({ status: "error", message: err instanceof Error ? err.message : "Không tải được hồ sơ." });
      }
    },
    [eventId],
  );

  useEffect(() => {
    void load();
  }, [load, attempt]);

  if (state.status === "loading") {
    return (
      <Card className="p-xl text-center space-y-md my-lg">
        <Spinner size="lg" className="mx-auto" />
        <p className="text-body-sm text-fg-muted">Đang tải hồ sơ thẩm định...</p>
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
  if (state.status === "not_found") {
    return (
      <Card className="p-xl text-center space-y-sm my-lg">
        <h1 className="text-heading-md font-bold text-fg">Không tìm thấy hồ sơ</h1>
        <p className="text-body-sm text-fg-muted">Sự kiện &ldquo;{eventId}&rdquo; không tồn tại hoặc đã bị xoá.</p>
      </Card>
    );
  }
  if (state.status === "error") {
    return (
      <div className="space-y-md my-lg">
        <Alert variant="error" title="Không tải được hồ sơ">
          {state.message}
        </Alert>
        <Button variant="secondary" onClick={() => setAttempt((n) => n + 1)}>
          Thử lại
        </Button>
      </div>
    );
  }

  const { event, review, axes, audit } = state.detail;

  return (
    <div className="space-y-lg py-md">
      <header className="space-y-xs">
        <p className="text-body-sm text-fg-muted">Thẩm định hồ sơ</p>
        <h1 className="text-heading-lg font-bold text-fg">{event.title}</h1>
        <div className="flex flex-wrap items-center gap-xs">
          <Badge variant="neutral">{event.status}</Badge>
          {review ? (
            <Badge variant={review.aiVerdict === "AUTO_REJECTED" ? "failed" : "pending"}>
              {VERDICT_LABELS[review.aiVerdict]}
            </Badge>
          ) : null}
        </div>
      </header>

      {conflict ? (
        <Alert variant="warning" title="Hồ sơ vừa thay đổi">
          Moderator khác vừa quyết định hồ sơ này. Dữ liệu dưới đây đã được tải lại.
        </Alert>
      ) : null}

      <OriginalContent detail={state.detail} />
      <AiAnalysis review={review} axes={axes} />

      <Section title="Quyết định của Moderator">
        {decided ? (
          <Alert variant="success" title={decided === "approved" ? "Đã duyệt hồ sơ" : "Đã từ chối hồ sơ"}>
            Quyết định đã được ghi nhật ký kiểm duyệt.
            {decided === "approved" ? " Sự kiện chuyển sang trạng thái đã lên lịch mở bán." : " Ban tổ chức sẽ thấy lý do."}
          </Alert>
        ) : (
          <DecisionPanel
            eventId={event.id}
            status={event.status}
            onDecided={(evt, decision) => {
              setDecided(decision);
              setState((s) =>
                s.status === "ready" ? { status: "ready", detail: { ...s.detail, event: { ...s.detail.event, status: evt.status } } } : s,
              );
            }}
            onConflict={() => {
              setConflict(true);
              void load(true);
            }}
          />
        )}
      </Section>

      <Section title="Lịch sử xử lý">
        {audit.length === 0 ? (
          <p className="text-body-sm text-fg-muted">Chưa có bản ghi nào.</p>
        ) : (
          <ol className="space-y-sm">
            {audit.map((a) => (
              <li key={a.id} className="border-l-2 border-border pl-sm">
                <p className="text-body-sm text-fg">
                  <span className="font-semibold">{auditLabel(a.action)}</span>
                  <span className="text-fg-muted"> · {actorLabel(a.actorType)} · {formatDateTimeVn(a.createdAt)}</span>
                </p>
                {a.reason ? <p className="text-body-sm text-fg-muted">Lý do: {a.reason}</p> : null}
              </li>
            ))}
          </ol>
        )}
      </Section>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  const id = useId();
  return (
    <Card className="p-md space-y-sm">
      <section aria-labelledby={id} className="space-y-sm">
        <h2 id={id} className="text-heading-sm font-semibold text-fg">
          {title}
        </h2>
        {children}
      </section>
    </Card>
  );
}

function OriginalContent({ detail }: { detail: ModerationDetail }) {
  const { event } = detail;
  return (
    <Section title="Nội dung gốc từ ban tổ chức">
      <p className="whitespace-pre-line text-body-md text-fg">{event.description || "Ban tổ chức chưa nhập mô tả."}</p>
      <dl className="grid gap-sm sm:grid-cols-3 text-body-sm">
        <div>
          <dt className="text-fg-muted">Địa điểm</dt>
          <dd className="text-fg">{event.venue || "—"}</dd>
        </div>
        <div>
          <dt className="text-fg-muted">Bắt đầu</dt>
          <dd className="text-fg">{formatDateTimeVn(event.startsAt)}</dd>
        </div>
        <div>
          <dt className="text-fg-muted">Mở bán</dt>
          <dd className="text-fg">{formatDateTimeVn(event.saleStartAt)}</dd>
        </div>
      </dl>
      {event.tiers.length > 0 ? (
        <table className="w-full text-body-sm">
          <caption className="sr-only">Hạng vé và hạn ngạch</caption>
          <thead>
            <tr className="text-left text-fg-muted">
              <th scope="col" className="py-xs">Hạng vé</th>
              <th scope="col" className="py-xs">Giá</th>
              <th scope="col" className="py-xs text-right">Hạn ngạch</th>
            </tr>
          </thead>
          <tbody>
            {event.tiers.map((t) => (
              <tr key={t.id} className="border-t border-border">
                <td className="py-xs text-fg">{t.name}</td>
                <td className="py-xs text-fg">{formatMoney(t.priceCents, t.currency)}</td>
                <td className="py-xs text-right text-fg">{t.quota}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <p className="text-body-sm text-fg-muted">Chưa có hạng vé.</p>
      )}
    </Section>
  );
}

function AiAnalysis({ review, axes }: { review: ModerationDetail["review"]; axes: Record<string, number> }) {
  const keys = [...AXIS_ORDER.filter((k) => k in axes), ...Object.keys(axes).filter((k) => !AXIS_ORDER.includes(k))];
  return (
    <Section title="Phân tích AI">
      <Alert variant="info">
        Kết quả AI chỉ là khuyến nghị để tham khảo. Quyết định duyệt hay từ chối thuộc về Moderator.
      </Alert>
      {review ? (
        <>
          <p className="text-body-sm text-fg">
            {VERDICT_LABELS[review.aiVerdict]} · Rủi ro tổng <strong>{percent(review.aiScore)}%</strong>
          </p>
          {review.aiReasons.length > 0 ? (
            <ul className="list-disc space-y-xs pl-lg text-body-sm text-fg">
              {review.aiReasons.map((r, i) => (
                <li key={i}>{r}</li>
              ))}
            </ul>
          ) : null}
        </>
      ) : (
        <p className="text-body-sm text-fg-muted">Chưa có kết quả phân tích AI cho hồ sơ này.</p>
      )}
      {keys.length > 0 ? (
        <div className="space-y-sm">
          {keys.map((k) => {
            const label = AXIS_LABELS[k] ?? k;
            const value = percent(axes[k]);
            return (
              <div key={k} className="space-y-xs">
                <div className="flex justify-between text-body-sm">
                  <span className="text-fg">{label}</span>
                  <span className="text-fg-muted">{value}%</span>
                </div>
                <div
                  role="meter"
                  aria-label={label}
                  aria-valuemin={0}
                  aria-valuemax={100}
                  aria-valuenow={value}
                  className="h-2 w-full overflow-hidden rounded-full bg-neutral-soft"
                >
                  <div
                    className={value >= 80 ? "h-full bg-danger" : value >= 30 ? "h-full bg-warning" : "h-full bg-success"}
                    style={{ width: `${value}%` }}
                  />
                </div>
              </div>
            );
          })}
        </div>
      ) : null}
    </Section>
  );
}
