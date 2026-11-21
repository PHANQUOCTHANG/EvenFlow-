/**
 * ops-warroom AC-26 (client-side list, no SSR token fetch), AC-27 (RoleGuard on the (ops) layout).
 *
 * ASSUMPTIONS:
 *  - (ops)/layout.tsx wraps its children in RoleGuard allowedRoles=["ops"] (hooks/use-auth + next/navigation mocked here).
 *  - /ops/events page default export renders (sync or async) a client list that loads events with getAllPublicEvents
 *    from "@/lib/event-service"; states: loading, empty text "Chưa có sự kiện nào đang hoặc sắp mở bán", error + "Thử lại".
 *  - /ops/events/[eventId] page default export performs NO fetch and does not call getOpsWarRoom itself; it renders OpsWarRoom
 *    with initialSnapshot null (the client hook fetches).
 */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({
  push: vi.fn(),
  auth: { user: null as null | { id: string; role: string }, loading: false, isAuthenticated: false },
  getAll: vi.fn(),
  getWarRoom: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  usePathname: () => "/ops/events",
  useRouter: () => ({ push: h.push, replace: vi.fn() }),
  notFound: vi.fn(),
}));
vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({
    user: h.auth.user,
    loading: h.auth.loading,
    isAuthenticated: h.auth.isAuthenticated,
    hasRole: (roles: string[]) => !!h.auth.user && roles.includes(h.auth.user.role),
  }),
}));
vi.mock("@/lib/event-service", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/event-service")>();
  return { ...actual, getAllPublicEvents: h.getAll };
});
vi.mock("@/lib/ops-service", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/ops-service")>();
  return { ...actual, getOpsWarRoom: h.getWarRoom };
});

import OpsLayout from "./layout";
import OpsEventsPage from "./ops/events/page";
import OpsWarRoomPage from "./ops/events/[eventId]/page";

const CHILD = "Noi dung ops";

function setUser(role: string | null) {
  h.auth.user = role ? { id: "u1", role } : null;
  h.auth.isAuthenticated = role !== null;
  h.auth.loading = false;
}

beforeEach(() => {
  h.push.mockReset();
  h.getAll.mockReset();
  h.getWarRoom.mockReset();
  h.getWarRoom.mockImplementation(() => new Promise(() => {}));
});
afterEach(cleanup);

describe("(ops)/layout RoleGuard (AC-27)", () => {
  it("ops sees the content, no redirect", () => {
    setUser("ops");
    render(<OpsLayout><p>{CHILD}</p></OpsLayout>);
    expect(screen.getByText(CHILD)).toBeInTheDocument();
    expect(h.push).not.toHaveBeenCalled();
  });

  it.each(["buyer", "organizer", "moderator"])("%s is redirected to /unauthorized and never sees the content", async (role) => {
    setUser(role);
    render(<OpsLayout><p>{CHILD}</p></OpsLayout>);
    expect(screen.queryByText(CHILD)).toBeNull();
    await waitFor(() => expect(h.push).toHaveBeenCalledWith("/unauthorized"));
  });

  it("anonymous is sent to login with a redirect back", async () => {
    setUser(null);
    render(<OpsLayout><p>{CHILD}</p></OpsLayout>);
    expect(screen.queryByText(CHILD)).toBeNull();
    await waitFor(() => expect(h.push).toHaveBeenCalledTimes(1));
    expect(String(h.push.mock.calls[0][0])).toMatch(/^\/auth\/sign-in\?redirect=/);
  });

  it("while the session loads, content is not shown", () => {
    setUser("ops");
    h.auth.loading = true;
    render(<OpsLayout><p>{CHILD}</p></OpsLayout>);
    expect(screen.queryByText(CHILD)).toBeNull();
  });
});

const ev = (id: string, title: string, state = "ON_SALE") => ({ id, slug: id, title, state, tiers: [] });

describe("/ops/events list (AC-26)", () => {
  it("loads client-side from the public list and links to each war room", async () => {
    h.getAll.mockResolvedValue([ev("e1", "Rock Fest"), ev("e2", "Jazz Night", "SCHEDULED")]);
    const { container } = render(await OpsEventsPage());
    expect(await screen.findByText("Rock Fest")).toBeInTheDocument();
    expect(screen.getByText("Jazz Night")).toBeInTheDocument();
    expect(container.querySelector('a[href="/ops/events/e1"]')).not.toBeNull();
    expect(container.querySelector('a[href="/ops/events/e2"]')).not.toBeNull();
  });

  it("shows a loading state first", async () => {
    h.getAll.mockReturnValue(new Promise(() => {}));
    render(await OpsEventsPage());
    expect(screen.queryByRole("link")).toBeNull();
    expect(screen.getByText(/đang tải/i)).toBeInTheDocument();
  });

  it("empty list => explicit empty text", async () => {
    h.getAll.mockResolvedValue([]);
    render(await OpsEventsPage());
    expect(await screen.findByText("Chưa có sự kiện nào đang hoặc sắp mở bán")).toBeInTheDocument();
  });

  it("error => message and a retry that reloads", async () => {
    h.getAll.mockRejectedValueOnce(new Error("down")).mockResolvedValueOnce([ev("e1", "Rock Fest")]);
    render(await OpsEventsPage());
    const retry = await screen.findByRole("button", { name: /thử lại/i });
    fireEvent.click(retry);
    expect(await screen.findByText("Rock Fest")).toBeInTheDocument();
    expect(h.getAll).toHaveBeenCalledTimes(2);
  });
});

describe("/ops/events/[eventId] page (AC-26)", () => {
  it("does no server-side fetch: getOpsWarRoom is not called by the page function itself", async () => {
    const fetchSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);
    try {
      const el = await OpsWarRoomPage({ params: Promise.resolve({ eventId: "e1" }) });
      expect(h.getWarRoom).not.toHaveBeenCalled();
      expect(fetchSpy).not.toHaveBeenCalled();
      expect(el).toBeTruthy();
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("the rendered shell loads data on the client (hook), starting from no snapshot", async () => {
    const el = await OpsWarRoomPage({ params: Promise.resolve({ eventId: "e1" }) });
    render(el);
    await waitFor(() => expect(h.getWarRoom).toHaveBeenCalledWith("e1"));
  });
});
