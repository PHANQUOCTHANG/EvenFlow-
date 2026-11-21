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
