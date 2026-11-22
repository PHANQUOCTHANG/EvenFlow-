# Plan: antibot-phase1 (draft round 3, step 2b)

Inputs: `spec.md` (D1-D10 and AC-1..AC-19 approved 2026-11-22; **Amendment 1** D11-D15, changed ACs and AC-20 approved
2026-11-22; D16 join button withdrawn by the human), `contracts.md` revision 3, `plan-review.md` rounds 1-2. Base develop @ 918667c. No migration, no new
module dependency. Tests are written first and must fail (red check) before production code. Section 8 maps every
review finding to its resolution.

## 1. Work packages and order

| WP | Scope | Depends on | Parallel |
|---|---|---|---|
| WP1 | `libs/go/antibot`: config, lenient decoder, rules, score, cap, `Evaluate` (pure), corpus package | — | with WP3 |
| WP2 | `libs/go/antibot/redisstate` (`observe.lua`), `Guard.Decide` (decode under recover, budget, breaker, metrics), decision sink | WP1 | — |
| WP3 | Web collector + wiring + golden signal fixtures (no join button, R3) | contracts 1 | with WP1 |
| WP4 | Waitingroom: Lua (join/shuffle/status/revoke), `JoinLane`/`Revoke`, handler, `buildGuard`, writer, controller penalty | WP2 | — |
| WP5 | Deploy (k8s + compose + policy), alerts, corpus tests, bot script, perf + load gate, docs note | WP1, WP4 | — |

## 2. Files

| File | Change | AC |
|---|---|---|
| `libs/go/antibot/config.go` | `LoadConfig(getenv)`; partial `ANTIBOT_WEIGHTS`/`ANTIBOT_RULES` merge with defaults; integer checks for counts; errors name the variable | 16 |
| `libs/go/antibot/signals.go` | `DecodeSignals(json.RawMessage)`: `map[string]json.RawMessage` over known keys, per-field decode, number rounding, `InvalidReason` only for the four whole-payload cases | 3 |
| `libs/go/antibot/rules.go` | rule table R1, R2, R3, R4D, R4N (skipped when `NetSuppressed`), R5/R5W (strong kinds tagged missing vs real; `fc` skipped for mobile UA), R6, R7; tz cache stores valid zones only | 4 |
| `libs/go/antibot/score.go` | noisy-OR; round to 3 decimals; tier from rounded score; D11 cap (`CAPPED`); D12 (`REJOIN`); sticky; mode + fallback → `Applied` | 8, 10, 12, 13 |
| `libs/go/antibot/privacy.go` | `FPHash`, `NetPrefix` | 15 |
| `libs/go/antibot/guard.go` | `Decide`: one `recover()` around decode + observe + evaluate; `context.WithTimeout`; breaker (N degraded in window → skip `Observe`, reason `breaker_open`); non-blocking sink with sampling (seedable RNG) | 5, 7, 14 |
| `libs/go/antibot/metrics.go` | collectors of contracts 6 | 17 |
| `libs/go/antibot/redisstate/{store.go,observe.lua}` | one `EVALSHA`; client options of contracts 4 (`NewClient(addr, cfg)`) | 6 |
| `libs/go/antibot/corpus/` (new, **not internal**, P-20) | deterministic generator + jsonl loader; `go run ./antibot/corpus/cmd/gen` writes `testdata/corpus/*.jsonl` | 18 |
| `services/waitingroom/internal/adapter/redis/script/{join,shuffle,status}.lua`, new `revoke.lua` | contracts 4 (optional lane args, `BLOCKED`, slow lobby in both shuffle branches, `slow_lottery_size`, revoke sets flag) | 11, 12 |
| `services/waitingroom/internal/adapter/redis/queue.go` | `Join` unchanged; new `JoinLane`, `Revoke`; `FencedQueue.Shuffle` passes penalty | 11, 12 |
| `services/waitingroom/internal/adapter/http/handler.go` | `BotGuard.Decide`; `MaxBytesReader` 4 KiB; raw `signals` + `BodyErr` into `Input`; `LaneStore` by type assertion; mapping of contracts 2; `ProblemRequestDenied` | 3, 9-13 |
| `services/waitingroom/internal/adapter/postgres/{pool.go,bot_decisions.go}` (new) | `NewPoolFromEnv`; writer of contracts 5 (simple protocol, multi-row INSERT, NULL for non-UUID, panic recovery) | 14 |
| `services/waitingroom/cmd/server/{main.go,guard.go}` | `buildGuard(cfg, rdbAddr, pool, log) (httpadapter.BotGuard, func())`, untyped nil for `off`; `main.go`'s `store` adapter gains `JoinLane`/`Revoke` | 13, 16 |
| `services/waitingroom/cmd/controller/main.go` | read `ANTIBOT_SLOW_LANE_PENALTY` for `shuffle.lua` | 11 |
| `apps/web/src/lib/antibot-signals.ts` (new) | collector: discrete-event `iv`/`sq` (D14), sampled counts, caps, FNV-1a with `rtb`, entropy floor, `Math.round` on all numbers; `snapshotSignals`, `readSignals` (`null` when absent), `clearSignals` | 1, 2 |
| `apps/web/src/components/event/event-action-panel.tsx` | start collector; snapshot at the top of `handleAction` | 1 |
| `apps/web/src/components/queue/waiting-room.tsx` | **(R3)** auto-join on mount as today; `joinQueue(eventId, readSignals(eventId) ?? {v: 1}, ...)`; the 401 re-join keeps the snapshot; `clearSignals` only on leave/checkout/sold-out. No button | 1 |
| deploy files of contracts 7 | k8s base + overlays + policy + compose | 16, 20 |
| `deploy/observability/alerts.yml`; `alerts_test.yml` (append only) | 4 alerts | 17 |
| `docs/02-kien-truc.md` | one line: antibot runs in-process in waitingroom (D1) | — |

