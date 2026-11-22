# k6-load-suite — Plan (draft, step 2b)

Spec: [spec.md](spec.md). Tier: medium. No production code, no compose/k8s/workflow change.

## 1. Files

| File | Kind | Content |
|---|---|---|
| `tests/load/main.js` | k6 entry | reads `PROFILE`, exports `options` from `buildOptions()`, `setup()`, scenario functions `joinBurst`, `waitAndPoll`, `checkoutWave`, `teardown()`, `handleSummary()` |
| `tests/load/lib/profiles.js` | pure | `buildOptions(env)` → `{scenarios, thresholds}`; defaults table §3; validation errors (AC-2) |
| `tests/load/lib/jwt.js` | pure + injected HMAC | `accessClaims({sub, did, now, ttlS})`, `encodeJwt(claims, hmacFn)` — base64url header/payload; in k6 `hmacFn` = `k6/crypto.hmac('sha256', …, 'binary')`, in node = `crypto.createHmac` (AC-4) |
| `tests/load/lib/signals.js` | pure, seeded PRNG | `signalsFor(mode, identityIdx, deviceId, rng)` (AC-7) |
| `tests/load/lib/flow.js` | pure | `nextPollDelayMs(res)`, `idemKey(runId, identity, step)`, `identityFor(n)`, `deviceFor(n)`, `guardTarget(base, env)` (AC-4/5/6) |
| `tests/load/lib/api.js` | k6 | thin `http` wrappers: `join`, `status`, `hold`, `getOrder`, `pay`, `webhook`; tags `{phase}`; records metrics |
| `tests/load/lib/metrics.js` | k6 | Trend/Rate/Counter definitions (§4) |
| `tests/load/lib/summary.js` | pure | `renderSummary(data, quota)` → text table SLO/target/actual/PASS, oversell suspicion (AC-10/11) |
| `tests/load/lib/*.test.mjs` | node --test | AC-12 (written first, red) |
| `tests/load/lib/fixtures/access-token.json` | fixture | claims + secret + token produced once by `authx.SignAccessToken` (tiny Go test-helper run, output pasted; generator command recorded in the fixture) |
| `tests/load/seed-load.sh` | bash | `seed` (default): `deploy/scripts/seed.sh` with `EVENT_ID=$LOAD_EVENT RESET=1 QUOTA SALE_START_IN_SECONDS=0`, then `INSERT … generate_series(1,$USERS) ON CONFLICT DO NOTHING` identities `00000000-0000-4000-9000-{n:012}` role buyer, `otp_verified_at=now()`, `password_hash='!'`; prints `EVENT`, `TICKET_TYPE`, `QUOTA`. `verify`: SQL issued + open holds ≤ quota per ticket type, exit 1 otherwise |
| `tests/load/opening-spike.js` | deleted | replaced (D6) |
| `Makefile` | edit | `load` → `k6 run -e PROFILE=$(PROFILE) … tests/load/main.js`; new `load-smoke` (seed-load + PROFILE=smoke), `load-seed`, `load-verify`, `test-load-lib` (`node --test tests/load/lib`) |
| `.gitignore` | edit | `tests/load/results/` |

k6 has no npm install: `lib/*.js` are ES modules without k6 imports (except `api.js`, `metrics.js`), so node can import
them. No new package.json dependency.

## 2. Flow per VU

1. Identity `n` = global VU-iteration index (`exec.scenario.iterationInTest` offset per scenario so scenarios never share
   identities); `id = identityFor(n)` (`n ≤ USERS`, else abort with a clear message), `did = deviceFor(n)`.
2. Token minted lazily per VU (not in `setup()`: setup data is copied to every VU). `exp = now + runSeconds + 300`.
3. `join` body `{signals: signalsFor(...)}`; `Authorization: Bearer`, `Content-Type`, `User-Agent` realistic browser
   string. Expect 200 with `queue_token` or `state=SOLD_OUT`.
4. `status` loop: send `X-Queue-Token`, `If-None-Match`; delay = `nextPollDelayMs` (body `poll_after_ms` on 200,
   header `X-Poll-After-Ms` on 304, `Retry-After`×1000 on 429, floor 1000 ms when missing) + jitter 0..250 ms.
   Stop on `ADMITTED` (→ checkout), `EXPIRED`, `SOLD_OUT`, or scenario end.
5. Checkout: `POST /v1/events/{ev}/holds` (`ticket_type_id`, quantity 1, `X-Queue-Token`, `Idempotency-Key`) →
   `GET /v1/orders/{id}` (amount) → `POST /v1/events/{ev}/payments` (`method:"sandbox"`) →
   `POST /v1/payments/webhook/sandbox?…&status=SUCCESS` (paced, D4) → poll `GET /v1/orders/{id}` until `ISSUED`
   (max 20 s, every 1 s; counts `checkout_issued` / `checkout_issue_timeout`). Replay (AC-6) on `REPLAY_RATIO` sample.
6. `setup()`: `GET /healthz`, `GET /v1/events/{ev}/availability` until `AVAILABLE` (≤ 30 s, ticketing-worker loads Redis
   every 5 s) and take `TICKET_TYPE` if not given; fail fast otherwise. No auth calls.

