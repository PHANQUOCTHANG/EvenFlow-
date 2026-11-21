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

## Round 2

Reviewer: new clean-context plan reviewer. Inputs: `spec.md` (errata E1-E7 and D7 human-approved), `plan.md`
("Round 2" section), `policy-contract.md` (frozen), round 1 above. Read-only checks against the worktree @ f72bad8.
The untracked WP0 files under `deploy/k8s/policy/` were not reviewed.

## Verdict (round 2): **FAIL** (2 major, 0 blocker)

M1, M2 and M3 from round 1 are addressed in substance, and 12 of the 13 minors are resolved (m13 only partly). Two new
major problems sit in the round-2 contract and need fixing before WP0 freezes fixtures:
- R2-M1: the staging "prod-like" guarantee still fails open whenever a key is **absent**, because the service code
  falls back to demo or insecure defaults.
- R2-M2: exemptions apply to a whole rule. That switches off checks the spec meant to keep: the singleton no-HPA
  guard for ticketing-worker, and the whole web security context.

Both fixes are small changes to the contract text plus a few fixtures.

## Round-1 resolutions verified

| Finding | Status | Evidence (checked now) |
|---|---|---|
| M1 interface frozen | **Resolved**, with the gaps listed in R2-M2, r2-m1, r2-m3 and r2-m10 | `policy-contract.md` §2-§7: rule ids, CLI, exit codes, output regex, `# policy-args:` line, one-fixture-one-rule, per-rule annotation. The annotation key `eventflow.io/policy-exempt.AC-9` is a valid qualified name (uppercase is allowed in the name part) |
| M2 offline without Docker | **Resolved** | Ran on this machine (Go 1.27, Git Bash, Docker down), from the repo root where `GOWORK` is set: `go run sigs.k8s.io/kustomize/kustomize/v5@v5.7.1 version` → `v5.7.1`; yq `v4.44.3`; `go run …/kubeconform@v0.8.0 -strict -summary -kubernetes-version 1.32.0 -cache <dir> deploy/k8s/*.yaml` → `Valid: 5, Invalid: 0, Errors: 0, Skipped: 0`. The `@version` form ignores `go.work`. Module `go` directives: kustomize 1.22.7, yq 1.21, **kubeconform v0.8.0 = 1.26**. CI `GO_VERSION` is "1.26" (`ci.yml:28`), so that is OK, but it is the floor. `python3` is available (msys 3.12) |
| M3 demo settings | **Partly resolved.** Present values are now overlay-specific. Absent values still leak, see R2-M1 | spec E2; compose `docker-compose.yml:165-172,214,292-294,309-310,228-229` all covered by the E2 table |
| m1 least privilege | **Resolved.** The E3 table matches the code | env reads: gateway only `JWT_SECRET` (`gateway/internal/config/config.go:72-79`); `QueueTokenSecretFromEnv` only in `ticketing/cmd/server/main.go:123` and `waitingroom/cmd/server/main.go:74`; `RABBITMQ_URL` only in event, payment and ticketing-worker; `AMQP_URL` only in notification and ai-worker; the ticketing server reads no MQ URL; the controller reads only Redis (active set via `libs/go/activeevents`, `controller/main.go:20,66`) |
| m2 single render | Resolved | plan Round 2 CI: one render to `$RUNNER_TEMP/k8s-render`, shared by kubeconform and policy |
| m3 migrate Job | Resolved | `GOOSE_DRIVER`/`GOOSE_DBSTRING` is the same mechanism compose uses (`docker-compose.yml:131-136`). goose with pgx accepts the `postgres://…?sslmode=disable` URL form of `DATABASE_URL_DIRECT` |
| m4 Dockerfile filter | Resolved | adds `'deploy/docker/**'` to the `deploy` filter (`ci.yml:76-79`); the AC-13 grep runs in the third CI step (`--skip-render --skip-kubeconform`) |
| m5 PII keys | Resolved | a 44-char base64 value decodes to 32 bytes only if its last data char is one of `AEIMQUYcgkosw048` before the `=`; the planned python assert catches any mistake |
| m6, m7, m10, m11, m12 | Resolved | — |
| m8 gates namespace | Resolved, but deleting the Job is new, see r2-m6 | `gates.sh:225-270` |
| m9 sizing | Resolved for sizing. The security-context question is open, see r2-m2 | — |
| m13 header | **Not fully.** `plan.md:3-4` still says "D7 and errata E1/E2/E4-E7 pending sign-off", but spec.md:369-371 records approval | — |
| CRLF `.env` | Resolved | `core.autocrlf=true` here. Tracked env files are only `.env.example` files, which `*.env` does not match, so nothing is renormalised. The rule also covers `tools.env`, which bash sources (a CRLF there would put `\r` into the `@version`) |
| test-workflows AC-5 | Still passes | `ac5_k8s` greps the `run` text of any step in a `deploy`-gated job for `kubeconform` and `-strict` (`test-workflows.sh:606-619`). The new step has both. `ac11_no_deploy` only scans for `kubectl apply/create…` without `--dry-run`, and CI has no kubectl. `ac1_no_latest` is not triggered |

