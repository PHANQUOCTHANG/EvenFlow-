# Plan: antibot-phase1 (draft, step 2b)

Inputs: `spec.md` (D1-D10, AC-1..AC-19; draft, awaiting human approval; plan assumes the Q-1..Q-5 defaults),
`contracts.md`. Base develop @ 918667c. No migration, no new module dependency. Tests are written first and must
fail (red check) before any production code.

## 1. Work packages and order

| WP | Scope | Depends on | Parallel |
|---|---|---|---|
| WP1 | `libs/go/antibot`: config, signal decoder, rules, score, `Evaluate` (pure) | — | with WP3 |
| WP2 | `libs/go/antibot/redisstate` (`observe.lua`, `MarkBlocked`), `Guard.Decide` (budget, degraded, panic, metrics), decision sink | WP1 | — |
| WP3 | Web collector + wiring (`apps/web`) | contracts 1 only | with WP1 |
| WP4 | Waitingroom: Lua lane/revoke, handler mapping, main wiring, bot_decisions writer | WP2 | — |
| WP5 | Corpus + bot script + benchmarks + alerts + docs note | WP1 (corpus), WP4 (integration) | — |

## 2. Files

| File | Change | AC |
|---|---|---|
| `libs/go/antibot/config.go` (new) | `Config`, `LoadConfig(getenv func(string) string) (Config, error)`; parses contracts 3; errors name the variable; dev hash key + `DevKey bool` | 16 |
| `libs/go/antibot/signals.go` (new) | `Signals` (present flags), `DecodeSignals(raw json.RawMessage)`; strict types via `json.Decoder` on the `signals` value, per-field range checks, `InvalidReason` | 3 |
| `libs/go/antibot/rules.go` (new) | rule table `[]rule{id, eval func(Config, Input, State) bool}` for R1, R2, R3, R4, R5/R5W, R6, R7; UA deny matcher (lower-cased once); tz check with `time.LoadLocation` cached in a bounded `sync.Map` (<= 600 zones) + `import _ "time/tzdata"` | 4 |
| `libs/go/antibot/score.go` (new) | noisy-OR, 3-decimal rounding, tier from cut-offs (lower bound inclusive), `Applied` from mode + fallback | 8, 10, 13 |
| `libs/go/antibot/privacy.go` (new) | `FPHash` = hex(HMAC-SHA256(key, fp))[:32]; `NetPrefix(addr)` = /24 or /64 after `Unmap()` | 15 |
| `libs/go/antibot/guard.go` (new) | `Guard.Decide`: decode already done by caller; `context.WithTimeout(ANTIBOT_REDIS_TIMEOUT_MS)` around `Observe`; error → `State{Known:false}` + `DEGRADED` tag; `recover()` → PASS + panic metric; `MarkBlocked` only when applied BLOCK and not sticky; hands the decision to `Sink` (non-blocking) | 5, 7, 12, 14 |
| `libs/go/antibot/metrics.go` (new) | Prometheus collectors of contracts 6, registered on a given `Registerer` | 17 |
| `libs/go/antibot/redisstate/store.go` + `observe.lua` (new) | one `EVALSHA` per decision (contracts 4); keys built with `{ev}` hash tag | 6 |
| `services/waitingroom/internal/adapter/redis/script/join.lua` | lane + penalty args; idempotency also checks `lobby_slow` | 9, 11 |
| `services/waitingroom/internal/adapter/redis/script/shuffle.lua` | slow-lobby shuffle after the main lottery | 11 |
| `services/waitingroom/internal/adapter/redis/script/status.lua` | `lobby_slow` → `LOBBY` | 11 |
| `services/waitingroom/internal/adapter/redis/script/revoke.lua` (new) + `queue.go` | `Queue.Join(..., lane Lane)`, `Queue.Revoke(ctx, ev, identity) (bool, error)`; keys list gains `lobby_slow` | 11, 12 |
| `services/waitingroom/internal/adapter/http/handler.go` | `BotGuard` → `Decide`; `MaxBytesReader` 4 KiB on join body; decode `signals` once; build `antibot.Input` (UA, XFF first element, principal); map `Applied` per contracts 2; `ProblemRequestDenied`; `QueueStore.Join` gains `lane` | 3, 9-13 |
| `services/waitingroom/internal/adapter/postgres/bot_decisions.go` (new) | `DecisionWriter` (channel 10 000, flush 200 ms / 500 rows, `CopyFrom`, shutdown flush 2 s, drop counter) using the pool from `NewAuditorFromEnv`'s `DATABASE_URL` | 14 |
| `services/waitingroom/cmd/server/main.go` | `antibot.LoadConfig(os.Getenv)`; mode `off` → guard `nil`; else dedicated redis client (contracts 4) + store + writer + guard; startup log `antibot_mode` | 13, 16 |
| `apps/web/src/lib/antibot-signals.ts` (new) | `startSignalCollector(eventId)` (passive listeners, sampling, caps, ring buffer, FNV-1a), `snapshotSignals(eventId)` (records the click first, probes WebGL/fonts once, writes `ef:ab:{eventId}`), `readSignals(eventId)` (returns `{v:1}` when absent/corrupt), `clearSignals` | 1, 2 |
| `apps/web/src/components/event/event-action-panel.tsx` | start collector on mount; `snapshotSignals` at the top of `handleAction` (before jitter) | 1 |
| `apps/web/src/components/queue/waiting-room.tsx` | `joinQueue(eventId, readSignals(eventId), ...)`; `clearSignals` beside `clearQueueToken` | 1 |
| `deploy/observability/alerts.yml`, `alerts_test.yml` | 3 alerts (contracts 6) | 17 |
| `deploy/compose/docker-compose.yml` | waitingroom env `ANTIBOT_MODE: shadow` (comment: enforce only after shadow review) | 13 |
| `docs/02-kien-truc.md` | one line under the service table: antibot runs in-process in waitingroom (D1) | — |

