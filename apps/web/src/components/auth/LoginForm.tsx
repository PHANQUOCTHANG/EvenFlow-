"use client";

import { useState } from "react";
import Link from "next/link";
import Image from "next/image";
import { loginUser, loginWithGoogle } from "@/lib/api/auth";
import { GoogleOAuthProvider, GoogleLogin } from "@react-oauth/google";
import { Alert, Button, useToast } from "@/components/ui";


// --- Icons ---
function MailIcon({ className = "size-5" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <rect width="20" height="16" x="2" y="4" rx="2" />
      <path d="m22 7-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 7" />
    </svg>
  );
}

function LockIcon({ className = "size-5" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <rect width="18" height="11" x="3" y="11" rx="2" ry="2" />
      <path d="M7 11V7a5 5 0 0 1 10 0v4" />
    </svg>
  );
}

function EyeIcon({ className = "size-5" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M2.062 12.348a1 1 0 0 1 0-.696 10.75 10.75 0 0 1 19.876 0 1 1 0 .696 10.75 10.75 0 0 1-19.876 0" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  );
}

function EyeOffIcon({ className = "size-5" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="m2 2 20 20" />
      <path d="M9.88 9.88a3 3 0 1 0 4.24 4.24" />
      <path d="M10.73 5.08A10.43 10.43 0 0 1 12 5c7 0 10 7 10 7a13.16 13.16 0 0 1-1.67 2.68" />
      <path d="M6.61 6.61A13.526 13.526 0 0 0 2 12s3 7 10 7a9.74 9.74 0 0 0 5.39-1.61" />
    </svg>
  );
}

function LoginIcon({ className = "size-5" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4" />
      <polyline points="10 17 15 12 10 7" />
      <line x1="15" x2="3" y1="12" y2="12" />
    </svg>
  );
}

function GoogleIcon({ className = "size-5" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" fill="#4285F4"/>
      <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853"/>
      <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" fill="#FBBC05"/>
      <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335"/>
      <path d="M1 1h22v22H1z" fill="none"/>
    </svg>
  );
}

function MobileIcon({ className = "size-5" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round">
      <rect width="14" height="20" x="5" y="2" rx="2" ry="2" />
      <path d="M12 18h.01" />
    </svg>
  );
}

export default function LoginForm() {
  const { toast } = useToast();

  const [identifier, setIdentifier] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [remember, setRemember] = useState(true);
  const [loading, setLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState("");
  const [isRateLimited, setIsRateLimited] = useState(false);

  const handleGoogleLogin = () => {
    toast({
      variant: "default",
      title: "Google Login",
      description: "Hệ thống đang kết nối với Google OAuth...",
    });
    // TODO: Tích hợp Google OAuth Client
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (isRateLimited) {
      return;
    }

    setLoading(true);
    setErrorMessage("");

    try {
      const res = await loginUser(identifier, password);
      
      // Save token (Task EV-112)
      if (res.token) {
        if (remember) {
          localStorage.setItem("ef_token", res.token);
        } else {
          sessionStorage.setItem("ef_token", res.token);
        }
      }

      toast({
        variant: "success",
        title: "Đăng nhập thành công!",
        description: "Đang tự động chuyển tiếp...",
      });
      setTimeout(() => {
        window.location.href = "/events";
      }, 1000);
    } catch (err: any) {
      if (err?.status === 429) {
        setIsRateLimited(true);
        toast({
          variant: "warning",
          title: "Giới hạn đăng nhập",
          description: "Bạn đã đăng nhập sai quá nhiều lần. Vui lòng thử lại sau ít phút."
        });
      } else {
        toast({
          variant: "error",
          title: "Lỗi đăng nhập",
          description: err?.detail || 'Email hoặc mật khẩu không chính xác. Vui lòng kiểm tra lại thông tin.'
        });
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="flex flex-col w-full">
      {/* Main Login Card */}
      <div className="bg-white rounded-[24px] p-6 sm:p-7 shadow-xl shadow-black/5 border border-border-subtle flex flex-col gap-5 relative overflow-hidden">
        {/* Subtle top highlight for premium feel */}
        <div className="absolute top-0 left-0 right-0 h-1 bg-gradient-to-r from-indigo-500 via-blue-500 to-indigo-500 opacity-20" />

        <div className="flex items-center justify-between mb-2 w-full">
          <Image 
            src="/logo.png" 
            alt="EventFlow Logo" 
            width={100} 
            height={28} 
            className="h-7 w-auto object-contain drop-shadow-sm" 
            priority
          />
          <h1 className="text-xl sm:text-2xl font-extrabold text-fg tracking-tight text-center whitespace-nowrap px-2">Đăng nhập</h1>
          <div className="w-[100px]" />
        </div>



        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <div className="flex items-center justify-between">
              <label htmlFor="login-identifier" className="text-[13.5px] font-bold text-fg">
                Email hoặc Số điện thoại
              </label>
              <span className="text-[11px] font-bold text-indigo-600 bg-indigo-50 px-2 py-0.5 rounded-full">Bắt buộc</span>
            </div>
            <div className="relative">
              <div className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-4 text-fg-muted/80">
                <MailIcon className="size-[18px]" />
              </div>
              <input
                id="login-identifier"
                type="text"
                required
                disabled={loading || isRateLimited}
                value={identifier}
                onChange={(e) => setIdentifier(e.target.value)}
                placeholder="Nhập email hoặc SĐT"
                className="ef-focus-ring h-[48px] w-full rounded-[12px] border border-border-strong bg-white pl-11 pr-4 text-[14px] text-fg placeholder:text-fg-muted/60 transition-colors disabled:cursor-not-allowed disabled:opacity-60 hover:border-indigo-400 focus:border-indigo-500"
              />
            </div>
          </div>

          <div className="flex flex-col gap-1.5">
            <div className="flex items-center justify-between">
              <label htmlFor="login-password" className="text-[13.5px] font-bold text-fg">
                Mật khẩu
              </label>
              <Link href="/auth/forgot-password" className="ef-focus-ring rounded-xs text-[12.5px] font-semibold text-indigo-600 hover:text-indigo-700 hover:underline">
                Quên mật khẩu?
              </Link>
            </div>
            <div className="relative">
              <div className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-4 text-fg-muted/80">
                <LockIcon className="size-[18px]" />
              </div>
              <input
                id="login-password"
                type={showPassword ? "text" : "password"}
                required
                disabled={loading || isRateLimited}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••••••"
                className="ef-focus-ring h-[48px] w-full rounded-[12px] border border-border-strong bg-white pl-11 pr-12 text-[14px] text-fg placeholder:text-fg-muted/60 transition-colors disabled:cursor-not-allowed disabled:opacity-60 font-medium tracking-[0.2em] hover:border-indigo-400 focus:border-indigo-500"
              />
              <button
                type="button"
                onClick={() => setShowPassword(!showPassword)}
                aria-label={showPassword ? "Ẩn mật khẩu" : "Hiện mật khẩu"}
                aria-pressed={showPassword}
                className="ef-focus-ring absolute inset-y-0 right-0 flex items-center pr-4 text-fg-muted transition-colors hover:text-fg"
              >
                {showPassword ? <EyeOffIcon className="size-[18px]" /> : <EyeIcon className="size-[18px]" />}
              </button>
            </div>
          </div>

          <div className="flex items-center justify-between mt-1">
            <label className="flex cursor-pointer select-none items-center gap-2.5">
              <input
                type="checkbox"
                checked={remember}
                disabled={loading || isRateLimited}
                onChange={(e) => setRemember(e.target.checked)}
                className="ef-focus-ring size-[18px] rounded-[4px] border-border-strong text-indigo-600 accent-indigo-600"
              />
              <span className="text-[13px] font-medium text-fg-muted">
                Ghi nhớ đăng nhập
              </span>
            </label>
            <MobileIcon className="size-[16px] text-indigo-600 hidden sm:block" />
          </div>

          <Button
            type="submit"
            loading={loading}
            disabled={isRateLimited}
            className="mt-1 h-[48px] w-full rounded-[12px] bg-indigo-600 hover:bg-indigo-700 text-white text-[15px] font-bold shadow-md shadow-indigo-600/20 hover:shadow-indigo-600/30 transition-all active:scale-[0.98]"
          >
            {loading ? (
              <span>Đang xử lý...</span>
            ) : (
              <span className="inline-flex items-center justify-center gap-2.5">
                <LoginIcon className="size-[18px]" />
                <span>Đăng nhập</span>
              </span>
            )}
          </Button>
        </form>

        {/* Divider */}
        <div className="relative flex items-center py-1">
          <div className="flex-grow border-t border-border-subtle"></div>
          <span className="flex-shrink-0 mx-4 text-fg-muted text-[12px] font-semibold tracking-wider uppercase">Hoặc</span>
          <div className="flex-grow border-t border-border-subtle"></div>
        </div>

        {/* Alternative Action (Google) */}
        <div className="w-full flex justify-center">
          <GoogleOAuthProvider clientId={process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID || ""}>
            <GoogleLogin
              onSuccess={async (credentialResponse) => {
                if (credentialResponse.credential) {
                  try {
                    setLoading(true);
                    const res = await loginWithGoogle(credentialResponse.credential);
                    if (res.token) {
                      localStorage.setItem("ef_token", res.token);
                    }
                    toast({
                      variant: "success",
                      title: "Đăng nhập thành công!",
                      description: "Đang tự động chuyển tiếp...",
                    });
                    setTimeout(() => {
                      window.location.href = "/events";
                    }, 1000);
                  } catch (err: any) {
                    toast({
                      variant: "error",
                      title: "Lỗi đăng nhập",
                      description: err?.detail || 'Không thể đăng nhập bằng Google.',
                    });
                  } finally {
                    setLoading(false);
                  }
                }
              }}
              onError={() => {
                toast({
                  variant: "error",
                  title: "Lỗi đăng nhập",
                  description: "Đăng nhập Google bị hủy hoặc thất bại.",
                });
              }}
              width="100%"
            />
          </GoogleOAuthProvider>
        </div>

        <div className="mt-1 text-center text-[13.5px] text-fg-muted border-t border-border-subtle pt-4">
          Chưa có tài khoản EventFlow?{" "}
          <Link href="/auth/sign-up" className="font-bold text-indigo-600 hover:text-indigo-700 hover:underline">
            Đăng ký ngay
          </Link>
        </div>
      </div>
    </div>
  );
}
