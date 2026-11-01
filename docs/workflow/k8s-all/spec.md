# Spec: Kubernetes manifests for every service (k8s-all)

Tier: **High-risk** (`tier.json`). Branch `feature/k8s-all` from develop @ c5fb598. Jira EV-190 (EVF-120)
"K8s manifest mọi service: probe, resource, PDB, anti-affinity"; EV-191 (EVF-121) only partly (section 8, D3).
Process: v3. Earlier web-only slice: `docs/workflow/deploy-k8s-web/` (its decisions 3.1-3.5 stay valid and are
not repeated here). **Human must approve this spec and decisions D1-D6 (section 8) before implementation.**

## 1. Goal

Every deployable process of EventFlow has a Kubernetes manifest with separate liveness/readiness probes,
requests+limits, a PDB, soft anti-affinity + zone spread, a non-root security context, HPA where stateless, and
secrets that never sit in a plain manifest; and CI proves all of that **offline, without a cluster**, on every PR
that touches `deploy/k8s/**`.

## 2. Facts found (read-only research, base c5fb598)

| # | Fact | Source |
|---|---|---|
| F1 | `deploy/k8s/` has web only (00-namespace … 40-hpa). CI `validate-deploy` runs `kubeconform -strict` on the **raw directory** `deploy/k8s` | `.github/workflows/ci.yml:305-318` |
| F2 | `.github/scripts/tests/test-workflows.sh:619,629` asserts a job gated by `deploy/k8s/**` runs a step whose `run` text contains `kubeconform` and `-strict` | test-workflows.sh |
| F3 | `gates.sh` C4.4 runs `kubectl apply --dry-run=server -f` on every yaml under `deploy/k8s` and pre-creates `00-namespace.yaml`, only when `kubectl cluster-info` succeeds | `.github/scripts/gates.sh:225-260` |
| F4 | Go images end with `USER nonroot:nonroot`, ai-worker with `USER worker`: **non-numeric**. With `runAsNonRoot: true` kubelet rejects them (`CreateContainerConfigError: image has non-numeric user`), the same failure the web slice hit with `USER node` | `deploy/docker/{go-service,ticketing,waitingroom,ai-worker}.Dockerfile` |
| F5 | `/healthz` + `/readyz` (readyz checks deps, 503 on failure): gateway (no checks), waitingroom (Redis ping), ticketing, ai-worker. **Only `/healthz`**: identity, event, payment, notification. **No HTTP endpoint except `/metrics`**: waitingroom-controller (:9091), ticketing-worker (:9092) | `libs/go/httpx/health.go`, `services/*/cmd/*/main.go`, `services/ai-worker/app/main.py:145-165` |
| F6 | Metrics: on the API port for identity/event/waitingroom/ticketing/payment/notification/ai-worker; separate port gateway :9094, controller :9091, worker :9092 | `deploy/observability/prometheus.yml` |
| F7 | Graceful shutdown on SIGTERM: gateway/ticketing/waitingroom 20 s (`httpx.Run`), payment/notification 10 s, event 5 s, controller/worker via ctx. **identity has none** (`http.ListenAndServe`, no signal handling) | `services/*/cmd/server/main.go` |
| F8 | Outbox relays (event, payment, ticketing) use `FOR UPDATE SKIP LOCKED` (multi-replica safe); admit controller uses Redis leases + fencing (multi-replica safe, EV-142); ai-worker Gemini limiter is global in Redis. ticketing-worker's inventory reconciler is **not proven** safe with two instances | `services/*/internal/worker/outbox_relay.go`, `services/waitingroom/cmd/controller/main.go:7-9`, `services/ai-worker/app/gemini/limiter.py:10` |
| F9 | Compose binds service ports to loopback because a direct caller can forge `X-Identity-Id`; only gateway is public. In-cluster, a ClusterIP Service is reachable from every pod unless a NetworkPolicy says otherwise | `deploy/compose/docker-compose.yml` comments |
| F10 | `APP_ENV=production` makes gateway/identity refuse to start without `JWT_SECRET`, `QUEUE_TOKEN_SECRET`, `PII_ENC_KEY`, `PII_HASH_KEY` | `libs/go/authx/env.go`, `services/{gateway,identity}/internal/config/config.go` |
| F11 | identity and event must connect to Postgres **directly** (lib/pq + PgBouncer transaction mode breaks prepared statements); ticketing, ticketing-worker, payment, notification go through PgBouncer | compose comments |
| F12 | No service uses MinIO; RabbitMQ topology is declared by code (`libs/go/mq/topology.go`, ai-worker reads its baked definitions file); compose mounts `definitions.json` but never loads it | grep, EVF-7 plan-review T7 |
| F13 | Web reads only `NEXT_PUBLIC_API_BASE` (baked at build). There is no server-side/internal API base, so SSR fetches from a web pod use the public host | `apps/web/src/lib/api-base.ts` |
| F14 | Local tooling: kubectl 1.34.1 with built-in kustomize v5.7.1; no kind, no yq; Docker daemon not running at spec time | checked 2026-11-01 |

## 3. Scope

**In**
1. Kustomize layout: `deploy/k8s/base` (all app workloads, namespace, NetworkPolicies), `components/dev-infra`,
   `overlays/local` (Docker Desktop / kind, dev infra in-cluster), `overlays/staging` (prod-like, infra external).