## Findings (round 2)

### R2-M1 (major): staging fails open when a key is absent (demo defaults, known secrets)

AC-32 only forbids **values** that are present. E3 marks the payment provider secrets `optional`. The code turns
every absent key into the demo or insecure behaviour:
- **Payment.** `PAYMENT_DEFAULT_PROVIDER` absent → `"sandbox"`. `PAYMENT_ENABLED_PROVIDERS` absent → `sandbox,vnpay,momo,stripe`
  (`services/payment/internal/config/config.go:52-70`). A staging app-config that omits these keys, or reads them
  through `configMapKeyRef optional: true` (E2 allows `optional: true` "where staging omits the key"), passes AC-32
  and re-enables the in-process sandbox, which pays any order with mode `success`.
- **Gemini.** `GEMINI_MOCK` absent → `gemini_mock: bool = True` (`services/ai-worker/app/config.py:35`). Absent passes
  AC-32 ("no `GEMINI_MOCK` other than `"0"`"), so staging silently mocks.
- **Provider secrets.** E3 lists `VNPAY_HASH_SECRET`, `MOMO_SECRET_KEY`, `STRIPE_*` as `optional` "(required in
  staging by D7)". But AC-18 is marked "Overlay-specific: no" in `policy-contract.md` §2, and the plan has no
  staging patch, so "required in staging" cannot be encoded or checked. With `optional: true` and the key missing,
  payment starts with the **public code defaults** (`config.go:82-93`, e.g. `VNPAY_HASH_SECRET` =
  `EVENTFLOW_VNPAY_SECRET_KEY_123456`). The VNPay callback check uses that secret (`vnpay.go:121`), and
  `/v1/payments/webhook/` is a Public gateway route (`gateway/internal/proxy/routes.go:65`). So anyone can forge a
  "paid" callback. A non-optional `secretKeyRef` would instead fail loudly with `CreateContainerConfigError`.
- Related existing debt that D7 now exposes: the Stripe callback accepts a request with **no** signature, and the
  literal `v1=valid_sig` (`stripe.go:122-148`). Enabling `stripe` in staging makes that reachable.

**Fix (contract change request + spec erratum, before WP0 freezes):**
1. AC-32 (staging) also requires these keys to be **present**: `GEMINI_MOCK="0"`, `PAYMENT_DEFAULT_PROVIDER`,
   `PAYMENT_ENABLED_PROVIDERS`. Each must come from an env literal or a non-optional `configMapKeyRef` to an existing key.
   Add a fixture for "key absent in staging".
2. Make AC-18 overlay-specific: in staging, the `secretKeyRef`s for `GEMINI_API_KEY` and for the provider keys of
   every enabled provider are **not** `optional`. Simplest: non-optional in both overlays, with dev values in
   `secrets.dev.env`. Add a fixture.
3. Record the Stripe webhook verification as debt in the runbook (spec §9). Ask the human whether `stripe` stays in
   staging `PAYMENT_ENABLED_PROVIDERS` until it is fixed.

### R2-M2 (major): exemptions at rule level switch off checks the spec keeps

`policy-contract.md` §3 says: "the checker skips that rule for that workload only (and for its HPA/PDB …)". The
rule is the whole AC. This causes two problems:
- **ticketing-worker AC-9.** E1 and contract §3 say the worker "needs" `policy-exempt.AC-9`. But AC-9 itself
  demands "no HPA targets the fixed-replica workloads". That is the only guard against an HPA scaling the singleton
  to 2, the D5 double-reconciler risk. If the exemption is present, the checker must skip AC-9 for the worker, so a
  bad fixture "HPA targeting ticketing-worker" (which carries the annotation the contract says it needs) cannot fail
  with AC-9. The contract contradicts itself, and the D5 guard is lost. The same applies to the waitingroom-controller
  AC-9 entry in spec §7.
