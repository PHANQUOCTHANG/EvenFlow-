# Plan: k8s-all (round 1, before plan review)

Inputs: `spec.md` (AC-1..AC-31, decisions D1-D6 pending human approval), base develop @ c5fb598.
No migrations, no service code changes, no new runtime dependencies. New CI tool images only (D1).
Plan review is mandatory (High-risk); this plan assumes D1-D6 are approved as recommended — section 8 says what
changes if they are not.

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
