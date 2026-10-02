import { Badge, Button, Card } from "@/components/ui";

/** <main> va ThemeToggle bay gio thuoc PublicShell (layout cua route group (marketing)),
 *  khong con o day — neu de ca hai noi thi trang co 2 <main> va 2 nut doi theme. */
export default function Home() {
  return (
    <div className="flex max-w-3xl flex-col gap-lg">
      <h1 className="text-headline-xl text-fg">EventFlow</h1>

      <p className="text-body-lg text-fg-muted">
        Nền tảng bán vé sự kiện chịu tải đột biến, có phòng chờ ảo và AI hỗ trợ vận hành. Xem{" "}
        <code className="rounded-sm bg-surface-subtle px-xs text-body-md">docs/</code> để biết kế
        hoạch và backlog.
      </p>

      <Card
        header="Trạng thái dự án"
        footer="Design system EVF-1801 — token và component dùng chung đã sẵn sàng."
      >
        <ul className="flex flex-col gap-sm text-body-md text-fg">
          <li className="flex items-center justify-between gap-md">
            <span>Phòng chờ ảo</span>
            <Badge variant="admitted">Đã có mã</Badge>
          </li>
          <li className="flex items-center justify-between gap-md">
            <span>Bán vé &amp; giữ chỗ</span>
            <Badge variant="admitted">Đã có mã</Badge>
          </li>
          <li className="flex items-center justify-between gap-md">
            <span>Identity, Event, Payment, Anti-bot</span>
            <Badge variant="pending">Đang là stub</Badge>
          </li>
        </ul>
      </Card>

      <div className="flex flex-wrap gap-sm">
        <Button>Xem sự kiện</Button>
        <Button variant="secondary">Tài liệu</Button>
      </div>
    </div>
  );
}
