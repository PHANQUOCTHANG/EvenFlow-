# Plan: k8s-all (round 3, addresses plan-review.md rounds 1 and 2)

Inputs: `spec.md` (AC-1..AC-32, errata E1-E8, decisions D1-D7, **all approved** by the human on 2026-11-01, plus the
Stripe-disabled-in-staging decision), `policy-contract.md` revision 2 (frozen), base develop @ c5fb598.
No migrations, no service code changes, no new runtime dependencies. New CI tools are pinned Go modules (D1, E7).
**Section "Round 3" at the end overrides sections 1-9 and "Round 2" wherever they disagree; "Round 2" overrides
sections 1-9.**

## 1. Target layout

```
deploy/k8s/
  base/
    kustomization.yaml            # resources only; NO commonLabels / labels-with-selectors (AC-3)
    namespace.yaml                # moved from 00-namespace.yaml; PSA labels set by overlays (AC-14)
    networkpolicy/                # default-deny-ingress.yaml + one allow-<dest>.yaml per row of spec 6.2 (AC-15)
    web/                          # deployment.yaml (= old 10-deployment.yaml + PDB/affinity/spread/automount), service, ingress, hpa, pdb
    gateway/                      # deployment, service (8080 + metrics 9094), ingress (api host, SSE annotations), hpa, pdb
    identity/ event/ waitingroom/ ticketing/ payment/ ai-worker/   # deployment, service, hpa, pdb
    notification/                 # deployment, service (health/metrics only), hpa, pdb
    waitingroom-controller/       # deployment (replicas 2, cmd /controller), pdb
    ticketing-worker/             # deployment (replicas 1, Recreate, cmd /worker), pdb
  components/dev-infra/           # kind: Component
    kustomization.yaml
    postgres.yaml pgbouncer.yaml redis.yaml rabbitmq.yaml mailpit.yaml   # StatefulSet/Deployment + Service each
    migrate-job.yaml              # goose Job, image eventflow-migrate, DATABASE_URL_DIRECT
    networkpolicy.yaml            # dev-infra reachable only from namespace eventflow
  overlays/local/
    kustomization.yaml            # base + component; images: (sha tags); namespace labels baseline/warn restricted
    endpoints.env                 # configMapGenerator -> eventflow-endpoints (REDIS_ADDR=eventflow-redis:6379, ...; APP_ENV=development)
    secrets.dev.env               # secretGenerator source, "DEV ONLY" header, all values contain "dev" (AC-16)
  overlays/staging/
    kustomization.yaml            # base only; images:; namespace enforce restricted; no Secrets
    endpoints.env                 # placeholder external hosts (redis.example.internal ...), APP_ENV=production
  policy/
    check_policy.py               # python3 stdlib only; input = JSON list of rendered objects
    rules.md                      # rule id -> AC mapping, allowed exemptions (spec 7)
    testdata/good/…  testdata/<RULE-ID>-*.yaml   # written by the TEST DESIGNER (step 2a), protected afterwards
    test_policy.sh                # runs checker on good (must pass) and each bad fixture (must fail with its id)
  validate.sh                     # entry point used by CI, gates and humans
deploy/docker/migrate.Dockerfile  # FROM ghcr.io/kukymbr/goose-docker:3.22.0 + COPY migrations
deploy/docker/{go-service,ticketing,waitingroom,ai-worker}.Dockerfile   # numeric USER only (AC-13)
.github/workflows/ci.yml          # validate-deploy steps only (D1)
.github/scripts/gates.sh          # C4.4 only (AC-26)
docs/workflow/k8s-all/runbook.md  # AC-29
docs/workflow/deploy-k8s-web/runbook.md   # one-line pointer (AC-29)
```

Deleted: `deploy/k8s/{00-namespace,10-deployment,20-service,30-ingress,40-hpa}.yaml` (moved with `git mv` so
history and the long comments survive).

Why kustomize: one base, two environments differing only in endpoints, secrets source, PSA level and dev infra;
kubectl ≥1.14 has it built in (no new local tool); plain manifests would need copy-paste per environment.

## 2. Validation pipeline (`deploy/k8s/validate.sh`)

1. For each overlay in `local staging`: render with `kubectl kustomize` (local kubectl if present, else
   `docker run registry.k8s.io/kubectl:v1.32.x kustomize`) into `$TMP/<overlay>.yaml`.
2. `kubeconform -strict -summary -kubernetes-version 1.32.0 -` on each render (docker image
   `ghcr.io/yannh/kubeconform:v0.8.0`, already used by CI). Fail on any invalid/error/skipped.
3. Convert each render to JSON with `mikefarah/yq:<pinned 4.x>` (`yq ea -o=json '[.]'`), then
   `python3 -I deploy/k8s/policy/check_policy.py --overlay <name> <json>`; prints `FAIL <RULE-ID> <kind>/<name>: <why>`
   per violation, exit 1 if any.
