# gateway-trusted-proxy: plan review, round 1 (v3 step 2b, High-risk)

| Item | Value |
|---|---|
| Reviewer | Independent plan reviewer (clean context) |
| Input | `spec.md`, `plan.md` (this folder) |
| Code checked | `services/gateway/internal/proxy/gateway.go` (`rateKey`, `clientIP`, `newReverseProxy`), `internal/proxy/gateway_test.go` (fixture, AC-1 spoof test), `internal/config/config.go`, `cmd/server/main.go`, identity `newDevice`, payment `vnp_IpAddr` reader |
| Base | `feature/gateway-trusted-proxy` (develop @ c5fb598) |
| Date | 2026-11-21 |

## Verdict: **FAIL** (round 1)

There are no blockers. The design is sound: the right-to-left walk, the 16-hop cap, `/0` rejection, `Rewrite` kept and a
context-carried override all hold up. Two **major** findings must be fixed in `plan.md` before tests are written.
P-1 is a real AC-1 regression with a per-connection rate-limit bypass. P-2 is a test helper that cannot be built
without touching the frozen `gateway_test.go`. Both fixes are small. Re-review only needs to look at P-1 and P-2.

## 1. AC coverage

| AC | Plan step | Test | OK? |
|---|---|---|---|
| AC-1 | gateway.go wiring, no override when trust is off | `TestResolve` rows + `trusted_proxy_test` AC-1 + frozen spoof test | Only after **P-1** (peer fallback differs from develop) |
| AC-2 | `Resolve` step 2 | unit + integration (11 login POSTs) | Yes, but needs **P-2** for the peer |
| AC-3 | walk + rateKey | integration A/B + `Retry-After` | Yes (P-2) |
| AC-4 | right-to-left walk | unit + integration | Yes (P-2) |
| AC-5 | walk | unit | Yes |
| AC-6 | `parseAddr` | unit | Yes; multi-line also through the gateway is a nice-to-have (P-6) |
| AC-7 | walk + hop cap + step 1 | unit (16 vs 17) | Yes; step 1 wording per P-1 |
| AC-8 | `ParseTrusted` + `config.Load` | `TestParseTrusted` + config test | Yes; bare-IP rules need detail (P-4). "`proxy.New` never reached" is shown by `main.go:30` returning before `New`; code reading is enough |
| AC-9 | `Rewrite` override from context | integration | Yes, except mapped untrusted peers (P-3) |
| AC-10 | `rateKey(rt, r, p, ip)` | integration | Yes. The key classes are unchanged (`ip:` for anon/auth, `id:` otherwise, `|eventID` for queue_poll) |
| AC-11 | — | `FuzzResolve` | Yes; invariant per P-1, input shape per P-5 |

## 2. Findings

### P-1 (major): the peer fallback is not byte-identical to develop, and a zoned peer gets a per-port key

Today `clientIP` returns `net.SplitHostPort(RemoteAddr)` host, and the raw `RemoteAddr` only when SplitHostPort fails.
The spec (3.2 "applies to every address compared or returned", 3.3 step 1 "does not parse → raw `RemoteAddr`") and
the plan (`parseAddr` rejects zones, fuzz invariant "result is the raw `RemoteAddr` or canonical") change this in two
ways:

- A link-local peer `[fe80::1%eth0]:51234` passes SplitHostPort but fails the zone-rejecting netip parse. It would
  return the **raw `RemoteAddr` with port**, so the key becomes `ip:[fe80::1%eth0]:51234`. Every new connection gets
  a fresh bucket, which bypasses the auth limit. On develop the key is `ip:fe80::1%eth0`.
- Trust is off (AC-1), but the peer is still normalised (`Unmap`, canonical form). For any peer whose host string
  is not already canonical, the key then drifts from develop. Go's listener mostly gives canonical hosts, but AC-1
  promises "exactly as on develop".

