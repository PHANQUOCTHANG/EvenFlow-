# Plan review: frontend-debt-r4 (v3 step 2b, High-risk)

Reviewer: clean-context plan reviewer. Inputs: `spec.md`, `plan.md` (d3d539e), the code they reference at base 43375eb,
and `docs/workflow/frontend-real-api/findings-round4-{review,challenge}.json`. Nothing executed except read-only greps.

## Verdict: **FAIL**

There is 1 blocker and 4 majors. All of them can be fixed in the plan text. None needs a spec redesign. After the plan
is updated (and the test-change request in B-1 is approved), a re-review should be quick.

## AC coverage

| AC | Covered by plan | Notes |
|---|---|---|
| AC-1 | yes | m-1 |
| AC-2 | yes, under-specified | B-1, M-1, M-2, M-3 |
| AC-3 | yes | M-1 (SecurityError fallback), m-3 |
| AC-4 | yes | m-2 |
| AC-5 | yes, unsafe matching rule | M-4, m-5 |
| AC-6 | yes, file list vague | m-4 |
| AC-7 | yes | B-1 (frozen test conflict), m-6 |

## Blocker

### B-1. The AC-2 per-caller 45 s race contradicts an existing frozen test, and the plan has no test-change request
- Evidence: `apps/web/src/lib/fix2/g1-lock-refresh.test.ts:152-172` (Z-1b). The holder's POST takes 60 s and the test
  expects the holder tab's own caller to end `"A:200"` after 76 s. Spec AC-2 says that *each caller (same tab or
  other tabs)* rejects with `NETWORK_ERROR` after at most 45 s, so tab A's caller must now get `A:NETWORK_ERROR` at
  45 s. AC-7 forbids changing an existing test without a human-approved test-change request. The plan does not
  mention one. The implementer will either break the gate or quietly edit the test.
