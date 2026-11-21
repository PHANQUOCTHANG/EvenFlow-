# Plan review: antibot-phase1 (v3 step 2b, round 1)

Reviewer: plan reviewer, clean context. Inputs: `spec.md`, `contracts.md`, `plan.md`, `tier.json` at 0e199a6, plus the
code they reference on develop @ 918667c (waitingroom handler/queue/Lua, cmd/server, postgres audit, migrations 0001,
gateway proxy/clientip, web waiting-room / event-action-panel / queue-client / device, deploy/k8s, libs/go).

## Verdict: **FAIL** (round 1)

3 blockers, 6 majors, 12 minors. The architecture (in-process library, one script on the `{ev}` slot, shadow default,
no migration, no new dependency) is sound. It fails on three points: missing or regular-but-human signals reach BLOCK
for real buyers, a merge would stop waitingroom from starting in staging, and the corpus gate passes by construction.

## What holds (checked)

- **One slot.** `ab:{ev}:*` and `wr:{ev}:*` use the same hash tag, so they land on the same Redis Cluster slot. One
  `observe.lua` call per join is feasible. That also lets a script read or write both key spaces, which the fixes for
  P-1 and P-12 rely on.
- **Client IP.** The gateway's `Rewrite` strips client-sent IP headers and sets `X-Forwarded-For` to a single element
  (the resolved client, or the peer). Taking the first element in waitingroom is correct. Staging and local set
  `TRUSTED_PROXY_CIDRS=10.0.0.0/8`.
- **No migration.** The `bot_decisions` schema matches contracts 5: `action` CHECK allows
  `ALLOW/CHALLENGE/SLOW_LANE/BLOCK`, `score NUMERIC(4,3)`, `fired_rules TEXT[]`, `ip INET`, `device_fp TEXT`.
- **No new dependency.** go-redis, pgx and prometheus are already direct dependencies of both modules. The
  testcontainers postgres/redis modules are already indirect in waitingroom `go.mod`. `time/tzdata` is stdlib.
- **BLOCK cuts checkout.** Ticketing's `IsAdmitted` reads `wr:{ev}:admitted` (`ticketing/.../redis/gate.go:134`), so
  `revoke.lua` removing the ZSET member really cuts checkout. Status becomes `UNKNOWN` as AC-12 says.
- **Idempotency is kept.** `join.lua` returns an existing slot before any lane logic, so a later decision can only
  change the slot through BLOCK. This matches D5 and BR-Q3.
- **`off` and `shadow` responses are unchanged.** `httpadapter.New(store, nil, log)` with an untyped nil keeps
  develop's path (see P-15 for the typed-nil trap). In shadow, `Applied` is always PASS and the lane is always `n`.
- **Every AC maps to a plan step and a test** (AC-1..AC-19). Smaller gaps are in P-16.

## Findings

### P-1 (blocker): a legitimate re-join without a snapshot is BLOCKed, the buyer's slot is revoked and the BLOCK sticks for 24 h

`waiting-room.tsx` calls `joinQueue` on mount whenever sessionStorage holds no queue token. On a 401 from status it
calls `clearQueueToken`, then re-joins (lines 117-124). The plan puts `clearSignals` next to `clearQueueToken`. So the
automatic 401 re-join (token expired, or a token from before EV-137) sends `{"v":1}`, which counts as missing.
Missing fires R3 and R5 (strong), giving 0.88 BLOCK. Under enforce that 403s the buyer, revokes the slot they
already hold and sets the 24 h sticky flag. There is no unblock in phase 1 (EV-177 is out of scope).

The same thing happens in every case where sessionStorage is empty but the identity already has a slot:

- a second tab, or opening `/waiting/{eventId}` directly (sessionStorage is per tab);
- another device, which is exactly the BR-Q3 re-join that `join.lua` documents;
- a browser restart.

AC-1's "also on re-join" is therefore false for most real re-joins.

Fix:

1. Do not clear the snapshot in the re-join path. Clear it only on checkout or on leaving the event.
2. Make the decision slot-aware. `observe.lua` already runs on the same slot, so it can also return
   `HEXISTS wr:{ev}:tok identity`.
3. When the identity already holds a slot, a decision whose fired rules are only "signals missing" (R3 and R5 from
   the missing case) must not BLOCK or revoke. Return the existing slot (BR-Q3) and persist the row with a new tag
   such as `REJOIN`.
