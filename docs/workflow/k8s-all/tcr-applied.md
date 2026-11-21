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

## TCR-WP5: optional fixture for the AC-12 rootUid sub-check (test-change-request-wp5.md, applied by WP6 on 2026-11-01)

- Added test, no existing assertion changed. `gen_fixtures.py` gains one mutant; regenerating changed no other file and
  `gen_fixtures.py --check` is clean.
- New `bad/AC-12--dev-infra-run-as-uid-0.yaml` (local, `--processes web,gateway`): postgres pod
  `securityContext.runAsUser: 0`. Dev-infra may set `runAsUser`, so only `rootUid` catches it; the checker prints exactly
  `FAIL AC-12 StatefulSet/eventflow-postgres: rootUid: pod runAsUser 0`. Now 153 bad fixtures, selftest total 166.
- Second row of the request (invalid exemption taking effect) is not applied: it needs two ids in one fixture, which
  contract §7 forbids. Recorded as an accepted limitation in `deploy/k8s/policy/rules.md`.

## TCR-WP5 approval status (review finding R-10, 2026-11-20)

- The human approval of TCR-WP5 is **pending confirmation**: no dated approval is recorded for
  `test-change-request-wp5.md` (its status line still reads "open, optional"). The fixture only adds a test and is
  kept; the coordinator is asking TranDinhVo to confirm. This entry does not claim it was approved.

## TCR-2: fixtures for the review / challenge bypasses (test-change-request-2.md), approved by TranDinhVo 2026-11-20 via coordinator

- Added tests only, no existing assertion or fixture changed: `gen_fixtures.py` gains 25 mutants and 3 Dockerfile
  cases; regenerating changed no other file and `gen_fixtures.py --check` is clean. `test_policy.sh` unchanged.
- Covers R-2, X-1..X-5, X-7, X-8, X-10 (table in test-change-request-2.md); checks are contract rev 3.
- Now 178 bad fixtures and 6 bad Dockerfile cases.

## TCR-2 additions (fix round 2, findings-round2.json R2-1 / R2-2), 2026-11-21, under the TCR-2 approval

- Added tests only: 2 mutants (`AC-17--gateway-args-split-password`, `AC-17--gateway-shell-pgpassword`) and 1 Dockerfile
  case (`AC-13--user-root-group`). No existing fixture or assertion changed; `gen_fixtures.py --check` clean;
  `test_policy.sh` unchanged. Now 180 bad fixtures and 7 bad Dockerfile cases.
- R-13 closed as no-op (not reproducible). R-10 (TCR-WP5 confirmation) and R-14 (AC-30/31 sign-off) are tracked as
  human merge preconditions in `merge-preconditions.md`.

- TCR-WP5: approved by TranDinhVo on 2026-11-21 in the Claude Code session (was pending confirmation).
- TCR-3: approved by TranDinhVo on 2026-11-21 in the Claude Code session (additions only; see test-change-request-3.md once applied).

## TCR-3: fixtures and a C4.4 test for the verifier's mutation survivors (test-change-request-3.md), approved by TranDinhVo 2026-11-21

- Added tests only: 4 mutants in `gen_fixtures.py` (`AC-32--unreferenced-forbidden-configmap-key`,
  `AC-32--vnpay-return-url-empty`, `EXEMPT--web-allowed-pair-empty-reason`, `EXEMPT--gateway-malformed-key`) and 1
  Dockerfile case (`AC-13--user-uid-0-no-gid`). No existing fixture or assertion changed; `gen_fixtures.py --check`
  clean; `test_policy.sh` unchanged. Now 184 bad fixtures and 8 bad Dockerfile cases (selftest 202).
- New `.github/scripts/tests/test-gates-context-guard.sh` (11 cases, stub kubectl), run by CI `lint-ci` and by
  `full-gate.sh` step `gates-context-guard`.

## TCR-3 addition: waitingroom audit DB (develop integration, PR #99), 2026-11-21, under the TCR-2/TCR-3 approval

- develop PR #99 gives waitingroom `DATABASE_URL` (via PgBouncer) for the ops audit log. The manifest now maps it to
  `eventflow-db/DATABASE_URL_POOLED`, which `check_policy.py` AC-18 reported as an unexpected secret env.