2. Workloads: web (moved, not redesigned), gateway, identity, event, waitingroom, waitingroom-controller, ticketing,
   ticketing-worker, payment, notification, ai-worker; plus a migrate Job (local overlay only).
3. ConfigMap/Secret wiring and the Secret contract (section 6).
4. Numeric `USER` in the four non-web Dockerfiles; new `deploy/docker/migrate.Dockerfile` (goose + migrations).
5. Offline validation: render every overlay, `kubeconform -strict`, a policy checker with its own negative fixtures,
   wired into CI `validate-deploy` and into `gates.sh` C4.4 (protected paths, D1).
6. Runbook `docs/workflow/k8s-all/runbook.md` (build, load, apply, verify, rollback, known limits).

**Out** (with reason)
| Item | Reason |
|---|---|
| HPA on custom metrics (rps, queue_depth), prometheus-adapter (rest of EV-191) | needs Prometheus + adapter in cluster; nothing to validate offline; D3 |
| Production HA data stores (Postgres HA, Redis Cluster, RabbitMQ quorum) | belong to operators / managed services; hand-written HA StatefulSets would be unverifiable here. Staging overlay treats them as external endpoints |
| External Secrets Operator / sealed secrets objects | need CRDs; kubeconform has no schema offline. Secret contract is written so ESO can fill it later; D4 |
| antibot service | stub with no compose entry, not routed by gateway |
| Observability stack in cluster (Jaeger, Prometheus, Grafana), ServiceMonitor CRDs | out of EV-190; pods get scrape annotations and `OTEL_EXPORTER_OTLP_ENDPOINT` comes from config (empty = traces not exported, `libs/go/otelx/otelx.go:4-6`) |
| Adding `/readyz` to identity/event/payment/notification, graceful shutdown in identity, internal API base for web SSR | service code changes; recorded as debt (section 9) |
| CD publishing of non-web images, deploy step in CD | touches CD + registry; separate decision |
| Load test, chaos drill, pre-warm CronJob (EVF-122..126) | later phase-9 items |

## 4. Definitions

- **App workload**: a Deployment in `base/` for one of the 11 processes above. **Dev-infra workload**: anything in
  `components/dev-infra` (label `eventflow.io/tier: dev-infra`).
- **Request-serving**: web, gateway, identity, event, waitingroom, ticketing, payment, ai-worker (have a Service that
  receives traffic). **Background**: waitingroom-controller, ticketing-worker, notification (consumer; its HTTP port
  is health/metrics only).
- **HPA-managed**: web, gateway, identity, event, waitingroom, ticketing, payment, notification, ai-worker.
  **Fixed replicas**: waitingroom-controller = 2 (lease-based, F8), ticketing-worker = 1 (F8, D5).
- **Rendered overlay**: output of `kubectl kustomize deploy/k8s/overlays/<name>`; every AC is checked on the
  rendered output of **both** overlays unless it names one.
- **Exemption**: annotation `eventflow.io/policy-exempt: "<RULE-ID>: <reason>"` on the workload. Only the
  exemptions listed in section 7 are allowed; any other exemption fails the policy.
- Names: every resource of a process is `eventflow-<process>` (e.g. `eventflow-ticketing-worker`); labels
  `app.kubernetes.io/name: eventflow-<process>` + `app.kubernetes.io/part-of: eventflow` (selector = exactly these
  two, as web today); namespace `eventflow`.

### 4.1 Workload matrix (contract for the implementer and the test designer)

| Process | Image (`ghcr.io/phanquocthang/…`) | Cmd | Port(s) | Liveness | Readiness | Startup | App grace | HPA min/max | requests cpu/mem | limits cpu/mem |
|---|---|---|---|---|---|---|---|---|---|---|
| web | eventflow-web | image default | http 3000 | /api/healthz | /api/readyz | /api/healthz | 5 s drain | 2/6 | 100m/256Mi | 500m/512Mi |
| gateway | eventflow-gateway | default | http 8080, metrics 9094 | /healthz | /readyz | /healthz | 20 s | 2/10 | 200m/128Mi | 1000m/256Mi |
| identity | eventflow-identity | default | http 8083 | /healthz | /healthz (F5, debt) | /healthz | none (F7) | 2/6 | 100m/128Mi | 1000m/256Mi |
| event | eventflow-event | default | http 8084 | /healthz | /healthz (debt) | /healthz | 5 s | 2/6 | 100m/128Mi | 500m/256Mi |
| waitingroom | eventflow-waitingroom | default | http 8081 | /healthz | /readyz | /healthz | 20 s | 2/10 | 200m/128Mi | 1000m/256Mi |
| waitingroom-controller | eventflow-waitingroom | `/controller` | metrics 9091 | /metrics | /metrics | /metrics | ctx | fixed 2 | 50m/64Mi | 250m/128Mi |
| ticketing | eventflow-ticketing | default | http 8082 | /healthz | /readyz | /healthz | 20 s | 2/10 | 200m/256Mi | 1000m/512Mi |
| ticketing-worker | eventflow-ticketing | `/worker` | metrics 9092 | /metrics | /metrics | /metrics | ctx | fixed 1 | 100m/128Mi | 500m/256Mi |
| payment | eventflow-payment | default | http 8085 | /healthz | /healthz (debt) | /healthz | 10 s | 2/6 | 100m/128Mi | 500m/256Mi |
| notification | eventflow-notification | default | http 8087 | /healthz | /healthz (debt) | /healthz | 10 s | 2/4 | 50m/64Mi | 250m/128Mi |
| ai-worker | eventflow-ai-worker | default | http 8090 | /healthz | /readyz | /healthz | uvicorn | 2/6 | 200m/256Mi | 1000m/512Mi |