## 3. Profiles (defaults; every value overridable by env)

| Profile | Scenarios | Defaults | Wall time |
|---|---|---|---|
| `smoke` | join (shared-iterations 20 VU × 1) → wait+poll (same VUs, ≤ 60 s) → checkout | `USERS=50`, `QUOTA=200`, `SLO_SCALE=4` (latency targets ×4 on a laptop) | < 2 min |
| `spike` | `joinBurst`: ramping-arrival-rate 0 → `JOIN_TARGET/JOIN_RAMP` per s over `JOIN_RAMP`, tail 25 s at 2%, `preAllocatedVUs` = rate, `maxVUs=JOIN_MAX_VUS` | `JOIN_TARGET=100000`, `JOIN_RAMP=5s`, `JITTER_MS=5000` | ~40 s |
| `wait` | `waitAndPoll`: constant-vus | `WAIT_VUS=20000`, `WAIT_DURATION=30m` | 30 min |
| `checkout` | `checkoutWave`: constant-arrival-rate of pre-joined identities polling to ADMITTED | `CHECKOUT_RATE=200/s`, `CHECKOUT_DURATION=5m`, `WEBHOOK_RATE=8` | 5 min |
| `full` | spike at t=0, wait from t=10 s, checkout from t=60 s, all thresholds | union | ~31 min |

`SLO_SCALE` is 1 for every profile except smoke; count invariants (holds ≤ QUOTA, idem_mismatch == 0) never scale.

## 4. Metrics and thresholds

Trends: `wr_join_latency`, `wr_status_latency`, `co_hold_latency`, `co_payment_latency`, `co_issue_wait`.
Rates: `wr_join_success`, `wr_status_304_ratio`, `wr_status_429`, `http_req_failed` via `responseCallback`
(`expectedStatuses(200, 201, 204, 304, 409)`, so 409 sold-out and 304 are not failures).
Counters: `wr_join_arrivals`, `wr_admitted`, `wr_expired`, `wr_sold_out`, `checkout_holds_ok`, `checkout_sold_out`,
`checkout_issued`, `checkout_issue_timeout`, `idem_mismatch`.
Thresholds (per scenario via `{scenario:…}` tags): join p99 < 300·S, status p99 < 150·S, hold/payment p99 < 800·S,
join success > 0.995, 429 rate < 0.001, http_req_failed < 0.01, `checkout_holds_ok count<=QUOTA`, `idem_mismatch count==0`,
smoke only: `wr_admitted count>=1`, `checkout_issued count>=1`, `wr_status_304_ratio rate>0` (AC-1). `abortOnFail` only
for the count invariants.

## 5. How to run

```bash
make up && make migrate
make load-smoke                                   # seed-load + PROFILE=smoke, < 2 min
make load PROFILE=spike JOIN_TARGET=5000          # scaled-down spike on a laptop
make load-verify                                  # authoritative oversell check in Postgres
docker run --rm -i --network host -v "$PWD:/w" -w /w grafana/k6:<pinned> run -e PROFILE=smoke tests/load/main.js  # no local k6
```
Full scale: k6 on ≥ N load hosts (or `k6-operator` on the k8s cluster, EVF-120..122) with `ALLOW_REMOTE=1`,
`JWT_SECRET` of that environment from a secret store (never committed), `ANTIBOT_MODE=shadow` (D5), raised
`RATE_DEFAULT` for the webhook or a separate webhook host (D4). Use `USERS` ≥ JOIN_TARGET + WAIT_VUS.

## 6. Results

`handleSummary` → `tests/load/results/{profile}-{RUN_ID}.json` (gitignored) + stdout table. A run worth keeping is copied
into `evidence.md` (commit, profile, env overrides, host spec, table, `seed-load.sh verify` output).

## 7. CI

None in this task (D3). Follow-up proposal: one step in `e2e-nightly.yml` after `make demo`:
`grafana/setup-k6-action` pinned by SHA + `make load-smoke` (+ `make load-verify`). Requires re-triage (CI rule hit).
`make test-load-lib` can join the existing node job in `ci.yml` in the same follow-up.

## 8. Order of work (TDD)

1. `*.test.mjs` for profiles, jwt (fixture), signals, flow, summary — run, confirm red (modules missing).
2. Implement pure libs until green.
3. `metrics.js`, `api.js`, `main.js`; `k6 inspect tests/load/main.js` per profile (via Docker image) to check options.
4. `seed-load.sh`, Makefile, .gitignore, delete `opening-spike.js`.
5. With Docker: `make load-smoke` ×2 (idempotent seed), `make load-verify`; record evidence.

## 9. Verifiable where

| Check | Offline (this machine) | Compose (Docker on) | k8s + large generator |
|---|---|---|---|
| AC-2, AC-4 guard/claims, AC-5 rule, AC-6 key, AC-7, AC-12 | yes (`make test-load-lib`) | – | – |
| AC-1, AC-6 replay, AC-10 verify, AC-11 | – | yes | – |
| AC-3 at SLO scale, AC-8 100k/5 s, AC-9 30 min × 20k | – | scaled down only | yes (not in this task) |
