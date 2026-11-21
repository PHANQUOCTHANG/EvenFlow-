/** Plan P-6 (mutant M-14): an identity response with an unknown/missing role keeps the previous role ONLY for the same
 *  user id; a previous session of a DIFFERENT user id -> buyer. */
import { describe, expect, it } from "vitest";

import { sessionFromIdentity } from "@/lib/session";

describe("P-6 prev role is only inherited by the same user id", () => {
  for (const role of ["admin", ""]) {
    it(`role ${JSON.stringify(role)} for u-2, prev u-1 organizer -> buyer`, () => {
      const s = sessionFromIdentity(
        { id: "u-2", role, access_token: "a", refresh_token: "r" },
        { id: "u-1", role: "organizer" },
      );
      expect(s?.user).toEqual({ id: "u-2", role: "buyer" });
    });
  }

  it("GUARD: role 'admin' for u-1, prev u-1 organizer -> organizer", () => {
    const s = sessionFromIdentity(
      { id: "u-1", role: "admin", access_token: "a", refresh_token: "r" },
      { id: "u-1", role: "organizer" },
    );
    expect(s?.user.role).toBe("organizer");
  });
});
