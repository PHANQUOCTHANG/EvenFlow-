"use client";

import { useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { loginUser } from "@/lib/api/auth";
import { Alert, Badge, Button, useToast } from "@/components/ui";

export type SimulationMode = "default" | "pending" | "error" | "ratelimit";

// --- Icons ---
function ArrowLeftIcon({ className = "size-5" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="m12 19-7-7 7-7" />
      <path d="M19 12H5" />
    </svg>
  );
}

function SlidersIcon({ className = "size-4" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <line x1="4" x2="20" y1="21" y2="21" />
      <line x1="4" x2="20" y1="14" y2="14" />
      <line x1="4" x2="20" y1="7" y2="7" />
      <circle cx="8" cy="21" r="2" />
      <circle cx="16" cy="14" r="2" />
      <circle cx="10" cy="7" r="2" />
    </svg>
  );
}

function CalendarIcon({ className = "size-3.5" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <rect width="18" height="18" x="3" y="4" rx="2" ry="2" />
      <line x1="16" x2="16" y1="2" y2="6" />
      <line x1="8" x2="8" y1="2" y2="6" />
      <line x1="3" x2="21" y1="10" y2="10" />
    </svg>
  );
}

function ArrowRightIcon({ className = "size-4" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M5 12h14" />
      <path d="m12 5 7 7-7 7" />
    </svg>
  );
}

function ShieldCheckIcon({ className = "size-5" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.8 17 5 19 5a1 1 0 0 1 1 1z" />
      <path d="m9 12 2 2 4-4" />
    </svg>
  );
}

function MailIcon({ className = "size-5" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <rect width="20" height="16" x="2" y="4" rx="2" />
      <path d="m22 7-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 7" />
    </svg>
  );
}

function IdCardIcon({ className = "size-4" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <rect width="18" height="14" x="3" y="5" rx="2" />
      <circle cx="9" cy="11" r="2" />
      <path d="M15 10h2" />
      <path d="M15 14h2" />
      <path d="M6 16c0-1.5 1.5-2 3-2s3 .5 3 2" />
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

function DeviceShieldIcon({ className = "size-5" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <rect width="14" height="20" x="5" y="2" rx="2" ry="2" />
      <path d="M12 18h.01" />
      <path d="M9 10h6v4H9z" />
      <path d="M10 10V8a2 2 0 1 1 4 0v2" />
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

function FingerprintIcon({ className = "size-5" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M12 10a2 2 0 0 0-2 2c0 1.02-.1 2.51-.26 3" />
      <path d="M14 13.12c0 2.38 0 6.38-1 8.88" />
      <path d="M2 12a10 10 0 0 1 18-6" />
      <path d="M2 16h.01" />
      <path d="M21.8 16c.2-2 .131-5.354 0-6" />
      <path d="M5 19.5C5.5 18 6 15 6 12a6 6 0 0 1 .34-2" />
      <path d="M8.65 22c.21-.66.45-1.32.57-2" />
      <path d="M9 6.8a6 6 0 0 1 9 5.2v2" />
    </svg>
  );
}

function UserAvatarIcon({ className = "size-5" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <circle cx="12" cy="8" r="4" />
      <path d="M20 21a8 8 0 1 0-16 0" />
    </svg>
  );
}

function AxeaPosterThumbnail() {
  return (
    <div className="relative size-14 rounded-xl overflow-hidden flex-shrink-0 shadow-xs bg-[#101426] flex items-center justify-center">
      <svg viewBox="0 0 64 64" className="w-full h-full object-cover" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <linearGradient id="axeaGrad" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#1a1236" />
            <stop offset="50%" stopColor="#2a1d63" />
            <stop offset="100%" stopColor="#0d1f4d" />
          </linearGradient>
          <radialGradient id="stageGlow" cx="50%" cy="35%" r="55%">
            <stop offset="0%" stopColor="#9381ff" stopOpacity="0.85" />
            <stop offset="50%" stopColor="#c084fc" stopOpacity="0.35" />
            <stop offset="100%" stopColor="#1a1236" stopOpacity="0" />
          </radialGradient>
        </defs>
        <rect width="64" height="64" fill="url(#axeaGrad)" />
        <circle cx="32" cy="22" r="26" fill="url(#stageGlow)" />
        <polygon points="10,0 32,30 16,0" fill="#38bdf8" opacity="0.35" />
        <polygon points="54,0 32,30 48,0" fill="#e879f9" opacity="0.35" />
        <text x="32" y="24" textAnchor="middle" fill="#ffffff" fontSize="9" fontWeight="800" fontFamily="sans-serif" letterSpacing="0.06em">
          AXEA
        </text>
        <text x="32" y="32" textAnchor="middle" fill="#a5b4fc" fontSize="5.5" fontWeight="600" fontFamily="sans-serif">
          2025
        </text>
        <path d="M0,64 Q12,48 24,53 Q32,45 42,51 Q54,47 64,64 Z" fill="#080914" />
      </svg>
    </div>
  );
}

export default function LoginForm() {
  const { toast } = useToast();

  // State
  const [simState, setSimState] = useState<SimulationMode>("default");
  const [identifier, setIdentifier] = useState("nguyenvana.citizen@eventflow.vn");
  const [password, setPassword] = useState("••••••••••••");
  const [showPassword, setShowPassword] = useState(false);
  const [remember, setRemember] = useState(true);
  const [eventClosed, setEventClosed] = useState(false);
  const [localSubmitting, setLocalSubmitting] = useState(false);

  const isPending = simState === "pending" || localSubmitting;
  const isRateLimited = simState === "ratelimit";

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (isRateLimited) {
      return;
    }

    if (simState === "error") {
      // Keep showing error message
      return;
    }

    setLocalSubmitting(true);

    try {
      // Attempt backend login if available
      await loginUser(identifier, password);
      toast({
        variant: "success",
        title: "Đăng nhập thành công!",
        description: "Đang tự động chuyển tiếp vào phiên phòng chờ...",
      });
      setTimeout(() => {
        window.location.href = "/events";
      }, 1000);
    } catch {
      // In sandbox/demo mode, handle gracefully
      toast({
        variant: "info",
        title: "Xác thực mô phỏng thành công [Demo]",
        description: "Đang chuẩn bị phiên phòng chờ bảo mật EventFlow...",
      });
    } finally {
      setLocalSubmitting(false);
    }
  };

  const handleBiometricAuth = () => {
    toast({
      variant: "info",
      title: "Xác thực FaceID / VNeID [Demo]",
      description: "Đang kết nối xác thực danh tính điện tử qua cổng VNeID định danh...",
    });
  };

  return (
    <div className="w-full flex flex-col items-center">
      {/* =========================================================================
          SECTION A: FIXED STICKY HEADER
          ========================================================================= */}
      <header className="sticky top-0 z-30 w-full border-b border-border bg-surface/90 backdrop-blur-md shadow-xs">
        <div className="mx-auto flex h-16 w-full max-w-[490px] items-center justify-between px-4">
          {/* Back button */}
          <button
            type="button"
            onClick={() => window.history.back()}
            aria-label="Quay lại"
            className="ef-focus-ring -ml-2 rounded-full p-2 text-fg transition-colors hover:bg-surface-subtle"
          >
            <ArrowLeftIcon className="size-5" />
          </button>

          {/* Logo & Brand title */}
          <div className="flex items-center gap-2.5">
            <div className="flex size-9 items-center justify-center rounded-lg border border-primary/20 bg-surface-subtle overflow-hidden">
              <Image src="/logo.png" alt="EventFlow" width={28} height={28} className="object-contain" priority />
            </div>
            <div className="flex flex-col text-left">
              <span className="text-label-lg font-bold leading-tight text-fg">Đăng Nhập Eventflow</span>
              <span className="text-label-sm font-semibold leading-tight text-primary">EventFlow Secure</span>
            </div>
          </div>

          {/* Right avatar badge */}
          <div className="flex size-9 items-center justify-center rounded-full border border-border bg-surface-subtle text-fg-muted shadow-2xs">
            <UserAvatarIcon className="size-5" />
          </div>
        </div>
      </header>

      {/* Main Container: Mobile 390px - 490px, centered on larger screens */}
      <main className="w-full max-w-[490px] px-4 py-4 flex flex-col gap-4">
        {/* =========================================================================
            SECTION B: SIMULATION STATE CONTROLLER
            ========================================================================= */}
        <section
          aria-label="Bộ điều khiển trạng thái mô phỏng"
          className="rounded-card border border-border/70 bg-[#f0f4ff] p-3.5 shadow-2xs flex flex-col gap-3"
        >
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <SlidersIcon className="size-4 text-primary" />
              <span className="text-label-md font-semibold text-fg">Mô phỏng trạng thái xác thực</span>
            </div>
            <span className="rounded-full border border-border/40 bg-surface/90 px-2.5 py-0.5 text-label-sm font-medium text-fg-muted shadow-2xs">
              [Dữ liệu mô phỏng demo]
            </span>
          </div>

          <div className="grid grid-cols-4 gap-2">
            <button
              type="button"
              onClick={() => setSimState("default")}
              aria-pressed={simState === "default"}
              className={`ef-focus-ring flex h-9 items-center justify-center rounded-control text-label-md font-semibold transition-all ${
                simState === "default"
                  ? "bg-primary text-primary-fg shadow-xs"
                  : "border border-border bg-surface text-fg hover:bg-surface-subtle"
              }`}
            >
              Chuẩn
            </button>
            <button
              type="button"
              onClick={() => setSimState("pending")}
              aria-pressed={simState === "pending"}
              className={`ef-focus-ring flex h-9 items-center justify-center rounded-control text-label-md font-semibold transition-all ${
                simState === "pending"
                  ? "bg-primary text-primary-fg shadow-xs"
                  : "border border-border bg-surface text-fg hover:bg-surface-subtle"
              }`}
            >
              Đang gửi
            </button>
            <button
              type="button"
              onClick={() => setSimState("error")}
              aria-pressed={simState === "error"}
              className={`ef-focus-ring flex h-9 items-center justify-center rounded-control text-label-md font-semibold transition-all ${
                simState === "error"
                  ? "bg-danger text-on-semantic shadow-xs"
                  : "border border-border bg-surface text-fg hover:bg-surface-subtle"
              }`}
            >
              Lỗi sai
            </button>
            <button
              type="button"
              onClick={() => setSimState("ratelimit")}
              aria-pressed={simState === "ratelimit"}
              className={`ef-focus-ring flex h-9 items-center justify-center rounded-control text-label-md font-semibold transition-all ${
                simState === "ratelimit"
                  ? "bg-[#c47700] text-on-semantic shadow-xs"
                  : "border border-border bg-surface text-fg hover:bg-surface-subtle"
              }`}
            >
              Khóa 429
            </button>
          </div>
        </section>

        {/* =========================================================================
            SECTION C: EVENT CARD (AXEA Festival 2025)
            ========================================================================= */}
        {!eventClosed ? (
          <section
            aria-label="Thông tin sự kiện đang giữ chỗ"
            className="rounded-card border border-border bg-surface p-4 shadow-card flex flex-col"
          >
            <div className="flex items-start justify-between gap-3">
              <div className="flex items-center gap-3">
                <AxeaPosterThumbnail />
                <div className="flex flex-col">
                  <div className="flex items-center gap-2">
                    <Badge variant="queued" className="bg-info-soft normal-case text-primary font-semibold text-[11px] px-2 py-0.5">
                      Đang mua vé
                    </Badge>
                    <span className="text-body-sm text-fg-muted font-normal">[Demo]</span>
                  </div>
                  <h3 className="text-label-lg font-bold text-fg mt-0.5 leading-snug">AXEA Festival 2025</h3>
                  <div className="flex items-center gap-1.5 text-body-sm text-fg-muted mt-0.5">
                    <CalendarIcon className="size-3.5 text-fg-muted flex-shrink-0" />
                    <span>15/11/2025 • SVĐ Quân Khu 7</span>
                  </div>
                </div>
              </div>

              {/* Close button */}
              <button
                type="button"
                onClick={() => setEventClosed(true)}
                aria-label="Đóng thông tin sự kiện"
                className="ef-focus-ring flex size-7 items-center justify-center rounded-full bg-surface-subtle text-fg-muted transition-colors hover:bg-neutral-soft"
              >
                ✕
              </button>
            </div>

            {/* Bottom transition bar */}
            <div className="mt-3.5 flex items-center justify-between rounded-control bg-[#f0f4ff] px-3 py-2 text-label-sm font-medium text-fg">
              <div className="flex items-center gap-2">
                <span className="size-2 rounded-full bg-primary flex-shrink-0" />
                <span>Tự động chuyển tiếp vào phiên phòng chờ sau khi xác thực</span>
              </div>
              <ArrowRightIcon className="size-4 text-primary flex-shrink-0" />
            </div>
          </section>
        ) : null}

        {/* =========================================================================
            SECTION D: IDENTITY & FAIR TICKETING BANNER
            ========================================================================= */}
        <section
          aria-label="Quy định định danh vé công bằng"
          className="rounded-card border border-border/50 bg-[#f0f4ff] p-4 shadow-2xs flex flex-col"
        >
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <ShieldCheckIcon className="size-5 text-primary flex-shrink-0" />
              <h4 className="text-label-lg font-bold text-fg">Quy định định danh vé công bằng</h4>
            </div>
            <span className="rounded-full border border-border/40 bg-surface px-2.5 py-0.5 text-label-sm font-semibold text-primary shadow-2xs">
              An toàn
            </span>
          </div>
          <p className="mt-2.5 text-body-sm leading-relaxed text-fg-muted">
            Xác thực danh tính là bắt buộc trước khi tham gia phòng chờ hoặc nhận hạn ngạch giữ vé nhằm loại trừ hành vi
            bot đầu cơ và phân phối công bằng (tối đa 4 vé/CCCD định danh). EventFlow không chia sẻ dữ liệu nhạy cảm.
          </p>
        </section>

        {/* =========================================================================
            SECTION E: ERROR ALERT (AUTH_401)
            ========================================================================= */}
        {simState === "error" ? (
          <Alert variant="error" title="Thông tin đăng nhập không hợp lệ">
            <p className="mt-1 text-body-sm leading-relaxed">
              Email hoặc mật khẩu không chính xác. Vui lòng kiểm tra lại thông tin hoặc sử dụng liên kết &quot;Quên mật
              khẩu&quot;. <span className="font-mono text-xs opacity-90">[Demo Code: AUTH_401]</span>
            </p>
          </Alert>
        ) : null}

        {/* =========================================================================
            SECTION F: HTTP 429 RATE LIMIT ALERT
            ========================================================================= */}
        {simState === "ratelimit" ? (
          <Alert variant="warning" title="Giới hạn tần suất đăng nhập (HTTP 429)">
            <p className="mt-1 text-body-sm leading-relaxed">
              Bạn đã thao tác đăng nhập quá nhiều lần. Hệ thống tạm khóa thao tác nhằm phòng ngừa tự động hóa. Vui lòng
              thử lại sau: <span className="font-mono font-bold text-warning-fg tabular-nums">14:52</span>.
            </p>
          </Alert>
        ) : null}

        {/* =========================================================================
            SECTION G: MAIN LOGIN FORM CARD
            ========================================================================= */}
        <section
          aria-label="Đăng nhập tài khoản"
          className="rounded-card border border-border bg-surface p-5 sm:p-6 shadow-card flex flex-col gap-5"
        >
          <div>
            <h1 className="text-headline-md font-bold text-fg sm:text-headline-lg">Đăng nhập tài khoản</h1>
            <p className="mt-1 text-body-sm text-fg-muted">
              Nhập thông tin tài khoản đã kích hoạt căn cước định danh
            </p>
          </div>

          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            {/* Field 1: Email hoặc Số điện thoại liên kết */}
            <div className="flex flex-col gap-1.5">
              <div className="flex items-center justify-between">
                <label htmlFor="login-identifier" className="text-label-lg font-semibold text-fg">
                  Email hoặc Số điện thoại liên kết
                </label>
                <span className="text-label-md font-semibold text-primary">Bắt buộc</span>
              </div>
              <div className="relative">
                <div className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3.5 text-fg-muted">
                  <MailIcon className="size-5" />
                </div>
                <input
                  id="login-identifier"
                  type="text"
                  required
                  disabled={isPending || isRateLimited}
                  value={identifier}
                  onChange={(e) => setIdentifier(e.target.value)}
                  placeholder="nguyenvana.citizen@eventflow.vn"
                  className="ef-focus-ring h-12 w-full rounded-control border border-border-strong bg-surface pl-11 pr-3 text-body-md text-fg placeholder:text-fg-muted transition-colors disabled:cursor-not-allowed disabled:opacity-60"
                />
              </div>
              <div className="mt-0.5 flex items-center gap-1.5 text-body-sm text-fg-muted">
                <IdCardIcon className="size-4 flex-shrink-0 text-primary" />
                <span>Sử dụng tài khoản đã định danh số định danh cá nhân / CCCD</span>
              </div>
            </div>

            {/* Field 2: Mật khẩu */}
            <div className="flex flex-col gap-1.5">
              <div className="flex items-center justify-between">
                <label htmlFor="login-password" className="text-label-lg font-semibold text-fg">
                  Mật khẩu
                </label>
                <Link href="#" className="ef-focus-ring rounded-xs text-label-md font-semibold text-primary hover:underline">
                  Quên mật khẩu?
                </Link>
              </div>
              <div className="relative">
                <div className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3.5 text-fg-muted">
                  <LockIcon className="size-5" />
                </div>
                <input
                  id="login-password"
                  type={showPassword ? "text" : "password"}
                  required
                  disabled={isPending || isRateLimited}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="••••••••••••"
                  className="ef-focus-ring h-12 w-full rounded-control border border-border-strong bg-surface pl-11 pr-11 text-body-md text-fg placeholder:text-fg-muted transition-colors disabled:cursor-not-allowed disabled:opacity-60"
                />
                <button
                  type="button"
                  onClick={() => setShowPassword(!showPassword)}
                  aria-label={showPassword ? "Ẩn mật khẩu" : "Hiện mật khẩu"}
                  aria-pressed={showPassword}
                  className="ef-focus-ring absolute inset-y-0 right-0 flex items-center pr-3.5 text-fg-muted transition-colors hover:text-fg"
                >
                  {showPassword ? <EyeOffIcon className="size-5" /> : <EyeIcon className="size-5" />}
                </button>
              </div>
            </div>

            {/* Checkbox: Ghi nhớ phiên làm việc trên thiết bị này */}
            <div className="flex items-center justify-between pt-1">
              <label className="flex cursor-pointer select-none items-center gap-2.5">
                <input
                  type="checkbox"
                  checked={remember}
                  disabled={isPending || isRateLimited}
                  onChange={(e) => setRemember(e.target.checked)}
                  className="ef-focus-ring size-4.5 rounded-sm border-border-strong text-primary accent-primary"
                />
                <span className="text-body-sm font-medium text-fg">
                  Ghi nhớ phiên làm việc trên thiết bị này
                </span>
              </label>
              <div className="flex items-center text-primary" title="Bảo mật thiết bị">
                <DeviceShieldIcon className="size-5" />
              </div>
            </div>

            {/* Primary Action Button */}
            <Button
              type="submit"
              variant="primary"
              loading={isPending}
              disabled={isRateLimited}
              className="mt-2 h-13 w-full text-body-md font-semibold shadow-sm"
            >
              {isPending ? (
                <span>Đang xác thực bảo mật...</span>
              ) : (
                <span className="inline-flex items-center justify-center gap-2">
                  <LoginIcon className="size-5" />
                  <span>Đăng nhập và Tiếp tục</span>
                </span>
              )}
            </Button>

            {/* Secondary Action: FaceID / VNeID */}
            <button
              type="button"
              onClick={handleBiometricAuth}
              disabled={isPending || isRateLimited}
              className="ef-focus-ring flex h-12 w-full items-center justify-center gap-2 rounded-control border border-transparent bg-[#eef4ff] text-body-md font-semibold text-fg transition-colors hover:bg-[#e4efff] disabled:cursor-not-allowed disabled:opacity-60 shadow-2xs"
            >
              <FingerprintIcon className="size-5 text-primary" />
              <span>Xác thực nhanh qua FaceID / VNeID định danh</span>
            </button>
          </form>
        </section>

        {/* =========================================================================
            SECTION H: REGISTER CARD
            ========================================================================= */}
        <section
          aria-label="Đăng ký tài khoản mới"
          className="rounded-card border border-border bg-surface p-4 shadow-card flex items-center justify-between gap-4"
        >
          <div className="flex flex-col">
            <h2 className="text-label-lg font-bold text-fg">Chưa có tài khoản EventFlow?</h2>
            <p className="mt-0.5 text-body-sm text-fg-muted">Đăng ký mới để xác thực số định danh</p>
          </div>
          <Link href="/auth/sign-up" className="ef-focus-ring rounded-control flex-shrink-0">
            <span className="inline-flex items-center rounded-control bg-[#eef4ff] px-4 py-2.5 text-body-sm font-semibold text-primary transition-colors hover:bg-[#e4efff] shadow-2xs">
              Đăng ký mới
            </span>
          </Link>
        </section>

        {/* =========================================================================
            SECTION I: SECURITY FOOTER
            ========================================================================= */}
        <footer className="flex flex-col items-center gap-2.5 py-4 text-center text-fg-muted">
          <div className="inline-flex items-center gap-1.5 rounded-full border border-border/50 bg-[#eef4ff] px-3 py-1 text-label-sm font-semibold text-fg shadow-2xs">
            <ShieldCheckIcon className="size-4 text-primary" />
            <span>Bảo vệ bởi EventFlow Zero-Trust Identity Guard</span>
          </div>
          <p className="text-body-sm text-fg-muted">
            Mã hóa đường truyền TLS 1.3 • Chống đăng nhập hàng loạt tự động
          </p>
          <p className="font-mono text-[11px] text-fg-muted/80">
            [Dữ liệu mô phỏng demo - Môi trường Sandbox]
          </p>
        </footer>
      </main>
    </div>
  );
}
