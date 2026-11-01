# Policy interface contract: k8s-all (FROZEN, revision 2 = round 3)

Frozen interface between WP0 (test designer: fixtures + `test_policy.sh`) and WP5 (implementer:
`check_policy.py`, `validate.sh`). Hand this file to WP0 together with `spec.md` (incl. errata E1-E8).
Changes after freeze need a change request approved by the human, like a test change request.

Revision history:
- r1: round 2.
- r2 (this revision): round 3, contract change request approved by the human as erratum E8 (2026-11-01). Changes:
  - §3: exemptions scoped to sub-checks; AC-9 is not exemptable.
  - §2: AC-1/AC-12/AC-18/AC-22/AC-32 semantics; new AC-13 dir mode.
  - §8 (per-overlay secret table) and §9 (name matching) are new.
  - §4: `null` input entries are ignored.

The fixtures that must change because of r2 are listed in `plan.md` "Round 3" §C.

## 1. Files

| Path | Owner | Notes |
|---|---|---|
| `deploy/k8s/policy/check_policy.py` | WP5 | python3 stdlib only, run as `python3 -I` |
| `deploy/k8s/policy/test_policy.sh` | WP0 | self-test; protected after freeze |
| `deploy/k8s/policy/testdata/**` | WP0 | fixtures (incl. `gen_fixtures.py`, `dockerfiles/`); protected after freeze |
| `deploy/k8s/tools.env` | WP5 | pinned tool versions (§6); sourced by both scripts; LF only |
| `deploy/k8s/validate.sh` | WP5 | entry point |

## 2. Rule ids

Rule id = the spec AC number it enforces. One id = one check function, so the mutation test can disable exactly one
id at a time. "Workload" = Deployment, StatefulSet or Job. "App workload" = workload **without** label
`eventflow.io/tier: dev-infra`; "dev-infra workload" = with it.

| Id | Checks | Scope / overlay |
|---|---|---|
| AC-1 | one **app** Deployment per process in `--processes` (Deployments labelled dev-infra are not counted), exactly one Namespace `eventflow`, no namespaced object outside it | whole render |
| AC-3 | web names, selector, container `web`, port `http`, probe paths, strategy unchanged | web |
| AC-4 | liveness + readiness on every container of every workload; startup on app containers; paths/ports per spec 4.1 (port by name) | all workloads (Jobs excluded) |
| AC-5 | readiness ≠ liveness where `/readyz` exists; no liveness on `/readyz` or `/api/readyz` | all workloads |
| AC-6 | request-serving: maxUnavailable 0, maxSurge 1, preStop, grace ≥ 5 + app grace + 5 | app + dev-infra (dev-infra exemptable) |
| AC-7 | worker replicas 1 + Recreate; controller replicas 2; HPA-managed have no `spec.replicas` | app |
| AC-8 | requests + limits cpu/mem on every container incl. init and Jobs; app values = 4.1 | all workloads |
| AC-9 | one autoscaling/v2 HPA per HPA-managed Deployment (min ≥ 2, max per 4.1, cpu 60-75, scaleDown window ≥ 300); **no HPA targets any other workload** (fixed-replica, dev-infra). Not exemptable | HPA + workloads |
| AC-10 | exactly one policy/v1 PDB per Deployment/StatefulSet, `maxUnavailable: 1`, selector = pod labels | app + dev-infra (dev-infra exemptable) |
| AC-11 | preferred hostname anti-affinity + zone spread `ScheduleAnyway` on own labels | app + dev-infra (exemptable as §3) |
| AC-12 | security sub-checks, §2.1 | all workloads |
| AC-13 | every `*.Dockerfile` in the given dir ends its final stage with `USER <digits>[:<digits>]` | `--dockerfiles` mode (§4) |
| AC-14 | Namespace PSA labels: staging enforce=restricted; local enforce=baseline, warn=restricted, audit=restricted | overlay-specific |
| AC-15 | default-deny ingress; every app pod has an allow policy; matrix spec 6.2; no open `from` for backends | NetworkPolicy |
| AC-16 | Secret objects: none in staging; in local only base names of §8, every decoded value contains `dev` | overlay-specific |
| AC-17 | no env literal / ConfigMap entry whose name matches `(?i)(PASSWORD\|SECRET\|TOKEN\|_KEY$\|API_KEY)` or whose value matches `://[^/@\s]+:[^/@\s]+@` | all |
| AC-18 | per process, the set of secret env vars equals §8 for the overlay (env name, Secret base name, key); no `optional: true` on any `secretKeyRef`; no `envFrom.secretRef`. Exception: in staging, the payment provider keys are checked for **absence of extras** only; their **presence** is AC-32 | overlay-specific |
| AC-19 | `APP_ENV` = production (staging) / development (local) on every Go process, via `eventflow-app-config` or a literal | overlay-specific |
| AC-20 | app image `ghcr.io/phanquocthang/eventflow-<name>` with `sha-[0-9a-f]{7}` tag or `@sha256:` digest, `IfNotPresent` | app (dev-infra exemptable) |
| AC-21 | dev-infra images pinned: explicit tag, not `latest` | dev-infra, local |
| AC-22 | cross refs: Service → exactly one workload, targetPort names, Ingress → Service/port, HPA/PDB targets exist, refs resolve (§9) | whole render |
| AC-23 | only web + gateway Ingress, hosts and SSE annotations; no NodePort/LoadBalancer | Ingress/Service |
| AC-32 | staging fail-closed settings, §2.2 | staging only |
| EXEMPT | exemption annotation not in the §3 allowed table, empty reason, or malformed key | annotations |

