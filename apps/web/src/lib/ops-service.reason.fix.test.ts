/** ops-warroom fix round 1: R-1 + X-4 on the web client. Reasons with control characters or without at least
 * three visible characters are rejected with INVALID before any fetch. */
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { OpsWriteError, clearAdmitOverride, normalizeOverrideReason, setAdmitOverride } from "@/lib/ops-service";
import { installFetch, json, rejection, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

beforeEach(() => {
  setupEnv();
  seedSession("acc-ops", "ref-ops", "ops");
});
afterEach(resetEnv);

const EV = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";
const ch = (...codes: number[]) => String.fromCharCode(...codes);
const ZWSP = ch(0x200b);

const BAD = [
  "abc" + ch(0),
  "ab" + ch(7) + "c",
  "ab" + ch(10) + "c",
  "ab" + ch(0x7f) + "c",
  ZWSP + ZWSP + ZWSP,
  ch(0x200c, 0x200d, 0xfeff),
  ch(0xa0, 0x3000, 0x2003) + ZWSP,
  "a" + ZWSP + ZWSP + "b",
];

describe("reason with control or invisible characters (R-1, X-4)", () => {
  it.each(BAD.map((r) => [JSON.stringify(r), r]))("rejects %s on the client without a fetch", async (_n, reason) => {
    expect(normalizeOverrideReason(reason)).toBeNull();
    const { calls } = installFetch(() => json({}));
    const e1 = await rejection(setAdmitOverride(EV, 5, reason));
    const e2 = await rejection(clearAdmitOverride(EV, reason));
    for (const e of [e1, e2]) {
      expect(e).toBeInstanceOf(OpsWriteError);
      expect((e as OpsWriteError).code).toBe("INVALID");
    }
    expect(calls).toHaveLength(0);
  });

  it("counts visible characters only and trims zero-width ends", () => {
    expect(normalizeOverrideReason("a" + ZWSP + "bc")).toBe("a" + ZWSP + "bc");
    expect(normalizeOverrideReason(ZWSP + " abc " + ch(0xfeff))).toBe("abc");
  });
});