Frozen (byte-identical): every existing waitingroom test (`handler_test.go`, `test/queuetoken`, `test/soldout`,
`test/opsapi`, ops and Lua tests), `audit.go`'s `NewAuditorFromEnv` and `audit_test.go`, existing web tests for
`queue-client`, `waiting-room`, `event-action-panel`, `device`, and existing `alerts_test.yml` cases. They compile
unchanged because the 4-argument `Join` and `New(store, nil, log)` stay. If an existing `waiting-room` test asserts
the auto-join on mount with no snapshot ... **(R3)** moot: D16 is withdrawn, auto-join stays, so the 5 frozen tests
(`waiting-room.test.tsx:44`, `:189`, `waiting-room.wp2.test.tsx:47`, `:58`, `:65`) stay unchanged and need no web
test change request. The only TCR is the approved k8s one (contracts 7, N-5).

## 3. Tests (written first)

| File | Kind | Covers |
|---|---|---|
| `antibot/config_test.go` | defaults; partial merge; each invalid value names its variable; float in a count key rejected; prod without key fails | 16 |
| `antibot/signals_test.go` | per-field leniency (wrong type → only that field absent; `123.4` → 123); the four missing reasons; `{"v":1}` = empty not missing; decodes every `testdata/signals/*.json` golden file to its expected struct; `FuzzDecodeSignals` | 3 |
| `antibot/rules_test.go` | each rule at / below / above threshold / absent; R4N suppressed; R5 missing vs real strong; `fc` skipped for Android/iOS UA; invalid tz not cached; R6 dormant | 4 |
| `antibot/score_test.go` | every row of contracts 3 "Default outcomes"; boundaries on the rounded score; D11 cap; D12 `REJOIN`; sticky score 1.000; mode × fallback | 8, 10, 12, 13 |
| `antibot/privacy_test.go` | HMAC length/stability, prefixes, mapped IPv6 | 15 |
| `antibot/guard_test.go` | fake store: timeout / error → DEGRADED; breaker opens after N, closes after open window; decode panic and store panic → PASS + `panic`; shadow never revokes; sink full → dropped, no block; sample rate with seeded RNG; every BLOCK has rules or `STICKY`; slog output of a decision has no raw fp/UA/IP | 7, 14, 15 |
| `antibot/metrics_test.go` | all metric names present in `prometheus.DefaultGatherer` after one decision | 17 |
| `antibot/engine_bench_test.go` | `TestEvaluateP99` (10 000 runs, p99 < 0.2 ms, skipped under `-race`) | 5 |
| `antibot/redisstate/store_integration_test.go` | testcontainers Redis: counts, windows, caps, TTLs, `hasSlot`, sticky early return, **(R3, N-2)** `TestNetShareSuppression`: 20 000 joins in the window, 2000 from one /24 (10%) → `NetSuppressed`; 500 from one /24 (2.5%) → not suppressed; < 1000 event joins → never suppressed (batch-loaded via a pipeline of `observe.lua` calls), command count = 1 (go-redis hook), keys contain no raw fp or full IP; `TestDecideP99` sequential and with 64 goroutines (p99 < 5 ms each); delay injection via an in-test TCP proxy (`net.Listen` + delayed `io.Copy`, no new dependency): 20 ms delay → DEGRADED within 6 ms, no reconnect storm (dial count bounded by pool size) | 5, 6, 7, 15 |
| `antibot/corpus_test.go` + `testdata/corpus/` | see 3.1; prints per-family, per-rule and ±20% sensitivity tables | 18 |
| `waitingroom/.../redis/lane_integration_test.go` | +P after T0; pre-T0 `LOBBY`; slow lobby placed with **empty** main lobby; every pre-T0 slow member ahead of every post-T0 slow joiner (`+m`); idempotent re-join and token equality per lane across T0; chi-square fairness per lane; `lottery_size` unchanged; 4-arg `Join` ignores the flag; `JoinLane` refuses while flag set; revoke removes all sets + sets flag TTL 86400 ⇒ status `UNKNOWN` | 11, 12 |
| `waitingroom/.../http/antibot_handler_test.go` | PASS byte-identical; SLOW_LANE same shape; BLOCK 403 + revoke; `BLOCKED` from `JoinLane` → 403; store without `LaneStore` falls back; body > 4 KiB still joins; **existing slot + `{v:1}` → 200 same token (REJOIN), no revoke**; **401 re-join path with kept snapshot** | 1, 3, 9-12 |
| `waitingroom/cmd/server/guard_test.go` | `buildGuard` for `off` returns an interface equal to nil | 13 |
| `waitingroom/.../postgres/bot_decisions_integration_test.go` | columns, INET prefix, `ALLOW`, non-UUID → NULL, simple-protocol path, batch + shutdown flush, failed batch counted, panic recovered | 14, 15 |
| `waitingroom/test/antibot_bot_test.go` + `test/botsim` | AC-19: handler + real Redis + real guard in `enforce`; bot families and recorded human sessions over HTTP; botsim reads `libs/go/antibot/testdata/corpus/*.jsonl` by relative path | 19 |
| `apps/web/src/lib/antibot-signals.test.ts` | 3 s steady mousemove (`iv` has no pointermove, `sq` not emitted or high-entropy), mobile tap sequence, counts/caps, entropy floor, click recorded first, all numbers integers, <= 2 KiB, no key values; writes the golden files | 1, 2 |
| `waiting-room.antibot.test.tsx`, `event-action-panel.antibot.test.tsx` | snapshot used on join and on the 401 re-join; **(R3)** no snapshot → auto-join on mount with `signals: {v: 1}`; click snapshot before jitter | 1 |
| `deploy/k8s` checks | `deploy/k8s/validate.sh`, `policy/test_policy.sh`, and `kustomize build overlays/{local,staging} \| grep -c ANTIBOT_HASH_KEY` >= 1 | 16, 20 |
| `alerts_test.yml` (appended cases) | promtool tests for the 4 alerts | 17 |

