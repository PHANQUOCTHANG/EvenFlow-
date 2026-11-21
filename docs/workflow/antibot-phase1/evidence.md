# Verification evidence: antibot-phase1 (v3 step 6, High risk)

Independent verifier, read-only on code. Worktree `EvenFlow-antibot`, HEAD `63bde04`, verified 2026-11-22.
Logs: `verify-logs/` (baseline, verbose AC runs, one log per mutant in `verify-logs/mutants/`).

## Verdict: FAIL (test adequacy), no code defect found

- 57 of 61 behaviour-changing mutants killed by the unit suites.
- 3 behaviour-changing mutants survive every suite that can run here (M16, M47, M61). None is a code defect:
  the shipped code does the right thing in each case. The gap is that no test pins that behaviour.
- 1 mutant (M53) is killed only by an integration test that needs Docker, so it was not run here.
- Under the v3 rule ("every behaviour-changing mutant killed"), this is a FAIL. Three small tests close it
  (see "Required to reach PASS"). Each needs a TCR because the test files are frozen.

## 1. Gate commit vs code

| Check | Result |
|---|---|
| Gate report `commit_sha` | `2b27aa5` (gate-report.json, 17 steps PASS, integration and promtool NOT_RUN, exit 77) |
| Last commit touching `libs services apps deploy` | `3a40db8` test(antibot): pin the Amendment 2 revision defaults |
| `git diff --stat 3a40db8 63bde04 -- . ':!docs'` | empty: `72a9238`, `2b27aa5`, `63bde04` touch only `docs/` |
| Verdict | gate tree == last code tree: OK |

Baseline at HEAD (`verify-logs/baseline-*.log`, `ac-*.log`): `libs/go/antibot/...` ok (293 PASS lines),
`services/waitingroom/...` ok (250 PASS lines), web antibot and session tests 5 files / 43 tests pass.
The only SKIP is `TestCorpus_AC18_RecordedSessions` (pending input).

## 2. Acceptance criteria

V = verified here by running the tests. P = partly verified: the unit part passed, and the rest needs
integration (Docker down) or human input. NV = not verified.

