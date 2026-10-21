"use client";

/** Danh sách sự kiện của Ban tổ chức (EV-186, EVF-116, Stitch 10.12).
 *
 * Hỗ trợ:
 * - Tìm kiếm theo tên hoặc địa điểm.
 * - Lọc theo trạng thái vòng đời (ON_SALE, SCHEDULED, PENDING_REVIEW, DRAFT, REJECTED, v.v.).
 * - Thẻ sự kiện hiển thị trạng thái duyệt, thời gian, hạng vé.
 * - Hành động nhanh: Xem doanh số realtime, xem trạng thái duyệt, chỉnh sửa sự kiện, tạo sự kiện mới.
 */

import Link from "next/link";
import { useId, useMemo, useState } from "react";

import { Badge, type BadgeVariant } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/cn";
import { formatVnd } from "@/lib/format";
import { canEditEvent, canOpenEventEditor, type EventLifecycleState, type OrganizerEvent } from "@/lib/organizer-service";

export interface EventListProps {
  events: OrganizerEvent[];
  className?: string;
}

export const LIFECYCLE_BADGE_CONFIG: Record<
  EventLifecycleState,
  { label: string; variant: BadgeVariant }
> = {
  ON_SALE: { label: "Đang mở bán", variant: "admitted" },
  SCHEDULED: { label: "Đã lên lịch", variant: "queued" },
  PENDING_REVIEW: { label: "Chờ duyệt", variant: "pending" },
  DRAFT: { label: "Bản nháp", variant: "neutral" },
  REJECTED: { label: "Bị từ chối", variant: "failed" },
  SOLD_OUT: { label: "Hết vé", variant: "soldout" },
  CLOSED: { label: "Đã đóng", variant: "neutral" },
  COMPLETED: { label: "Đã kết thúc", variant: "neutral" },
  CANCELLED: { label: "Đã huỷ", variant: "failed" },
};