AC-2 (kubeconform) and AC-24..AC-31 are not checker rules.

### 2.1 AC-12 sub-checks (ids are also the exemption suffix)

| Sub-check | Requirement | App | Dev-infra |
|---|---|---|---|
| `runAsNonRoot` | pod `securityContext.runAsNonRoot: true` | yes | yes (exemptable) |
| `seccomp` | pod `seccompProfile.type: RuntimeDefault` | yes | yes |
| `automount` | `automountServiceAccountToken: false` | yes | yes |
| `allowPrivilegeEscalation` | every container `false` | yes | yes |
| `dropAll` | every container `capabilities.drop` contains `ALL` and `add` is empty | yes | yes |
| `privileged` | no container `privileged: true` | yes | yes |
| `hostNamespaces` | no `hostNetwork` / `hostPID` / `hostIPC` | yes | yes |
| `hostPath` | no `hostPath` volume | yes | yes |
| `rootUid` | no `runAsUser: 0` (pod or container) | yes | yes |
| `noRunAsUser` | no `runAsUser` at pod or container level | yes | no (dev-infra must set its image uid) |
| `readOnlyRootFilesystem` | every container `true` | yes (web exemptable) | no |
| `tmpEmptyDir` | ai-worker mounts an `emptyDir` at `/tmp` | ai-worker only | no |

### 2.2 AC-32 (staging only)

Definitions:
- A key is **set** if it is an env literal, or any `configMapKeyRef` (optional or not) points to a key that exists in
  the rendered ConfigMap.
- A key is **present** if it is an env literal, a **non-optional** `configMapKeyRef` to an existing key, or (secret
  keys) a non-optional `secretKeyRef`.
- An optional ref whose key exists = set but **not present**. An optional ref whose key is missing = absent.

Rules:
- **Forbidden (must not be set):** `MIN_SALE_LEAD_SECONDS`, `ACTIVE_EVENT_IDS`, any `ADMIT_*`,
  `PAYMENT_SANDBOX_MODE`, any `STRIPE_*` read from a ConfigMap or literal. Stripe `secretKeyRef`s in staging are an
  AC-18 extra, not AC-32, so one fixture trips one rule. A forbidden key that exists in a rendered ConfigMap but that no container
  reads is also a violation, so a later env ref cannot pick it up silently.
- **Required present, with these values:**
  - ai-worker: `GEMINI_MOCK = "0"`.
  - payment: `PAYMENT_ENABLED_PROVIDERS = "vnpay,momo"` (order-insensitive; no `sandbox`, no `stripe`), and
    `PAYMENT_DEFAULT_PROVIDER` ∈ that list.
  - notification: `EMAIL_PROVIDER = "smtp"`.
- **Required present, any non-empty value, on payment, per enabled provider:**
  - vnpay: `VNPAY_RETURN_URL`; secret keys `VNPAY_TMN_CODE`, `VNPAY_HASH_SECRET`.
  - momo: `MOMO_RETURN_URL`, `MOMO_NOTIFY_URL`; secret keys `MOMO_PARTNER_CODE`, `MOMO_ACCESS_KEY`, `MOMO_SECRET_KEY`.

## 3. Exemption annotation

One annotation **per exempted pair**, on the workload's top-level `metadata.annotations`:

```yaml
metadata:
  annotations:
    eventflow.io/policy-exempt.AC-12.readOnlyRootFilesystem: "Next ISR writes .next/cache"   # web
    eventflow.io/policy-exempt.AC-11: "singleton: no spread for 1 replica (spec D5)"           # ticketing-worker
```

- Key regex: `^eventflow\.io/policy-exempt\.(AC-[0-9]+)(\.([A-Za-z]+))?$` (rule, optional sub-check). Value =
  non-empty reason after trim.