- **web AC-12.** Spec §7 exempts web from `readOnlyRootFilesystem` only. A whole-rule AC-12 skip also drops
  `runAsNonRoot`, `capabilities.drop`, `allowPrivilegeEscalation`, seccomp and automount for the public web pod. The
  contract narrows the exemption to a sub-check ("runAsNonRoot only") for dev-infra, but not for web. So WP0 and WP5
  can read it differently, which is the M1 failure mode again.

**Fix (contract change request):**
- Define the scope of each exemption at sub-check level:
  - web `AC-12` → `readOnlyRootFilesystem` only;
  - dev-infra `AC-12` → `runAsNonRoot` only;
  - ticketing-worker `AC-11` → whole rule.
- Remove AC-9 from the exemption pairs for both fixed-replica workloads. AC-9 already defines them as "must have no
  HPA", so nothing needs exempting. A worker or controller carrying `policy-exempt.AC-9` → `FAIL EXEMPT`.
- Say whether section-7 exceptions are realised **only** through annotations (that is, a web pod without the
  annotation fails AC-12), or are hard-coded.
- Add fixtures: "HPA targets ticketing-worker" → AC-9; "web with exemption but `runAsNonRoot` missing" → AC-12.

### Minor (round 2)

| # | Finding | Evidence | Fix |
|---|---|---|---|
| r2-m1 | AC-1 is ambiguous with dev-infra. The contract example counts all Deployments (`10 Deployments, want 11`), but the local render also has Deployments for PgBouncer and Mailpit | contract §5; plan layout `components/dev-infra` | Count only app Deployments (no `eventflow.io/tier: dev-infra` label), and say so in contract §2 |
| r2-m2 | It is not stated which AC-12 checks apply to dev-infra. Official postgres, redis and rabbitmq entrypoints start as root and `gosu`/`chown`, which needs CHOWN/SETUID/SETGID. With `drop: [ALL]` (and only `runAsNonRoot` exemptable) they crash at AC-30 time | spec AC-12 "app pod" vs "No pod uses …"; contract §3 | Contract: list the dev-infra AC-12 sub-checks (host*, hostPath, privileged, `runAsUser: 0`, caps). WP4: run the stores as their image uid (999 postgres/redis/rabbitmq, 70 pgbouncer) with `fsGroup`, plus `emptyDir` for `/var/run/postgresql`, `/tmp` and `/etc/pgbouncer`. Then no root exemption is needed |
| r2-m3 | Generator hash suffixes. In local, `secretGenerator`/`configMapGenerator` rename `eventflow-db` → `eventflow-db-<hash>` (and `eventflow-app-config-<hash>` in both overlays). AC-16 "allowed names", AC-18 `secret/key` and AC-22 compare names, and fixtures will probably use plain names | kustomize default | Contract: names match `^<name>(-[a-z0-9]{10})?$`, or overlays set `disableNameSuffixHash` (but then pods do not roll when a Secret changes). Pick one |
| r2-m4 | kubeconform `-cache` fails if the directory does not exist. Verified: `failed opening cache folder …: The system cannot find the file specified`, exit 1 | run above | `mkdir -p` the cache dir in `validate.sh` |
| r2-m5 | `actions/setup-go@v5` "module cache on": the repo has no root `go.sum` (only `go.work`, `go.work.sum`), and the tool modules are not in any `go.sum`. The cache either warns or misses | `ls go.*`; `ci.yml:90-93` | `cache-dependency-path: deploy/k8s/tools.env` |
| r2-m6 | gates C4.4 now **deletes** `job/eventflow-migrate` on whatever kube-context is current, which could be a shared cluster, mid-migration | plan Round 2 m3/m8 | Delete only when the context is `docker-desktop` or `kind-*`, or only a Job with `status.succeeded`. Otherwise print that the immutable-Job error is expected |
| r2-m7 | `validate.sh` flags do not cover steps 5-6 (AC-13 grep, self-test), so the CI "render" step (`--skip-kubeconform --skip-policy`) also runs the self-test | plan Round 2 `validate.sh` block | `--skip-policy` skips 4-6, or add `--render-only` |
| r2-m8 | Per-environment keys and secrets the code reads are in neither E2 nor E3: notification `SMTP_USER`/`SMTP_PASSWORD`/`SMS_API_KEY` (`notification/internal/config/config.go:31-36`, note `SMTP_PASSWORD` vs identity `SMTP_PASS`); payment `VNPAY_RETURN_URL`, `MOMO_RETURN_URL`/`MOMO_NOTIFY_URL` (an empty IPN URL means MoMo can never complete in staging), `STRIPE_SUCCESS/CANCEL_URL`, `STRIPE_PUBLISHABLE_KEY` (matches the AC-17 `_KEY$` name pattern, so it can only come from a Secret); identity `GOOGLE_CLIENT_ID`; notification `SMTP_FROM` `.local` | env grep over services | Add them to E2/E3 (staging placeholders), or list them as known limits in the runbook |
| r2-m9 | AC-13 negative path is untested: the contract excludes AC-13 from fixtures, so the mutation "disable the AC-13 grep" survives | contract §7, spec AC-25 | Give the AC-13 function a directory argument and add one bad Dockerfile fixture to the self-test |
| r2-m10 | Where `APP_ENV`/`CORS_ALLOWED_ORIGINS` live: spec 4.1 says `eventflow-endpoints`, E2 says `eventflow-app-config`, plan Round 2 says endpoints is "infra hosts only". AC-19/AC-22 fixtures need one answer | spec 4.1, E2 | State in the contract: `eventflow-app-config` only |
| r2-m11 | A staging Redis with a password cannot be wired: only identity reads `REDIS_PASSWORD`, and a `redis://:pw@` URL in a ConfigMap breaks AC-17 | env grep; spec AC-17 | Runbook known limit: staging Redis must be private and passwordless (or TLS/ACL later) |
| r2-m12 | yq turns an empty YAML document (trailing `---`) into `null` in the array. The contract does not say whether that is input error (exit 2) or ignored | contract §4 | Checker ignores `null` entries; state it |
| r2-m13 | `POSTGRES_PASSWORD` (`eventflow-dev-infra`) must equal the password inside the `eventflow-db` URLs. That is two copies in one file, and nothing checks them | plan §4, E3 | One comment in `secrets.dev.env`. AC-30 proves it at runtime |

