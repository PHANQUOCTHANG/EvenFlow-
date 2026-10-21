"use client";

import { useEffect, useState, useCallback } from "react";

import { logout as logoutRequest } from "@/lib/api/auth";
import { getSession, SESSION_EVENT, type Role as SessionRole } from "@/lib/session";

export type Role = SessionRole;

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

  // Phien lay tu lib/session (ef_user), khong doc email tu JWT. Access token het han van tinh la da dang nhap:
  // apiFetch se refresh o lan goi ke tiep, client khong tu quyet dinh han.
  const loadAuth = useCallback(() => {
    const session = getSession();
    setUser(session ? { ...session.user } : null);
    setToken(session ? session.accessToken : null);
    setLoading(false);
  }, []);

  useEffect(() => {
    loadAuth();

    // Dong bo giua cac tab (storage) va trong cung tab (ef:session)
    const handleStorageChange = (e: StorageEvent) => {
      if (e.key === null || e.key === "ef_token" || e.key === "ef_refresh" || e.key === "ef_user") {
        loadAuth();
      }
    };

    window.addEventListener("storage", handleStorageChange);
    window.addEventListener(SESSION_EVENT, loadAuth);
    return () => {
      window.removeEventListener("storage", handleStorageChange);
      window.removeEventListener(SESSION_EVENT, loadAuth);
    };
  }, [loadAuth]);

  const logout = useCallback(async () => {
    if (typeof window === "undefined") return;
    // Thu hoi refresh token tren server; phien client luon bi xoa du loi.
    await logoutRequest();
    setUser(null);
    setToken(null);
    window.location.href = "/auth/sign-in";
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
