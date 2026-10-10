"use client";

import { useState, useRef, useEffect } from "react";
import Link from "next/link";
import Image from "next/image";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import * as z from "zod";
import { requestRegistrationOTP, verifyRegistrationOTP } from "@/lib/api/auth";
import { Alert, Button, useToast } from "@/components/ui";
import OTPVerification from "./OTPVerification";

// --- Validation Schema ---
const registerSchema = z.object({
  identifier: z.string().min(1, "Vui lòng nhập Email hoặc Số điện thoại").refine((val) => {
    const isEmail = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(val);
    const isPhone = /^(84|0[3|5|7|8|9])+([0-9]{8})\b$/.test(val);
    return isEmail || isPhone;
  }, "Định dạng Email hoặc Số điện thoại (VN) không hợp lệ"),
  password: z.string().min(8, "Mật khẩu phải có tối thiểu 8 ký tự"),
  confirmPassword: z.string().min(1, "Vui lòng xác nhận mật khẩu"),
  agreedTerms: z.boolean().refine((val) => val === true, "Bạn phải đồng ý với điều khoản sử dụng")
}).refine((data) => data.password === data.confirmPassword, {
  message: "Mật khẩu xác nhận không khớp",
  path: ["confirmPassword"]
});

type RegisterFormValues = z.infer<typeof registerSchema>;

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