| AC | Proving tests run here (all PASS) | Status | What is not verified |
|---|---|---|---|
| AC-1 | antibot-signals.test.ts, waiting-room.antibot.test.tsx, event-action-panel.antibot.test.tsx, fixround1 (re-join keeps snapshot, logout clears) | V | snapshot clearing when sold-out is reached by polling is untested (M61) |
| AC-2 | antibot-signals.test.ts (passive, caps, pointermove not in iv, sq floor, <= 2 KiB, probes once), fixround1 X-8 auto-repeat | V | none |
| AC-3 | TestDecodeSignals_AC3_* (9), golden_test, TestGuard_AC3_InvalidSignalsMetric, TestAntibotJoin_AC3_BadBodiesStillJoin, FuzzDecodeSignals seeds | P | TestAC3_InvalidSignalsCountedEndToEnd [integration] |
| AC-4 | TestRule_AC4_* (11), TestEvaluate_AC4_*, FuzzDecodeSignals / FuzzEvaluate (gate fuzz.log PASS) | V | none |
| AC-5 | TestEvaluate_AC5_P99: pure engine p99 = 50 µs (< 0.2 ms) | P | Decide p99 on real Redis (sequential and 64 goroutines) [integration]; load gate opening-spike.js develop vs branch |
| AC-6 | TestGuard_AC6_ObserveGetsHashPrefixAndSq, TestKeys_R31_AllKeysDeclaredUnderOneTag (16 keys, one `{ev}` tag) | P | TestStore_AC6_* (one script call, caps, TTLs) [integration] |
| AC-7 | TestGuard_AC7_* (timeout, error, panic, breaker, concurrent), TestEvaluate_AC7_*, TestFix1_X1/X2_* | P | CLIENT PAUSE and Redis-down tests [integration]; half-open single probe is not pinned (M16) |
| AC-8 | TestScore_AC8_* (table, noisy-OR rounding, lower-inclusive tiers, D11 cap), TestFix1_Amendment2_WorkedExamples | V | none |
| AC-9 | TestAntibotJoin_AC9_PassIsDevelop | P | TestLane_AC9_NormalLaneIsDevelop [integration] |
| AC-10 | TestScore_AC10_AC13_AppliedMatrix | V | none |
| AC-11 | TestScore_AC11_RejoinKeepsSlotUnlessBlock, TestAntibotJoin_AC11_SlowLaneSameResponse, TestAntibotJoin_P4_* | P | lane, shuffle, fairness and P-13 ordering tests [integration] |
| AC-12 | TestScore_AC12_Sticky, TestGuard_AC12_StickyBlocks, TestAntibotJoin_AC12_*, TestAntibotJoin_D6_JoinLaneBlocked, TestFix1_R2_StickyDoesNotRevokeAgain | P | TestLane_AC12_RevokeEverywhere, revoke.lua NX, TestAC12_StickyEndToEnd [integration] |
| AC-13 | TestGuard_AC13_ShadowAlwaysPasses, TestAntibotJoin_AC13_OffIsDevelop / ShadowProceeds, TestBuildGuard_AC13_OffIsUntypedNil | P | shadow is not pinned to the 4-arg `Join` path that contracts section 2 requires (M47) |
| AC-14 | TestGuard_AC14_SinkSelection, TestScore_AC14_FiredSorted, TestDecisionWriter_AC14_* (4), corpus: every BLOCK lists a rule | P | RowShape, NonUUIDAsNull, BoundedBuffer [integration]; the PASS→ALLOW mapping is pinned only there (M53) |
| AC-15 | TestFPHash_AC15_IsTruncatedHMAC, TestNetPrefix_AC15, TestEvaluate/Guard_AC15_*, TestAntibotJoin_AC15_LogsHaveNoRawValues | P | Redis key and DB row privacy [integration] |
| AC-16 | TestLoadConfig_AC16_* (7), TestFix1_R5_ProductionRefusesDevKeys; gate k8s.log PASS (both overlays, policy with the R-6 exception) | V | none |
| AC-17 | TestGuard_AC17_DecisionMetrics plus the counter tests; alerts AntibotDegraded / Slow / BlockSpike / NetSuppressed are in alerts.yml | P | promtool NOT RUN. The alert cases are in `alerts_antibot_test.yml`, not appended to `alerts_test.yml` as traceability says |
| AC-18 | TestCorpus_AC18_DefaultConfig: bots caught 95.80% (958/1000), humans demoted 0.10% (5/5000), every BLOCK lists a rule; both-sides and sensitivity tables printed | P | TestCorpus_AC18_RecordedSessions SKIP (the 50-100 recorded sessions are pending). Family replay_residential is 0% caught and replay_proxy_pool 85%: accepted limitation X-3 |
| AC-19 | none (integration only) | NV | TestAC19_BotScriptAndHumanReplay [integration] |
| AC-20 | not a test | NV | rollout go/no-go: Secret present, same image, shadow demotion < 1% on real traffic |

## 3. Mutation testing

Method: `verify-logs/mutants/<id>.log`. For each mutant, one exact-string edit was applied, the suite was
run, and the original bytes were written back. After the run, `git status` shows only `verify-logs/` as untracked.

Suites:
- G = `go test -count=1 ./antibot/...` in `libs/go`
- W = `go test` for the waitingroom http, cmd/server and postgres packages
- WEB = the 5 vitest files

Survived mutants are in bold.

