"use client";

import { useEffect, useState, useCallback } from "react";

export type Role = "buyer" | "organizer" | "moderator" | "ops";

export interface AuthUser {
  id: string;
  role: Role;
  email?: string;
}

export interface JwtPayload {
  sub?: string;
  role?: Role | string;
  email?: string;
  exp?: number;
  iat?: number;
  [key: string]: unknown;
}

export function parseJwtPayload(token: string): JwtPayload | null {
  try {
    const parts = token.split(".");
    if (parts.length !== 3) return null;
    const base64Url = parts[1];
    const base64 = base64Url.replace(/-/g, "+").replace(/_/g, "/");
    const jsonPayload = decodeURIComponent(
      atob(base64)
        .split("")
        .map((c) => "%" + ("00" + c.charCodeAt(0).toString(16)).slice(-2))
        .join("")
    );
    return JSON.parse(jsonPayload) as JwtPayload;
  } catch {
    return null;
  }
}

export function useAuth() {
  const [user, setUser] = useState<AuthUser | null>(null);
  const [token, setToken] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const loadAuth = useCallback(() => {
    if (typeof window === "undefined") {
      setLoading(false);
      return;
    }

    const storedToken =
      localStorage.getItem("ef_token") || sessionStorage.getItem("ef_token");

    if (!storedToken) {
      setUser(null);
      setToken(null);
      setLoading(false);
      return;
    }

    const payload = parseJwtPayload(storedToken);
    if (!payload || (typeof payload.exp === "number" && payload.exp * 1000 < Date.now())) {
      // Token hết hạn hoặc không hợp lệ -> xoá
      localStorage.removeItem("ef_token");
      sessionStorage.removeItem("ef_token");
      setUser(null);
      setToken(null);
      setLoading(false);
      return;
    }

    setToken(storedToken);
    setUser({
      id: payload.sub || "",
      role: (payload.role as Role) || "buyer",
      email: typeof payload.email === "string" ? payload.email : undefined,
    });
    setLoading(false);
  }, []);

  useEffect(() => {
    loadAuth();

    // Lắng nghe sự kiện thay đổi storage giữa các tab
    const handleStorageChange = (e: StorageEvent) => {
      if (e.key === "ef_token") {
        loadAuth();
      }
    };

    window.addEventListener("storage", handleStorageChange);
    return () => window.removeEventListener("storage", handleStorageChange);
  }, [loadAuth]);

  const logout = useCallback(() => {
    if (typeof window !== "undefined") {
      localStorage.removeItem("ef_token");
      sessionStorage.removeItem("ef_token");
      setUser(null);
      setToken(null);
      window.location.href = "/auth/sign-in";
    }
  }, []);

  const hasRole = useCallback(
    (allowedRoles: Role[]): boolean => {
      if (!user) return false;
      return allowedRoles.includes(user.role);
    },
    [user]
  );

  return {
    user,
    token,
    loading,
    isAuthenticated: !!user,
    role: user?.role || null,
    isBuyer: user?.role === "buyer",
    isOrganizer: user?.role === "organizer",
    isModerator: user?.role === "moderator",
    isOps: user?.role === "ops",
    hasRole,
    logout,
  };
}
