"use client";

import { useState } from "react";
import { registerUser } from "@/lib/api/auth";

export default function RegisterForm() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError("");

    if (password !== confirmPassword) {
      setError("Mật khẩu xác nhận không khớp.");
      setLoading(false);
      return;
    }

    try {
      await registerUser(email, password);
      setSuccess(true);
    } catch (err: any) {
      setError(err.detail || "Đã xảy ra lỗi khi đăng ký.");
    } finally {
      setLoading(false);
    }
  };

  if (success) {
    return (
      <div className="w-full max-w-md p-8 text-center bg-white rounded-[16px] shadow-sm border border-[#DCE2EA]">
        <h2 className="text-xl font-semibold text-[#18794E] mb-2">Đăng ký thành công!</h2>
        <p className="text-[#526071] mb-6">Tài khoản của bạn đã được tạo.</p>
        <a
          href="/auth/sign-in"
          className="inline-block px-6 py-2 bg-[#5B55E7] text-white rounded-[10px] hover:bg-[#4338ca]"
        >
          Về trang Đăng nhập
        </a>
      </div>
    );
  }

  return (
    <div className="w-full max-w-md bg-white rounded-[16px] shadow-sm border border-[#DCE2EA] p-8">
      <h1 className="text-2xl font-bold text-[#182230] mb-2">Đăng ký tài khoản mới</h1>
      <p className="text-[#526071] text-sm mb-6">
        Vui lòng sử dụng email thật để nhận mã vé
      </p>

      {error && (
        <div className="mb-4 p-3 bg-[#fef2f2] text-[#B42318] text-sm rounded-[10px] border border-[#fca5a5]">
          {error}
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label className="block text-sm font-medium text-[#182230] mb-1">
            Email
          </label>
          <input
            type="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            className="w-full px-4 py-2 border border-[#DCE2EA] rounded-[10px] focus:outline-none focus:ring-2 focus:ring-[#5B55E7]"
          />
        </div>

        <div>
          <label className="block text-sm font-medium text-[#182230] mb-1">
            Mật khẩu (Tối thiểu 8 ký tự)
          </label>
          <div className="relative">
            <input
              type={showPassword ? "text" : "password"}
              required
              minLength={8}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full px-4 py-2 border border-[#DCE2EA] rounded-[10px] focus:outline-none focus:ring-2 focus:ring-[#5B55E7]"
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

        <div>
          <label className="block text-sm font-medium text-[#182230] mb-1">
            Xác nhận mật khẩu
          </label>
          <input
            type={showPassword ? "text" : "password"}
            required
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            className="w-full px-4 py-2 border border-[#DCE2EA] rounded-[10px] focus:outline-none focus:ring-2 focus:ring-[#5B55E7]"
          />
        </div>

        <button
          type="submit"
          disabled={loading}
          className="w-full py-2.5 bg-[#5B55E7] text-white font-medium rounded-[10px] hover:bg-[#4338ca] transition-colors disabled:bg-[#a5b4fc]"
        >
          {loading ? "Đang xử lý..." : "Tạo tài khoản"}
        </button>
      </form>

      <div className="mt-6 text-center">
        <span className="text-sm text-[#526071]">Đã có tài khoản? </span>
        <a href="/auth/sign-in" className="text-sm text-[#5B55E7] hover:underline">
          Đăng nhập ngay
        </a>
      </div>
    </div>
  );
}
