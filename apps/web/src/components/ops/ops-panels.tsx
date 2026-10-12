/** Cac khoi hien thi cua war room Ops (EV-187). Chi hien thi, khong goi service.
 *
 * Quy tac "khong to xanh gia" (AC-3): nguon `unavailable` -> o hien "Không có dữ liệu";
 * `stale` -> o van hien so nhung gan nhan "(cũ)". Khong co chi bao "binh thuong" nao. */

import type { ReactNode } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { formatCount, formatDecimal } from "@/lib/format";
import {
  OPS_POLL_INTERVAL_MS,
  tierInventoryViolation,
  type DegradationMode,
  type OpsWarRoomSnapshot,
  type SourceHealth,
} from "@/lib/ops-service";

const CONTROLS_NOTE_ID = "ops-controls-note";

/** Dong phu moi khoi: so lieu tuc thoi, khong phai lich su. */
const LIVE_NOTE = `Số liệu tức thời · làm mới mỗi ${OPS_POLL_INTERVAL_MS / 1000} giây`;

function StaleMark() {
  return <span className="text-warning-fg"> (cũ)</span>;
}

function NoData() {
  return <span className="text-body-md text-fg-muted">Không có dữ liệu</span>;
}

function PanelHeading({ title, health }: { title: string; health?: SourceHealth }) {
  return (
    <div className="space-y-xs">
      <h3 className="text-headline-sm font-semibold text-fg">
        {title}
        {health === "stale" ? <StaleMark /> : null}
      </h3>
      <p className="text-body-sm text-fg-muted">{LIVE_NOTE}</p>
    </div>
  );
}

interface MetricProps {
  label: string;
  health: SourceHealth;
  value: ReactNode;
  unit?: string;
}

/** Mot chi so: nhan, so, don vi (so va don vi o hai phan tu rieng). */
function Metric({ label, health, value, unit }: MetricProps) {
  return (
    <div className="space-y-xs">
      <p className="text-body-md text-fg-muted">
        {label}
        {health === "stale" ? <StaleMark /> : null}
      </p>
      {health === "unavailable" ? (
        <NoData />
      ) : (
        <p className="flex items-baseline gap-xs">
          <span className="text-headline-md font-bold text-fg">{value}</span>
          {unit ? <span className="text-body-md text-fg-muted">{unit}</span> : null}
        </p>
      )}
    </div>
  );
}

/** O so trong bang ton kho. */
function NumCell({ value }: { value: number }) {
  return <td className="py-xs pr-md text-right">{formatCount(value)}</td>;
}

type FunnelKey = keyof OpsWarRoomSnapshot["funnel"];

const FUNNEL_STEPS: {
  key: FunnelKey;
  label: string;
  unit: string;
  source: "waitingroom" | "ticketing";
}[] = [
  { key: "lobby", label: "Sảnh chờ", unit: "khách", source: "waitingroom" },
  { key: "queued", label: "Trong hàng chờ", unit: "khách", source: "waitingroom" },
  { key: "admitted", label: "Đã được vào", unit: "khách", source: "waitingroom" },
  { key: "holding", label: "Đang giữ vé", unit: "vé", source: "ticketing" },
  { key: "paid", label: "Đã thanh toán", unit: "vé", source: "ticketing" },
  { key: "issued", label: "Đã phát hành", unit: "vé", source: "ticketing" },
];

export function OpsFunnelPanel({ snapshot }: { snapshot: OpsWarRoomSnapshot }) {
  return (
    <Card className="space-y-md">
      <PanelHeading title="Phễu chuyển đổi" />
      <ol className="grid grid-cols-2 gap-md md:grid-cols-3 xl:grid-cols-6">
        {FUNNEL_STEPS.map((step, index) => (
          <li key={step.key}>
            <Metric
              label={`${index + 1}. ${step.label}`}
              health={snapshot.sources[step.source]}
              value={formatCount(snapshot.funnel[step.key])}
              unit={step.unit}
            />
          </li>
        ))}
      </ol>
    </Card>
  );
}

export function OpsAdmissionPanel({ snapshot }: { snapshot: OpsWarRoomSnapshot }) {
  const health = snapshot.sources.waitingroom;
  // Nhan "(cũ)" da nam o tieu de khoi; o con chi can biet co du lieu hay khong.
  const metricHealth = health === "stale" ? "ok" : health;
  const { admission } = snapshot;

  return (
    <Card className="space-y-md">
      <PanelHeading title="Tuyển sinh hàng chờ" health={health} />
      <div className="grid grid-cols-1 gap-md sm:grid-cols-3">
        <Metric
          label="Admit rate"
          health={metricHealth}
          value={formatDecimal(admission.ratePerSec)}
          unit="khách/giây"
        />
        <div className="space-y-xs">
          <p className="text-body-md text-fg-muted">Chế độ</p>
          {health === "unavailable" ? (
            <NoData />
          ) : (
            <Badge variant={admission.mode === "MANUAL" ? "pending" : "neutral"}>
              {admission.mode}
            </Badge>
          )}
        </div>
        <Metric
          label="Độ sâu hàng chờ"
          health={metricHealth}
          value={formatCount(admission.queueDepth)}
          unit="khách"
        />
      </div>
    </Card>
  );
}