4. `bash deploy/k8s/policy/test_policy.sh` (self-test, AC-25) + grep of numeric USER in the 4 Dockerfiles (AC-13).
5. Prints a summary; any step failing → non-zero exit with the step name (same style as `gates.sh`).

Exact image tags (kubectl, yq) are chosen at implementation time from tags verified with
`docker manifest inspect` and written once in `validate.sh`; CI and local use the same variables.

### CI change (D1, protected path) — the whole diff to `ci.yml`

`validate-deploy` replaces the single raw-directory step with:
```yaml
- name: kubeconform -strict (rendered overlays)
  run: |
    set -euo pipefail
    for o in local staging; do
      docker run --rm -v "$PWD/deploy/k8s:/k:ro" "$KUBECTL_IMAGE" kustomize "/k/overlays/$o" \
        | docker run --rm -i ghcr.io/yannh/kubeconform:v0.8.0 -strict -summary -kubernetes-version 1.32.0 -
    done
- name: k8s policy + policy self-test
  run: bash deploy/k8s/validate.sh --skip-kubeconform
```
The kubeconform line stays literally in the step `run`, so `test-workflows.sh` AC-5 (F2) keeps matching without a
test change. `KUBECTL_IMAGE` is a job-level `env:` with the pinned tag. Paths filter `deploy` already covers
`deploy/k8s/**` (Dockerfiles stay covered by trivy/build). compose and promtool steps unchanged. `lint-ci` must stay
green (actionlint + `.github/scripts/tests/*.sh`).

`gates.sh` C4.4: when `kubectl cluster-info` succeeds, `kubectl kustomize overlays/local` → apply only the Namespace
for real (filter `kind: Namespace`), then `kubectl apply --dry-run=server -k deploy/k8s/overlays/local`. The "no
cluster → say nothing was checked" branch is kept verbatim.

## 3. Work packages and order

Contracts frozen by the spec (4.1, 6.1, 6.2, 7). Max 3 implementers in parallel.

| WP | Content | AC | Depends |
|---|---|---|---|
| WP0 test design (separate agent, parallel with this plan) | `policy/testdata/**`, `policy/test_policy.sh`, `traceability.yaml`, `baseline.json`; RED check = `test_policy.sh` fails because `check_policy.py` does not exist / rejects nothing | AC-25 | spec |
| WP1 images | numeric USER in 4 Dockerfiles; `migrate.Dockerfile` | AC-13, AC-27 | — |
| WP2 base apps | `git mv` web files into `base/web/` + PDB/affinity/spread/automount; 10 other processes per 4.1; Services; HPAs; PDBs; gateway Ingress | AC-3..AC-12, AC-18, AC-20, AC-22, AC-23 | contracts |
| WP3 security + config | namespace, NetworkPolicies (6.2), overlays local/staging (endpoints, secret generator, PSA labels, images:) | AC-1, AC-14..AC-17, AC-19 | WP2 skeleton (names only) |
| WP4 dev infra | component: Postgres (16-alpine, PVC 1Gi, `pg_isready -h 127.0.0.1` probe), PgBouncer (pinned, scram), Redis 7-alpine (same flags as compose minus 2gb → 256mb), RabbitMQ 3.13 (`check_port_connectivity` probe, startup 120 s), Mailpit v1.21, migrate Job | AC-8, AC-21, AC-30 prep | contracts |
| WP5 policy + validate | `check_policy.py`, `rules.md`, `validate.sh` until WP0 self-test is green | AC-2, AC-24, AC-25 | WP0 |
| WP6 integrator | merge WP1-5; CI + gates change (after D1); runbook; full gate; AC-30/31 evidence if Docker is up | AC-26..AC-31 | all |

Order: WP0 ∥ WP1 ∥ WP2 → WP3 ∥ WP4 ∥ WP5 → WP6. Checkpoint commit after each WP with fast gate =
`bash deploy/k8s/validate.sh` (what exists so far) + `docker compose config -q` + `bash .github/scripts/tests/test-workflows.sh`.

## 4. Implementation notes that matter

- No `replicas` on HPA-managed Deployments (web slice C2.3). Controller `replicas: 2`, worker `replicas: 1` + `Recreate`.
- `preStop: {sleep: {seconds: 5}}` (core API since 1.29; distroless has no `sleep` binary). Web keeps `node -e`.
- `terminationGracePeriodSeconds`: 30 for all except gateway/waitingroom/ticketing = 35 (5 + 20 + margin).
- Probe numbers: startup period 2 s × 30; liveness period 10 s, timeout 2 s, failure 3; readiness period 5 s
  (web keeps 2 s/failure 1 from its slice), timeout 2 s.
- ai-worker: `readOnlyRootFilesystem: true` + `emptyDir` `/tmp`; env `REDIS_URL` from endpoints, `INTERNAL_API_BASE`
  = `http://eventflow-gateway:8080`, `MODERATION_API_BASE` = `http://eventflow-event:8084`.
