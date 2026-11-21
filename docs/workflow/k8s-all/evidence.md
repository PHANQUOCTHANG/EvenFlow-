# k8s-all: verification evidence (v3 step 6, independent verifier, High-risk)

Date 2026-11-21. Worktree `EvenFlow-k8s-all`, branch `feature/k8s-all`, HEAD `ce0fcbe`, base `c5fb598`.
Read-only on code; only this file and `verify-logs/**` were written. Logs: `docs/workflow/k8s-all/verify-logs/`.

## Verdict: **FAIL** (mutation threshold "all killed" not met; no product defect found)

- Every functional check passes on HEAD (validate.sh, test_policy.sh 245/245, test-workflows.sh 137/137).
- Mutation on `check_policy.py`: **23 of 28 killed, 5 survived** (M02, M20, M22, M23, M28). For each survivor the
  **original** checker rejects the bad input correctly (shown in `verify-logs/survivors/demo.txt`). The rules work,
  but the frozen self-test does not pin them down, so a later regression in those branches would go unnoticed.
- Mutation on `gates.sh` C4.4: **6 of 6 killed by a verifier-built stub-kubectl probe**, but **0 of 6 by the repo's
  own suites**. `test-workflows.sh` still passes 137/137 with the context guard removed (G1), and the full gate only
  runs `bash -n gates.sh`. The safety guard has no regression test in the repo.
- How to reach PASS: one test-only change request (TCR-3, needs human approval after the freeze) that adds 5 bad
  fixtures and a C4.4 stub test. No change to `check_policy.py` or `gates.sh` is needed. See "Remedy".

## 1. Gate commit vs HEAD

| Check | Result |
|---|---|
| `gate-report.json` commit_sha | `f746bcd` (verdict PASS, 10 checks exit 0, ac30/ac31 exit 77 NOT RUN) |
| Last code commit | `f746bcd fix(k8s): close round 2 findings` (no later commit touches code) |
| `git diff --stat f746bcd ce0fcbe` | 15 files, all under `docs/workflow/k8s-all/` (gate logs, `full-gate.sh`, `gate_secret_scan.py`, report) |
| Conclusion | gate commit == last code commit; HEAD differs only by docs. **OK** |

## 2. Proving checks re-run by the verifier at HEAD

| Command | Exit | Key output | Log |
|---|---|---|---|
| `bash deploy/k8s/validate.sh` | 0 | `selftest: 197/197`, `RESULT pass=228 fail=0 skip=0`, `validate: OK` | `verify-logs/validate.log` |
| `bash deploy/k8s/policy/test_policy.sh` | 0 | `selftest: 197/197`, `RESULT pass=245 fail=0 skip=0` (0 SKIP) | `verify-logs/test_policy.log` |
| `bash .github/scripts/tests/test-workflows.sh` | 0 | `137 PASS, 0 FAIL` | `verify-logs/test-workflows.log` |

Go build, the ai-worker pytest run (203 passed) and actionlint come from the gate at `f746bcd` (`gate/*.log`). The
verifier did not re-run them; the only non-deploy/docs/.github file changed since base is `.gitattributes` (LF for `*.env`).

## 3. Per-AC evidence

V = verified by a check the verifier ran; G = verified by the gate log at f746bcd; NV = NOT VERIFIED.

