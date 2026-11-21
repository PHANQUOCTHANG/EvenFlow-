# Test change request from WP5 (policy checker)

Status: **open, optional** (no fixture is wrong; this is a coverage gap found by the mutation check). Needs human
approval like any test change after freeze. WP5 did not edit `test_policy.sh` or `testdata/**`.

## Context

`check_policy.py` passes the frozen self-test (`selftest: 165/165` with `--fixtures-only`). The AC-25 mutation check
kills all 24 rule-level mutants (every rule id incl. AC-13 and EXEMPT). Two finer-grained mutants survive:

| Mutant | Why it survives | Proposal |
|---|---|---|
| AC-12 sub-check `rootUid` disabled | The only fixture with `runAsUser: 0` (`AC-12--identity-container-uid-0`) is an app workload, where `noRunAsUser` also fires, so the fixture still fails with AC-12 | Add `bad/AC-12--dev-infra-run-as-uid-0.yaml` (local): postgres pod `securityContext.runAsUser: 0`. Dev-infra may set `runAsUser`, so only `rootUid` can catch it. Expect AC-12 only |
| Invalid exemption takes effect (e.g. gateway `AC-12.readOnlyRootFilesystem` accepted) | Every EXEMPT fixture adds an invalid key whose target rule already passes, so whether the key suppresses the rule is not observable. A fixture where it would matter trips AC-12 **and** EXEMPT, which the one-id-per-fixture criterion (contract §7) does not allow | Either accept the gap, or add a `test_policy.sh` case that expects exactly the two ids `AC-12 EXEMPT` for a gateway with `readOnlyRootFilesystem: false` plus `policy-exempt.AC-12.readOnlyRootFilesystem` (needs a contract §7 amendment for multi-id cases) |

Neither gap blocks AC-25 as written ("disabling any single rule makes the self-test fail").