- Secret env names keep compose names: `secretKeyRef {name: eventflow-mq, key: AMQP_URL}` mapped to `RABBITMQ_URL`
  for event/payment/ticketing-worker and to `AMQP_URL` for ticketing/notification/ai-worker.
- Pod annotations `prometheus.io/scrape: "true"`, `prometheus.io/port`, `prometheus.io/path: /metrics`.
- Images in base use tag `sha-c5fb598` placeholder only through overlay `images:` (AC-20); runbook builds every
  image from `git archive <sha>` and sets the tag with a temporary overlay (`kustomize edit set image` in a temp dir
  that references `overlays/local`), never by editing committed files.
- `secrets.dev.env` values copied from compose defaults (`dev-jwt-secret-change-me-0123456789`, …) and URLs built
  for in-cluster hosts (`postgres://eventflow:dev-eventflow-pw@eventflow-postgres:5432/...`). Postgres password
  changes from `eventflow` to a `dev-…` value so AC-16 ("contains dev") holds; compose untouched.
- Secret scan (full gate): add a single allow entry for `deploy/k8s/overlays/local/secrets.dev.env` with reason.

## 5. Risks (plan level)

| Risk | Mitigation |
|---|---|
| kubeconform fails on kustomize files once raw dir is no longer scanned | CI validates rendered output only (D1) |
| `test-workflows.sh` AC-5 assertion breaks on the CI edit | inline kubeconform line in the step `run`; run `lint-ci` tests locally before push |
| `registry.k8s.io/kubectl` / yq image tag does not exist | verify with `docker manifest inspect` at WP5; fallback local kubectl (present) and `python3 -m` JSON conversion is NOT possible without PyYAML → yq stays required in CI |
| Docker Desktop not running → no runtime evidence | AC-30 allows NOT VERIFIED with human sign-off; AC-1..29 are offline |
| dev-infra images need root (rabbitmq/postgres entrypoints) | local PSA = baseline (AC-14); exemptions listed per workload (spec 7) |
| Teammates edit `deploy/k8s/*.yaml` or `ci.yml` meanwhile | `git merge origin/develop` before PR, rerun full gate |
| Large diff hard to review | one directory per process, identical structure; policy proves the invariants mechanically |

## 6. Rollback

- Git: `git revert -m 1 <merge-commit>` restores flat web manifests, old CI step and Dockerfile USER lines.
  Check during verification: on a rollback branch run `docker run … kubeconform -strict … /m` on the restored
  `deploy/k8s` (old CI command) and `bash .github/scripts/tests/test-workflows.sh`.
- Cluster: object names of web are unchanged, so `kubectl apply -f deploy/k8s/` from the reverted tree updates
  in place; new objects (other services, NetworkPolicies) are removed with
  `kubectl delete -k deploy/k8s/overlays/local` **before** reverting, or by deleting namespace `eventflow` on a dev
  cluster. Per-Deployment rollback: `kubectl -n eventflow rollout undo deployment/<name>`.
- NetworkPolicy lockout: `kubectl -n eventflow delete networkpolicy --all` restores open ingress immediately.

## 7. Mutation test (threshold: all killed)

For each rule id in `rules.md`, comment out its check in `check_policy.py` → `test_policy.sh` must fail
(≥ 1 fixture accepted wrongly). Plus three manifest mutants on the real tree, each must make `validate.sh` fail:
(1) liveness of ticketing → `/readyz`, (2) `DATABASE_URL` as literal `value:` in payment, (3) drop the PDB of
gateway.

## 8. If a decision is not approved

- D1 refused: no kustomize; plain manifests in `deploy/k8s/<process>/`, two environments collapse to one (local);
  policy runs only via `validate.sh` locally and in gates; spec AC-24/AC-26 become "manual evidence".
- D2 refused: manifests add `runAsUser: 65532` / `10001` and the policy allows it for those images only (weaker:
  hides future Dockerfile regressions).
- D5 refused: worker gets HPA-less `replicas: 2` + affinity like others; requires a concurrency proof for the
  reconciler first (out of this task).
- D6 changed to "all external": dev-infra component dropped, AC-30 cannot run end to end locally.

## 9. Expected test-change-request

None expected: no existing test changes. If the CI edit forces an edit to `.github/scripts/tests/test-workflows.sh`
(e.g. the AC-5 check must point to an overlay path), it goes through a test change request for human approval.

## Round 2 (plan-review.md round 1: FAIL, 3 major + 13 minor)

### Finding → resolution

