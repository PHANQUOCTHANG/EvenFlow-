import { redirect } from "next/navigation";
import { describe, expect, it, vi } from "vitest";

import OrganizerIndexPage from "./page";

vi.mock("next/navigation", () => ({
  redirect: vi.fn(),
}));

describe("OrganizerIndexPage route", () => {
  it("chuyển hướng sang /organizer/events", () => {
    OrganizerIndexPage();
    expect(redirect).toHaveBeenCalledWith("/organizer/events");
  });
});
