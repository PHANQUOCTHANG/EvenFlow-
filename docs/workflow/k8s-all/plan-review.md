# Plan review: k8s-all (v3 step 2b, High-risk, round 1)

Reviewer: clean-context plan reviewer. Inputs: `spec.md` (human-approved, D1-D6 as recommended), `plan.md`,
`tier.json`, plus read-only checks against the worktree @ c5de208 (base c5fb598). No files changed except this one.

## Verdict: **FAIL** (3 major, 0 blocker)

The layout, selector handling, CI/test-workflows compatibility, probe/port mapping and NetworkPolicy matrix are
sound. The plan fails on three points that would make parallel work drift or ship a wrong staging config:
the policy-checker interface is not frozen for the parallel test designer (M1), offline validation cannot run on
the dev machine as planned (M2), and demo-only app settings would leak into the "prod-like" staging overlay (M3).
All three have cheap fixes. Round 2 needs only those fixes; the minors can be fixed during implementation.

## What was verified and is OK

| Check | Result | Evidence |
|---|---|---|
| Web selectors immutable after move | OK. Plan forbids `commonLabels`/selector transformers, uses `git mv`, keeps names; current selector = exactly `app.kubernetes.io/name` + `part-of` | `deploy/k8s/10-deployment.yaml:31-42`, plan.md:13,48 |
| Web HPA needs a change for AC-9 | Expected: current web HPA has no `behavior.scaleDown`; adding it does not touch selector | `deploy/k8s/40-hpa.yaml:21-38` |
| kubeconform vs kustomize files | Correct analysis: raw-dir scan (`/m`) would read `kustomization.yaml`/component and fail; plan renders first and pipes to `kubeconform -` | `.github/workflows/ci.yml:313-318`, plan.md:69-82 |
| `test-workflows.sh` AC-5 keeps passing | OK. `ac5_k8s` only glob-matches the path `deploy/k8s/10-deployment.yaml` against filter `deploy/k8s/**` (file need not exist) and greps the step `run` for `kubeconform` and `-strict`; plan keeps both literally in the run text | `.github/scripts/tests/test-workflows.sh:604-619,629` |
| No other test/workflow depends on flat files | Only `gates.sh` C4.4 (`00-namespace.yaml`, glob) and ci.yml | grep over `.github`, apps, services |
| Probes vs real endpoints | Matches spec 4.1: `httpx.Health` (healthz no deps, readyz checks) in gateway (no checks), waitingroom (Redis), ticketing (pgx pool + Redis); identity/event/payment/notification only `/healthz`; ai-worker `/healthz` + `/readyz`; controller/worker serve only `/metrics` via `otelx.ServeMetrics` on 9091/9092; gateway metrics 9094 | `libs/go/httpx/health.go:20-35`, `services/ticketing/cmd/server/main.go:148-153`, `services/waitingroom/cmd/server/main.go:89-92`, `libs/go/otelx/metrics.go:23-29`, `services/gateway/cmd/server/main.go:59-66`, `services/ai-worker/app/main.py:145-165` |
| Ports | 8080/8081/8082/8083/8084/8085/8087/8090, metrics 9091/9092/9094 agree with code defaults and compose | compose; `env("METRICS_ADDR", …)` lines above |
| Singletons | waitingroom-controller is lease + fencing per event (2 replicas safe); ticketing-worker runs sweeper + seeder + reconciler + relay + order.paid consumer + discovery in one process, reconciler concurrency unproven → `replicas: 1` + `Recreate` is the right call | `services/waitingroom/cmd/controller/main.go:1-21`, `services/ticketing/cmd/worker/main.go:1-10,195-240` |
| NetworkPolicy matrix vs real call paths | Complete. Only inter-service HTTP: gateway → 6 upstreams (`IDENTITY_URL`…`ASSISTANT_URL`), ai-worker → gateway (`internal_api_base`) and → event (`moderation_api_base`). Payment webhooks enter via gateway route `/v1/payments/webhook/`. Ingress-only policies, so pod → Postgres/Redis/RabbitMQ/DNS egress is unaffected; dev-infra allows namespace `eventflow` (covers migrate Job) | `services/gateway/internal/config/config.go:42-49`, `services/ai-worker/app/config.py:59-67`, `services/gateway/internal/proxy/routes.go:65`; grep found no other `http://<svc>:<port>` |
| readOnlyRootFilesystem | No service writes files (`os.Create/WriteFile` only in `libs/go/testx`); ai-worker has `PYTHONDONTWRITEBYTECODE`; `/tmp` emptyDir is enough | grep, `deploy/docker/ai-worker.Dockerfile:3` |
| Numeric USER | Same uids: distroless `nonroot` = 65532:65532; ai-worker `useradd -u 10001`. `migrations/` is not in `.dockerignore`, so `migrate.Dockerfile` can COPY it | `deploy/docker/*.Dockerfile`, `.dockerignore` |
| AC coverage | Every AC-1…AC-31 is assigned to a WP (AC-28 implicitly via WP6 full gate) | plan.md:96-104 |
| Rollback | Revert + delete-before-revert + `rollout undo` + NetworkPolicy escape hatch are present and correct | plan.md:141-150 |

