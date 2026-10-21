/** Regression G-7 (R2-6): the registered identifier never goes into the sign-in URL; one-shot sessionStorage instead. */
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installFetch, json, problem, resetEnv, setupEnv } from "@/test-utils/fetch-mock";

vi.mock("@/components/auth/OTPVerification", () => ({
  default: ({ onVerify }: { onVerify: (otp: string) => Promise<void> }) => (
    <button onClick={() => void onVerify("123456").catch(() => undefined)}>submit-otp</button>
  ),
}));
vi.mock("@react-oauth/google", () => ({
  GoogleOAuthProvider: ({ children }: { children: unknown }) => <>{children}</>,
  GoogleLogin: () => null,
}));
vi.mock("@/components/ui", async (orig) => ({ ...(await orig<object>()), useToast: () => ({ toast: vi.fn() }) }));

import LoginForm from "@/components/auth/LoginForm";
import RegisterForm from "@/components/auth/RegisterForm";

const PREFILL_KEY = "ef:signin:identifier";

beforeEach(setupEnv);
afterEach(resetEnv);

describe("G-7", () => {
  it("register -> sign-in link has no identifier in the URL and stores it one-shot in sessionStorage", async () => {
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
    expect(link.getAttribute("href")).not.toContain("a%40b.co");
    expect(link.getAttribute("href")).not.toContain("identifier");
    fireEvent.click(link);
    expect(sessionStorage.getItem(PREFILL_KEY)).toBe("a@b.co");
  });

  it("LoginForm prefills from the one-shot key and removes it", async () => {
    sessionStorage.setItem(PREFILL_KEY, "a@b.co");
    render(<LoginForm />);
    await waitFor(() => expect((document.getElementById("login-identifier") as HTMLInputElement).value).toBe("a@b.co"));
    expect(sessionStorage.getItem(PREFILL_KEY)).toBeNull();
  });
});
