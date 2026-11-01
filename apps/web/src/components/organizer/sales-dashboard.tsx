"use client";

/** Dashboard Doanh số Realtime cho Ban tổ chức (EV-186, EVF-116, Stitch 10.14, AC-4, AC-5).
 *
 * Tiêu chí nghiệm thu:
 * - AC-4: Cập nhật realtime trong <= 5s.
 * - Chỉ số trực quan: Doanh thu, Vé đã bán, Đơn hàng thành công, Vé đang giữ 10:00.
 * - Chi tiết từng hạng vé với thanh tiến độ lấp đầy (Fill rate %).
 * - AC-5: Báo cáo tổng kết sau bán (Post-sale report: AOV, Peak TPS, Cơ cấu thanh toán).
 * - ops-warroom AC-30: số liệu thật từ GET /v1/organizer/events/{id}/sales, tự làm mới mỗi 4s mặc định.
 *   Peak TPS chưa có nguồn nên hiện "Chưa có dữ liệu", không bịa số.
 */

import Link from "next/link";
import { useState } from "react";

import { Alert } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Spinner } from "@/components/ui/spinner";
import { useOrganizerSales } from "@/hooks/use-organizer-sales";
import { cn } from "@/lib/cn";
import { formatVnd } from "@/lib/format";
import type { OrganizerEvent, RealtimeSalesSnapshot } from "@/lib/organizer-service";

export interface SalesDashboardProps {
  event: OrganizerEvent;
  initialSalesData?: RealtimeSalesSnapshot | null;
  className?: string;
}