## Required for round 3

Fix R2-M1 and R2-M2 as a `policy-contract.md` change request plus a short spec erratum (E8). Both need human
approval **before WP0 freezes fixtures**, so tell the WP0 agent now. Minors can be fixed during implementation;
r2-m1, r2-m3 and r2-m10 change fixture expectations, so put them into the same contract change.

## Round 3

Reviewer: new clean-context plan reviewer (last allowed round). Inputs: `spec.md` (E1-E8, D7 and the
Stripe-disabled decision, all human-approved), `policy-contract.md` revision 2, `plan.md` "Round 3", rounds 1-2
above. Read-only checks against the worktree @ 505de06. The WP0 working-tree changes under `deploy/k8s/policy/`
(uncommitted) were looked at only to confirm fixture names. They were not reviewed. Docker is still down, so image
uids are checked against the plan's "read the uid from the image" rule, not by running the images.

## Verdict (round 3): **PASS** (0 blocker, 0 major, 6 minor)

R2-M1 and R2-M2 are resolved in the contract and checked against the code. The round-2 minors are resolved. The
contract is precise enough that each C3 fixture trips exactly one rule; I traced every one of them below. The
author's extra choices ("no optional secret refs in local either", `EMAIL_PROVIDER=smtp`, required callback URLs in
staging) are in the approved E8 text and match its fail-closed intent, so they are not scope creep. The minors below
can be fixed during implementation. r3-m1 is the only one worth folding into the contract before WP0 freezes.

## Major resolutions verified against the code

