# Test change request — antibot-phase1

Status: **open** (needs the test designer / human; the implementer has not edited any frozen test)

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