### 3.1 Corpus (AC-18, P-7): a regression check, not FP evidence

`testdata/corpus/README.md` fixes the human parameters **before** looking at thresholds, each with a source
(published UX timing studies for click/reaction latency, Vietnamese mobile-share and CGNAT practice, browser
defaults). Populations sit on **both** sides of each threshold: `rt` including 50-200 ms tails, steady mouse,
mobile taps, keyboard-only, privacy browsers (`gl` false, few fonts), Android low font count, held-key auto-repeat, travellers, CGNAT
groups of 200-2000 identities per /24, a single-LB-IP scenario (everyone on one IP), households of 2-5 sharing an
fp, multi-tab and other-device re-joins (existing slot + no snapshot). Bots: B1 raw HTTP, B2 headless default, B3
fixed-rhythm stealth, B4 account farm, B5 replay (small proxy pool and distinct networks, reported separately).
`humans_recorded.jsonl`: 50-100 sessions recorded by the team with the real collector (dev build, several
browsers/phones), own assertion < 1% demoted. The report states the limits; the real FP gate is the shadow
demotion rate on real traffic (AC-20).

## 4. Gates

- Go: `go vet`, `gofmt -l` empty, `go test -race ./...` in `libs/go` and `services/waitingroom`; integration tests;
  `-fuzz=FuzzDecodeSignals -fuzztime=30s`; `go mod tidy` diff empty in both modules.
- Perf: `TestEvaluateP99`, `TestDecideP99` (sequential + concurrent) without `-race`.
- **Load (P-9)**: `tests/load/opening-spike.js` against a compose stack on develop and on this branch with
  `ANTIBOT_MODE=shadow` (same machine, same seed): join p99 delta <= +5 ms; Redis CPU (`INFO cpu` delta) <= +50%;
  recorded in `evidence.md`.
  **(R3, N-7)** This is a worst case: k6 sends `{"signals":{}}`, so every join is missing (0.88, CAPPED) and is
  persisted (100% of joins in `bot_decisions` vs ~1-5% for real traffic). Noted as such in `evidence.md`.
