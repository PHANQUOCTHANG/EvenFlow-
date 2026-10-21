"use client";

/** Form Tạo / Sửa Sự kiện dạng Stepper Wizard (EV-186, EVF-116, Stitch 10.13).
 *
 * Nghiệp vụ bắt buộc:
 * - BR-E1: Bắt buộc có >= 1 hạng vé với quota > 0 và price >= 0.
 * - BR-E2: sale_start_at phải cách thời điểm hiện tại >= 30 phút để kịp pre-warm hạ tầng.
 * - BR-E3: Sau khi ON_SALE, CẤM giảm quota của hạng vé; chỉ được phép tăng.
 */

import Link from "next/link";
import React, { useState } from "react";

import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/cn";
import { formatVnd } from "@/lib/format";
import {
  canIncreaseTierQuota,
  canSubmitEvent,
  validateEventDates,
  validateEventSchedule,
  validateTierQuotaUpdate,
  validateTiers,
  type CreateEventInput,
  type OrganizerEvent,
  type TicketTierConfig,
} from "@/lib/organizer-service";

/** Kết quả một phần: bản nháp đã được tạo nhưng một bước sau đó lỗi (AC-21). */
export interface WizardSubmitResult {
  error?: string;
  failedStep?: "tiers" | "submit";
  /** Liên kết tới bản nháp đã tạo để người dùng không mất dữ liệu. */
  draftHref?: string;
  /** Tạo mới: bản nháp đã tồn tại, KHÔNG được POST /events lần nữa (khoá nút lưu). */
  blockResubmit?: boolean;
}

export interface EventWizardProps {
  initialEvent?: OrganizerEvent;
  onSubmit: (data: CreateEventInput) => Promise<void | WizardSubmitResult>;
  onCancel?: () => void;
  className?: string;
}

type WizardStep = 1 | 2 | 3 | 4;

function FormField({
  label,
  required,
  htmlFor,
  children,
}: {
  label: string;
  required?: boolean;
  htmlFor?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-xs">
      <span className="flex items-baseline gap-xs text-label-md text-fg font-medium">
        {htmlFor ? <label htmlFor={htmlFor}>{label}</label> : <label>{label}</label>}
        {required && (
          <span className="text-danger" aria-hidden="true">
            *
          </span>
        )}
      </span>
      {children}
    </div>
  );
}

function toLocalDatetimeString(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  const year = date.getFullYear();
  const month = pad(date.getMonth() + 1);
  const day = pad(date.getDate());
  const hours = pad(date.getHours());
  const minutes = pad(date.getMinutes());
  return `${year}-${month}-${day}T${hours}:${minutes}`;
}