| # | Mutant | Suite | Result | Killed by |
|---|---|---|---|---|
| M01 | D11 CAPPED cap dropped (missing-only reaches BLOCK) | G | killed | TestFix1_Amendment2_WorkedExamples |
| M02 | CAPPED tag missing | G | killed | WorkedExamples |
| M03 | REJOIN removed (slot holder demoted / new slot) | G | killed | TestScore_AC11_RejoinKeepsSlotUnlessBlock, AC8 table |
| M04 | REJOIN also shields BLOCK | G | killed | AC11_RejoinKeepsSlotUnlessBlock |
| M05 | BLOCK eligible without a positive sign (missing R5) | G | killed | WorkedExamples |
| M06 | BLOCK tier `>=` → `>` | G | killed | WorkedExamples (X-4 0.85) |
| M07 | CHALLENGE tier `>=` → `>` | G | killed | AC8_TierBoundariesLowerInclusive |
| M08 | tier from the unrounded score | G | killed | AC8_TierBoundariesLowerInclusive |
| M09 | R4N suppression off | G | killed | TestRule_AC4_R4N/suppressed_prefix |
| M10 | R7 sn/sb floors dropped | G | killed | TestRule_AC4_R7 |
| M11 | R7 `>= MinIDs` → `>` | G | killed | AC4_R7/3_identities |
| M12 | decoder cap 4096 → 4097 | G | killed | AC3_InvalidPayloadReasons/over_4_KiB |
| M13 | per-field lenient → strict (bad rt invalidates the whole payload) | G | killed | AC3_OutOfRangeFieldDoesNotInvalidatePayload |
| M14 | `recover()` removed | G | killed | TestGuard_AC7_PanicPasses |
| M15 | breaker counts a parent cancellation | G | killed | TestFix1_X1_CancelDuringObserveIsNotCounted |
| **M16** | **half-open lets several probes through** | G | **SURVIVED** | see below |
| M17 | breaker never opens | G | killed | X2_LastingSlowness, X2_HalfOpenProbe, AC7_Breaker |
| M18 | no context timeout around Observe | G | killed | the suite hangs (timeout 600 s, rc 124) |
| M19 | invalid-signals metric not incremented | G | killed | AC3_InvalidSignalsMetric |
| M20 | BodyErr ignored | G | killed | AC3_InvalidSignalsMetric |
| M21 | sink offered on every PASS | G | killed | AC14_SinkSelection |
| M22 | shadow enforces (Applied = tier) | G | killed | AC13_ShadowAlwaysPasses, AppliedMatrix |
| M23 | STICKY score != 1 | G | killed | AC12_Sticky (score and guard) |
| M24 | CHALLENGE fallback ignored | G | killed | AC10_AC13_AppliedMatrix |
| M25 | privacy: full IPv4 kept (/32) | G | killed | corpus suites (R4N grouping) |
| M26 | privacy: full IPv6 kept (/128) | G | killed | TestNetPrefix_AC15 |
| M27 | FPHash key unused | G | killed | TestFPHash_AC15_IsTruncatedHMAC |
| M28 | FPHash length 40 | G | killed | FPHash_AC15, FuzzEvaluate seed |
| M29 | production accepts the dev key | G | killed | TestFix1_R5_ProductionRefusesDevKeys |
| M30 | production accepts a missing key | G | killed | AC16_InvalidValueNamesVariable |
| M31 | `off` validates the config | G | killed | AC16_OffSkipsValidation |
| M32 | config error message lacks the variable name | G | killed | AC16_InvalidValueNamesVariable |
| M33 | R5 UA not lower-cased | G | killed | AC4_R5Strong/case-insensitive |
| M34 | empty UA not strong | G | killed | AC4_R5Strong/empty_UA |
| M35 | mobile fc exemption dropped | G | killed | AC4_R5Weak (iPhone, Android) |
| M36 | R5W fires on one weak sign | G | killed | AC4_R5Weak, WorkedExamples |
| M37 | R6 fires on dormant -1 | G | killed | corpus, WorkedExamples |
| M38 | R1 client `<` → `<=` | G | killed | AC4_R1Client/CV_exactly_0.10 |
| M39 | R2 `<` → `<=` | G | killed | AC4_R2/rt_120 |
| M40 | R4D `>` → `>=` | G | killed | AC4_R4D/3_identities |
| M41 | score not rounded | G | killed | AC8_NoisyOrRounded |
| M42 | invalid zone counted as a disagreement | G | killed | AC4_R5Weak/invalid_zone |
| M43 | UA cut 512 → none | W | killed | TestAntibotJoin_Contracts_InputBuiltFromRequest |
| M44 | BLOCK without Revoke | W | killed | AC12_BlockDeniesAndRevokes |
| M45 | SLOW_LANE uses the normal lane | W | killed | AC11_SlowLaneSameResponse |
| M46 | PASS (enforce) uses the slow lane (PASS path differs from develop) | W | killed | AC9_PassIsDevelop |
| **M47** | **shadow PASS goes through `JoinLane(n)` instead of the develop `Join`** | W | **SURVIVED** | see below |
| M48 | XFF last element used | W | killed | Contracts_ClientIP |
| M49 | BLOCKED state from JoinLane returns 200 | W | killed | D6_JoinLaneBlocked |
| M50 | STICKY revokes again | W | killed | TestFix1_R2_StickyDoesNotRevokeAgain |
| M51 | oversize body rejected (413) | W | killed | AC3_BadBodiesStillJoin/over_4_KiB |
| M52 | `off` returns a typed nil guard | W | killed | TestBuildGuard_AC13_OffIsUntypedNil |
| **M53** | **PASS stored as 'PASS' instead of 'ALLOW'** | W | **survives the unit suites** | only the RowShape integration test pins it (NOT RUN) |
| M54 | nil pool drop not counted | W | killed | AC14_NoPoolDropsAndCounts |
| M55 | web: key auto-repeat enters iv | WEB | killed | fixround1 X-8 / R-7 |
| M56 | web: listeners not passive | WEB | killed | AC-2 passive (2 tests) |
| M57 | web: pointermove enters iv | WEB | killed | AC-2 pointermove |
| M58 | web: count cap 10000 → 100000 | WEB | killed | AC-2 caps |
| M59 | web: snapshot not cleared on logout / account switch | WEB | killed | fixround1 X-6 (2 tests) |
| M60 | web: `{}` sent instead of `{"v":1}` | WEB | killed | waiting-room.antibot no-snapshot |
| **M61** | **web: snapshot not cleared when SOLD_OUT is reached by polling** (`waiting-room.tsx:137` effect removed) | WEB | **SURVIVED** | see below |

