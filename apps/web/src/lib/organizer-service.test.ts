import { beforeEach, describe, expect, it } from "vitest";

import {
  createOrganizerEvent,
  evaluateAiScanRisk,
  getOrganizerEventById,
  getOrganizerEvents,
  getRealtimeSales,
  resetOrganizerEventsStore,
  resubmitForModeration,
  updateOrganizerEvent,
  validateEventSchedule,
  validateTierQuotaUpdate,
  validateTiers,
  type ModerationAxisResult,
  type TicketTierConfig,
} from "./organizer-service";

describe("organizer-service — Business Rules & Validation", () => {
  beforeEach(() => {
    resetOrganizerEventsStore();
  });

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

  describe("Repository & Operations", () => {
    it("lấy danh sách toàn bộ sự kiện của organizer", async () => {
      const events = await getOrganizerEvents();
      expect(events.length).toBeGreaterThanOrEqual(5);
      const onSale = events.find((e) => e.state === "ON_SALE");
      expect(onSale).toBeDefined();
    });

    it("lấy sự kiện theo ID hoặc Slug", async () => {
      const byId = await getOrganizerEventById("evt-tech-summit-2026");
      expect(byId?.slug).toBe("vietnam-tech-summit-2026");

      const bySlug = await getOrganizerEventById("vietnam-tech-summit-2026");
      expect(bySlug?.id).toBe("evt-tech-summit-2026");

      const notFound = await getOrganizerEventById("non-existent-id");
      expect(notFound).toBeNull();
    });

    it("tạo mới sự kiện thành công và lưu vào store", async () => {
      const futureSale = new Date(Date.now() + 60 * 60 * 1000).toISOString(); // 1 giờ sau
      const futureStart = new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString();

      const created = await createOrganizerEvent({
        title: "Test Event Demo 2026",
        description: "Mô tả test event",
        venue: "Hà Nội",
        startsAt: futureStart,
        saleStartAt: futureSale,
        tiers: [{ id: "tier-1", name: "Vé Thường", price: 100000, quota: 200 }],
        submitForReview: true,
      });

      expect(created.id).toBeDefined();
      expect(created.state).toBe("SCHEDULED"); // Auto-approved do test input hợp lệ
      expect(created.moderation?.decision).toBe("AUTO_APPROVED");

      const fetched = await getOrganizerEventById(created.id);
      expect(fetched?.title).toBe("Test Event Demo 2026");
    });

    it("từ chối cập nhật giảm quota của sự kiện đang ON_SALE (BR-E3)", async () => {
      await expect(
        updateOrganizerEvent("evt-tech-summit-2026", {
          tiers: [
            { id: "tier-standard-pass", name: "Standard Pass", price: 900000, quota: 100 }, // giảm từ 1200 xuống 100
            { id: "tier-all-access", name: "All-Access Executive Pass", price: 2500000, quota: 300 },
          ],
        }),
      ).rejects.toThrow(/BR-E3/);
    });

    it("resubmitForModeration cập nhật trạng thái sự kiện bị từ chối sang SCHEDULED hoặc PENDING_REVIEW", async () => {
      const rejectedEvent = await getOrganizerEventById("evt-secret-underground-party");
      expect(rejectedEvent?.state).toBe("REJECTED");

      // Cập nhật lại thời gian hợp lệ trước khi resubmit
      const futureSale = new Date(Date.now() + 60 * 60 * 1000).toISOString();
      await updateOrganizerEvent("evt-secret-underground-party", {
        saleStartAt: futureSale,
      });

      const resubmitted = await resubmitForModeration("evt-secret-underground-party");
      expect(resubmitted.state).toBe("SCHEDULED");
      expect(resubmitted.moderation?.decision).toBe("AUTO_APPROVED");
    });

    it("getRealtimeSales tính toán đúng doanh số, tỉ lệ lấp đầy và báo cáo sau bán", async () => {
      const sales = await getRealtimeSales("evt-tech-summit-2026");
      expect(sales.eventId).toBe("evt-tech-summit-2026");
      expect(sales.grossRevenue).toBeGreaterThan(0);
      expect(sales.totalTicketsSold).toBeGreaterThan(0);
      expect(sales.tierSales.length).toBe(2);
      expect(sales.postSaleReport?.averageOrderValue).toBeGreaterThan(0);
      expect(sales.postSaleReport?.paymentBreakdown.vnpay).toBeGreaterThan(0);
    });
  });
});
