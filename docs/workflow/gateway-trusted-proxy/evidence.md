# Verification evidence: gateway-trusted-proxy (v3 step 6, High risk)

- Verifier: independent verifier agent, read-only on code, 2026-11-21.
- Branch `feature/gateway-trusted-proxy`, HEAD `a5eaa37`. Go `go1.27.0 windows/amd64`.
- **Verdict: PASS** (with the NOT VERIFIED items and open human items below).

## 1. Gate commit vs HEAD

| Check | Result |
|---|---|
| `gate-report.json` `commit_sha` | `4abf6a1` |
| Last commit touching code (`services/`) | `c45c4f9`; `git diff --name-only c45c4f9 4abf6a1` lists only `docs/**` |
| `git diff 4abf6a1 a5eaa37` | only `docs/workflow/gateway-trusted-proxy/**` (gate logs, mutation logs, gate-report) |
| Code under gate == code at HEAD | yes |

## 2. Test run

- `cd services/gateway && go test ./... -race -count=1 -v`: exit 0, 231 `--- PASS`, 0 FAIL (packages clientip, config, proxy, ratelimit). Log: `verify-logs/tests.log`.
- `go test ./internal/clientip -run='^$' -fuzz=FuzzResolve -fuzztime=30s`: exit 0, no crasher. Log: `verify-logs/fuzz.log`.

## 3. Acceptance criteria

PASS counts are `--- PASS` lines (subtests included) in `verify-logs/tests.log`.

| AC | Proving tests (traceability.yaml) | Result |
|---|---|---|
| AC-1 default = today | `TestResolve/AC1_*` (7), `TestPeerKeyMatchesDevelop`, `TestTrustedProxy_AC1_TrustOffIgnoresForwardingHeaders` (6), `TestGateway_XoaDanhTinhGiaMaoTrenMoiRoute` (existing, unchanged) | PASS; also section 5 (210 cases, 0 diffs vs develop) |
| AC-2 untrusted peer spoof | `TestResolve/AC2_*` (2), `TestTrustedProxy_AC2_UntrustedPeerCannotSpoof` | PASS |
| AC-3 own bucket behind proxy | `TestResolve/AC3_*` (3), `TestTrustedProxy_AC3_ClientsBehindTrustedProxyHaveOwnBucket` | PASS |
| AC-4 forged leftmost | `TestResolve/AC4_*` (3), `TestResolve_ForgedPrefixNeverChangesResult`, `TestTrustedProxy_AC4_ForgedLeftmostSharesBucket` | PASS |
| AC-5 proxy chain | `TestResolve/AC5_*` (3), `TestTrustedProxy_AC9_UpstreamGetsResolvedIPOnly/AC5_*` (2) | PASS |
| AC-6 formats | `TestResolve/AC6_*` (20), `.../AC9_.../AC6_*` (7), `TestResolve_Round1_StrictElementForms`, `TestResolve_Round1_LeadingTab` | PASS |
| AC-7 malformed / edge | `TestResolve/AC7_*` (28), `.../AC9_.../AC7_*` (8) | PASS |
| AC-8 config fail fast (amended) | `TestParseTrusted`, `TestLoad_TrustedProxyCIDRsHopLe`, `TestLoad_TrustedProxyCIDRsHongThiLoi`, `TestParseTrusted_Round1_WidePrefixRejected`, `..._FloorBoundaryAccepted`, `..._TrimsNewlines`, `TestLoad_Round1_WidePrefixFailsFast`, `TestLoad_Round1_TrailingNewlineLoads`; inspection: `cmd/server/main.go:30-33` returns on `config.Load` error before `proxy.New` | PASS |
| AC-9 upstream and identity | `TestTrustedProxy_AC9_UpstreamGetsResolvedIPOnly`, AC2/AC3 proxy tests, `TestGateway_XoaDanhTinhGiaMaoTrenMoiRoute`, `TestTrustedProxy_Round1_ClientIPHeadersStripped` (4); inspection: `r.RemoteAddr` only read (`gateway.go:135`), never assigned | PASS |
| AC-10 key classes | `TestTrustedProxy_AC10_KeyClasses` (5), `TestGateway_GioiHanTocDo` (existing) | PASS |
| AC-11 robustness | `FuzzResolve` 30 s, no crasher | PASS |

## 4. Independent mutation (new set V1..V10, not M1..M8)

Script `verify-logs/mutation-verify/vmutate.sh` (perl substitution, confirm diff, `go test ./... -count=1`, revert with `git -c core.autocrlf=false checkout -- services/gateway`). A run that fails to build is reported `INVALID_BUILD`, not killed; the first attempt of V7/V9 hit unused-variable build errors and was rewritten so every mutant compiles. Tree clean after the run. Logs: `verify-logs/mutation-verify/V*.log`, `summary.txt`.

