"use client";

import { useState } from "react";
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



export default function RegisterForm() {
  const { toast } = useToast();
  
  const [step, setStep] = useState<"REGISTER" | "OTP" | "SUCCESS">("REGISTER");
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

  // Step 1: Submit Form to send OTP
  const onSubmitRegister = async (data: RegisterFormValues) => {
    try {
      // Bắn API gọi request OTP thật
      await requestRegistrationOTP(data.identifier);
      
      setRegisteredPassword(data.password);
      
      toast({
        variant: "default",
        title: "Đã gửi mã OTP",
        description: `Mã xác thực đã được gửi tới ${data.identifier}`,
      });
      setStep("OTP");
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
                disabled={isSubmitting}
                {...register("identifier")}
                placeholder="Nhập email hoặc SĐT"
                className={`ef-focus-ring h-[48px] w-full rounded-[12px] border bg-white pl-11 pr-4 text-[14px] text-fg placeholder:text-fg-muted/60 transition-colors disabled:cursor-not-allowed disabled:opacity-60 ${errors.identifier ? 'border-red-500 focus:border-red-500' : 'border-border-strong hover:border-indigo-400 focus:border-indigo-500'}`}
              />
            </div>
            {errors.identifier && <span className="text-red-500 text-[12.5px] font-medium">{errors.identifier.message}</span>}
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
                disabled={isSubmitting}
                {...register("password")}
                placeholder="Tối thiểu 8 ký tự"
                className={`ef-focus-ring h-[48px] w-full rounded-[12px] border bg-white pl-11 pr-12 text-[14px] text-fg placeholder:text-fg-muted/60 transition-colors disabled:cursor-not-allowed disabled:opacity-60 font-medium ${errors.password ? 'border-red-500 focus:border-red-500' : 'border-border-strong hover:border-indigo-400 focus:border-indigo-500'}`}
              />
              <button
                type="button"
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
                disabled={isSubmitting}
                {...register("confirmPassword")}
                placeholder="Nhập lại mật khẩu"
                className={`ef-focus-ring h-[48px] w-full rounded-[12px] border bg-white pl-11 pr-12 text-[14px] text-fg placeholder:text-fg-muted/60 transition-colors disabled:cursor-not-allowed disabled:opacity-60 font-medium ${errors.confirmPassword ? 'border-red-500 focus:border-red-500' : 'border-border-strong hover:border-indigo-400 focus:border-indigo-500'}`}
              />
              <button
                type="button"
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
