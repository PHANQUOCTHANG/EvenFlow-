# Contracts change request 1 (ops-warroom, from plan review round 1)

Status: **pending human approval**. Amends `contracts.md` (already edited, marked "amended CR-1") and spec AC-6,
AC-7, AC-9, AC-12, AC-14, AC-15, AC-17 (marked "amended round 2").

| # | Change | Reason |
|---|---|---|
| 1 | `held` (§4.1) and `active_holds` (§4.2) count only open holds whose order is `HELD` or `PAYMENT_PENDING` | holds are consumed only at ticket issuance (`services/ticketing/internal/adapter/postgres/ticket_repository.go:152-158`); payment only updates `orders` (`services/payment/internal/adapter/postgres/repository.go:294-297`). Without the filter, a PAID-not-issued order is counted in both `held` and `paid`, which raises a false OVER_QUOTA alert near sold-out |
| 2 | Inventory figures come from one SQL statement; sales figures from one `REPEATABLE READ READ ONLY` transaction | a status change between separate statements would double count |
| 3 | `payment_breakdown`: one payment per order (latest SUCCEEDED); orders without one → provider `"unknown"`; totals reconcile | an order may have several SUCCEEDED rows (only `uq_provider_txn` exists) |
| 4 | Cache: detached loader context, `generated_at` = fill time, `organizer_id` cached and checked before serving; AC-17 counts snapshot queries | cancelled first caller must not fail waiters; honest age; owner check without an extra query |
| 5 | `rate_per_sec` must be an integer 0..10000; response read back from the stored value | fractions below 0.5 give batch 0 while `paused=false`; below 0.001 they are stored as 0 |
| 6 | `/problems/audit-unavailable` only when `DATABASE_URL` is unset or unparsable; unreachable DB → apply + `audit:"failed"` | a DB blip at startup must not disable emergency pause for the life of the process |
| 7 | Audit insert and Redis apply on a detached context with their own timeouts | a client disconnect after the apply must not cancel the audit |

Frozen tests written before this CR must follow it. The test designer updates them in the same freeze round and lists
them in the test change request.