| AC | Evidence (test id / fixture) | Status |
|---|---|---|
| AC-1 | E-AC1-RENDER/DEPLOYMENTS(11)/NAMESPACE for local and staging; bad/AC-1--* | V |
| AC-2 | E-AC2-KUBECONFORM local 77/77 valid, staging 57/57 valid, 0 skipped | V |
| AC-3 | E-AC3-FLAT, E-AC3-SELECTOR; bad/AC-3--*; git shows the 4 web files renamed (R081-R092) into `base/web/` | V |
| AC-4..AC-11 | one or more bad/AC-n--* fixtures each, rejected with only that id (section C); M24/M25 (AC-9) killed | V |
| AC-12 | bad/AC-12--* incl. sub-check exemptions and `dev-infra-run-as-uid-0` (TCR-WP5); M01, M27 killed | V |
| AC-13 | D-USER-* (4 Dockerfiles numeric), C-DOCKERFILES-* (7 bad cases); M03/M04 killed, **M02 survived** (`USER 0` with no gid has no fixture). `docker build` + `docker inspect` part: Docker not available | V (grep part) / NV (image part) |
| AC-14 | bad/AC-14--* | V |
| AC-15 | bad/AC-15--* incl. endPort, empty from; M05/M06 killed | V |
| AC-16 | E-AC16-STAGING-SECRET, -HEADER, -DEVVALUES; bad/AC-16--*; M26 killed | V |
| AC-17 | bad/AC-17--* incl. DSN password, split flag, shell PGPASSWORD; M07/M08/M09 killed | V |
| AC-18 | bad/AC-18--* incl. envFrom secretRef, optional refs; M10/M11 killed | V |
| AC-19 | bad/AC-19--* | V |
| AC-20 | bad/AC-20--* incl. CronJob foreign image; M12/M13 (pod kinds list) killed | V |
| AC-21 | bad/AC-21--* incl. dev-infra in staging; M14 killed | V |
| AC-22 | bad/AC-22--* incl. duplicate env; M15 killed | V |
| AC-23 | bad/AC-23--* | V |
| AC-24 | validate.sh exit 0; E-AC24-POLICY both overlays; C-EXIT2-*; test-workflows AC-5 "kubeconform -strict" kept | V |
| AC-25 | 180 bad fixtures, each rejected with only its id; good-local/good-staging accepted. Mutation: 23/28, see section 4 | **partly: 5 survivors** |
| AC-26 | gates.sh C4.4 code read; behaviour shown with a stub kubectl (P1-P5 in `verify-logs/tools/gprobe.sh`): non-local context untouched, `docker-desktop-prod` lookalike rejected, unfinished migrate Job kept and excluded, finished Job deleted, every call pinned with `--context`. Real cluster dry run: no cluster | V (stub) / NV (real cluster) |
| AC-27 | E-AC27-COMPOSE `docker compose config -q` | V |
| AC-28 | test-workflows.sh 137/137 (V); go build and pytest at gate (G); no test deleted (TCR files only add) | V/G |
| AC-29 | runbook sections 1-9 read: build at one commit, load, staging Secrets, apply, readiness, port-forward, rollback, NOT VERIFIED list. Gap: no explicit commands for two AC-30 checks (pod deletion endpoints, scratch-namespace NetworkPolicy probe); given in section 7 below | V (minor doc gap) |
| AC-30 | no Docker daemon and no kube context on the verifier host (`kubectl config current-context`: not set) | **NV** |
| AC-31 | same | **NV** |
| AC-32 | bad/AC-32--* (20 fixtures); M16-M19 killed; **M20, M28 survived** | V (with gaps) |
| EXEMPT | bad/EXEMPT--*; M21, M24 killed; **M22, M23 survived** | V (with gaps) |

## 4. Mutation results

### 4.1 `deploy/k8s/policy/check_policy.py` (28 mutants)

Method: `verify-logs/tools/mutate.py` replaces one exact unique string, runs
`bash deploy/k8s/policy/test_policy.sh --fixtures-only` (sections A-D, the self-test), and restores the original
bytes. M01 ran in the worktree. M02-M28 ran in 6 parallel copies of `deploy/` in the scratchpad (one run takes about
3.5 min). An unmutated control copy passed (`selftest 197/197`, exit 0). After the run every copy and the worktree
file have the same md5 `a4385d82…`; `git status` shows only `verify-logs/`. Per-mutant logs: `verify-logs/mutation/Mnn.log`.

