import LoginForm from "@/components/auth/LoginForm";

export const metadata = {
  title: "Đăng nhập | EventFlow",
  description: "Đăng nhập vào hệ thống EventFlow để mua vé.",
};

export default function SignInPage() {
  return (
    <div className="min-h-screen flex items-center justify-center bg-[#F5F7FA] py-12 px-4 sm:px-6 lg:px-8">
      <div className="w-full max-w-md flex flex-col items-center">
        {/* Placeholder for Logo */}
        <div className="mb-8 flex items-center gap-2">
          <div className="w-8 h-8 bg-[#5B55E7] rounded-lg"></div>
          <span className="text-2xl font-bold text-[#18243A]">EventFlow</span>
        </div>
        
        <LoginForm />
      </div>
    </div>
  );
}