4. Add a handler test and a web test for the 401 re-join path and for "existing slot + `{v:1}`".
5. Add a mutation: "missing signals on re-join revokes".

Also consider capping missing-only at SLOW_LANE for new joins, so BLOCK needs at least one positive sign (UA deny,
`wd`, R1, R4 or R7). An old cached bundle then costs a demotion instead of a 24 h ban. Record the choice as a
spec decision.

### P-2 (blocker): pointermove sampling makes R1-client and R7 fire for ordinary mouse users; R1+R7 = 0.85 = BLOCK

Contracts 1 samples pointermove "at most 1 per 100 ms" and feeds sampled events into `iv` and `sq`. A human moving
the mouse steadily towards the button for 3 s produces about 30 intervals of about 100-110 ms, a CV well under 0.10,
so R1-client fires (0.50). The same run digests to `pointermove:2` repeated, ending in `pointerdown/pointerup`.
That low-entropy `sq` is shared by thousands of desktop buyers within 15 min, so R7 fires (0.70, SLOW_LANE on its
own). Together that is `1 - 0.5*0.3 = 0.85`, which is BLOCK plus sticky. Mobile taps have the same problem
(touchstart/pointerdown/pointerup at about 0/100 ms buckets).

The synthetic humans ("jittered intervals") do not model the sampler, so AC-18 cannot see this (see P-7).

Fix:

1. Keep sampled pointermove and scroll out of `iv` and `sq`. Use only discrete events (pointerdown/up, keydown,
   touchstart, the first scroll of a burst).
2. Give `sq` an entropy floor (for example >= 12 discrete events and >= 4 distinct buckets) and include `rt`'s bucket.
3. Lower R7's weight below the SLOW_LANE cut-off on its own (for example 0.50), or require R7 together with a
   server-side rule.
4. Add web tests that drive a 3 s steady mousemove and a mobile tap and assert R1 and R7 do not fire on the decoded
   snapshot.
5. Add the corresponding families to the human corpus (see P-7).

### P-3 (blocker): staging would crash-loop waitingroom on merge (default `shadow` + `APP_ENV=production` + no `ANTIBOT_HASH_KEY`)

`deploy/k8s/overlays/staging/app-config.env` sets `APP_ENV=production`. `base/waitingroom/deployment.yaml` has no
`ANTIBOT_HASH_KEY`. AC-16 makes the default mode (`shadow`, not `off`) fail at startup without the key. The plan
changes only `deploy/compose/docker-compose.yml`. The rollout claim "a merge alone never denies a buyer" is
therefore false: the new waitingroom pods never become ready.

Fix:

1. Add `ANTIBOT_HASH_KEY` to the `eventflow-auth` Secret contract: the staging kustomization header comment, the
   `secretKeyRef` in the waitingroom deployment, and `overlays/local/secrets.dev.env`.
2. Add `ANTIBOT_MODE: shadow` to the app config.
3. Make sure `deploy/k8s/policy/check_policy.py` still passes.
4. Add a rollout step: create the Secret key before applying.
5. Add a gate step: `kustomize build overlays/staging | grep ANTIBOT_HASH_KEY`.

### P-4 (major): the plan's signature changes break frozen tests at compile time; the compatibility design must be fixed now, not "at red check"

Frozen files implement or call the 4-argument API:

- `handler_test.go` `fakeStore.Join(ctx, ev, id, token)`;
- `test/queuetoken` `store.Join` + `q.Join(ctx, ev, id, tok)`;
- `test/soldout` and `test/opsapi` call `q.Join(..., token)`.

Plan 2 changes `QueueStore.Join` and `Queue.Join` to take a `lane`, so all four packages fail to compile.

Fix:

1. Keep both `Join` signatures.
2. Add `(*Queue).JoinLane(ctx, ev, id, tok, lane, penalty)` and `(*Queue).Revoke`.
3. In the handler, use an optional interface `LaneStore interface{ JoinLane(...); Revoke(...) }` found by type
   assertion. If the store does not implement it, SLOW_LANE falls back to `Join` with a warning and BLOCK still
   answers 403 without a revoke.