| Id | Mutant (behaviour removed) | Result | Killed by (first failing fixture) |
|---|---|---|---|
| M01 | AC-12 pod `runAsNonRoot` check | KILLED | C-AC-12--dev-infra-runasnonroot-false (+3) |
| M02 | AC-13 `USER` uid 0 check | **SURVIVED** | `USER 0:0` fixture still caught by the gid-0 branch; no `USER 0` fixture |
| M03 | AC-13 `USER` gid 0 check | KILLED | C-DOCKERFILES-AC-13--user-root-group |
| M04 | AC-13 non-numeric `USER` | KILLED | C-DOCKERFILES-AC-13--lowercase-suffix (+1) |
| M05 | AC-15 NetworkPolicy `endPort` | KILLED | C-AC-15--identity-endport |
| M06 | AC-15 empty `from` | KILLED | C-AC-15--identity-empty-from |
| M07 | AC-17 DSN `password=` regex never matches | KILLED | C-AC-17--gateway-args-dsn-password (+2) |
| M08 | AC-17 split-flag args (`--password` `x`) | KILLED | C-AC-17--gateway-args-split-password |
| M09 | AC-17 credentials-in-URL regex never matches | KILLED | C-AC-17--configmap-url-with-credentials (+2) |
| M10 | AC-18 `envFrom.secretRef` | KILLED | C-AC-18--web-envfrom-secret |
| M11 | AC-18 `optional: true` secretKeyRef | KILLED | C-AC-18--local-provider-key-optional (+1) |
| M12 | AC-20 pod kinds list cut to Deployment/StatefulSet/Job | KILLED | 5 fixtures |
| M13 | AC-20 pod kinds list without CronJob | KILLED | C-AC-20--cronjob-foreign-image |
| M14 | AC-21 dev-infra object in staging | KILLED | C-AC-21--staging-dev-infra-mailpit (+1) |
| M15 | AC-22 duplicate env names | KILLED | C-AC-22--staging-duplicate-env-gemini-mock (+1) |
| M16 | AC-32 `GEMINI_MOCK` must be "0" | KILLED | C-AC-32--gemini-mock-1-configmap |
| M17 | AC-32 `EMAIL_PROVIDER` must be smtp | KILLED | C-AC-32--email-provider-mock |
| M18 | AC-32 `GOOGLE_CLIENT_ID` non-empty | KILLED | C-AC-32--google-client-id-empty |
| M19 | AC-32 `envFrom.configMapRef` in staging | KILLED | C-AC-32--envfrom-configmap-prefix |
| M20 | AC-32 forbidden key in a staging ConfigMap | **SURVIVED** | every *-configmap fixture also references the key from env, so the env check fires |
| M21 | EXEMPT pair not in allow-list | KILLED | C-EXEMPT--unknown-pair-gateway-ac12 (+3) |
| M22 | EXEMPT empty reason | **SURVIVED** | the only empty-reason fixture uses a pair that is not allowed anyway |
| M23 | EXEMPT malformed key silently ignored | **SURVIVED** | no fixture has a malformed key |
| M24 | AC-9 non-exemptable (guard removed and AC-9 in the worker allow-list) | KILLED | C-EXEMPT--worker-ac9 |
| M25 | AC-9 HPA on a fixed-replica workload | KILLED | C-AC-9--hpa-on-controller (+1) |
| M26 | AC-16 Secret rendered in staging | KILLED | C-AC-16--staging-plain-secret |
| M27 | AC-12 web exemption hard-coded (sub-check ignored) | KILLED | C-AC-12--web-exempt-runasnonroot-missing (+1) |
| M28 | AC-32 provider callback URLs non-empty | **SURVIVED** | absent-URL fixtures fail via "absent"; no empty-URL fixture |

Equivalent-mutant note: removing only the AC-9 guard (without adding AC-9 to an allow-list) changes nothing, because
AC-9 is in no allow-list, so "not allowed" fires. M24 is the non-equivalent form and is killed.

Impact of the survivors (the original checker rejects each case, demonstrated in `verify-logs/survivors/demo.txt`):

| Id | Input a regression would let through | Risk |
|---|---|---|
| M02 | Dockerfile ending `USER 0` (root, no gid) | Medium for the rule. The real tree is still covered by D-USER-* exact greps |
| M22 | An allowed exemption with an empty reason (`policy-exempt.AC-12.readOnlyRootFilesystem: " "`) | Low/medium: the exemption would be accepted with no justification |
| M23 | A malformed key (`policy-exempt.AC-12.read-only`) | Low: it is ignored and suppresses nothing, but the contract says it must fail EXEMPT |
| M28 | Staging `VNPAY_RETURN_URL=""` (likewise MOMO URLs) | Medium: weakens the E8 fail-closed staging guarantee |
| M20 | A forbidden key in a staging ConfigMap that no container references | Low: inert unless mounted; envFrom is blocked separately (M19 killed) |

### 4.2 `.github/scripts/gates.sh` C4.4 (6 mutants)