| Finding | Status | Evidence (checked now) |
|---|---|---|
| R2-M1 payment | **Resolved** | `PAYMENT_DEFAULT_PROVIDER` absent → `sandbox`; `PAYMENT_ENABLED_PROVIDERS` absent/empty → all four (`services/payment/internal/config/config.go:52-70`). `NewRegistry` turns an invalid default into `sandbox` and always enables the default (`internal/adapter/gateway/factory.go:20-38`). Contract §2.2 requires both keys **present**, the list = {vnpay,momo}, and the default in that list, so all three fallbacks are closed. Webhooks resolve through `registry.Get`, which rejects disabled providers (`internal/app/process_webhook.go:67`, `factory.go:58-60`). So with Stripe disabled, the unsigned / `v1=valid_sig` Stripe callback (`stripe.go:122-148`) is unreachable in staging. The debt is in E8.3 and plan §D |
| R2-M1 provider secrets | **Resolved** | `getEnvOrDefault` falls back to public defaults on absent **or empty** (`config.go:82-99`). Contract §8 + AC-18 forbid `optional` on every `secretKeyRef`, and AC-32 requires the vnpay/momo keys present in staging. So a missing key fails as `CreateContainerConfigError`. An empty value inside the external Secret is not checkable offline (see r3-m5) |
| R2-M1 Gemini | **Resolved** | `gemini_mock: bool = True` (`services/ai-worker/app/config.py:35`). AC-32 requires `GEMINI_MOCK="0"` present. The validator (`config.py:69-73`) then refuses to start without `GEMINI_API_KEY`, and that ref is non-optional |
| R2-M1 notification | **Resolved** | `EMAIL_PROVIDER` default `mock` (`services/notification/internal/config/config.go:28`), and only the exact string `smtp` selects SMTP (`cmd/server/main.go:66`). AC-32 requires `"smtp"` exactly. The SMTP sender authenticates only when user and password are both non-empty (`adapter/sender/smtp.go:43-45`), so local (no creds, Mailpit) still works |
| R2-M1 identity SMTP | Consistent | identity uses SMTP whenever `SMTP_HOST` is set, with user/pass if given (`services/identity/cmd/server/main.go:96-102`; `SMTP_PASS`, not `SMTP_PASSWORD`, `config.go:78-79`). Contract §8 uses the right names per service |
| R2-M2 | **Resolved** | §2.1 splits AC-12 into 12 named sub-checks. §3 has a closed allow table, AC-9 is not exemptable, an invalid exemption has no effect, and nothing is hard-coded. `HPA-managed` is a static list (spec §4, line 66), so "HPA on controller/worker" cannot also trip AC-7 |

## Round-2 minors verified

| # | Status | Notes |
|---|---|---|
| r2-m1 | Resolved | AC-1 counts app Deployments only. `good-local` pins it with Mailpit |
| r2-m2 | Resolved, see r3-m4 | Run as the image uid, `dropAll` kept, no cap exemption. Official entrypoints skip `chown`/`su-exec` when not uid 0 (postgres, redis, rabbitmq), so drop ALL is fine. postgres:16-**alpine** = 70:70 is correct (the debian variant is 999). The plan's table is only a hint, but two values differ from the images: redis:7-alpine = uid 999 / **gid 1000**, and rabbitmq:3.13-*-alpine = **100:101**. The rule "WP4 reads uid and gid from the image" covers both |
| r2-m3 | Resolved | Kustomize's suffix is 10 chars from a subset of `[0-9a-z]` (hex with 0/1/3/a/e remapped), so `-[a-z0-9]{10}` matches. No real base name ends in a 10-char segment, so stripping cannot misfire. Kustomize rewrites `secretKeyRef`/`configMapKeyRef` names only for objects generated in the same render. That matches §9 exactly: suffixed names in local, plain external names in staging |
| r2-m4, r2-m5, r2-m7, r2-m9, r2-m10, r2-m11, r2-m12, r2-m13, r1-m13 | Resolved | — |
| r2-m6 | Resolved | The guard deletes only on `docker-desktop`/`kind-*` **and** only a Job with `succeeded ≥ 1`. Otherwise it dry-runs with `-l 'app.kubernetes.io/name!=eventflow-migrate'`; objects without the label (Namespace etc.) still match `!=`. `kubectl apply -k … -l` filters the rendered objects. Note: the `-l` branch alone would always be enough, so the delete is optional; it is harmless as guarded |
| r2-m8 | Resolved, see r3-m1 | — |

## One fixture → one rule (C3 traced)

Every C3 fixture trips only its named rule under the contract text:
- `AC-32--momo-secret-missing` is not AC-18, because of the staging presence exception.
- `AC-18--staging-stripe-key-extra` is not AC-22 (the key resolves via "any column"), and not AC-32 (only ConfigMap or literal `STRIPE_*` counts).
- `AC-22--ref-uses-base-name-local` is not AC-18, because AC-18 compares base names.
- `*-absent` fixtures are not AC-22, because an optional ref to a missing key is allowed.
- `EXEMPT--web-ac12-whole-rule`: the valid sub-check key keeps AC-12 green.
- `EXEMPT--worker-ac9` has no HPA.
- `AC-12--dev-infra-*` trip one sub-check each.

