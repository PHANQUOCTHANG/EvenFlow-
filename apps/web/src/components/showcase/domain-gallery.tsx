"use client";

import { useEffect, useState } from "react";

import { OrderSummary, type HoldState } from "@/components/checkout/order-summary";
import { TicketTierCard } from "@/components/checkout/ticket-tier-card";
import { EventCard, type EventPublicState } from "@/components/event/event-card";
import { QueueStatusPanel, type QueueState } from "@/components/queue/queue-status-panel";
import { ServerExpiryCountdown } from "@/components/ui";

import { Row, Section } from "./section";

const QUEUE_STATES: QueueState[] = [
  "LOBBY",
  "QUEUED",
  "ADMITTED",
  "EXPIRED",
  "SOLD_OUT",
  "RECONNECTING",
  "DROPPED",
  "UNKNOWN",
];

const EVENT_STATES: EventPublicState[] = [
  "SCHEDULED",
  "ON_SALE",
  "SOLD_OUT",
  "CLOSED",
  "COMPLETED",
  "CANCELLED",
];

const HOLD_STATES: HoldState[] = [
  "idle",
  "creating",
  "active",
  "expired",
  "sold_out",
  "ambiguous",
];

const DEMO_ITEMS = [
  { tierName: "Vé thường (mẫu)", quantity: 2, unitAmount: 650_000, currency: "VND" },
  { tierName: "Vé VIP (mẫu)", quantity: 1, unitAmount: 1_250_000, currency: "VND" },
];

interface Deadlines {
  hold: number;
  admission: number;
  saleStart: number;
  nearlyOver: number;
  past: number;
}

/** Moc thoi han demo sinh SAU KHI MOUNT, khong tinh trong than render.
 *
 *  Trang nay prerender tinh. Neu tinh `Date.now() + 10 phut` ngay trong component thi moc cua
 *  THOI DIEM BUILD bi nuong vao HTML, va nguoi mo link hom sau se thay moi dong ho da het han.
 *  Day dung la loi da sua trong `useServerCountdown`; trang showcase khong duoc tai tao no o
 *  tang tren. */
function useDemoDeadlines(): Deadlines | null {
  const [deadlines, setDeadlines] = useState<Deadlines | null>(null);

  useEffect(() => {
    const now = Date.now();
    setDeadlines({
      hold: now + 10 * 60_000,
      admission: now + 15 * 60_000,
      saleStart: now + 3 * 60 * 60_000,
      nearlyOver: now + 90_000,
      past: now - 60_000,
    });
  }, []);

  return deadlines;
}

function Pending({ label }: { label: string }) {
  return (
    <p className="text-body-sm text-fg-muted" role="status">
      {label}
    </p>
  );
}

