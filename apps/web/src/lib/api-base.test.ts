import { readFileSync } from "node:fs";
import path from "node:path";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

/** Hop dong EV-198 plan 1.2: `apiBase()` doc `process.env.NEXT_PUBLIC_API_BASE`, mac dinh "".
 *
 * Moi test nap lai module sau khi stub env (vi.resetModules + import dong) de test dung du
 * implementation doc env luc goi ham hay luc nap module — Next inline gia tri luc build nen ca
 * hai cach deu hop le o production. */
async function loadApiBase() {
  vi.resetModules();
  const mod = await import("./api-base");
  return mod.apiBase;
}

describe("apiBase (EV-198, B2)", () => {
  beforeEach(() => {
    vi.unstubAllEnvs();
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.resetModules();
  });

  it("trả về NEXT_PUBLIC_API_BASE khi biến môi trường được đặt", async () => {
    vi.stubEnv("NEXT_PUBLIC_API_BASE", "http://localhost:8080");
    const apiBase = await loadApiBase();
    expect(apiBase()).toBe("http://localhost:8080");
  });

  it("trả về đúng giá trị khác (không hard-code localhost)", async () => {
    vi.stubEnv("NEXT_PUBLIC_API_BASE", "https://api.evenflow.test");
    const apiBase = await loadApiBase();
    expect(apiBase()).toBe("https://api.evenflow.test");
  });

  it("trả về chuỗi rỗng khi biến môi trường không được đặt", async () => {
    vi.stubEnv("NEXT_PUBLIC_API_BASE", undefined);
    const apiBase = await loadApiBase();
    expect(apiBase()).toBe("");
  });

  it("luôn trả về string (không undefined) để ghép URL an toàn", async () => {
    vi.stubEnv("NEXT_PUBLIC_API_BASE", undefined);
    const apiBase = await loadApiBase();
    expect(typeof apiBase()).toBe("string");
    expect(`${apiBase()}/v1/events/evt-1/holds`).toBe("/v1/events/evt-1/holds");
  });
});

/** plan-review F7 (code-review C4/F-5): Next chi inline `process.env.NEXT_PUBLIC_API_BASE` khi bieu
 *  thuc viet NGUYEN VAN. Destructure hay khoa dong van xanh moi test chay tren Node (env co that)
 *  nhung tra `undefined` tren trinh duyet -> hold/thanh toan quay lai URL tuong doi (B2). Test nay
 *  kiem ma nguon (da bo comment) de chan hoi quy do. */
describe("api-base.ts — biểu thức env phải để Next inline được (plan-review F7)", () => {
  const raw = readFileSync(path.resolve(__dirname, "api-base.ts"), "utf8");
  const code = raw.replace(/\/\*[\s\S]*?\*\//g, "").replace(/(^|[^:])\/\/.*$/gm, "$1");

  it("chứa nguyên văn process.env.NEXT_PUBLIC_API_BASE", () => {
    expect(code).toContain("process.env.NEXT_PUBLIC_API_BASE");
  });

  it("không destructure process.env", () => {
    // `const { NEXT_PUBLIC_API_BASE } = process.env` hoac `const env = process.env`.
    expect(code).not.toMatch(/\}\s*=\s*process\.env\b/);
    expect(code).not.toMatch(/=\s*process\.env\s*(;|$)/m);
  });

  it("không đọc env bằng khoá động (process.env[...])", () => {
    expect(code).not.toMatch(/process\.env\s*\[/);
  });
});
