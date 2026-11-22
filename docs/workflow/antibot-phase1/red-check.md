# Red check — antibot-phase1 (v3 step 2a)

Tests written against spec.md + Amendment 1 (+ revision after plan review round 2) and contracts.md revision 2
(9eb3bd9, 9bc1e92; re-checked against revision 3 in 0bb1706: no test change needed). plan.md was read only for file and package names. No production code was written.
Baseline counts: `baseline.json`. AC → test map and mutation targets: `traceability.yaml`.

## Files

| Module | New test files |
|---|---|
| libs/go/antibot | `signals_test.go`, `rules_test.go`, `score_test.go`, `guard_test.go`, `config_test.go`, `privacy_test.go`, `golden_test.go`, `corpus_test.go`, `engine_bench_test.go`, `helpers_test.go`, `race_on_test.go` / `race_off_test.go` |
| libs/go/antibot fixtures | `testdata/signals/{scripted_keyboard,mobile_touch,click_only}.json` (shared with web), `testdata/corpus/README.md` (parameter sources) |
| libs/go/antibot/corpus | `corpus.go`: deterministic generator (test fixture code, imports nothing from the engine) |
| libs/go/antibot/redisstate | `store_integration_test.go` (`integration`) |
| services/waitingroom | `internal/adapter/http/antibot_handler_test.go`, `internal/adapter/redis/lane_integration_test.go` (`integration`), `internal/adapter/postgres/bot_decisions_test.go`, `bot_decisions_integration_test.go` (`integration`), `test/antibot/antibot_bot_test.go` (`integration`) |
| apps/web | `src/lib/antibot-signals.test.ts`, `src/components/queue/waiting-room.antibot.test.tsx`, `src/components/event/event-action-panel.antibot.test.tsx` |

No existing test file was edited. The 5 frozen waiting-room web tests are untouched. Under the revised
Amendment 1 there is no join button, so no TCR is needed for them.

## Result (all red for the right reason)

| Command | Result | Reason |
|---|---|---|
| `cd libs/go && go vet ./...` / `go test ./...` | FAIL `libs/go/antibot [build failed]` | "build constraints exclude all Go files in libs/go/antibot": the package does not exist yet. All other packages ok (262 pass, same as baseline) |
| `go vet -tags integration ./antibot/...` | FAIL | same, plus "no non-test Go files in libs/go/antibot/redisstate" |
| `cd services/waitingroom && go vet ./...` / `go test ./...` | FAIL `internal/adapter/http`, `internal/adapter/postgres` [build failed] | import of the missing `libs/go/antibot`. Other packages ok. With the new files moved aside, all 228 baseline tests pass, so existing tests are unaffected |
| `npx vitest run <3 new files>` | 3 files fail, 6 assertion failures + 1 import failure | `antibot-signals.test.ts`: `Failed to resolve import "./antibot-signals"`. Component tests fail on behaviour: join sends `{}` instead of the snapshot / `{"v":1}`, the snapshot is not kept or cleared, the panel registers no passive collector and writes no snapshot |
| full vitest | 2727 pass (= baseline), 6 fail (new) | |
| `tsc --noEmit` | only TS2307 for `./antibot-signals` | |

Compile check: all new Go test code (unit and `integration`) was vetted against a throwaway stub of the contracted
API in the scratchpad (not committed). It compiles. A scratch reference engine written from contracts 1-3 passes
every pure-engine, decoder, config, golden and corpus test. That confirms the tests encode the contracts and the
corpus thresholds are reachable. Corpus result on that reference: bots 95.8% (958/1000), humans 0.10% (5/5000).

## Identifiers the implementation must provide

