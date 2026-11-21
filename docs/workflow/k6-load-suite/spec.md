# k6-load-suite — k6 load suite: opening spike, 30-minute wait, checkout wave (Spec)

| Item | Value |
|---|---|
| Jira | EV-194 = backlog EVF-124 (E10, 8 SP, P0) |
| Tier | medium ([tier.json](tier.json)) — test tooling only |
| SLO source | [docs/00-tong-quan.md §5](../../00-tong-quan.md); epic criteria E5/E10 in [04-jira-backlog.md](../../04-jira-backlog.md) |
| Branch | `feature/k6-load-suite` from `origin/develop` 918667c |

## 1. Problem

`tests/load/opening-spike.js` is a sketch. It calls services with forged `X-Identity-*` headers (so it skips the gateway, which
strips them), it never goes past `POST /holds`, its `ticketing_oversell_detected` counter is never incremented, it sends
`signals:{}` (= "missing" for anti-bot, score 0.88 BLOCK under enforce), it has no profile small enough for a laptop or CI, and
it writes results to a path that is not gitignored. No workflow runs k6.

## 2. Scope

1. A k6 suite under `tests/load/` with one entry script and a `PROFILE` switch: `smoke`, `spike`, `wait`, `checkout`, `full`.
2. All traffic goes **through the gateway** with real `Authorization: Bearer` access tokens (no forged identity headers).
3. Scenarios: opening spike (join burst), waiting room (polling that honours `poll_after_ms` / `X-Poll-After-Ms` and
   `ETag`), checkout wave for admitted users (hold → order → payment → sandbox webhook → `ISSUED`) with idempotency keys.
4. Per-scenario thresholds that encode the SLOs, plus custom metrics.
5. Setup without OTP: load identities pre-seeded in Postgres, access tokens minted locally.
6. Pure helpers unit-tested with `node --test` (runs without Docker and without k6).

**Out of scope:** production code, compose/k8s config, CI workflow changes (D3), proving 100k VU on this machine, chaos
drill (EVF-126), anti-bot enforce-mode exemption itself (belongs to `antibot-phase1`, see D5).

## 3. Acceptance criteria

**AC-1 Smoke profile.** Given compose is up, migrated, and `tests/load/seed-load.sh` has run with defaults,
When `make load-smoke` runs, Then k6 exits 0 in < 2 minutes wall time, every threshold passes, and the run includes at least
one join, one status `304`, one hold, one payment, one webhook and one order observed `ISSUED`.

**AC-2 Profiles are parameterised.** Given `PROFILE` ∈ {smoke, spike, wait, checkout, full}, When env overrides are set
(`JOIN_TARGET`, `JOIN_RAMP`, `WAIT_VUS`, `WAIT_DURATION`, `CHECKOUT_RATE`, `CHECKOUT_DURATION`, `USERS`), Then the generated
`options.scenarios` use those values and the defaults of each profile are those in plan.md §3.
Given an unknown profile or a malformed value (non-positive number, bad duration), Then the script throws at init and k6
exits non-zero before sending any request.

**AC-3 Thresholds encode the SLOs.** Every profile declares, scoped by the `scenario`/`phase` tag:
`join p(99) < 300 ms`, `status p(99) < 150 ms`, `checkout p(99) < 800 ms` (hold + payment create, each), `join success rate >
99.5%`, `http_req_failed` (5xx and network errors only) `< 1%`, `checkout_holds_ok count <= QUOTA`, `idem_mismatch count == 0`.
The smoke profile may relax latency (compose on a laptop) via `SLO_SCALE` but never the count invariants.

**AC-4 Auth without OTP.** Given `JWT_SECRET` (default = compose dev secret) and seeded identity rows, When a VU starts,
Then it uses an HS256 access token it minted itself (`iss=eventflow-identity`, `aud=eventflow-api`, `typ=access`,
`ver=true`, `did`=its device id, `exp` ≥ run end + 5 min) and the run makes **zero** calls to `/api/v1/auth/*`.
Given `BASE` is not `localhost`/`127.0.0.1` and `ALLOW_REMOTE` ≠ `1`, Then the script refuses to start.
Given `seed-load.sh` runs twice, Then it is idempotent (same identities, no duplicate rows).

**AC-5 Polling honours the server.** Given a queued VU, When it receives `200`, Then it sleeps `poll_after_ms` from the body;
When it receives `304`, Then it sleeps `X-Poll-After-Ms`; it always sends `If-None-Match` with the last `ETag`; the next
poll is never earlier than the server value (± 0 ms, plus jitter ≥ 0). Metrics `wr_status_304_ratio` and
`wr_status_429` are recorded; threshold `wr_status_429 rate < 0.1%` (any steady 429 means the suite outpaces BR-Q4).

**AC-6 Checkout idempotency.** Every write (`POST /holds`, `POST /payments`, `POST issue-tickets` if used) carries
`Idempotency-Key` = `lt-{RUN_ID}-{identity}-{step}`. Given `REPLAY_RATIO` (default 0.1), When a sampled VU repeats the
same write with the same key, Then the response carries the same `hold_id` / `payment_id`; otherwise `idem_mismatch` += 1.
Hold `409` (sold out / limit) is a valid outcome, counted in `checkout_sold_out`, not a failure.

