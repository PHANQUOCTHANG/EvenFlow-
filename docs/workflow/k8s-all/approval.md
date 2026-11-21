# Human sign-off: k8s-all (EV-190, High-risk)

- Approver: TranDinhVo, in the Claude Code session on 2026-11-21.
- Spec: approved earlier in this task (`spec.md`, `policy-contract.md`, errata E2-E8).
- MP-1: accepted. AC-30 (local cluster apply) and AC-31 (staging server dry run, image builds / uids) stay
  **NOT VERIFIED**; the full gate records them NOT RUN (77). The offline checks (kustomize render, kubeconform,
  policy checker and self-test, CI workflow tests) are the merge evidence; the live run follows runbook sections 1-4.
- MP-2: TCR-WP5 approved (one added fixture, no assertion changed).
- TCR-2 and TCR-3 approved (additions only), including the TCR-3 addition for the waitingroom audit DB recorded in
  `tcr-applied.md`.
- Decision Q-1: gateway `TRUSTED_PROXY_CIDRS` trusts the pod CIDR, `10.0.0.0/8` in both overlays; risk and per-cluster
  setting in `runbook.md` section 4.5. This closes the deferred R-1 on the manifest side (gateway code is PR #100).
- Decision input: `evidence.md` verdict PASS (34/34 mutants killed by the repo's own suites); full gate PASS at `568e188` on the
  integration commit with `origin/develop` (PR #99, PR #100) merged, except AC-30/AC-31 NOT RUN (`gate-report.json`).
- Accepted debt: the items in `deferred.md` (round 1 and round 2), unchanged by this sign-off.
- Merge: merge commit into `develop` once PR CI is green; `main` untouched.