Kill count: **57 / 61** killed. 3 survive all runnable suites (M16, M47, M61). M53 is killed only by integration, which was not run.

### Survivors: none is equivalent

- **M16 half-open probe** (`guard.go` `allow()`, the `if b.probing` check removed).
  - The intent is tested: `TestFix1_X2_HalfOpenProbe` asserts "exactly one probe". It still passes with the
    check removed. The blocked probe runs under the default 4 ms Redis budget, so it fails and re-opens the
    breaker before the test's 20 ms sleep ends. The concurrent calls therefore meet an open breaker, never a
    half-open one.
  - This is behaviour-changing: on real traffic, every join in the probe's in-flight window (up to 4 ms at
    the opening spike) would hit a Redis that is already failing.
  - Fix: run the test with `ANTIBOT_REDIS_TIMEOUT_MS` well above the sleep (for example 200), or block the
    probe without honouring ctx.
- **M47 shadow join path** (`antibot.go`: `if !hasLanes || hasTag(d, MODE_SHADOW)` → `if !hasLanes`).
  - Contracts section 2 requires shadow PASS to use the develop `Join`. `TestAntibotJoin_AC13_ShadowProceeds`
    explicitly also accepts `JoinLane(normal)`.
  - This is not equivalent: the lane path checks the 24 h block flag (join.lua `lane and KEYS[8]`). After an
    enforce → shadow demotion, which is itself a rollback step, an identity flagged in the last 24 h would get
    403 in shadow, which breaks AC-13 ("every join proceeds").
  - Fix: assert `js[0].method == "Join"` in the shadow test.
- **M61 sold-out by polling.**
  - `waiting-room.tsx:137` clears the snapshot when the status poll turns SOLD_OUT. The only sold-out test
    covers the join response, which is cleared at line 72.
  - This is behaviour-changing: the snapshot outlives the event (AC-1 says it is cleared on leaving the event).
    Privacy impact is low, because the snapshot is tab-scoped and logout still clears it.
  - Fix: add a test where the token exists and the status poll returns SOLD_OUT.
- **M53 PASS → ALLOW.** The integration test RowShape covers it. A unit test on `dbAction`, or the integration
  run in CI, is needed before merge.

### Redis Lua mutants: not run

These need real Redis (`testx.Redis` uses testcontainers, and Docker is down). Adding miniredis temporarily to
`libs/go/go.mod` was refused by the permission system, so no scratch emulator was used:

- revoke.lua without `NX`
- join.lua slow post-T0 with `m = 0`
- observe.lua counting re-joins (`hasSlot == 0` guards removed)
- observe.lua window from Redis TIME
- ZSET cap not applied
- shuffle.lua early return or `lobby_slow` not deleted
- status.lua not reporting `lobby_slow`

Each is covered by a named integration test in traceability.yaml (TestLane_*, TestStore_*,
lane_fixround1_integration_test.go). Earlier TCR evidence (test-change-request.md) records the frozen
integration files run against miniredis in a scratch module, but this verifier did not reproduce that.

## 4. Rollback

**`ANTIBOT_MODE=off` keeps the join on develop's path:**
- `cmd/server/guard.go` returns an untyped nil (`TestBuildGuard_AC13_OffIsUntypedNil`, PASS). M52 kills a typed nil.
- In `handler.go`, `h.bots == nil` runs develop's code verbatim: decode body, then `store.Join` with 4 args.
  `TestAntibotJoin_AC13_OffIsDevelop` (PASS) checks the develop key set, the jti binding and exactly one
  4-arg `Join`.
- `LoadConfig` skips validation for `off` (`AC16_OffSkipsValidation`, PASS, killed by M31). A bad config
  therefore cannot block a rollback, and the k8s hash-key ref is `optional` (R-6, human-accepted).
