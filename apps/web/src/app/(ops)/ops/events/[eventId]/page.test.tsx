/**
 * Test cho trang war room `/ops/events/[eventId]` (EV-187, AC-1, AC-3, AC-7; plan §1.4 + §6 M-2, m-3).
 *
 * Render component thật với dữ liệu mẫu thật (không mock service) để bắt lỗi nối dây.
 * notFound được mock ném lỗi như (checkout)/checkout/[eventId]/page.test.tsx.
 */
import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as opsService from "@/lib/ops-service";
import { __setOpsSampleOverride } from "@/lib/ops-service";

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

const KNOWN_ID = "evt-tech-summit-2026";
const KNOWN_TITLE = "Vietnam Tech Summit 2026 — Kỷ Nguyên AI & Cloud Scale";

afterEach(() => {
  vi.restoreAllMocks();
  __setOpsSampleOverride(KNOWN_ID, null);
  notFoundMock.mockClear();
});

describe("OpsWarRoomPage route (/ops/events/[eventId])", () => {
  it("sự kiện có thật: render war room với h1 là tên sự kiện và nhãn 'Dữ liệu mẫu'", async () => {
    const Component = await OpsWarRoomPage({ params: Promise.resolve({ eventId: KNOWN_ID }) });
    render(Component);

    expect(screen.getByRole("heading", { level: 1, name: KNOWN_TITLE })).toBeInTheDocument();
    expect(screen.getAllByText(/Dữ liệu mẫu/i).length).toBeGreaterThan(0);
    expect(notFoundMock).not.toHaveBeenCalled();
  });

  it("sự kiện không tồn tại (404): gọi notFound()", async () => {
    await expect(
      OpsWarRoomPage({ params: Promise.resolve({ eventId: "evt-khong-ton-tai-ev187" }) }),
    ).rejects.toThrow("NEXT_NOT_FOUND");
    expect(notFoundMock).toHaveBeenCalledTimes(1);
  });

  it("lỗi 403: KHÔNG gọi notFound, render màn 'Không có quyền' (M-2)", async () => {
    __setOpsSampleOverride(KNOWN_ID, { error: 403 });

    const Component = await OpsWarRoomPage({ params: Promise.resolve({ eventId: KNOWN_ID }) });
    render(Component);

    expect(notFoundMock).not.toHaveBeenCalled();
    expect(screen.getAllByText(/Không có quyền/i).length).toBeGreaterThan(0);
  });

  it("lỗi khác (500): render thông báo lỗi kèm nút 'Thử lại' (M-2)", async () => {
    __setOpsSampleOverride(KNOWN_ID, { error: 500 });

    const Component = await OpsWarRoomPage({ params: Promise.resolve({ eventId: KNOWN_ID }) });
    render(Component);

    expect(notFoundMock).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: /thử lại/i })).toBeInTheDocument();
  });

  it('export dynamic = "force-dynamic" (m-3)', () => {
    expect(dynamic).toBe("force-dynamic");
  });
});

type WarRoomElement = { props: { initialError?: { status: number; message: string } } };

describe("OpsWarRoomPage — chỉ đưa message của OpsServiceError ra client (plan §7 m-7)", () => {
  it("lỗi không phải OpsServiceError: message chung, không lộ message gốc", async () => {
    vi.spyOn(opsService, "getOpsWarRoom").mockRejectedValueOnce(
      new Error("ECONNREFUSED 10.0.0.5:6379 redis-internal"),
    );

    const el = (await OpsWarRoomPage({
      params: Promise.resolve({ eventId: KNOWN_ID }),
    })) as unknown as WarRoomElement;

    expect(notFoundMock).not.toHaveBeenCalled();
    expect(el.props.initialError).toBeDefined();
    expect(el.props.initialError?.message).toBe("Không tải được số liệu war room.");
    expect(JSON.stringify(el.props.initialError)).not.toContain("ECONNREFUSED");
  });

  it("OpsServiceError (403): giữ status và message của lỗi dịch vụ", async () => {
    __setOpsSampleOverride(KNOWN_ID, { error: 403 });
    let expected = "";
    try {
      await opsService.getOpsWarRoom(KNOWN_ID);
    } catch (err) {
      expected = (err as Error).message;
    }

    const el = (await OpsWarRoomPage({
      params: Promise.resolve({ eventId: KNOWN_ID }),
    })) as unknown as WarRoomElement;

    expect(el.props.initialError).toEqual({ status: 403, message: expected });
  });
});
