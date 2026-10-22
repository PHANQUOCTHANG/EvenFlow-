# Test change request: frontend-debt-r4

## TCR-1: ac3-locks-errors.test.ts, "SecurityError (reject|throw) -> in-tab single-flight" (timer assertion)

File: `apps/web/src/lib/debt/ac3-locks-errors.test.ts`, lines 78 (`expect(vi.getTimerCount()).toBeLessThanOrEqual(timersBefore)`).

Status: 2 tests stay red on the implementation of AC-3; everything else in the file is green.

Evidence: the assertion reads `vi.getTimerCount()` immediately after the requests resolve, without letting the faked
event loop drain. Response/body reads in this environment (undici streams under `vi.useFakeTimers()`) leave transient
immediate-type timers. They are independent of the refresh code: with `navigator.locks` removed entirely
(no lock code on the path at all) a single `apiFetch` that goes 401 -> refresh POST -> 200 gives
`before=3, after=8`, then `after advanceTimersByTimeAsync(0) = 0`. The same 5 timers appear whatever the
implementation does, because each response body read (401 problem read, refresh json, 200) adds one. The test needs 3
responses (2 requests + 1 refresh) on the happy path, so the count can never return to 3 without a drain.
The NETWORK_ERROR cases in the same file pass because they never read a response body.

No timer of the implementation is left pending: the lock-wait deadline timer is cleared in `finally`, and the
equivalent assertion in ac2-refresh-deadlines.test.ts ("timer hygiene", which does `advanceTimersByTimeAsync(0)` first)
is green.

Proposed change: insert `await vi.advanceTimersByTimeAsync(0);` before the `getTimerCount` assertion at line 78.
Nothing else changes.

## Approval

TCR-1 APPROVED by the human (TranDinhVo) on 2026-10-21 in the Claude Code session. Applied exactly: one added line `await vi.advanceTimersByTimeAsync(0);` before the timer assertion of the SecurityError `it.each` (both reject and throw cases). No assertion removed or weakened.