4. Make `join.lua` treat a missing `ARGV[5]`/`ARGV[6]` as lane `n`.
5. Removing `BotGuard.Check` is safe: no test implements it.

### P-5 (major): `shuffle.lua` returns early when the main lobby is empty, stranding the slow lobby forever

`shuffle.lua` does `if n == 0 then HSET shuffled 1; return 0 end` before any later step. If only slow-lane members
joined before T0 (a small event, or a heavy attack early on), `lobby_slow` is never placed. Its members stay `LOBBY`
for the session TTL: `join.lua` keeps returning `LOBBY` and `status.lua` reports `LOBBY`.

Fix:

1. Process `lobby_slow` in both branches.
2. Add a lane test where the main lobby is empty and the slow lobby is not.
3. Note that the controller binary (a separate Deployment, same image) runs `shuffle.lua`. Enforce must only be
   switched on after both waitingroom and waitingroom-controller run the new scripts. Add this to the rollout.

### P-6 (major): R4-network alone reaches SLOW_LANE; carrier-grade NAT puts far more than 30 real buyers on one /24 in an on-sale

R4 (device or network) has a single weight of 0.60, which equals the SLOW_LANE cut-off. Vietnamese mobile carriers
use CGNAT: one public /24 serves thousands of subscribers. In a 300k-join on-sale, 30 distinct identities on one
/24 within 5 min is the normal case for mobile users, not a bot signal. The same happens if an ingress or LB in
front of the gateway is not covered by `TRUSTED_PROXY_CIDRS` (every buyer then shares one IP). The corpus uses
"campus groups of 25", just under the threshold of 30.

Fix:

1. Split the rule into `R4D` (device, keep 0.60) and `R4N` (network, for example 0.25: a contributor, never a tier
   on its own).
2. Add corpus humans in CGNAT groups of 200-2000 identities per /24, and a "single LB IP" scenario.
3. Add a metric or alert for a top-prefix share. For example, when one prefix carries more than 5% of an event's
   joins, R4N is not evaluated for that prefix.

### P-7 (major): the offline corpus is self-fulfilling; AC-18 and AC-19 pass by construction

The same author generates bots and humans, with full knowledge of thresholds and weights. Every human hard case
sits just inside a threshold:

- `rt` p0.1 >= 150 ms against R2's 120 ms;
- fast clickers capped at exactly 1%;
- households of 2-3 against R4-device's > 3;
- campus groups of 25 against R4-network's > 30;
- travellers with a tz mismatch alone (one weak sign);
- privacy browsers with exactly 2 weak signs (R5W alone is CHALLENGE, which passes).

Bots get exactly the signs the rules look for. "< 1% humans demoted" then measures the generator, not the engine.
AC-19's "human replay from the corpus" is the same data again. P-1, P-2 and P-6 are three real false-positive
classes this corpus cannot detect.

Fix:

1. Define human generator parameters independently of `contracts 3`. Write them in `testdata/corpus/README.md`
   with their source (published UX timing data or measured sessions).
2. Add populations on both sides of each threshold: CGNAT, steady mouse, mobile tap, Android low font count,
   households of 4-5, multi-tab and other-device re-join.
3. Add 50-100 real sessions recorded by the team with the actual collector (dev build, any browser) as a separate
   `humans_recorded.jsonl` with its own < 1% assertion.
4. Report per-threshold sensitivity (demotion rate at threshold ±20%).
5. Keep the real gate where D8 puts it: the shadow demotion rate on real traffic before enforce. State in the
   report that AC-18 is a regression check, not evidence of a low FP rate.

### P-8 (major): strict typed decoding lets one wrong-typed field invalidate the whole payload, and a float `rt` would do that for every buyer

Plan 2 decodes the `signals` value into a typed struct with `json.Decoder`. In `encoding/json`, one type mismatch (a
float into an `int` field) fails the whole value. AC-3 then counts the payload as missing, giving R3+R5 = 0.88
BLOCK. `rt` and `iv` come from `performance.now()` differences, which are floats unless the web rounds them. One
forgotten `Math.round` would BLOCK every buyer under enforce. AC-3 says "each out-of-range field is treated as
absent" but is ambiguous for wrong types.

Fix:

1. Decode each field independently (`map[string]json.RawMessage` or `json.Number` per field, capped to the known
   keys) so that a wrong type or a non-integer makes only that field absent. Reserve "missing" for an over-cap body,
   bad JSON, a non-object `signals`, or a bad `v`.
