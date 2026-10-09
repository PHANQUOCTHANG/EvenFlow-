"use client";

import { useState } from "react";
import { loginUser } from "@/lib/api/auth";

export default function LoginForm() {
  const [identifier, setIdentifier] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError("");

    try {
      await loginUser(identifier, password);
      setSuccess(true);
      // TODO: Redirect user or store auth state in context
      window.location.href = "/events";
    } catch (err: any) {
      setError(err.detail || "Đã xảy ra lỗi khi đăng nhập.");
    } finally {
      setLoading(false);
    }
  };

  if (success) {
    return (
      <div className="p-8 text-center bg-white rounded-[16px] shadow-sm border border-[#DCE2EA]">
        <h2 className="text-xl font-semibold text-[#18794E]">Đăng nhập thành công!</h2>
        <p className="mt-2 text-[#526071]">Đang chuyển hướng...</p>
      </div>
    );
  }

  return (
    <div className="w-full max-w-md bg-white rounded-[16px] shadow-sm border border-[#DCE2EA] p-8">
      <h1 className="text-2xl font-bold text-[#182230] mb-2">Đăng nhập tài khoản</h1>
      <p className="text-[#526071] text-sm mb-6">
        Nhập thông tin tài khoản đã kích hoạt căn cước định danh
      </p>

      {error && (
        <div className="mb-4 p-3 bg-[#fef2f2] text-[#B42318] text-sm rounded-[10px] border border-[#fca5a5]">
          {error}
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <div className="flex justify-between mb-1">
            <label className="block text-sm font-medium text-[#182230]">
              Email hoặc Số điện thoại liên kết
            </label>
            <span className="text-xs text-[#5B55E7]">Bắt buộc</span>
          </div>
          <input
            type="text"
            required
            value={identifier}
            onChange={(e) => setIdentifier(e.target.value)}
            placeholder="nguyenvana.citizen@eventflow.vn"
            className="w-full px-4 py-2 border border-[#DCE2EA] rounded-[10px] focus:outline-none focus:ring-2 focus:ring-[#5B55E7] text-[#182230]"
          />
          <p className="mt-1 text-xs text-[#526071]">
            Sử dụng tài khoản đã định danh số định danh cá nhân / CCCD
          </p>
        </div>

        <div>
          <div className="flex justify-between mb-1">
            <label className="block text-sm font-medium text-[#182230]">
              Mật khẩu
            </label>
            <a href="#" className="text-xs text-[#5B55E7] hover:underline">
              Quên mật khẩu?
            </a>
          </div>
          <div className="relative">
            <input
              type={showPassword ? "text" : "password"}
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••••••"
              className="w-full px-4 py-2 border border-[#DCE2EA] rounded-[10px] focus:outline-none focus:ring-2 focus:ring-[#5B55E7] text-[#182230]"
            />
            <button
              type="button"
              onClick={() => setShowPassword(!showPassword)}
              className="absolute right-3 top-2.5 text-[#526071] hover:text-[#182230]"
            >
              {showPassword ? "Ẩn" : "Hiện"}
            </button>
          </div>
        </div>

        <div className="flex items-center">
          <input
            type="checkbox"
            id="remember"
            className="w-4 h-4 text-[#5B55E7] border-[#DCE2EA] rounded focus:ring-[#5B55E7]"
          />
          <label htmlFor="remember" className="ml-2 text-sm text-[#182230]">
            Ghi nhớ phiên làm việc trên thiết bị này
          </label>
        </div>

        <button
          type="submit"
          disabled={loading}
          className="w-full py-2.5 bg-[#5B55E7] text-white font-medium rounded-[10px] hover:bg-[#4338ca] transition-colors disabled:bg-[#a5b4fc]"
        >
          {loading ? "Đang xử lý..." : "Đăng nhập và Tiếp tục"}
        </button>
      </form>

      <div className="mt-8 text-center bg-[#F5F7FA] p-4 rounded-[10px]">
        <p className="text-sm text-[#182230]">
          Chưa có tài khoản EventFlow?
        </p>
        <p className="text-xs text-[#526071] mb-3">
          Đăng ký mới để xác thực số định danh
        </p>
        <a
          href="/auth/sign-up"
          className="inline-block w-full py-2 bg-[#EEEDFF] text-[#5B55E7] font-medium rounded-[10px] hover:bg-[#e0e7ff] transition-colors"
        >
          Đăng ký mới
        </a>
      </div>
    </div>
  );
}