## Findings

### M1 (major): the checker interface is not frozen for the parallel test designer

WP0 (test design) runs in parallel with WP5 and "has this spec only" (spec.md:296, plan.md:98). The fixtures and
`test_policy.sh` it writes need an interface that exists only in plan.md, and partly nowhere:
- **Rule IDs**: AC-25 requires "reject with that rule id" (spec.md:186-188), but the spec never defines rule IDs.
  The plan defers them to `rules.md`, which WP5 writes later (plan.md:36).
- **CLI and I/O**: `check_policy.py --overlay <name> <json>`, input = JSON list converted by yq, output
  `FAIL <RULE-ID> <kind>/<name>: <why>` (plan.md:60-62). The test designer cannot see this. Overlay-dependent rules
  (AC-14, AC-16, AC-19) also need a defined way for fixtures to choose an overlay.
- **Exemption format**: the spec allows one annotation value `"<RULE-ID>: <reason>"` (spec.md:70-71), but
  ticketing-worker needs two exemptions (AC-9 and AC-11, spec.md:245). Dev-infra exemptions are "listed by name in
  the component" (spec.md:247) with no format given.

Impact: WP0 and WP5 will build incompatible fixtures and checker. Fixing that later needs a test-change-request on
frozen tests, or tempts the implementer to fit the checker to whatever the fixtures happen to say.
**Fix:** add a "Policy interface contract" section, frozen and handed to WP0 together with the spec. It must
define: rule ID = `AC-<n>` (or an explicit list), CLI signature and exit codes, input format and who converts YAML
to JSON (`test_policy.sh` uses the same pinned yq), the output line regex, how a fixture selects its overlay, and a
multi-rule exemption format (for example one key per rule, `eventflow.io/policy-exempt-AC-11: "<reason>"`, or a
`;`-separated list). If this changes spec text, record it as a spec erratum with human approval.

### M2 (major): offline validation cannot run on the dev machine as planned

The plan calls AC-1…AC-29 "offline" and treats Docker as needed only for runtime evidence (plan.md:136, spec AC-24).
But every validation step except rendering needs Docker:
- kubeconform runs as a docker image (plan.md:58-59).
- YAML → JSON needs the yq image (plan.md:60). The plan itself says there is no python fallback (plan.md:135).
- Verified now: the Docker daemon is not running (`open //./pipe/dockerDesktopLinuxEngine: The system cannot find
  the file specified`). There is no `yq` and no `kubeconform` on PATH. `kubectl create --dry-run=client
  --validate=false -o json` cannot convert offline either: it fails with "unable to recognize … /api" because it
  needs an API server.

So `validate.sh`, `test_policy.sh`, the WP0 RED check, the mutation test (plan.md:152-157) and the per-WP fast gate
(plan.md:106-107) cannot run locally. That breaks the High-risk consequences in `tier.json`. Docker `-v "$PWD/…"` on
Git Bash also needs `MSYS_NO_PATHCONV=1` (or `pwd -W`), which the plan does not mention, although AC-24 requires
Git Bash support.
**Fix:** pick one and write it into the plan:
- (a) Make "Docker Desktop running" an explicit precondition of WP0/WP5/WP6, with a stop rule if it is not.
- (b) Add a pinned non-docker fallback at the same versions. Go is installed, so for example
  `go run github.com/mikefarah/yq/v4@v4.<x.y>` and `go run github.com/yannh/kubeconform/cmd/kubeconform@v0.8.0`.

