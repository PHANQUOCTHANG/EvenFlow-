/** Regression F-16 (S-1): Google sign-in honours "remember me". */
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installFetch, json, noContent, resetEnv, setupEnv } from "@/test-utils/fetch-mock";

vi.mock("@react-oauth/google", () => ({
  GoogleOAuthProvider: ({ children }: { children: unknown }) => <>{children}</>,
  GoogleLogin: ({ onSuccess }: { onSuccess: (r: { credential: string }) => void }) => (
    <button onClick={() => onSuccess({ credential: "cred" })}>google</button>
  ),
}));
vi.mock("@/components/ui", async (orig) => ({ ...(await orig<object>()), useToast: () => ({ toast: vi.fn() }) }));

import LoginForm from "@/components/auth/LoginForm";

beforeEach(() => {
  setupEnv();
  installFetch((c) => {
    if (c.path === "/api/v1/auth/google" || c.path === "/api/v1/auth/refresh")
      return json({ id: "u-1", role: "buyer", access_token: "g-acc", refresh_token: "g-ref" });
    if (c.path === "/api/v1/auth/devices") return json({ device_id: "d1" });
    return noContent();
  });
});
afterEach(resetEnv);

describe("F-16", () => {
  it("remember unchecked -> session stored in sessionStorage only", async () => {
    render(<LoginForm />);
    const box = screen.getByRole("checkbox");
    if ((box as HTMLInputElement).checked) fireEvent.click(box);
    fireEvent.click(screen.getByText("google"));
    await waitFor(() => expect(sessionStorage.getItem("ef_token")).not.toBeNull());
    expect(localStorage.getItem("ef_token")).toBeNull();
    expect(localStorage.getItem("ef_refresh")).toBeNull();
  });
  it("remember checked -> localStorage", async () => {
    render(<LoginForm />);
    const box = screen.getByRole("checkbox") as HTMLInputElement;
    if (!box.checked) fireEvent.click(box);
    fireEvent.click(screen.getByText("google"));
    await waitFor(() => expect(localStorage.getItem("ef_token")).not.toBeNull());
  });
  it("F-6/G-7: sign-in prefills identifier from the one-shot sessionStorage key (not the URL)", async () => {
    sessionStorage.setItem("ef:signin:identifier", "a@b.co");
    render(<LoginForm />);
    await waitFor(() => expect(document.getElementById("login-identifier") as HTMLInputElement).toHaveValue("a@b.co"));
    expect(sessionStorage.getItem("ef:signin:identifier")).toBeNull();
  });
});