- Allowed pairs (a constant table in `check_policy.py`):

  | Workload | Allowed keys |
  |---|---|
  | `eventflow-web` | `AC-12.readOnlyRootFilesystem` |
  | `eventflow-ticketing-worker` | `AC-11` |
  | any dev-infra workload | `AC-6`, `AC-10`, `AC-11`, `AC-20`, `AC-12.runAsNonRoot` |

  Everything else → `FAIL EXEMPT`, including:
  - any `AC-9` / `AC-9.*` key;
  - `AC-12` without a sub-check;
  - an unknown sub-check;
  - a pair on another workload.
- Effect of a valid exemption: a whole-rule key skips that rule for that workload (and its HPA/PDB); a sub-check key
  skips only that sub-check.
- An invalid exemption has **no** effect: the rule is still evaluated, and `EXEMPT` is reported in addition.
- An unused valid exemption is not an error.
- Spec §7 exceptions exist only through these annotations; nothing is hard-coded. A web Deployment without the
  annotation fails `AC-12`.

## 4. CLI

```
python3 -I deploy/k8s/policy/check_policy.py --overlay {local|staging} --input <file.json|-> [--processes <csv>]
python3 -I deploy/k8s/policy/check_policy.py --dockerfiles <dir>
```

- `--overlay` (required in render mode) selects the overlay-specific rules: AC-14, AC-16, AC-18, AC-19, AC-21, AC-32.
- `--input`: a JSON **array** of Kubernetes objects; `-` = stdin.
  - It is produced only by `yq ea -o=json '[.]' <render.yaml>` with the pinned yq.
  - `null` entries (from empty YAML documents) are ignored.
  - The checker never reads YAML.
- `--processes`: comma-separated process names from spec 4.1 that the input is expected to contain. Default = all
  11. AC-1/AC-9/AC-10/AC-15/AC-18/AC-22 only demand objects for these processes.
- `--dockerfiles <dir>`: runs only AC-13, on every `*.Dockerfile` directly in `<dir>`. `validate.sh` passes
  `deploy/docker`; the self-test passes the fixture dirs (§7).
- Exit codes:
  - `0`: no violation.
  - `1`: one or more violations.
  - `2`: usage or input error (bad args, unreadable file, JSON not an array, unknown process name). Exit 2 prints
    `ERROR <message>` on stderr.

## 5. Output

- One line per violation on stdout, regex: `^FAIL (AC-[0-9]+|EXEMPT) ([A-Za-z]+)/([a-z0-9.-]+): (.+)$`
  (kind, `metadata.name`, human reason).
  - Whole-render violations use kind `Render` and name = overlay, e.g.
    `FAIL AC-1 Render/staging: 10 app Deployments, want 11`.
  - AC-13 uses kind `Dockerfile` and name = the file name without extension.
- Last line, always:
  - `policy: OK (<n> objects, overlay=<o>)` on exit 0;
  - `policy: <k> violation(s) (overlay=<o>)` on exit 1.
  - In `--dockerfiles` mode, overlay is printed as `dockerfiles`.
- The order of FAIL lines is not part of the contract.

## 6. Pinned tools (`deploy/k8s/tools.env`)

```
KUSTOMIZE_VERSION=v5.7.1     # sigs.k8s.io/kustomize/kustomize/v5
KUBECONFORM_VERSION=v0.8.0   # github.com/yannh/kubeconform/cmd/kubeconform (module go directive 1.26 = floor)
YQ_VERSION=v4.44.3           # github.com/mikefarah/yq/v4
K8S_SCHEMA_VERSION=1.32.0
```

- `TOOLS=go` (default): `go run <module>@<version>`.
- `TOOLS=docker` (optional): images at the same versions, with `MSYS_NO_PATHCONV=1` on Git Bash.
- `validate.sh` runs `mkdir -p` on the kubeconform `-cache` dir.

## 7. Fixtures and self-test

- YAML fixtures:
  - `testdata/good-local.yaml` and `testdata/good-staging.yaml` must pass.
  - `testdata/bad/<RULE-ID>--<slug>.yaml` must fail.
  - First line of each: `# policy-args: --overlay <o> [--processes <csv>]`.
- Dockerfile fixtures:
  - `testdata/dockerfiles/good/` holds one copy of each real Dockerfile tail (must pass).
  - `testdata/dockerfiles/bad/AC-13--<slug>/` holds exactly one Dockerfile per case (must fail with AC-13).
- A bad fixture passes the self-test only if all of these hold:
  - the checker exits 1;
  - it prints ≥ 1 `FAIL <RULE-ID>` line for the id in its name;
  - it prints no FAIL line for any other id.
- A good fixture passes only on exit 0 with the `policy: OK` line.
- Every id of §2 has ≥ 1 bad fixture. The minimum set for §2.2 and §3 is listed in `plan.md` Round 3 §C.
- `test_policy.sh` exits 0 only if every fixture behaves as above; it prints `selftest: <pass>/<total>`.
- Good fixtures model a real render: generator outputs carry hash-suffixed names (§9), and refs use those names.

