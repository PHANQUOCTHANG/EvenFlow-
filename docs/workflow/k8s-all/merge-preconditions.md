# k8s-all: merge preconditions (human)

These items cannot be closed by an agent. The branch must not be merged into `develop` until each one has a dated
sign-off line filled in below (who, date, decision). Source: `findings-review.json` R-10 / R-14, tracked by
`findings-round2.json` R2-3.

| Id | Source | What the human must do | Sign-off |
|---|---|---|---|
| MP-1 | R-14 | AC-30 (overlays/local applied on Docker Desktop / kind: migrate Job, readiness, PDB, NetworkPolicy) and AC-31 (staging server dry-run, image builds / uids) are **NOT VERIFIED** (runbook section 9; full gate records them NOT RUN 77). Either run runbook section 3 on a local cluster and attach the evidence, or record an explicit acceptance of merging with AC-30/AC-31 NOT VERIFIED. | _pending_ |
| MP-2 | R-10 | Confirm (date, who) the approval of TCR-WP5 (`test-change-request-wp5.md`, fixture `bad/AC-12--dev-infra-run-as-uid-0.yaml`, additions only), or ask for the fixture to be reverted. `tcr-applied.md` records it as pending confirmation. | _pending_ |

Closed without action: R-13 (not reproducible, see `deferred.md`).

## Human decisions (TranDinhVo, Claude Code session, 2026-11-21)

- MP-1: accepted. AC-30 and AC-31 stay NOT VERIFIED (no Docker daemon or kube context on this host). The offline checks (kustomize render, kubeconform, policy, CI workflow tests) are the merge evidence; the live run follows runbook sections 1–4 later.
- MP-2: TCR-WP5 approved (one added fixture `bad/AC-12--dev-infra-run-as-uid-0.yaml`, no assertion changed).
- TCR-3 approved: add five bad fixtures for the surviving mutants M02, M20, M22, M23, M28 and one stub-kubectl test for the gates.sh C4.4 context guard. Additions only.
