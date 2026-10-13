/** Organizer Service — Quản lý sự kiện, kiểm duyệt AI và doanh số realtime (EV-186, EVF-116).
 *
 * Tuân thủ nghiêm ngặt các Business Rules:
 * - BR-E1: Sự kiện chỉ chuyển SCHEDULED khi đã có >= 1 hạng vé với quota > 0 và price >= 0.
 * - BR-E2: sale_start_at phải cách thời điểm duyệt/hiện tại >= 30 phút để kịp pre-warm hạ tầng.
 * - BR-E3: Sau khi ON_SALE, CẤM giảm quota của hạng vé; chỉ được phép tăng.
 * - BR-E4: AI kiểm duyệt chấm 4 trục (nội dung cấm, sai lệch/lừa đảo, bản quyền hình ảnh, thông tin thiếu).
 *          Điểm rủi ro >= 0.8 -> AUTO_REJECTED; 0.3–0.8 -> NEEDS_HUMAN; < 0.3 -> AUTO_APPROVED.
 * - BR-E5: Mọi quyết định ghi nhận audit_log kèm lý do minh bạch, Moderator có thể lật ngược.
 */

export type EventLifecycleState =
  | "DRAFT"
  | "PENDING_REVIEW"
  | "SCHEDULED"
  | "ON_SALE"
  | "SOLD_OUT"
  | "CLOSED"
  | "REJECTED"
  | "CANCELLED";

export interface TicketTierConfig {
  id: string;
  name: string;
  price: number;
  quota: number;
  description?: string;
}

export type ModerationAxis =
  | "PROHIBITED_CONTENT"
  | "MISLEADING_SCAM"
  | "IMAGE_COPYRIGHT"
  | "MISSING_INFO";

export interface ModerationAxisResult {
  axis: ModerationAxis;
  title: string;
  riskScore: number; // 0.0 -> 1.0
  passed: boolean;
  notes?: string;
}

export type ModerationDecision =
  | "AUTO_APPROVED"
  | "NEEDS_HUMAN"
  | "AUTO_REJECTED"
  | "MODERATOR_OVERRIDE";

export interface ModerationDetail {
  overallRiskScore: number;
  decision: ModerationDecision;
  scannedAt: string;
  reasons: string[];
  moderatorNotes?: string;
  axes: ModerationAxisResult[];
}

export interface OrganizerEvent {
  id: string;
  slug: string;
  title: string;
  description: string;
  venue: string;
  startsAt: string;
  saleStartAt: string;
  state: EventLifecycleState;
  organizerId: string;
  tiers: TicketTierConfig[];
  moderation?: ModerationDetail;
  createdAt: string;
  updatedAt: string;
}

export interface TierSalesDetail {
  tierId: string;
  name: string;
  price: number;
  quota: number;
  sold: number;
  activeHolds: number;
  revenue: number;
  fillRatePercentage: number;
}

export interface RealtimeSalesSnapshot {
  eventId: string;
  timestamp: string;
  grossRevenue: number;
  totalTicketsSold: number;
  totalQuota: number;
  overallFillRatePercentage: number;
  paidOrdersCount: number;
  activeHoldsCount: number;
  tierSales: TierSalesDetail[];
  postSaleReport?: {
    averageOrderValue: number;
    peakSalesVelocityTps: number;
    paymentBreakdown: {
      vnpay: number;
      momo: number;
      card: number;
    };
  };
}

export interface CreateEventInput {
  title: string;
  slug?: string;
  description: string;
  venue: string;
  startsAt: string;
  saleStartAt: string;
  tiers: TicketTierConfig[];
  submitForReview?: boolean;
}

export interface ValidationResult {
  isValid: boolean;
  error?: string;
}