Env values per process = the compose `environment:` block, with hostnames replaced by Service names
(`http://eventflow-identity:8083`, …) or by keys of the overlay ConfigMap `eventflow-endpoints`
(`REDIS_ADDR`, `REDIS_URL`, `SMTP_HOST`, `SMTP_PORT`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `CORS_ALLOWED_ORIGINS`,
`APP_ENV`), and every secret-bearing value taken from the Secret contract (section 6).

## 5. Acceptance criteria

All AC except AC-30/AC-31 are checked **offline** by `bash deploy/k8s/validate.sh` (render + kubeconform + policy +
policy self-test), which CI runs (AC-24).

### A. Layout and rendering
- **AC-1** Given the repo, When `kubectl kustomize deploy/k8s/overlays/local` and `…/overlays/staging` run, Then both
  exit 0 and each output contains exactly one Deployment per process of 4.1 (11), one Namespace `eventflow`, and no
  object outside namespace `eventflow` except the Namespace itself.
- **AC-2** Given the rendered overlays, When `kubeconform -strict -kubernetes-version 1.32.0` reads them, Then 0
  invalid, 0 errors, 0 skipped kinds (no CRDs are used).
- **AC-3** Given the old web manifests, When the web resources are rendered, Then names
  (`eventflow-web` Deployment/Service/Ingress/HPA), Deployment `spec.selector`, container name `web`, port name `http`,
  probe paths and the `maxUnavailable: 0`/`maxSurge: 1` strategy are unchanged (selector is immutable on a live
  cluster), and `deploy/k8s/*.yaml` at the top level no longer exist. No `commonLabels`/selector-touching
  transformer is used.

### B. Probes and lifecycle
- **AC-4** Every container of every Deployment/StatefulSet has `livenessProbe` and `readinessProbe`; every app
  container also has `startupProbe`. Paths and ports are exactly as in 4.1 (port referenced by name).
- **AC-5** For processes that have `/readyz` (web, gateway, waitingroom, ticketing, ai-worker), readiness path ≠
  liveness path; no liveness probe anywhere points at a readiness path (`/readyz`, `/api/readyz`).
- **AC-6** Every request-serving Deployment has `strategy.rollingUpdate.maxUnavailable: 0`, `maxSurge: 1`, a
  `preStop` hook (`sleep: {seconds: 5}` for distroless/python images; web keeps its `node -e` hook) and
  `terminationGracePeriodSeconds ≥ 5 + app grace + 5` (4.1; identity = 30 by default rule).
- **AC-7** ticketing-worker has `replicas: 1` and `strategy.type: Recreate`; waitingroom-controller has `replicas: 2`.
  HPA-managed Deployments declare **no** `spec.replicas`.

### C. Resources, scaling, availability
- **AC-8** Every container (incl. init containers and dev-infra) has `requests` and `limits` for cpu and memory;
  app values equal 4.1.
- **AC-9** Each HPA-managed Deployment has exactly one `autoscaling/v2` HPA whose `scaleTargetRef` names it,
  `minReplicas ≥ 2`, `maxReplicas` per 4.1, CPU `averageUtilization` 60-75, and
  `behavior.scaleDown.stabilizationWindowSeconds ≥ 300` (no flapping); no HPA targets the fixed-replica workloads.
- **AC-10** Every app Deployment is selected by exactly one PDB (`policy/v1`) with `maxUnavailable: 1` and a selector
  equal to its pod labels (no `minAvailable`, which would block node drains for the 1-replica worker).
- **AC-11** Every app Deployment except ticketing-worker has `podAntiAffinity.preferredDuringScheduling…` on
  `kubernetes.io/hostname` and a `topologySpreadConstraints` entry on `topology.kubernetes.io/zone` with
  `whenUnsatisfiable: ScheduleAnyway`, both selecting its own pod labels (soft, so one-node Docker Desktop still
  schedules; staging may tighten later).

### D. Security context and permissions
- **AC-12** Every app pod: `securityContext.runAsNonRoot: true`, `seccompProfile.type: RuntimeDefault`,
  `automountServiceAccountToken: false`; every app container: `allowPrivilegeEscalation: false`,
  `capabilities.drop: [ALL]`, no `privileged`, `readOnlyRootFilesystem: true` (web exempt, section 7; ai-worker gets
  an `emptyDir` at `/tmp`). No pod uses `hostNetwork`, `hostPID`, `hostIPC`, `hostPath`, or `runAsUser: 0`.
  App manifests do **not** set `runAsUser` (web slice decision 3.5: a regression in the image must fail loudly).
- **AC-13** `deploy/docker/{go-service,ticketing,waitingroom}.Dockerfile` end with `USER 65532:65532` and
  `ai-worker.Dockerfile` with `USER 10001` (numeric, same uids as today, F4). Checked by grep in the policy self-test
  and by `docker build` + `docker inspect -f '{{.Config.User}}'` when Docker is available.
