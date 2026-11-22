# Test change request — antibot-phase1

Status: **approved** by TranDinhVo in the Claude Code session on 2026-11-22 (TCR-1..TCR-4 as written); applied by the coordinator session.

## TCR-1: `TestStore_AC6_WindowsUseInputNow` asserts an identity outside the R7 window is still counted

File: `libs/go/antibot/redisstate/store_integration_test.go` (frozen at 36aab08), lines ~128-140.

```go
st = e.observe(t, "y", 1202*time.Second, fph, pfx, "00000000000000bb")
if st.SeqIdentities != 2 { // z (301 s) still inside, a/b/c expired
```

`z` was observed at 301 s. At 1202 s it is 901 s old, and the R7 window is 900 s (contracts 3
`R7_WINDOW_S=900`, contracts 4 `ab:{ev}:sq:{sq}` "same as fp" trimming). The comment says z is "still inside"; by
the contract it is 1 s outside. The test's own in-memory emulator in `corpus_test.go` (`zadd`: drops `v < now-window`)
would also return 1 here. The red check never ran this file (no Docker), so the arithmetic was not caught.

Evidence: the frozen integration file was run against an in-memory Redis with Lua (miniredis, scratch module, not
committed). Every other `TestStore_*` test passes; this one reports `sq after 1202 s: 1, want 2`.

Proposed change (keeps the intent: a/b/c expired, z inside, y new): observe `y` at **1200 s** instead of 1202 s
(z is then 899 s old, a/b/c 1200 s old).

```diff
-	st = e.observe(t, "y", 1202*time.Second, fph, pfx, "00000000000000bb")
-	if st.SeqIdentities != 2 { // z (301 s) still inside, a/b/c expired
-		t.Fatalf("sq after 1202 s: %d, want 2", st.SeqIdentities)
+	st = e.observe(t, "y", 1200*time.Second, fph, pfx, "00000000000000bb")
+	if st.SeqIdentities != 2 { // z (301 s) still inside, a/b/c expired
+		t.Fatalf("sq after 1200 s: %d, want 2", st.SeqIdentities)
```

The implementation follows the contract (900 s window, entries older than `now - window` removed) and is not
changed to fit the test.

## TCR-2: `TestAC19_BotScriptAndHumanReplay` sends non-UUID identity / device ids, so every join is 401

File: `services/waitingroom/test/antibot/antibot_bot_test.go` (frozen at 36aab08), `(*env).join`, with ids from
`libs/go/antibot/corpus/corpus.go` (`g.id`: `hum-000001`, `bdev-000002`, ...).

`authx.PrincipalFromHeaders` rejects an `X-Identity-Id` or `X-Device-Id` that is not a UUID (`uuidRe`), and the join
handler then answers 401 before antibot runs. The test's first join fails with
`join status 401: {"detail":"can dang nhap truoc khi xep hang",...}`. `TestAC12_StickyEndToEnd` and
`TestAC3_InvalidSignalsCountedEndToEnd` already use UUIDs and pass.

Evidence: run against an in-memory Redis with Lua (miniredis, scratch copy of the module, not committed). With
one change in the scratch copy only (the header values mapped to a deterministic UUID per corpus id), the test
passes: bots caught 958/1000 (95.8%), humans demoted 5/5000 (0.10%), the same as the pure-engine corpus test.

Proposed change (test side, corpus untouched):

```diff
-	req.Header.Set(authx.HeaderIdentityID, s.IdentityID)
+	req.Header.Set(authx.HeaderIdentityID, asUUID(s.IdentityID))
 	...
-	req.Header.Set(authx.HeaderDeviceID, s.DeviceID)
+	req.Header.Set(authx.HeaderDeviceID, asUUID(s.DeviceID))
```

with a helper that maps each corpus id to a distinct, stable UUID (for example the hex of its bytes, left-padded to
32 digits and formatted 8-4-4-4-12 with version 4 / variant 8), returning ids that already are UUIDs unchanged.
Alternative: make `corpus.(*gen).id` emit UUID-shaped ids (changes `corpus.go`, also frozen).

## TCR-3: `antibot-signals.test.ts` cannot load the golden fixture path under Vite

File: `apps/web/src/lib/antibot-signals.test.ts` (frozen at 36aab08), lines 13-15.

```ts
const GOLDEN = fileURLToPath(
  new URL("../../../../libs/go/antibot/testdata/signals/scripted_keyboard.json", import.meta.url),
);
```

Vite rewrites the `new URL("<relative>", import.meta.url)` asset pattern, so in vitest (jsdom) the URL becomes
`http://localhost:3000/@fs/D:/.../scripted_keyboard.json` and `fileURLToPath` throws
`TypeError: The URL must be of scheme file` while the module loads. The whole file then reports "0 test" and fails.
The red check did not reach this line (the import of `./antibot-signals` failed first). The rewrite is done by
Vite's asset plugin, not by the OS, so CI fails the same way.

