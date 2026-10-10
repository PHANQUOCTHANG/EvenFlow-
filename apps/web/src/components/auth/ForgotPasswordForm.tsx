"use client";

import { useState, useRef, useEffect } from "react";
import Link from "next/link";
import Image from "next/image";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import * as z from "zod";
import { requestResetPasswordOTP, resetPassword } from "@/lib/api/auth";
import { Button, useToast } from "@/components/ui";

// --- Validation Schema ---
const requestSchema = z.object({
  identifier: z.string().min(1, "Vui lòng nhập Email hoặc Số điện thoại").refine((val) => {
    const isEmail = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(val);
    const isPhone = /^(84|0[3|5|7|8|9])+([0-9]{8})\b$/.test(val);
    return isEmail || isPhone;
  }, "Định dạng Email hoặc Số điện thoại (VN) không hợp lệ"),
});

type RequestFormValues = z.infer<typeof requestSchema>;

const resetSchema = z.object({
  password: z.string().min(8, "Mật khẩu phải có tối thiểu 8 ký tự"),
  confirmPassword: z.string().min(1, "Vui lòng xác nhận mật khẩu"),
}).refine((data) => data.password === data.confirmPassword, {
  message: "Mật khẩu xác nhận không khớp",
  path: ["confirmPassword"]
});

type ResetFormValues = z.infer<typeof resetSchema>;

// --- Icons ---
function ContactIcon({ className = "size-5" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round">
      <path d="M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2" />
      <circle cx="12" cy="7" r="4" />
    </svg>
  );
}

function LockIcon({ className = "size-5" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round">
      <rect width="18" height="11" x="3" y="11" rx="2" ry="2" />
      <path d="M7 11V7a5 5 0 0 1 10 0v4" />
    </svg>
  );
}

function EyeIcon({ className = "size-5" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round">
      <path d="M2.062 12.348a1 1 0 0 1 0-.696 10.75 10.75 0 0 1 19.876 0 1 1 0 .696 10.75 10.75 0 0 1-19.876 0" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  );
}

function EyeOffIcon({ className = "size-5" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round">
      <path d="m2 2 20 20" />
      <path d="M9.88 9.88a3 3 0 1 0 4.24 4.24" />
      <path d="M10.73 5.08A10.43 10.43 0 0 1 12 5c7 0 10 7 10 7a13.16 13.16 0 0 1-1.67 2.68" />
      <path d="M6.61 6.61A13.526 13.526 0 0 0 2 12s3 7 10 7a9.74 9.74 0 0 0 5.39-1.61" />
    </svg>
  );
}

function CheckCircleIcon({ className = "size-6" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="10" />
      <path d="m9 12 2 2 4-4" />
    </svg>
  );
}

function TelegramIcon({ className = "size-5" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="currentColor">
      <path d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.892-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z"/>
    </svg>
  );
}

