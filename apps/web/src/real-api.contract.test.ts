/**
 * Contract test dang "quet ma nguon" cho frontend-real-api (AC-3, AC-19, AC-27).
 * Chi quet file san xuat: bo qua *.test.*, thu muc test-utils.
 */
import fs from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";

const SRC = path.resolve(__dirname);
const WEB_ROOT = path.resolve(__dirname, "..");
const REPO_ROOT = path.resolve(WEB_ROOT, "..", "..");

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === "test-utils" || entry.name === "__fixtures__" || entry.name === "node_modules") continue;
      walk(full, out);
    } else if (/\.(ts|tsx)$/.test(entry.name) && !/\.(test|spec)\.(ts|tsx)$/.test(entry.name)) {
      out.push(full);
    }
  }
  return out;
}

const files = walk(SRC).map((f) => ({ file: path.relative(SRC, f).replace(/\\/g, "/"), text: fs.readFileSync(f, "utf8") }));

function offenders(re: RegExp): string[] {
  return files.filter((f) => re.test(f.text)).map((f) => f.file);
}

describe("khong con mock trong ma san xuat (AC-19)", () => {
  it("khong module san xuat nao chua MOCK_EVENTS (doi ten SAMPLE_EVENTS, chuyen sang lib/sample-data)", () => {
    expect(offenders(/\bMOCK_EVENTS\b/)).toEqual([]);
  });

  it("event-service va organizer-service khong import lib/sample-data", () => {
    for (const name of ["lib/event-service.ts", "lib/organizer-service.ts"]) {
      const f = files.find((x) => x.file === name);
      expect(f, name).toBeDefined();
      expect(f!.text, name).not.toMatch(/sample-data/);
    }
  });

  it("lib/sample-data/events.ts ton tai va chi duoc import boi ops-service", () => {
    expect(files.some((f) => f.file === "lib/sample-data/events.ts")).toBe(true);
    const importers = offenders(/sample-data\/events/).filter((f) => f !== "lib/sample-data/events.ts");
    expect(importers.sort()).toEqual(["lib/ops-service.ts"]);
  });

  it("organizer-service khong con kho mock (INITIAL_ORGANIZER_EVENTS / resetOrganizerEventsStore)", () => {
    expect(offenders(/INITIAL_ORGANIZER_EVENTS|resetOrganizerEventsStore/)).toEqual([]);
  });

  it("payment-client khong con nhanh gia thanh cong / gia gia tri mac dinh", () => {
    const f = files.find((x) => x.file === "lib/payment-client.ts");
    expect(f).toBeDefined();
    expect(f!.text).not.toMatch(/mock_pay_/);
    expect(f!.text).not.toMatch(/Vé Tiêu Chuẩn/);
  });
});

describe("khong credentials include, khong X-Identity-* (AC-3)", () => {
  it("khong file san xuat nao dung credentials: 'include'", () => {
    expect(offenders(/credentials\s*:\s*["']include["']/)).toEqual([]);
  });

  it("khong file san xuat nao gui header X-Identity-*", () => {
    expect(offenders(/x-identity-/i)).toEqual([]);
  });
});

describe("mot bien moi truong NEXT_PUBLIC_API_BASE (AC-1, AC-27)", () => {
  it("khong file san xuat nao con NEXT_PUBLIC_API_URL hoac cong 8083", () => {
    expect(offenders(/NEXT_PUBLIC_API_URL/)).toEqual([]);
    expect(offenders(/localhost:8083/)).toEqual([]);
  });

  it(".env.example tai liệu hoa NEXT_PUBLIC_API_BASE va khong con NEXT_PUBLIC_API_URL", () => {
    const candidates = [path.join(WEB_ROOT, ".env.example"), path.join(REPO_ROOT, ".env.example")].filter((p) =>
      fs.existsSync(p),
    );
    const all = candidates.map((p) => fs.readFileSync(p, "utf8")).join("\n");
    expect(all).toMatch(/NEXT_PUBLIC_API_BASE/);
    expect(all).not.toMatch(/NEXT_PUBLIC_API_URL/);
  });
});