Either way, handle MSYS path conversion in `validate.sh`.

### M3 (major): demo-only app settings leak into the prod-like staging overlay

Spec 4.1 says per-process env = the compose `environment:` block (spec.md:92-95). Only the fixed key list of
`eventflow-endpoints` varies per overlay (spec.md:93-95, plan.md:29,33). The plan has no mechanism for other
per-environment values, so they land in `base/` and therefore in staging (which has `APP_ENV=production`):
- `MIN_SALE_LEAD_SECONDS=60`. Compose says "Production KHONG dat bien nay" (`deploy/compose/docker-compose.yml:292-294`).
- `PAYMENT_SANDBOX_MODE=true`, `PAYMENT_DEFAULT_PROVIDER=sandbox` (compose:309-310).
- `GEMINI_MOCK=1`. Spec 6.1 itself says "`GEMINI_MOCK=1` in local" (spec.md:223), which implies staging differs.
  Note that the ai-worker settings validator (`config.py` `_gemini_that_can_key`) rejects mock=0 without a key.
- Demo `ACTIVE_EVENT_IDS` UUIDs (compose:165,214) and the "tran thap cho local" `ADMIT_*` ceilings (compose:166-172).

**Fix:** add a second per-overlay ConfigMap (for example `eventflow-app-config`, via configMapGenerator in each
overlay, referenced with `configMapKeyRef`/`envFrom` from base), or use overlay patches. Then:
- List the staging values explicitly: omit `MIN_SALE_LEAD_SECONDS` and `ACTIVE_EVENT_IDS`; make an explicit sandbox
  decision for payment; set `GEMINI_MOCK` explicitly.
- Add a policy rule (with a fixture) that the staging render contains no `MIN_SALE_LEAD_SECONDS` and no
  `GEMINI_MOCK=1`.
- Record this in spec 4.1 as an erratum.

### Minor