From contracts 2 / 8.5, used as written: `antibot.{RuleID, Action, Mode, InvalidReason, Signals, Signals.Missing,
DecodeSignals, Input{EventID, IdentityID, DeviceID, UserAgent, ClientIP, RawSignals, BodyErr, Now}, State{...},
Decision{...}, Evaluate(cfg, in, sig, st), StateStore.Observe(ctx, in, fpHash, net, sq), Sink.Offer, NewMetrics,
NewGuard, Guard.Decide, FPHash, NetPrefix, LoadConfig, Config.Mode, Config.DevKey}`; `domain.Lane`,
`domain.LaneNormal` (`"n"`), `domain.LaneSlow` (`"s"`); `(*redisadapter.Queue).JoinLane`, `.Revoke`;
`httpadapter.LaneStore`, `httpadapter.ProblemRequestDenied`; `BotGuard.Decide`.

**Chosen by the tests (not named in contracts):**

- `redisstate.New(ctx context.Context, rdb *redis.Client, cfg antibot.Config) (*redisstate.Store, error)`. The cfg
  is needed for windows and the R4N share threshold.
- `postgresadapter.NewDecisionWriter(pool *pgxpool.Pool, m *antibot.Metrics) *postgresadapter.DecisionWriter`, with
  `Offer(antibot.Input, antibot.Decision)` (implements `antibot.Sink`) and `Close(ctx) error` (shutdown flush). A nil
  pool means disabled: every row is dropped and counted.
- `ANTIBOT_BREAKER` format `N/window,open=duration` (e.g. `5/1s,open=200ms`), per contracts 3 default `20/1s,open=3s`.
- Web: `startSignalCollector` measures `rt` from its own start (mount). `snapshotSignals` records the join click as
  a discrete **pointer** event (pe +1, one interval). The collector uses `performance.now()`/`Date` (both faked in
  tests).
- `(*FencedQueue).Shuffle` keeps its signature. The slow lobby uses `shuffle.lua`'s default P (100000).

## Semantics the tests pin (please confirm in review)

1. `DecodeSignals`: `nil`, empty, `null` or a non-object give `missing`. `{}` or absent/non-1 `v` (including `"1"`)
   gives `bad_version`. More than 4096 bytes passed to `DecodeSignals` gives `too_large`.
2. `{"v":1}` → R3 fires (counts absent = 0), R5 does not → 0.40 CHALLENGE → pass; with a slot → applied PASS
   `REJOIN`. Matches contracts revision 3 (N-6: R3 is the one rule where absent inputs count).
3. R4N ignores `ClientIP` itself and trusts `State.NetIdentities` (the store skips absent inputs).
4. Sticky in shadow: `Applied` is PASS (shadow never applies anything else; FuzzEvaluate asserts it).
5. Revoke of an identity without a slot returns false **and still sets the flag**.
6. Writer: `fired_rules` = sorted rule IDs followed by tags in Decision order (e.g. `{R3,R5,CAPPED,REJOIN}`).
   `action` = tier.
7. SLOW_LANE via `JoinLane`: the handler passes a penalty > 0 (source of P is the implementer's choice).
8. Windows timer resolution: the pure-engine p99 test averages batches of 50 calls on Windows only.

## Open items / not covered by these tests

- **AC-18 recorded sessions**: `TestCorpus_AC18_RecordedSessions` skips with "PENDING" until the team adds 50-100
  snapshots under `libs/go/antibot/testdata/corpus/recorded/`.
- **AC-17 alerts**: the 4 alert cases go into the existing (frozen) `deploy/observability/alerts_test.yml` as
  appended cases, per contracts 6. They are left to the implementer and the promtool gate.
- **AC-16 k8s**: Secret and policy changes (N-5 TCR) are gate steps, not tests here. `cmd/server.buildGuard`
  (untyped nil for off) is not unit-tested. It is checked in review.
- **Integration tests were not run here** (no Docker on this machine). They were only compiled (`go vet -tags
  integration` against the stub).
- The `replay_residential` family (R7 only, CHALLENGE → pass until EV-176) is reported but not gated.
- Corpus sensitivity: `R2_MS` × 0.8 drops bots caught to about 89.7% on the reference. The gate is fragile to that
  threshold.
