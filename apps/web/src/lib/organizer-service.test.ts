import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { installFetch, json, problem, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

import {
  createOrganizerEvent,
  evaluateAiScanRisk,
  getOrganizerEventById,
  getOrganizerEvents,
  getRealtimeSales,
  submitOrganizerEvent,
  updateTierQuota,
  validateEventSchedule,
  validateTierQuotaUpdate,
  validateTiers,
  type ModerationAxisResult,
  type TicketTierConfig,
} from "./organizer-service";

describe("organizer-service — Business Rules & Validation", () => {
  beforeEach(() => {
    setupEnv();
    seedSession("acc-org", "ref-org", "organizer");
  });
  afterEach(resetEnv);

  describe("validateEventSchedule — BR-E2 (Lịch mở bán >= now + 30m)", () => {
    const fixedNow = new Date("2026-10-11T10:00:00Z");

    it("hợp lệ khi thời điểm mở bán cách hiện tại >= 30 phút", () => {
      const validTime = new Date("2026-10-11T10:35:00Z");
      const result = validateEventSchedule(validTime, fixedNow);
      expect(result.isValid).toBe(true);
      expect(result.diffMinutes).toBe(35);
      expect(result.error).toBeUndefined();
    });

    it("từ chối khi thời điểm mở bán cách hiện tại dưới 30 phút (BR-E2)", () => {
      const tooSoonTime = new Date("2026-10-11T10:20:00Z");
      const result = validateEventSchedule(tooSoonTime, fixedNow);
      expect(result.isValid).toBe(false);
      expect(result.diffMinutes).toBe(20);
      expect(result.error).toContain("BR-E2");
      expect(result.error).toContain("tối thiểu 30 phút");
    });

    it("từ chối khi thời gian mở bán nằm trong quá khứ", () => {
      const pastTime = new Date("2026-10-11T09:00:00Z");
      const result = validateEventSchedule(pastTime, fixedNow);
      expect(result.isValid).toBe(false);
      expect(result.error).toContain("tối thiểu 30 phút");
    });

    it("từ chối khi định dạng ngày giờ không hợp lệ", () => {
      const result = validateEventSchedule("invalid-date-string", fixedNow);
      expect(result.isValid).toBe(false);
      expect(result.error).toContain("không hợp lệ");
    });
  });

  describe("validateTiers — BR-E1 (Cấu hình hạng vé)", () => {
    it("hợp lệ khi có ít nhất 1 hạng vé với quota > 0 và price >= 0", () => {
      const validTiers: TicketTierConfig[] = [
        { id: "t1", name: "Vé Tiêu Chuẩn", price: 200000, quota: 500 },
        { id: "t2", name: "Vé Miễn Phí", price: 0, quota: 100 },
      ];
      const result = validateTiers(validTiers);
      expect(result.isValid).toBe(true);
    });

    it("từ chối khi danh sách hạng vé rỗng (BR-E1)", () => {
      const result = validateTiers([]);
      expect(result.isValid).toBe(false);
      expect(result.error).toContain("ít nhất 1 hạng vé");
      expect(result.error).toContain("BR-E1");
    });

    it("từ chối khi hạng vé có quota <= 0", () => {
      const tiers: TicketTierConfig[] = [
        { id: "t1", name: "Vé VIP", price: 500000, quota: 0 },
      ];
      const result = validateTiers(tiers);
      expect(result.isValid).toBe(false);
      expect(result.error).toContain("phải lớn hơn 0");
    });

    it("từ chối khi hạng vé có giá âm", () => {
      const tiers: TicketTierConfig[] = [
        { id: "t1", name: "Vé VIP", price: -10000, quota: 100 },
      ];
      const result = validateTiers(tiers);
      expect(result.isValid).toBe(false);
      expect(result.error).toContain("lớn hơn hoặc bằng 0");
    });

    it("từ chối khi hạng vé thiếu tên gọi", () => {
      const tiers: TicketTierConfig[] = [
        { id: "t1", name: "   ", price: 100000, quota: 100 },
      ];
      const result = validateTiers(tiers);
      expect(result.isValid).toBe(false);
      expect(result.error).toContain("chưa có tên gọi");
    });
  });

  describe("validateTierQuotaUpdate — BR-E3 (Không giảm quota khi ON_SALE)", () => {
    it("từ chối giảm quota khi sự kiện đang ON_SALE (BR-E3)", () => {
      const result = validateTierQuotaUpdate(1000, 800, "ON_SALE");
      expect(result.isValid).toBe(false);
      expect(result.error).toContain("Tuyệt đối cấm giảm số lượng vé");
      expect(result.error).toContain("BR-E3");
    });

    it("cho phép tăng quota khi sự kiện đang ON_SALE (BR-E3)", () => {
      const result = validateTierQuotaUpdate(1000, 1500, "ON_SALE");
      expect(result.isValid).toBe(true);
    });

    it("cho phép giữ nguyên quota khi đang ON_SALE", () => {
      const result = validateTierQuotaUpdate(1000, 1000, "ON_SALE");
      expect(result.isValid).toBe(true);
    });

    it("cho phép giảm quota khi sự kiện đang DRAFT hoặc SCHEDULED", () => {
      const draftResult = validateTierQuotaUpdate(1000, 500, "DRAFT");
      expect(draftResult.isValid).toBe(true);

      const scheduledResult = validateTierQuotaUpdate(1000, 500, "SCHEDULED");
      expect(scheduledResult.isValid).toBe(true);
    });
  });

  describe("evaluateAiScanRisk — BR-E4 (AI kiểm duyệt 4 trục)", () => {
    it("chấm AUTO_APPROVED khi rủi ro < 0.3", () => {
      const axes: ModerationAxisResult[] = [
        { axis: "PROHIBITED_CONTENT", title: "Nội dung cấm", riskScore: 0.1, passed: true },
        { axis: "MISLEADING_SCAM", title: "Sai lệch / Lừa đảo", riskScore: 0.15, passed: true },
        { axis: "IMAGE_COPYRIGHT", title: "Bản quyền", riskScore: 0.2, passed: true },
        { axis: "MISSING_INFO", title: "Thông tin thiếu", riskScore: 0.05, passed: true },
      ];
      const result = evaluateAiScanRisk(axes);
      expect(result.overallRiskScore).toBe(0.2);
      expect(result.decision).toBe("AUTO_APPROVED");
      expect(result.passed).toBe(true);
    });

    it("chấm NEEDS_HUMAN khi rủi ro trong khoảng 0.3 đến 0.8", () => {
      const axes: ModerationAxisResult[] = [
        { axis: "PROHIBITED_CONTENT", title: "Nội dung cấm", riskScore: 0.2, passed: true },
        {
          axis: "MISLEADING_SCAM",
          title: "Sai lệch / Lừa đảo",
          riskScore: 0.65,
          passed: false,
          notes: "Cần xác minh giấy tờ",
        },
        { axis: "IMAGE_COPYRIGHT", title: "Bản quyền", riskScore: 0.1, passed: true },
        { axis: "MISSING_INFO", title: "Thông tin thiếu", riskScore: 0.35, passed: false },
      ];
      const result = evaluateAiScanRisk(axes);
      expect(result.overallRiskScore).toBe(0.65);
      expect(result.decision).toBe("NEEDS_HUMAN");
      expect(result.passed).toBe(false);
      expect(result.reasons.length).toBeGreaterThan(0);
    });

    it("chấm AUTO_REJECTED khi rủi ro >= 0.8", () => {
      const axes: ModerationAxisResult[] = [
        {
          axis: "PROHIBITED_CONTENT",
          title: "Nội dung cấm",
          riskScore: 0.88,
          passed: false,
          notes: "Tổ chức sự kiện vi phạm an ninh",
        },
        { axis: "MISLEADING_SCAM", title: "Sai lệch / Lừa đảo", riskScore: 0.4, passed: false },
        { axis: "IMAGE_COPYRIGHT", title: "Bản quyền", riskScore: 0.1, passed: true },
        { axis: "MISSING_INFO", title: "Thông tin thiếu", riskScore: 0.2, passed: true },
      ];
      const result = evaluateAiScanRisk(axes);
      expect(result.overallRiskScore).toBe(0.88);
      expect(result.decision).toBe("AUTO_REJECTED");
      expect(result.passed).toBe(false);
      expect(result.reasons).toContain("Nội dung cấm: Tổ chức sự kiện vi phạm an ninh");
    });
  });

  describe("Repository & Operations (qua API mock)", () => {
    const EVT = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";
    const wire = (over: Record<string, unknown> = {}) => ({
      id: EVT,
      organizer_id: "org-1",
      slug: "tech-summit",
      title: "Tech Summit",
      description: "Mo ta",
      venue: "Ha Noi",
      starts_at: "2027-12-01T12:00:00Z",
      sale_start_at: "2027-11-01T03:00:00Z",
      status: "ON_SALE",
      created_at: "2026-10-01T00:00:00Z",
      updated_at: "2026-10-01T00:00:00Z",
      tiers: [
        { id: "t-1", event_id: EVT, name: "Standard", price_cents: 900000, quota: 1200 },
        { id: "t-2", event_id: EVT, name: "VIP", price_cents: 2500000, quota: 300 },
      ],
      ...over,
    });

    it("lấy danh sách toàn bộ sự kiện của organizer từ API", async () => {
      installFetch(() => json([wire(), wire({ id: "e2", status: "DRAFT" })]));
      const events = await getOrganizerEvents();
      expect(events).toHaveLength(2);
      expect(events.find((e) => e.state === "ON_SALE")).toBeDefined();
    });

    it("lấy sự kiện theo ID; 404 -> null", async () => {
      installFetch(() => json(wire()));
      const byId = await getOrganizerEventById(EVT);
      expect(byId?.slug).toBe("tech-summit");
      installFetch(() => problem(404, "/problems/not-found"));
      await expect(getOrganizerEventById("non-existent-id")).resolves.toBeNull();
    });

    it("tạo mới sự kiện với submitForReview -> trạng thái PENDING_REVIEW từ backend", async () => {
      const futureSale = new Date(Date.now() + 60 * 60 * 1000).toISOString();
      const futureStart = new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString();
      installFetch((c) => {
        if (c.path.endsWith("/submit")) return json(wire({ status: "PENDING_REVIEW" }));
        if (c.path.endsWith("/tiers")) return json({ id: "srv-1", event_id: EVT, name: "Vé Thường", price_cents: 100000, quota: 200 }, 201);
        return json(wire({ status: "DRAFT", tiers: [] }), 201);
      });
      const result = await createOrganizerEvent({
        title: "Test Event Demo 2026",
        description: "Mô tả test event",
        venue: "Hà Nội",
        startsAt: futureStart,
        saleStartAt: futureSale,
        tiers: [{ id: "tier-1", name: "Vé Thường", price: 100000, quota: 200 }],
        submitForReview: true,
      });
      expect(result.failedStep).toBeUndefined();
      expect(result.event.state).toBe("PENDING_REVIEW");
      expect(result.event.tiers).toHaveLength(1);
    });

    it("từ chối cập nhật giảm quota của sự kiện đang ON_SALE (BR-E3) mà không gọi PUT", async () => {
      const { calls } = installFetch(() => json(wire()));
      await expect(updateTierQuota(EVT, "t-1", 100)).rejects.toThrow(/BR-E3/);
      expect(calls.some((c) => c.method === "PUT")).toBe(false);
    });

    it("submitOrganizerEvent chuyển sự kiện bị từ chối sang PENDING_REVIEW (thay resubmitForModeration)", async () => {
      const { calls } = installFetch(() => json(wire({ status: "PENDING_REVIEW" })));
      const submitted = await submitOrganizerEvent(EVT);
      expect(calls[0].path).toBe(`/api/v1/events/${EVT}/submit`);
      expect(submitted.state).toBe("PENDING_REVIEW");
    });

    it("getRealtimeSales là dữ liệu mẫu (isSample) suy từ quota thật, vẫn tính đúng doanh số", async () => {
      installFetch(() => json(wire()));
      const sales = await getRealtimeSales(EVT);
      expect(sales.isSample).toBe(true);
      expect(sales.eventId).toBe(EVT);
      expect(sales.grossRevenue).toBeGreaterThan(0);
      expect(sales.totalTicketsSold).toBeGreaterThan(0);
      expect(sales.tierSales.length).toBe(2);
      expect(sales.postSaleReport?.averageOrderValue).toBeGreaterThan(0);
      expect(sales.postSaleReport?.paymentBreakdown.vnpay).toBeGreaterThan(0);
    });
  });
});
