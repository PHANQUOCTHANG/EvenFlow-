# Spec: gateway client IP behind trusted proxies (gateway-trusted-proxy)

Tier: **High-risk** (`tier.json`). Branch `feature/gateway-trusted-proxy` from develop @ c5fb598. Gateway only, no
migration, no new module dependency. Source: k8s-all review finding R-1.

## 1. Problem

`rateKey` (services/gateway/internal/proxy/gateway.go:192) keys anonymous traffic and the whole `auth` class by
`clientIP(r)`, which is the host of `r.RemoteAddr` and deliberately ignores `X-Forwarded-For` (XFF). Behind the K8s
ingress-nginx (`feature/k8s-all`: `deploy/k8s/base/gateway/ingress.yaml`, NetworkPolicy `allow-gateway` admits the
`ingress-nginx` namespace), every external user arrives from an ingress pod IP, so all users share one bucket per
gateway replica: login/register `1 rps, burst 10`, anonymous `10 rps, burst 30`. One attacker locks everyone out of
login, and normal traffic hits 429.

## 2. Approved direction (human)

The gateway trusts XFF **only** when the direct peer (`RemoteAddr`) is inside a configured trusted-proxy set from env
`TRUSTED_PROXY_CIDRS`. Empty or unset = never trust = today's behaviour. Compose keeps it empty (the gateway is the
outermost layer there).

## 3. Design

### 3.1 Config `TRUSTED_PROXY_CIDRS`

- Comma-separated; each element trimmed; empty elements ignored. Each element is a CIDR (`10.244.0.0/16`,
  `fd00::/8`) or a bare IP (treated as `/32` or `/128`).
- Parsed in `config.Load`. Any invalid element makes `Load` return an error naming the element, so the process exits
  non-zero at startup (fail fast, same as a bad `RATE_*`). Rejected: unparsable text, prefix with host bits set
  (`10.0.0.1/8`, hint: use `10.0.0.0/8`), any `/0` prefix (`0.0.0.0/0`, `::/0` = trust the world = spoofable), zones
  (`fe80::1%eth0`, also as a bare IP), IPv4-mapped IPv6 prefixes and bare IPs (`::ffff:10.0.0.0/104`,
  `::ffff:10.0.0.1`, hint: write the IPv4 form; they could never match because addresses are unmapped). Checks:
  `p != p.Masked()` = host bits set, `p.Bits() == 0` = `/0`, `addr.Zone() != ""`, `addr.Is4In6()`.
- Startup log line `gateway dang lang nghe` gains `trusted_proxies` (list, or empty).

### 3.2 Peer key and address normalisation

`peerKey` = host from `net.SplitHostPort(RemoteAddr)`, or the raw `RemoteAddr` if that fails: exactly today's
`clientIP`, unchanged. **Every fallback to the peer returns `peerKey`** (byte-identical to develop). The
netip-normalised peer (below) is used **only** for the trust `Contains` check; a peer that netip cannot parse
(zoned `[fe80::1%eth0]:x`, garbage) is never trusted, so its key stays `peerKey` (e.g. `fe80::1%eth0`, no port).

XFF elements are normalised as follows. Parse `net/netip`; strip an optional port (`1.2.3.4:5678`, `[2001:db8::1]:443`) and brackets
(`[2001:db8::1]`); a bare IPv6 (`2001:db8::1`) is an address, not host:port. IPv4-mapped IPv6 (`::ffff:1.2.3.4`) is
unmapped to `1.2.3.4` before matching and in the key. The key from an XFF element uses the canonical `netip.Addr.String()` (IPv6
lower-case, compressed). Zones, empty strings, `unknown`, `_obfuscated` and anything else unparsable = **malformed**.

### 3.3 Resolution algorithm `clientIP(r)`

1. Compute `peerKey` (3.2). If the trusted set is empty, return `peerKey`; XFF is ignored.
2. Normalise the peer with netip. If that fails, or the peer is not in the trusted set: return `peerKey`.
3. Otherwise build the hop list from **all** `X-Forwarded-For` header lines in received order, each split on `,`
   (= one list, as RFC 7230 allows). Elements are trimmed of spaces/tabs; empty elements are skipped.