/** BR-E2: Kiểm tra sale_start_at phải cách thời điểm hiện tại/duyệt tối thiểu 30 phút */
export function validateEventSchedule(
  saleStartAtInput: string | Date,
  nowInput: Date = new Date(),
): ValidationResult & { diffMinutes?: number } {
  const saleStart = typeof saleStartAtInput === "string" ? new Date(saleStartAtInput) : saleStartAtInput;
  const now = nowInput;

  if (isNaN(saleStart.getTime())) {
    return {
      isValid: false,
      error: "Thời gian mở bán (sale_start_at) không hợp lệ.",
    };
  }

  const diffMs = saleStart.getTime() - now.getTime();
  const diffMinutes = Math.floor(diffMs / (1000 * 60));

  if (diffMinutes < 30) {
    return {
      isValid: false,
      diffMinutes,
      error: `Thời gian mở bán phải cách thời điểm hiện tại tối thiểu 30 phút (hiện cách ${diffMinutes} phút) theo quy định BR-E2 để hệ thống kịp pre-warm hạ tầng.`,
    };
  }

  return {
    isValid: true,
    diffMinutes,
  };
}

/** BR-E1: Phải có ít nhất 1 hạng vé với quota > 0 và price >= 0 */
export function validateTiers(tiers: TicketTierConfig[]): ValidationResult {
  if (!tiers || tiers.length === 0) {
    return {
      isValid: false,
      error: "Sự kiện phải có ít nhất 1 hạng vé để có thể lên lịch hoặc gửi kiểm duyệt (BR-E1).",
    };
  }

  for (let i = 0; i < tiers.length; i++) {
    const tier = tiers[i];
    if (!tier.name || tier.name.trim().length === 0) {
      return {
        isValid: false,
        error: `Hạng vé #${i + 1} chưa có tên gọi.`,
      };
    }
    if (typeof tier.price !== "number" || isNaN(tier.price) || tier.price < 0) {
      return {
        isValid: false,
        error: `Giá vé của hạng "${tier.name}" phải lớn hơn hoặc bằng 0 VND (BR-E1).`,
      };
    }
    if (typeof tier.quota !== "number" || isNaN(tier.quota) || tier.quota <= 0) {
      return {
        isValid: false,
        error: `Số lượng vé (quota) của hạng "${tier.name}" phải lớn hơn 0 (BR-E1).`,
      };
    }
  }

  return { isValid: true };
}

/** BR-E3: Sau khi ON_SALE, cấm giảm quota của hạng vé; chỉ được phép tăng */
export function validateTierQuotaUpdate(
  oldQuota: number,
  newQuota: number,
  eventState: EventLifecycleState,
): ValidationResult {
  if (eventState === "ON_SALE" && newQuota < oldQuota) {
    return {
      isValid: false,
      error: `Sự kiện đang mở bán (ON_SALE). Tuyệt đối cấm giảm số lượng vé của hạng vé từ ${oldQuota} xuống ${newQuota} (BR-E3); chỉ được phép tăng quota.`,
    };
  }
  return { isValid: true };
}

/** BR-E4: Đánh giá điểm rủi ro kiểm duyệt 4 trục của AI */
export function evaluateAiScanRisk(axes: ModerationAxisResult[]): {
  overallRiskScore: number;
  decision: ModerationDecision;
  reasons: string[];
  passed: boolean;
} {
  if (!axes || axes.length === 0) {
    return {
      overallRiskScore: 0,
      decision: "AUTO_APPROVED",
      reasons: [],
      passed: true,
    };
  }

  // Điểm rủi ro tổng hợp lấy theo giá trị lớn nhất trong 4 trục
  const maxScore = Math.max(...axes.map((a) => a.riskScore));
  const roundedScore = Math.round(maxScore * 100) / 100;
  const reasons: string[] = [];

  for (const axis of axes) {
    if (axis.riskScore >= 0.3 && axis.notes) {
      reasons.push(`${axis.title}: ${axis.notes}`);
    }
  }

  let decision: ModerationDecision;
  if (roundedScore >= 0.8) {
    decision = "AUTO_REJECTED";
  } else if (roundedScore >= 0.3) {
    decision = "NEEDS_HUMAN";
  } else {
    decision = "AUTO_APPROVED";
  }

  return {
    overallRiskScore: roundedScore,
    decision,
    reasons,
    passed: decision === "AUTO_APPROVED",
  };
}

