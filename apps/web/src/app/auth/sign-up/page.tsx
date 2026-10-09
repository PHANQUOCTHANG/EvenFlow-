import RegisterForm from "@/components/auth/RegisterForm";

export const metadata = {
  title: "Đăng ký tài khoản | EventFlow",
  description: "Đăng ký tài khoản mới trên EventFlow.",
};

export default function SignUpPage() {
  return (
    <div data-theme="light" className="min-h-screen flex items-center justify-center bg-bg py-12 px-4 sm:px-6 lg:px-8">
      <div className="w-full max-w-[448px] flex flex-col items-center">
        {/* Placeholder for Logo */}
        <div className="mb-8 flex items-center gap-2">
          <div className="w-8 h-8 bg-primary rounded-lg"></div>
          <span className="text-2xl font-bold text-brand">EventFlow</span>
        </div>
        
        <RegisterForm />
      </div>
    </div>
  );
}