2. Accept integral floats (`123.0`).
3. Add a shared fixture (`testdata/signals/*.json`) produced by the web collector test and decoded by
   `signals_test.go`, so the client/server contract is tested end to end.

### P-9 (major): no system-level performance gate, and the degraded path can amplify a Redis incident

AC-5 measures `Decide` alone, sequentially, against an idle container. In production, `observe.lua` (about 15-20
commands: ZADD, ZREMRANGEBYSCORE, ZREMRANGEBYRANK, ZCARD, EXPIRE × 4 keys, plus the list) runs on the **same
single-threaded shard** as `join.lua` for that event. That roughly doubles Redis CPU per join on the hottest shard
of the on-sale.

go-redis also discards a connection after a read timeout. With a 4 ms timeout during a latency spike, every timed-out
call closes a connection and the next one re-dials. That is a reconnect storm against the Redis that is already slow.

Fix:

1. Configure the dedicated client explicitly: `MaxRetries: -1`, a bounded `PoolSize` (for example 128) with
   `MinIdleConns`, and `DialTimeout` within the budget.
2. Add a circuit breaker in `Guard`: after N degraded calls in 1 s, skip `Observe` for 2-5 s (stateless only,
   `DEGRADED` reason `breaker_open`).
3. Add a concurrent variant of `TestDecideP99` (for example 64 goroutines).
4. Add a gate: run `tests/load/opening-spike.js` against develop and against this branch in `shadow`, and compare
   join p99 and Redis CPU (budget: join p99 +5 ms, Redis CPU +X%, written in the gate).

### P-10 (minor): decode is outside the panic recovery, and the decode signatures disagree

Contracts 2 has `DecodeSignals(body []byte)`, plan 2 has `DecodeSignals(raw json.RawMessage)`, and the guard row
says "decode already done by caller". AC-5 counts decode inside `Decide`. A panic in decode would reach
`httpx.Recover` and return a 500, which refuses the join.

Fix: pass the raw `signals` bytes in `Input` and decode inside `Decide`, under the same `recover()`. Fix the
signature in both documents.

### P-11 (minor): D7 contradicts AC-16

D7 says "a panic or config fault: PASS". AC-16 says an invalid config fails startup. Reword D7 to "config is
validated at startup (AC-16); a runtime panic → PASS".

### P-12 (minor): BLOCK and sticky are not atomic with join

`MarkBlocked` runs on the antibot client (4 ms) after `revoke.lua`. A concurrent join of the same identity that
observed before the flag existed can create a slot after the revoke. A failed `MarkBlocked` leaves no sticky flag.

Fix: have `revoke.lua` set `ab:{ev}:blk:{id}` itself (same slot, see "What holds"), and have `join.lua` check that
key for new slots (refuse). Drop the separate `MarkBlocked` call from the handler path.

### P-13 (minor): pre-T0 slow-lobby members interleave with post-T0 slow joiners

Slow-lobby member i gets `1e12+P+i`; a post-T0 slow joiner with seq s gets `1e12+s+P`. A slow joiner arriving after
T0 can therefore rank ahead of a slow member who joined before T0. This is the reverse of the main lane, where every
lottery winner is ahead of every post-T0 joiner.

Fix: store `slow_lottery_size` m in meta at shuffle and score post-T0 slow joins as `1e12+seq+P+m`. Alternatively,
state and test the interleaving as intended.

### P-14 (minor): `bot_decisions` writer details

1. `identity_id` is UUID. A non-UUID identity, which tests and k6 use (`id-0001`), makes `CopyFrom` fail the whole
   500-row batch. Map non-UUIDs to NULL as for `event_id`, or drop the row and count it.
2. k8s uses `DATABASE_URL_POOLED` (PgBouncer). pgx v5 `CopyFrom` runs a describe before COPY. Run it inside a
   transaction, or use `QueryExecModeSimpleProtocol` with a multi-row INSERT.
3. `NewAuditorFromEnv` returns `(app.Auditor, func())` and has no pool accessor, and `audit_test.go` is frozen. Add
   a new `NewPoolFromEnv` constructor instead of changing it. The audit pool has `MaxConns=2`: the writer needs its
   own pool or a conn budget.