**AC-7 Anti-bot compatible payload.** With `SIGNALS=realistic` (default), each join body is a v1 snapshot that satisfies
every range in `antibot-phase1/contracts.md §1`: `v=1`, `rt`, `pe`, `se`, `ke`, `iv` (≥ 8 non-constant intervals),
`sq`/`sn` unique per identity, `fp` = the VU device id, `wd=false`, `gl=true`, `fc` ≥ 3, `tz`/`tzo` consistent.
`SIGNALS=min` sends `{"v":1}`; `SIGNALS=none` sends `{}` (to measure the "missing" path). Serialised size ≤ 2 KiB.

**AC-8 Spike shape.** In `spike`/`full`, Then joins use `ramping-arrival-rate` reaching `JOIN_TARGET` (default 100 000)
total arrivals within `JOIN_RAMP` (default 5 s), each VU with client jitter 0..`JITTER_MS` (default 5000, EVF-61);
`wr_join_arrivals` counts attempts and the summary prints achieved vs requested arrival rate and `dropped_iterations`.

**AC-9 30-minute wait.** In `wait`/`full`, Then `WAIT_VUS` VUs join once and poll for `WAIT_DURATION` (default 30m);
their access token does not expire mid-run (AC-4); an `ADMITTED` VU hands over to the checkout flow; an `EXPIRED` or
`SOLD_OUT` VU stops polling and is counted (`wr_expired`, `wr_sold_out`).

**AC-10 Oversell signal.** Given `QUOTA` (seeded tickets), Then `oversell_suspected` += 1 whenever the run's successful
distinct successful holds exceed `QUOTA` (computed in `handleSummary` from the `checkout_holds_ok` count; the threshold-visible flag is `checkout_holds_ok count <= QUOTA`), and `seed-load.sh verify` reports from Postgres
`issued + open holds ≤ quota` per ticket type (authoritative check; needs Docker).

**AC-11 Results recorded.** `handleSummary` writes `tests/load/results/{profile}-{RUN_ID}.json` and a text summary with
each SLO, its target and PASS/FAIL; `tests/load/results/` is gitignored; evidence for a run is copied into
`docs/workflow/k6-load-suite/evidence.md` by hand (profile, machine, commit, table).

**AC-12 Offline unit tests.** `node --test tests/load/lib` passes without Docker or k6 and covers: profile builder
(AC-2 values and errors), JWT encoder (fixed vector matches a token signed by `authx.SignAccessToken` with the same
claims, captured once into a fixture), signals generator (AC-7 ranges, uniqueness, size), poll-delay rule (AC-5),
idempotency key format (AC-6), remote-target guard (AC-4).

## 4. Decisions (need human approval)

| # | Decision | Why |
|---|---|---|
| D1 | Mint access tokens in k6 (HS256, `JWT_SECRET`) instead of logging in each VU | Login is limited to 1 rps/IP (burst 10) and OTP goes by email; 100k logins would test identity, not the sale. Gateway verification is stateless, so minted tokens exercise the same path. |
| D2 | Seed `USERS` identities (`00000000-0000-4000-9000-{n}`) + a dedicated event via `tests/load/seed-load.sh` (reuses `deploy/scripts/seed.sh` for the event, inserts identities with `generate_series`) | `orders.identity_id REFERENCES identities`; the seed's 10 k6 buyers are too few; the `9000` range cannot collide with seed.sh (`8000`). |
| D3 | No CI job in this task; `make load-smoke` documented for local/nightly; proposal to add one step to `e2e-nightly.yml` as a follow-up | Changing workflows is a CI rule hit (re-triage); nightly already has compose up. |
| D4 | Sandbox webhook goes through the gateway but is paced by `WEBHOOK_RATE` (default 8/s) | The webhook route is public, so the limit is per IP (10 rps, burst 30) and compose does not trust `X-Forwarded-For`; a single k6 host would 429 itself. Full-scale runs need `RATE_DEFAULT` raised on the target or the webhook sent from separate hosts (Q2). |
| D5 | Default `SIGNALS=realistic`; the suite does not try to beat anti-bot | Under `ANTIBOT_MODE=enforce`, R4-network (> 30 identities / IP / 300 s) and R7 still flag one k6 host; load runs must use `shadow`/`off` or an exemption that `antibot-phase1` must provide (Q3). |
| D6 | Replace `tests/load/opening-spike.js` with `tests/load/main.js` + `lib/`; `make load` keeps its name and gains `PROFILE` | One entry, one set of thresholds; the old forged-header path is removed. |
| D7 | Oversell in k6 is a *suspicion* counter; the authoritative check is SQL (`seed-load.sh verify`) and `make test-oversell` | k6 sees only HTTP responses; a missing ISSUED or a retried hold must not be misread as oversell. |

## 5. Risks / cannot verify here

- This machine: Docker usually off, no k6 installed, cannot generate 100k VU (needs ~ several load hosts or k6 Cloud /
  distributed `k6-operator`). Only AC-2/4(guard)/5(rule)/6(format)/7/12 are verifiable offline; AC-1, AC-6 replay, AC-10
  need compose; AC-3 latencies, AC-8, AC-9 at scale need the k8s stack (EVF-120..122) and a large generator.
- Admission rate on compose is AIMD-limited; the smoke relies on the sale being open at seed time (`SALE_START_IN_SECONDS=0`).
- Gateway forwarding of `ETag` / `If-None-Match` / `X-Poll-After-Ms` is assumed; the smoke (AC-1 "one 304") proves it.