/** Trục kiểm duyệt mặc định với tên tiếng Việt */
export const DEFAULT_MODERATION_AXES_METADATA: Record<ModerationAxis, string> = {
  PROHIBITED_CONTENT: "Nội dung cấm & bạo lực / phản cảm",
  MISLEADING_SCAM: "Thông tin sai lệch & dấu hiệu lừa đảo",
  IMAGE_COPYRIGHT: "Bản quyền hình ảnh & nhận diện thương hiệu",
  MISSING_INFO: "Thông tin pháp lý & quy định tổ chức",
};

/** Mock data mẫu sự kiện của Organizer */
const INITIAL_ORGANIZER_EVENTS: OrganizerEvent[] = [
  {
    id: "evt-tech-summit-2026",
    slug: "vietnam-tech-summit-2026",
    title: "Vietnam Tech Summit 2026 — Kỷ Nguyên AI & Cloud Scale",
    description:
      "Hội thảo công nghệ hàng đầu khu vực quy tụ các chuyên gia Kiến trúc hệ thống, High Concurrency và Trí tuệ nhân tạo.",
    venue: "Trung tâm Hội nghị Quốc gia, Hà Nội",
    startsAt: "2026-12-05T08:30:00+07:00",
    saleStartAt: "2026-09-01T09:00:00+07:00",
    state: "ON_SALE",
    organizerId: "org-evenflow-tech",
    tiers: [
      {
        id: "tier-standard-pass",
        name: "Standard Pass",
        price: 900000,
        quota: 1200,
        description: "Tham dự toàn bộ các phiên hội thảo chính và triển lãm công nghệ.",
      },
      {
        id: "tier-all-access",
        name: "All-Access Executive Pass",
        price: 2500000,
        quota: 300,
        description: "Bao gồm tiệc networking tối cùng diễn giả và tài liệu độc quyền.",
      },
    ],
    moderation: {
      overallRiskScore: 0.1,
      decision: "AUTO_APPROVED",
      scannedAt: "2026-08-25T14:30:00+07:00",
      reasons: ["Nội dung rõ ràng, thông tin pháp lý đầy đủ và không có yếu tố vi phạm."],
      axes: [
        { axis: "PROHIBITED_CONTENT", title: "Nội dung cấm", riskScore: 0.05, passed: true },
        { axis: "MISLEADING_SCAM", title: "Sai lệch / Lừa đảo", riskScore: 0.1, passed: true },
        { axis: "IMAGE_COPYRIGHT", title: "Bản quyền hình ảnh", riskScore: 0.1, passed: true },
        { axis: "MISSING_INFO", title: "Thông tin thiếu", riskScore: 0.05, passed: true },
      ],
    },
    createdAt: "2026-08-25T10:00:00+07:00",
    updatedAt: "2026-09-01T09:00:00+07:00",
  },
  {
    id: "evt-grand-concert-2026",
    slug: "evenflow-grand-concert-2026",
    title: "EvenFlow Grand Concert 2026 — Đêm Nhạc Ánh Sáng",
    description:
      "Đại nhạc hội quy tụ dàn nghệ sĩ hàng đầu Việt Nam và quốc tế. Hệ thống bán vé áp dụng công nghệ chống quá tải và phòng chờ công bằng.",
    venue: "Sân vận động Quốc gia Mỹ Đình, Hà Nội",
    startsAt: "2026-11-20T19:00:00+07:00",
    saleStartAt: "2026-11-01T10:00:00+07:00",
    state: "SCHEDULED",
    organizerId: "org-evenflow-music",
    tiers: [
      {
        id: "tier-ga",
        name: "Vé Tiêu Chuẩn (GA)",
        price: 650000,
        quota: 8000,
        description: "Khu vực đứng tự do tại khán đài B.",
      },
      {
        id: "tier-vip",
        name: "Vé VIP Gần Sân Khấu",
        price: 1800000,
        quota: 1500,
        description: "Khu vực gần sân khấu kèm quà tặng độc quyền và lối vào ưu tiên.",
      },
      {
        id: "tier-svip",
        name: "Vé SVIP Lounge",
        price: 3200000,
        quota: 300,
        description: "Ghế ngồi khán đài VIP, cocktail nhẹ và check-in riêng.",
      },
    ],
    moderation: {
      overallRiskScore: 0.2,
      decision: "AUTO_APPROVED",
      scannedAt: "2026-10-01T09:15:00+07:00",
      reasons: ["Sự kiện âm nhạc hợp lệ, giấy phép biểu diễn đầy đủ."],
      axes: [
        { axis: "PROHIBITED_CONTENT", title: "Nội dung cấm", riskScore: 0.1, passed: true },
        { axis: "MISLEADING_SCAM", title: "Sai lệch / Lừa đảo", riskScore: 0.15, passed: true },
        { axis: "IMAGE_COPYRIGHT", title: "Bản quyền hình ảnh", riskScore: 0.2, passed: true },
        { axis: "MISSING_INFO", title: "Thông tin thiếu", riskScore: 0.1, passed: true },
      ],
    },
    createdAt: "2026-09-28T14:20:00+07:00",
    updatedAt: "2026-10-01T09:15:00+07:00",
  },
  {
    id: "evt-fintech-expo-2026",
    slug: "fintech-blockchain-expo-2026",
    title: "Triển Lãm Công Nghệ Tài Chính & Web3 Summit 2026",
    description:
      "Hội chợ xúc tiến đầu tư tài chính, công nghệ số và tài sản mã hóa tại TP. Hồ Chí Minh.",
    venue: "Trung tâm Triển lãm & Hội chợ Sài Gòn (SECC), TP.HCM",
    startsAt: "2026-12-15T09:00:00+07:00",
    saleStartAt: "2026-11-10T14:00:00+07:00",
    state: "PENDING_REVIEW",
    organizerId: "org-fintech-club",
    tiers: [
      {
        id: "tier-pass-general",
        name: "Vé Tham Quan Triển Lãm",
        price: 200000,
        quota: 2000,
        description: "Vé vào cửa khu triển lãm công nghệ.",
      },
      {
        id: "tier-pass-vip",
        name: "Vé Hội Thảo Chuyên Đề VIP",
        price: 1500000,
        quota: 400,
        description: "Bao gồm vé hội thảo và tiệc trưa.",
      },
    ],
    moderation: {
      overallRiskScore: 0.55,
      decision: "NEEDS_HUMAN",
      scannedAt: "2026-10-09T16:00:00+07:00",
      reasons: [
        "Nội dung liên quan đến tài sản số và đầu tư tài chính có rủi ro trung bình (0.55).",
        "Đang chờ nhân sự Kiểm duyệt viên (Moderator) xác minh giấy phép và chấp thuận trong hàng đợi EVF-24.",
      ],
      moderatorNotes: "Chờ bổ sung bản chụp giấy phép tổ chức hội thảo tài chính từ Sở TT&TT.",
      axes: [
        { axis: "PROHIBITED_CONTENT", title: "Nội dung cấm", riskScore: 0.25, passed: true },
        {
          axis: "MISLEADING_SCAM",
          title: "Sai lệch / Lừa đảo",
          riskScore: 0.55,
          passed: false,
          notes: "Có thuật ngữ đầu tư tài chính cần kiểm tra cam kết lợi nhuận.",
        },
        { axis: "IMAGE_COPYRIGHT", title: "Bản quyền hình ảnh", riskScore: 0.2, passed: true },
        {
          axis: "MISSING_INFO",
          title: "Thông tin thiếu",
          riskScore: 0.45,
          passed: false,
          notes: "Thiếu giấy chứng nhận tổ chức sự kiện tài chính.",
        },
      ],
    },
    createdAt: "2026-10-09T15:30:00+07:00",
    updatedAt: "2026-10-09T16:00:00+07:00",
  },
  {
    id: "evt-secret-underground-party",
    slug: "underground-sound-night",
    title: "Đêm Nhạc Bí Mật Underground EDM",
    description: "Đêm nhạc điện tử không gian kín với âm thanh công suất lớn.",
    venue: "Địa điểm chưa xác định, Hà Nội",
    startsAt: "2026-11-25T23:00:00+07:00",
    saleStartAt: "2026-11-05T20:00:00+07:00",
    state: "REJECTED",
    organizerId: "org-underground",
    tiers: [
      {
        id: "tier-pass-free",
        name: "Vé Vào Cửa",
        price: 350000,
        quota: 500,
      },
    ],
    moderation: {
      overallRiskScore: 0.85,
      decision: "AUTO_REJECTED",
      scannedAt: "2026-10-08T11:00:00+07:00",
      reasons: [
        "Nội dung cấm: Địa điểm không rõ ràng, tiềm ẩn nguy cơ an ninh trật tự (BR-E4).",
        "Thông tin thiếu: Không có đơn vị tổ chức chịu trách nhiệm pháp lý và giấy phép phòng cháy chữa cháy.",
      ],
      axes: [
        {
          axis: "PROHIBITED_CONTENT",
          title: "Nội dung cấm",
          riskScore: 0.85,
          passed: false,
          notes: "Sự kiện tự phát ban đêm không có kế hoạch an ninh bảo vệ.",
        },
        { axis: "MISLEADING_SCAM", title: "Sai lệch / Lừa đảo", riskScore: 0.4, passed: false },
        { axis: "IMAGE_COPYRIGHT", title: "Bản quyền hình ảnh", riskScore: 0.1, passed: true },
        {
          axis: "MISSING_INFO",
          title: "Thông tin thiếu",
          riskScore: 0.8,
          passed: false,
          notes: "Địa điểm chưa xác định, thiếu toàn bộ giấy phép.",
        },
      ],
    },
    createdAt: "2026-10-08T10:45:00+07:00",
    updatedAt: "2026-10-08T11:00:00+07:00",
  },
  {
    id: "evt-draft-workshop-2026",
    slug: "workshop-thiet-ke-sang-tao",
    title: "Workshop Thiết Kế Trải Nghiệm Người Dùng (Bản Nháp)",
    description: "Khóa đào tạo chuyên sâu về UX Research và Design Systems cho doanh nghiệp.",
    venue: "Toà nhà Innovation Hub, Quận 1, TP.HCM",
    startsAt: "2026-12-10T14:00:00+07:00",
    saleStartAt: "2026-11-15T09:00:00+07:00",
    state: "DRAFT",
    organizerId: "org-design-lab",
    tiers: [
      {
        id: "tier-early-bird",
        name: "Vé Early Bird",
        price: 500000,
        quota: 50,
      },
    ],
    createdAt: "2026-10-10T08:00:00+07:00",
    updatedAt: "2026-10-10T08:00:00+07:00",
  },
];

