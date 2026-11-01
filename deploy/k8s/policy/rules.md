# EventFlow k8s policy rules

Checker: `deploy/k8s/policy/check_policy.py` (python3 stdlib, run as `python3 -I`). Interface frozen in
`docs/workflow/k8s-all/policy-contract.md` (rev 2); requirements in `docs/workflow/k8s-all/spec.md` (incl. errata
E1-E8). This file says how each rule is evaluated, so a reviewer can compare it with the contract without reading
the code. One rule id = one `check_ac<n>` function, listed in `RULES`.

## Running

```
bash deploy/k8s/validate.sh                    # render both overlays, kubeconform, policy, AC-13, self-test
bash deploy/k8s/validate.sh --fixtures-only    # self-test only (no render)
bash deploy/k8s/policy/test_policy.sh          # self-test incl. the real tree (section E)
python3 -I deploy/k8s/policy/check_policy.py --overlay staging --input render.json [--processes web,gateway]
python3 -I deploy/k8s/policy/check_policy.py --dockerfiles deploy/docker
```

`render.json` must come from the pinned yq: `yq ea -o=json '[.]' render.yaml`. `null` entries are ignored.
Exit 0 = OK, 1 = violations, 2 = usage/input error (`ERROR ...` on stderr).

## Vocabulary

- **Workload**: Deployment, StatefulSet or Job. **Dev-infra** = label `eventflow.io/tier: dev-infra`; every other
  workload is **app**.
- **Process**: an app Deployment named `eventflow-<p>` with `<p>` from spec 4.1. Per-process rules only demand
  objects for processes in `--processes` (default: all 11).
- **Base name**: rendered name minus a trailing `-[a-z0-9]{10}` (kustomize hash suffix).
- **Exemption**: annotation `eventflow.io/policy-exempt.AC-<n>[.<subcheck>]: "<reason>"` on the workload's
  top-level metadata. Valid only if the pair is in the allowed table and the reason is non-empty (see EXEMPT).
  A valid whole-rule key skips that rule for that workload; a sub-check key skips only that sub-check. Invalid keys
  have no effect.

## Rules