| # | Finding | Evidence | Fix |
|---|---|---|---|
| m1 | Secret contract 6.1 contradicts AC-18's own "no process mounts a Secret it does not use". gateway and identity never read `QUEUE_TOKEN_SECRET`; only ticketing and waitingroom call `authx.QueueTokenSecretFromEnv`. The ticketing **server** reads neither `AMQP_URL` nor `RABBITMQ_URL` (only the worker does), so compose's `AMQP_URL` for ticketing is dead config. F10 is wrong for gateway, which requires only `JWT_SECRET` in production. Gateway is the most exposed pod. | `services/ticketing/cmd/server/main.go:123`, `services/waitingroom/cmd/server/main.go:74`, `services/gateway/internal/config/config.go:66-79`, `services/identity/internal/config/config.go:53-68`, `services/ticketing/cmd/worker/main.go:96` | Spec erratum (human approval, **before WP0 freezes fixtures**): gateway {JWT_SECRET}; identity {JWT_SECRET} + `eventflow-identity`; ticketing gets no `eventflow-mq`. |
| m2 | Tool pinning is inconsistent. `validate.sh` prefers the local kubectl (plan.md:56-57). In CI the policy step (`--skip-kubeconform`) would render with the runner's preinstalled, unpinned kubectl, while the kubeconform step uses `$KUBECTL_IMAGE`. So the two checks run on renders from different kustomize versions. Locally it is kubectl 1.34.1 / kustomize v5.7.1 vs a v1.32.x image. | plan.md:56-57,73-81 | Render once with the pinned image in CI and pass the files (`validate.sh --render-dir`), or force the docker renderer in CI. Print the kustomize version in the log. |
| m3 | migrate Job `spec.template` is immutable. Re-applying with a new image tag or a regenerated secret hash fails with `field is immutable`. The same happens to the gates C4.4 dry-run on a cluster that already has the Job. The env mapping is also unspecified: goose reads `GOOSE_DRIVER`/`GOOSE_DBSTRING`. | plan.md:25,102; compose:131-136 | `ttlSecondsAfterFinished` plus a runbook step `kubectl -n eventflow delete job eventflow-migrate --ignore-not-found` before apply. Map `DATABASE_URL_DIRECT` → `GOOSE_DBSTRING` with `GOOSE_DRIVER=postgres`. |
| m4 | AC-13 (numeric USER) is only checked when `deploy/k8s/**` changes. The `deploy` filter does not include `deploy/docker/**`, so a later PR that restores `USER nonroot:nonroot` passes CI and breaks every pod. | `.github/workflows/ci.yml:76-79`, plan.md:84-85 | Add `'deploy/docker/**'` to the `deploy` filter (needs human OK, since D1 said "filter unchanged"), or record the gap in the runbook. |
| m5 | Dev PII keys are unspecified. identity needs base64 of 32 bytes (`pii.NewProtector`). The code's dev defaults do not contain "dev" and compose has no PII values, yet AC-16 requires "every value contains dev". | `services/identity/internal/config/config.go:12-13`, `services/identity/cmd/server/main.go:43`, plan.md:124 | Generate 44-char base64 strings that contain `dev` and decode to exactly 32 bytes. Note this in `secrets.dev.env` and prove it with identity startup (AC-30). |
| m6 | ai-worker app grace is non-numeric in 4.1 ("uvicorn"), but the app has `shutdown_grace_seconds=20`. The plan's 30 s is exactly 5+20+5, with no margin and no number the policy can use. | `services/ai-worker/app/config.py:56`, spec.md:90, plan.md:113 | Freeze ai-worker app grace = 20 in the rules and use 35 s, like the other 20-s services. |
| m7 | The `preStop.sleep` action needs a cluster ≥ 1.30 (beta, on by default; GA 1.34). The plan says "core API since 1.29" (alpha, gate off), and kubeconform 1.32 will not catch a too-old staging cluster. | plan.md:112 | State the minimum Kubernetes version in the runbook and the staging assumptions. |
| m8 | gates.sh C4.4 "filter `kind: Namespace`" has no tool: gates deps are bash/git/node/npm, with no yq. | `.github/scripts/gates.sh:21,255-262`, plan.md:88-90 | `kubectl get ns eventflow >/dev/null 2>&1 \|\| kubectl create namespace eventflow`, then `kubectl apply --dry-run=server -k …`. Keep the no-cluster branch verbatim. |
| m9 | Dev-infra sizing and kind are under-specified. RabbitMQ must be a StatefulSet (node name comes from the hostname; PVC data is tied to it). Redis `limits.memory` must exceed `maxmemory 256mb`. Postgres compose flags set `shared_buffers=512MB`, so either size the limit for it or lower it. `rabbitmq-diagnostics check_port_connectivity` needs `timeoutSeconds ≥ 10`. | plan.md:24,102; compose:14-19,54-58,79-84 | Fix these in WP4 and list the values in the component. |
| m10 | Secret-scan allow entry scope. The full-gate regex (`(secret\|password\|api_key)…[:=]…[A-Za-z0-9/+]{16,}`) most likely does not even match the hyphenated dev values, while a path exclusion makes the file a permanent blind spot. | `docs/workflow/frontend-debt-r4/full-gate.sh:65-68`, plan.md:127 | Run the scan first. Add an exclusion only if it actually hits, and then only the exact pathspec `:(exclude)deploy/k8s/overlays/local/secrets.dev.env` (never `*.env`). The policy's AC-16 "dev" check is the compensating control. |
| m11 | Rollback: `kubectl delete -k overlays/local` also deletes the Namespace, because it is part of the render. That takes web and the dev PVC data with it. | plan.md:146-149 | Say so explicitly in the runbook, or delete with `-l` / by kind to keep the namespace. |
| m12 | Placeholder tag `sha-c5fb598` is not a commit any image is built from. The web slice rule says a tag must match its build commit. | plan.md:121-123, `deploy/k8s/10-deployment.yaml:79-96` | Mark it as a placeholder in a comment. The runbook already sets the real tag via a temp overlay. |
| m13 | Plan header still says D1-D6 are "pending human approval", but the spec records approval. | plan.md:3,5 vs spec.md:300 | Update the header. |

## Required for round 2

M1, M2 and M3 fixed in plan.md. If spec text changes (M1 exemption format, M3 4.1 env split, m1 secret contract),
record it as a spec erratum signed by the human before WP0 freezes fixtures.
