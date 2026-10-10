import ForgotPasswordForm from "@/components/auth/ForgotPasswordForm";

export const metadata = {
  title: "Quên mật khẩu — EventFlow Secure",
  description: "Khôi phục tài khoản của bạn để tiếp tục sử dụng EventFlow.",
};

export default function ForgotPasswordPage() {
  return (
    <div
      data-theme="light"
      className="min-h-screen flex flex-col items-center justify-center py-10 px-4 font-sans relative overflow-hidden bg-slate-950"
    >
      {/* Premium Background Effects */}
      <div className="absolute inset-0 w-full h-full">
        {/* Glow 1 - Top Left */}
        <div className="absolute -top-[20%] -left-[10%] w-[50%] h-[50%] rounded-full bg-indigo-600/20 blur-[120px] pointer-events-none" />
        {/* Glow 2 - Bottom Right */}
        <div className="absolute -bottom-[20%] -right-[10%] w-[50%] h-[50%] rounded-full bg-blue-600/20 blur-[120px] pointer-events-none" />
        {/* Subtle Grid Pattern Overlay */}
        <div className="absolute inset-0 bg-[url('https://grainy-gradients.vercel.app/noise.svg')] opacity-20 mix-blend-overlay pointer-events-none" />
      </div>

      <div className="relative z-10 w-full max-w-[480px]">
        <ForgotPasswordForm />
      </div>
    </div>
  );
}