| Id | What fails |
|---|---|
| AC-1 | The set of app Deployment names is not exactly `eventflow-<p>` for the expected processes; not exactly one Namespace, or it is not `eventflow`; any namespaced object whose `metadata.namespace` is not `eventflow` (cluster-scoped kinds skipped). |
| AC-3 | web: selector not exactly `{name: eventflow-web, part-of: eventflow}`; no container `web`; no port `http`; web probe paths changed; strategy not RollingUpdate 0/1; web Service/Ingress/HPA missing. |
| AC-4 | A Deployment/StatefulSet container lacks liveness or readiness; an app container lacks startup; for processes: each probe must be `httpGet` with the 4.1 path and the port **name** (`http`, or `metrics` for the controller/worker). Dev-infra probes are checked for presence only. Jobs skipped. |
| AC-5 | Any liveness probe on `/readyz` or `/api/readyz`; for processes with `/readyz`, readiness path equal to liveness path. |
| AC-6 | Request-serving processes (web, gateway, identity, event, waitingroom, ticketing, payment, ai-worker) and non-exempt dev-infra: Deployment strategy not RollingUpdate with `maxUnavailable: 0`, `maxSurge: 1`; a container without `preStop`; `terminationGracePeriodSeconds` (default 30) below `5 + app grace + 5` (web 5, gateway/waitingroom/ticketing/ai-worker 20, payment 10, event 5, identity 0). |
| AC-7 | ticketing-worker `replicas` ≠ 1 or strategy ≠ Recreate; waitingroom-controller `replicas` ≠ 2; an HPA-managed Deployment sets `spec.replicas`. |
| AC-8 | Any container or init container of any workload without requests and limits for cpu and memory; process containers whose values differ from 4.1 (quantities compared numerically, so `1` = `1000m`). |
| AC-9 | Not exactly one HPA targeting each HPA-managed Deployment; apiVersion ≠ `autoscaling/v2`; `minReplicas` < 2 (default 1); `maxReplicas` ≠ 4.1; not exactly one cpu Resource metric with `averageUtilization` 60-75; `scaleDown.stabilizationWindowSeconds` < 300; an HPA targeting an existing workload that is not HPA-managed (controller, worker, dev-infra). Never exemptable. An HPA whose target does not exist is AC-22. |
| AC-10 | Each process Deployment and each non-exempt dev-infra Deployment/StatefulSet must be selected by exactly one PDB, `policy/v1`, `maxUnavailable: 1`, no `minAvailable`, `selector == {matchLabels: <pod labels>}`. A PDB that selects nothing is AC-22. |
| AC-11 | Process Deployments and dev-infra, unless exempt: no preferred podAntiAffinity term on `kubernetes.io/hostname` whose `matchLabels` equal the Deployment selector; no topologySpreadConstraint on `topology.kubernetes.io/zone` with `ScheduleAnyway` and the same `matchLabels`. |
| AC-12 | Sub-checks of contract §2.1, each reported as `AC-12 ... <subcheck>: <reason>`: `runAsNonRoot`, `seccomp`, `automount`, `allowPrivilegeEscalation`, `dropAll`, `privileged`, `hostNamespaces`, `hostPath`, `rootUid` (all workloads); `noRunAsUser`, `readOnlyRootFilesystem` (app only); `tmpEmptyDir` (ai-worker). Container checks include init containers. |
| AC-13 | `--dockerfiles <dir>`: a `*.Dockerfile` directly in `<dir>` whose final stage (after the last `FROM`) has no `USER`, or whose last `USER` is not `<digits>[:<digits>]`. Kind `Dockerfile`, name = file name without `.Dockerfile`. |
| AC-14 | Namespace `eventflow` PSA labels: staging `enforce=restricted`; local `enforce=baseline`, `warn=restricted`, `audit=restricted`. |
| AC-15 | No NetworkPolicy with `podSelector: {}`, policy type Ingress and no ingress rules (default deny); a process pod not selected by any policy with ingress rules; in a policy selecting a process pod: a rule with empty `from`, or a peer/port not in the spec 6.2 matrix. Peers are recognised only as `podSelector` on `app.kubernetes.io/name: eventflow-<p>`, `namespaceSelector` on `kubernetes.io/metadata.name: <ns>`, or `namespaceSelector` on `eventflow.io/monitoring: "true"`; anything else (e.g. `namespaceSelector: {}`) fails. Missing `ports` = all ports = fails. |
| AC-16 | staging: any Secret. local: a Secret whose base name is not in contract §8, or a decoded `data` / `stringData` value without `dev`. |
| AC-17 | An env literal whose name matches `(?i)(PASSWORD\|SECRET\|TOKEN\|_KEY$\|API_KEY)` or is `DATABASE_URL`/`AMQP_URL`/`RABBITMQ_URL`; an env literal value with URL credentials `://user:pass@`; a ConfigMap key matching the name pattern or value with URL credentials. |
| AC-18 | Any `secretKeyRef` with `optional: true` (any workload); any `envFrom.secretRef`; per process, the secret env set (env name → Secret base name, key) differs from contract §8 for the overlay. Staging payment: vnpay/momo keys may be absent (AC-32 checks presence) but nothing else may be added (Stripe keys are extras). Refs that do not resolve by base name are left to AC-22. |
| AC-19 | A Go process without `APP_ENV`, or `APP_ENV` from a ConfigMap other than `eventflow-app-config`, or a value other than `production` (staging) / `development` (local). |
| AC-20 | App containers (and dev-infra without the AC-20 exemption): image not `ghcr.io/phanquocthang/<4.1 image>` with `:sha-<7 hex>` or `@sha256:<64 hex>`; `imagePullPolicy` ≠ `IfNotPresent`. |
| AC-21 | Dev-infra image without a tag, or tagged `latest` (digests pass). |
| AC-22 | Service selector matching ≠ 1 Deployment/StatefulSet; a string targetPort that is not a container port name; a numeric targetPort for a named port (or unknown number); Ingress backend Service or port missing; HPA target missing; PDB selecting nothing; a `secretKeyRef`/`configMapKeyRef`/`envFrom`/volume ref that does not name a rendered object **exactly** (local Secrets and ConfigMaps carry the hash suffix), or names a missing key. Staging Secrets may instead be contract external Secrets with a key from any §8 column. An optional `configMapKeyRef` to a missing key is allowed. |
| AC-23 | An Ingress other than `eventflow-web` / `eventflow-gateway`, or one exposing another existing Service; web Ingress host ≠ `eventflow.localtest.me`; gateway Ingress missing, host ≠ `api.eventflow.localtest.me`, `proxy-buffering` ≠ `"off"`, `proxy-read-timeout` < 3600; any NodePort/LoadBalancer Service. |
| AC-32 | staging only. Forbidden (`MIN_SALE_LEAD_SECONDS`, `ACTIVE_EVENT_IDS`, `ADMIT_*`, `PAYMENT_SANDBOX_MODE`, `STRIPE_*`): present as a key of any rendered ConfigMap, or set on a container as a literal or via any `configMapKeyRef` (optional or not) to an existing key. Required present (literal, or non-optional `configMapKeyRef` to an existing key): ai-worker `GEMINI_MOCK="0"`, notification `EMAIL_PROVIDER="smtp"`, identity `GOOGLE_CLIENT_ID` non-empty, payment `PAYMENT_ENABLED_PROVIDERS` = {vnpay, momo}, `PAYMENT_DEFAULT_PROVIDER` in that list, and per enabled provider the callback URLs (non-empty) and secret keys (non-optional `secretKeyRef`). |
| EXEMPT | An exemption key that is malformed, names AC-9, is not an allowed pair for the workload (web: `AC-12.readOnlyRootFilesystem`; ticketing-worker: `AC-11`; dev-infra: `AC-6`, `AC-10`, `AC-11`, `AC-20`, `AC-12.runAsNonRoot`), or has an empty reason. |

## Mutation check (AC-25)

Each rule was disabled one at a time (function body replaced by `return`; AC-13 by redirecting its output) and the
self-test cases of sections B/C re-evaluated with the `test_policy.sh` criteria: all 24 rule mutants fail the
self-test. Extra mutants (per AC-12 sub-check, sub-check exemption widened to the whole rule, AC-9 exemption
accepted) are listed with their results in the WP5 report.