- **AC-14** Namespace labels: staging `pod-security.kubernetes.io/enforce: restricted`; local
  `enforce: baseline`, `warn: restricted`, `audit: restricted` (dev-infra images may need root init, section 7).
- **AC-15** NetworkPolicy: a default-deny-ingress policy selects all pods; every app pod is selected by at least one
  allow policy; ingress is allowed only per the matrix in section 6.2; no allow rule for identity, event,
  waitingroom, ticketing, payment, ai-worker has an empty `from` or a namespaceSelector matching all namespaces.

### E. Secrets and config
- **AC-16** Rendered `staging` and `base` contain **no** `kind: Secret`. Rendered `local` contains only the Secrets
  of section 6.1, produced by `secretGenerator` from `overlays/local/secrets.dev.env`, whose every value contains
  `dev` and which starts with a "DEV ONLY – never use outside a local cluster" header.
- **AC-17** No container `env[].value` and no ConfigMap value in any rendered overlay has a name matching
  `(?i)(PASSWORD|SECRET|TOKEN|_KEY$|API_KEY)` or a value matching `://[^/@\s]+:[^/@\s]+@` (credentials in a URL);
  such values come only from `secretKeyRef` / `secretRef`. `DATABASE_URL`, `AMQP_URL`, `RABBITMQ_URL` always come
  from a Secret.
- **AC-18** Each process gets exactly the secret keys listed for it in 6.1 (no process mounts a Secret it does not
  use, e.g. notification does not get `JWT_SECRET`); identity/event use `DATABASE_URL_DIRECT`, ticketing,
  ticketing-worker, payment, notification use `DATABASE_URL_POOLED` (F11). Optional keys use `optional: true`.
- **AC-19** Staging sets `APP_ENV=production` for every Go process; local sets `APP_ENV=development`.

### F. Images
- **AC-20** Every app image is `ghcr.io/phanquocthang/eventflow-<name>` with a tag matching `^sha-[0-9a-f]{7}$` or
  an `@sha256:` digest, `imagePullPolicy: IfNotPresent`; tags are set only via the overlay `images:` list.
- **AC-21** Every dev-infra image has an explicit version tag (no `latest`, no missing tag); compose's
  `edoburu/pgbouncer:latest` is replaced in k8s by a pinned tag verified to exist at implementation time.

### G. Cross-reference integrity (what kubeconform cannot see)
- **AC-22** Every Service selector matches the pod labels of exactly one workload; every Service `targetPort` name
  exists on that container; every Ingress backend names an existing Service and port; every HPA/PDB target exists;
  every `secretKeyRef`/`configMapKeyRef` names a Secret (local) / ConfigMap that the overlay renders or that 6.1
  declares as externally provided (staging), with an existing key for ConfigMaps.
- **AC-23** Ingress: `eventflow-web` keeps host `eventflow.localtest.me`; new `eventflow-gateway` Ingress on
  `api.eventflow.localtest.me` with `nginx.ingress.kubernetes.io/proxy-buffering: "off"` and
  `proxy-read-timeout ≥ 3600` (queue SSE). No other Service is exposed by Ingress or NodePort/LoadBalancer.

### H. Validation tooling and CI
- **AC-24** `deploy/k8s/validate.sh` exits 0 on the branch and non-zero with a message naming the rule and object on
  any violation; it runs on Git Bash (Windows) and ubuntu CI; it needs only docker (CI) or kubectl+python3 with
  pinned docker images as fallback for yq/kubeconform. CI `validate-deploy` runs it for both overlays and its step
  text still contains `kubeconform` and `-strict` (F2 test keeps passing).
- **AC-25** Policy self-test: for every rule in AC-4…AC-23 there is at least one fixture under
  `deploy/k8s/policy/testdata/` that violates only that rule; the checker must reject each one with that rule id and
  accept `testdata/good`. Disabling any single rule in the checker makes the self-test fail (mutation check).
- **AC-26** `gates.sh` C4.4, when a cluster is reachable, runs `kubectl apply --dry-run=server -k
  deploy/k8s/overlays/local` (namespace pre-created from the rendered output) and no longer globs raw yaml files.
- **AC-27** `docker compose -f deploy/compose/docker-compose.yml config -q` still passes and compose behaviour is
  unchanged apart from the numeric USER (same uid).
- **AC-28** Existing tests (`.github/scripts/tests/*.sh`, Go/web/python suites) still pass; no test is deleted or
  skipped.

### I. Docs and runtime evidence
- **AC-29** `docs/workflow/k8s-all/runbook.md` gives exact commands: build all images at one commit with tag
  `sha-<short>` (web with `NEXT_PUBLIC_API_BASE=http://api.eventflow.localtest.me`), load into the cluster,
  create Secrets for staging from the contract, apply each overlay, check readiness, port-forward, rollback
  (`kubectl rollout undo`, `git revert`), and lists what is NOT verified (section 9). The web runbook gets a one-line
  pointer to the new apply command.
- **AC-30** (runtime, not CI; Docker Desktop Kubernetes or kind) Given Docker is available at verification, When
  `overlays/local` is applied, Then the migrate Job completes, every app pod becomes Ready, `GET /healthz` of gateway
  via port-forward is 200, deleting one gateway pod never leaves 0 Ready endpoints, and a pod from a scratch
  namespace cannot reach `eventflow-ticketing:8082` while gateway can (only if the CNI enforces NetworkPolicy; Docker
  Desktop does not, which must then be stated). If Docker is not available the evidence file says **NOT VERIFIED**
  and why; that is acceptable for merge only with explicit human sign-off.