export function OpsInventoryPanel({ snapshot }: { snapshot: OpsWarRoomSnapshot }) {
  const health = snapshot.sources.ticketing;
  // Nguon ticketing khong `ok` thi khong kiem BR-O1: so lieu co the cu hoac khong co.
  const checked = health === "ok";

  return (
    <Card className="space-y-md">
      <PanelHeading title="Tồn kho theo hạng vé" health={health} />
      {health === "unavailable" ? (
        <NoData />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-left text-body-md">
            <caption className="sr-only">Tồn kho từng hạng vé của sự kiện</caption>
            <thead>
              <tr className="border-b border-border text-fg-muted">
                <th scope="col" className="py-xs pr-md font-medium">Hạng vé</th>
                <th scope="col" className="py-xs pr-md text-right font-medium">Quota</th>
                <th scope="col" className="py-xs pr-md text-right font-medium">Đang giữ</th>
                <th scope="col" className="py-xs pr-md text-right font-medium">Đã thanh toán</th>
                <th scope="col" className="py-xs pr-md text-right font-medium">Đã phát hành</th>
                <th scope="col" className="py-xs pr-md text-right font-medium">Còn lại</th>
                <th scope="col" className="py-xs font-medium">Trạng thái</th>
              </tr>
            </thead>
            <tbody>
              {snapshot.inventory.length === 0 ? (
                <tr>
                  <td colSpan={7} className="py-xs text-fg-muted">
                    Chưa có hạng vé.
                  </td>
                </tr>
              ) : null}
              {snapshot.inventory.map((tier) => {
                const violation = checked ? tierInventoryViolation(tier) : null;
                return (
                  <tr key={tier.tierId} className="border-b border-border last:border-0">
                    <th scope="row" className="py-xs pr-md font-medium text-fg">{tier.name}</th>
                    <NumCell value={tier.quota} />
                    <NumCell value={tier.held} />
                    <NumCell value={tier.paid} />
                    <NumCell value={tier.issued} />
                    <NumCell value={tier.available} />
                    <td className="py-xs">
                      {violation === "INVALID_DATA" ? (
                        <Badge variant="failed">Dữ liệu không hợp lệ</Badge>
                      ) : violation === "OVER_QUOTA" ? (
                        <Badge variant="failed">Vượt quota</Badge>
                      ) : violation === "NEGATIVE_AVAILABLE" ? (
                        <Badge variant="failed">Còn lại âm</Badge>
                      ) : (
                        <span className="text-fg-muted">—</span>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}

const UNKNOWN_AI_IMPACT = "Không rõ tác động tới người mua.";

const AI_IMPACT: Record<DegradationMode, string> = {
  NORMAL: "Trợ lý trả lời đầy đủ.",
  SAVING: "Trợ lý trả lời ngắn gọn hơn để tiết kiệm tài nguyên.",
  FAQ_ONLY: "Trợ lý chỉ trả lời các câu hỏi thường gặp.",
  OFF: "Trợ lý tạm tắt, người mua được hướng dẫn liên hệ CSKH.",
};

export function OpsAiPanel({ snapshot }: { snapshot: OpsWarRoomSnapshot }) {
  const health = snapshot.sources.ai;
  const { ai } = snapshot;
  const metricHealth = health === "stale" ? "ok" : health;

  return (
    <Card className="space-y-md">
      <PanelHeading title="Trợ lý AI" health={health} />
      {health === "unavailable" ? (
        <NoData />
      ) : (
        <>
          <div className="space-y-xs">
            <p className="text-body-md text-fg-muted">Nấc suy biến</p>
            <p className="flex flex-wrap items-center gap-xs">
              <Badge variant={ai.mode === "NORMAL" ? "neutral" : "pending"}>{ai.mode}</Badge>
              {ai.manualOverride ? <Badge variant="pending">Ops đặt tay</Badge> : null}
            </p>
            <p className="text-body-md text-fg">{AI_IMPACT[ai.mode] ?? UNKNOWN_AI_IMPACT}</p>
          </div>
          <div className="grid grid-cols-1 gap-md sm:grid-cols-2">
            <Metric
              label="Độ trễ hàng đợi chat"
              health={metricHealth}
              value={`${formatDecimal(ai.chatQueueLagSeconds)} giây`}
            />
            <Metric
              label="Ngân sách token đã dùng"
              health={metricHealth}
              value={
                Number.isFinite(ai.budgetUsedRatio) ? `${Math.round(ai.budgetUsedRatio * 100)}%` : "—"
              }
            />
          </div>
        </>
      )}
    </Card>
  );
}

/** Khu "Điều khiển" tach rieng khoi giam sat. Chua co Ops API ghi de (EVF-60) nen moi nut
 *  bi vo hieu va khong gui request nao (AC-6). */
export function OpsControlsPanel() {
  return (
    <Card className="space-y-md">
      <div className="flex flex-wrap gap-sm">
        <Button variant="secondary" disabled aria-describedby={CONTROLS_NOTE_ID}>
          Ghi đè admit rate
        </Button>
        <Button variant="secondary" disabled aria-describedby={CONTROLS_NOTE_ID}>
          Tạm dừng hàng chờ
        </Button>
      </div>
      <p id={CONTROLS_NOTE_ID} className="text-body-md text-fg-muted">
        Chưa có API ghi đè (EVF-60)
      </p>
    </Card>
  );
}
