# Human sign-off: gateway-trusted-proxy (High-risk)

- Approver: TranDinhVo, in the Claude Code session on 2026-11-21.
- Approved code: gate PASS at `4abf6a1`; later commits are docs only plus a merge of `origin/develop`.
- Decision input: `evidence.md` PASS (independent mutants 10/10, gate mutants 8/8, 231 tests, fuzz clean, empty-trust behaviour identical to develop in 210 cases, rollback PASS); review, challenge and re-review with no blocker or major.
- Earlier human decisions: spec approved with Q-1 (pod CIDR), Q-2 (SSR shares one bucket for now), Q-3 (leftmost when all trusted); Amendment 1 (reject IPv4 wider than /8, IPv6 wider than /7).
- Not verified: gosec (incompatible with go1.27), real ingress-nginx behaviour (needs a cluster).
- Deferred: R-3/C-2 IPv6 /64 keying; C-4 k8s-all runbook pod-CIDR risk and NetworkPolicy precondition; R2-1..R2-4 (see `deferred.md`).
- Merge: merge commit into `develop` once PR CI is green; `main` untouched.