export default function ForgotPasswordForm() {
  const { toast } = useToast();
  
  const [step, setStep] = useState<"REQUEST" | "RESET" | "SUCCESS">("REQUEST");
  const [identifier, setIdentifier] = useState("");
  const isPhoneNumber = /^(84|0[3|5|7|8|9])+([0-9]{8})\b$/.test(identifier);

  // -- Step 1 State --
  const { register: registerReq, handleSubmit: handleSubmitReq, formState: { errors: errorsReq, isSubmitting: isSubmittingReq } } = useForm<RequestFormValues>({
    resolver: zodResolver(requestSchema),
    defaultValues: { identifier: "" }
  });

  // -- Step 2 State --
  const [otp, setOtp] = useState<string[]>(Array(6).fill(""));
  const inputRefs = useRef<(HTMLInputElement | null)[]>([]);
  const [showPassword, setShowPassword] = useState(false);
  const [showConfirm, setShowConfirm] = useState(false);

  const { register: registerRes, handleSubmit: handleSubmitRes, formState: { errors: errorsRes, isSubmitting: isSubmittingRes } } = useForm<ResetFormValues>({
    resolver: zodResolver(resetSchema),
    defaultValues: { password: "", confirmPassword: "" }
  });

  const onSubmitRequest = async (data: RequestFormValues) => {
    try {
      await requestResetPasswordOTP(data.identifier);
      setIdentifier(data.identifier);
      toast({
        variant: "default",
        title: "Đã gửi mã xác thực",
        description: isPhoneNumber 
          ? `Vui lòng mở Telegram để nhận mã OTP.` 
          : `Mã OTP đã được gửi tới ${data.identifier}`,
      });
      setStep("RESET");
    } catch (err: any) {
      toast({
        variant: "error",
        title: "Lỗi yêu cầu",
        description: err?.detail || "Hệ thống đang bận. Vui lòng thử lại sau.",
      });
    }
  };

  const handleOTPChange = (index: number, value: string) => {
    const cleaned = value.replace(/[^0-9]/g, "");
    if (!cleaned) {
      setOtp(prev => {
        const newOtp = [...prev];
        newOtp[index] = "";
        return newOtp;
      });
      return;
    }

    if (cleaned.length > 1) {
      setOtp(prev => {
        const newOtp = [...prev];
        let currIdx = index;
        for (let i = 0; i < cleaned.length && currIdx < 6; i++) {
          newOtp[currIdx] = cleaned[i];
          currIdx++;
        }
        setTimeout(() => {
          const focusIndex = Math.min(index + cleaned.length, 5);
          inputRefs.current[focusIndex]?.focus();
        }, 10);
        return newOtp;
      });
      return;
    }
    
    setOtp(prev => {
      const newOtp = [...prev];
      newOtp[index] = cleaned;
      return newOtp;
    });

    if (index < 5 && inputRefs.current[index + 1]) {
      setTimeout(() => {
        inputRefs.current[index + 1]?.focus();
      }, 10);
    }
  };

  const handleOTPKeyDown = (index: number, e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Backspace" && !otp[index] && index > 0) {
      inputRefs.current[index - 1]?.focus();
    }
  };

  const handleOTPPaste = (e: React.ClipboardEvent<HTMLInputElement>) => {
    e.preventDefault();
    const pastedData = e.clipboardData.getData("text/plain").slice(0, 6).replace(/[^0-9]/g, "");
    if (!pastedData) return;
    
    const newOtp = [...otp];
    for (let i = 0; i < pastedData.length; i++) {
      newOtp[i] = pastedData[i];
    }
    setOtp(newOtp);
    const focusIndex = Math.min(pastedData.length, 5);
    inputRefs.current[focusIndex]?.focus();
  };

  const onSubmitReset = async (data: ResetFormValues) => {
    const otpValue = otp.join("");
    if (otpValue.length < 6) {
      toast({
        variant: "warning",
        title: "Thiếu mã OTP",
        description: "Vui lòng nhập đủ 6 số OTP"
      });
      return;
    }

    try {
      await resetPassword(identifier, otpValue, data.password);
      setStep("SUCCESS");
    } catch (err: any) {
      toast({
        variant: "error",
        title: "Đổi mật khẩu thất bại",
        description: err?.detail || "OTP không hợp lệ hoặc đã hết hạn",
      });
    }
  };

  if (step === "SUCCESS") {
    return (
      <div className="flex flex-col gap-4 w-full">
        <div className="bg-white rounded-[24px] p-6 sm:p-8 shadow-xl shadow-black/5 border border-border-subtle flex flex-col gap-6 relative overflow-hidden text-center">
           <div className="absolute top-0 left-0 right-0 h-1 bg-gradient-to-r from-emerald-400 via-green-500 to-emerald-400 opacity-30" />
           <div className="flex size-16 mx-auto items-center justify-center rounded-full bg-emerald-50">
             <CheckCircleIcon className="size-8 text-emerald-500" />
           </div>
           <div>
             <h1 className="text-2xl font-extrabold text-fg tracking-tight">Thành công!</h1>
             <p className="mt-2 text-[14px] leading-relaxed text-fg-muted">
               Mật khẩu của bạn đã được thay đổi. Vui lòng đăng nhập lại bằng mật khẩu mới.
             </p>
           </div>
           <Link href="/auth/sign-in" className="w-full mt-2">
             <Button className="h-[48px] w-full rounded-[12px] bg-indigo-600 hover:bg-indigo-700 text-white text-[15px] font-bold shadow-md shadow-indigo-600/20 hover:shadow-indigo-600/30 transition-all active:scale-[0.98]">
               Về trang Đăng nhập
             </Button>
           </Link>
        </div>
      </div>
    );
  }

  if (step === "RESET") {
    return (
      <div className="flex flex-col w-full">
        <div className="bg-white rounded-[24px] p-6 sm:p-8 shadow-xl shadow-black/5 border border-border-subtle flex flex-col gap-6 relative overflow-hidden">
          <div className="absolute top-0 left-0 right-0 h-1 bg-[#0088cc]" />
          
          <div className="flex flex-col items-center text-center">
            {isPhoneNumber ? (
              <div className="flex flex-col items-center gap-3 w-full mb-4">
                <div className="flex size-16 mx-auto items-center justify-center rounded-full bg-[#0088cc]/10 animate-pulse">
                  <TelegramIcon className="size-8 text-[#0088cc]" />
                </div>
                <h1 className="text-xl font-extrabold text-fg tracking-tight">Tạo mật khẩu mới</h1>
                <p className="text-[13.5px] leading-relaxed text-fg-muted px-4 mb-2">
                  Hệ thống đã chuẩn bị mã OTP cho số <b>{identifier}</b>.
                </p>
                <div className="p-3 bg-white border border-border-subtle rounded-xl shadow-sm">
                  <img 
                    src="https://api.qrserver.com/v1/create-qr-code/?size=150x150&data=https://t.me/EventFlowAuth_bot" 
                    alt="Telegram Bot QR Code" 
                    className="w-[120px] h-[120px]"
                  />
                </div>
                
                <div className="text-left text-[13px] text-fg-muted space-y-1.5 w-full bg-slate-50 p-3 rounded-lg border border-border-subtle">
                  <p className="font-bold text-fg mb-1">Hướng dẫn nhận mã:</p>
                  <p>1. Dùng điện thoại quét mã QR ở trên (Hoặc tìm <span className="font-semibold text-[#0088cc]">@EventFlowAuth_bot</span>).</p>
                  <p>2. Nhấn nút <b>Start</b> (Hoặc gõ <code className="bg-slate-200 px-1 py-0.5 rounded text-[#0088cc]">/start</code> nếu đã từng chat với Bot).</p>
                  <p>3. Bấm nút <b>Chia sẻ Số điện thoại</b> ở bàn phím chat.</p>
                </div>

                <a 
                  href={`https://t.me/EventFlowAuth_bot`} 
                  target="_blank"
                  rel="noopener noreferrer"
                  className="w-full mt-1"
                >
                  <Button type="button" className="h-[44px] w-full rounded-[10px] bg-[#0088cc] hover:bg-[#0077b3] text-white text-[14px] font-bold shadow-sm transition-all active:scale-[0.98]">
                    <span className="inline-flex items-center justify-center gap-2">
                      <TelegramIcon className="size-4" />
                      <span>Mở nhanh Telegram (Nếu có App)</span>
                    </span>
                  </Button>
                </a>
              </div>
            ) : (
              <div className="flex flex-col items-center mb-4">
                <div className="flex size-16 mx-auto items-center justify-center rounded-full bg-indigo-50 mb-4">
                  <LockIcon className="size-8 text-indigo-600" />
                </div>
                <h1 className="text-xl font-extrabold text-fg tracking-tight mb-2">Tạo mật khẩu mới</h1>
                <p className="text-[13.5px] leading-relaxed text-fg-muted px-4">
                  Nhập mã xác thực OTP đã được gửi tới <b>{identifier}</b> và mật khẩu mới của bạn.
                </p>
              </div>
            )}
          </div>

          <form onSubmit={handleSubmitRes(onSubmitReset)} className="flex flex-col gap-5 w-full">
            {/* Box OTP */}
            <div className="w-full bg-slate-50 border border-border-subtle rounded-xl p-4 flex flex-col gap-3">
              <label className="text-[13px] font-bold text-fg text-center">Nhập mã OTP (6 số)</label>
              <div className="flex justify-between gap-1 sm:gap-2 max-w-[320px] mx-auto w-full">
                {otp.map((digit, index) => (
                  <input
                    key={index}
                    ref={(el) => { inputRefs.current[index] = el; }}
                    type="text"
                    inputMode="numeric"
                    pattern="[0-9]*"
                    value={digit}
                    onChange={(e) => handleOTPChange(index, e.target.value)}
                    onKeyDown={(e) => handleOTPKeyDown(index, e)}
                    onPaste={handleOTPPaste}
                    disabled={isSubmittingRes}
                    className="w-full sm:w-11 h-12 text-center text-lg font-bold border border-border-strong rounded-lg focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600 outline-none transition-all"
                  />
                ))}
              </div>
            </div>

            {/* Field: New Password */}
            <div className="flex flex-col gap-1.5 mt-2">
              <label htmlFor="reset-password" className="text-[13.5px] font-bold text-fg">Mật khẩu mới</label>
              <div className="relative">
                <div className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-4 text-fg-muted/80">
                  <LockIcon className="size-[18px]" />
                </div>
                <input
                  id="reset-password"
                  type={showPassword ? "text" : "password"}
                  suppressHydrationWarning
                  disabled={isSubmittingRes}
                  {...registerRes("password")}
                  placeholder="Tối thiểu 8 ký tự"
                  className={`ef-focus-ring h-[48px] w-full rounded-[12px] border bg-white pl-11 pr-12 text-[14px] text-fg transition-colors font-medium ${errorsRes.password ? 'border-red-500' : 'border-border-strong focus:border-indigo-500'}`}
                />
                <button
                  type="button"
                  suppressHydrationWarning
                  onClick={() => setShowPassword(!showPassword)}
                  className="absolute inset-y-0 right-0 flex items-center pr-4 text-fg-muted"
                >
                  {showPassword ? <EyeOffIcon className="size-[18px]" /> : <EyeIcon className="size-[18px]" />}
                </button>
              </div>
              {errorsRes.password && <span className="text-red-500 text-[12.5px]">{errorsRes.password.message}</span>}
            </div>

            {/* Field: Confirm Password */}
            <div className="flex flex-col gap-1.5">
              <label htmlFor="reset-confirm" className="text-[13.5px] font-bold text-fg">Xác nhận mật khẩu</label>
              <div className="relative">
                <div className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-4 text-fg-muted/80">
                  <LockIcon className="size-[18px]" />
                </div>
                <input
                  id="reset-confirm"
                  type={showConfirm ? "text" : "password"}
                  suppressHydrationWarning
                  disabled={isSubmittingRes}
                  {...registerRes("confirmPassword")}
                  placeholder="Nhập lại mật khẩu"
                  className={`ef-focus-ring h-[48px] w-full rounded-[12px] border bg-white pl-11 pr-12 text-[14px] text-fg transition-colors font-medium ${errorsRes.confirmPassword ? 'border-red-500' : 'border-border-strong focus:border-indigo-500'}`}
                />
                <button
                  type="button"
                  suppressHydrationWarning
                  onClick={() => setShowConfirm(!showConfirm)}
                  className="absolute inset-y-0 right-0 flex items-center pr-4 text-fg-muted"
                >
                  {showConfirm ? <EyeOffIcon className="size-[18px]" /> : <EyeIcon className="size-[18px]" />}
                </button>
              </div>
              {errorsRes.confirmPassword && <span className="text-red-500 text-[12.5px]">{errorsRes.confirmPassword.message}</span>}
            </div>

            <Button
              type="submit"
              loading={isSubmittingRes}
              className="mt-2 h-[48px] w-full rounded-[12px] bg-[#0088cc] hover:bg-[#0077b3] text-white font-bold"
            >
              Cập nhật mật khẩu
            </Button>
          </form>

          <button 
            type="button"
            onClick={() => setStep("REQUEST")}
            disabled={isSubmittingRes}
            className="text-fg-muted hover:text-indigo-600 font-medium text-[13.5px]"
          >
            Quay lại
          </button>
        </div>
      </div>
    );
  }

  // Step 1: Request
  return (
    <div className="flex flex-col w-full">
      <div className="bg-white rounded-[24px] p-6 sm:p-7 shadow-xl shadow-black/5 border border-border-subtle flex flex-col gap-5 relative overflow-hidden">
        <div className="absolute top-0 left-0 right-0 h-1 bg-gradient-to-r from-indigo-500 via-blue-500 to-indigo-500 opacity-20" />

        <div className="flex items-center justify-between mb-2 w-full">
          <Image src="/logo.png" alt="EventFlow Logo" width={100} height={28} className="h-7 w-auto" priority />
          <h1 className="text-xl font-extrabold text-fg tracking-tight px-2">Quên mật khẩu</h1>
          <div className="w-[100px]" />
        </div>

        <p className="text-[13.5px] text-fg-muted text-center px-4 leading-relaxed">
          Nhập Email hoặc Số điện thoại của bạn, chúng tôi sẽ gửi mã OTP để bạn tạo mật khẩu mới.
        </p>

        <form onSubmit={handleSubmitReq(onSubmitRequest)} className="flex flex-col gap-4 mt-2">
          <div className="flex flex-col gap-1.5">
            <label htmlFor="req-identifier" className="text-[13.5px] font-bold text-fg">Email hoặc Số điện thoại</label>
            <div className="relative">
              <div className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-4 text-fg-muted/80">
                <ContactIcon className="size-[18px]" />
              </div>
              <input
                id="req-identifier"
                type="text"
                disabled={isSubmittingReq}
                {...registerReq("identifier")}
                placeholder="Nhập email hoặc SĐT"
                className={`ef-focus-ring h-[48px] w-full rounded-[12px] border bg-white pl-11 pr-4 text-[14px] text-fg transition-colors ${errorsReq.identifier ? 'border-red-500 focus:border-red-500' : 'border-border-strong focus:border-indigo-500'}`}
              />
            </div>
            {errorsReq.identifier && <span className="text-red-500 text-[12.5px] font-medium">{errorsReq.identifier.message}</span>}
          </div>

          <Button
            type="submit"
            loading={isSubmittingReq}
            className="mt-2 h-[48px] w-full rounded-[12px] bg-indigo-600 hover:bg-indigo-700 text-white font-bold"
          >
            {isSubmittingReq ? "Đang xử lý..." : "Gửi mã OTP"}
          </Button>
        </form>

        <div className="mt-2 text-center text-[13.5px] text-fg-muted">
          Nhớ ra mật khẩu?{" "}
          <Link href="/auth/sign-in" className="font-bold text-indigo-600 hover:text-indigo-700 hover:underline">
            Đăng nhập
          </Link>
        </div>
      </div>
    </div>
  );
}