Method: `verify-logs/tools/gprobe.sh` pulls the C4.4 block out of a gates.sh copy and runs it against a stub
`kubectl` that records every call. Mutants were written to scratchpad copies only. The control (unmutated) gives PROBE PASS.

| Id | Mutant | Probe | Repo suite |
|---|---|---|---|
| G1 | context guard accepts any context | KILLED (P1: `--context prod-cluster get namespace`) | **test-workflows.sh 137/137 PASS (survives)**; `verify-logs/test-workflows-G1.log` |
| G2 | guard regex unanchored (`docker-desktop-prod` accepted) | KILLED (P5) | not covered (only `bash -n`) |
| G3 | `kc()` without `--context` pin | KILLED (P3/P4) | not covered |
| G4 | delete migrate Job even when not finished | KILLED (P2) | not covered |
| G5 | unfinished Job not excluded from the dry run | KILLED (P2) | not covered |
| G6 | non-local branch removed | KILLED (P1) | not covered |

Total kill count: **29 of 34 by verifier harnesses** (23/28 policy + 6/6 gates). **23 of 34 by the repo's own suites** (0/6 for gates.sh).

### Remedy (test-only, TCR-3, needs human approval)

1. `dockerfiles/bad/AC-13--user-uid-0-no-gid` (`USER 0`): kills M02.
2. `bad/EXEMPT--web-allowed-pair-empty-reason`: kills M22.
3. `bad/EXEMPT--gateway-malformed-key`: kills M23.
4. `bad/AC-32--vnpay-return-url-empty` (staging): kills M28.
5. `bad/AC-32--unreferenced-forbidden-configmap-key` (staging): kills M20.
6. A `.github/scripts/tests/` case that runs C4.4 against a stub kubectl (the same idea as `gprobe.sh`): kills G1-G6.

## 5. Rollback

- Base `c5fb598` has the web manifests at `deploy/k8s/{00-namespace,10-deployment,20-service,30-ingress,40-hpa}.yaml`.
- HEAD: `00-namespace.yaml` deleted; the other four moved with `git mv` (similarity 81-92 %) to
  `deploy/k8s/base/web/{deployment,service,ingress,hpa}.yaml`. Plus `base/`, `components/`, `overlays/`, `policy/`,
  `tools.env`, `validate.sh`.
- In a temporary worktree of `c5fb598`, `test-workflows.sh` gives 137 PASS / 0 FAIL
  (`verify-logs/rollback-base-test-workflows.log`). Base gates.sh C4.4 globs `deploy/k8s/**/*.yaml`, and base ci.yml
  runs kubeconform on the flat dir.
- Simulated merge and revert in that worktree: `git merge --no-ff ce0fcbe`, then `git revert -m 1 HEAD`.
  `git diff c5fb598 HEAD -- deploy .github` is **empty**, and `deploy/k8s/` again holds exactly the 5 flat files. So
  reverting the merge restores the CI and gates behaviour. The worktree was removed and nothing was pushed.
- Runtime rollback (`kubectl rollout undo`, runbook 6.1-6.3): not exercised (no cluster).

## 6. NOT VERIFIED

- **AC-30** (local apply on Docker Desktop/kind) and **AC-31** (staging server dry run): this host has no Docker
  daemon and no kube context. The gate also records them as exit 77 NOT RUN.
- **AC-13** image part: `docker build` + `docker inspect -f '{{.Config.User}}'`.
- **AC-26** against a real cluster (verified only with the stub kubectl).
- Runtime rollback; NetworkPolicy enforcement (Docker Desktop does not enforce it, spec AC-30).
- Go, web and python suites were not re-run by the verifier (gate logs only).

## 7. For the human: TCR-WP5, AC-30/AC-31, MP-1/MP-2

**What TCR-WP5 was.** In WP5 the policy author ran a mutation check and found that switching off the AC-12 `rootUid`
sub-check (pod `runAsUser: 0`) did not fail any test. The only uid-0 fixture was an app pod, which the `noRunAsUser`
check also catches. `test-change-request-wp5.md` (commit `ae9b58a`) proposed two things:
(a) one new fixture, `bad/AC-12--dev-infra-run-as-uid-0.yaml`, a postgres pod with `runAsUser: 0`, which only
`rootUid` can catch;
(b) a two-id test for an invalid exemption that takes effect.
WP6 applied (a) as an addition only, in commit `0850849`: no existing assertion changed and `gen_fixtures --check`
is clean; the fixture count went from 152 to 153. (b) was not applied, because contract §7 allows one rule id per
fixture; it is recorded as an accepted gap in `rules.md`. Later commits `8486f82` and `f746bcd` only update
`tcr-applied.md`. **No dated human approval of TCR-WP5 exists.** That is MP-2.

