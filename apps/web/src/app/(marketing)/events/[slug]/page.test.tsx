import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installFetch, json, problem, resetEnv, setupEnv } from "@/test-utils/fetch-mock";

import EventDetailPage, {
  generateMetadata,
  generateStaticParams,
} from "./page";

// Mock next/navigation
const mockNotFound = vi.fn();
vi.mock("next/navigation", () => ({
  notFound: () => {
    mockNotFound();
    throw new Error("NEXT_NOT_FOUND");
  },
  useRouter: () => ({
    push: vi.fn(),
  }),
}));

const SLUG = "evenflow-grand-concert-2026";
const EVT_ID = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";

function wireEvent(over: Record<string, unknown> = {}) {
  return {
    id: EVT_ID,
    organizer_id: "org-1",
    slug: SLUG,
    title: "EvenFlow Grand Concert 2026 — Đêm Nhạc Ánh Sáng",
    description: "Đại nhạc hội",
    venue: "Mỹ Đình",
    starts_at: "2026-11-20T12:00:00Z",
    sale_start_at: "2026-10-15T03:00:00Z",
    status: "ON_SALE",
    max_per_order: 4,
    tiers: [
      { id: "11111111-1111-4111-8111-111111111111", name: "Vé Tiêu Chuẩn (GA)", price_cents: 650000, currency: "VND" },
    ],
    ...over,
  };
}

/** Server hop le: su kien + availability. */
function serveEvent(over: Record<string, unknown> = {}) {
  return installFetch((c) =>
    c.path.endsWith("/availability")
      ? json({ event_id: EVT_ID, tiers: [{ ticket_type_id: "11111111-1111-4111-8111-111111111111", availability: "AVAILABLE" }] })
      : json(wireEvent(over)),
  );
}

beforeEach(() => {
  setupEnv();
  mockNotFound.mockClear();
  serveEvent();
});
afterEach(resetEnv);

describe("EventDetailPage (EVF-111, BR-A4, BR-Q1)", () => {
  it("generateStaticParams khong doc backend luc build: tra ve [] (ISR theo yeu cau)", async () => {
    const { calls } = installFetch(() => json({}));
    await expect(generateStaticParams()).resolves.toEqual([]);
    expect(calls).toHaveLength(0);
  });

  it("generateMetadata tra ve metadata chuan SEO cho event", async () => {
    const meta = await generateMetadata({
      params: Promise.resolve({ slug: SLUG }),
    });

    expect(meta.title).toContain("EvenFlow Grand Concert 2026");
    expect(meta.description).toBeDefined();
  });

  it("generateMetadata tra ve title loi khi event khong ton tai", async () => {
    installFetch(() => problem(404, "/problems/not-found"));
    const meta = await generateMetadata({
      params: Promise.resolve({ slug: "khong-ton-tai" }),
    });

    expect(meta.title).toContain("Không tìm thấy");
  });

  it("render day du thong tin su kien, notice va ticket tiers", async () => {
    const jsx = await EventDetailPage({
      params: Promise.resolve({ slug: SLUG }),
    });

    render(jsx);

    expect(
      screen.getByRole("heading", {
        level: 1,
        name: /EvenFlow Grand Concert 2026/i,
      }),
    ).toBeInTheDocument();

    expect(screen.getByText(/Quy chế phòng chờ công bằng/i)).toBeInTheDocument();
    expect(screen.getByText(/Các hạng vé/i)).toBeInTheDocument();
    expect(screen.getByText("Vé Tiêu Chuẩn (GA)")).toBeInTheDocument();
  });

  it("backend loi (503): hien trang loi co lien ket thu lai, khong nem va khong notFound()", async () => {
    installFetch(() => problem(503, "/problems/upstream-unavailable"));
    const jsx = await EventDetailPage({ params: Promise.resolve({ slug: SLUG }) });
    render(jsx);
    expect(screen.getByRole("alert")).toHaveTextContent(/Không tải được sự kiện/);
    expect(screen.getByRole("link", { name: /thử lại/i })).toHaveAttribute("href", `/events/${SLUG}`);
    expect(mockNotFound).not.toHaveBeenCalled();
  });

  it("goi notFound() khi slug khong ton tai", async () => {
    installFetch(() => problem(404, "/problems/not-found"));
    await expect(
      EventDetailPage({
        params: Promise.resolve({ slug: "slug-khong-ton-tai" }),
      }),
    ).rejects.toThrow("NEXT_NOT_FOUND");

    expect(mockNotFound).toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// EV-188 AC-3 (axe incomplete `aria-valid-attr-value` + `aria-prohibited-attr`): section hang ve
// chi tro aria-labelledby khi heading thuc su ton tai; moi aria-labelledby tren trang tro toi id co that.
// ---------------------------------------------------------------------------
describe("EventDetailPage — section hang ve co ten (EV-188 I1)", () => {
  function danglingLabelledby(container: HTMLElement): string[] {
    return Array.from(container.querySelectorAll("[aria-labelledby]")).flatMap((el) =>
      (el.getAttribute("aria-labelledby") ?? "")
        .split(/\s+/)
        .filter((id) => id !== "" && container.ownerDocument.getElementById(id) === null),
    );
  }

  it("co hang ve: section la region co ten 'Các hạng vé', khong tham chieu id hong", async () => {
    const jsx = await EventDetailPage({ params: Promise.resolve({ slug: SLUG }) });
    const { container } = render(jsx);

    const region = screen.getByRole("region", { name: "Các hạng vé" });
    expect(region.tagName).toBe("SECTION");
    expect(region).toHaveAttribute("aria-labelledby", "ticket-tiers-heading");
    expect(danglingLabelledby(container)).toEqual([]);
  });

  it("khong co hang ve: section KHONG co aria-labelledby, khong tham chieu id hong", async () => {
    serveEvent({ tiers: [] });

    const jsx = await EventDetailPage({ params: Promise.resolve({ slug: SLUG }) });
    const { container } = render(jsx);

    expect(screen.getByText("Chưa có thông tin hạng vé cho sự kiện này.")).toBeInTheDocument();
    const sections = Array.from(container.querySelectorAll("section"));
    expect(sections.length).toBeGreaterThan(0);
    for (const section of sections) {
      expect(section).not.toHaveAttribute("aria-labelledby");
    }
    expect(danglingLabelledby(container)).toEqual([]);
  });
});