- **AC-31** (runtime) `kubectl apply --dry-run=server -k deploy/k8s/overlays/staging` succeeds on any cluster with
  the Secrets of 6.1 pre-created (or reports only "secret not found" warnings, which is expected and listed).

## 6. Contracts

### 6.1 Secret contract (names and keys; values never in git except local dev file)

| Secret | Keys | Used by |
|---|---|---|
| `eventflow-db` | `DATABASE_URL_DIRECT`, `DATABASE_URL_POOLED` | identity, event (direct); ticketing, ticketing-worker, payment, notification (pooled); migrate Job (direct, local) |
| `eventflow-mq` | `AMQP_URL` (mapped to env `AMQP_URL` or `RABBITMQ_URL` as compose names it) | event, ticketing, ticketing-worker, payment, notification, ai-worker |
| `eventflow-auth` | `JWT_SECRET`, `QUEUE_TOKEN_SECRET` | gateway (both), identity (both), waitingroom, ticketing (`QUEUE_TOKEN_SECRET`) |
| `eventflow-qr` | `HMAC_QR_SECRET` | ticketing, ticketing-worker (must be the same value) |
| `eventflow-identity` | `PII_ENC_KEY`, `PII_HASH_KEY`; optional `SMTP_USER`, `SMTP_PASS`, `TELEGRAM_BOT_TOKEN`, `REDIS_PASSWORD` | identity |
| `eventflow-payment` | optional `STRIPE_WEBHOOK_SECRET` | payment |
| `eventflow-ai` | optional `GEMINI_API_KEY` | ai-worker (`GEMINI_MOCK=1` in local) |
| `eventflow-dev-infra` (local only) | `POSTGRES_PASSWORD`, `RABBITMQ_DEFAULT_PASS` | dev-infra Postgres, PgBouncer, RabbitMQ |

### 6.2 NetworkPolicy ingress matrix

| Destination pods | Allowed sources (port) |
|---|---|
| web | ingress controller namespace (label `kubernetes.io/metadata.name: ingress-nginx`) on 3000 |
| gateway | ingress controller ns; web pods; ai-worker pods — on 8080. Monitoring ns on 9094 |
| identity, event, waitingroom, ticketing, payment | gateway pods on their API port; event also from ai-worker (moderation PATCH). Monitoring ns on the API port (`/metrics`) |
| ai-worker | gateway pods on 8090; monitoring ns |
| notification, waitingroom-controller, ticketing-worker | monitoring ns on metrics/HTTP port only |
| dev-infra (local) | pods of namespace `eventflow` only |

Monitoring namespace = namespaces labelled `eventflow.io/monitoring: "true"` (none exists yet; documented).
Kubelet probes are not affected by NetworkPolicy on mainstream CNIs; stated as an assumption in the runbook.

## 7. Allowed exemptions (anything else fails AC-25)

| Workload | Rule | Reason |
|---|---|---|
| web | readOnlyRootFilesystem (AC-12) | Next ISR writes `.next/cache`; web slice is out of redesign scope; follow-up |
| ticketing-worker | anti-affinity/spread (AC-11), HPA (AC-9) | singleton (F8, D5) |
| waitingroom-controller | HPA (AC-9) | fixed 2 replicas, lease-based |
| dev-infra workloads | PDB, anti-affinity, HPA, maxUnavailable 0, app image rule, `runAsNonRoot` only where the upstream image cannot start as non-root (each listed by name with reason in the component) | single-replica dev databases, local overlay only |

## 8. Decisions for the human (block implementation until answered)

- **D1 — Protected CI change (recommended: approve).** Kustomize needs `kustomization.yaml` + partial patches under
  `deploy/k8s`; the current raw-directory kubeconform (F1) would fail on them. Change only the `validate-deploy`
  steps (render both overlays → kubeconform -strict → `validate.sh` policy) and `gates.sh` C4.4 (AC-26); paths
  filter unchanged. Fallback if refused: plain manifests per service with no kustomize, policy script run only
  locally (weaker: CI would not enforce AC-4…AC-23).
- **D2 — Numeric USER in four Dockerfiles (recommended: approve).** Without it no Go/ai-worker pod can start under
  `runAsNonRoot` (F4). Same uids, compose unaffected.
- **D3 — EV-191 split (recommended).** Deliver CPU HPA with safe scale-down now (AC-9); custom-metric HPA
  (prometheus-adapter, rps/queue_depth) stays open as EV-191 because it needs Prometheus in the cluster.
- **D4 — Secrets (recommended).** No Secret objects in git for staging; Secret contract 6.1 is the interface for
  External Secrets later. Local dev values are committed in `secrets.dev.env` — the same dev defaults already
  committed in compose. Secret scanning baseline must allow that one file.
- **D5 — ticketing-worker singleton (recommended).** `replicas: 1` + `Recreate` until the reconciler is proven safe
  with two instances (F8); accepts a short gap (seconds) in sweeping/relay during rollout.
