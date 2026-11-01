/**
 * Test cho trang war room `/ops/events/[eventId]` (EV-187, AC-1, AC-3, AC-7; plan §1.4 + §6 M-2, m-3).
 *
 * ops-warroom (test change request): trang là vỏ mỏng, KHÔNG fetch ở server (token nằm ở trình duyệt, AC-26).
 * Hook của OpsWarRoom tải số liệu ở client; 404 hiện OpsNotFoundView ngay trong war room (không gọi notFound()).
 * Các trường hợp cũ (sự kiện có thật, 404, 403, lỗi khác, chỉ đưa message an toàn ra client) được giữ, nhưng
 * kiểm trên luồng client thay cho luồng server.
 */
import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { OpsServiceError, getOpsWarRoom, type OpsWarRoomSnapshot } from "@/lib/ops-service";

import OpsWarRoomPage, { dynamic } from "./page";

const { notFoundMock } = vi.hoisted(() => ({
  notFoundMock: vi.fn(() => {
    throw new Error("NEXT_NOT_FOUND");
  }),
}));

vi.mock("next/navigation", () => ({
  notFound: notFoundMock,
  usePathname: () => "/ops/events/evt-tech-summit-2026",
  useRouter: () => ({ push: vi.fn(), refresh: vi.fn(), replace: vi.fn() }),
}));

vi.mock("@/lib/ops-service", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/ops-service")>();
  return { ...actual, getOpsWarRoom: vi.fn() };
});
const mockGet = vi.mocked(getOpsWarRoom);

const KNOWN_ID = "evt-tech-summit-2026";
const KNOWN_TITLE = "Vietnam Tech Summit 2026 — Kỷ Nguyên AI & Cloud Scale";

function snapshot(): OpsWarRoomSnapshot {
  return {
    eventId: KNOWN_ID,
    eventTitle: KNOWN_TITLE,
    saleState: "ON_SALE",
    generatedAt: new Date().toISOString(),
    funnel: { lobby: 0, queued: 10, admitted: 5, holding: 1, paid: 2, issued: 3 },
    admission: {
      ratePerSec: 5,
      mode: "AUTO",
      queueDepth: 10,
      overrideRatePerSec: null,
      paused: false,
      soldOut: false,
      stockZero: false,
      leaderPresent: true,
    },
    checkout: null,
    inventory: [],
    ai: { mode: "NORMAL", manualOverride: false, chatQueueLagSeconds: 0, budgetUsedRatio: 0 },
    sources: { waitingroom: "ok", ticketing: "ok", ai: "ok" },
    isSample: false,
  };
}

async function renderPage() {
  const Component = await OpsWarRoomPage({ params: Promise.resolve({ eventId: KNOWN_ID }) });
  return render(Component);
}

afterEach(() => {
  mockGet.mockReset();
  notFoundMock.mockClear();
});

describe("OpsWarRoomPage route (/ops/events/[eventId])", () => {
  it("sự kiện có thật: war room tải ở client, h1 là tên sự kiện, không có nhãn 'Dữ liệu mẫu'", async () => {
    mockGet.mockResolvedValue(snapshot());
    await renderPage();

    expect(await screen.findByRole("heading", { level: 1, name: KNOWN_TITLE })).toBeInTheDocument();
    expect(mockGet).toHaveBeenCalledWith(KNOWN_ID);
    expect(screen.queryAllByText(/Dữ liệu mẫu/i)).toHaveLength(0);
    expect(notFoundMock).not.toHaveBeenCalled();
  });

  it("sự kiện không tồn tại (404): hiện 'Không tìm thấy sự kiện', không gọi notFound() từ client", async () => {
    mockGet.mockRejectedValue(new OpsServiceError(404));
    await renderPage();

    expect(await screen.findByText("Không tìm thấy sự kiện")).toBeInTheDocument();
    expect(notFoundMock).not.toHaveBeenCalled();
  });

  it("lỗi 403: KHÔNG gọi notFound, render màn 'Không có quyền' (M-2)", async () => {
    mockGet.mockRejectedValue(new OpsServiceError(403));
    await renderPage();

    expect((await screen.findAllByText(/Không có quyền/i)).length).toBeGreaterThan(0);
    expect(notFoundMock).not.toHaveBeenCalled();
  });

  it("lỗi khác (503): render thông báo lỗi kèm nút 'Thử lại' (M-2)", async () => {
    mockGet.mockRejectedValue(new OpsServiceError(503));
    await renderPage();

    expect(await screen.findByRole("button", { name: /thử lại/i })).toBeInTheDocument();
    expect(notFoundMock).not.toHaveBeenCalled();
  });

  it('export dynamic = "force-dynamic" (m-3)', () => {
    expect(dynamic).toBe("force-dynamic");
  });
});

type WarRoomElement = {
  props: { initialSnapshot?: OpsWarRoomSnapshot | null; initialError?: { status: number; message: string } };
};

describe("OpsWarRoomPage — không lộ lỗi server ra client (plan §7 m-7)", () => {
  it("trang không fetch ở server: không có initialError (không có gì để lộ), initialSnapshot = null", async () => {
    const el = (await OpsWarRoomPage({
      params: Promise.resolve({ eventId: KNOWN_ID }),
    })) as unknown as WarRoomElement;

    expect(mockGet).not.toHaveBeenCalled();
    expect(el.props.initialSnapshot).toBeNull();
    expect(el.props.initialError).toBeUndefined();
  });

  it("lỗi 503 ở client: chỉ hiện message cố định của OpsServiceError", async () => {
    mockGet.mockRejectedValue(new OpsServiceError(503));
    await renderPage();

    expect(await screen.findByText("Không tải được số liệu war room.")).toBeInTheDocument();
  });
});