function UserPlusIcon({ className = "size-5" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
      <circle cx="9" cy="7" r="4" />
      <line x1="19" x2="19" y1="8" y2="14" />
      <line x1="22" x2="16" y1="11" y2="11" />
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

function TelegramOTPVerification({ 
  identifier, 
  onVerify, 
  onCancel 
}: { 
  identifier: string; 
  onVerify: (otp: string) => Promise<void>; 
  onCancel: () => void; 
}) {
  const { toast } = useToast();
  const [otp, setOtp] = useState<string[]>(Array(6).fill(""));
  const [loading, setLoading] = useState(false);
  const inputRefs = useRef<(HTMLInputElement | null)[]>([]);

  useEffect(() => {
    if (inputRefs.current[0]) {
      inputRefs.current[0].focus();
    }
  }, []);

  const handleChange = (index: number, value: string) => {
    const cleaned = value.replace(/[^0-9]/g, "");
    
    if (!cleaned) {
      setOtp(prev => {
        const newOtp = [...prev];
        newOtp[index] = "";
        return newOtp;
      });
      return;
    }

    // Handle multiple digits (fast typing or copy-paste without trigger paste event)
    if (cleaned.length > 1) {
      setOtp(prev => {
        const newOtp = [...prev];
        let currIdx = index;
        for (let i = 0; i < cleaned.length && currIdx < 6; i++) {
          newOtp[currIdx] = cleaned[i];
          currIdx++;
        }
        
        // Focus next box
        setTimeout(() => {
          const focusIndex = Math.min(index + cleaned.length, 5);
          inputRefs.current[focusIndex]?.focus();
        }, 10);
        
        return newOtp;
      });
      return;
    }
    
    // Normal single character
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

  const handleKeyDown = (index: number, e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Backspace" && !otp[index] && index > 0) {
      inputRefs.current[index - 1]?.focus();
    } else if (e.key === "Enter") {
      handleSubmit();
    }
  };

  const handlePaste = (e: React.ClipboardEvent<HTMLInputElement>) => {
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

  const handleSubmit = async () => {
    const otpValue = otp.join("");
    if (otpValue.length < 6) {
      toast({
        variant: "warning",
        title: "Thiếu thông tin",
        description: "Vui lòng nhập đủ 6 số OTP"
      });
      return;
    }
    setLoading(true);
    try {
      await onVerify(otpValue);
    } catch (err: any) {
      toast({
        variant: "error",
        title: "Lỗi xác thực",
        description: err.message || "Xác thực thất bại"
      });
      setOtp(Array(6).fill(""));
      inputRefs.current[0]?.focus();
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="flex flex-col w-full">
      <div className="bg-white rounded-[24px] p-6 sm:p-8 shadow-xl shadow-black/5 border border-border-subtle flex flex-col gap-6 relative overflow-hidden items-center text-center">
        <div className="absolute top-0 left-0 right-0 h-1 bg-[#0088cc]" />
        
        <div className="flex size-16 mx-auto items-center justify-center rounded-full bg-[#0088cc]/10 animate-pulse">
          <TelegramIcon className="size-8 text-[#0088cc]" />
        </div>
        
        <div>
          <h1 className="text-xl font-extrabold text-fg tracking-tight mb-2">Xác thực qua Telegram</h1>
          <p className="text-[13.5px] leading-relaxed text-fg-muted px-4">
            Hệ thống đã chuẩn bị mã xác thực cho số <span className="font-bold text-fg">{identifier}</span>. Vui lòng bấm vào nút dưới đây để mở Telegram và nhận mã.
          </p>
        </div>

        <div className="flex flex-col items-center gap-3 my-2">
          {/* QR Code */}
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
        </div>

        <div className="flex items-center gap-3 w-full">
          <div className="h-px bg-border-subtle flex-1"></div>
          <span className="text-[12px] text-fg-muted/60 font-semibold uppercase tracking-wider">HOẶC</span>
          <div className="h-px bg-border-subtle flex-1"></div>
        </div>

        <a 
          href={`https://t.me/EventFlowAuth_bot`} 
          target="_blank"
          rel="noopener noreferrer"
          className="w-full"
        >
          <Button className="h-[44px] w-full rounded-[10px] bg-[#0088cc] hover:bg-[#0077b3] text-white text-[14px] font-bold shadow-sm transition-all active:scale-[0.98]">
            <span className="inline-flex items-center justify-center gap-2">
              <TelegramIcon className="size-4" />
              <span>Mở nhanh Telegram (Nếu có App)</span>
            </span>
          </Button>
        </a>

        <div className="w-full mt-2">
          <label className="text-[13px] font-bold text-fg mb-3 block text-left">Nhập 6 số OTP từ Bot</label>
          <div className="flex justify-between gap-2 max-w-[300px] mx-auto">
            {otp.map((digit, index) => (
              <input
                key={index}
                ref={(el) => { inputRefs.current[index] = el; }}
                type="text"
                inputMode="numeric"
                pattern="[0-9]*"
                value={digit}
                onChange={(e) => handleChange(index, e.target.value)}
                onKeyDown={(e) => handleKeyDown(index, e)}
                onPaste={handlePaste}
                disabled={loading}
                className="w-10 h-12 text-center text-lg font-bold border-2 border-border-strong rounded-lg focus:border-[#0088cc] focus:ring-1 focus:ring-[#0088cc] outline-none transition-all"
              />
            ))}
          </div>
          
          <Button
            type="button"
            loading={loading}
            disabled={otp.join("").length < 6}
            className="mt-6 h-[48px] w-full rounded-[12px] bg-indigo-600 hover:bg-indigo-700 text-white font-bold"
            onClick={handleSubmit}
          >
            Xác nhận
          </Button>
        </div>

        <div className="mt-2 flex flex-col gap-2 w-full text-[13.5px]">
          <button 
            type="button"
            onClick={onCancel}
            disabled={loading}
            className="text-fg-muted hover:text-indigo-600 font-medium transition-colors"
          >
            Bạn không sử dụng Telegram? Quay lại
          </button>
        </div>
      </div>
    </div>
  );
}export default function RegisterForm() {
  const { toast } = useToast();
  
  const [step, setStep] = useState<"REGISTER" | "OTP" | "TELEGRAM_OTP" | "SUCCESS">("REGISTER");
  const [showPassword, setShowPassword] = useState(false);
  const [showConfirm, setShowConfirm] = useState(false);
  const [registeredPassword, setRegisteredPassword] = useState(""); // Cached for OTP step
  
  const { register, handleSubmit, formState: { errors, isSubmitting }, watch } = useForm<RegisterFormValues>({
    resolver: zodResolver(registerSchema),
    defaultValues: {
      identifier: "",
      password: "",
      confirmPassword: "",
      agreedTerms: false
    },
    mode: "onChange"
  });

  const currentIdentifier = watch("identifier");
  
  const isPhoneNumber = currentIdentifier && /^(84|0[3|5|7|8|9])+([0-9]{8})\b$/.test(currentIdentifier);

  // Step 1: Submit Form to send OTP
  const onSubmitRegister = async (data: RegisterFormValues) => {
    try {
      // Bắn API gọi request OTP thật
      await requestRegistrationOTP(data.identifier);
      
      setRegisteredPassword(data.password);
      
      toast({
        variant: "default",
        title: "Đã gửi mã xác thực",
        description: isPhoneNumber 
          ? `Vui lòng mở Telegram để nhận mã.` 
          : `Mã xác thực đã được gửi tới ${data.identifier}`,
      });
      setStep(isPhoneNumber ? "TELEGRAM_OTP" : "OTP");
    } catch (err: any) {
      toast({
        variant: "error",
        title: "Lỗi đăng ký",
        description: err?.detail || "Hệ thống đang bận. Vui lòng thử lại sau.",
      });
    }
  };

  // Step 2: Submit OTP
  const handleVerifyOTP = async (otp: string) => {
    try {
      // Gọi API kiểm tra OTP thật
      await verifyRegistrationOTP(currentIdentifier, registeredPassword, otp);
      setStep("SUCCESS");
    } catch (err: any) {
      throw new Error(err?.detail || "OTP không hợp lệ");
    }
  };

  const handleResendOTP = async () => {
    try {
      await requestRegistrationOTP(currentIdentifier);
      toast({
        variant: "default",
        title: "Gửi lại thành công",
        description: `Mã OTP mới đã được gửi tới ${currentIdentifier}`,
      });
    } catch (err: any) {
      toast({
        variant: "destructive",
        title: "Không thể gửi lại",
        description: err?.detail || "Quá nhiều yêu cầu. Hãy đợi thêm 5 phút.",
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
             <h1 className="text-2xl font-extrabold text-fg tracking-tight">Xác thực thành công!</h1>
             <p className="mt-2 text-[14px] leading-relaxed text-fg-muted">
               Tài khoản của bạn đã được tạo và định danh thành công. Bạn có thể đăng nhập ngay để trải nghiệm EventFlow.
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

  if (step === "OTP") {
    return (
      <OTPVerification 
        identifier={currentIdentifier} 
        onVerify={handleVerifyOTP} 
        onResend={handleResendOTP} 
        onCancel={() => setStep("REGISTER")} 
      />
    );
  }

  if (step === "TELEGRAM_OTP") {
    return (
      <TelegramOTPVerification 
        identifier={currentIdentifier}
        onVerify={handleVerifyOTP}
        onCancel={() => setStep("REGISTER")}
      />
    );
  }
  return (
    <div className="flex flex-col w-full">
      <div className="bg-white rounded-[24px] p-6 sm:p-7 shadow-xl shadow-black/5 border border-border-subtle flex flex-col gap-5 relative overflow-hidden">
        {/* Subtle top highlight */}
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
          <h1 className="text-xl sm:text-2xl font-extrabold text-fg tracking-tight text-center whitespace-nowrap px-2">Đăng ký mới</h1>
          <div className="w-[100px]" />
        </div>

        <form onSubmit={handleSubmit(onSubmitRegister)} className="flex flex-col gap-4">
          {/* Field 1: Identifier */}
          <div className="flex flex-col gap-1.5">
            <div className="flex items-center justify-between">
              <label htmlFor="register-identifier" className="text-[13.5px] font-bold text-fg">
                Email hoặc Số điện thoại
              </label>
            </div>
            <div className="relative">
              <div className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-4 text-fg-muted/80">
                <ContactIcon className="size-[18px]" />
              </div>
              <input
                id="register-identifier"
                type="text"
                suppressHydrationWarning
                disabled={isSubmitting}
                {...register("identifier")}
                placeholder="Nhập email hoặc SĐT"
                className={`ef-focus-ring h-[48px] w-full rounded-[12px] border bg-white pl-11 pr-4 text-[14px] text-fg placeholder:text-fg-muted/60 transition-colors disabled:cursor-not-allowed disabled:opacity-60 ${errors.identifier ? 'border-red-500 focus:border-red-500' : 'border-border-strong hover:border-indigo-400 focus:border-indigo-500'}`}
              />
            </div>
            {errors.identifier && <span className="text-red-500 text-[12.5px] font-medium">{errors.identifier.message}</span>}
            
            {/* Inline warning cho Telegram */}
            {isPhoneNumber && !errors.identifier && (
              <div className="mt-1 flex items-start gap-2 bg-[#0088cc]/10 text-[#0088cc] p-2.5 rounded-[10px] text-[13px] leading-relaxed animate-in fade-in slide-in-from-top-1">
                <TelegramIcon className="size-[18px] shrink-0 mt-[1px]" />
                <p>
                  Mã OTP sẽ được gửi qua ứng dụng <b>Telegram</b> để bảo mật. Hãy chắc chắn bạn đã cài đặt Telegram.
                </p>
              </div>
            )}
          </div>

          {/* Field 2: Mật khẩu */}
          <div className="flex flex-col gap-1.5">
            <div className="flex items-center justify-between">
              <label htmlFor="register-password" className="text-[13.5px] font-bold text-fg">
                Mật khẩu
              </label>
            </div>
            <div className="relative">
              <div className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-4 text-fg-muted/80">
                <LockIcon className="size-[18px]" />
              </div>
              <input
                id="register-password"
                type={showPassword ? "text" : "password"}
                suppressHydrationWarning
                disabled={isSubmitting}
                {...register("password")}
                placeholder="Tối thiểu 8 ký tự"
                className={`ef-focus-ring h-[48px] w-full rounded-[12px] border bg-white pl-11 pr-12 text-[14px] text-fg placeholder:text-fg-muted/60 transition-colors disabled:cursor-not-allowed disabled:opacity-60 font-medium ${errors.password ? 'border-red-500 focus:border-red-500' : 'border-border-strong hover:border-indigo-400 focus:border-indigo-500'}`}
              />
              <button
                type="button"
                suppressHydrationWarning
                onClick={() => setShowPassword(!showPassword)}
                className="ef-focus-ring absolute inset-y-0 right-0 flex items-center pr-4 text-fg-muted transition-colors hover:text-fg"
              >
                {showPassword ? <EyeOffIcon className="size-[18px]" /> : <EyeIcon className="size-[18px]" />}
              </button>
            </div>
            {errors.password && <span className="text-red-500 text-[12.5px] font-medium">{errors.password.message}</span>}
          </div>

          {/* Field 3: Xác nhận mật khẩu */}
          <div className="flex flex-col gap-1.5">
            <div className="flex items-center justify-between">
              <label htmlFor="register-confirm" className="text-[13.5px] font-bold text-fg">
                Xác nhận mật khẩu
              </label>
            </div>
            <div className="relative">
              <div className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-4 text-fg-muted/80">
                <LockIcon className="size-[18px]" />
              </div>
              <input
                id="register-confirm"
                type={showConfirm ? "text" : "password"}
                suppressHydrationWarning
                disabled={isSubmitting}
                {...register("confirmPassword")}
                placeholder="Nhập lại mật khẩu"
                className={`ef-focus-ring h-[48px] w-full rounded-[12px] border bg-white pl-11 pr-12 text-[14px] text-fg placeholder:text-fg-muted/60 transition-colors disabled:cursor-not-allowed disabled:opacity-60 font-medium ${errors.confirmPassword ? 'border-red-500 focus:border-red-500' : 'border-border-strong hover:border-indigo-400 focus:border-indigo-500'}`}
              />
              <button
                type="button"
                suppressHydrationWarning
                onClick={() => setShowConfirm(!showConfirm)}
                className="ef-focus-ring absolute inset-y-0 right-0 flex items-center pr-4 text-fg-muted transition-colors hover:text-fg"
              >
                {showConfirm ? <EyeOffIcon className="size-[18px]" /> : <EyeIcon className="size-[18px]" />}
              </button>
            </div>
            {errors.confirmPassword && <span className="text-red-500 text-[12.5px] font-medium">{errors.confirmPassword.message}</span>}
          </div>

          {/* Checkbox: Điều khoản */}
          <div className="flex items-start gap-2.5 mt-1">
            <input
              id="register-terms"
              type="checkbox"
              disabled={isSubmitting}
              {...register("agreedTerms")}
              className="ef-focus-ring mt-0.5 size-[18px] flex-shrink-0 rounded-[4px] border-border-strong text-indigo-600 accent-indigo-600 cursor-pointer"
            />
            <div className="flex flex-col">
              <label htmlFor="register-terms" className="cursor-pointer select-none text-[13px] text-fg-muted leading-relaxed">
                Tôi đồng ý với{" "}
                <Link href="#" className="font-semibold text-indigo-600 hover:text-indigo-700 hover:underline">
                  Điều khoản sử dụng
                </Link>{" "}
                và{" "}
                <Link href="#" className="font-semibold text-indigo-600 hover:text-indigo-700 hover:underline">
                  Chính sách bảo mật
                </Link>
              </label>
              {errors.agreedTerms && <span className="text-red-500 text-[12.5px] font-medium mt-1">{errors.agreedTerms.message}</span>}
            </div>
          </div>

          {/* Primary Action */}
          <Button
            type="submit"
            suppressHydrationWarning
            loading={isSubmitting}
            className="mt-1 h-[48px] w-full rounded-[12px] bg-indigo-600 hover:bg-indigo-700 text-white text-[15px] font-bold shadow-md shadow-indigo-600/20 hover:shadow-indigo-600/30 transition-all active:scale-[0.98]"
          >
            {isSubmitting ? (
              <span>Đang xử lý...</span>
            ) : (
              <span className="inline-flex items-center justify-center gap-2">
                <UserPlusIcon className="size-[18px]" />
                <span>Tiếp tục</span>
              </span>
            )}
          </Button>
        </form>

        <div className="mt-0.5 text-center text-[13.5px] text-fg-muted pt-4">
           Đã có tài khoản?{" "}
          <Link href="/auth/sign-in" className="font-bold text-indigo-600 hover:text-indigo-700 hover:underline">
            Đăng nhập ngay
          </Link>
        </div>
      </div>
    </div>
  );
}