- **D6 — Infra placement (recommended).** In-cluster single-replica Postgres, PgBouncer, Redis, RabbitMQ, Mailpit
  only in `overlays/local` (dev-infra component) so a local cluster can run end to end; staging/prod point at
  external endpoints via `eventflow-endpoints`. MinIO dropped (F12). Jaeger/Prometheus/Grafana not deployed.

## 9. Known limits and debt (go into runbook)

- identity/event/payment/notification readiness = liveness (no `/readyz`, F5): a pod that lost its DB stays in
  rotation. Debt: add `httpx.Health` checks.
- identity has no graceful shutdown (F7): preStop sleep drains endpoints, but in-flight requests at SIGTERM are cut.
- Workers' probes hit `/metrics`: they prove the process serves HTTP, not that the work loop is alive.
- Web SSR in cluster calls the public API host (F13), which inside a pod may not resolve; client-side fetches work.
- Docker Desktop: no metrics-server (HPA shows `<unknown>`, min replicas still applied), no ingress controller,
  no NetworkPolicy enforcement.
- Images other than web are not published to GHCR; local clusters use locally built images.

## 10. Risks

| Risk | Mitigation |
|---|---|
| Image USER non-numeric → every pod fails | AC-13, D2 |
| Selector change on web → `apply` fails on live cluster | AC-3 forbids selector transformers |
| Liveness wired to dependency checks → restarts during Redis blip | AC-5 |
| HPA flapping / no scale | AC-8 requests, AC-9 stabilization |
| Credentials leak through env literal or ConfigMap | AC-16, AC-17, secret scan in full gate |
| NetworkPolicy blocks a real call path (e.g. ai-worker → event) | matrix 6.2 derived from compose URLs; AC-22 cross-check; AC-30 smoke |
| Policy checker silently passes | AC-25 negative fixtures + mutation check |
| CI change breaks other jobs | lint-ci (actionlint + test-workflows) must stay green; change limited to two steps |
| Two ticketing-workers double-correct inventory | D5 |

## 11. Definition of Ready

Spec approved with D1-D6 answered; 4.1, 6.1, 6.2, 7 frozen as contracts; test designer has this spec only.

## Human approval

Approved by TranDinhVo on 2026-11-01 in the Claude Code session, with all recommended decisions.

## Erratum (round 2, after plan-review.md round 1)

Status: E3 recorded as implicitly approved by the human (least privilege, relayed by the coordinator 2026-11-01);
E1, E2, E4-E7 need the human's sign-off **before WP0 freezes fixtures**. Where this section and the text above
disagree, this section wins.

- **E1 (M1) — exemption format and rule ids.** Section 4 "Exemption" is replaced by `policy-contract.md` §3: one
  annotation per rule, `eventflow.io/policy-exempt.AC-<n>: "<reason>"` (ticketing-worker needs AC-9 and AC-11).
  Rule ids = AC numbers, listed in `policy-contract.md` §2. AC-25 reads "reject with the rule id defined in
  `policy-contract.md`".
- **E2 (M3) — per-overlay app settings; new AC-32.** Spec 4.1 "env = compose block" applies only to values that are
  the same in every environment. Environment-dependent app settings live in a per-overlay ConfigMap
  `eventflow-app-config` (configMapGenerator from `overlays/<o>/app-config.env`) read with `configMapKeyRef`
  (`optional: true` where staging omits the key). Values:

  | Key | local | staging |
  |---|---|---|
  | `APP_ENV` | development | production |
  | `MIN_SALE_LEAD_SECONDS` (event) | 60 | **absent** (BR-E2 default 30 min) |
  | `ACTIVE_EVENT_IDS` (controller, worker) | the two demo UUIDs | **absent** (discovery from Postgres, EV-118) |
  | `ADMIT_MIN_RATE/MAX_RATE/STEP_UP/BACKOFF/TTL_SECONDS` | 5 / 50 / 10 / 0.7 / 900 | **absent** (`app.DefaultAdmitConfig`) |
  | `GEMINI_MOCK` | 1 | 0 (`GEMINI_API_KEY` then **required** in `eventflow-ai`, ai-worker refuses to start otherwise) |
  | `PAYMENT_SANDBOX_MODE` | success | **absent** |
  | `PAYMENT_DEFAULT_PROVIDER` | sandbox | vnpay |
  | `PAYMENT_ENABLED_PROVIDERS` | sandbox,vnpay,momo,stripe | vnpay,momo,stripe |
  | `CORS_ALLOWED_ORIGINS` | http://eventflow.localtest.me | https://<staging web host> placeholder |

  Note: compose sets `PAYMENT_SANDBOX_MODE: "true"`, which is not a valid mode (`success|fail|delay`,
  `services/payment/internal/adapter/gateway/sandbox.go:16-18`); local uses `success`. Compose is not changed.
  **AC-32 (new)** Given the staging render, Then no container env and no ConfigMap carries `MIN_SALE_LEAD_SECONDS`,
  `ACTIVE_EVENT_IDS`, any `ADMIT_*`, `PAYMENT_SANDBOX_MODE`, `GEMINI_MOCK` ≠ `"0"`, `PAYMENT_DEFAULT_PROVIDER=sandbox`
  or `sandbox` in `PAYMENT_ENABLED_PROVIDERS`. Checked by the policy (rule AC-32, fixtures per `policy-contract.md` §7).
  **D7 (new, recommended):** staging payment uses the providers' own test endpoints (code defaults for
  `VNPAY_BASE_URL`, `MOMO_ENDPOINT`) with credentials from `eventflow-payment`, not the in-process sandbox; a load test
  that must not call providers adds its own patch outside this task.
