# Test change request (fix round 2)

## G-9: 403 `/problems/forbidden` must not map to NOT_ADMITTED

- Protected test: `apps/web/src/lib/hold-client.contract.test.ts` line 83 asserts `[403, "/problems/forbidden", "NOT_ADMITTED"]` in the "requestHold: anh xa loi (AC-10)" table.
- Finding G-9 wants `/problems/forbidden` -> a distinct `FORBIDDEN` code/message (other 403s per contracts.md s9: `about:blank` -> NOT_ADMITTED).
- Required change to the contract test: replace the row with `[403, "/problems/forbidden", "FORBIDDEN"]` (keep `[403, "about:blank", "NOT_ADMITTED"]`).
- Source change that would follow (not applied): add `"FORBIDDEN"` to `HoldClientErrorType`, map in `requestHold` before the generic 403 case, add FORBIDDEN to the exclusion list and a distinct message in `checkout-view.tsx` (line ~135), and `use-hold-timer.ts` error list.
- Status: NOT applied. Awaiting an approved edit of the protected test.
