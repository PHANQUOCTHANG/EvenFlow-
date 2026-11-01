# Test change request: frontend-debt-r5

No existing test is expected to break legitimately. Checked: no existing test seeds a role outside buyer|organizer|moderator|ops
as a session (the only `role: "admin"` hit is in assistant-panel.test.tsx, unrelated to session storage); no existing test
asserts a non-http(s) redirect_url. Watch item for the implementer (not a request): existing tests re-use the same refresh
string across `it` blocks; the AC-2 record lives in sessionStorage, which `setupEnv()` clears before each test, so none should
be affected. If a failure appears only because of that reuse, report it here rather than weakening AC-2.

## TCR-1 (implementer, step 3): frozen AC-2 record key conflicts with existing R3-1 (open, needs human decision)

**Conflict.** Two protected tests cannot both pass with any implementation:

- `apps/web/src/lib/debt5/ac2-used-refresh.test.ts` (frozen at 3e939bb) fixes the record key as sessionStorage
  `ef:refresh:used` (`const USED = "ef:refresh:used"`), requires it to exist after refreshes that ended in 401 (each
  calls `clearSession()`) and to hold 16 entries after 20 of them ("record format" test). So the record key contains
  `refresh` and must survive `clearSession()`.
- `apps/web/src/lib/fix2/g1-lock-refresh.test.ts` "H-1 > R3-1: the refresh token lives only under ef_refresh
  (persist=local|session)" scans both stores during the POST, after a successful refresh and after `clearSession()`, and
  fails on ANY key other than `ef_refresh` that matches `/refresh/` (`if (k !== "ef_refresh" && (... || /refresh/.test(k)))`
  and `expect(st.key(i)).not.toMatch(/refresh/)`). AC-2 marks the token on 2xx (P-5; the frozen "2xx rotation r1->r2"
  test requires it), so after the successful refresh `ef:refresh:used` exists and R3-1 fails.

**Evidence** (after ea37b03, `npx vitest run src/lib/fix2/g1-lock-refresh.test.ts`):

```
AssertionError: expected 'ef:refresh:used' not to match /refresh/
Tests  2 failed | 7 passed (9)
```

R3-1's intent is "the refresh token is stored only under ef_refresh". The record holds only FNV-1a hex hashes
(`[0-9a-f]`, so R3-1's value check `v.includes("r1"|"r2")` can never match); only the key-name check trips.

**Current state.** Implemented with the key the frozen debt5 tests and traceability require (`ef:refresh:used`, single
constant `K_REFRESH_USED` in `apps/web/src/lib/session.ts`). R3-1 (2 parametrised cases) is red; all else green.

**Options (pick one):**

1. (Recommended, touches only new tests) Rename the record key to a name without `refresh`, e.g. `ef:rt:used`: change
   `const USED` in `ac2-used-refresh.test.ts` plus the spec/traceability wording; the implementation change is the one
   constant. All existing tests stay unchanged (AC-12).
2. Amend R3-1 to exempt `ef:refresh:used` from the key-name check (value check kept). This edits a protected existing test.

## Note (not a test change): P-4 eager wipe not implemented, the frozen tests require the lazy wipe only

Plan §7 P-4 / spec §5 ask to wipe every store still holding the posted token right after marking it on the abort /
400-401-403 / AC-1 paths when the guard did not clear. The frozen R2-3 and R2-S1 repros in `ac2-used-refresh.test.ts`
then send request B and expect `B:AUTH_REQUIRED`. With the eager wipe the stale copy is already gone, so B starts with no
session, goes out as a public request and returns the raw `401` (`B:401`), which the AC-4 guard "a call that starts without
any session is still sent without a bearer" requires. Only the lazy wipe in `doRefresh` (used check before the reuse
branch; wipe only the store holding the used token) satisfies both, so that is what is implemented. Residual: a stale copy
can outlive its record entry only if 16 further refresh tokens get marked in the same tab before the copy is read again.
Step 4 reviewers should confirm this trade-off.
