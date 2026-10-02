import { Row, Section } from "./section";

/** Khong hard-code mau o day: moi o mau duoc ve bang chinh utility sinh ra tu token,
 *  nen neu token doi thi trang nay doi theo. Ten token hien kem de tra cuu. */
const SURFACES = [
  { token: "bg", className: "bg-bg" },
  { token: "surface", className: "bg-surface" },
  { token: "surface-subtle", className: "bg-surface-subtle" },
  { token: "border", className: "bg-border" },
  { token: "border-strong", className: "bg-border-strong" },
  { token: "brand", className: "bg-brand" },
];

const SEMANTIC = [
  { token: "primary", className: "bg-primary" },
  { token: "accent", className: "bg-accent" },
  { token: "success", className: "bg-success" },
  { token: "danger", className: "bg-danger" },
  { token: "info-soft", className: "bg-info-soft" },
  { token: "success-soft", className: "bg-success-soft" },
  { token: "warning-soft", className: "bg-warning-soft" },
  { token: "danger-soft", className: "bg-danger-soft" },
  { token: "neutral-soft", className: "bg-neutral-soft" },
];

const TYPE_SCALE = [
  "text-headline-xl",
  "text-headline-lg",
  "text-headline-md",
  "text-headline-sm",
  "text-body-lg",
  "text-body-md",
  "text-body-sm",
  "text-label-lg",
  "text-label-md",
  "text-label-sm",
];

const RADII = [
  { token: "control (10px)", className: "rounded-control" },
  { token: "card (16px)", className: "rounded-card" },
  { token: "panel (20px)", className: "rounded-panel" },
  { token: "pill", className: "rounded-full" },
];

function Swatch({ token, className }: { token: string; className: string }) {
  return (
    <div className="flex w-28 flex-col gap-xs">
      <div className={`h-12 w-full rounded-control border border-border ${className}`} />
      <code className="text-body-sm text-fg-muted">{token}</code>
    </div>
  );
}

export function TokenGallery() {
  return (
    <Section
      title="Design token"
      description="Mọi màu trong hệ thống chỉ được định nghĩa ở src/styles/tokens.css. Các ô dưới đây vẽ bằng chính utility sinh ra từ token, nên chúng đổi theo theme — bấm nút đổi giao diện ở đầu trang để xem bảng dark."
    >
      <Row label="Bề mặt">
        {SURFACES.map((item) => (
          <Swatch key={item.token} {...item} />
        ))}
      </Row>

      <Row label="Semantic">
        {SEMANTIC.map((item) => (
          <Swatch key={item.token} {...item} />
        ))}
      </Row>

      <div className="flex flex-col gap-sm">
        <span className="text-label-md text-fg-muted">Typography</span>
        <div className="flex flex-col gap-xs">
          {TYPE_SCALE.map((cls) => (
            <div key={cls} className="flex flex-wrap items-baseline gap-md">
              <span className={`${cls} text-fg`}>EventFlow — mở bán lúc 20:00</span>
              <code className="text-body-sm text-fg-muted">{cls}</code>
            </div>
          ))}
          <div className="flex flex-wrap items-baseline gap-md">
            <span className="text-numeric-timer tabular-nums text-fg">09:30</span>
            <code className="text-body-sm text-fg-muted">text-numeric-timer (tabular)</code>
          </div>
          <div className="flex flex-wrap items-baseline gap-md">
            <span className="text-numeric-metric tabular-nums text-fg">1.248</span>
            <code className="text-body-sm text-fg-muted">text-numeric-metric (tabular)</code>
          </div>
        </div>
      </div>

      <Row label="Bo góc">
        {RADII.map((item) => (
          <div key={item.token} className="flex w-32 flex-col gap-xs">
            <div className={`h-12 w-full border border-border-strong bg-surface ${item.className}`} />
            <code className="text-body-sm text-fg-muted">{item.token}</code>
          </div>
        ))}
      </Row>
    </Section>
  );
}