**What AC-30 / AC-31 would prove.**
- AC-30: on a real local cluster, overlays/local actually runs. The migrate Job completes and every pod becomes Ready.
  Gateway `/healthz` returns 200. Deleting one gateway pod never leaves 0 Ready endpoints, which tests the PDB, the
  rolling update and the preStop hook. A pod in another namespace cannot reach ticketing:8082 while gateway can, but
  only on a CNI that enforces NetworkPolicy (kind with Calico); on Docker Desktop this must be stated as not enforced.
- AC-31: the API server itself (admission, schema, PSA `restricted`) accepts the staging render. "Secret not found"
  warnings are expected.

Commands (runbook sections 0-3 and 4.4, plus the two AC-30 checks the runbook does not spell out):

```bash
kubectl config use-context docker-desktop        # or: kind create cluster --name ef && kubectl config use-context kind-ef
bash deploy/k8s/validate.sh                       # builds pinned kustomize; KUSTOMIZE=$(ls ~/.cache/eventflow-k8s/bin/kustomize-v5.7.1/kustomize* | head -1)
# runbook 1: build all images at one commit (TAG=sha-<short>); kind only: kind load docker-image ... for each
# runbook 2: temporary overlay RUN=deploy/k8s/overlays/run.XXXX with `kustomize edit set image` (+ ephemeral-data on Docker Desktop)
kubectl get ns eventflow >/dev/null 2>&1 || kubectl create namespace eventflow
"$KUSTOMIZE" build "$RUN" | kubectl apply --dry-run=server -f - && "$KUSTOMIZE" build "$RUN" | kubectl apply -f -
kubectl -n eventflow wait --for=condition=complete job/eventflow-migrate --timeout=5m
for d in $(kubectl -n eventflow get deploy -o name); do kubectl -n eventflow rollout status "$d" --timeout=5m; done
kubectl -n eventflow port-forward svc/eventflow-gateway 8080:8080 & curl -fsS http://localhost:8080/healthz
# AC-30 endpoints never 0 while one gateway pod is deleted:
P=$(kubectl -n eventflow get pod -l app.kubernetes.io/name=eventflow-gateway -o jsonpath='{.items[0].metadata.name}')
kubectl -n eventflow delete pod "$P" --wait=false
for i in $(seq 30); do kubectl -n eventflow get endpointslices -l kubernetes.io/service-name=eventflow-gateway -o jsonpath='{range .items[*].endpoints[?(@.conditions.ready==true)]}x{end}{"\n"}'; sleep 1; done   # never an empty line
# AC-30 NetworkPolicy (meaningful only on an enforcing CNI):
kubectl create ns scratch; kubectl -n scratch run probe --rm -i --restart=Never --image=busybox:1.36 -- wget -qO- -T 3 http://eventflow-ticketing.eventflow:8082/healthz   # expect timeout
kubectl -n eventflow exec deploy/eventflow-gateway -- /bin/true 2>/dev/null || echo "gateway is distroless: test gateway->ticketing via a labelled debug pod in eventflow instead"
# AC-31 (staging): create the Secrets of runbook 4.2 (or accept "secret not found" warnings), then
OVERLAY=staging  # rebuild RUN per runbook 2
"$KUSTOMIZE" build "$RUN" | kubectl apply --dry-run=server -f -
```

**MP-1 / MP-2 (merge preconditions, human only; `merge-preconditions.md`).**
- MP-1: AC-30/AC-31 are NOT VERIFIED. Either run the commands above and attach the output here, or write a dated
  line accepting the merge with them NOT VERIFIED.
- MP-2: confirm with a date that TCR-WP5 is approved (it adds the single fixture
  `bad/AC-12--dev-infra-run-as-uid-0.yaml` and changes no assertion), or ask for it to be reverted.

Both are still `_pending_`. Separately from both, this verification asks for TCR-3 (section 4, Remedy) before the
mutation threshold can be called met.