Frozen (byte-identical): existing waitingroom tests (`handler_test.go`, Lua/queue tests, ops tests), existing web
tests for `queue-client`, `waiting-room`, `event-action-panel`, `device`. If an existing test constructs
`httpadapter.New(store, nil, log)` or calls `QueueStore.Join` with the old signature, the adapter keeps a
compatibility wrapper rather than editing the frozen test (to be confirmed at red check; otherwise a test change
request).

## 3. Tests (written first)

| File | Kind | Covers |
|---|---|---|
| `libs/go/antibot/config_test.go` | table: defaults, each invalid env value names its variable, prod without key fails, dev key warning flag | 16 |
| `libs/go/antibot/signals_test.go` | table: valid payload, each field at/over range, wrong types, unknown fields, `v` != 1, `{}`, `{"v":1}`, 4 KiB + 1; `FuzzDecodeSignals` (no panic, invalid ⇒ missing) | 3 |
| `libs/go/antibot/rules_test.go` | per rule: at threshold, just below, just above, input absent; R5 strong vs 2-weak vs 1-weak; R6 with `CardIdentities=-1` never fires | 4 |
| `libs/go/antibot/score_test.go` | noisy-OR values incl. the contracts 3 expected outcomes, boundary 0.30/0.60/0.85 inclusive, mode × fallback matrix for `Applied` | 8, 10, 13 |
| `libs/go/antibot/privacy_test.go` | HMAC length/stability, different key ⇒ different hash, /24 and /64 prefixes, mapped IPv6 | 15 |
| `libs/go/antibot/guard_test.go` | fake store: timeout ⇒ DEGRADED + stateless rules only; error ⇒ same; store panics ⇒ PASS + panic metric; sticky ⇒ BLOCK `{STICKY}` without rule eval; shadow never calls `MarkBlocked`; sink full ⇒ dropped counter, `Decide` does not block | 7, 12, 13, 14 |
| `libs/go/antibot/engine_bench_test.go` | `BenchmarkEvaluate` + `TestEvaluateP99` (10 000 runs, sorted durations, p99 < 0.2 ms; skipped under `-race`) | 5 |
| `libs/go/antibot/redisstate/store_integration_test.go` | testcontainers Redis (`libs/go/testx`): counts distinct identities, window expiry, caps (64 members, 9 gaps), TTLs set, one round trip (`MONITOR`-free: wrap client with a hook counting commands = 1), `TestDecideP99` 10 000 `Decide` calls p99 < 5 ms | 5, 6 |
| `libs/go/antibot/corpus_test.go` + `testdata/corpus/{bots,humans}.jsonl` | replay corpus through `Evaluate` with an in-memory `State` emulator that mirrors `observe.lua` (same windows and caps); asserts AC-18 thresholds; prints confusion table per family and per rule | 18 |
| `libs/go/antibot/internal/corpusgen` (new, `go run`) | deterministic generator (fixed seed) that writes the two jsonl files; committed output; test fails if regenerated output differs | 18 |
| `services/waitingroom/internal/adapter/redis/lane_integration_test.go` | slow lane after T0 = +P; before T0 `LOBBY`, post-shuffle behind lottery and first P; idempotent re-join per lane; chi-square fairness within each lane (1 000 runs, p > 0.01); `lottery_size` unchanged; revoke removes from every set ⇒ status `UNKNOWN`; revoke of absent identity = 0 | 11, 12 |
| `services/waitingroom/internal/adapter/http/antibot_handler_test.go` | fake guard per `Applied`: PASS byte-identical to develop response; SLOW_LANE same field set and state; BLOCK 403 type + revoke called + no token; body > 4 KiB still joins (signals missing); guard nil (`off`) = develop | 3, 9-13 |
| `services/waitingroom/internal/adapter/postgres/bot_decisions_integration_test.go` | testcontainers Postgres with `migrations/`: row columns per contracts 5, prefix INET, `ALLOW` mapping, no raw fp/IP in any column, batch flush, shutdown flush | 14, 15 |
| `services/waitingroom/test/antibot_bot_test.go` | AC-19: handler + real Redis + real guard in `enforce`; runs the bot script families (below) and the human replay through HTTP; asserts >= 95% / < 1% | 19 |
| `apps/web/src/lib/antibot-signals.test.ts` | vitest + jsdom: counts, sampling, caps (32 intervals, 10 000), `sq` only with >= 6 events, click recorded first, snapshot <= 2 KiB, no key values stored, corrupt storage ⇒ `{v:1}`, passive listeners, removal on unmount | 1, 2 |
| `apps/web/src/components/queue/waiting-room.antibot.test.tsx`, `event-action-panel.antibot.test.tsx` | join sends stored signals (also on re-join); click writes the snapshot before jitter | 1 |
| `deploy/observability/alerts_test.yml` | promtool unit tests for the 3 alerts | 17 |

