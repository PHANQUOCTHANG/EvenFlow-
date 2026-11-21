# Plan: gateway-trusted-proxy (draft, step 2b)

Inputs: `spec.md` (AC-1..AC-11, awaiting human approval). Base develop @ c5fb598. Gateway module only, no new
module dependency (`net/netip` is stdlib), no migration. One work package: the parser, config and wiring are small
and share one type, so no parallel split.

## 1. Files

| File | Change | AC |
|---|---|---|
| `services/gateway/internal/clientip/clientip.go` (new) | `type Resolver struct{ trusted []netip.Prefix }`; `ParseTrusted(s string) ([]netip.Prefix, error)` (spec 3.1 rules); `New(trusted) *Resolver`; `(*Resolver).Resolve(remoteAddr string, xff []string) (string, bool)` (spec 3.2-3.3); unexported `parseAddr(string) (netip.Addr, bool)` (port/brackets/unmap/zone reject). Right-to-left scan with `strings.LastIndexByte` over the header lines from last to first, no full split. `maxTrustedHops = 16`. | 1-7, 11 |
| `services/gateway/internal/config/config.go` | `Config.TrustedProxies []netip.Prefix`; `Load` calls `clientip.ParseTrusted(os.Getenv("TRUSTED_PROXY_CIDRS"))`, wraps the error as `TRUSTED_PROXY_CIDRS: ...`. (config may import clientip; clientip imports nothing internal.) | 8 |
| `services/gateway/internal/proxy/gateway.go` | `Gateway.ips *clientip.Resolver` built in `New` from `cfg.TrustedProxies`. In the route handler, resolve once: `ip := g.ips.Resolve(r.RemoteAddr, r.Header.Values("X-Forwarded-For"))`, pass to `rateKey(rt, r, p, ip)`, `Resolve` returns `(ip string, fromXFF bool)`; when `fromXFF`, store ip in the request context (`withForwardedClient`). Delete `clientIP`. In `newReverseProxy.Rewrite`, after `pr.SetXForwarded()`: if the context holds an IP, `pr.Out.Header.Set("X-Forwarded-For", ip)` (trust off = no override = byte-identical). Update the two comments that say XFF is never trusted. | 1, 2, 9, 10 |
| `services/gateway/cmd/server/main.go` | Add `"trusted_proxies", cfg.TrustedProxies` to the startup log. | 3.1 |
| `deploy/compose/docker-compose.yml` | Comment only under `gateway.environment`: `# TRUSTED_PROXY_CIDRS: de trong - gateway la lop ngoai cung`. No value. | 2 |

`Resolve` reads only XFF, so it sits at step 3 next to `rateKey`; the identity strip/set order is untouched.

## 2. Tests (written first, red check before code)

| File | Kind | Covers |
|---|---|---|
| `internal/clientip/clientip_test.go` | Table test `TestResolve` (name, trusted, remoteAddr, xff lines, want). Rows: every example in AC-1..AC-7 (default/no trust, untrusted peer + XFF, forged leftmost, chain, untrusted middle hop, two header lines, spaces/tabs/empty elements, IPv4 port, IPv6 bracket+port, bracket only, upper-case IPv6, mapped IPv6 entry, mapped peer vs IPv4 CIDR, malformed rightmost x4, malformed left of client, no XFF, all trusted, 16 vs 17 trusted hops, malformed RemoteAddr). | 1-7 |
| same | Table test `TestParseTrusted`: valid list (CIDR v4/v6, bare IP to /32 and /128, spaces, empty elements, empty string = nil) and each AC-8 invalid element with `strings.Contains(err, element)`. | 8 |
| same | `FuzzResolve(f, xff string, peer string)` with seeds from the table. Invariants: no panic; peer untrusted ⇒ result == normalised peer (or raw `RemoteAddr` when unparsable); result is the raw `RemoteAddr`, or parses with `netip.ParseAddr` and `result == addr.Unmap().String()`, and that address occurs in the input (peer or one XFF element after normalisation). Trusted set fixed to `10.0.0.0/8, 2001:db8::/32`. Gate runs `go test -fuzz=FuzzResolve -fuzztime=30s`. | 11 |
| same | `BenchmarkResolve` on a 1 MiB XFF of trusted elements: bounded by the 16-hop cap (sanity, not a gate). | 3.3 |
| `internal/config/config_test.go` | `t.Setenv("TRUSTED_PROXY_CIDRS", ...)`: valid → `cfg.TrustedProxies` as expected; invalid → `Load` error contains `TRUSTED_PROXY_CIDRS` and the element; unset → nil. | 8 |
| `internal/proxy/trusted_proxy_test.go` (new) | Uses `newFixture(t, func(c){ c.TrustedProxies = ... })` and a test wrapper handler that sets `r.RemoteAddr` to the chosen peer before the gateway handler (helper `newFixtureAs(t, peer, mutate...)`, test file only). Fixed clock via `WithClock` so bursts are deterministic. Cases: AC-2 (11 login POSTs with different XFF from untrusted peer → 11th 429), AC-3 (A exhausts auth burst, B through same peer not 429, A gets 429 + `Retry-After`), AC-4 (varying forged prefix shares a bucket), AC-1 (no trust: same as develop, upstream XFF = peer), AC-9 (upstream sees exactly one XFF element = resolved IP; `X-Identity-*` stripped), AC-10 (logged-in on auth route keyed by IP, default route by id, queue-poll suffix). | 1-4, 9, 10 |
| `internal/proxy/gateway_test.go` | **Unchanged** (frozen). Its AC-1 spoof test must stay green. | 9 |

## 3. Gates

`cd services/gateway && go vet ./... && go test -race ./... && go test ./internal/clientip -run=^$ -fuzz=FuzzResolve
-fuzztime=30s`, plus `gofmt -l` empty and `go mod tidy` diff empty (proves no new dependency). Repo-wide Go gate as in
earlier tasks (`go test ./...` in each touched module via go.work).

## 4. Mutation test (threshold 6/6)

(1) Walk left to right (take first element). (2) Skip the peer-trusted check (always read XFF). (3) On a malformed
element `continue` instead of returning the peer. (4) Drop `Unmap()`. (5) Accept `/0` in `ParseTrusted`. (6) Remove
the `X-Forwarded-For` override in `Rewrite` (upstream keeps the peer). Each must fail at least one test.

## 5. Risks

| Risk | Mitigation |
|---|---|
| Trusting a too-wide CIDR lets internal pods spoof any IP | `/0` rejected; spec Q-1; k8s follow-up documents the scope |
| Header bomb (huge XFF) costs CPU | Right-to-left scan stops at first untrusted or after 16 trusted hops; benchmark |
| Upstream behaviour drift (identity device IP, payment `vnp_IpAddr`) | With trust off the XFF value is byte-identical to develop (AC-1 test asserts it); with trust on it becomes the real client, which is what those readers expect |
| Mis-set env takes the gateway down | Intended fail fast; message names the element; compose leaves it empty |
| IPv4-mapped peer silently not matching | Unmap on both sides; AC-6 test row |

## 6. Rollback

One PR, merge commit; `git revert -m 1 <merge>`. No data or schema. Leaving `TRUSTED_PROXY_CIDRS` empty is also a
runtime rollback (exact develop behaviour, AC-1). Check after revert: `go test -race ./...` in `services/gateway`
passes and the existing gateway AC-1 spoof test is green.
