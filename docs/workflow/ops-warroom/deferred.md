# Deferred findings (ops-warroom)

Recorded in fix round 2 (2026-11-21). Not fixed in this feature; follow-up debt.

| ID | Severity | Item | Why deferred |
|---|---|---|---|
| R2-2 | nit | Client/server edge-trim mismatch for U+0085 | Covered by the round 2 reason fix (X2-5): U+0085 is now a control character on both sides and rejected. No separate work left. |
| R2-4 | nit | Polling keeps running after a 401 while the sign-in prompt is shown (`use-ops-war-room.ts`, sales dashboard the same) | No redirect loop; polling lets the page recover after re-login in another tab. Optional: stop or back off on 401 and add a return-to parameter on the sign-in link. |
| X2-6 | nit | ai-worker identity check differs from `authx.PrincipalFromHeaders` (unknown role -> 403 instead of 401; malformed `X-Device-Id` ignored) | No spoofing path; gateway strips client `X-Identity-*`. Parity change can follow in an ai-worker task. |

Not deferred here: R2-5 (integration suites with the `integration` tag are vetted but not executed in the gate) is a human decision; the gate was not changed.
