"use client";

import React, { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useAuth, Role } from "@/hooks/use-auth";

interface RoleGuardProps {
  allowedRoles: Role[];
  children: React.ReactNode;
  fallback?: React.ReactNode;
  redirectTo?: string;
}

export function RoleGuard({
  allowedRoles,
  children,
  fallback,
  redirectTo = "/unauthorized",
}: RoleGuardProps) {
  const router = useRouter();
  const { user, loading, isAuthenticated, hasRole } = useAuth();

  useEffect(() => {
    if (loading) return;

    if (!isAuthenticated) {
      router.push(`/auth/sign-in?redirect=${encodeURIComponent(window.location.pathname)}`);
      return;
    }

    if (!hasRole(allowedRoles)) {
      router.push(redirectTo);
    }
  }, [loading, isAuthenticated, hasRole, allowedRoles, redirectTo, router]);

  if (loading) {
    return (
      <div className="flex min-h-[300px] items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-indigo-500 border-t-transparent" />
      </div>
    );
  }

  if (!isAuthenticated || !user || !hasRole(allowedRoles)) {
    return fallback ? <>{fallback}</> : null;
  }

  return <>{children}</>;
}