4. Recover panics in the writer goroutine (a panic there kills the process).
5. Specify the score stored for `STICKY` rows.

### P-15 (minor): the typed-nil trap in main for `off`

If main builds `var g *antibot.Guard` and passes it as `BotGuard` when mode is `off`, the interface is non-nil and
the handler calls into it.

Fix: extract `buildGuard(cfg, ...) (httpadapter.BotGuard, func())` in `cmd/server`, returning an untyped nil for
`off`, with a unit test.

### P-16 (minor): test gaps against the ACs

- AC-12: an integration test of the sticky flag through the real `observe.lua` plus the block flag (TTL 86400 s).
- AC-14: the PASS 1% sample rate (seeded RNG); every BLOCK row has a non-empty `fired_rules`.
- AC-15: assert that the Redis keys created by `observe.lua` and the slog output of a decision contain no raw fp, UA
  or full IP (the plan tests DB columns only).
- AC-17: a test that the 7 metric names are gathered from `prometheus.DefaultGatherer` (main serves
  `promhttp.Handler()`).
- The "injected 20 ms Redis delay" needs a mechanism with no new dependency: an in-test TCP proxy (`net.Listen`
  plus a delayed copy), or `DEBUG SLEEP` from a second client. Name it in the plan.
- AC-11: an `IsNew`/token-equality test per lane across T0 (joined slow before T0, re-join after the shuffle).
- `deploy/observability/alerts_test.yml` is an existing file: add test cases only, never edit existing ones.

### P-17 (minor): R4-device should key on the session-bound device id, not the body `fp`

Contracts 1 notes that `fp` in the body equals `X-Device-Id`. A bot can rotate the body `fp` on every request
without logging in again. `authx.Principal.DeviceID` is bound at login and `QUEUE_REQUIRE_DEVICE` defaults to true.

Fix: hash `p.DeviceID` when present and fall back to the body `fp`; tag the source.

### P-18 (minor): Android font count

The 20-font canvas probe is Windows/macOS oriented. Android Chrome, the majority of buyers here, may detect fewer
than 3 fonts, so `fc < 3` is a weak sign for most Android users; with `gl` false (low-end GPU blocklist) that
already gives R5W.

Fix: choose the font list per platform family, or do not evaluate `fc` when the UA is mobile. Add an Android
profile to the corpus.

### P-19 (minor): rollback limits

Revoked slots cannot be restored by switching to `off` or reverting. A falsely blocked buyer re-joins at the tail.
State this in plan 7 and in the enforce go/no-go note. Leftover `ab:{ev}:blk` keys are inert in `off`.

### P-20 (minor): `botsim` cannot import `libs/go/antibot/internal/corpusgen`

That path is internal to the libs/go module and invisible to the waitingroom module.

Fix: keep `botsim` self-contained and read `libs/go/antibot/testdata/corpus/*.jsonl` by relative path, or move the
generator to a non-internal `antibot/corpus` package.

### P-21 (minor): config and scoring edge cases

- Compute the tier from the rounded score, so the 0.85 boundary case R1+R7 is stable.
- State whether a partial `ANTIBOT_WEIGHTS` merges with the defaults or zeroes the IDs it leaves out.
- Cache only valid time zones in the tz `sync.Map`; invalid names are rejected without caching.
- Validate all `ANTIBOT_RULES` keys as integers where they are counts.

## Required for round 2

Resolve P-1, P-2, P-3 (blockers) and P-4 to P-9 (majors) in spec, contracts and plan. Put the P-1 decision and the
missing-signals cap, and the P-6 weight split, in spec section 2 as decisions for human approval, since they change
default outcomes in contracts 3. Minors can be folded in or explicitly deferred with a reason.

---

## Round 2

Inputs: spec.md Amendment 1 (approved in 46bb054, including the D16 join button and the k8s changes), contracts.md
revision 2 and plan.md round 2 (9eb3bd9). I re-checked them against the same code, plus `waiting-room.test.tsx`,
`waiting-room.wp2.test.tsx`, `deploy/k8s/policy/` and `migrations/0001_core.sql:17-24`.

### Verdict: **FAIL** (round 2): 0 blockers, 1 new major (N-2), 6 new minors

