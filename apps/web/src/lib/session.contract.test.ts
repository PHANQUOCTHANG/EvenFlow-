/**
 * Contract test: lib/session.ts (AC-1, AC-3, AC-6, AC-7).
 * Khoa luu tru dong bang: localStorage `ef_token` (access), `ef_refresh`, `ef_user` (JSON).
 */
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { clearSession, getSession, setSession, type Session } from "@/lib/session";

const SESSION: Session = {
  accessToken: "access-1",
  refreshToken: "refresh-1",
  user: { id: "u-1", role: "buyer", email: "a@example.com" },
};

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});
afterEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

describe("session (AC-1, AC-7)", () => {
  it("getSession tra null khi chua co phien", () => {
    expect(getSession()).toBeNull();
  });

  it("setSession ghi dung 3 khoa localStorage", () => {
    setSession(SESSION);
    expect(localStorage.getItem("ef_token")).toBe("access-1");
    expect(localStorage.getItem("ef_refresh")).toBe("refresh-1");
    expect(JSON.parse(localStorage.getItem("ef_user") as string)).toEqual({
      id: "u-1",
      role: "buyer",
      email: "a@example.com",
    });
  });

  it("getSession doc lai dung nhung gi setSession da ghi", () => {
    setSession(SESSION);
    expect(getSession()).toEqual(SESSION);
  });

  it("refreshToken null: khong ghi ef_refresh va getSession tra refreshToken null", () => {
    setSession({ ...SESSION, refreshToken: null });
    expect(localStorage.getItem("ef_refresh")).toBeNull();
    expect(getSession()?.refreshToken).toBeNull();
    expect(getSession()?.accessToken).toBe("access-1");
  });

  it("setSession de len phien cu, khong de sot refresh token cu", () => {
    setSession(SESSION);
    setSession({ accessToken: "access-2", refreshToken: null, user: { id: "u-2", role: "organizer" } });
    expect(localStorage.getItem("ef_refresh")).toBeNull();
    expect(getSession()).toEqual({
      accessToken: "access-2",
      refreshToken: null,
      user: { id: "u-2", role: "organizer" },
    });
  });

  it("email la tuy chon", () => {
    setSession({ accessToken: "a", refreshToken: "r", user: { id: "u", role: "ops" } });
    expect(getSession()?.user).toEqual({ id: "u", role: "ops" });
  });
});

describe("clearSession (AC-4, AC-6)", () => {
  it("xoa ca 3 khoa", () => {
    setSession(SESSION);
    clearSession();
    expect(localStorage.getItem("ef_token")).toBeNull();
    expect(localStorage.getItem("ef_refresh")).toBeNull();
    expect(localStorage.getItem("ef_user")).toBeNull();
    expect(getSession()).toBeNull();
  });

  it("goi khi chua co phien khong nem loi", () => {
    expect(() => clearSession()).not.toThrow();
  });
});

describe("getSession bao ve du lieu hong", () => {
  it("ef_user khong phai JSON -> null (khong nem)", () => {
    localStorage.setItem("ef_token", "a");
    localStorage.setItem("ef_user", "{not json");
    expect(() => getSession()).not.toThrow();
    expect(getSession()).toBeNull();
  });

  it("co ef_token nhung thieu ef_user -> null (khong du de dung lam phien)", () => {
    localStorage.setItem("ef_token", "a");
    expect(getSession()).toBeNull();
  });

  it("co ef_user nhung thieu ef_token -> null", () => {
    localStorage.setItem("ef_user", JSON.stringify({ id: "u", role: "buyer" }));
    expect(getSession()).toBeNull();
  });
});

describe("persist: local | session (plan round 2, spec muc 3)", () => {
  it("persist 'session' ghi 3 khoa vao sessionStorage, khong dong vao localStorage", () => {
    setSession(SESSION, { persist: "session" });
    expect(sessionStorage.getItem("ef_token")).toBe("access-1");
    expect(sessionStorage.getItem("ef_refresh")).toBe("refresh-1");
    expect(JSON.parse(sessionStorage.getItem("ef_user") as string)).toMatchObject({ id: "u-1" });
    expect(localStorage.getItem("ef_token")).toBeNull();
    expect(localStorage.getItem("ef_user")).toBeNull();
  });

  it("getSession doc duoc phien nam o sessionStorage", () => {
    setSession(SESSION, { persist: "session" });
    expect(getSession()).toEqual(SESSION);
  });

  it("doc localStorage truoc: neu co ca hai thi uu tien local", () => {
    setSession({ ...SESSION, accessToken: "from-session" }, { persist: "session" });
    setSession({ ...SESSION, accessToken: "from-local" }, { persist: "local" });
    expect(getSession()?.accessToken).toBe("from-local");
  });

  it("mac dinh (khong tuy chon) la localStorage", () => {
    setSession(SESSION);
    expect(localStorage.getItem("ef_token")).toBe("access-1");
    expect(sessionStorage.getItem("ef_token")).toBeNull();
  });

  it("clearSession xoa ca hai noi luu", () => {
    setSession(SESSION, { persist: "session" });
    setSession(SESSION, { persist: "local" });
    clearSession();
    for (const store of [localStorage, sessionStorage]) {
      expect(store.getItem("ef_token")).toBeNull();
      expect(store.getItem("ef_refresh")).toBeNull();
      expect(store.getItem("ef_user")).toBeNull();
    }
    expect(getSession()).toBeNull();
  });

  it("ghi lai bang persist khac khong de lai ban sao cu o noi luu kia", () => {
    setSession(SESSION, { persist: "local" });
    setSession({ ...SESSION, accessToken: "access-2" }, { persist: "session" });
    expect(localStorage.getItem("ef_token")).toBeNull();
    expect(getSession()?.accessToken).toBe("access-2");
  });
});
