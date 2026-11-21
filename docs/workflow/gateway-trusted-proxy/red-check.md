# Red check: gateway-trusted-proxy (v3 step 2a)

Base: `b946bf8` (baseline in `baseline.json`: 31 tests + 10 subtests, all pass, vet clean).
Command: `cd services/gateway && go vet ./... ; go test -count=1 ./...`

New test files (existing `gateway_test.go` and `config_test.go` are byte-identical to b946bf8, checked with
`git diff --quiet b946bf8 -- ...`):

- `services/gateway/internal/clientip/clientip_test.go` (package `clientip`, internal, R2-3)
- `services/gateway/internal/config/config_trusted_test.go` (package `config_test`)
- `services/gateway/internal/proxy/trusted_proxy_test.go` (package `proxy_test`)

## Result (red, for the right reason)

| Package | Result | Reason |
|---|---|---|
| internal/clientip | build failed | `undefined: New`, `peerKey`, `ParseTrusted`, `Resolver` (package has no production file yet) |
| internal/config | build failed | `c.TrustedProxies undefined (type *config.Config has no field or method TrustedProxies)` |
| internal/proxy | build failed | `unknown field TrustedProxies in struct literal of type config.Config` |
| internal/ratelimit | ok | unaffected |

`go vet` reports the same three missing identifiers and nothing else, so once they exist the existing tests in
config/proxy compile unchanged.

## Sanity checks by the test designer (no production code committed)

1. **clientip tests vs a throwaway reference implementation** (scratchpad only, never in the repo): `TestResolve`
   (66 rows), `TestParseTrusted`, `TestPeerKeyMatchesDevelop`, `TestResolve_ForgedPrefixNeverChangesResult` pass;
   `FuzzResolve` ran 20 s / 6.5 M execs with no failure (the oracle agrees with a spec-conforming walk); the
   benchmark runs. Mutations of the reference killed: M2 skip peer trust, M3 skip malformed instead of returning
   the peer, M4 drop `Unmap`, M5 accept `/0`, M7 remove the 16-hop cap, M8 drop the host-bits check. M1
   (left to right) is killed by the AC-4 rows.
2. **proxy tests vs develop behaviour**: with only a temporary `TrustedProxies []netip.Prefix` field added to
   `config.Config` (reverted; `git status` showed only the new test files):
   - PASS, regression guards that must stay green: AC-1 (5 peers incl. zoned, mapped, `garbage`), AC-2, AC-9 rows
     whose expected XFF is the peer (untrusted, malformed rightmost, no XFF, empty elements, 17 hops, zoned peer,
     mapped peer without XFF, unparsable RemoteAddr = no XFF per R2-1), AC-10 id and queue-poll rows.
   - FAIL for the right reason (feature missing): AC-3, AC-4, AC-9 rows whose expected XFF is the client
     (`upstream X-Forwarded-For = ["10.1.2.3"], want exactly ["198.51.100.1"]`), AC-10 auth-by-resolved-IP and
     anonymous-default-by-resolved-IP (`429, want 200`). M6 (no XFF override in `Rewrite`) is therefore killed by
     AC-3/AC-4/AC-9.

## Not covered by an automated test

- AC-8 "`proxy.New` is never reached": `cmd/server/main.go` already returns on any `config.Load` error; covered by
  `config.Load` returning `(nil, err)` (asserted) plus inspection.
- AC-9 "`RemoteAddr` seen by handlers is unchanged": not observable from outside; inspection in review.
- Startup log `trusted_proxies` (spec 3.1): inspection.