All 3 round-1 blockers and all 6 round-1 majors are resolved. The verdict is FAIL because of one new major
introduced by the revision: the R4N suppression it adds cannot trigger at on-sale volume (N-2). The fix is local to
`observe.lua` and its test, so round 3 only needs to re-check N-2.

### Round-1 findings re-checked

| Finding | Status | Notes |
|---|---|---|
| P-1 blocker | **resolved** | D11 caps missing-only at SLOW_LANE (`CAPPED`). D12 returns the existing slot for any non-BLOCK decision when `hasSlot`. D16 keeps the snapshot on the 401 re-join. `revoke.lua` sets the flag, and `join.lua` (lane path) enforces it even when antibot is degraded. With the breaker open, `hasSlot` is unknown, but a missing-only SLOW_LANE goes to `JoinLane(s)` and `join.lua` still returns the existing slot first, so that case is safe. A slot is revoked only by a positive-sign BLOCK. |
| P-2 blocker | **resolved** | `iv` and `sq` use only discrete events; the `sq` entropy floor (12 events, 4 buckets) means most humans send no `sq`; R7 is now 0.50 and R1+R7 is 0.75. Residual: keydown auto-repeat (a held key, about 30 ms) gives a regular `iv` and fires R1 (0.50 alone, CHALLENGE). That is acceptable; add it to the corpus. |
| P-3 blocker | **resolved**, see N-5 | Secret key, app config, rollout step 1 and the `kustomize build` grep are all in. |
| P-4 major | **resolved** | 4-argument `Join` and `New(store, nil, log)` are kept; `LaneStore` is found by type assertion; the Lua args are optional. The `store` in `test/queuetoken` has no `LaneStore` and takes the tested fallback path. |
| P-5 major | **resolved** | Both shuffle branches place the slow lobby; mutation 13 covers it; the controller reads the penalty; AC-20 requires the same image for both deployments. |
| P-6 major | **weights resolved**; the new suppression is broken (N-2) | R4N at 0.25 cannot set a tier on its own. |
| P-7 major | **resolved** | AC-18 is restated as a regression check, with sourced parameters, populations on both sides of every threshold, recorded sessions with their own assertion, and a ±20% sensitivity table. The real gate is shadow on real traffic (AC-20, see N-4). |
| P-8 major | **resolved** | Per-field decoding, rounding, the four whole-payload reasons, and shared golden fixtures. |
| P-9 major | **resolved** | Client options are explicit (`MaxRetries -1`, pool 128, `DialTimeout` 50 ms, `PoolTimeout` = budget). There is a TCP-proxy delay test with a bounded dial count, a concurrent p99, and a load gate. Breaker semantics are sound: open time counts as degraded (so `AntibotDegraded` fires); the sticky flag is still enforced by `join.lua` while it is open; it is per pod with no half-open state, which is fine for 3 s. |
| P-10 … P-16, P-18 … P-21 | resolved | Checked briefly; each has a test or a mutation. |
| P-17 | **rejection accepted** | `devices.id` is a row id per `(identity_id, fingerprint)` (`UNIQUE` at 0001:24), so it never repeats across identities, and R4D keyed on it could never fire. Keeping the body `fp` is correct; the identity-signed fp hash stays a follow-up. |

**Contracts §3 table.** I recomputed all rows (0.88, 0.70, 0.60, 0.625, 0.50, 0.25, 0.55, 0.5875→0.588, 0.7525,
0.67, 0.75). They are correct, and every tier follows from the rounded score. For 0.588, either rounding direction
stays CHALLENGE.

**`CAPPED` / `REJOIN` order.** The order is sticky → evaluate → D11 cap → D12 (`hasSlot` && not BLOCK → PASS
`REJOIN`) → mode and fallback. It is consistent, and the mutations cover both new steps. R1-server ignores joins
made while the identity holds a slot, as intended.

### New findings

#### N-2 (major): R4N's "> 5% of joins" suppression can never trigger at on-sale volume

`netCount` comes from `ab:{ev}:net:{prefix}`, a ZSET trimmed to 64 members. The denominator is the event's join
count over 5 min, with `R4N_SHARE_MIN_JOINS=1000`. So the share is at most 64/1000 = 6.4%, and it is below 5% as soon
as the window holds more than 1280 joins. A real on-sale has around 10^5 joins.

