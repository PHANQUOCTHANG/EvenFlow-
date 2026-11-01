# Test change requests applied

## TCR-E8: fixtures updated to contract rev 2, approved via E8 on 2026-11-01

Scope: `deploy/k8s/policy/test_policy.sh` and `deploy/k8s/policy/testdata/**` only, as listed in plan.md Round 3 section C.

- Generator: web exemption is `AC-12.readOnlyRootFilesystem`; no AC-9 exemption anywhere; ticketing-worker keeps `AC-11`;
  Postgres exemptions reduced to AC-6/10/11/20 and it now runs as uid 70 with drop ALL; no `optional` on any
  `secretKeyRef`; per-overlay secret sets (local: 8 payment keys; staging: 5 vnpay/momo keys, identity SMTP_USER/SMTP_PASS,
  notification SMTP_USER/SMTP_PASSWORD); `eventflow-app-config` gains the new keys, staging providers `vnpay,momo`;
  hash-suffixed names (`-5g8h2k9m4t`) for local Secrets and both overlays' ConfigMaps; `eventflow-mailpit` dev-infra
  Deployment; `good-local.yaml` ends with an empty document.
- Deleted: `bad/AC-18--stripe-key-not-optional.yaml`.
- Added 22 bad fixtures (AC-9 1, AC-12 4, AC-18 4, AC-22 1, AC-32 9, EXEMPT 3) and `dockerfiles/{good,bad/AC-13--*}`
  (6 good tails, 3 bad cases).
- `test_policy.sh`: `--dockerfiles` runs (good dir once, each bad dir expects exit 1 with only `FAIL AC-13`), exit-2 case for a
  missing Dockerfile dir, AC-13 coverage check.
- All other bad fixtures keep their rule id and intent; their bytes changed because they derive from the good fixtures.

## TCR-E8b: AC-32 requires non-empty GOOGLE_CLIENT_ID in staging (human-confirmed 2026-11-01, spec.md aec252a)

- good-staging already carries a non-empty GOOGLE_CLIENT_ID (verified).
- Added bad/AC-32--google-client-id-empty.yaml (staging, --processes web,gateway,identity, empty string in eventflow-app-config). Now 152 bad fixtures, selftest total 165.

## TCR-E8c: AC-1 count excludes dev-infra Deployments (covered by E8 / contract rev 2 section 2)

- test_policy.sh E-AC1-DEPLOYMENTS now counts rendered Deployments without label eventflow.io/tier=dev-infra via the pinned yq and still expects 11 (SKIP when yq is unavailable).