- Web: lint, typecheck, `npx vitest run`.
- Deploy: `deploy/k8s/validate.sh`, `policy/test_policy.sh`, `policy/testdata/gen_fixtures.py --check`, the
  `kustomize build | grep ANTIBOT_HASH_KEY` check.
- Observability: `promtool check rules` + `promtool test rules`.
- `docs/workflow/antibot-phase1/full-gate.sh` runs all of the above except the load gate.

## 5. Mutation test (threshold 13/13)

(1) noisy-OR → sum. (2) tier `>=` → `>`. (3) R2 `<` → `<=`. (4) R5W `>= 2` → `>= 1`. (5) R4D counts the caller
twice. (6) shadow applies the tier. (7) degraded refuses the join. (8) `join.lua` drops the penalty. (9) `revoke.lua`
skips `admitted`. (10) `FPHash` returns the raw fp. (11) **missing signals on re-join revokes** (drop D12).
(12) drop the D11 cap. (13) `shuffle.lua` returns before the slow lobby when n == 0.

## 6. Risks

See spec 6. Plan-specific: go-redis needs `ContextTimeoutEnabled` and `MaxRetries: -1` (asserted by the proxy
delay test); `time/tzdata` adds ~450 KB; jsdom cannot measure fonts/WebGL (probes stubbed); observe.lua adds
~12-16 commands on the event's shard (load gate).

## 7. Rollout and rollback

Rollout (AC-20): (1) create `ANTIBOT_HASH_KEY` in Secret `eventflow-auth` (staging) **before** applying; (2) deploy
web + waitingroom + waitingroom-controller from one image tag, mode `shadow`; (3) one rehearsal on-sale in shadow,
review `antibot_decisions_total`, `capped`, `net_suppressed` and `bot_decisions`; (4) enforce only if the shadow
demotion rate < 1% **(R3, N-4) measured over joins whose signals decode valid** (missing-payload joins, i.e. k6,
`demo.sh` and old bundles, are excluded and their share reported separately; load-test windows are excluded) and both deployments run the new scripts (a controller on old `shuffle.lua` would strand the slow
lobby).

Rollback: `ANTIBOT_MODE=off` (guard not wired; 4-arg `Join` ignores the flag; lane always `n`). Code: one PR, merge
commit, `git revert -m 1 <merge>`; no schema change; `ab:*` keys expire within 24 h. **Limit (P-19):** slots revoked
under enforce are not restored by `off` or a revert; such a buyer re-joins at the tail. This goes into the enforce
go/no-go note. Check after revert: waitingroom `go test -race ./...` and web vitest green.

## 8. Round 2 changes (plan-review.md round 1: FAIL, 3 blockers, 6 majors, 12 minors)