- `check_policy.py`: minimal change, no rule removed. New table `EXTRA_SECRET_ENV_BOTH` (waitingroom may hold
  `DATABASE_URL <- eventflow-db/DATABASE_URL_POOLED`; allowed, not required, since the app only fails the ops override
  closed without it) merged into AC-18's existing `extra_ok`. Any other source is still `unexpected secret env`.
  Contract §8 row for waitingroom updated to match.
- Added tests only: 2 mutants in `gen_fixtures.py` (`AC-18--waitingroom-uses-direct-db` local,
  `AC-18--staging-waitingroom-uses-direct-db` staging). No existing fixture or assertion changed (regenerating wrote only
  the two new files; `gen_fixtures.py --check` clean); `test_policy.sh` unchanged. Now 186 bad fixtures.

## antibot-phase1 TCR N-5: `ANTIBOT_HASH_KEY` for waitingroom, approved by TranDinhVo 2026-11-22

Source: `docs/workflow/antibot-phase1/contracts.md` §7 (rev 3) and `plan-review.md` round 2 N-5; human approval in
`docs/workflow/antibot-phase1/spec.md` ("Amendment 1, revision after plan review round 2"). Additions only.

- Contract §8: the waitingroom row gains `ANTIBOT_HASH_KEY ← eventflow-auth/ANTIBOT_HASH_KEY` (both overlays), so the
  `eventflow-auth` key set gains it too.
- `check_policy.py`: `SECRET_ENV_BOTH["waitingroom"]` gains `ANTIBOT_HASH_KEY`; `EXTERNAL_SECRET_KEYS["eventflow-auth"]`
  gains `ANTIBOT_HASH_KEY`. No rule removed, no check changed.
- `rules.md`: a short "antibot-phase1 addition" note (rules.md has no per-process secret table; it defers to §8).
- `testdata/gen_fixtures.py`: the waitingroom env and the local `eventflow-auth` Secret gain the key (value contains
  `dev`). Regenerated: `good-local.yaml`, `good-staging.yaml` and every `bad/*.yaml` (each changes only in the rendered
  waitingroom env / auth Secret line); no fixture added or removed; `gen_fixtures.py --check` clean; `test_policy.sh`
  unchanged. No `ANTIBOT_*` key is on the AC-32 forbidden list.
- Manifests (contracts §7): waitingroom env `ANTIBOT_HASH_KEY` (secretKeyRef, not optional), `ANTIBOT_MODE` and
  `ANTIBOT_SLOW_LANE_PENALTY` (configMapKeyRef `eventflow-app-config`); waitingroom-controller
  `ANTIBOT_SLOW_LANE_PENALTY`; both overlays' `app-config.env` set `ANTIBOT_MODE=shadow`,
  `ANTIBOT_SLOW_LANE_PENALTY=100000`; local `secrets.dev.env` + a replacement into `eventflow-auth`; staging
  kustomization header lists the key. Staging rollout: create the key in Secret `eventflow-auth` before applying (AC-20).

### antibot-phase1 fix round 1, R-6 (2026-11-22, under TCR N-5 as directed by the coordinator)

- `deploy/k8s/base/waitingroom/deployment.yaml`: the `ANTIBOT_HASH_KEY` secretKeyRef is `optional: true`
  (findings-review.json R-6: a missing key must not stop the waiting room; `ANTIBOT_MODE=off` stays a rollback).
- `check_policy.py`: AC-18 skips its optional-ref failure only for `OPTIONAL_SECRET_OK = {("waitingroom",
  "ANTIBOT_HASH_KEY")}`. This narrows an existing check for one ref (not an addition). **Accepted by TranDinhVo
  on 2026-11-22** (antibot-phase1 spec.md "Amendment 2, revision": one AC-18 exception; every other secret ref
  stays non-optional). Contract §8 and `rules.md` say so.
- `gen_fixtures.py`: the good waitingroom env marks the ref optional; 2 new mutants
  (`AC-18--waitingroom-queue-token-optional`, `AC-18--staging-waitingroom-queue-token-optional`). Regenerated;
  `--check` clean; now 188 bad fixtures; `test_policy.sh` unchanged.
