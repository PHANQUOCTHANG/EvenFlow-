/**
 * ops-warroom AC-29: getRealtimeSales against GET /v1/organizer/events/{id}/sales (contracts §4.2, §7).
 * ASSUMPTION: goes through apiFetch with the session Bearer; errors are Error instances (404 has the exact Vietnamese message).
 */
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { getRealtimeSales } from "@/lib/organizer-service";
import { installFetch, json, problem, rejection, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

beforeEach(() => {
  setupEnv();
  seedSession("acc-org", "ref", "organizer");
});
afterEach(resetEnv);

const EV = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";
const WIRE = {
  event_id: EV,
  generated_at: "2026-11-01T10:00:00Z",
  currency: "VND",
  totals: { gross_revenue_cents: 123000000, tickets_sold: 820, quota: 1200, paid_orders: 455, active_holds: 48 },
  tiers: [
    { ticket_type_id: "t1", name: "Standard", price_cents: 150000, quota: 1000, sold: 700, active_holds: 40, revenue_cents: 105000000 },
    { ticket_type_id: "t2", name: "VIP", price_cents: 500000, quota: 0, sold: 0, active_holds: 0, revenue_cents: 0 },
  ],
  payment_breakdown: [
    { provider: "vnpay", orders: 300, amount_cents: 80000000 },
    { provider: "sandbox", orders: 155, amount_cents: 43000000 },
  ],
};

describe("getRealtimeSales (AC-29)", () => {
  it("calls the sales endpoint with the Bearer token and maps the snapshot", async () => {
    const { calls } = installFetch(() => json(WIRE));
    const s = await getRealtimeSales(EV);
    expect(calls).toHaveLength(1);
    expect(calls[0].method).toBe("GET");
    expect(calls[0].path).toBe(`/v1/organizer/events/${EV}/sales`);
    expect(calls[0].headers.get("authorization")).toBe("Bearer acc-org");
    expect(s.isSample).toBe(false);
    expect(s.eventId).toBe(EV);
    expect(s.timestamp).toBe("2026-11-01T10:00:00Z");
    expect(s).toMatchObject({
      grossRevenue: 123000000, totalTicketsSold: 820, totalQuota: 1200, paidOrdersCount: 455, activeHoldsCount: 48,
      overallFillRatePercentage: Math.round((820 / 1200) * 100),
    });
    expect(s.tierSales[0]).toEqual({
      tierId: "t1", name: "Standard", price: 150000, quota: 1000, sold: 700, activeHolds: 40,
      revenue: 105000000, fillRatePercentage: 70,
    });
    expect(s.tierSales[1].fillRatePercentage).toBe(0); // quota 0 => 0, never NaN/Infinity
    expect(s.postSaleReport.averageOrderValue).toBe(Math.round(123000000 / 455));
    expect(s.postSaleReport.peakSalesVelocityTps).toBeNull();
    expect(s.postSaleReport.paymentBreakdown).toEqual([
      { provider: "vnpay", orders: 300, amount: 80000000 },
      { provider: "sandbox", orders: 155, amount: 43000000 },
    ]);
  });

  it("nothing sold: AOV 0, empty breakdown, fill 0", async () => {
    installFetch(() =>
      json({ ...WIRE, totals: { gross_revenue_cents: 0, tickets_sold: 0, quota: 0, paid_orders: 0, active_holds: 0 }, tiers: [], payment_breakdown: [] }),
    );
    const s = await getRealtimeSales(EV);
    expect(s.overallFillRatePercentage).toBe(0);
    expect(s.postSaleReport.averageOrderValue).toBe(0);
    expect(s.postSaleReport.paymentBreakdown).toEqual([]);
    expect(s.tierSales).toEqual([]);
  });

  it("404 => the ownership-hiding message", async () => {
    installFetch(() => problem(404, "/problems/not-found", "SERVER-DETAIL"));
    const e = (await rejection(getRealtimeSales(EV))) as Error;
    expect(e).toBeInstanceOf(Error);
    expect(e.message).toBe("Không tìm thấy sự kiện hoặc bạn không sở hữu sự kiện này");
  });

  it.each([401, 403, 503])("status %s rejects", async (status) => {
    installFetch(() => problem(status, "/problems/x"));
    expect(await rejection(getRealtimeSales(EV))).toBeInstanceOf(Error);
  });
});