- **E3 (m1) — least-privilege secret mapping; replaces the "Used by" column of 6.1 and corrects F10.** Gateway in
  production requires only `JWT_SECRET` (`services/gateway/internal/config/config.go:66-79`); only waitingroom and
  ticketing read `QUEUE_TOKEN_SECRET`; the ticketing **server** reads no AMQP URL (compose's `AMQP_URL` there is dead
  config). Payment also reads provider secrets via `getEnvOrDefault` (`services/payment/internal/config/config.go:82-96`).
  AC-18 checks exactly this table:

  | Process | Secret keys (env name ← `secret/key`) |
  |---|---|
  | gateway | `JWT_SECRET ← eventflow-auth/JWT_SECRET` |
  | identity | `DATABASE_URL ← eventflow-db/DATABASE_URL_DIRECT`, `JWT_SECRET ← eventflow-auth/JWT_SECRET`, `PII_ENC_KEY`, `PII_HASH_KEY` ← `eventflow-identity`; optional `SMTP_USER`, `SMTP_PASS`, `TELEGRAM_BOT_TOKEN`, `REDIS_PASSWORD` ← `eventflow-identity` |
  | event | `DATABASE_URL ← eventflow-db/DATABASE_URL_DIRECT`, `RABBITMQ_URL ← eventflow-mq/AMQP_URL` |
  | waitingroom | `QUEUE_TOKEN_SECRET ← eventflow-auth/QUEUE_TOKEN_SECRET` |
  | waitingroom-controller | none |
  | ticketing | `DATABASE_URL ← eventflow-db/DATABASE_URL_POOLED`, `QUEUE_TOKEN_SECRET ← eventflow-auth/QUEUE_TOKEN_SECRET`, `HMAC_QR_SECRET ← eventflow-qr/HMAC_QR_SECRET` |
  | ticketing-worker | `DATABASE_URL ← eventflow-db/DATABASE_URL_POOLED`, `RABBITMQ_URL ← eventflow-mq/AMQP_URL`, `HMAC_QR_SECRET ← eventflow-qr/HMAC_QR_SECRET` |
  | payment | `DATABASE_URL ← eventflow-db/DATABASE_URL_POOLED`, `RABBITMQ_URL ← eventflow-mq/AMQP_URL`; optional `VNPAY_TMN_CODE`, `VNPAY_HASH_SECRET`, `MOMO_PARTNER_CODE`, `MOMO_ACCESS_KEY`, `MOMO_SECRET_KEY`, `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET` ← `eventflow-payment` (required in staging by D7) |
  | notification | `DATABASE_URL ← eventflow-db/DATABASE_URL_POOLED`, `AMQP_URL ← eventflow-mq/AMQP_URL` |
  | ai-worker | `AMQP_URL ← eventflow-mq/AMQP_URL`; `GEMINI_API_KEY ← eventflow-ai` (optional local, required staging) |
  | web | none |
  | migrate Job (local) | `GOOSE_DBSTRING ← eventflow-db/DATABASE_URL_DIRECT`, `GOOSE_DRIVER=postgres` literal |

  Secret objects only by key reference (`secretKeyRef`), never `envFrom: secretRef`, so a pod sees only its keys.
  Dev PII keys in `secrets.dev.env`: 44-char base64 strings that contain `dev` and decode to exactly 32 bytes.
- **E4 (m6)** ai-worker app grace = 20 s (`shutdown_grace_seconds`, `services/ai-worker/app/config.py:56`);
  `terminationGracePeriodSeconds` 35 like the other 20-s services.
- **E5 (m7)** Minimum cluster version 1.30 (`preStop.sleep` beta on by default; GA 1.34). Stated in runbook.
- **E6 (m4)** `ci.yml` paths filter `deploy` gains `'deploy/docker/**'` so AC-13 runs when a Dockerfile changes.
  This replaces "paths filter unchanged" in D1; still inside the approved validate-deploy scope.
- **E7 (M2)** AC-24 tooling: validation needs **no cluster and no Docker**; it renders with pinned kustomize
  (not kubectl) and runs kubeconform/yq at pinned versions via `go run` (`policy-contract.md` §6). It needs network
  on first run (Go modules, kubeconform schemas) and caches both. Docker images are an optional mode.

## Human approval of round-2 errata

Errata E1, E2, E4, E5, E6, E7 and decision D7 APPROVED by TranDinhVo on 2026-11-01 in the Claude Code session (E3 approved earlier).

## Erratum E8 (round 3, after plan-review.md round 2) — text of the approved change

Approved by the human on 2026-11-01 (see the approval section below). Where E8 disagrees with E1-E7 or the body, E8 wins.
The precise checker semantics are in `policy-contract.md` (round-3 revision). This section states the intent.

