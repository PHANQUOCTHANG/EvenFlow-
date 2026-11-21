"use client";

import type { ReactNode } from "react";

import { RoleGuard } from "@/components/auth/RoleGuard";

/** Chỉ tài khoản moderator mới thấy nội dung. Đây là lớp hiển thị — quyền thật nằm ở gateway và event service. */
export function ModeratorOnly({ children }: { children: ReactNode }) {
  return <RoleGuard allowedRoles={["moderator"]}>{children}</RoleGuard>;
}