| Finding | Resolution |
|---|---|
| P-1 blocker: re-join without snapshot → BLOCK + revoke + sticky | D11 (BLOCK needs a positive sign; missing-only capped at SLOW_LANE), D12 (`observe.lua` returns `hasSlot`; existing slot + non-BLOCK → existing slot, `REJOIN`), D16 (snapshot kept on re-join; no auto-join without snapshot → join button); R1-server ignores slot-holding joins; handler + web tests; mutation 11, 12 |
| P-2 blocker: sampled pointermove → R1 + R7 = BLOCK | D14: `iv`/`sq` from discrete events only; `sq` needs >= 12 events and >= 4 buckets and includes `rtb`; R7 weight 0.50; R1+R7 now 0.75 (SLOW_LANE, and rare); web tests for steady mouse and mobile tap; corpus populations |
| P-3 blocker: staging crash loop without `ANTIBOT_HASH_KEY` | contracts 7: Secret key in `eventflow-auth` (base deployment, local secrets + replacement, staging header), `ANTIBOT_MODE=shadow` app config, policy update; gate `kustomize build \| grep`; rollout step 1 |
| P-4 major: signature changes break frozen tests | 4-arg `Join` kept; `JoinLane`/`Revoke` via optional `LaneStore`; `join.lua` lane args optional |
| P-5 major: empty main lobby strands slow lobby | `shuffle.lua` handles `lobby_slow` in both branches; test with empty main lobby; mutation 13; rollout requires controller on new scripts |
| P-6 major: R4-network alone → SLOW_LANE under CGNAT | D13: `R4D` 0.60 / `R4N` 0.25; R4N suppressed for a prefix > 5% of joins (min 1000 joins); metric + alert; CGNAT and single-LB corpus scenarios |
| P-7 major: corpus self-fulfilling | 3.1: parameters sourced independently, both sides of thresholds, recorded human sessions with own assertion, ±20% sensitivity, AC-18 restated as regression check; real gate = shadow on real traffic (AC-20) |
| P-8 major: one wrong type invalidates the payload | D15 per-field lenient decode, rounding; missing only for the 4 whole-payload cases; golden fixtures shared web ↔ Go; web rounds all numbers |
| P-9 major: no system perf gate, reconnect storm | client `MaxRetries -1`, `PoolSize 128`, `MinIdleConns 32`, `DialTimeout 50ms`; breaker; concurrent p99; TCP-proxy delay test; opening-spike shadow vs develop load gate |
| P-10 minor: decode outside recover, signature mismatch | `Input.RawSignals`; decode inside `Decide` under the same `recover()`; one signature in both docs |
| P-11 minor: D7 vs AC-16 | D7 reworded in Amendment 1 |
| P-12 minor: BLOCK/sticky not atomic | `revoke.lua` sets the flag; `join.lua` (lane path) refuses while set; `MarkBlocked` removed |
| P-13 minor: slow lane interleave | post-T0 slow score `+ m` (`slow_lottery_size`); test. Edge accepted: between T0 and the shuffle m = 0 (seconds) |
| P-14 minor: writer details | non-UUID → NULL + counter; simple protocol multi-row INSERT for PgBouncer; `NewPoolFromEnv` (audit untouched, own 2 conns); panic recovery; STICKY score 1.000 |
| P-15 minor: typed nil | `buildGuard` + test |
| P-16 minor: test gaps | sticky via real `observe.lua`; sample rate; BLOCK rows non-empty; Redis keys and slog privacy; metrics gathered; TCP-proxy delay; per-lane token equality across T0; `alerts_test.yml` append only |
| P-17 minor: key R4D on session device id | **Not adopted**: `devices.id` is unique per (identity, fingerprint) (0001_core.sql:17-24), so it never repeats across identities and R4D could not fire. Body `fp` stays; rotation is still seen by R4N/R1; signed fp hash from identity is a follow-up |
| P-18 minor: Android fonts | `fc` not evaluated for mobile UAs; Android profile in corpus |
| P-19 minor: rollback limits | stated in 7 and AC-20 |
| P-20 minor: `internal/corpusgen` not importable | non-internal `libs/go/antibot/corpus`; botsim reads jsonl by path |
| P-21 minor: config/scoring edges | tier from rounded score; partial lists merge; only valid tz cached; integer validation |

## 9. Round 3 changes (plan-review.md round 2: FAIL, N-2 major + N-1, N-3..N-7 minor; human revision of Amendment 1)

| Finding | Resolution |
|---|---|
| N-2 major: R4N suppression can never trigger (64-member ZSET vs joins) | contracts 4: per-prefix join buckets `ab:{ev}:nj:{prefix}:{minute}` (`INCR`, TTL 360 s, same slot); share = prefix joins / event joins over the same 5 buckets (same unit); `net` ZSET only for the distinct count. `TestNetShareSuppression`: 2000/20 000 suppressed, 500/20 000 not, < 1000 joins never. Self-suppression of a > 5% farm accepted (contracts 4) |
| N-1 minor: D16 button breaks 5 frozen web tests | Human withdrew D16: no button, auto-join kept, `{v:1}` sent without a snapshot. Button work removed from WP3, files and tests; the 5 tests stay frozen; no web TCR |
| N-3 minor: `{v:1}` now passes | Trade-off recorded in contracts 1 (and by the human in spec); worked examples added to contracts 3 (0.40 CHALLENGE → pass; `REJOIN` with a slot) |
| N-4 minor: AC-20 polluted by synthetic traffic | Rollout step 4: demotion rate over valid-decode joins only, load windows excluded, missing share reported separately |
| N-5 minor: frozen k8s policy contract and generated fixtures | contracts 7 lists policy-contract §8 row (TCR approved), `check_policy.py` lines 86 and 116, `rules.md`, `gen_fixtures.py` + regenerated good fixtures with every bad fixture still tripping its own rule; gate adds `gen_fixtures.py --check` |
| N-6 minor: R3 absent counts vs spec 3 | contracts 1: R3 fires when `pe`, `se`, `ke` are each absent or 0 (the missing-input exception); all other rules with absent input are not evaluated; `score_test` asserts it |
| N-7 minor: load gate is a worst case | Noted in gate 4 and to be stated in `evidence.md` |
| P-2 residual: keydown auto-repeat fires R1 (0.50 alone) | Accepted (CHALLENGE → pass); held-key population added to the corpus (3.1) |