1. **Staging fails closed (R2-M1).**
   - AC-32 requires these to be **present** in staging, not only "not demo":
     - `GEMINI_MOCK="0"`.
     - `PAYMENT_DEFAULT_PROVIDER` ∈ enabled providers, and `PAYMENT_ENABLED_PROVIDERS` = `vnpay,momo`.
     - `EMAIL_PROVIDER=smtp` (notification code default is `mock`).
     - For each enabled provider, its callback URLs (`VNPAY_RETURN_URL`; `MOMO_RETURN_URL`, `MOMO_NOTIFY_URL`) and its
       secret keys (`VNPAY_TMN_CODE`, `VNPAY_HASH_SECRET`; `MOMO_PARTNER_CODE`, `MOMO_ACCESS_KEY`, `MOMO_SECRET_KEY`).
   - "Present" means one of:
     - an env literal;
     - a **non-optional** `configMapKeyRef` to a key that exists in the rendered ConfigMap;
     - for the secret keys, a **non-optional** `secretKeyRef`.
   - A demo/forbidden key counts as set when it is an env literal, or when any ref (optional or not) points to a key
     that exists in the rendered ConfigMap. An optional ref to a key that does not exist counts as absent, which is
     allowed for forbidden keys.
2. **No optional secret refs, overlay-specific key sets (R2-M1).**
   - AC-18 forbids `optional: true` on every `secretKeyRef`, in both overlays. A missing Secret must fail loudly
     (`CreateContainerConfigError`) instead of falling back to public code defaults. Local therefore gets dev values
     for every referenced key.
   - The per-process key sets differ per overlay. The table in `policy-contract.md` §8 replaces the E3 table:
     - identity and notification get SMTP credentials in staging only;
     - payment gets vnpay+momo+stripe keys in local and vnpay+momo in staging;
     - the optional identity keys (`TELEGRAM_BOT_TOKEN`, `REDIS_PASSWORD`) are dropped from both overlays.
3. **Stripe disabled in staging.** Staging `PAYMENT_ENABLED_PROVIDERS=vnpay,momo`; no Stripe key or URL is rendered in
   staging. Security debt (runbook + spec §9): the Stripe webhook accepts unsigned requests and the literal `v1=valid_sig`
   (`services/payment/internal/adapter/gateway/stripe.go:122-148`). Re-enable Stripe in staging only after that is fixed.
4. **Exemptions scoped to a sub-check (R2-M2).**
   - Exemption annotations name a rule and, where the rule has sub-checks, the sub-check:
     `eventflow.io/policy-exempt.AC-12.readOnlyRootFilesystem`.
   - Allowed pairs:
     - web: `AC-12.readOnlyRootFilesystem` only;
     - dev-infra: `AC-12.runAsNonRoot` only (expected unused, since dev-infra runs as the image uid), plus the
       whole-rule `AC-6`, `AC-10`, `AC-11`, `AC-20`;
     - ticketing-worker: whole-rule `AC-11`.
   - **AC-9 is never exemptable.** The fixed-replica workloads are defined by AC-9 as "must have no HPA", so an HPA
     targeting them fails AC-9, and any `policy-exempt.AC-9*` annotation fails `EXEMPT`.
   - The spec §7 exceptions exist **only** through annotations. Nothing is hard-coded, so a web Deployment without
     the annotation fails AC-12.
5. **Clarifications (r2-m1, r2-m3, r2-m10).**
   - AC-1 counts only Deployments without label `eventflow.io/tier: dev-infra`.
   - Generated Secret/ConfigMap names may carry the kustomize hash suffix. Rules compare the base name. References
     must name the rendered object exactly.
   - `APP_ENV` and `CORS_ALLOWED_ORIGINS` live in `eventflow-app-config` only. `eventflow-endpoints` holds only infra
     hosts and ports: `REDIS_ADDR`, `REDIS_URL`, `SMTP_HOST`, `SMTP_PORT`, `OTEL_EXPORTER_OTLP_ENDPOINT`.
6. **Per-environment settings added (r2-m8).** `eventflow-app-config` also carries:
   - `EMAIL_PROVIDER`;
   - `SMTP_FROM`;
   - `GOOGLE_CLIENT_ID`;
   - `VNPAY_RETURN_URL`, `MOMO_RETURN_URL`, `MOMO_NOTIFY_URL`;
   - local only: `STRIPE_SUCCESS_URL`, `STRIPE_CANCEL_URL`.

   `STRIPE_PUBLISHABLE_KEY` comes from a Secret, because its name matches the AC-17 pattern. New Secret
   `eventflow-notification` (`SMTP_USER`, `SMTP_PASSWORD`; staging only). Known limits that go into the runbook:
   - `SMS_PROVIDER` stays `mock` (no SMS provider in scope);
   - staging Redis must be private and passwordless (r2-m11).

## Human approval of erratum E8 (2026-11-01)

APPROVED by TranDinhVo: (1) staging fails closed: GEMINI_MOCK="0" and all payment provider secrets for enabled providers are REQUIRED (no optional refs in staging; AC-18 becomes overlay-specific, AC-32 also requires presence); (2) exemptions are scoped to a named sub-check (web AC-12: readOnlyRootFilesystem only; dev-infra AC-12: runAsNonRoot only); AC-9 is NOT exemptable for fixed-replica workloads. Decision: Stripe is DISABLED in staging (enabled providers = vnpay, momo) until the payment Stripe webhook signature check is fixed (recorded as security debt).

## Human confirmation after plan review round 3 (2026-11-01)

TranDinhVo confirmed: (1) GOOGLE_CLIENT_ID must be present and non-empty in staging (added to the AC-32 required list; one bad fixture); (2) no optional secretKeyRef in BOTH overlays (local and staging), as E8.2 text says.
