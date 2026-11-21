# Deferred findings: gateway-trusted-proxy (fix round 1, 2026-11-21)

| Finding | Severity | Why deferred | Where it goes |
|---|---|---|---|
| R-3 / C-2: IPv6 clients keyed per /128, so a host rotating inside its /64 escapes the auth limit (also IPv4-embedded IPv6 spellings are separate buckets) | minor | Pre-existing on develop for direct IPv6 peers; spec 3.5 keeps key classes unchanged (out of scope). Only reachable on an IPv6-reachable ingress. | Follow-up task: key IPv6 rate buckets by `netip.PrefixFrom(a, 64).Masked()` while still forwarding the full address upstream; until then the k8s-all runbook records it as an accepted limit if the ingress stays IPv4-only. |
| C-4: pod-CIDR trust (Q-1, accepted) lets any pod that can reach the gateway lock out a chosen victim IP, escape the limit, and choose the IP seen in identity `last_ip` / payment `vnp_IpAddr` | minor | Works as specified and accepted by the human (Q-1); no code change in this branch. Only the runbook wording understates it. | `feature/k8s-all`: runbook spells out the three consequences; require the NetworkPolicy that limits gateway ingress to ingress-nginx (and web) as a precondition for setting `TRUSTED_PROXY_CIDRS`; policy test for it. |

R-1 is covered by C-1 (Amendment 1 in `spec.md`).

## From re-review round 2 (optional, not blocking)

- R2-1: strip more client-IP headers (X-Client-IP, X-Cluster-Client-IP, Fastly-Client-IP, Client-IP); upstreams must trust only X-Forwarded-For.
- R2-2: move `clientIPHeaders` above the `newReverseProxy` doc comment.
- R2-3: port 0 in an XFF element is accepted (port is discarded; harmless).
- R2-4: the /8 and /7 floors still allow wide prefixes; note in the k8s-all runbook.
