"use client";

import type { ReactNode } from "react";

import { RoleGuard } from "@/components/auth/RoleGuard";

/** Chỉ tài khoản ops mới thấy nội dung vận hành; còn lại RoleGuard chuyển hướng (đăng nhập / unauthorized). */
export function OpsOnly({ children }: { children: ReactNode }) {
  return <RoleGuard allowedRoles={["ops"]}>{children}</RoleGuard>;
}