export function DomainGallery() {
  const deadlines = useDemoDeadlines();

  return (
    <>
      <Section
        title="Phòng chờ ảo"
        description="QueueStatusPanel, 8 trạng thái. Lưu ý trạng thái LOBBY cố ý KHÔNG hiển thị số thứ tự: trước giờ mở bán không ai có rank, đúng T0 hệ thống mới xáo trộn rồi cấp — đây là cơ chế chống bot, không phải chi tiết trình bày. Mọi số dưới đây là dữ liệu mẫu."
      >
        <div className="grid gap-md md:grid-cols-2">
          {QUEUE_STATES.map((state) => (
            <QueueStatusPanel
              key={state}
              state={state}
              rank={1248}
              initialRank={5000}
              etaSeconds={state === "QUEUED" ? 240 : undefined}
              connectionState={state === "RECONNECTING" ? "reconnecting" : "live"}
              admissionExpiresAt={deadlines?.admission}
              admissionWarningThresholdMs={2 * 60_000}
            />
          ))}
        </div>
      </Section>

      <Section
        title="Đồng hồ theo giờ server"
        description="ServerExpiryCountdown, 3 biến thể. Suất mua 15 phút và giữ vé 10 phút là HAI đồng hồ khác nhau và không bao giờ được gộp, nên mỗi biến thể có nhãn cố định mà nơi gọi không đặt lại được. Đồng hồ đếm theo mốc tuyệt đối của server, không giảm dần, nên tab chạy nền bao lâu cũng không sai."
      >
        {deadlines ? (
          <Row label="Đang chạy / cảnh báo / đã hết hạn">
            <ServerExpiryCountdown variant="sale-start" expiresAt={deadlines.saleStart} />
            <ServerExpiryCountdown variant="admission" expiresAt={deadlines.admission} />
            <ServerExpiryCountdown variant="hold" expiresAt={deadlines.hold} />
            <ServerExpiryCountdown
              variant="hold"
              expiresAt={deadlines.nearlyOver}
              warningThresholdMs={2 * 60_000}
            />
            <ServerExpiryCountdown variant="hold" expiresAt={deadlines.past} />
          </Row>
        ) : (
          <Pending label="Đang khởi tạo mốc thời gian mẫu…" />
        )}
      </Section>

      <Section
        title="Thẻ sự kiện"
        description="EventCard, 6 trạng thái công khai theo state machine nghiệp vụ. Giờ hiển thị là giờ địa điểm, không đổi theo múi giờ người xem. Tên sự kiện và thời gian là dữ liệu mẫu."
      >
        <div className="grid gap-md sm:grid-cols-2 lg:grid-cols-3">
          {EVENT_STATES.map((state) => (
            <EventCard
              key={state}
              state={state}
              title={`Đêm nhạc mẫu — ${state}`}
              venue="Nhà hát Lớn (mẫu)"
              startsAt={deadlines?.saleStart}
              href={state === "ON_SALE" ? "/" : undefined}
            />
          ))}
          <EventCard state="ON_SALE" title="Đang tải" loading />
        </div>
      </Section>

      <Section
        title="Chọn vé"
        description="TicketTierCard. Không có prop số vé còn lại: tồn kho chỉ hiển thị bằng nhãn khái quát, vì một con số sai sớm vài chục giây là lời hứa sai với người mua. Giá là dữ liệu mẫu."
      >
        <div className="grid gap-md md:grid-cols-2 lg:grid-cols-3">
          <TicketTierCard
            name="Vé thường (mẫu)"
            unitAmount={650_000}
            currency="VND"
            availability="available"
            maxSelectable={4}
            quantity={2}
            selected
          />
          <TicketTierCard
            name="Vé VIP (mẫu)"
            unitAmount={1_250_000}
            currency="VND"
            availability="limited"
            maxSelectable={4}
            quantity={0}
          />
          <TicketTierCard
            name="Vé hạng A (mẫu)"
            unitAmount={2_500_000}
            currency="VND"
            availability="sold_out"
          />
          <TicketTierCard
            name="Vé thường (mẫu)"
            unitAmount={650_000}
            currency="VND"
            availability="available"
            maxSelectable={4}
            hasActiveHoldElsewhere
          />
          <TicketTierCard
            name="Chưa biết tồn kho (mẫu)"
            unitAmount={650_000}
            currency="VND"
            maxSelectable={4}
          />
          <TicketTierCard
            name="Hết suất mua của bạn (mẫu)"
            unitAmount={650_000}
            currency="VND"
            availability="available"
            maxSelectable={0}
          />
        </div>
      </Section>

      <Section
        title="Tóm tắt đơn hàng"
        description="OrderSummary, 6 trạng thái giữ vé. Trạng thái 'ambiguous' là khi đã gửi yêu cầu giữ vé nhưng chưa có phản hồi: lúc đó giao diện không nói thành công, không nói thất bại, và không có nút nào tạo lượt giữ vé mới — bấm lại có thể giữ kho hai lần. Mọi số tiền là dữ liệu mẫu."
      >
        {deadlines ? (
          <div className="grid gap-md md:grid-cols-2 lg:grid-cols-3">
            {HOLD_STATES.map((holdState) => (
              <OrderSummary
                key={holdState}
                items={DEMO_ITEMS}
                holdState={holdState}
                holdExpiresAt={deadlines.hold}
                holdWarningThresholdMs={2 * 60_000}
              />
            ))}
          </div>
        ) : (
          <Pending label="Đang khởi tạo mốc thời gian mẫu…" />
        )}
      </Section>
    </>
  );
}
