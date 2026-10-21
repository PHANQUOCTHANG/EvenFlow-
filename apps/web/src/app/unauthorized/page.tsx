"use client";

import Link from "next/link";
import { useAuth } from "@/hooks/use-auth";

// lang="vi" o goc: trang chua dich, tranh bi doc nhu tieng Anh khi <html lang> la en (WCAG 3.1.2).
export default function UnauthorizedPage() {
  const { user, logout } = useAuth();

  return (
    <div lang="vi" className="flex min-h-[80vh] flex-col items-center justify-center px-4 text-center">
      <div className="mx-auto max-w-md rounded-2xl border border-red-500/20 bg-slate-900/60 p-8 shadow-2xl backdrop-blur-xl">
        <div className="mx-auto mb-6 flex h-16 w-16 items-center justify-center rounded-full bg-red-500/10 text-red-500 ring-8 ring-red-500/5">
          <svg
            className="h-8 w-8"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2}
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
            />
          </svg>
        </div>

        <h1 className="mb-2 text-2xl font-bold tracking-tight text-white">
          403 - Quyền truy cập bị từ chối
        </h1>

        <p className="mb-6 text-sm text-slate-400">
          Tài khoản của bạn{" "}
          {user ? (
            <span className="font-semibold text-indigo-400">
              (vai trò: {user.role})
            </span>
          ) : (
            ""
          )}{" "}
          không có đủ đặc quyền để truy cập vào tài nguyên hoặc thực hiện hành động này theo quy định phân quyền của EventFlow.
        </p>

        <div className="flex flex-col gap-3 sm:flex-row sm:justify-center">
          <Link
            href="/events"
            className="inline-flex items-center justify-center rounded-xl bg-indigo-600 px-5 py-2.5 text-sm font-medium text-white shadow-lg shadow-indigo-500/20 transition-all hover:bg-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 focus:ring-offset-slate-900"
          >
            Khám phá sự kiện
          </Link>

          <button
            onClick={() => void logout()}
            className="inline-flex items-center justify-center rounded-xl border border-slate-700 bg-slate-800/80 px-5 py-2.5 text-sm font-medium text-slate-300 transition-all hover:bg-slate-700 hover:text-white focus:outline-none focus:ring-2 focus:ring-slate-500"
          >
            Đổi tài khoản khác
          </button>
        </div>
      </div>
    </div>
  );
}