### Corpus and bot script (AC-18, AC-19)

Bot families (>= 200 sessions each): **B1** raw HTTP, no signals, `python-requests` UA; **B2** headless default
(`wd` true, `HeadlessChrome` UA, `rt` 20-80 ms, no pointer moves, only the click); **B3** stealth with forged
human-looking counts but fixed rhythm (`iv` CV < 0.03, `rt` constant 60-100 ms); **B4** account farm: 40 identities
over 8 fp values and one /24, random human-looking signals; **B5** replay: one recorded human sequence replayed on
50 identities (same `sq`). Humans (>= 5 000): log-normal `rt` (median 1.8 s, p0.1 >= 150 ms), jittered intervals,
pointer/touch/keyboard-only users, 3% privacy browsers (`gl` false, `fc` 1-2), 2% travellers (`tz`/`tzo` mismatch
alone), campus groups of 25 identities on one /24, households of 2-3 identities sharing an fp, 1% fast clickers
(`rt` 80-119 ms). The bot script is `services/waitingroom/test/botsim` (Go helper used by the test, no network
outside the test), so CI needs no browser. Limits stated in the report: synthetic humans, non-adversarial bots.

## 4. Gates

- Go: `go vet`, `gofmt -l` empty, `go test -race ./...` in `libs/go` and `services/waitingroom`; integration tests
  (testcontainers) as in earlier tasks; `go test -run=^$ -fuzz=FuzzDecodeSignals -fuzztime=30s ./antibot`;
  `go mod tidy` diff empty in both modules (no new dependency).
- Perf: `TestEvaluateP99`, `TestDecideP99` run without `-race` in a dedicated step.
- Web: `npm run lint`, `npm run typecheck`, `npx vitest run`.
- Observability: `promtool check rules` + `promtool test rules`.
- Repo gate script `docs/workflow/antibot-phase1/full-gate.sh` modelled on `gateway-trusted-proxy/full-gate.sh`.

## 5. Mutation test (threshold 10/10)

(1) noisy-OR → sum. (2) tier cut-off `>=` → `>`. (3) R2 `<` → `<=` threshold. (4) R5 weak count `>= 2` → `>= 1`.
(5) R4 counts the caller twice / skips it. (6) shadow applies the tier. (7) degraded path refuses the join.
(8) `join.lua` drops the penalty. (9) `revoke.lua` skips `admitted`. (10) `FPHash` returns the raw fp. Each must fail
at least one test.

## 6. Risks

See spec 6. Plan-specific: go-redis ignores context deadlines unless `ContextTimeoutEnabled` (hence a dedicated
client, asserted by `TestDecideP99` with an injected 20 ms Redis delay ⇒ DEGRADED within 6 ms); `time/tzdata` adds
~450 KB to the binary (accepted); jsdom cannot measure fonts/WebGL, so the web tests stub the probes.

## 7. Rollback

Runtime: `ANTIBOT_MODE=off` (guard not wired; join path identical to develop; the slow-lane Lua paths are inert
because lane is always `n` and `lobby_slow` stays empty). `shadow` is the default, so a merge alone never denies a
buyer. Code: one PR, merge commit, `git revert -m 1 <merge>`; no schema change; leftover `ab:*` keys expire within
24 h, `wr:{ev}:lobby_slow` expires with the session TTL. Check after revert: waitingroom `go test -race ./...` green,
web vitest green, `bot_decisions` rows remain (harmless, append-only).