export function EventList({ events, className }: EventListProps) {
  const searchInputId = useId();
  const [searchTerm, setSearchTerm] = useState("");
  const [selectedState, setSelectedState] = useState<string>("ALL");

  const filteredEvents = useMemo(() => {
    return events.filter((event) => {
      const matchSearch =
        searchTerm.trim() === "" ||
        event.title.toLowerCase().includes(searchTerm.toLowerCase()) ||
        event.venue.toLowerCase().includes(searchTerm.toLowerCase());

      const matchState = selectedState === "ALL" || event.state === selectedState;

      return matchSearch && matchState;
    });
  }, [events, searchTerm, selectedState]);

  return (
    <div className={cn("space-y-lg", className)}>
      {/* Thanh tiêu đề và nút CTA Tạo sự kiện */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-md">
        <div>
          <h1 className="text-heading-xl font-bold text-fg">Sự kiện của tôi</h1>
          <p className="text-body-sm text-fg-muted mt-xs">
            Quản lý các sự kiện, theo dõi trạng thái kiểm duyệt và doanh số bán vé trực tiếp.
          </p>
        </div>
        <Link href="/organizer/events/new" className="inline-block">
          <Button variant="primary">
            + Tạo sự kiện mới
          </Button>
        </Link>
      </div>

      {/* Bộ lọc và ô tìm kiếm */}
      <Card className="p-md space-y-md">
        <div className="flex flex-col md:flex-row gap-md items-center justify-between">
          <div className="w-full md:w-80">
            <Input
              id={searchInputId}
              label="Tìm kiếm sự kiện"
              placeholder="Tìm theo tên sự kiện, địa điểm..."
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
            />
          </div>

          <div className="flex flex-wrap gap-xs w-full md:w-auto" role="tablist" aria-label="Lọc theo trạng thái">
            {[
              { id: "ALL", label: "Tất cả" },
              { id: "ON_SALE", label: "Đang mở bán" },
              { id: "SCHEDULED", label: "Đã lên lịch" },
              { id: "PENDING_REVIEW", label: "Chờ duyệt" },
              { id: "REJECTED", label: "Bị từ chối" },
              { id: "DRAFT", label: "Bản nháp" },
              { id: "SOLD_OUT", label: "Hết vé" },
              { id: "CLOSED", label: "Đã đóng" },
              { id: "COMPLETED", label: "Đã kết thúc" },
              { id: "CANCELLED", label: "Đã huỷ" },
            ].map((tab) => {
              const isActive = selectedState === tab.id;
              return (
                <button
                  key={tab.id}
                  type="button"
                  role="tab"
                  aria-selected={isActive}
                  onClick={() => setSelectedState(tab.id)}
                  className={cn(
                    "px-sm py-xs text-label-sm rounded-control transition-colors font-medium",
                    isActive
                      ? "bg-primary text-fg-inverse shadow-sm"
                      : "bg-surface-elevated text-fg-muted hover:text-fg hover:bg-neutral-soft",
                  )}
                >
                  {tab.label}
                </button>
              );
            })}
          </div>
        </div>
      </Card>

      {/* Danh sách thẻ sự kiện */}
      {filteredEvents.length === 0 ? (
        <Card className="p-xl text-center space-y-sm">
          <p className="text-heading-sm font-semibold text-fg">Không tìm thấy sự kiện nào</p>
          <p className="text-body-sm text-fg-muted max-w-md mx-auto">
            {searchTerm || selectedState !== "ALL"
              ? "Không có sự kiện nào khớp với từ khoá hoặc bộ lọc hiện tại. Vui lòng thử lại."
              : "Bạn chưa tạo sự kiện nào. Hãy bắt đầu bằng cách tạo sự kiện đầu tiên."}
          </p>
          {searchTerm || selectedState !== "ALL" ? (
            <Button
              variant="secondary"
              onClick={() => {
                setSearchTerm("");
                setSelectedState("ALL");
              }}
            >
              Đặt lại bộ lọc
            </Button>
          ) : (
            <Link href="/organizer/events/new">
              <Button variant="primary">+ Tạo sự kiện mới ngay</Button>
            </Link>
          )}
        </Card>
      ) : (
        <div className="grid grid-cols-1 gap-md" role="region" aria-label="Danh sách sự kiện">
          {filteredEvents.map((event) => {
            const badge = LIFECYCLE_BADGE_CONFIG[event.state] || {
              label: event.state,
              variant: "neutral",
            };
            // GET /events không kèm hạng vé: chỉ hiện thông tin vé khi đã có dữ liệu.
            const hasTiers = event.tiers.length > 0;
            const totalQuota = event.tiers.reduce((acc, t) => acc + t.quota, 0);
            const lowestPrice = hasTiers ? Math.min(...event.tiers.map((t) => t.price)) : 0;

            return (
              <Card
                key={event.id}
                className="p-lg hover:border-primary/50 transition-all flex flex-col md:flex-row justify-between items-start md:items-center gap-md"
              >
                <div className="space-y-xs max-w-2xl">
                  <div className="flex flex-wrap items-center gap-sm">
                    <h2 className="text-heading-md font-semibold text-fg hover:text-primary transition-colors">
                      {event.title}
                    </h2>
                    <Badge variant={badge.variant}>{badge.label}</Badge>
                  </div>

                  <p className="text-body-sm text-fg-muted">
                    📍 {event.venue} • 🗓️ Diễn ra:{" "}
                    {new Date(event.startsAt).toLocaleDateString("vi-VN", {
                      day: "2-digit",
                      month: "2-digit",
                      year: "numeric",
                    })}
                  </p>

                  <div className="flex flex-wrap gap-md text-label-sm text-fg-muted pt-xs">
                    {hasTiers && (
                      <>
                        <span>
                          🎟️ {event.tiers.length} hạng vé (Tổng {totalQuota.toLocaleString("vi-VN")} vé)
                        </span>
                        <span>💵 Giá từ: {formatVnd(lowestPrice)}</span>
                      </>
                    )}
                    <span>
                      ⏰ Mở bán:{" "}
                      {new Date(event.saleStartAt).toLocaleTimeString("vi-VN", {
                        hour: "2-digit",
                        minute: "2-digit",
                      })}{" "}
                      {new Date(event.saleStartAt).toLocaleDateString("vi-VN", {
                        day: "2-digit",
                        month: "2-digit",
                      })}
                    </span>
                  </div>

                  {event.state === "REJECTED" && event.moderation?.reasons && (
                    <div className="bg-danger-soft text-danger p-xs rounded-control text-body-xs font-medium">
                      ⚠️ Bị từ chối: {event.moderation.reasons[0]}
                    </div>
                  )}
                </div>

                <div className="flex flex-wrap items-center gap-xs w-full md:w-auto pt-sm md:pt-0 border-t md:border-t-0 border-border">
                  <Link href={`/organizer/events/${event.id}/sales`}>
                    <Button variant="secondary" className="text-body-xs">
                      📊 Doanh số
                    </Button>
                  </Link>

                  <Link href={`/organizer/events/${event.id}/review-status`}>
                    <Button variant="secondary" className="text-body-xs">
                      🔍 Kiểm duyệt
                    </Button>
                  </Link>

                  {canOpenEventEditor(event.state) && (
                    <Link href={`/organizer/events/${event.id}/edit`}>
                      <Button variant="tertiary" className="text-body-xs">
                        {canEditEvent(event.state) ? "✏️ Chỉnh sửa" : "✏️ Tăng số lượng vé"}
                      </Button>
                    </Link>
                  )}
                </div>
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
}