- Fix: add a TCR (`test-change-request-r4.md`) to the plan for Z-1b, to be approved before implementation. The
  change: at 45 s, `pA` is `A:NETWORK_ERROR`. Still asserted: 1 POST with r1, no REUSE, session `r2` after the late
  success (this keeps the plan's own risk row "late POST success still stores the pair"). Grep the other fix1/fix2/fix3
  suites for holder waits longer than 45 s and list every affected assertion in the TCR. The full-gate tamper check
  must allow exactly that diff.

## Major

### M-1. `inflight` must live exactly as long as the POST on every path, including the no-locks and SecurityError fallbacks
- Evidence: `api-client.ts:201-207` clears `inflight` in the `.finally` of `withRefreshLock`. `api-client.ts:164` (no
  `navigator.locks`) and the planned SecurityError fallback depend only on `inflight` for single-flight, because no
  lock exists there.
  - If the 45 s race is put inside `withRefreshLock`, for example by reusing the deadline at `:170-177`, the shared
    promise rejects at 45 s and `inflight` becomes null while the POST is still in flight. The next call then starts
    a new `doRefresh`, which re-reads the same `ef_refresh` and POSTs it a second time. That is reuse, and the whole
    token family is revoked: the round-1 bug.
  - In the SecurityError fallback, the lock-wait deadline timer is still armed (`granted` is false). It fires at 45 s
    with the same effect.
- Fix: state in the plan that the race is applied per call outside the shared promise, i.e.
  `refreshAccessToken() { inflight ??= …; return raceDeadline(inflight) }`. `inflight` is cleared only when the
  POST and the body read have settled.
  - On SecurityError, clear the lock-wait deadline (or set `granted`) before falling back to `doRefresh`. Remember
    "locks unusable" for the tab.
  - Keep the existing lock-wait abort (`:166-190`, `abandoned` check at `:182`). A waiter whose callers all timed
    out must still never be granted the lock and POST.
  - Add a test: no `navigator.locks`, POST never answers, calls at t=0, 46 s and 100 s → exactly 1 POST. Run the same
    test with a SecurityError lock manager. Add the mutation "clear `inflight` on caller timeout" and require that it
    is killed.

### M-2. The 120 s abort clears the session without the ownership guard
- Evidence: the existing 4xx path only clears when the epoch is unchanged and storage still holds the token that was
  POSTed (`api-client.ts:136`). Logout and login in another tab do not take the Web Lock (`session.ts:107`, `:184`).
  So within the 120 s window, another tab can sign out and sign in as B, or sign in again as A. An unconditional
  `clearSession()` on abort would then wipe that new, valid session. This is the "clobber another user's session"
  class that AC-1 explicitly protects ("B's stored session is left untouched").
- Fix: on abort, throw `AUTH_REQUIRED`. Clear only if
  `epoch === getSessionEpoch() && getSession()?.refreshToken === cur.refreshToken`. Test: other tab logs in as B at
  t=60 s and the abort fires at 120 s → B's session is intact and no second POST is sent.

### M-3. `AbortSignal.timeout` cannot be tested at the planned 119/121 s boundaries
- Evidence: the refresh suites use `vi.useFakeTimers()` (`g1-lock-refresh.test.ts:132` etc.). In Node,
  `AbortSignal.timeout` runs on an internal timer that Vitest's fake timers do not control, and the existing
  identity stubs (`installIdentity`, `:40-47`) ignore `init.signal`. So the boundary tests and mutation (3) would be
  nondeterministic or would pass vacuously. The abort also surfaces as `TimeoutError`, not `AbortError`, while the
  current catch at `:129-131` maps every fetch throw to `NETWORK_ERROR`.
- Fix: use `new AbortController()` with `setTimeout(REFRESH_POST_TIMEOUT_MS)`, keep it armed until `res.json()` has
  settled (a hung body must also abort), and clear it in `finally`. Classify by `controller.signal.aborted`, not by
  error name. The test stubs must reject on `signal` abort; say so in the plan.

### M-4. AC-5 name matching can adopt the wrong server tier and silently change its quota
- Evidence: `edit-event-client.tsx:68-82`. The plan matches still-unsaved tiers by trimmed name against all of
  `current.tiers`. A new tier that the organizer gives the same name as a different, existing tier is then "matched".
  No 422 is shown, and `updateTierQuota` runs on the existing tier with the new tier's quota, so an unrelated tier is
  changed without any error. Also, the backend compares names case-insensitively after trimming
  (`services/event/internal/domain/event.go:122` `strings.EqualFold`, `ticket_tier.go:23`). An exact-case comparison
  therefore disagrees with the server's duplicate rule.
- Fix: only consider server tiers whose id is not already referenced, i.e. not an id in `data.tiers` and not a value
  in `created.current`. Require equal price as well (the lost-response tier has the same name, price and quota as
  sent). Compare `trim().toLowerCase()`. If there is no unique match, keep the 422 and show the server detail (spec
  edge case). Add tests: "new tier named like an existing tier → 422 shown, existing tier's quota untouched" and
  "lost response with case-only difference".

## Minor

- **m-1 (AC-1)** `inflight` is shared regardless of who started it (`api-client.ts:201-207`). After a same-tab switch,
  a caller with a different `sentUserId` joins a promise that checks the first caller's user and gets
  `AUTH_REQUIRED` even though its own session is valid. Fix: key `inflight` by `sentUserId`, or skip the join when
  they differ. Also spell out that the post-POST check at `:147` compares `after.user.id` with `sentUserId`, and that
  the proactive path (`:240-243`) passes `session.user.id`.
- **m-2 (AC-4)** `reconcileBuyFlowOwner` calls `getSession()` (`session.ts:99`). If `getSession()` itself
  reconciles, the two recurse forever. Reconcile must read through `readFrom(...)` directly. The read path should
  write `ef:lastuid` only when the value differs, and must never call `notify()` or `setSession` (each re-renders
  through `use-auth`).
  - Install the `storage` listener once at module level in `session.ts`, guarded by `typeof window` and a flag. Do
    not install it in `use-auth.ts`, which only runs on pages that mount the hook. Handle `e.key === null`.
  - Cost per `getSession()` is one extra `sessionStorage.getItem`, which is acceptable.
  - Keep the G-4 rule: no session means do nothing, and `ef:lastuid` is kept.
- **m-3 (AC-3)** The spec says "no POST" for every lock error but allows a fallback that POSTs on SecurityError. The
  tests must state the expectation per error name: SecurityError → in-tab refresh (1 POST); InvalidStateError,
  NotSupportedError, AbortError, sync TypeError → `NETWORK_ERROR`, 0 POST, `vi.getTimerCount()` unchanged.
- **m-4 (AC-6)** "Payment page client / result page client" is too vague. The files and lines to change:
  - `hooks/use-order-status.ts:92` (order polling stop) and `:173` (ticket issuing stop)
  - `components/checkout/order-result-view.tsx:75-84` (currently shows a retry button for any non-NOT_FOUND error)
  - `components/checkout/payment-handoff-view.tsx:126-140`
  - `hooks/use-payment-session.ts:145-149`

  W-5 says a TCR is needed. I checked: no contract test pins payment 403 → UNKNOWN
  (`payment-client.contract.test.ts` has none), so none is needed. Record that in the plan.
- **m-5 (AC-5)** `addTier` failures arrive as `ApiError` 422 from `call()` (`organizer-service.ts:343`). There is no
  distinct problem type, only the detail "ticket tier name already exists for this event". The backend also returns
  ErrDuplicateTierName for an empty name (`ticket_tier.go:26`). Plan: on any 422 from `addTier`, reload and run the
  M-4 lookup. If no match, show the server detail.
- **m-6 (mutation)** The 5 mutations cover only AC-1..AC-4. Add:
  - clear `inflight` on caller timeout (M-1)
  - unguarded `clearSession` on abort (M-2)
  - name match ignoring the "already referenced" filter (M-4)
  - FORBIDDEN not stopping polling (AC-6)

  Keep the threshold at all killed.
- **m-7 (process)** The plan lists no test files and no red-check evidence for the new ACs. List the planned test
  files per AC. Rollback is concrete enough: there is no new storage key, `ef:lastuid` semantics are backward
  compatible, and `git revert -m 1` plus `npm test` and `npm run build` is fine. Also add `npx tsc --noEmit` and
  lint to the rollback check.

## Consistency check (no finding)

- Double POST per token is avoided if M-1 is applied: one POST, inside the lock, with no steal, and the abandoned
  waiter never POSTs.
- Refresh outside the lock: none is introduced, except the deliberate SecurityError fallback, where no other tab
  can share the lock.
- Session resurrection after logout: the F-12 epoch and after-check at `:142-147` are untouched. The 120 s abort only
  clears and never writes.
- Interaction with other tabs: they keep their own 45 s lock-wait deadline. The lock is released by about 120 s plus
  the body read, so other tabs recover after that.
