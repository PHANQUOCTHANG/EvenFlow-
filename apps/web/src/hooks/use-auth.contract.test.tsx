/**
 * Contract test: hooks/use-auth.ts doc phien tu lib/session (AC-7).
 * Khoa: ef_token (access), ef_refresh, ef_user (JSON). Khong doc email tu JWT nua.
 */
import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuth } from "@/hooks/use-auth";

function b64url(obj: unknown) {
  return btoa(JSON.stringify(obj)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}
function jwt(payload: Record<string, unknown>) {
  return `${b64url({ alg: "HS256", typ: "JWT" })}.${b64url(payload)}.sig`;
}

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});
afterEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

describe("useAuth (AC-7)", () => {
  it("khong co phien -> user null, loading xong, chua dang nhap", async () => {
    const { result } = renderHook(() => useAuth());
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.user).toBeNull();
    expect(result.current.isAuthenticated).toBe(false);
  });

  it("tra {id, role, email} tu ef_user", async () => {
    localStorage.setItem("ef_token", jwt({ sub: "u-1", role: "buyer", exp: Math.floor(Date.now() / 1000) + 600 }));
    localStorage.setItem("ef_refresh", "r1");
    localStorage.setItem("ef_user", JSON.stringify({ id: "u-1", role: "organizer", email: "o@example.com" }));
    const { result } = renderHook(() => useAuth());
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.user).toEqual({ id: "u-1", role: "organizer", email: "o@example.com" });
    expect(result.current.isAuthenticated).toBe(true);
    expect(result.current.isOrganizer).toBe(true);
  });

  it("KHONG doc email tu JWT: ef_user khong co email thi email undefined", async () => {
    localStorage.setItem(
      "ef_token",
      jwt({ sub: "u-1", role: "buyer", email: "from-jwt@example.com", exp: Math.floor(Date.now() / 1000) + 600 }),
    );
    localStorage.setItem("ef_refresh", "r1");
    localStorage.setItem("ef_user", JSON.stringify({ id: "u-1", role: "buyer" }));
    const { result } = renderHook(() => useAuth());
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.user?.email).toBeUndefined();
  });

  it("access token da het han nhung con refresh token -> van dang nhap (refresh o lan goi ke tiep)", async () => {
    localStorage.setItem("ef_token", jwt({ sub: "u-1", role: "buyer", exp: Math.floor(Date.now() / 1000) - 3600 }));
    localStorage.setItem("ef_refresh", "r1");
    localStorage.setItem("ef_user", JSON.stringify({ id: "u-1", role: "buyer" }));
    const { result } = renderHook(() => useAuth());
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.isAuthenticated).toBe(true);
    expect(result.current.user?.id).toBe("u-1");
    // Het han khong duoc lam hook xoa phien.
    expect(localStorage.getItem("ef_refresh")).toBe("r1");
    expect(localStorage.getItem("ef_token")).not.toBeNull();
  });

  it("token khong phai JWT (opaque) nhung co ef_user -> van dang nhap (khong quyet dinh han o client)", async () => {
    localStorage.setItem("ef_token", "opaque-access");
    localStorage.setItem("ef_refresh", "r1");
    localStorage.setItem("ef_user", JSON.stringify({ id: "u-9", role: "ops" }));
    const { result } = renderHook(() => useAuth());
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.user).toEqual({ id: "u-9", role: "ops" });
    expect(result.current.isOps).toBe(true);
  });

  it("hasRole dung vai tro tu ef_user", async () => {
    localStorage.setItem("ef_token", "opaque-access");
    localStorage.setItem("ef_user", JSON.stringify({ id: "u-9", role: "moderator" }));
    const { result } = renderHook(() => useAuth());
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.hasRole(["moderator", "ops"])).toBe(true);
    expect(result.current.hasRole(["buyer"])).toBe(false);
  });
});