## 8. Secret keys per process and overlay (AC-18; replaces the spec E3 table)

Format: env name ← `<Secret base name>/<key>`. No ref is optional, in either overlay.

| Process | Both overlays | local only | staging only |
|---|---|---|---|
| gateway | `JWT_SECRET ← eventflow-auth/JWT_SECRET` | — | — |
| identity | `DATABASE_URL ← eventflow-db/DATABASE_URL_DIRECT`, `JWT_SECRET ← eventflow-auth/JWT_SECRET`, `PII_ENC_KEY`, `PII_HASH_KEY ← eventflow-identity/<same>` | — | `SMTP_USER`, `SMTP_PASS ← eventflow-identity/<same>` |
| event | `DATABASE_URL ← eventflow-db/DATABASE_URL_DIRECT`, `RABBITMQ_URL ← eventflow-mq/AMQP_URL` | — | — |
| waitingroom | `QUEUE_TOKEN_SECRET ← eventflow-auth/QUEUE_TOKEN_SECRET` | — | — |
| waitingroom-controller | — | — | — |
| ticketing | `DATABASE_URL ← eventflow-db/DATABASE_URL_POOLED`, `QUEUE_TOKEN_SECRET ← eventflow-auth/QUEUE_TOKEN_SECRET`, `HMAC_QR_SECRET ← eventflow-qr/HMAC_QR_SECRET` | — | — |
| ticketing-worker | `DATABASE_URL ← eventflow-db/DATABASE_URL_POOLED`, `RABBITMQ_URL ← eventflow-mq/AMQP_URL`, `HMAC_QR_SECRET ← eventflow-qr/HMAC_QR_SECRET` | — | — |
| payment | `DATABASE_URL ← eventflow-db/DATABASE_URL_POOLED`, `RABBITMQ_URL ← eventflow-mq/AMQP_URL` | `VNPAY_TMN_CODE`, `VNPAY_HASH_SECRET`, `MOMO_PARTNER_CODE`, `MOMO_ACCESS_KEY`, `MOMO_SECRET_KEY`, `STRIPE_PUBLISHABLE_KEY`, `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET ← eventflow-payment/<same>` | vnpay + momo keys only (presence = AC-32; Stripe keys or other extras = AC-18) |
| notification | `DATABASE_URL ← eventflow-db/DATABASE_URL_POOLED`, `AMQP_URL ← eventflow-mq/AMQP_URL` | — | `SMTP_USER`, `SMTP_PASSWORD ← eventflow-notification/<same>` |
| ai-worker | `AMQP_URL ← eventflow-mq/AMQP_URL`, `GEMINI_API_KEY ← eventflow-ai/GEMINI_API_KEY` | — | — |
| web | — | — | — |
| migrate Job (dev-infra, local) | `GOOSE_DBSTRING ← eventflow-db/DATABASE_URL_DIRECT` | | |
| dev-infra Postgres/PgBouncer/RabbitMQ (local) | `POSTGRES_PASSWORD`, `RABBITMQ_DEFAULT_PASS ← eventflow-dev-infra/<same>`, as each image needs | | |

- Local Secrets allowed by AC-16: `eventflow-db`, `-mq`, `-auth`, `-qr`, `-identity`, `-payment`, `-ai`,
  `-dev-infra`.
- External Secrets expected in staging (AC-22): `eventflow-db`, `-mq`, `-auth`, `-qr`, `-identity`, `-payment`,
  `-ai`, `-notification`, with the keys above.

## 9. Names and references

- Generated objects may carry a kustomize hash suffix: base name = rendered name minus a trailing `-[a-z0-9]{10}`.
  This applies to local Secrets and to the `eventflow-app-config` / `eventflow-endpoints` ConfigMaps in both
  overlays. AC-16/AC-18/AC-32 compare **base** names.
- AC-22 requires every `secretKeyRef` / `configMapKeyRef` to name an object **exactly as rendered**:
  - local: the suffixed generator name;
  - staging: the plain §8 names (Secrets are external).
- The referenced key must exist, either in the rendered object or, for staging Secrets, among the keys §8 lists for
  that Secret in **any** column. So a Stripe key ref in staging resolves for AC-22, and only AC-18 flags it as an extra. Exception: an optional
  `configMapKeyRef` to a missing key is allowed by AC-22; AC-32 then treats the key as absent.
- `APP_ENV` and `CORS_ALLOWED_ORIGINS` are read only from `eventflow-app-config`. `eventflow-endpoints` holds only
  `REDIS_ADDR`, `REDIS_URL`, `SMTP_HOST`, `SMTP_PORT`, `OTEL_EXPORTER_OTLP_ENDPOINT`.