4. Walk the list **right to left**. For each element: malformed → return `peerKey` (stop); not trusted → return it
   (this is the client); trusted → continue.
5. If the walk ends (list empty, or every element trusted) → return the leftmost element when one exists, otherwise
   `peerKey` (Q-3 default: leftmost).
6. At most 16 trusted elements are walked; a 17th trusted element in a row → return `peerKey`.

Elements left of the returned one are never read, so a forged or malformed leftmost value cannot change the result.
The walk parses from the right without building the whole list: cost is linear in header size (empty elements are
skipped, not counted, so `MaxHeaderBytes` is the bound), no allocation per element.

### 3.4 Other headers

- `X-Real-IP` and `Forwarded` (RFC 7239) are **not** read: ingress-nginx sends XFF, one source of truth keeps the
  spoofing surface small.
- `r.RemoteAddr` is not rewritten.
- Identity headers: unchanged (`StripIdentityHeaders` runs first, `SetIdentityHeaders` after auth).
- Upstream: `newReverseProxy` keeps `Rewrite` (which drops inbound `Forwarded`/`X-Forwarded-*`) and
  `SetXForwarded()`; when the resolved client came from XFF (step 4/5 of 3.3), it then **sets `X-Forwarded-For` to
  exactly that IP** (one element). Otherwise `SetXForwarded`'s value (the peer host = `peerKey`) is kept,
  byte-identical to today. Identity (`deviceFrom`) and payment (`vnp_IpAddr`) read the first XFF
  element, so they get the real client behind the ingress with no change on their side. Untrusted/forged entries
  never reach upstreams. `X-Forwarded-Host`/`-Proto` unchanged.

### 3.5 Rate-limit key and observability

`rateKey` logic is unchanged apart from its IP source: `ip:<resolved>` for anonymous and for the `auth` class even
when logged in; `id:<sub>` otherwise; `|<eventID>` suffix for queue-poll. No per-request log line and no new metric
(raw IPs must not become label values; a counter would add a direct Prometheus dependency to the gateway module).

## 4. Acceptance criteria

- **AC-1 (default = today)** Given `TRUSTED_PROXY_CIDRS` empty/unset, When any request arrives with XFF,
  `X-Real-IP` and `Forwarded` all set (forged), Then the rate-limit key is `ip:<peerKey>` and the upstream XFF is
  that host, exactly as on develop, for any `RemoteAddr` (incl. zoned, mapped, unparsable).
- **AC-2 (spoof: untrusted peer)** Given trust `10.0.0.0/8` and peer `203.0.113.9`, When it sends
  `X-Forwarded-For: 1.2.3.4`, Then the key is `ip:203.0.113.9` and upstream XFF is `203.0.113.9`; 11 login requests
  with 11 different XFF values from that peer → the 11th gets 429.
- **AC-3 (the R-1 fix)** Given trust `10.0.0.0/8` and peer `10.1.2.3`, When client A (`XFF: 198.51.100.1`) exhausts
  the auth burst, Then client B (`XFF: 198.51.100.2`) through the same peer still gets a non-429 response, and A's
  11th request gets 429 with `Retry-After`.
- **AC-4 (spoof: forged leftmost)** Given trust `10.0.0.0/8`, peer `10.1.2.3`, When XFF is
  `6.6.6.6, 198.51.100.1` (client forged the first value, the proxy appended the real one), Then the key is
  `ip:198.51.100.1`; varying the forged part never changes the bucket.
- **AC-5 (proxy chain)** Given trust `10.0.0.0/8, 192.0.2.0/24`, peer `10.1.2.3`, When XFF is
  `198.51.100.1, 192.0.2.10`, Then the key is `198.51.100.1`. When XFF is `198.51.100.1, 203.0.113.50, 192.0.2.10`
  (untrusted hop in the middle), Then the key is `203.0.113.50`.
- **AC-6 (formats)** Given a trusted peer, Then: two XFF header lines `198.51.100.1` + `10.0.0.5` resolve like one
  line `198.51.100.1, 10.0.0.5`; spaces/tabs and empty elements are ignored; `198.51.100.1:5555` → `198.51.100.1`;
  `[2001:db8::1]:443`, `[2001:db8::1]`, `2001:DB8::1` → `2001:db8::1`; `::ffff:198.51.100.1` → `198.51.100.1`;
  a peer `[::ffff:10.1.2.3]:4000` matches trust `10.0.0.0/8` (and its fallback key is `::ffff:10.1.2.3` = `peerKey`).