Evidence: `import.meta.url` itself is `file:///…/antibot-signals.test.ts`; only the `new URL(…, import.meta.url)`
form is rewritten (checked with a throwaway test, deleted). A temporary copy of the frozen file with only this
constant changed (not committed) passes all 17 tests against the implementation.

Proposed change (resolve the path without the Vite-rewritten pattern):

```diff
-import { fileURLToPath } from "node:url";
+import { dirname, resolve } from "node:path";
+import { fileURLToPath } from "node:url";
 ...
-const GOLDEN = fileURLToPath(
-  new URL("../../../../libs/go/antibot/testdata/signals/scripted_keyboard.json", import.meta.url),
-);
+const GOLDEN = resolve(
+  dirname(fileURLToPath(import.meta.url)),
+  "../../../../libs/go/antibot/testdata/signals/scripted_keyboard.json",
+);
```

## TCR-4: golangci-lint (CI `lint-go`, v2.12.2) fails on the frozen corpus fixture and corpus test

`golangci-lint run ./...` in `libs/go` reports 5 issues, all in files added by 36aab08; the implementation files
report 0 (and `services/waitingroom` reports 0 issues). CI's `lint-go` job runs this exact command, so it fails
until these lines change:

| File (frozen) | Line | Linter | Issue | Proposed change |
|---|---|---|---|---|
| `libs/go/antibot/corpus/corpus.go` | 233 | errcheck | `fmt.Fprintf(h, "rtb:%d", rtb)` unchecked | `_, _ = fmt.Fprintf(...)` |
| `libs/go/antibot/corpus/corpus.go` | 238 | errcheck | `fmt.Fprintf(h, "\|%s:%d", e.kind, b)` unchecked | `_, _ = fmt.Fprintf(...)` |
| `libs/go/antibot/corpus/corpus.go` | 321 | gocritic ifElseChain | `if autorepeat {...} else if ...` | rewrite as `switch` |
| `libs/go/antibot/corpus_test.go` | 124 | staticcheck QF1002 | `switch {` on `d.Applied` | `switch d.Applied {` |
| `libs/go/antibot/corpus_test.go` | 471 | staticcheck S1009 | `j.Signals != nil && len(j.Signals) > 2048` | drop the nil check |

All are behaviour-neutral (the generator output and every assertion stay the same). Not changed by the implementer.

## TCR-5 (approved, formatting only)

- Approved by TranDinhVo in the Claude Code session on 2026-11-22; applied by the coordinator session.
- `gofmt -w` on `services/waitingroom/internal/adapter/redis/lane_integration_test.go` (one-line `afterT0`/`beforeT0`) and `services/waitingroom/test/antibot/antibot_bot_test.go` (one-line `store.Revoke`). Line breaks only; no assertion changed.

## TCR-6 (approved, R4D = 0.60 variant): Amendment 2 defaults R5 = 0.85 and the weak HTTP-library UA list

- Approved by TranDinhVo on 2026-11-22 (spec.md "Amendment 2, revision"): X-6 withdrawn (R4D stays 0.60); X-4
  (R5 = 0.85) and X-5 (`okhttp`, `Go-http-client`, `axios/`, `node-fetch` move from the R5 deny list to the R5W weak
  list) become built-in defaults, conditional on the corpus gate. Approved for exactly the frozen assertions below.
- Corpus with exactly that configuration (before any frozen edit): **bots caught 95.80% (958/1000), humans demoted
  0.10% (5/5000)** — gate holds. Per family: raw_http 200/200, headless 200/200, stealth_rhythm 200/200,
  account_farm 188/200, replay_proxy_pool 170/200 (replay_residential 0/50, not gated). Identical to revision 3.

Frozen assertions that encode the old R5 weight or deny list (all in `libs/go/antibot`, nothing else changes):

| Test (frozen) | Case | Before | After |
|---|---|---|---|
| `score_test.go` `TestScore_AC8_DefaultOutcomesTable` | missing payload, browser UA | R3,R5 0.88 SLOW_LANE CAPPED | 0.91, same tier/tags |
| same | missing payload, identity holds a slot | 0.88 | 0.91 |
| same | raw HTTP python-requests | 0.88 BLOCK | 0.91 BLOCK |
| same | headless default | R2,R5 0.88 BLOCK | 0.91 BLOCK |
| `rules_test.go` `TestRule_AC4_R5Strong` | deny entries `Go-http-client/1.1`, `okhttp/4.12`, `node-fetch/1.0`, `axios/1.7.2` | R5 fires | R5 does not fire (weak list; one weak sign alone fires nothing) |