- Redis side, by inspection: `Queue.Join` now sends 8 keys (adds `lobby_slow` and `ab:{ev}:blk:{id}`) but no
  lane argument. join.lua then skips the flag check (`lane and KEYS[8]`) and uses the main lobby and develop's
  score. The only difference is the `SISMEMBER lobby_slow` lookup for an existing token. That can return
  LOBBY only for a slot placed in the slow lane during an earlier enforce period.
  `TestLane_AC9_NormalLaneIsDevelop` [integration] is NOT RUN.

**Reverting the merge leaves no DB residue requirement.** The `bot_decisions` table is in
`migrations/0001_core.sql` (from develop's initial commit `13882a2`), and the branch adds no migration
(`git diff 918667c HEAD -- migrations` is empty). Rows written in shadow stay valid under develop's schema.

**Redis residue: every new key expires.**

| Key | TTL |
|---|---|
| `ab:{ev}:fp/net/sq:*` | `PEXPIRE 2 × window` (R4 300 s → 600 s; R7 900 s → 1800 s) |
| `ab:{ev}:jt:{id}` | `PEXPIRE 300 000 ms` |
| `ab:{ev}:jc:*`, `ab:{ev}:nj:*` | `EXPIRE 360` |
| `ab:{ev}:blk:{id}` | `EX 86400 NX` (24 h) |
| `wr:{ev}:lobby_slow` | session TTL, like the main lobby; deleted by shuffle |
| `wr:{ev}:meta` | gains a `slow_lottery_size` field; lives and dies with develop's key |

Develop code never reads `ab:*`.

**Caveats:**
- Slots revoked by BLOCK are not restored by `off` or a revert (AC-20, accepted).
- If the branch is reverted after `enforce` and before T0, members of `lobby_slow` are not placed by develop's
  shuffle.lua. Shadow never uses the slow lane, so this applies only to a revert after enforce. Recommend
  reverting only after the event's shuffle, or setting `off` (which keeps the branch's shuffle.lua) instead.

## 5. NOT VERIFIED

- All `-tags integration` suites. Docker is down, and gate-report.json also shows integration as NOT_RUN
  (exit 77). This covers lane, shuffle, fairness and P-13 ordering, revoke and the 24 h flag, observe.lua
  caps/TTLs/one call, the RowShape/NULL/bounded-buffer writer tests, and AC-19.
- promtool alert tests (`alerts_antibot_test.yml`), NOT RUN at the gate either.
- AC-5 Decide p99 on a real Redis. Earlier evidence only has miniredis concurrent p99 5.68 ms (a note, not
  real Redis, and over the 5 ms bound), and the opening-spike load gate (develop vs branch in shadow) was not run.
- AC-18 recorded sessions: `TestCorpus_AC18_RecordedSessions` SKIP, waiting on the team's 50-100 sessions.
- AC-20 shadow demotion < 1% on real traffic, Secret present, and the same image for waitingroom and the
  controller.
- E2E and full-gate.sh were not re-run here (gate at `2b27aa5` PASS; the code tree is identical).

## 6. Required to reach PASS

Each item needs a TCR, because the test files are frozen.
1. M16: make `TestFix1_X2_HalfOpenProbe` hold the probe in flight (set `ANTIBOT_REDIS_TIMEOUT_MS` above the
   sleep, for example `200`).
2. M47: `TestAntibotJoin_AC13_ShadowProceeds` must require `method == "Join"`. Optionally add a laneStore case
   where JoinLane would return BLOCKED.
3. M61: add a waiting-room test where SOLD_OUT arrives from the status poll and the snapshot is removed.
4. M53: run the integration suite in CI (or add a unit test of `dbAction`) before merge.

## 7. Open human items

- Approve the TCR for items 1-3 above, or explicitly accept the three survivors as residual test debt.
- Run the integration suites and promtool on a machine with Docker before merge. Phase 1 merges as shadow,
  which is safe if these are deferred, but enforce is not.
- Record AC-18 sessions, and run the AC-5 load gate and real-Redis p99. The miniredis 5.68 ms concurrent figure
  is above the 5 ms target and needs a real-Redis number.
- AC-20 go/no-go before enforce: Secret key present, same image for waitingroom and the controller, and shadow
  demotion < 1% over a rehearsal on-sale.
- Note the documentation drift: traceability AC-17 says the alert cases are appended to `alerts_test.yml`, but
  they are in `alerts_antibot_test.yml`.