let eventsStore: OrganizerEvent[] = [...INITIAL_ORGANIZER_EVENTS];

/** Lấy toàn bộ sự kiện của Ban tổ chức */
export async function getOrganizerEvents(): Promise<OrganizerEvent[]> {
  return [...eventsStore];
}

/** Lấy chi tiết sự kiện theo ID hoặc Slug */
export async function getOrganizerEventById(id: string): Promise<OrganizerEvent | null> {
  const event = eventsStore.find((e) => e.id === id || e.slug === id);
  return event ? { ...event } : null;
}

/** Tạo sự kiện mới cho Organizer */
export async function createOrganizerEvent(input: CreateEventInput): Promise<OrganizerEvent> {
  // Validate tiers (BR-E1)
  const tierValidation = validateTiers(input.tiers);
  if (!tierValidation.isValid) {
    throw new Error(tierValidation.error);
  }

  // Validate schedule if submitting (BR-E2)
  if (input.submitForReview) {
    const scheduleValidation = validateEventSchedule(input.saleStartAt);
    if (!scheduleValidation.isValid) {
      throw new Error(scheduleValidation.error);
    }
  }

  const id = `evt-${Date.now()}`;
  const slug =
    input.slug ||
    input.title
      .toLowerCase()
      .normalize("NFD")
      .replace(/[\u0300-\u036f]/g, "")
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/(^-|-$)+/g, "");

  const nowIso = new Date().toISOString();

  let moderation: ModerationDetail | undefined;
  let state: EventLifecycleState = "DRAFT";

  if (input.submitForReview) {
    // Tự động kích hoạt AI Scan giả lập theo BR-E4
    const axes: ModerationAxisResult[] = [
      { axis: "PROHIBITED_CONTENT", title: "Nội dung cấm", riskScore: 0.05, passed: true },
      { axis: "MISLEADING_SCAM", title: "Sai lệch / Lừa đảo", riskScore: 0.1, passed: true },
      { axis: "IMAGE_COPYRIGHT", title: "Bản quyền hình ảnh", riskScore: 0.1, passed: true },
      { axis: "MISSING_INFO", title: "Thông tin thiếu", riskScore: 0.05, passed: true },
    ];
    const evalResult = evaluateAiScanRisk(axes);
    moderation = {
      overallRiskScore: evalResult.overallRiskScore,
      decision: evalResult.decision,
      scannedAt: nowIso,
      reasons:
        evalResult.reasons.length > 0
          ? evalResult.reasons
          : ["Nội dung rõ ràng, hợp lệ và sẵn sàng mở bán."],
      axes,
    };

    if (evalResult.decision === "AUTO_APPROVED") {
      state = "SCHEDULED";
    } else if (evalResult.decision === "NEEDS_HUMAN") {
      state = "PENDING_REVIEW";
    } else {
      state = "REJECTED";
    }
  }

  const newEvent: OrganizerEvent = {
    id,
    slug,
    title: input.title,
    description: input.description,
    venue: input.venue,
    startsAt: input.startsAt,
    saleStartAt: input.saleStartAt,
    state,
    organizerId: "org-current-user",
    tiers: input.tiers,
    moderation,
    createdAt: nowIso,
    updatedAt: nowIso,
  };

  eventsStore = [newEvent, ...eventsStore];
  return newEvent;
}

