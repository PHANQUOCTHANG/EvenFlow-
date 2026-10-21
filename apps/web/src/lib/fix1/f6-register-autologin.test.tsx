/** Regression F-6 (X-4): verify succeeded but auto-login failed -> not an "OTP invalid" error; route to sign-in with identifier. */
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { verifyRegistrationOTP } from "@/lib/api/auth";
import { installFetch, json, problem, rejection, resetEnv, setupEnv } from "@/test-utils/fetch-mock";

vi.mock("@/components/auth/OTPVerification", () => ({
  default: ({ onVerify }: { onVerify: (otp: string) => Promise<void> }) => (
    <button
      onClick={() => {
        onVerify("123456").catch((e: Error) => {
          document.body.setAttribute("data-otp-error", e.message);
        });
      }}
    >
      submit-otp
    </button>
  ),
}));
vi.mock("@react-oauth/google", () => ({ GoogleOAuthProvider: ({ children }: { children: unknown }) => <>{children}</>, GoogleLogin: () => null }));
vi.mock("@/components/ui", async (orig) => ({ ...(await orig<object>()), useToast: () => ({ toast: vi.fn() }) }));

import RegisterForm from "@/components/auth/RegisterForm";

beforeEach(setupEnv);
afterEach(() => {
  document.body.removeAttribute("data-otp-error");
  resetEnv();
});

describe("F-6", () => {
  it("lib: verify 201 + login 429 rejects with REGISTERED_LOGIN_FAILED (account exists)", async () => {
    installFetch((c) =>
      c.path === "/api/v1/auth/register/verify" ? json({ id: "u-1", role: "buyer", created_at: "x" }, 201) : problem(429, "/problems/rate-limited"),
    );
    const err = (await rejection(verifyRegistrationOTP("a@b.co", "password1", "123456"))) as { code: string };
    expect(err.code).toBe("REGISTERED_LOGIN_FAILED");
  });

  it("lib: verify failure keeps its own error (409)", async () => {
    installFetch(() => problem(409, "/problems/conflict"));
    const err = (await rejection(verifyRegistrationOTP("a@b.co", "password1", "123456"))) as { status: number; code: string };
    expect(err.status).toBe(409);
    expect(err.code).not.toBe("REGISTERED_LOGIN_FAILED");
  });

  it("form: shows account-created + sign-in link with identifier, never 'OTP khong hop le'", async () => {
    installFetch((c) => {
      if (c.path === "/api/v1/auth/register/request-otp") return json({});
      if (c.path === "/api/v1/auth/register/verify") return json({ id: "u-1", role: "buyer", created_at: "x" }, 201);
      return problem(429, "/problems/rate-limited");
    });
    render(<RegisterForm />);
    fireEvent.change(screen.getByLabelText(/Email hoặc Số điện thoại/i), { target: { value: "a@b.co" } });
    fireEvent.change(document.getElementById("register-password") as HTMLInputElement, { target: { value: "password1" } });
    fireEvent.change(document.getElementById("register-confirm") as HTMLInputElement, { target: { value: "password1" } });
    fireEvent.click(document.getElementById("register-terms") as HTMLInputElement);
    fireEvent.submit(document.querySelector("form") as HTMLFormElement);
    fireEvent.click(await screen.findByText("submit-otp"));
    const link = await waitFor(() => screen.getByRole("link", { name: /Đăng nhập/ }));
    expect(link.getAttribute("href")).toBe("/auth/sign-in?identifier=a%40b.co");
    expect(screen.getByText(/tài khoản đã được tạo/i)).toBeInTheDocument();
    expect(document.body.getAttribute("data-otp-error")).toBeNull();
  });
});
