import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as eventService from "@/lib/event-service";

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

describe("EventDetailPage (EVF-111, BR-A4, BR-Q1)", () => {
  it("generateStaticParams tra ve danh sach slug cho ISR", async () => {
    const params = await generateStaticParams();
    expect(params.length).toBeGreaterThan(0);
    expect(params[0]).toHaveProperty("slug");
  });

  it("generateMetadata tra ve metadata chuan SEO cho event", async () => {
    const meta = await generateMetadata({
      params: Promise.resolve({ slug: "evenflow-grand-concert-2026" }),
    });

    expect(meta.title).toContain("EvenFlow Grand Concert 2026");
    expect(meta.description).toBeDefined();
  });

  it("generateMetadata tra ve title loi khi event khong ton tai", async () => {
    const meta = await generateMetadata({
      params: Promise.resolve({ slug: "khong-ton-tai" }),
    });

    expect(meta.title).toContain("Không tìm thấy");
  });

  it("render day du thong tin su kien, notice va ticket tiers", async () => {
    const jsx = await EventDetailPage({
      params: Promise.resolve({ slug: "evenflow-grand-concert-2026" }),
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

  it("goi notFound() khi slug khong ton tai", async () => {
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
  afterEach(() => {
    vi.restoreAllMocks();
  });

  function danglingLabelledby(container: HTMLElement): string[] {
    return Array.from(container.querySelectorAll("[aria-labelledby]")).flatMap((el) =>
      (el.getAttribute("aria-labelledby") ?? "")
        .split(/\s+/)
        .filter((id) => id !== "" && container.ownerDocument.getElementById(id) === null),
    );
  }

  it("co hang ve: section la region co ten 'Các hạng vé', khong tham chieu id hong", async () => {
    const jsx = await EventDetailPage({ params: Promise.resolve({ slug: "evenflow-grand-concert-2026" }) });
    const { container } = render(jsx);

    const region = screen.getByRole("region", { name: "Các hạng vé" });
    expect(region.tagName).toBe("SECTION");
    expect(region).toHaveAttribute("aria-labelledby", "ticket-tiers-heading");
    expect(danglingLabelledby(container)).toEqual([]);
  });

  it("khong co hang ve: section KHONG co aria-labelledby, khong tham chieu id hong", async () => {
    const real = await eventService.getEventBySlug("evenflow-grand-concert-2026");
    expect(real).not.toBeNull();
    vi.spyOn(eventService, "getEventBySlug").mockResolvedValue({ ...(real as NonNullable<typeof real>), tiers: [] });

    const jsx = await EventDetailPage({ params: Promise.resolve({ slug: "evenflow-grand-concert-2026" }) });
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