export function EventWizard({
  initialEvent,
  onSubmit,
  onCancel,
  className,
}: EventWizardProps) {
  const isEditing = Boolean(initialEvent);
  const isCurrentOnSale = initialEvent?.state === "ON_SALE" || initialEvent?.state === "SOLD_OUT";
  // ON_SALE: chỉ được tăng quota; trường sự kiện chỉ đọc (PUT /events không cho phép).
  const quotaOnly = Boolean(initialEvent && canIncreaseTierQuota(initialEvent.state));
  // Chỉ DRAFT/REJECTED mới nộp duyệt được (khi tạo mới luôn được).
  const canSubmit = !initialEvent || canSubmitEvent(initialEvent.state);
  // Hạng vé đã tồn tại trên backend: chỉ được đổi quota (PUT .../tiers/{id} {new_quota}).
  const existingTierIds = new Set((initialEvent?.tiers ?? []).map((t) => t.id));

  const [step, setStep] = useState<WizardStep>(1);

  // Step 1: Cơ bản
  const [title, setTitle] = useState(initialEvent?.title ?? "");
  const [slug, setSlug] = useState(initialEvent?.slug ?? "");
  const [description, setDescription] = useState(initialEvent?.description ?? "");
  const [venue, setVenue] = useState(initialEvent?.venue ?? "");
  const [startsAt, setStartsAt] = useState(
    initialEvent?.startsAt
      ? toLocalDatetimeString(new Date(initialEvent.startsAt))
      : toLocalDatetimeString(new Date(Date.now() + 7 * 24 * 60 * 60 * 1000)),
  );

  // Step 2: Hạng vé
  const [tiers, setTiers] = useState<TicketTierConfig[]>(
    initialEvent?.tiers && initialEvent.tiers.length > 0
      ? initialEvent.tiers
      : [
          {
            id: "tier-1",
            name: "Vé Tiêu Chuẩn (Standard)",
            price: 500000,
            quota: 1000,
            description: "Vé vào cửa khu vực tự do.",
          },
        ],
  );

  // Step 3: Lịch mở bán
  const [saleStartAt, setSaleStartAt] = useState(
    initialEvent?.saleStartAt
      ? toLocalDatetimeString(new Date(initialEvent.saleStartAt))
      : toLocalDatetimeString(new Date(Date.now() + 2 * 60 * 60 * 1000)), // 2 giờ sau
  );

  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [partial, setPartial] = useState<WizardSubmitResult | null>(null);

  // Xử lý thêm hạng vé mới
  const handleAddTier = () => {
    const newId = `tier-${Date.now()}`;
    setTiers([
      ...tiers,
      {
        id: newId,
        name: `Hạng vé #${tiers.length + 1}`,
        price: 200000,
        quota: 100,
      },
    ]);
  };

  // Xử lý xoá hạng vé
  const handleRemoveTier = (id: string) => {
    if (tiers.length <= 1) {
      setErrorMsg("Sự kiện bắt buộc phải có ít nhất 1 hạng vé (BR-E1).");
      return;
    }
    setTiers(tiers.filter((t) => t.id !== id));
  };

  // Cập nhật hạng vé
  const handleUpdateTier = (
    id: string,
    field: keyof TicketTierConfig,
    value: string | number,
  ) => {
    setErrorMsg(null);
    setTiers(
      tiers.map((t) => {
        if (t.id === id) {
          return { ...t, [field]: value };
        }
        return t;
      }),
    );
  };

  // Tiến sang bước tiếp theo với validation từng bước
  const handleNext = () => {
    setErrorMsg(null);

    if (step === 1) {
      if (!title.trim()) {
        setErrorMsg("Vui lòng nhập tên sự kiện.");
        return;
      }
      if (!venue.trim()) {
        setErrorMsg("Vui lòng nhập địa điểm tổ chức.");
        return;
      }
      if (!startsAt) {
        setErrorMsg("Vui lòng chọn thời gian diễn ra sự kiện.");
        return;
      }
      setStep(2);
      return;
    }

    if (step === 2) {
      // Validate BR-E1
      const tierVal = validateTiers(tiers);
      if (!tierVal.isValid) {
        setErrorMsg(tierVal.error ?? "Cấu hình hạng vé không hợp lệ.");
        return;
      }

      // Validate BR-E3 nếu sự kiện đang ON_SALE
      if (isCurrentOnSale && initialEvent) {
        for (const newTier of tiers) {
          const oldTier = initialEvent.tiers.find((t) => t.id === newTier.id);
          if (oldTier) {
            const quotaVal = validateTierQuotaUpdate(
              oldTier.quota,
              newTier.quota,
              initialEvent.state,
            );
            if (!quotaVal.isValid) {
              setErrorMsg(quotaVal.error ?? "Không thể giảm quota.");
              return;
            }
          }
        }
      }

      setStep(3);
      return;
    }

    if (step === 3) {
      // BR-E2 (>= 30 phút) chỉ chặn "Gửi kiểm duyệt" ở handleSubmit, không chặn lưu nháp/sửa.
      const datesVal = validateEventDates(
        new Date(startsAt).toISOString(),
        new Date(saleStartAt).toISOString(),
      );
      if (!datesVal.isValid) {
        setErrorMsg(datesVal.error ?? "Thời gian sự kiện không hợp lệ.");
        return;
      }

      setStep(4);
      return;
    }
  };

  // ON_SALE: chỉ lưu quota tăng; chặn giảm (BR-E3) trước khi gọi mạng.
  const handleSaveQuotas = async () => {
    setErrorMsg(null);
    setPartial(null);
    if (!initialEvent) return;
    const tierVal = validateTiers(tiers);
    if (!tierVal.isValid) {
      setErrorMsg(tierVal.error ?? "Cấu hình hạng vé không hợp lệ.");
      return;
    }
    for (const t of tiers) {
      const old = initialEvent.tiers.find((o) => o.id === t.id);
      if (!old) continue;
      const v = validateTierQuotaUpdate(old.quota, t.quota, initialEvent.state);
      if (!v.isValid) {
        setErrorMsg(v.error ?? "Không thể giảm quota.");
        return;
      }
    }
    setIsSubmitting(true);
    try {
      const result = await onSubmit({
        title: initialEvent.title,
        slug: initialEvent.slug,
        description: initialEvent.description,
        venue: initialEvent.venue,
        startsAt: initialEvent.startsAt,
        saleStartAt: initialEvent.saleStartAt,
        tiers,
        submitForReview: false,
      });
      if (result && result.error) {
        setPartial(result);
        setErrorMsg(result.error);
      }
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleBack = () => {
    setErrorMsg(null);
    if (step > 1) {
      setStep((prev) => (prev - 1) as WizardStep);
    }
  };

  // Nộp form
  const handleSubmit = async (submitForReview: boolean) => {
    setErrorMsg(null);
    setPartial(null);

    if (submitForReview) {
      const scheduleVal = validateEventSchedule(saleStartAt);
      if (!scheduleVal.isValid) {
        setErrorMsg(scheduleVal.error ?? "Thời gian mở bán không hợp lệ.");
        return;
      }
    }
    setIsSubmitting(true);

    try {
      const data: CreateEventInput = {
        title: title.trim(),
        slug: slug.trim() || undefined,
        description: description.trim(),
        venue: venue.trim(),
        startsAt: new Date(startsAt).toISOString(),
        saleStartAt: new Date(saleStartAt).toISOString(),
        tiers,
        submitForReview,
      };

      const result = await onSubmit(data);
      if (result && result.error) {
        setPartial(result);
        setErrorMsg(result.error);
      }
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setIsSubmitting(false);
    }
  };

  const blocked = Boolean(partial?.blockResubmit);

  if (quotaOnly && initialEvent) {
    return (
      <Card className={cn("p-lg max-w-4xl mx-auto space-y-lg", className)}>
        <div className="border-b border-border pb-md">
          <h2 className="text-heading-lg font-bold text-fg">
            Tăng số lượng vé: {initialEvent.title}
          </h2>
          <p className="text-body-xs text-fg-muted mt-xs">
            Sự kiện đang mở bán hoặc đã hết vé ({initialEvent.state}): thông tin sự kiện chỉ đọc, chỉ được tăng số lượng vé
            (BR-E3 cấm giảm).
          </p>
        </div>

        {errorMsg && (
          <Alert variant="error" title="Lỗi">
            {errorMsg}
          </Alert>
        )}

        <div className="space-y-md">
          {tiers.map((tier, idx) => (
            <Card key={tier.id} className="p-md space-y-sm border border-border-strong">
              <span className="font-semibold text-body-sm text-fg">Hạng vé #{idx + 1}</span>
              <div className="grid grid-cols-1 md:grid-cols-3 gap-sm">
                <Input
                  id={`tier-name-${tier.id}`}
                  label="Tên hạng vé"
                  value={tier.name}
                  disabled
                  readOnly
                />
                <Input
                  id={`tier-price-${tier.id}`}
                  label="Giá vé (VND)"
                  value={String(tier.price)}
                  disabled
                  readOnly
                />
                <div className="flex flex-col gap-xs">
                  <span className="text-label-lg text-fg">
                    <label htmlFor={`tier-quota-${tier.id}`}>Số lượng vé (Quota)</label>
                  </span>
                  <input
                    id={`tier-quota-${tier.id}`}
                    type="number"
                    min={initialEvent.tiers.find((o) => o.id === tier.id)?.quota ?? 1}
                    value={tier.quota}
                    onChange={(e) => handleUpdateTier(tier.id, "quota", Number(e.target.value))}
                    className="ef-focus-ring h-11 w-full rounded-control border border-border-strong bg-surface px-sm text-body-lg text-fg"
                  />
                </div>
              </div>
            </Card>
          ))}
        </div>

        <div className="flex items-center justify-between pt-md border-t border-border">
          {onCancel ? (
            <Button variant="tertiary" onClick={onCancel} disabled={isSubmitting}>
              Huỷ bỏ
            </Button>
          ) : (
            <span />
          )}
          <Button variant="primary" onClick={handleSaveQuotas} disabled={isSubmitting}>
            {isSubmitting ? "Đang xử lý..." : "Lưu số lượng vé"}
          </Button>
        </div>
      </Card>
    );
  }

  return (
    <Card className={cn("p-lg max-w-4xl mx-auto space-y-lg", className)}>
      {/* Stepper Header */}
      <div className="border-b border-border pb-md">
        <h2 className="text-heading-lg font-bold text-fg">
          {isEditing ? `Chỉnh sửa: ${initialEvent?.title}` : "Tạo sự kiện mới"}
        </h2>
        <div className="flex items-center justify-between mt-md">
          {[
            { s: 1, label: "1. Thông tin cơ bản" },
            { s: 2, label: "2. Hạng vé & Quota" },
            { s: 3, label: "3. Lịch mở bán" },
            { s: 4, label: "4. Xem lại & Gửi duyệt" },
          ].map((item) => (
            <div
              key={item.s}
              className={cn(
                "flex items-center gap-xs text-label-sm font-semibold",
                step === item.s
                  ? "text-primary border-b-2 border-primary pb-xs"
                  : step > item.s
                    ? "text-success"
                    : "text-fg-muted",
              )}
            >
              <span>{item.label}</span>
            </div>
          ))}
        </div>
      </div>

      {/* Hiển thị lỗi nếu có */}
      {errorMsg && !partial && (
        <Alert variant="error" title="Lỗi xác thực quy tắc nghiệp vụ">
          {errorMsg}
        </Alert>
      )}

      {partial && (
        <Alert
          variant="error"
          title={`${isEditing ? "Đã lưu thông tin sự kiện" : "Đã tạo bản nháp"} nhưng bước ${
            partial.failedStep === "submit" ? "gửi kiểm duyệt" : "tạo hạng vé"
          } thất bại`}
        >
          <p>{errorMsg}</p>
          {partial.draftHref && (
            <p className="mt-xs">
              <Link href={partial.draftHref} className="underline font-semibold">
                Mở bản nháp đã lưu để hoàn tất
              </Link>
            </p>
          )}
        </Alert>
      )}

      {/* BƯỚC 1: THÔNG TIN CƠ BẢN */}
      {step === 1 && (
        <div className="space-y-md" role="region" aria-label="Bước 1: Thông tin cơ bản">
          <Input
            id="event-title"
            label="Tên sự kiện"
            required
            placeholder="VD: Vietnam Tech Summit 2026"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
          />

          <Input
            id="event-slug"
            label="Đường dẫn tĩnh (Slug - tuỳ chọn)"
            placeholder="VD: vietnam-tech-summit-2026"
            value={slug}
            disabled={isEditing}
            helpText={isEditing ? "Không thể đổi sau khi tạo." : undefined}
            onChange={(e) => setSlug(e.target.value)}
          />

          <Input
            id="event-venue"
            label="Địa điểm tổ chức"
            required
            placeholder="VD: Trung tâm Hội nghị Quốc gia, Hà Nội"
            value={venue}
            onChange={(e) => setVenue(e.target.value)}
          />

          <div className="flex flex-col gap-xs">
            <span className="flex items-baseline gap-xs text-label-lg text-fg">
              <label htmlFor="event-starts-at">Thời gian diễn ra sự kiện</label>
              <span className="text-danger" aria-hidden="true">
                *
              </span>
            </span>
            <input
              id="event-starts-at"
              type="datetime-local"
              required
              value={startsAt}
              onChange={(e) => setStartsAt(e.target.value)}
              className="ef-focus-ring h-11 w-full rounded-control border border-border-strong bg-surface px-sm text-body-lg text-fg placeholder:text-fg-muted disabled:cursor-not-allowed disabled:opacity-60"
            />
          </div>

          <FormField label="Mô tả sự kiện" htmlFor="event-description">
            <textarea
              id="event-description"
              className="w-full rounded-control border border-border bg-surface p-sm text-body-sm text-fg ef-focus-ring focus:outline-none"
              rows={4}
              placeholder="Nhập thông tin giới thiệu chi tiết về sự kiện..."
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </FormField>
        </div>
      )}

      {/* BƯỚC 2: HẠNG VÉ & QUOTA */}
      {step === 2 && (
        <div className="space-y-md" role="region" aria-label="Bước 2: Hạng vé và Quota">
          <div className="flex items-center justify-between">
            <div>
              <h3 className="text-heading-sm font-semibold text-fg">Danh sách hạng vé</h3>
              <p className="text-body-xs text-fg-muted">
                Quy tắc BR-E1: Phải có ít nhất 1 hạng vé với quota &gt; 0 và price &ge; 0.
                {isCurrentOnSale && " (Đang mở bán: Tuyệt đối CẤM giảm quota - BR-E3)"}
              </p>
            </div>
            <Button variant="secondary" onClick={handleAddTier}>
              + Thêm hạng vé
            </Button>
          </div>

          <div className="space-y-md">
            {tiers.map((tier, idx) => (
              <Card key={tier.id} className="p-md space-y-sm border border-border-strong">
                <div className="flex items-center justify-between">
                  <span className="font-semibold text-body-sm text-fg">
                    Hạng vé #{idx + 1}
                  </span>
                  {existingTierIds.has(tier.id) ? (
                    <span className="text-body-xs text-fg-muted">
                      Hạng vé đã tạo — chỉ được đổi số lượng
                    </span>
                  ) : (
                    tiers.length > 1 && (
                    <button
                      type="button"
                      onClick={() => handleRemoveTier(tier.id)}
                      className="text-danger text-body-xs hover:underline"
                    >
                      Xoá hạng vé
                    </button>
                    )
                  )}
                </div>

                <div className="grid grid-cols-1 md:grid-cols-3 gap-sm">
                  <Input
                    id={`tier-name-${tier.id}`}
                    label="Tên hạng vé"
                    required
                    value={tier.name}
                    disabled={existingTierIds.has(tier.id)}
                    onChange={(e) => handleUpdateTier(tier.id, "name", e.target.value)}
                    placeholder="VD: VIP"
                  />

                  <div className="flex flex-col gap-xs">
                    <span className="flex items-baseline gap-xs text-label-lg text-fg">
                      <label htmlFor={`tier-price-${tier.id}`}>Giá vé (VND)</label>
                      <span className="text-danger" aria-hidden="true">
                        *
                      </span>
                    </span>
                    <input
                      id={`tier-price-${tier.id}`}
                      type="number"
                      min={0}
                      required
                      value={tier.price}
                      disabled={existingTierIds.has(tier.id)}
                      onChange={(e) =>
                        handleUpdateTier(tier.id, "price", Number(e.target.value))
                      }
                      className="ef-focus-ring h-11 w-full rounded-control border border-border-strong bg-surface px-sm text-body-lg text-fg placeholder:text-fg-muted disabled:cursor-not-allowed disabled:opacity-60"
                    />
                  </div>

                  <div className="flex flex-col gap-xs">
                    <span className="flex items-baseline gap-xs text-label-lg text-fg">
                      <label htmlFor={`tier-quota-${tier.id}`}>Số lượng vé (Quota)</label>
                      <span className="text-danger" aria-hidden="true">
                        *
                      </span>
                    </span>
                    <input
                      id={`tier-quota-${tier.id}`}
                      type="number"
                      min={1}
                      required
                      value={tier.quota}
                      onChange={(e) =>
                        handleUpdateTier(tier.id, "quota", Number(e.target.value))
                      }
                      className="ef-focus-ring h-11 w-full rounded-control border border-border-strong bg-surface px-sm text-body-lg text-fg placeholder:text-fg-muted disabled:cursor-not-allowed disabled:opacity-60"
                    />
                  </div>
                </div>
              </Card>
            ))}
          </div>
        </div>
      )}

      {/* BƯỚC 3: LỊCH MỞ BÁN */}
      {step === 3 && (
        <div className="space-y-md" role="region" aria-label="Bước 3: Lịch mở bán">
          <div>
            <h3 className="text-heading-sm font-semibold text-fg">Thời điểm mở bán (T0)</h3>
            <p className="text-body-xs text-fg-muted">
              Quy tắc BR-E2: Thời gian mở bán (sale_start_at) phải cách thời điểm hiện tại tối
              thiểu 30 phút để hệ thống kịp pre-warm hạ tầng chịu tải 300.000 rps.
            </p>
          </div>

          <div className="flex flex-col gap-xs">
            <span className="flex items-baseline gap-xs text-label-lg text-fg">
              <label htmlFor="event-sale-start">Thời gian bắt đầu mở bán (T0)</label>
              <span className="text-danger" aria-hidden="true">
                *
              </span>
            </span>
            <input
              id="event-sale-start"
              type="datetime-local"
              required
              value={saleStartAt}
              onChange={(e) => setSaleStartAt(e.target.value)}
              className="ef-focus-ring h-11 w-full rounded-control border border-border-strong bg-surface px-sm text-body-lg text-fg placeholder:text-fg-muted disabled:cursor-not-allowed disabled:opacity-60"
            />
          </div>

          {!validateEventSchedule(saleStartAt).isValid && (
            <Alert variant="warning" title="Chưa đủ điều kiện gửi kiểm duyệt (BR-E2)">
              Thời gian mở bán cách hiện tại dưới 30 phút: vẫn lưu nháp được, nhưng không thể gửi
              kiểm duyệt.
            </Alert>
          )}

          <Alert variant="info" title="Lưu ý pre-warm hạ tầng">
            Hệ thống phòng chờ công bằng (Fair Waiting Room) sẽ gom khách vào Lobby trước T0 và
            xáo trộn ngẫu nhiên toàn bộ người chờ đúng thời điểm mở bán.
          </Alert>
        </div>
      )}

      {/* BƯỚC 4: XEM LẠI & GỬI DUYỆT */}
      {step === 4 && (
        <div className="space-y-md" role="region" aria-label="Bước 4: Xem lại và gửi duyệt">
          <h3 className="text-heading-sm font-semibold text-fg">Xác nhận thông tin sự kiện</h3>

          <div className="bg-surface-elevated p-md rounded-control space-y-sm text-body-sm">
            <p>
              <strong>Tên sự kiện:</strong> {title}
            </p>
            <p>
              <strong>Địa điểm:</strong> {venue}
            </p>
            <p>
              <strong>Thời gian diễn ra:</strong> {new Date(startsAt).toLocaleString("vi-VN")}
            </p>
            <p>
              <strong>Thời gian mở bán (T0):</strong>{" "}
              {new Date(saleStartAt).toLocaleString("vi-VN")}
            </p>
            <div className="pt-xs">
              <strong>Hạng vé &amp; Quota:</strong>
              <ul className="list-disc list-inside mt-xs space-y-xs">
                {tiers.map((t) => (
                  <li key={t.id}>
                    {t.name}: {formatVnd(t.price)} — Quota: {t.quota.toLocaleString("vi-VN")} vé
                  </li>
                ))}
              </ul>
            </div>
          </div>

          <Alert variant="info" title="Quy trình kiểm duyệt AI (BR-E4 & BR-E5)">
            Khi bấm &ldquo;Gửi kiểm duyệt&rdquo;, sự kiện chuyển sang trạng thái PENDING_REVIEW để chờ
            kiểm duyệt (AI 4 trục: Nội dung cấm, Sai lệch/lừa đảo, Bản quyền hình ảnh và Thông tin
            thiếu). Kết quả duyệt sẽ được cập nhật ở trang trạng thái kiểm duyệt.
          </Alert>
        </div>
      )}

      {/* Footer Navigation Buttons */}
      <div className="flex items-center justify-between pt-md border-t border-border">
        <div>
          {step > 1 ? (
            <Button variant="secondary" onClick={handleBack} disabled={isSubmitting}>
              ← Quay lại
            </Button>
          ) : (
            onCancel && (
              <Button variant="tertiary" onClick={onCancel} disabled={isSubmitting}>
                Huỷ bỏ
              </Button>
            )
          )}
        </div>

        <div className="flex gap-sm">
          {step < 4 ? (
            <Button variant="primary" onClick={handleNext}>
              Tiếp tục →
            </Button>
          ) : (
            <>
              <Button
                variant="secondary"
                onClick={() => handleSubmit(false)}
                disabled={isSubmitting || blocked}
              >
                Lưu nháp
              </Button>
              {canSubmit && (
                <Button
                  variant="primary"
                  onClick={() => handleSubmit(true)}
                  disabled={isSubmitting || blocked}
                >
                  {isSubmitting ? "Đang xử lý..." : "🚀 Gửi kiểm duyệt"}
                </Button>
              )}
            </>
          )}
        </div>
      </div>
    </Card>
  );
}