export function SalesDashboard({
  event,
  initialSalesData = null,
  className,
}: SalesDashboardProps) {
  const {
    data,
    isLoading,
    isRefreshing,
    error,
    lastUpdatedAt,
    autoRefresh,
    setAutoRefresh,
    refresh,
  } = useOrganizerSales({
    eventId: event.id,
    initialData: initialSalesData,
    // EVF-116 AC-2: cập nhật trong <= 5s, mặc định bật tự làm mới (4s).
    initialAutoRefresh: true,
  });

  const [activeTab, setActiveTab] = useState<"realtime" | "report">("realtime");

  return (
    <div className={cn("space-y-lg max-w-6xl mx-auto", className)}>
      {/* Header thanh công cụ & realtime status */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-md border-b border-border pb-md">
        <div>
          <div className="flex items-center gap-xs text-body-xs text-fg-muted">
            <Link href="/organizer/events" className="hover:text-primary transition-colors">
              ← Sự kiện của tôi
            </Link>
            <span>/</span>
            <span>Doanh số trực tiếp</span>
          </div>
          <h1 className="text-heading-xl font-bold text-fg mt-xs">{event.title}</h1>
          <p className="text-body-sm text-fg-muted mt-xs">
            📍 {event.venue} • Trạng thái:{" "}
            <span className="font-semibold text-fg">{event.state}</span>
          </p>
        </div>

        {/* Realtime control toolbar */}
        <div className="flex flex-wrap items-center gap-sm">
          <Badge variant="neutral" className="flex items-center gap-xs text-label-sm">
            Realtime (≤ 5s)
          </Badge>

          <Button
            variant={autoRefresh ? "secondary" : "tertiary"}
            onClick={() => setAutoRefresh(!autoRefresh)}
            className="text-body-xs"
            aria-pressed={autoRefresh}
          >
            {autoRefresh ? "⏸ Tắt tự động cập nhật" : "▶️ Bật tự động (4s)"}
          </Button>

          <Button
            variant="primary"
            onClick={refresh}
            disabled={isRefreshing}
            className="text-body-xs flex items-center gap-xs"
          >
            {isRefreshing && <Spinner size="sm" />}
            {isRefreshing ? "Đang đồng bộ..." : "🔄 Làm mới ngay"}
          </Button>
        </div>
      </div>

      {/* Freshness banner & errors */}
      {error && (
        <Alert variant="error" title="Lỗi cập nhật doanh số">
          {error.message}
        </Alert>
      )}

      <div className="flex flex-col sm:flex-row sm:items-center justify-between text-body-xs text-fg-muted bg-surface-elevated px-md py-xs rounded-control gap-xs">
        <span>
          🕒 Lần cập nhật gần nhất:{" "}
          <strong className="text-fg">
            {lastUpdatedAt ? lastUpdatedAt.toLocaleTimeString("vi-VN") : "Đang kết nối..."}
          </strong>
        </span>
        <span>Chu kỳ đồng bộ: ≤ 5s (Autorun: {autoRefresh ? "Bật" : "Tắt"})</span>
      </div>

      {/* Tab chuyển đổi: Realtime Dashboard vs Báo cáo sau bán */}
      <div className="flex gap-xs border-b border-border" role="tablist">
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === "realtime"}
          onClick={() => setActiveTab("realtime")}
          className={cn(
            "px-md py-sm font-semibold text-body-sm border-b-2 transition-colors",
            activeTab === "realtime"
              ? "border-primary text-primary"
              : "border-transparent text-fg-muted hover:text-fg",
          )}
        >
          📊 Doanh số &amp; Tồn kho Realtime
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === "report"}
          onClick={() => setActiveTab("report")}
          className={cn(
            "px-md py-sm font-semibold text-body-sm border-b-2 transition-colors",
            activeTab === "report"
              ? "border-primary text-primary"
              : "border-transparent text-fg-muted hover:text-fg",
          )}
        >
          📑 Báo cáo tổng kết sau bán
        </button>
      </div>

      {isLoading && !data ? (
        <Card className="p-xl text-center space-y-md">
          <Spinner size="lg" className="mx-auto" />
          <p className="text-body-sm text-fg-muted">Đang tải dữ liệu doanh số sự kiện...</p>
        </Card>
      ) : data ? (
        activeTab === "realtime" ? (
          <div className="space-y-lg" role="region" aria-label="Khu vực doanh số realtime">
            {/* 4 Thẻ chỉ số tổng quan (Metric Cards) */}
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-md">
              <Card className="p-md space-y-xs">
                <span className="text-body-xs text-fg-muted font-medium">Tổng doanh thu</span>
                <p className="text-heading-lg font-bold text-primary">
                  {formatVnd(data.grossRevenue)}
                </p>
                <span className="text-label-sm text-fg-muted">Đơn đã thanh toán / đã phát hành vé</span>
              </Card>

              <Card className="p-md space-y-xs">
                <span className="text-body-xs text-fg-muted font-medium">Tổng vé đã bán</span>
                <p className="text-heading-lg font-bold text-fg">
                  {data.totalTicketsSold.toLocaleString("vi-VN")}{" "}
                  <span className="text-body-sm text-fg-muted">
                    / {data.totalQuota.toLocaleString("vi-VN")}
                  </span>
                </p>
                <span className="text-label-sm text-fg-muted">
                  Tỷ lệ lấp đầy: <strong>{data.overallFillRatePercentage}%</strong>
                </span>
              </Card>

              <Card className="p-md space-y-xs">
                <span className="text-body-xs text-fg-muted font-medium">Đơn hàng thành công</span>
                <p className="text-heading-lg font-bold text-fg">
                  {data.paidOrdersCount.toLocaleString("vi-VN")}
                </p>
                <span className="text-label-sm text-fg-muted">Giao dịch thành công</span>
              </Card>

              <Card className="p-md space-y-xs">
                <span className="text-body-xs text-fg-muted font-medium">Vé đang giữ (Hold)</span>
                <p className="text-heading-lg font-bold text-warning-fg">
                  {data.activeHoldsCount.toLocaleString("vi-VN")}
                </p>
                <span className="text-label-sm text-warning-fg">
                  Thời hạn giữ ghế 10:00 (BR-O2)
                </span>
              </Card>
            </div>

            {/* Bảng chi tiết doanh số từng hạng vé */}
            <Card className="p-lg space-y-md">
              <div className="flex items-center justify-between">
                <h2 className="text-heading-md font-semibold text-fg">
                  Chi tiết doanh số theo từng hạng vé
                </h2>
                <span className="text-body-xs text-fg-muted">
                  {data.tierSales.length} hạng vé
                </span>
              </div>

              {data.tierSales.length === 0 || data.totalTicketsSold === 0 ? (
                <p className="text-body-md text-fg-muted">Chưa bán được vé nào cho sự kiện này.</p>
              ) : null}
              <div className="overflow-x-auto">
                <table className="w-full text-left text-body-sm border-collapse">
                  <thead>
                    <tr className="border-b border-border text-fg-muted text-label-sm uppercase">
                      <th className="py-sm px-sm font-semibold">Hạng vé</th>
                      <th className="py-sm px-sm font-semibold">Giá vé</th>
                      <th className="py-sm px-sm font-semibold">Đã bán / Hạn mức</th>
                      <th className="py-sm px-sm font-semibold">Đang giữ (Hold)</th>
                      <th className="py-sm px-sm font-semibold">Doanh thu</th>
                      <th className="py-sm px-sm font-semibold">Tỷ lệ lấp đầy</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-border">
                    {data.tierSales.map((tier) => (
                      <tr key={tier.tierId} className="hover:bg-neutral-soft/50 transition-colors">
                        <td className="py-md px-sm font-semibold text-fg">{tier.name}</td>
                        <td className="py-md px-sm text-fg">{formatVnd(tier.price)}</td>
                        <td className="py-md px-sm text-fg">
                          {tier.sold.toLocaleString("vi-VN")} /{" "}
                          <span className="text-fg-muted">{tier.quota.toLocaleString("vi-VN")}</span>
                        </td>
                        <td className="py-md px-sm text-warning-fg font-medium">
                          {tier.activeHolds.toLocaleString("vi-VN")}
                        </td>
                        <td className="py-md px-sm font-semibold text-primary">
                          {formatVnd(tier.revenue)}
                        </td>
                        <td className="py-md px-sm w-44">
                          <div className="space-y-xs">
                            <div className="flex justify-between text-body-xs">
                              <span>{tier.fillRatePercentage}%</span>
                            </div>
                            <div
                              className="w-full bg-neutral-soft rounded-full h-2 overflow-hidden"
                              role="progressbar"
                              aria-valuenow={tier.fillRatePercentage}
                              aria-valuemin={0}
                              aria-valuemax={100}
                              aria-label={`Tiến độ bán vé ${tier.name}`}
                            >
                              <div
                                className={cn(
                                  "h-full rounded-full transition-all duration-300",
                                  tier.fillRatePercentage >= 90
                                    ? "bg-danger"
                                    : tier.fillRatePercentage >= 50
                                      ? "bg-primary"
                                      : "bg-success",
                                )}
                                style={{ width: `${Math.min(100, tier.fillRatePercentage)}%` }}
                              />
                            </div>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </Card>
          </div>
        ) : (
          /* TAB BÁO CÁO SAU BÁN (AC-5) */
          <div className="space-y-lg" role="region" aria-label="Khu vực báo cáo sau bán">
            <div className="grid grid-cols-1 md:grid-cols-3 gap-md">
              <Card className="p-md space-y-xs">
                <span className="text-body-xs text-fg-muted font-medium">
                  Giá trị trung bình mỗi đơn (AOV)
                </span>
                <p className="text-heading-lg font-bold text-fg">
                  {formatVnd(data.postSaleReport.averageOrderValue)}
                </p>
                <span className="text-label-sm text-fg-muted">Doanh thu / Số đơn</span>
              </Card>

              <Card className="p-md space-y-xs">
                <span className="text-body-xs text-fg-muted font-medium">
                  Tốc độ giao dịch cao điểm
                </span>
                <p className="text-heading-lg font-bold text-fg-muted">—</p>
                <span className="text-label-sm text-fg-muted">Chưa có dữ liệu</span>
              </Card>

              <Card className="p-md space-y-xs">
                <span className="text-body-xs text-fg-muted font-medium">
                  Tỷ lệ hoàn thành đơn
                </span>
                <p className="text-heading-lg font-bold text-fg-muted">—</p>
                <span className="text-label-sm text-fg-muted">Chưa có dữ liệu</span>
              </Card>
            </div>

            {/* Phân bổ phương thức thanh toán */}
            <Card className="p-lg space-y-md">
              <h3 className="text-heading-sm font-semibold text-fg">
                Cơ cấu doanh thu theo phương thức thanh toán
              </h3>

              {data.postSaleReport.paymentBreakdown.length === 0 ? (
                <p className="text-body-md text-fg-muted">Chưa có đơn thanh toán thành công.</p>
              ) : (
                <ul className="grid grid-cols-1 md:grid-cols-3 gap-md">
                  {data.postSaleReport.paymentBreakdown.map((p) => (
                    <li
                      key={p.provider}
                      className="p-md rounded-control bg-surface-elevated border border-border space-y-xs"
                    >
                      <span className="font-semibold text-body-sm text-fg">{p.provider}</span>
                      <p className="text-heading-sm font-bold text-primary">{formatVnd(p.amount)}</p>
                      <span className="text-body-xs text-fg-muted">
                        {p.orders.toLocaleString("vi-VN")} đơn ·{" "}
                        {data.grossRevenue > 0 ? Math.round((p.amount / data.grossRevenue) * 100) : 0}% doanh thu
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </Card>
          </div>
        )
      ) : null}
    </div>
  );
}
