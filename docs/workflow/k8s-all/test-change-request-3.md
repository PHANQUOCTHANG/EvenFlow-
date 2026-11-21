# TCR-3: tests for the verifier's mutation survivors (k8s-all)

- Status: **approved** by TranDinhVo on 2026-11-21 in the Claude Code session (`merge-preconditions.md`, "Human decisions").
- Source: `evidence.md` section 4 "Remedy". The verifier found 5 surviving mutants of `check_policy.py`
  (M02, M20, M22, M23, M28) and 6 `gates.sh` C4.4 mutants (G1-G6) that no repo suite killed.
- Scope: **additions only**. No existing fixture, test or assertion changed. `check_policy.py`, `gates.sh` and
  `test_policy.sh` are unchanged. `gen_fixtures.py` gains 21 lines and loses 0; regenerating changed no other file and
  `python -I gen_fixtures.py --check` is clean.

## 1. Policy fixtures (through `deploy/k8s/policy/testdata/gen_fixtures.py`)

Each fixture prints FAIL lines for exactly one rule id (contract §7). The original checker rejects each one; the
listed mutant accepts it, so the self-test fails under that mutant.

| Fixture | Overlay / processes | Input | Original checker | Kills |
|---|---|---|---|---|
| `dockerfiles/bad/AC-13--user-uid-0-no-gid/go-service.Dockerfile` | `--dockerfiles` | final stage `USER 0` (no group part) | `FAIL AC-13` (uid 0 is root) | M02 |
| `bad/AC-32--unreferenced-forbidden-configmap-key.yaml` | staging, web,gateway | new ConfigMap `eventflow-unused-flags` with `PAYMENT_SANDBOX_MODE`, referenced by no container | `FAIL AC-32 ConfigMap/eventflow-unused-flags` only | M20 |
| `bad/EXEMPT--web-allowed-pair-empty-reason.yaml` | local, web,gateway | web keeps its allowed `AC-12.readOnlyRootFilesystem` exemption but with reason `" "`; the container sets `readOnlyRootFilesystem: true`, so no AC-12 line fires | `FAIL EXEMPT` (empty reason) only | M22 |
| `bad/EXEMPT--gateway-malformed-key.yaml` | local, web,gateway | gateway annotation `eventflow.io/policy-exempt.AC-12.read-only` | `FAIL EXEMPT` (malformed key) only | M23 |
| `bad/AC-32--vnpay-return-url-empty.yaml` | staging, web,gateway,payment | `eventflow-app-config` `VNPAY_RETURN_URL: ""` | `FAIL AC-32 Deployment/eventflow-payment` only | M28 |

Counts: 180 -> 184 bad YAML fixtures, 7 -> 8 bad Dockerfile cases, selftest 197 -> 202.

## 2. gates.sh C4.4 behaviour test

- New script `.github/scripts/tests/test-gates-context-guard.sh` (modelled on the verifier's `gprobe.sh`). It cuts the
  C4.4 block out of `gates.sh` (from `K8S_CTX=""` to `# C4.5 -- kiem`), runs it against a stub `kubectl` that records
  every call, and fails at once if the block cannot be found. `SUT=<file>` points it at another copy (mutation runs).
- 11 cases: four non-local contexts (`prod-cluster`, `docker-desktop-prod`, `staging-kind-dev`, `xkind-dev`) touch no
  cluster and print the skip line; on `docker-desktop` an unfinished migrate Job is kept and excluded from the dry run,
  the namespace is created on the pinned context and every call carries `--context docker-desktop`; on `kind-dev` a
  finished Job is deleted on the pinned context and every call is pinned; on `kind-ci` with no Job nothing is deleted
  or excluded; an empty current-context touches no cluster.
- Wiring:
  - CI: the `lint-ci` job already runs every `.github/scripts/tests/*.sh` (and its path filter covers
    `.github/scripts/**`, which includes `gates.sh`), so the new script runs in CI with no `ci.yml` edit.
  - `test-workflows.sh` is not edited (no existing assertion touched).
  - `docs/workflow/k8s-all/full-gate.sh` gains the step `gates-context-guard`.

## 3. Verifier tool added

- `verify-logs/tools/gmutate.py`: writes G1-G6 (the mutants of `evidence.md` 4.2) to scratch copies of `gates.sh` and
  runs both the new repo test (with `SUT=`) and `gprobe.sh` on each. The worktree `gates.sh` is never written.
