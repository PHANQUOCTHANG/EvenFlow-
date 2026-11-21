# k6-load-suite — verification evidence (v3 step 6, Medium tier)

Verifier: independent, read-only. Branch `feature/k6-load-suite`, HEAD 3a9bae6, 2026-11-22.

**Verdict: PASS (offline scope).** AC-1, AC-3 latencies at scale, AC-6 replay, AC-8, AC-9, AC-10 (SQL) remain NOT VERIFIED until the human runs the Docker steps below.

## 1. Gate report vs code
- gate-report.json `commit_sha` = e3bd2c1 (docs: traceability). Last code commit = 7b0cedd (docs(load) README/Makefile/spec). Nothing between 7b0cedd and e3bd2c1 touches code; 3a9bae6 (HEAD) changes only gate-report.json, gate logs, traceability.yaml. OK.
- `node --test tests/load/lib/*.test.mjs` re-run at HEAD: 73 tests, 73 pass, 0 fail.
- Gate itself: 3 steps NOT RUN (exit 77): shellcheck, k6-inspect, load-smoke.

## 2. AC map
| AC | Evidence | Status |
|---|---|---|
| AC-1 smoke | main-wiring.test.mjs (fake gateway, k6 stubs), profiles.test.mjs smoke thresholds | Offline part PASS; real run NOT VERIFIED |
| AC-2 profiles | profiles.test.mjs (5 profiles, defaults, overrides, unknown/malformed throw) | PASS |
| AC-3 thresholds | profiles.test.mjs (SLO values, SLO_SCALE latency only, abortOnFail counts); mutants M4, M11, M15 | PASS (thresholds declared); latencies at scale NOT VERIFIED |
| AC-4 auth | jwt.test.mjs (fixture vs authx.SignAccessToken, claims, exp), main-wiring (zero /auth calls, no X-Identity), flow.test.mjs guardTarget; seed idempotency needs Docker | PASS offline; seed rerun NOT VERIFIED |
| AC-5 polling | flow.test.mjs nextPollDelayMs (200/304/429, floor, jitter); mutants M1, M2 | PASS |
| AC-6 idempotency | flow.test.mjs idemKey/shouldReplay, main-wiring replay and idem_mismatch, 409 sold out; M3 | PASS offline; replay vs real services NOT VERIFIED |
| AC-7 signals | signals.test.mjs ranges, uniqueness, size, min/none; M9 | PASS (contract copy only; not checked against live antibot-phase1 code) |
| AC-8 spike | profiles.test.mjs ramping-arrival-rate; summary.test.mjs dropped_iterations | PASS config; 100k run NOT VERIFIED |
| AC-9 wait | profiles.test.mjs constant-vus, jwt lifetime | PASS config; 30 min run NOT VERIFIED |
| AC-10 oversell | summary.test.mjs oversell_suspected; M12; `seed-load.sh verify` SQL | PASS suspicion counter; SQL check NOT VERIFIED |
| AC-11 results | summary.test.mjs path + PASS/FAIL table; .gitignore | PASS |
| AC-12 offline tests | 73/73 with the glob (Amendment: glob, not directory) | PASS |
| Amendment | `make test-load-lib` uses the glob (gate make-dry-run log) | PASS |

## 3. Mutation (15 mutants, applied one at a time, reverted; log: verify-logs/mutation.log)
| # | Mutant | Result |
|---|---|---|
| M1 | poll delay ignores server `poll_after_ms` | KILLED |
| M2 | 304 reads wrong header | KILLED |
| M3 | idempotency key loses step | KILLED |
| M4 | SLO_SCALE applied to hold count | KILLED |
| M5 | guardTarget allows remote | KILLED |
| M6 | webhook rate below checkout rate allowed | KILLED |
| M7 | FEW_LEFT rejected | KILLED |
| M8 | run id falls back to shared name | KILLED |
| M9 | signals > 2 KiB | KILLED |
| M10 | token typ wrong | KILLED |
| M11 | join latency SLO x10 | KILLED |
| M12 | oversell off by one | KILLED |
| M13 | webhook slot ignores hold-expiry margin | KILLED |
| M14 | identity range 9000 to 8000 | KILLED (first attempt hit a comment = no-op; rerun on the code line killed it) |
| M15 | idem_mismatch threshold weakened | KILLED |

Kill count 15/15, 0 survivors. Working tree reverted clean.

## 4. Rollback / scope
`git diff origin/develop...HEAD --name-only`: only tests/load/**, Makefile, .gitignore, docs/workflow/k6-load-suite/** (no other paths). Tooling only; rollback = revert the merge (restores the old tests/load/opening-spike.js). No production code, migrations, compose/k8s or CI changes.

## 5. NOT VERIFIED (needs Docker / k6 / large hosts)
- shellcheck of seed-load.sh, `k6 inspect`, real k6 execution (all exit 77 in gate).
- AC-1 real smoke, AC-6 replay same hold_id/payment_id, 304 via gateway (ETag forwarding assumption), seed-load.sh idempotency and `verify`.
- AC-3 latency SLOs at scale, AC-8 100k arrivals, AC-9 30-min wait: need k8s stack (EVF-120..122) and distributed generators.
- AC-7 against real anti-bot (`ANTIBOT_MODE=shadow` required, D5).

## 6. Human steps once Docker is on
1. Compose up, migrated, sale open (`SALE_START_IN_SECONDS=0`), `ANTIBOT_MODE=shadow`, k6 installed.
2. `make load-smoke` (first run, runs seed-load.sh defaults: 50 users, quota 200). Run it a **second time** (idempotent seed, fresh RUN_ID).
3. `make load-verify` (seed-load.sh verify, SQL).
4. Optional: `shellcheck tests/load/seed-load.sh`.

Pass criteria: each smoke run exits 0 in < 2 min; every threshold PASS and OVERALL PASS in the summary; results file `tests/load/results/smoke-<RUN_ID>.json` written; run includes >= 1 join, 1 status 304, 1 hold, 1 payment, 1 webhook, `checkout_issued >= 1`, `checkout_issue_timeout == 0`, `idem_mismatch == 0`, `oversell_suspected == 0`, zero `/api/v1/auth/*` calls; second run creates no duplicate identity rows; `make load-verify` reports `issued + open holds <= quota` for every ticket type and exits 0. Paste machine, commit and the summary table below this line.