/** Cập nhật sự kiện hiện có */
export async function updateOrganizerEvent(
  id: string,
  input: Partial<CreateEventInput>,
): Promise<OrganizerEvent> {
  const existing = await getOrganizerEventById(id);
  if (!existing) {
    throw new Error(`Không tìm thấy sự kiện với ID: ${id}`);
  }

  if (input.tiers) {
    const tierValidation = validateTiers(input.tiers);
    if (!tierValidation.isValid) {
      throw new Error(tierValidation.error);
    }

    // Kiểm tra BR-E3: cấm giảm quota nếu đang ON_SALE
    if (existing.state === "ON_SALE") {
      for (const newTier of input.tiers) {
        const oldTier = existing.tiers.find((t) => t.id === newTier.id);
        if (oldTier) {
          const quotaCheck = validateTierQuotaUpdate(oldTier.quota, newTier.quota, existing.state);
          if (!quotaCheck.isValid) {
            throw new Error(quotaCheck.error);
          }
        }
      }
    }
  }

  if (input.saleStartAt && existing.state === "DRAFT") {
    const scheduleCheck = validateEventSchedule(input.saleStartAt);
    if (!scheduleCheck.isValid && input.submitForReview) {
      throw new Error(scheduleCheck.error);
    }
  }

  const updated: OrganizerEvent = {
    ...existing,
    title: input.title ?? existing.title,
    description: input.description ?? existing.description,
    venue: input.venue ?? existing.venue,
    startsAt: input.startsAt ?? existing.startsAt,
    saleStartAt: input.saleStartAt ?? existing.saleStartAt,
    tiers: input.tiers ?? existing.tiers,
    updatedAt: new Date().toISOString(),
  };

  eventsStore = eventsStore.map((e) => (e.id === id ? updated : e));
  return updated;
}

