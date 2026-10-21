# Change request (WP2 -> owner of WP1 / integrator)

## CR-1: `lib/api-client.ts` comment fails `real-api.contract.test.ts` ("khong file san xuat nao gui header X-Identity-*")

`apps/web/src/lib/api-client.ts:4` has a doc comment containing the literal `X-Identity-*`. The scan test
`real-api.contract.test.ts > khong file san xuat nao gui header X-Identity-*` greps production files with
`/x-identity-/i`, so this comment is the only remaining offender in the repo (all WP2 files are clean).
The file is owned by WP1 and was not modified. Proposed fix (comment only): reword to "khong gui header danh tinh do client tu tao".

## CR-2: no QR rendering library, so the result page cannot draw a scannable QR image

The spec asks for "one QR per ticket from qrPayload using the existing QR rendering approach". The existing approach
was a static placeholder SVG (a fake glyph, not a QR) and `package.json` has no QR dependency (adding one is out of scope).
WP2 therefore renders, per ticket, the existing `role="img"` frame (`aria-label` "Ma QR ve {code}") that carries the real signed
`qr_payload` (`data-qr-payload` attribute and visible text), and removes the fake glyph and the "DEMO" label so no demo QR is ever shown.
A real scannable image needs a QR encoder dependency (for example `qrcode`) or an in-repo encoder: decision for the integrator/human.

## Notes (no action required)

- `issueTickets` forgets the stored `ef:issue:{orderId}` key after an HTTP error response so that a retry uses a fresh key
  (the idempotency middleware may replay a cached error for the old key). The key stays stable across reloads after success and
  across network failures.
- `apiBaseUrl` / `baseUrl` / `identityId` parameters were removed from hold, queue-status and hooks (apiFetch owns the base);
  `InitiatePaymentParams.apiBaseUrl` is kept as a deprecated ignored field because `payment-client.contract.test.ts` still passes it.