**Fix (plan + spec 3.3 step 1):** define `peerKey := host from net.SplitHostPort(RemoteAddr), or the raw RemoteAddr
if that fails` (today's `clientIP`, unchanged). `Resolve` returns `peerKey` whenever it falls back to the peer: trust
empty, peer not parseable by netip (zone included), peer untrusted, malformed XFF element, hop cap, empty XFF. Use the
netip-normalised peer **only for the `Contains` check**. A peer that does not parse is never trusted. Add rows: zoned
peer + trust on → `fe80::1%eth0` (no port); trust off + any input → `peerKey`. Change the AC-11 fuzz invariant to
"result == peerKey, or a canonical `Unmap().String()` address of an XFF element; peer untrusted ⇒ result == peerKey".
P-3 is resolved by the same change.

### P-2 (major): `newFixtureAs` cannot wrap the gateway handler without editing the frozen fixture

`newFixture` (`gateway_test.go:64`) calls `proxy.New` and `httptest.NewServer(h)` internally. `fixture` exposes only
`gw`, `up` and `now`, not `h`, so "a test wrapper handler that sets `r.RemoteAddr` before the gateway handler" has no
seam. Swapping `f.gw.Config.Handler` after the server starts is a data race under `-race`. With a loopback client the
peer is always `127.0.0.1`, so AC-2's untrusted `203.0.113.9` and AC-3's trusted `10.1.2.3` cannot be produced.

**Fix:** in `trusted_proxy_test.go` (package `proxy_test`), build a separate helper. It reuses the `upstreams`/`seen`
types and `token()`, which are fine to use, but builds its own `config.Config` (the same literal plus `TrustedProxies`
and a tight `RateAuth`, e.g. `1 rps / burst 10`). It calls `proxy.New(cfg, log, proxy.WithClock(...))` and serves
`httptest.NewServer(http.HandlerFunc(func(w, r){ r.RemoteAddr = peer; h.ServeHTTP(w, r) }))`. Another option is to
drive `h.ServeHTTP(httptest.NewRecorder(), req)` with `req.RemoteAddr` set directly. Either way `gateway_test.go`
stays byte-identical. State this in plan section 2.

### P-3 (minor): AC-9 "exactly one element equal to the resolved IP" fails for a mapped untrusted peer

The override is applied only when `fromXFF`. For an untrusted peer `[::ffff:203.0.113.9]:x`, `SetXForwarded` writes
`::ffff:203.0.113.9`. The spec's resolved value (unmapped) is `203.0.113.9`. **Fix:** with P-1 the resolved peer value
is `peerKey`, the same string `SetXForwarded` writes, so the mismatch goes away. Otherwise, apply the override whenever
the trusted set is non-empty.

### P-4 (minor): `ParseTrusted` bare-IP rules are under-specified

`netip.ParsePrefix` already rejects zones and `/33`, but for the bare-IP path `netip.ParseAddr` **accepts** zones
(`fe80::1%eth0`). A bare mapped IP (`::ffff:10.0.0.1`) is not covered by the spec at all. **Fix:** for bare IPs,
reject `addr.Zone() != ""` and `addr.Is4In6()` with the same "write the IPv4 form" hint (it could never match, since
addresses are unmapped). Check host bits with `p != p.Masked()` and `/0` with `p.Bits() == 0`. Add `::ffff:10.0.0.1`
to the invalid rows.

### P-5 (minor): the fuzz target does not cover multiple header lines

`FuzzResolve(f, xff string, peer string)` passes a single line. **Fix:** split the fuzzed `xff` on `\n` into the
`[]string` lines, so AC-6 multi-line ordering is fuzzed too. 30 s fits in the gate. It is a single package, the module
is `go 1.25.0` and the local toolchain is go1.27, so fuzzing is supported. Do not combine it with `-race`. If a crasher
is found, commit it under `testdata/fuzz/FuzzResolve/`.

### P-6 (minor): smaller items

- **Benchmark worst case:** the 16-hop cap does not bound a header of 1 MiB of empty elements (`,,,,`) or whitespace,
  because skipped empties are not counted. The cost stays linear and bounded by `http.Server` `MaxHeaderBytes`, which
  is acceptable. Benchmark that case as well as the trusted-element case, and state "linear in header size, no
  allocation per element" as the bound.
- **Context wiring:** `r = r.WithContext(withForwardedClient(r.Context(), ip))` must be assigned before the
  `MaxBytesReader` wrap and the `upstream.ServeHTTP(w, r)` call. `Rewrite` reads `pr.In.Context()`. Keep the context key
  unexported in `proxy`.
- **Frozen tests:** the plan adds cases to the existing `internal/config/config_test.go`. Put them in a new
  `config_trusted_test.go` so every pre-existing test file stays byte-identical, as `gateway_test.go` does.
- **Mutation set:** add (7) remove the 16-hop cap and (8) drop the host-bits check, so that every security rule in 3.1
  and 3.3 has a killing test. The threshold becomes 8/8.
- **AC-1 rows:** state explicitly that the AC-1 integration case also sends `X-Real-IP` and `Forwarded` (the spec
  lists them).
- **Open questions:** Q-1..Q-3 must be answered in the human sign-off before step 3. The plan assumes Q-3 = leftmost.
  Record that assumption.

## 3. Checks that passed

- **Walk is spoof-safe:** right to left; an untrusted element is returned immediately; a malformed element returns
  the peer; elements left of the result are never parsed, so a forged leftmost value (AC-4) has no effect. An
  untrusted peer never reads XFF (AC-2). The cap at 16 trusted hops plus the 17th → peer is consistent with step 5
  (exactly 16 → leftmost).
- **Upstream XFF:** `Rewrite` strips inbound `X-Forwarded-*`. `SetXForwarded` then writes only the peer host because
  `Out` has no XFF, so with trust off no override runs and the bytes are identical to develop. The frozen spoof test
  `TestGateway_XoaDanhTinhGiaMaoTrenMoiRoute` (loopback peer, no trust) stays green.
- **Identity and payment readers:** identity `newDevice` (`handler.go:53`) takes the first `,` element, then
  SplitHostPort, then trims. Payment (`handler.go:150`) takes the whole header. A single-element XFF is right for both,
  and a bare IPv6 address is handled the same as today. No other service reads XFF.
- **Rate-limit key classes:** unchanged. Only the IP source moves into a parameter.
- **Dependencies:** `net/netip` is stdlib. `config → clientip` and `proxy → clientip, config` form no import cycle.
  Adding `Config.TrustedProxies` does not break the keyed `config.Config{...}` literal in the frozen fixture. The
  `go mod tidy` diff should be empty.
- **Startup log:** `[]netip.Prefix` renders through `String()` in slog JSON. Fine.
- **Rollback:** a revert of the merge commit, or leaving the env empty, restores develop exactly. With P-1 applied,
  that also holds for odd peers.

## Round 2

| Item | Value |
|---|---|
| Input | `spec.md`, `plan.md` at `8cbccf2` (diff against `c141073`) |
| Date | 2026-11-21 |

### Verdict: **PASS** (round 2)

Both majors are resolved. P-3..P-6 are addressed. The revision introduced no blocker or major. Three new minor
findings (R2-1..R2-3) can be fixed during step 3 without another plan review.

### Re-check of round 1

- **P-1: resolved.** Spec 3.2 defines `peerKey` as today's `clientIP` (SplitHostPort host, else raw). 3.3 returns it
  on every fallback: trust empty, peer that netip cannot parse, untrusted peer, malformed element, hop cap, empty
  list. The netip form is used only for `Contains`. Each case traced against develop:
  - `[fe80::1%eth0]:51234` → `fe80::1%eth0` with trust on or off, so there is no port in the key and no bucket per
    connection.
  - `[::ffff:1.2.3.4]:5` → `::ffff:1.2.3.4`, as today.
  - `garbage` → raw `garbage`, as today.
  - The upstream XFF on every fallback is `SetXForwarded`'s SplitHostPort host, which is the same string as `peerKey`.

  AC-1, AC-6, AC-7 and AC-11 and the table rows match. The fuzz invariant now runs both a trusted set and an empty
  set.
- **P-2: resolved.** `newPeerFixture` lives in package `proxy_test`. It reuses `upstreams`, `seen`, `token()` and
  `secret` from the frozen file, all accessible there. It calls `proxy.New` itself behind a wrapper that sets
  `RemoteAddr`, and never touches `newFixture` or a running server, so there is no `-race` hazard. `gateway_test.go`
  and `config_test.go` are declared byte-identical.
- **P-3: resolved** by `peerKey`.
- **P-4: resolved.** The zone and `Is4In6` checks on bare IPs, `Masked()` and `Bits()==0` are in the spec, and
  `::ffff:10.0.0.1` is in AC-8.
- **P-5: resolved.** XFF lines are split on newline, the fuzz run uses no `-race`, and crashers go to `testdata/fuzz`.
- **P-6: resolved.**
  - The benchmark adds the empty-elements case, and the linear bound is written down.
  - `WithContext` comes before `MaxBytesReader`, with an unexported key.
  - The config tests are in a new `config_trusted_test.go`.
  - Mutations are 8/8.
  - AC-1 sends XFF, `X-Real-IP` and `Forwarded`.
  - Q-1..Q-3 are recorded as open, with defaults.

### New findings (minor)

- **R2-1 (minor): AC-9 vs an unparsable `RemoteAddr`.** AC-9 says "for AC-2..AC-7 inputs, the upstream receives
  exactly one XFF element". For the AC-7 row with an unparsable `RemoteAddr`, `SetXForwarded` **deletes** XFF, as
  develop does, so the upstream gets none. **Fix:** exempt that row in AC-9 ("upstream XFF absent, as develop"), or
  keep it as a `clientip` unit row only, not an integration case.
- **R2-2 (minor): a literal newline in `plan.md`.** Rows 25 and 69 contain a real newline inside the backticks of
  "split on `\n`", which breaks both markdown table rows. **Fix:** write `` `\n` `` (backslash-n) so each row stays on
  one line.
- **R2-3 (minor): peer normalisation and visibility.**
  - Spec 3.3 step 2 says "normalise the peer with netip" without the exact input. Pin it to `netip.ParseAddr(peerKey)`,
    then reject a zone, then `Unmap()`. A port-less `RemoteAddr` such as `10.1.2.3` then gives a deterministic
    result: it is trusted, and its fallback key is `10.1.2.3`.
  - `FuzzResolve` asserts on the unexported `peerKey`, so `clientip_test.go` must use the internal package
    `clientip`, not `clientip_test`.
  - The spec still names the identity reader `deviceFrom`. The function is `newDevice` (`handler.go:53`). This only
    affects the docs.