| Finding | Resolution |
|---|---|
| **M1** checker interface not frozen | New frozen `policy-contract.md`: rule id = AC number (table §2, incl. new AC-32 and `EXEMPT`), CLI `check_policy.py --overlay {local,staging} --input <json or -> [--processes <csv>]`, input = JSON array produced only by pinned yq, output regex `^FAIL (AC-[0-9]+ or EXEMPT) Kind/name: reason$` (exact regex in contract §5) + fixed last line, exit codes 0/1/2, fixture overlay selection via first line `# policy-args: …`, one-fixture-one-rule acceptance, multi-rule exemption `eventflow.io/policy-exempt.AC-<n>: "<reason>"` (ticketing-worker carries AC-9 and AC-11). Spec erratum E1. WP0 gets spec + erratum + contract; WP0 has already started in this worktree (`deploy/k8s/policy/`, `traceability.yaml`, `baseline.json` are untracked here), so it re-aligns its fixtures to the contract before the freeze |
| **M2** offline validation needs Docker | All tools pinned in `deploy/k8s/tools.env` and run with `go run` by default (`TOOLS=go`): kustomize `sigs.k8s.io/kustomize/kustomize/v5@v5.7.1` (renders; kubectl is no longer used for rendering anywhere), kubeconform `github.com/yannh/kubeconform/cmd/kubeconform@v0.8.0` (same version as today's CI image), yq `github.com/mikefarah/yq/v4@v4.44.3`. Verified on this Windows machine 2026-11-01 with Go 1.27 and the Docker daemon down: all three ran with `go run` (kustomize printed `v5.7.1`, yq `v4.44.3`, kubeconform ran). `validate.sh` finds `go` on PATH or at `/c/Program Files/Go/bin/go`, uses `python3` or `python`, and runs kubeconform with `-cache "$TMP/kubeconform-cache"`. Optional `TOOLS=docker` sets `MSYS_NO_PATHCONV=1` for every `docker -v` and mounts `$(pwd -W 2>/dev/null || pwd)`. Corrected claim (spec E7): "no cluster and no Docker; network only on first run for Go modules and JSON schemas". The fast gate, RED check, mutation test and full gate all run locally this way |
| **M3** demo settings leak into staging | Per-overlay ConfigMap `eventflow-app-config` (configMapGenerator from `overlays/<o>/app-config.env`), referenced with `configMapKeyRef` (`optional: true` for keys staging omits). Staging values are listed explicitly in spec E2: no `MIN_SALE_LEAD_SECONDS`, `ACTIVE_EVENT_IDS`, `ADMIT_*` or `PAYMENT_SANDBOX_MODE`; `GEMINI_MOCK=0` with a required key; providers vnpay/momo/stripe (decision D7). New AC-32 + policy rule + ≥ 3 fixtures (`policy-contract.md` §7). `eventflow-endpoints` stays for infra hosts only |
| m1 secrets least privilege | Spec erratum E3: per-process key table. Gateway gets only `JWT_SECRET`; identity gets no `QUEUE_TOKEN_SECRET`; the ticketing server gets no MQ URL; payment provider secrets are added; `secretKeyRef` only, never `envFrom.secretRef`. AC-18 checks that table |
| m2 inconsistent pinning | CI renders once with the pinned kustomize into `$RUNNER_TEMP/k8s-render/{local,staging}.yaml`; kubeconform and the policy both read those files (`validate.sh --render-dir`). `validate.sh` prints the kustomize, kubeconform and yq versions at the top of its log |
| m3 migrate Job immutable | `ttlSecondsAfterFinished: 600`; the runbook and `gates.sh` C4.4 run `kubectl -n eventflow delete job eventflow-migrate --ignore-not-found` before apply/dry-run; env `GOOSE_DRIVER=postgres`, `GOOSE_DBSTRING ← eventflow-db/DATABASE_URL_DIRECT` |
| m4 Dockerfile changes skip validate-deploy | Add `'deploy/docker/**'` to the `deploy` paths filter (spec E6). A small change inside the approved validate-deploy scope, recorded |
| m5 dev PII keys | `secrets.dev.env` gets two 44-char base64 values that contain `dev` and decode to 32 bytes. WP3 proves this with `python3 -c "import base64,sys; assert len(base64.b64decode(sys.argv[1]))==32" <v>`; identity startup proves it at runtime (AC-30) |
| m6 ai-worker grace | App grace 20 s (E4) → `terminationGracePeriodSeconds: 35`; the policy gets a numeric value |
| m7 preStop.sleep version | Minimum Kubernetes 1.30 (E5) in the runbook and the staging assumptions. The "core since 1.29" text in §4 is wrong (alpha there, feature gate off) |
| m8 gates C4.4 namespace | `kubectl get ns eventflow >/dev/null 2>&1 || kubectl create namespace eventflow`, delete the migrate Job, then `kubectl apply --dry-run=server -k deploy/k8s/overlays/local`. The no-cluster branch is kept verbatim. No yq in gates |
| m9 dev-infra sizing | Values fixed for WP4. Postgres: StatefulSet, PVC 1Gi, `shared_buffers=128MB`, `max_connections=200`, limits 1 CPU/768Mi. Redis: `--maxmemory 256mb`, limit 512Mi. RabbitMQ: **StatefulSet** (stable hostname), PVC 1Gi, probe `rabbitmq-diagnostics -q check_port_connectivity` with `timeoutSeconds: 15`, startup 5 s × 36, limit 1Gi. PgBouncer: pinned tag (verified against the registry at WP4), `AUTH_TYPE=scram-sha-256`. Mailpit v1.21 |
| m10 secret-scan exclusion | Run the full-gate scan first. Only if it hits, exclude exactly `:(exclude)deploy/k8s/overlays/local/secrets.dev.env` (never `*.env`); the AC-16 "contains dev" check is the compensating control |
| m11 delete -k removes namespace | Runbook rollback states: "`kubectl delete -k overlays/local` deletes Namespace `eventflow`, together with web and the dev PVC data". To keep the namespace: `kubectl -n eventflow delete deploy,sts,job,svc,ingress,hpa,pdb,networkpolicy,cm,secret -l app.kubernetes.io/part-of=eventflow` (every object carries that label) |
| m12 placeholder tag | Overlays carry `newTag: sha-c5fb598` with the comment `# PLACEHOLDER: no image is built from this commit; runbook sets the real tag via a temp overlay` |
| m13 header | Updated (top of file) |
| (new in round 2) CRLF in `.env` | `.gitattributes` gains `*.env text eol=lf`. With `core.autocrlf=true`, a CRLF `secrets.dev.env` / `app-config.env` would put `\r` into every Secret/ConfigMap value |

### Changed pieces of the plan

**Layout additions:** `deploy/k8s/tools.env`; `overlays/{local,staging}/app-config.env`; one line in `.gitattributes`.

**validate.sh:**

```
validate.sh [--render-dir DIR] [--skip-render] [--skip-kubeconform] [--skip-policy]
  1. source tools.env; print versions
  2. render: kustomize build overlays/<o> > DIR/<o>.yaml       (o in: local staging)
  3. kubeconform -strict -summary -kubernetes-version $K8S_SCHEMA_VERSION -cache ... DIR/<o>.yaml
  4. yq ea -o=json '[.]' DIR/<o>.yaml > DIR/<o>.json ; check_policy.py --overlay <o> --input DIR/<o>.json
  5. AC-13 grep of numeric USER (prints FAIL AC-13 Dockerfile/<name>: ...)
  6. bash policy/test_policy.sh
```

**CI `validate-deploy` (the whole change, D1 + E6):** add `actions/setup-go@v5` (`go-version: ${{ env.GO_VERSION }}`,
module cache on) and replace the raw-directory step with:

```yaml
- name: render overlays (pinned kustomize)
  run: bash deploy/k8s/validate.sh --render-dir "$RUNNER_TEMP/k8s-render" --skip-kubeconform --skip-policy
- name: kubeconform -strict (rendered overlays)
  run: |
    set -euo pipefail
    . deploy/k8s/tools.env
    for o in local staging; do
      go run "github.com/yannh/kubeconform/cmd/kubeconform@${KUBECONFORM_VERSION}" -strict -summary \
        -kubernetes-version "$K8S_SCHEMA_VERSION" "$RUNNER_TEMP/k8s-render/$o.yaml"
    done
- name: k8s policy + self-test
  run: bash deploy/k8s/validate.sh --render-dir "$RUNNER_TEMP/k8s-render" --skip-render --skip-kubeconform
```

The `deploy` filter also gains `'deploy/docker/**'`. The run text of the kubeconform step still contains
`kubeconform` and `-strict`, so test-workflows.sh AC-5 keeps matching. The compose and promtool steps are unchanged.
Before pushing, run the `lint-ci` script tests locally with `bash .github/scripts/tests/test-workflows.sh`.

**WP table changes:**
- WP0 inputs are spec + erratum + `policy-contract.md`.
- WP3 also owns the `app-config.env` files, the `.gitattributes` line and the PII key check.
- WP5 owns `tools.env`.
- WP6 owns the CI filter line.
- Fast gate per WP:
  - `bash deploy/k8s/validate.sh` (TOOLS=go)
  - `docker compose config -q` **only if Docker is up**; otherwise it is recorded as not run and CI runs it
  - `bash .github/scripts/tests/test-workflows.sh`

**Mutation test:** unchanged, plus a fourth manifest mutant. Adding `MIN_SALE_LEAD_SECONDS` to the staging
`app-config.env` must make `validate.sh` fail with AC-32.

**Risks added:**
- The first `go run` needs network; modules are cached afterwards, and CI uses the setup-go cache.
- The kubeconform schema download from GitHub can be rate-limited. Use the `-cache` dir; a failed download is a hard error, never a skip.

## Round 3 (plan-review.md round 2: FAIL, 2 major + 13 minor; last allowed round)

### A. Majors

| Finding | Resolution |
|---|---|
| **R2-M1** staging fails open when a key is absent | Erratum E8.1-E8.3 (approved) and contract r2 §2.2/§8. AC-32 now requires keys to be **present**, not just "not demo": `GEMINI_MOCK="0"`, `PAYMENT_ENABLED_PROVIDERS=vnpay,momo`, a default provider from that list, `EMAIL_PROVIDER=smtp`, and for each enabled provider its callback URLs and secret keys. "Present" = env literal or a **non-optional** ref to an existing key. An optional ref whose key exists counts as set but **not present**. An optional ref to a missing key counts as absent; that is allowed only for forbidden keys. AC-18 forbids `optional: true` on every `secretKeyRef` in both overlays (stricter than E8's "staging", same intent: a missing Secret fails with `CreateContainerConfigError` instead of using public code defaults). Local gets dev values for every referenced key. Stripe is disabled in staging (human decision); the unsigned / `v1=valid_sig` Stripe webhook is recorded as security debt in spec §9 and the runbook |
| **R2-M2** whole-rule exemptions disable kept checks | Contract r2 §2.1/§3: AC-12 is split into 12 named sub-checks. Exemption keys are `eventflow.io/policy-exempt.AC-<n>[.<subcheck>]`. Allowed pairs: web `AC-12.readOnlyRootFilesystem`; ticketing-worker `AC-11`; dev-infra `AC-6`, `AC-10`, `AC-11`, `AC-20`, `AC-12.runAsNonRoot`. AC-9 is never exemptable: any `policy-exempt.AC-9*` gives `FAIL EXEMPT`, and an HPA on a fixed-replica workload fails AC-9. An invalid exemption has no effect (the rule is still checked). Section-7 exceptions exist only through annotations, nothing is hard-coded |

### B. Minors

| Finding | Resolution |
|---|---|
| r2-m1 AC-1 vs dev-infra | AC-1 counts only Deployments without the dev-infra label (contract §2). `good-local` gains a dev-infra Deployment (`eventflow-mailpit`) to pin this |
| r2-m2 dev-infra under drop ALL | Dev-infra runs as the image's own non-root uid, so `dropAll` holds and no root exemption is needed. AC-12 sub-checks for dev-infra are in contract §2.1; it may set `runAsUser`, and readOnlyRootFilesystem is not required. Planned uids: postgres:16-alpine 70, edoburu/pgbouncer 70, redis:7-alpine 999, mailpit 1000 (no root need, ports > 1024), rabbitmq:3.13-management-alpine = the image's `rabbitmq` uid. WP4 reads every uid from the image itself (`docker run --rm --entrypoint id <image> <user>`, or the image config) and records it in the component. All of them use `fsGroup` = that gid. Postgres: `PGDATA=/var/lib/postgresql/data/pgdata` on the PVC, plus `emptyDir` for `/var/run/postgresql` and `/tmp`. PgBouncer: `emptyDir` `/etc/pgbouncer`. If an image cannot start without root caps, WP4 swaps to an image that can and records it; adding a cap exemption is **not** allowed |
| r2-m3 hash suffixes | Contract §9: rules compare base names (rendered name minus a `-[a-z0-9]{10}` suffix). AC-22 requires refs to name the rendered object exactly. Good fixtures use suffixed names |
| r2-m4 kubeconform -cache dir | `validate.sh` runs `mkdir -p "$CACHE"` before kubeconform. CI uses `$RUNNER_TEMP/kubeconform-cache` |
| r2-m5 setup-go cache | `actions/setup-go@v5` with `cache-dependency-path: deploy/k8s/tools.env` |
| r2-m6 gates deletes Jobs on any context | C4.4 deletes `job/eventflow-migrate` only if `kubectl config current-context` matches `^(docker-desktop\|kind-.+)$` **and** the Job has `status.succeeded ≥ 1`. Otherwise it runs `kubectl apply --dry-run=server -k deploy/k8s/overlays/local -l 'app.kubernetes.io/name!=eventflow-migrate'` and prints that the migrate Job was skipped because Jobs are immutable. It never deletes on other contexts |
| r2-m7 validate.sh flags | `--render-only` = steps 1-2. `--skip-policy` skips steps 4-6 (policy, AC-13, self-test). CI step 1 uses `--render-only` |
| r2-m8 missing per-env settings | E8.6: `eventflow-app-config` adds `EMAIL_PROVIDER`, `SMTP_FROM`, `GOOGLE_CLIENT_ID`, `VNPAY_RETURN_URL`, `MOMO_RETURN_URL`, `MOMO_NOTIFY_URL` (staging: placeholder `https://staging.example.com/...` values the runbook says to replace), and in local only `STRIPE_SUCCESS_URL`, `STRIPE_CANCEL_URL`. `STRIPE_PUBLISHABLE_KEY` moves to a Secret. New Secret `eventflow-notification` (`SMTP_USER`, `SMTP_PASSWORD`, staging only); identity `SMTP_USER`/`SMTP_PASS` in staging only. `SMS_PROVIDER` stays `mock` (known limit). MoMo IPN reachability needs a public `MOMO_NOTIFY_URL`, so the runbook says staging needs a public host for it |
| r2-m9 AC-13 untested | Checker `--dockerfiles <dir>` mode (contract §4) plus fixtures `testdata/dockerfiles/{good,bad/AC-13--*}`. Mutation test: disabling AC-13 must fail the self-test. `migrate.Dockerfile` also ends with `USER 65532:65532` |
| r2-m10 APP_ENV/CORS location | Contract §9: `eventflow-app-config` only. `eventflow-endpoints` = infra hosts only |
| r2-m11 staging Redis password | Runbook known limit: staging Redis must be private and passwordless (no TLS/ACL in this task) |
| r2-m12 yq `null` entries | Contract §4: ignored. `good-local` ends with an empty document to pin it |
| r2-m13 duplicated dev DB password | `secrets.dev.env` comment: "`POSTGRES_PASSWORD` must equal the password inside both `eventflow-db` URLs". AC-30 proves it at runtime |
| r1-m13 header | Fixed: the header now states that everything is approved |

### C. WP0 fixture changes required by the approved contract change (TCR, approved via E8)

All files are generated by `deploy/k8s/policy/testdata/gen_fixtures.py` (committed e408ea3/03d1ff5). Change the
generator, then regenerate. `gen_fixtures.py --check` must be clean afterwards. Every `bad/*.yaml` changes bytes
because it is derived from the good fixtures. Only the expectations listed below change; all other bad fixtures
keep their rule id and intent.

**C1. Generator / good fixtures (`good-local.yaml`, `good-staging.yaml`)**

1. web annotation `policy-exempt.AC-12` → `policy-exempt.AC-12.readOnlyRootFilesystem`.
2. ticketing-worker: remove `policy-exempt.AC-9` (keep `AC-11`). waitingroom-controller: remove `policy-exempt.AC-9`.
3. Postgres (`PG_EXEMPT`): remove the `AC-9` and `AC-12` entries.
   - Add pod `securityContext {runAsNonRoot: true, runAsUser: 70, runAsGroup: 70, fsGroup: 70, seccompProfile: RuntimeDefault}`.
   - Add container `securityContext {allowPrivilegeEscalation: false, capabilities: {drop: [ALL]}}`.
4. No `optional` on any `secretKeyRef` (`SECRET(..., opt)`): payment `STRIPE_WEBHOOK_SECRET`, ai-worker `GEMINI_API_KEY`.
5. `env_for(p, overlay)`: per-overlay secret sets per contract §8.
   - Local payment gets all 8 provider keys, and `SECRETS["eventflow-payment"]` gets `dev` values for them.
   - Staging payment gets only the 5 vnpay/momo keys.
   - Staging identity gets `SMTP_USER`/`SMTP_PASS` from `eventflow-identity`.
   - Staging notification gets `SMTP_USER`/`SMTP_PASSWORD` from `eventflow-notification`.
6. `eventflow-app-config`, staging:
   - `PAYMENT_ENABLED_PROVIDERS` → `vnpay,momo`;
   - add `EMAIL_PROVIDER=smtp`, `SMTP_FROM`, `GOOGLE_CLIENT_ID`, `VNPAY_RETURN_URL`, `MOMO_RETURN_URL`, `MOMO_NOTIFY_URL`.
7. `eventflow-app-config`, local: add the same keys plus `STRIPE_SUCCESS_URL`, `STRIPE_CANCEL_URL`.
8. New env refs:
   - non-optional: payment `VNPAY_RETURN_URL`, `MOMO_RETURN_URL`, `MOMO_NOTIFY_URL`; notification `EMAIL_PROVIDER`,
     `SMTP_FROM`; identity `GOOGLE_CLIENT_ID`; gateway `CORS_ALLOWED_ORIGINS`;
   - optional: payment `STRIPE_SUCCESS_URL`, `STRIPE_CANCEL_URL`.
9. Hash-suffixed names (any fixed 10-char `[a-z0-9]` suffix):
   - local: all Secrets and both ConfigMaps;
   - staging: both ConfigMaps.

   Every ref follows the rename. Helpers `find(o, "Secret", …)`, `cfg()` and `CMAP`/`APPCFG` must resolve the suffixed
   names, so mutations such as `AC-16--local-value-without-dev`, `AC-22--configmap-key-missing` and
   `AC-17--configmap-*` still hit the intended object.
10. `good-local` adds a dev-infra Deployment `eventflow-mailpit`:
    - image `axllent/mailpit:v1.21`, `runAsUser: 1000`;
    - probes and resources;
    - exemptions `AC-6`, `AC-10`, `AC-11`, `AC-20`.
11. `good-local` ends with an empty document (`---` then nothing).

**C2. Bad fixtures whose expectation changes**

- Delete `bad/AC-18--stripe-key-not-optional.yaml`. Under the new contract a non-optional Stripe key in local is
  correct, so the mutation would be a no-op.
- `bad/AC-9--hpa-on-ticketing-worker.yaml`: unchanged expectation (AC-9). It is now consistent, because the worker no
  longer carries an AC-9 exemption.
- `bad/EXEMPT--unknown-pair-gateway-ac12.yaml`, `bad/EXEMPT--empty-reason-gateway-ac11.yaml`,
  `bad/EXEMPT--unknown-pair-worker-ac4.yaml`: unchanged expectation (whole-rule `AC-12` is now invalid everywhere,
  which still gives EXEMPT).

**C3. New bad fixtures (minimum set)**

| File | Overlay | Mutation | Expect |
|---|---|---|---|
| `AC-9--hpa-on-controller` | local | HPA targeting waitingroom-controller | AC-9 |
| `EXEMPT--worker-ac9` | local | worker gets `policy-exempt.AC-9: "x"` (no HPA) | EXEMPT |
| `EXEMPT--web-ac12-whole-rule` | local | web gets `policy-exempt.AC-12` **in addition to** the sub-check key | EXEMPT |
| `EXEMPT--web-unknown-subcheck` | local | web gets `policy-exempt.AC-12.runAsNonRoot` (runAsNonRoot stays true) | EXEMPT |
| `AC-12--web-exempt-runasnonroot-missing` | local | web pod `runAsNonRoot` removed, exemption kept | AC-12 |
| `AC-12--web-exemption-missing` | local | web sub-check annotation removed | AC-12 |
| `AC-12--dev-infra-drop-all-missing` | local | postgres container `capabilities` removed | AC-12 |
| `AC-12--dev-infra-runasnonroot-false` | local | postgres pod `runAsNonRoot: false` (no exemption) | AC-12 |
| `AC-18--local-provider-key-optional` | local | payment `VNPAY_HASH_SECRET` ref `optional: true` | AC-18 |
| `AC-18--staging-optional-secret-ref` | staging | ai-worker `GEMINI_API_KEY` ref `optional: true` | AC-18 |
| `AC-18--staging-stripe-key-extra` | staging | payment gets `STRIPE_SECRET_KEY ← eventflow-payment/STRIPE_SECRET_KEY` | AC-18 |
| `AC-18--staging-identity-smtp-missing` | staging | identity `SMTP_PASS` ref removed | AC-18 |
| `AC-22--ref-uses-base-name-local` | local | event `RABBITMQ_URL` ref names `eventflow-mq` instead of the suffixed name | AC-22 |
| `AC-32--gemini-mock-absent` | staging | ai-worker `GEMINI_MOCK` ref made optional and the key removed from app-config | AC-32 |
| `AC-32--gemini-mock-optional-ref` | staging | ai-worker `GEMINI_MOCK` ref made optional, key `"0"` kept | AC-32 |
| `AC-32--enabled-providers-absent` | staging | payment `PAYMENT_ENABLED_PROVIDERS` ref optional and key removed | AC-32 |
| `AC-32--stripe-enabled` | staging | `PAYMENT_ENABLED_PROVIDERS=vnpay,momo,stripe` | AC-32 |
| `AC-32--default-provider-not-enabled` | staging | `PAYMENT_DEFAULT_PROVIDER=stripe` | AC-32 |
| `AC-32--momo-secret-missing` | staging | payment `MOMO_SECRET_KEY` env removed | AC-32 |
| `AC-32--momo-notify-url-absent` | staging | payment `MOMO_NOTIFY_URL` ref optional and key removed | AC-32 |
| `AC-32--email-provider-mock` | staging | `EMAIL_PROVIDER=mock` | AC-32 |
| `AC-32--stripe-url-in-configmap` | staging | `STRIPE_SUCCESS_URL` added to staging app-config (payment holds an optional ref to it) | AC-32 |
| `dockerfiles/bad/AC-13--named-user/` | — | final stage `USER nonroot:nonroot` | AC-13 |
| `dockerfiles/bad/AC-13--no-user/` | — | final stage without `USER` | AC-13 |
| `dockerfiles/good/` | — | tails of the 6 real Dockerfiles (numeric USER) | pass |

**C4. `test_policy.sh`**: add the `--dockerfiles` runs. Run the good dir once; run each `bad/AC-13--*` dir and
expect exit 1 plus only `FAIL AC-13` lines.

### D. Other plan updates

- Mutation test:
  - Add per-sub-check mutants for AC-12 (disable `readOnlyRootFilesystem` scoping, so web is fully exempt → must fail
    `AC-12--web-exempt-runasnonroot-missing`).
  - Add an AC-9 exemptability mutant (accept `policy-exempt.AC-9` → must fail `EXEMPT--worker-ac9`).
  - Add AC-13.
- Fast gate unchanged.
- WP4 also owns the uid table (r2-m2).
- WP6 owns the gates context guard (r2-m6) and the setup-go cache path (r2-m5).
- Runbook additions:
  - the Stripe debt;
  - Redis passwordless (r2-m11);
  - SMS mock;
  - the MoMo public notify URL;
  - minimum Kubernetes 1.30;
  - the `delete -k` warning.
