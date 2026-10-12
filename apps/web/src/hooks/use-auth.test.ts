import { describe, it, expect } from "vitest";
import { parseJwtPayload } from "./use-auth";

describe("parseJwtPayload", () => {
  it("trả về null nếu token không đúng định dạng 3 phần", () => {
    expect(parseJwtPayload("invalid-token")).toBeNull();
    expect(parseJwtPayload("a.b")).toBeNull();
    expect(parseJwtPayload("")).toBeNull();
  });

  it("giải mã thành công payload từ token JWT hợp lệ", () => {
    // Payload: {"sub":"123","role":"organizer","email":"org@eventflow.dev"}
    const header = btoa(JSON.stringify({ alg: "HS256", typ: "JWT" }));
    const payload = btoa(
      JSON.stringify({ sub: "123", role: "organizer", email: "org@eventflow.dev" })
    );
    const signature = "dummy-sig";
    const token = `${header}.${payload}.${signature}`;

    const decoded = parseJwtPayload(token);
    expect(decoded).not.toBeNull();
    expect(decoded?.sub).toBe("123");
    expect(decoded?.role).toBe("organizer");
    expect(decoded?.email).toBe("org@eventflow.dev");
  });
});