- **AC-7 (malformed / edge)** Given a trusted peer `10.1.2.3`, Then: rightmost element malformed (`unknown`,
  `1.2.3`, `fe80::1%eth0`, `<script>`) → key = peer; malformed element only left of the first untrusted one → ignored;
  no XFF or only empty elements → peer; every element trusted (`10.0.0.7`) → leftmost (`10.0.0.7`); 17 trusted
  elements → peer; every "peer" here means `peerKey`. With trust on, a zoned peer `[fe80::1%eth0]:51234` → key
  `fe80::1%eth0` (never trusted, no port); an unparsable `RemoteAddr` → raw `RemoteAddr`, XFF ignored.
- **AC-8 (config fail fast)** `10.244.0.0/16, fd00::/8`, `10.0.0.1`, ` 10.0.0.0/8 ,, ` load; each of `10.0.0.1/8`,
  `0.0.0.0/0`, `::/0`, `10.0.0.0/33`, `abc`, `fe80::1%eth0`, `::ffff:10.0.0.0/104`, `::ffff:10.0.0.1` makes `config.Load` fail with the
  element in the message, and `proxy.New` is never reached.
- **AC-9 (upstream and identity)** For AC-2..AC-7 inputs, the upstream receives exactly one XFF element equal to the
  resolved IP; no client-sent `X-Identity-*` reaches upstream (existing AC-1 test of gateway-auth-routing stays green
  unmodified); `RemoteAddr` seen by handlers is unchanged.
- **AC-10 (key classes)** A logged-in user on an `auth` route is still keyed by resolved IP; on a default route by
  `id:`; queue-poll keeps the `|<eventID>` suffix.
- **AC-11 (robustness)** A fuzz target over (XFF lines, peer) never panics; the result is `peerKey` or the canonical
  `Unmap().String()` form of an XFF element; peer untrusted (or trust empty) ⇒ result == `peerKey`.

## 5. Out of scope

Proxy-protocol, `Forwarded` parsing, per-route trust, changing identity/payment readers, web SSR forwarding the
browser IP (see question Q-2), and any edit to the `feature/k8s-all` branch.

## 6. Follow-up for k8s-all (do not edit that branch here)

- Gateway Deployment env: `TRUSTED_PROXY_CIDRS` = the cluster pod CIDR that holds the ingress-nginx pods (e.g. kind
  default `10.244.0.0/16`), per overlay via `eventflow-app-config`. Pod IPs are not stable, so a narrower value needs
  a CNI with per-namespace pools.
  The k8s-all runbook must state the risk: with the pod CIDR trusted, any compromised pod can forge XFF (Q-1).
- ingress-nginx: keep `use-forwarded-headers: "false"` (default) when clients connect straight to the controller,
  so nginx sends its own `$remote_addr` and drops client XFF. Only when a cloud L7 LB sits in front, set
  `use-forwarded-headers`/`compute-full-forwarded-for: "true"` with `proxy-real-ip-cidr` = LB range, and add the LB
  range to `TRUSTED_PROXY_CIDRS`. Verify against the controller version's `nginx.tmpl`.
- Controller Service `externalTrafficPolicy: Local` (or proxy-protocol), otherwise kube-proxy SNAT hides the client
  before nginx and the fix has nothing to read.
- A policy test that the gateway Deployment sets the env in non-dev overlays; never `0.0.0.0/0`.

## 7. Questions for the human (OPEN; answer at sign-off, before step 3)

| Q | Question | Recommended default (assumed by the plan) |
|---|---|---|
| Q-1 | K8s trust scope: whole pod CIDR or a narrower ingress-nginx range? | Pod CIDR; the k8s-all runbook records the risk that any compromised pod can then forge XFF |
| Q-2 | Web SSR pods share one bucket at the gateway. Accept or fix now? | Accept; follow-up task where web forwards the browser IP and its pods join the trusted set |
| Q-3 | Every XFF element trusted: leftmost or peer? | Leftmost (spec 3.3 step 5) |