/** Gửi nộp duyệt lại (Resubmit) cho sự kiện bị từ chối hoặc đang nháp */
export async function resubmitForModeration(id: string): Promise<OrganizerEvent> {
  const event = await getOrganizerEventById(id);
  if (!event) {
    throw new Error(`Không tìm thấy sự kiện: ${id}`);
  }

  const tierCheck = validateTiers(event.tiers);
  if (!tierCheck.isValid) {
    throw new Error(tierCheck.error);
  }

  const scheduleCheck = validateEventSchedule(event.saleStartAt);
  if (!scheduleCheck.isValid) {
    throw new Error(scheduleCheck.error);
  }

  const nowIso = new Date().toISOString();
  // Giả lập quét lại tự động với kết quả cải thiện
  const axes: ModerationAxisResult[] = [
    { axis: "PROHIBITED_CONTENT", title: "Nội dung cấm", riskScore: 0.1, passed: true },
    { axis: "MISLEADING_SCAM", title: "Sai lệch / Lừa đảo", riskScore: 0.1, passed: true },
    { axis: "IMAGE_COPYRIGHT", title: "Bản quyền hình ảnh", riskScore: 0.1, passed: true },
    { axis: "MISSING_INFO", title: "Thông tin thiếu", riskScore: 0.1, passed: true },
  ];
  const evalResult = evaluateAiScanRisk(axes);

  const updated: OrganizerEvent = {
    ...event,
    state: evalResult.decision === "AUTO_APPROVED" ? "SCHEDULED" : "PENDING_REVIEW",
    moderation: {
      overallRiskScore: evalResult.overallRiskScore,
      decision: evalResult.decision,
      scannedAt: nowIso,
      reasons: ["Đã tiếp nhận hồ sơ chỉnh sửa bổ sung. Quét AI sơ bộ hợp lệ."],
      axes,
    },
    updatedAt: nowIso,
  };

  eventsStore = eventsStore.map((e) => (e.id === id ? updated : e));
  return updated;
}