Latent overlap, which no fixture uses: in staging, an `optional: true` provider `secretKeyRef` trips both AC-18
(no optional) and AC-32 (not present). Do not add such a fixture, or name it under both ids.

## Findings (round 3), all minor

| # | Finding | Evidence | Fix |
|---|---|---|---|
| r3-m1 | **`GOOGLE_CLIENT_ID` fails open and nothing guards it.** `idtoken.Validate` skips the audience check when the audience is empty. Absent or empty `GOOGLE_CLIENT_ID` in staging therefore makes identity accept a Google ID token minted for **any** OAuth client (token-confusion login as the token's verified email). The planned staging value is non-empty, so the planned config fails closed (`good-staging.yaml` line 7). But AC-32 does not require the key, so a later edit that drops it passes the policy. Same class as R2-M1, though not in E8's list | `services/identity/internal/app/google_login.go:48`; `google.golang.org/api/idtoken/validate.go:109,160` ("If audience is not empty the audience claim … is validated") | Preferred: add identity `GOOGLE_CLIENT_ID` (present, non-empty) to contract §2.2, plus fixture `AC-32--google-client-id-absent`, in the same change request before WP0 freezes. It needs a one-line human OK. Otherwise: runbook known limit + a code ticket (identity must refuse an empty client id when `APP_ENV=production`) |
| r3-m2 | Approval wording differs. The human's approval line says "no optional refs **in staging**". E8.2 (labelled as the approved text) says both overlays, and plan Round 3 §A calls the plan "stricter than E8's 'staging'", which misquotes E8.2. The substance is consistent and fail-closed: local just needs dev values for all 8 payment keys, and nothing in the repo depends on the public default secrets (grep finds them only in `config.go`) | spec.md:393-394 vs :435; plan.md Round 3 §A | Coordinator: confirm with the human in one line that "both overlays" is intended, and fix the §A sentence |
| r3-m3 | AC-32 does not say how values are compared. The code lowercases and trims each provider (`domain/payment.go:19-21`). The contract only says "order-insensitive". `"vnpay, momo"` or `"VNPay"` would work at runtime but could fail a strict checker | contract §2.2 | State: split on `,`, trim, lowercase, then the set must equal {vnpay,momo}; normalise the default the same way. `GEMINI_MOCK`/`EMAIL_PROVIDER` stay exact-match (stricter than the code, which is fine) |
| r3-m4 | Dev-infra PVC permissions with no root allowed. (a) Docker Desktop's `hostpath` StorageClass backs PVs with hostPath, where kubelet does **not** apply `fsGroup`. The data dir is then writable only if the provisioner creates it world-writable. (b) Where `fsGroup` is applied (kind local-path), the default `fsGroupChangePolicy: Always` adds group rw on each mount. That makes RabbitMQ's `.erlang.cookie` group-readable, and RabbitMQ refuses to start on the second boot. The contract forbids the usual fix (a root `chown` init container: `rootUid`, `dropAll`). The plan's fallback ("swap image") does not help here | kubelet volume ownership behaviour; contract §2.1 dev-infra column | WP4: set `fsGroupChangePolicy: OnRootMismatch`. Prove AC-30 with a pod **restart**, not only the first start. Record the fallback now: if the hostpath dir is not writable, use `emptyDir` for local data (dev data loss accepted) rather than a new exemption |
| r3-m5 | Present ≠ non-empty for Secret values. Payment's `getEnvOrDefault` and notification's `getEnv` treat an **empty** value as absent, so an empty key in the external staging Secret still falls back to public defaults. This cannot be checked offline | `payment/internal/config/config.go:95-99` | Runbook: staging Secret values must be non-empty; list the keys whose emptiness falls back to public defaults (VNPay/MoMo, `EMAIL_PROVIDER`) |
| r3-m6 | Staging requires SMTP credentials for both identity and notification. An internal relay without auth still needs dummy values, and then both clients try `AUTH`, which such a relay may reject | `identity/cmd/server/main.go:96-99`, `notification/.../smtp.go:43-45` | Runbook: staging needs an authenticating SMTP provider, and the same credentials go into two Secrets |

## Outcome

PASS. The plan may proceed to WP0 freeze and implementation. Recommended before the freeze: r3-m1 (one contract line
plus one fixture, human OK) and the r3-m2 confirmation. All other minors go into WP4 and the runbook.
