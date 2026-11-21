# k6 load suite (EV-194 / EVF-124)

One entry script, `tests/load/main.js`, with a `PROFILE` switch. Spec and decisions:
[docs/workflow/k6-load-suite/spec.md](../../docs/workflow/k6-load-suite/spec.md), plan: [plan.md](../../docs/workflow/k6-load-suite/plan.md).

| Profile | What it does | Defaults |
|---|---|---|
| `smoke` | 20 VUs: join -> poll -> hold -> payment -> sandbox webhook -> `ISSUED` | `USERS=50 QUOTA=200 SLO_SCALE=4`, < 2 min |
| `spike` | opening spike: `JOIN_TARGET` joins within `JOIN_RAMP` (ramping-arrival-rate), jitter 0..`JITTER_MS` | `100000` in `5s`, jitter `5000` |
| `wait` | `WAIT_VUS` VUs join once and poll for `WAIT_DURATION`, honouring `poll_after_ms` / `X-Poll-After-Ms` / `ETag` | `20000` for `30m` |
| `checkout` | `CHECKOUT_RATE` new buyers per second join, wait for `ADMITTED`, buy | `200/s` for `5m` |
| `full` | spike at 0 s, wait from 10 s, checkout from 60 s | union |

All traffic goes **through the gateway** with an HS256 access token that each VU mints from `JWT_SECRET` (default: the
compose dev secret). No `/api/v1/auth/*` call is made; the identities `00000000-0000-4000-9000-{n}` are seeded by
`seed-load.sh`. A `BASE` other than `localhost` / `127.0.0.1` / `[::1]` is refused unless `ALLOW_REMOTE=1`.

## Requirement: anti-bot in shadow mode

Run the target with **`ANTIBOT_MODE=shadow`** (or `off`). A single k6 host sends thousands of identities from one IP,
which anti-bot rule R4-network reads as an account farm; under `enforce` those joins are slowed or blocked and the
run measures the bot filter, not the sale (spec D5). The suite still sends realistic v1 signals (`SIGNALS=realistic`,
default) so shadow decisions stay meaningful; `SIGNALS=min` sends `{"v":1}`, `SIGNALS=none` sends `{}`.

## Run

```bash
make up && make migrate
make load-smoke                                   # seed-load + PROFILE=smoke
make load-verify                                  # authoritative oversell check in Postgres
make test-load-lib                                # offline unit tests of the pure helpers (node, no k6)

# bigger profiles: seed enough identities first (k6 refuses to start if USERS is too small and prints the need)
make load-seed USERS=6000 QUOTA=2000
make load PROFILE=spike JOIN_TARGET=5000 USERS=6000 QUOTA=2000
```

No local k6: use the image (pin a version, e.g. `grafana/k6:0.57.0`):

```bash
make load K6="docker run --rm -i --network host -v $PWD:/w -w /w grafana/k6:0.57.0"
```

Every env value can be passed as a make variable: `PROFILE BASE EVENT TICKET_TYPE JWT_SECRET ALLOW_REMOTE RUN_ID USERS
QUOTA SLO_SCALE SIGNALS JOIN_TARGET JOIN_RAMP JOIN_MAX_VUS JITTER_MS WAIT_VUS WAIT_DURATION CHECKOUT_RATE
CHECKOUT_DURATION WEBHOOK_RATE REPLAY_RATIO`. A bad value (unknown profile, `0`, `5` instead of `5s`, ...) stops k6 at
init before any request.

## Thresholds (AC-3)

`join p(99) < 300 ms`, `status p(99) < 150 ms`, `hold` and `payment p(99) < 800 ms` (each multiplied by `SLO_SCALE`),
`join success > 99.5%`, `http_req_failed < 1%` (5xx and network errors only), `status 429 rate < 0.1%`,
`checkout_holds_ok count <= QUOTA` and `idem_mismatch count == 0` (never scaled, abort the run on failure).
Smoke also requires at least one `ADMITTED`, one `ISSUED` and one `304`.

## Results

`tests/load/results/{profile}-{RUN_ID}.json` and `.txt` (gitignored) plus the table on stdout: each SLO, target,
actual, PASS/FAIL, `oversell_suspected`, join arrivals, achieved vs requested arrival rate, `dropped_iterations`.
k6 only sees HTTP, so `oversell_suspected` is a suspicion; `make load-verify` is the real check (spec D7). Copy a run
worth keeping into `docs/workflow/k6-load-suite/evidence.md` (commit, profile, overrides, machine, table, verify output).

## Full scale (not on a laptop)

100k joins in 5 s needs several load hosts or `k6-operator` on the k8s stack. Use `ALLOW_REMOTE=1`, the target's
`JWT_SECRET` from a secret store (never commit it), `ANTIBOT_MODE=shadow`, `USERS >= JOIN_TARGET + WAIT_VUS + ...`,
and either a raised `RATE_DEFAULT` on the target or a separate webhook host: the sandbox webhook is public and rate
limited per IP, so `WEBHOOK_RATE` (default 8/s) paces it (spec D4).

## Files

- `main.js` - k6 entry: options, setup, scenarios `smokeFlow`, `joinBurst`, `waitAndPoll`, `checkoutWave`, summary.
- `lib/profiles.js`, `lib/jwt.js`, `lib/signals.js`, `lib/flow.js`, `lib/summary.js`, `lib/rng.js` - pure logic, no
  `k6/*` import, unit-tested by `lib/*.test.mjs`.
- `lib/api.js`, `lib/metrics.js` - k6-only HTTP wrappers and metrics.
- `lib/fixtures/access-token.json` - token signed by Go `authx.SignAccessToken`; the encoder must match it byte for byte.
- `seed-load.sh` - `seed` (event via `deploy/scripts/seed.sh` + identities) and `verify` (Postgres oversell check).