/** Đặt lại store về mock ban đầu (phục vụ test) */
export function resetOrganizerEventsStore(): void {
  eventsStore = [...INITIAL_ORGANIZER_EVENTS];
}

/** Lấy dữ liệu doanh số realtime (≤ 5s) của một sự kiện */
export async function getRealtimeSales(eventId: string): Promise<RealtimeSalesSnapshot> {
  const event = await getOrganizerEventById(eventId);
  if (!event) {
    throw new Error(`Sự kiện ${eventId} không tồn tại.`);
  }

  const now = new Date();
  const timestamp = now.toLocaleTimeString("vi-VN", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });

  // Tùy theo trạng thái sự kiện mà tính toán số lượng vé đã bán
  let soldRatio = 0;
  let activeHolds = 0;
  if (event.state === "ON_SALE") {
    soldRatio = 0.72;
    activeHolds = 48;
  } else if (event.state === "SOLD_OUT" || event.state === "CLOSED") {
    soldRatio = 1.0;
    activeHolds = 0;
  } else if (event.state === "SCHEDULED") {
    soldRatio = 0.0;
    activeHolds = 0;
  }

  const tierSales: TierSalesDetail[] = event.tiers.map((t) => {
    const sold = Math.floor(t.quota * soldRatio);
    const holds = Math.min(activeHolds, Math.floor(t.quota * 0.05));
    const revenue = sold * t.price;
    const fillRatePercentage = Math.round((sold / t.quota) * 100);

    return {
      tierId: t.id,
      name: t.name,
      price: t.price,
      quota: t.quota,
      sold,
      activeHolds: holds,
      revenue,
      fillRatePercentage,
    };
  });

  const totalTicketsSold = tierSales.reduce((acc, curr) => acc + curr.sold, 0);
  const totalQuota = tierSales.reduce((acc, curr) => acc + curr.quota, 0);
  const grossRevenue = tierSales.reduce((acc, curr) => acc + curr.revenue, 0);
  const overallFillRatePercentage =
    totalQuota > 0 ? Math.round((totalTicketsSold / totalQuota) * 100) : 0;
  const paidOrdersCount = Math.floor(totalTicketsSold / 1.8);

  return {
    eventId: event.id,
    timestamp,
    grossRevenue,
    totalTicketsSold,
    totalQuota,
    overallFillRatePercentage,
    paidOrdersCount,
    activeHoldsCount: activeHolds,
    tierSales,
    postSaleReport: {
      averageOrderValue: paidOrdersCount > 0 ? Math.round(grossRevenue / paidOrdersCount) : 0,
      peakSalesVelocityTps: 84.5,
      paymentBreakdown: {
        vnpay: Math.round(grossRevenue * 0.55),
        momo: Math.round(grossRevenue * 0.3),
        card: Math.round(grossRevenue * 0.15),
      },
    },
  };
}