The case the mechanism exists for (one LB or NAT IP in front of every buyer) is therefore never suppressed, and
`AntibotNetSuppressed` never fires. Every buyer carries R4N: one more weak sign gives CHALLENGE, two give
SLOW_LANE. The numerator (distinct identities) and the denominator (joins) are also in different units. A store test
with fewer than 64 identities would pass while production stays broken.

Fix:

1. Add per-prefix join buckets `ab:{ev}:nj:{prefix}:{minute}` (`INCR`, TTL 360 s, same slot) and compute
   share = prefix joins / event joins over the same 5 buckets. Keep the ZSET only for the distinct-identity count.
2. Add an integration test: 2000 joins from one prefix out of 20 000 must be suppressed; 500 out of 20 000 must not.
3. In spec §6, state that a farm holding more than 5% of joins from one /24 also suppresses R4N for itself. This is
   acceptable: R4D and R1 remain, and R4N is only 0.25.

#### N-1 (minor, process): the approved D16 button breaks 5 frozen web tests; file the test change request now

These frozen tests render `WaitingRoom` with sessionStorage cleared (no token, no snapshot) and expect an automatic
join on mount:

- `waiting-room.test.tsx:44`;
- `waiting-room.test.tsx:189`;
- `waiting-room.wp2.test.tsx:47`, `:58` and `:65`.

Under D16 all five fail, so the plan's conditional ("if an existing test asserts...") is certain.

D16 is human-approved, so the button stays. But the test change request must list these 5 tests, with the
replacement assertion (button shown, no request on mount, then the same assertions after a click), and get human
sign-off before the red check.

Note for the human: after D11 and D12 the button is no longer needed for safety. Auto-joining with `{"v":1}` scores
0.40 (pass), or `REJOIN` when a slot exists. Keeping auto-join would leave these tests untouched and save a click at
T0. Re-confirm this choice when signing the test change request.

#### N-3 (minor): `{"v":1}` is now valid-but-empty, which weakens the no-signals case

In round 1, `{"v":1}` counted as missing (0.88). Now a client sending `{"v":1}` with a browser UA scores 0.40
(pass), and adding `"pe":1` scores 0. This is within the stated limit (forged signals are out of scope, Q-1). Record
it in spec §6 as an explicit trade-off.

#### N-4 (minor): AC-20's shadow demotion rate is polluted by synthetic traffic

k6 and `demo.sh` send `{"signals":{}}`, which counts as missing and is CAPPED to SLOW_LANE. One load run makes the
rehearsal's demotion rate meaningless.

Fix: measure AC-20 over joins whose signals decode as valid (or exclude the load windows), and report the
missing-payload share separately.

#### N-5 (minor): the k8s policy contract is frozen and the policy fixtures are generated

`check_policy.py` AC-18 requires each process's secret env set to equal `docs/workflow/k8s-all/policy-contract.md`
§8, which is frozen. `testdata/gen_fixtures.py --check` also fails on drift.

Plan 2 must list:

1. a change request against the k8s-all policy contract §8 (waitingroom row and the `eventflow-auth` key set);
2. `check_policy.py`: the `waitingroom` map at line 86 and `EXTERNAL_SECRET_KEYS["eventflow-auth"]` at line 116;
3. `rules.md`;
4. `gen_fixtures.py`, plus regenerated `good-local.yaml`/`good-staging.yaml`, with every `bad/*.yaml` still tripping
   exactly its own rule.

No `ANTIBOT_*` key collides with the AC-32 forbidden list.

#### N-6 (minor): R3 for absent counts contradicts spec §3

Contracts 1 says `{"v":1}` fires R3. Spec §3 says an absent input is "not evaluated" except for the missing case.
State in spec §3 that R3 fires when `pe`, `se` and `ke` are all absent or 0; `score_test` depends on it.

#### N-7 (minor): the load gate is a worst case

Under k6 every join is missing (0.88) and is persisted, so `bot_decisions` gets 100% of joins instead of about
1-5%. That is acceptable as a conservative gate; note it in `evidence.md`.

### Required for round 3

Fix N-2 in contracts 4, plan 2/3 and the store integration test. Add the N-1 test change request to plan 8 and
`red-check.md`. Fold in or defer the other minors with a reason. Round 3 re-checks N-2 and the N-1 test change
request only.