| ID | Mutation | Status | Killed by (top-level tests) |
|---|---|---|---|
| V1 | drop `X-Real-IP` from `clientIPHeaders` | KILLED | `TestTrustedProxy_Round1_ClientIPHeadersStripped` |
| V2 | `fromXFF` treated as always false (no upstream XFF override) | KILLED | AC3, AC4, AC9 proxy tests, `..._Round1_ClientIPHeadersStripped` |
| V3 | hop cap 16 -> 17 | KILLED | `TestResolve` (AC7 17-hop rows), `TestTrustedProxy_AC9_...` |
| V4 | port bound 65535 -> 65536 | KILLED | `TestResolve_Round1_StrictElementForms` |
| V5 | IPv6 floor /7 -> /6 | KILLED | `TestParseTrusted_Round1_WidePrefixRejected` |
| V6 | per-element `TrimSpace` removed in `ParseTrusted` | KILLED | `TestParseTrusted`, `..._Round1_TrimsNewlines`, `TestLoad_TrustedProxyCIDRsHopLe`, `TestLoad_Round1_TrailingNewlineLoads` |
| V7 | XFF override applied even when not `fromXFF` | KILLED | `TestTrustedProxy_AC1_TrustOffIgnoresForwardingHeaders`, `TestTrustedProxy_AC9_...` |
| V8 | `trimSpaceTab` is identity | KILLED | `TestResolve`, `TestResolve_ForgedPrefixNeverChangesResult`, `..._Round1_LeadingTab`, AC4/AC9 proxy tests |
| V9 | accept bracketed IPv4 `[1.2.3.4]` in XFF | KILLED | `TestResolve_Round1_StrictElementForms` |
| V10 | accept IPv4-mapped bare IP in `TRUSTED_PROXY_CIDRS` | KILLED | `TestParseTrusted`, `TestLoad_TrustedProxyCIDRsHongThiLoi` |

**Kill count: 10/10** (gate set M1..M8 was 8/8 per `gate-report.json`; not re-run).

## 5. Rollback

Plan: revert the merge commit on `develop`; no migration, no data, config only (`TRUSTED_PROXY_CIDRS`, new and optional).

- **Trust unset == develop.** With `TRUSTED_PROXY_CIDRS` unset, the AC-1 tests pass (19 PASS, `verify-logs/rollback/ac1-tests.log`). A scratch program (`verify-logs/rollback/peerkey-compare.go.txt`) compares the branch `clientip.Resolve` with resolvers `New(nil)`, `New(ParseTrusted(""))`, and an empty slice against develop's `clientIP` (copied word for word from `c5fb598` `gateway.go:205-211`). It covers 14 peers (IPv4, IPv6, mapped, zoned, port-less, garbage, empty) x 5 XFF sets: **210 cases, 0 mismatches, `fromXFF` always false** (`verify-logs/rollback/peerkey-compare.log`). On the upstream side, `fromXFF=false` skips the override, so `Rewrite`/`SetXForwarded` behaves as on develop. The only other difference is that client-sent `X-Real-IP`/`True-Client-IP`/`X-Original-Forwarded-For`/`CF-Connecting-IP` are stripped (Amendment 1, C-3), which is a hardening change.
- **Revert target builds.** A temporary detached worktree at `c5fb598` ran `go build ./...` (ok) and `go test ./... -race -count=1` (exit 0). Log: `verify-logs/rollback/develop-c5fb598.log`. The worktree was removed and pruned afterwards.

## 6. NOT VERIFIED

- **gosec**: NOT RUN (exit 77 in the gate). The tool does not work with go1.27. There is no substitute static security scan; govulncheck passed in the gate.
- **Real ingress-nginx behaviour** (actual XFF shape, `use-forwarded-headers`, pod CIDR trust in a cluster): needs a cluster, so it was not exercised. Only unit and integration tests with `httptest` were run.

## 7. Open human items

- Amendment 1 (prefix floor IPv4 /8, IPv6 /7; CR/LF trimming; strict XFF forms; client IP header stripping) is recorded in `spec.md` with the human decision for C-1/R-1. Already decided, no action needed.
- Deferred **R-3 / C-2**: IPv6 rate buckets are per /128 (a host rotating within its /64 escapes the auth limit). Needs a follow-up task (key by /64), or the k8s-all runbook must accept it as a limit while ingress stays IPv4-only.
- Deferred **C-4**: pod-CIDR trust consequences (victim lockout, limit escape, chosen `last_ip`/`vnp_IpAddr`). The `feature/k8s-all` runbook must state these and require a NetworkPolicy limiting gateway ingress to ingress-nginx (and web) before `TRUSTED_PROXY_CIDRS` is set.
- Human sign-off on this evidence before merge (v3 step 7).
