"use client";

import Link from "next/link";

import { Alert, Badge, Button, Card, Spinner } from "@/components/ui";
import { useOrderStatus } from "@/hooks/use-order-status";
import { cn } from "@/lib/cn";
import { formatMoney } from "@/lib/money";
import type { OrderDetails } from "@/lib/payment-client";

export interface OrderResultViewProps {
  eventId: string;
  orderId: string;
  initialOrder?: OrderDetails | null;
  apiBaseUrl?: string;
  className?: string;
}

export function OrderResultView({
  eventId,
  orderId,
  initialOrder,
  apiBaseUrl,
  className,
}: OrderResultViewProps) {
  const { order, status, loading, refresh } = useOrderStatus({
    orderId,
    eventId,
    initialDetails: initialOrder,
    apiBaseUrl,
  });

  const eventTitle = order?.eventTitle || "Sự kiện EvenFlow";
  const venue = order?.venue || "Địa điểm tổ chức";
  const tierName = order?.tierName || "Vé tham dự";
  const quantity = order?.quantity || 1;
  const totalAmount = order?.totalAmount || 0;

  const handlePrint = () => {
    if (typeof window !== "undefined") {
      window.print();
    }
  };

  return (
    <div className={cn("mx-auto max-w-content space-y-lg py-md", className)}>
      <header className="space-y-xs">
        <h1 className="text-display-sm text-fg">Kết quả đơn hàng</h1>
        <p className="text-body-md text-fg-muted">
          Mã đơn: <span className="font-mono font-semibold text-fg">{orderId}</span> &bull; {eventTitle}
        </p>
      </header>

      {/* 1. Trạng thái PENDING: Webhook chậm hoặc đang chờ cổng phản hồi (AC-4, BR-O6) */}
      {status === "PENDING" && (
        <Card className="space-y-md p-xl text-center">
          <div className="flex justify-center">
            <Spinner size="lg" aria-label="Đang xác nhận" />
          </div>

          <div className="space-y-xs">
            <h2 className="text-headline-md text-fg">Đang xác nhận thanh toán</h2>
            <p className="mx-auto max-w-lg text-body-md text-fg-muted">
              Hệ thống đang chờ tín hiệu xác nhận giao dịch từ cổng thanh toán. Quá trình này có thể mất vài giây đến 1 phút.
            </p>
          </div>

          <Alert variant="info" className="mx-auto max-w-lg text-left">
            <p className="text-body-sm">
              <strong>Lưu ý:</strong> Vui lòng <strong>không</strong> đóng trình duyệt hoặc bấm thanh toán lại để tránh trừ tiền trùng lặp (BR-O5). Trang sẽ tự động cập nhật ngay khi nhận được xác nhận.
            </p>
          </Alert>

          <div className="pt-sm">
            <Button
              variant="secondary"
              loading={loading}
              onClick={refresh}
              className="text-label-md"
            >
              Kiểm tra trạng thái ngay
            </Button>
          </div>
        </Card>
      )}

      {/* 2. Trạng thái PAID / ISSUED: Thanh toán thành công & Vé điện tử (AC-5, BR-O8) */}
      {status === "PAID" && (
        <div className="space-y-lg">
          <Card className="border-success/30 bg-success/5 p-lg">
            <div className="flex flex-col gap-sm md:flex-row md:items-center md:justify-between">
              <div className="space-y-xs">
                <div className="flex items-center gap-xs">
                  <Badge variant="paid">Đã thanh toán thành công</Badge>
                  <span className="text-caption text-fg-muted">
                    {order?.paidAt ? new Date(order.paidAt).toLocaleString("vi-VN") : "Vừa xong"}
                  </span>
                </div>
                <h2 className="text-headline-lg text-fg">Vé điện tử của bạn đã sẵn sàng!</h2>
                <p className="text-body-md text-fg-muted">
                  Thông tin vé đã được lưu vào hệ thống EvenFlow và gửi tới email của bạn.
                </p>
              </div>

              <div className="flex flex-wrap gap-xs">
                <Button variant="secondary" onClick={handlePrint}>
                  In / Tải vé
                </Button>
                <Link
                  href="/"
                  className="ef-focus-ring inline-flex items-center justify-center rounded-control border border-border-strong bg-surface px-md py-sm text-label-md text-fg hover:bg-surface-subtle"
                >
                  Về trang chủ
                </Link>
              </div>
            </div>
          </Card>

          {/* Chi tiết đơn & Danh sách vé điện tử kèm mã QR demo an toàn (BR-O8) */}
          <section aria-label="Danh sách vé điện tử" className="space-y-md">
            <h3 className="text-headline-sm text-fg">Vé điện tử của bạn ({quantity} vé)</h3>

            <div className="grid grid-cols-1 gap-md md:grid-cols-2">
              {(order?.issuedTickets && order.issuedTickets.length > 0
                ? order.issuedTickets
                : Array.from({ length: quantity }).map((_, idx) => ({
                    ticketId: `tkt_${orderId}_${idx + 1}`,
                    ticketCode: `EF-${orderId.slice(-6).toUpperCase()}-${idx + 1}`,
                    tierName,
                    attendeeName: "Khách tham dự",
                    qrPayload: `DEMO-TICKET-${orderId}-${idx + 1}`,
                  }))
              ).map((ticket) => (
                <Card key={ticket.ticketId} className="flex flex-col justify-between border-border-strong p-lg">
                  <div className="space-y-sm">
                    <div className="flex items-center justify-between border-b border-border pb-xs">
                      <span className="text-label-md font-semibold text-primary">{ticket.tierName}</span>
                      <span className="font-mono text-caption text-fg-muted">{ticket.ticketCode}</span>
                    </div>

                    <div className="space-y-xxs">
                      <h4 className="text-headline-sm text-fg">{eventTitle}</h4>
                      <p className="text-body-sm text-fg-muted">{venue}</p>
                    </div>

                    <p className="text-body-sm text-fg">
                      Người sử dụng: <span className="font-medium">{ticket.attendeeName}</span>
                    </p>
                  </div>

                  {/* Khung mã QR demo an toàn (BR-O8, Stitch 10.10: visual placeholder, no PII) */}
                  <div className="mt-md flex flex-col items-center justify-center rounded-control border border-dashed border-border bg-surface-subtle p-md text-center">
                    <div
                      aria-label={`Mã QR vé ${ticket.ticketCode}`}
                      className="mb-xs flex size-32 items-center justify-center rounded-card border border-border bg-surface shadow-sm"
                    >
                      {/* SVG QR Placeholder Demo */}
                      <svg
                        className="size-24 text-fg"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        strokeWidth="1.5"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        aria-hidden="true"
                      >
                        <rect x="3" y="3" width="7" height="7" />
                        <rect x="14" y="3" width="7" height="7" />
                        <rect x="3" y="14" width="7" height="7" />
                        <rect x="14" y="14" width="3" height="3" />
                        <line x1="17" y1="17" x2="21" y2="17" />
                        <line x1="21" y1="17" x2="21" y2="21" />
                        <line x1="17" y1="21" x2="17" y2="21" />
                      </svg>
                    </div>
                    <span className="font-mono text-caption text-fg-muted">{ticket.ticketCode}</span>
                    <span className="text-caption text-fg-subtle">
                      Xuất trình mã này tại cổng sự kiện để check-in &bull; DEMO
                    </span>
                  </div>
                </Card>
              ))}
            </div>
          </section>
        </div>
      )}

      {/* 3. Trạng thái FAILED: Thanh toán thất bại (AC-6) */}
      {status === "FAILED" && (
        <Card className="space-y-md p-xl text-center">
          <Alert variant="error" title="Thanh toán không thành công" className="text-left">
            <p className="mb-xs text-body-sm">
              {order?.failureReason ||
                "Giao dịch bị từ chối hoặc không thể xử lý bởi cổng thanh toán. Tài khoản của bạn chưa bị trừ tiền."}
            </p>
            <p className="text-caption text-fg-muted">
              Mã giao dịch lỗi: <span className="font-mono">{orderId}</span>
            </p>
          </Alert>

          <div className="flex justify-center gap-sm pt-sm">
            <Link
              href={`/checkout/${eventId}/payment?orderId=${orderId}`}
              className="ef-focus-ring inline-flex items-center justify-center rounded-control border border-transparent bg-primary px-md py-sm text-label-md text-primary-fg hover:bg-primary-hover"
            >
              Thử thanh toán lại
            </Link>
            <Link
              href={`/checkout/${eventId}`}
              className="ef-focus-ring inline-flex items-center justify-center rounded-control border border-border-strong bg-surface px-md py-sm text-label-md text-fg hover:bg-surface-subtle"
            >
              Chọn lại vé
            </Link>
          </div>
        </Card>
      )}

      {/* 4. Trạng thái EXPIRED: Quá hạn giữ ghế giữa lúc thanh toán (AC-3, AC-6, BR-O2) */}
      {status === "EXPIRED" && (
        <Card className="space-y-md p-xl text-center">
          <Alert
            variant="error"
            title="Phiên giữ vé đã hết hạn (BR-O2)"
            className="text-left"
          >
            <p className="text-body-sm">
              Thời gian giữ chỗ 10:00 cho đơn hàng này đã kết thúc trước khi thanh toán hoàn tất. Toàn bộ số vé đã được tự động hoàn trả lại kho theo quy tắc chống oversell.
            </p>
          </Alert>

          <p className="text-body-sm text-fg-muted">
            Bạn không bị trừ bất kỳ khoản tiền nào cho giao dịch này. Vui lòng quay lại màn hình chọn vé để bắt đầu lượt mới.
          </p>

          <div className="pt-sm">
            <Link
              href={`/checkout/${eventId}`}
              className="ef-focus-ring inline-flex items-center justify-center rounded-control border border-transparent bg-primary px-lg py-sm text-label-md text-primary-fg hover:bg-primary-hover"
            >
              Quay lại chọn vé
            </Link>
          </div>
        </Card>
      )}

      {/* 5. Trạng thái CANCELLED: Đơn hàng đã huỷ */}
      {status === "CANCELLED" && (
        <Card className="space-y-md p-xl text-center">
          <Alert variant="warning" title="Đơn hàng đã được huỷ" className="text-left">
            <p className="text-body-sm">
              Đơn hàng này đã được đánh dấu huỷ. Nếu bạn cần hỗ trợ, vui lòng liên hệ trung tâm chăm sóc khách hàng.
            </p>
          </Alert>

          <div className="pt-sm">
            <Link
              href="/"
              className="ef-focus-ring inline-flex items-center justify-center rounded-control border border-border-strong bg-surface px-md py-sm text-label-md text-fg hover:bg-surface-subtle"
            >
              Về trang chủ
            </Link>
          </div>
        </Card>
      )}

      {/* Tóm tắt thanh toán chung */}
      <section aria-label="Tóm tắt thanh toán" className="mt-lg border-t border-border pt-md">
        <div className="flex flex-col justify-between gap-sm text-body-sm text-fg-muted sm:flex-row sm:items-center">
          <span>
            Hạng vé: <strong className="text-fg">{tierName}</strong> ({quantity} vé)
          </span>
          <span>
            Tổng thanh toán: <strong className="text-headline-sm text-fg">{formatMoney(totalAmount, "VND")}</strong>
          </span>
        </div>
      </section>
    </div>
  );
}
