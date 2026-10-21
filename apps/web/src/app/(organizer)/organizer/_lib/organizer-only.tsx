"use client";

import type { ReactNode } from "react";

import { RoleGuard } from "@/components/auth/RoleGuard";

/** Chỉ tài khoản organizer mới thấy nội dung; còn lại RoleGuard chuyển hướng (đăng nhập / unauthorized). */
export function OrganizerOnly({ children }: { children: ReactNode }) {
  return <RoleGuard allowedRoles={["organizer"]}>{children}</RoleGuard>;
}
