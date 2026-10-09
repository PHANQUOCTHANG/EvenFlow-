import LoginForm from "@/components/auth/LoginForm";

export const metadata = {
  title: "Đăng nhập Eventflow — EventFlow Secure",
  description: "Xác thực danh tính an toàn trước khi vào phiên phòng chờ mua vé EventFlow.",
};

export default function SignInPage() {
  return (
    <div data-theme="light" className="min-h-screen bg-bg text-fg">
      <LoginForm />
    </div>
  );
}
