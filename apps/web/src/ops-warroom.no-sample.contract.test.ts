/**
 * ops-warroom AC-28: no sample data left in production modules (replaces the sample-data assertions of
 * real-api.contract.test.ts via the test change request). Source scan, test files excluded.
 */
import fs from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";

const SRC = path.resolve(__dirname);

function walk(dir: string, out: string[] = []): string[] {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, e.name);
    if (e.isDirectory()) {
      if (["test-utils", "__fixtures__", "node_modules"].includes(e.name)) continue;
      walk(full, out);
    } else if (/\.(ts|tsx)$/.test(e.name) && !/\.(test|spec)\.(ts|tsx)$/.test(e.name)) {
      out.push(full);
    }
  }
  return out;
}

const files = walk(SRC).map((f) => ({
  file: path.relative(SRC, f).split(path.sep).join("/"),
  text: fs.readFileSync(f, "utf8"),
}));

describe("no sample data in production code (AC-28)", () => {
  it("lib/sample-data/events.ts is deleted", () => {
    expect(fs.existsSync(path.join(SRC, "lib", "sample-data", "events.ts"))).toBe(false);
  });

  it("no production module imports sample-data or defines SAMPLE_EVENTS / __setOpsSampleOverride", () => {
    expect(files.filter((f) => /sample-data/.test(f.text)).map((f) => f.file)).toEqual([]);
    expect(files.filter((f) => /SAMPLE_EVENTS|__setOpsSampleOverride/.test(f.text)).map((f) => f.file)).toEqual([]);
  });

  it("ops-service never marks a real snapshot as sample", () => {
    const f = files.find((x) => x.file === "lib/ops-service.ts");
    expect(f).toBeDefined();
    expect(f!.text).not.toMatch(/isSample:\s*true/);
  });
});