No other frozen assertion changes (R4D stays 0.60; `BlockEligibilityCap` asserts action + ids only; the
`guard_test.go` "0.80" is a comment). The earlier R4D = 0.55 variant (91.0% bots caught) is withdrawn.

<details><summary>Superseded text (TCR-6 open, R4D = 0.55 variant)</summary>

### TCR-6 (open, superseded): spec Amendment 2 defaults conflict with frozen tests, and miss the corpus gate

Amendment 2 (approved) sets `R5=0.85`, `R4D=0.55` and moves `okhttp`, `Go-http-client`, `axios/`, `node-fetch`
from the R5 deny list to a weak list. The frozen tests assert the revision-3 values, so the implementer did **not**
change the built-in defaults (fix-round brief: stop and file a TCR). The mechanism is implemented
(`ANTIBOT_UA_WEAK`; weights through `ANTIBOT_WEIGHTS`) and `libs/go/antibot/fixround1_test.go` checks the Amendment 2
worked examples with that configuration; switching the defaults is then a two-line change in `config.go`
(`DefaultWeights`, `DefaultUADeny` / `DefaultUAWeak`).

**Blocker found while checking (needs a human decision before any tuning):** with the Amendment 2 configuration the
frozen corpus gate fails. `TestFix1_Amendment2_Corpus` (report only, not asserted):

| family | demoted (rev 3 defaults) | demoted (Amendment 2) |
|---|---|---|
| raw_http | 200/200 | 200/200 |
| headless | 200/200 | 200/200 |
| stealth_rhythm | 200/200 | 200/200 |
| account_farm | 188/200 | **140/200** |
| replay_proxy_pool | 170/200 | 170/200 |
| replay_residential (not gated) | 0/50 | 0/50 |
| **gated bots caught** | 95.80% (958/1000) | **91.00% (910/1000)** |
| humans demoted | 0.10% (5/5000) | 0.00% (0/5000) |

The drop is R4D 0.55: farm members that fire R4D alone (no R4N on their /24) now score 0.55 = CHALLENGE → pass.
Options for the human: keep R4D 0.60 (X-6 then needs another mitigation, e.g. the X-6 snapshot clearing already
done), accept a lower AC-18 target, or adjust the generator's farm family (frozen corpus.go).

Frozen assertions that change once the defaults change (proposed new expectations, all in `libs/go/antibot`):

| Test (frozen) | Case | Now asserts | With Amendment 2 |
|---|---|---|---|
| `score_test.go` `TestScore_AC8_DefaultOutcomesTable` | missing payload, browser UA | R3,R5 0.88 SLOW_LANE CAPPED | 0.91, same tier/tags |
| same | missing payload, slot holder | 0.88 | 0.91 |
| same | raw HTTP python-requests | 0.88 BLOCK | 0.91 BLOCK |
| same | headless default | R2,R5 0.88 BLOCK | 0.91 BLOCK |
| same | account farm | R4D,R4N 0.70 | 0.6625 (tol 0.0011) SLOW_LANE |
| same | shared fp only | R4D 0.60 SLOW_LANE | 0.55 CHALLENGE / applied PASS |
| `score_test.go` `TestScore_AC10_AC13_AppliedMatrix` | SLOW_LANE input `FPIdentities=4` | tier SLOW_LANE | CHALLENGE: needs another slow-lane input (e.g. R1+R2: `rt` 90 + fixed `iv`) |
| `score_test.go` `TestScore_AC8_BlockEligibilityCap` | missing + R1-server | R1,R3,R5 0.94 BLOCK | 0.955 BLOCK (unchanged assertion: action + ids only) |
| `rules_test.go` `TestRule_AC4_R5Strong` | deny entries `Go-http-client/1.1`, `okhttp/4.12`, `node-fetch/1.0`, `axios/1.7.2` | R5 fires | move these 4 to a weak-sign case (R5 false; R5W only with a second weak sign) |
| `guard_test.go` `TestGuard_AC14_SinkSelection` | `wd` true | "0.80" comment only | 0.85 (comment) |
| `corpus_test.go` `TestCorpus_AC18_DefaultConfig` | gate | >= 95% | fails at 91.0% (see above) |

Also `contracts.md` §3.1 (revision 4) records the Amendment 2 table as approved but not yet the default.

</details>

## Note (fix round 2, R2-4): no new TCR needed

Re-review R2-4 asked about the two frozen files reformatted after the freeze
(`services/waitingroom/internal/adapter/redis/lane_integration_test.go`,
`services/waitingroom/test/antibot/antibot_bot_test.go`). Both changes are line breaks only and are covered by the
approved **TCR-5** (commit 82252a4); `full-gate.sh` lists 82252a4 in `TCR_COMMITS` and its tamper step checks that
no other commit touched them. Fix round 2 adds only new test files; no frozen test changes.
