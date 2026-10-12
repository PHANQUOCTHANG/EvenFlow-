import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

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
