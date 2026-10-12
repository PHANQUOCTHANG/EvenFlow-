"use client";

import { useState, useRef, useEffect, KeyboardEvent } from "react";
import Image from "next/image";
import { Button, Alert, useToast } from "@/components/ui";

interface OTPVerificationProps {
  identifier: string;
  onVerify: (otp: string) => Promise<void>;
  onResend: () => Promise<void>;
  onCancel: () => void;
}

export default function OTPVerification({ identifier, onVerify, onResend, onCancel }: OTPVerificationProps) {
  const { toast } = useToast();
  
  const [otp, setOtp] = useState<string[]>(Array(6).fill(""));
  const [loading, setLoading] = useState(false);
  const [countdown, setCountdown] = useState(60);
  
  const inputRefs = useRef<(HTMLInputElement | null)[]>([]);

  // Focus first input on mount
  useEffect(() => {
    if (inputRefs.current[0]) {
      inputRefs.current[0].focus();
    }
  }, []);

  // Timer logic
  useEffect(() => {
    if (countdown <= 0) return;
    const timer = setInterval(() => {
      setCountdown((prev) => prev - 1);
    }, 1000);
    return () => clearInterval(timer);
  }, [countdown]);

  const handleChange = (index: number, value: string) => {
    if (!/^[0-9]*$/.test(value)) return;
    
    const newOtp = [...otp];
    newOtp[index] = value;
    setOtp(newOtp);

    // Auto-advance
    if (value && index < 5 && inputRefs.current[index + 1]) {
      inputRefs.current[index + 1]?.focus();
    }
  };

  const handleKeyDown = (index: number, e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Backspace" && !otp[index] && index > 0) {
      // Go back if empty
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
    
    // Focus the next empty input or the last one
    const focusIndex = Math.min(pastedData.length, 5);
    inputRefs.current[focusIndex]?.focus();
  };

  const handleSubmit = async () => {
    const otpValue = otp.join("");
    if (otpValue.length < 6) {
      toast({
        variant: "warning",
        title: "Thiếu thông tin",
        description: "Vui lòng nhập đủ 6 số OTP."
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
        description: err?.detail || "Mã OTP không chính xác hoặc đã hết hạn."
      });
      // Clear inputs on error
      setOtp(Array(6).fill(""));
      inputRefs.current[0]?.focus();
    } finally {
      setLoading(false);
    }
  };

  const handleResend = async () => {
    if (countdown > 0) return;
    try {
      await onResend();
      setCountdown(60);
      setOtp(Array(6).fill(""));
      inputRefs.current[0]?.focus();
    } catch (err: any) {
      toast({
        variant: "error",
        title: "Không thể gửi lại",
        description: err?.detail || "Không thể gửi lại mã OTP lúc này."
      });
    }
  };

  return (
    <div className="flex flex-col w-full">
      <div className="bg-white rounded-[24px] p-6 sm:p-7 shadow-xl shadow-black/5 border border-border-subtle flex flex-col gap-5 relative overflow-hidden">
        {/* Top highlight */}
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
          <h1 className="text-xl sm:text-2xl font-extrabold text-fg tracking-tight text-center whitespace-nowrap px-2">Xác thực OTP</h1>
          <div className="w-[100px]" />
        </div>

        <div className="text-center sm:text-left mb-2">
          <p className="text-[14px] leading-relaxed text-fg-muted">
            Vui lòng nhập mã gồm 6 chữ số vừa được gửi tới:
          </p>
          <p className="text-[14px] font-bold text-fg mt-1 bg-slate-50 inline-block px-3 py-1 rounded-full border border-border-subtle">{identifier}</p>
        </div>

        <div className="flex justify-center sm:justify-between gap-2 sm:gap-3 py-2">
          {otp.map((digit, index) => (
            <input
              key={index}
              ref={(el) => { inputRefs.current[index] = el; }}
              type="text"
              inputMode="numeric"
              pattern="[0-9]*"
              maxLength={1}
              value={digit}
              onChange={(e) => handleChange(index, e.target.value)}
              onKeyDown={(e) => handleKeyDown(index, e)}
              onPaste={handlePaste}
              disabled={loading}
              className="ef-focus-ring w-11 h-14 sm:w-12 sm:h-16 text-center text-xl sm:text-2xl font-bold rounded-[12px] border border-border-strong bg-white text-fg transition-colors focus:border-indigo-500 disabled:opacity-60"
            />
          ))}
        </div>

        <Button
          onClick={handleSubmit}
          loading={loading}
          disabled={otp.join("").length < 6}
          className="mt-2 h-[48px] w-full rounded-[12px] bg-indigo-600 hover:bg-indigo-700 text-white text-[15px] font-bold shadow-md shadow-indigo-600/20 hover:shadow-indigo-600/30 transition-all active:scale-[0.98]"
        >
          Xác nhận tài khoản
        </Button>

        <div className="mt-2 flex flex-col items-center gap-3 text-[13.5px]">
          <div className="text-fg-muted">
            Chưa nhận được mã?{" "}
            {countdown > 0 ? (
              <span className="font-semibold text-fg">Gửi lại sau {countdown}s</span>
            ) : (
              <button 
                onClick={handleResend}
                className="font-bold text-indigo-600 hover:text-indigo-700 hover:underline transition-all"
              >
                Gửi lại mã ngay
              </button>
            )}
          </div>
          <button 
            onClick={onCancel}
            disabled={loading}
            className="text-fg-muted hover:text-fg font-medium transition-colors"
          >
            Quay lại sửa thông tin
          </button>
        </div>
      </div>
    </div>
  );
}
