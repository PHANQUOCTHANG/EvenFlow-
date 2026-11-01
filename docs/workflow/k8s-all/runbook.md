# Runbook: EventFlow on Kubernetes (k8s-all)

Closes AC-29. Spec: `spec.md` (errata E1-E8), policy: `policy-contract.md` rev 2, plan: `plan.md` (Round 3).
Run every command from the **repo root** in **Git Bash** (or any bash). Section 9 lists what is **not verified**;
read it before reporting "deployed".

```
deploy/k8s/
  base/                         apps, namespace, NetworkPolicies -- NO image tags, NO ConfigMaps/Secrets
  components/dev-infra/         Postgres, PgBouncer, Redis, RabbitMQ, Mailpit, migrate Job (local only)
  components/dev-infra/ephemeral-data/   emptyDir instead of PVCs (section 3.4)
  overlays/local/               base + dev-infra + dev config + dev Secrets (secrets.dev.env, DEV ONLY)
  overlays/staging/             base + staging config; Secrets are external (section 4.2)
  validate.sh, tools.env        offline validation, pinned tools
```

Requirements: Kubernetes **>= 1.30** (`preStop.sleep` is used by every Go/Python pod; it is alpha and off in 1.29,
spec E5). `kubectl` >= 1.30. Go (any recent version) for the pinned tools, or `TOOLS=docker`.

## 0. Rules that are easy to get wrong

1. **Apply only an overlay** (`overlays/local` or `overlays/staging`, or a temporary overlay on top of one,
   section 2). **Never** `kubectl apply -k deploy/k8s/base`: the base has no image tags (pods would pull `:latest`
   names that do not exist), no `eventflow-app-config` / `eventflow-endpoints`, no Secrets and no PSA level.
2. The committed overlays carry the placeholder tag `sha-c5fb598`. **No image is built from that commit.** The real
   tag is set by a temporary overlay (section 2); never edit committed files to change a tag or a host.
3. `kubectl delete -k <overlay>` **deletes Namespace `eventflow`** and everything in it, including the web slice and
   the dev PVC data. See section 6.3 for the label-based delete that keeps the namespace.
4. Before applying anything, the manifests must pass offline validation:

```bash
bash deploy/k8s/validate.sh          # both overlays render, kubeconform -strict, policy, AC-13, self-test
# expect last line: validate: OK
```

The first run needs network (Go modules and JSON schemas); afterwards everything is cached in
`~/.cache/eventflow-k8s/`. The pinned kustomize binary used below is then at:

```bash
KUSTOMIZE="$(ls ~/.cache/eventflow-k8s/bin/kustomize-v5.7.1/kustomize* | head -1)"
"$KUSTOMIZE" version     # expect v5.7.1
```

## 1. Build every image at one commit

All images come from a clean export of **one** commit and get the tag `sha-<short>` (7 hex). Never build from the
working tree: it may contain uncommitted changes, and the tag would then lie (this already happened in the web
slice, see `docs/workflow/deploy-k8s-web/runbook.md` section 1).

```bash
SHA=$(git rev-parse --short=7 HEAD)      # or the commit you want to deploy
TAG=sha-$SHA
R=ghcr.io/phanquocthang
SRC=$(mktemp -d)
git archive "$SHA" | tar -x -C "$SRC"

docker build -f "$SRC/deploy/docker/web.Dockerfile" \
  --build-arg NEXT_PUBLIC_API_BASE=http://api.eventflow.localtest.me -t "$R/eventflow-web:$TAG" "$SRC"
for s in gateway identity event payment notification; do
  docker build -f "$SRC/deploy/docker/go-service.Dockerfile" --build-arg SERVICE=$s -t "$R/eventflow-$s:$TAG" "$SRC"
done
docker build -f "$SRC/deploy/docker/waitingroom.Dockerfile" -t "$R/eventflow-waitingroom:$TAG" "$SRC"   # also /controller
docker build -f "$SRC/deploy/docker/ticketing.Dockerfile"   -t "$R/eventflow-ticketing:$TAG"   "$SRC"   # also /worker
docker build -f "$SRC/deploy/docker/ai-worker.Dockerfile"   -t "$R/eventflow-ai-worker:$TAG"   "$SRC"
docker build -f "$SRC/deploy/docker/migrate.Dockerfile"     -t "$R/eventflow-migrate:$TAG"     "$SRC"   # local only
```

`NEXT_PUBLIC_API_BASE` must be a `--build-arg`: Next inlines it at build time; a runtime env has no effect.
For staging, build web with the staging API host instead (`https://api.<your staging host>`).

Every image must run as a **numeric** user (pods have `runAsNonRoot: true`; the kubelet only reads `Config.User`):

```bash
for i in web gateway identity event payment notification waitingroom ticketing ai-worker migrate; do
  printf '%-14s ' $i; docker image inspect "$R/eventflow-$i:$TAG" --format '{{.Config.User}}'
done
# expect: web 1000, ai-worker 10001, all others 65532:65532
```

## 2. Load into the cluster and set the tag (temporary overlay)

- **Docker Desktop Kubernetes**: no load step; it shares the Docker image store (`imagePullPolicy: IfNotPresent`).
- **kind**: `for i in web gateway identity event payment notification waitingroom ticketing ai-worker migrate; do kind load docker-image "$R/eventflow-$i:$TAG"; done`
- **staging**: push the images to a registry the cluster can pull from (only web is published by CI today).

The tag is set in a temporary overlay **inside** `deploy/k8s/overlays/` (kustomize refuses absolute resource
paths, and a temp dir on another drive cannot be referenced relatively). It is never committed:

```bash
OVERLAY=local                         # or staging
RUN=$(mktemp -d -p deploy/k8s/overlays run.XXXXXX)
trap 'rm -rf "$RUN"' EXIT             # in an interactive shell: remember to rm -rf "$RUN" at the end
printf 'apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n  - ../%s\n' "$OVERLAY" > "$RUN/kustomization.yaml"
IMAGES="web gateway identity event waitingroom ticketing payment notification ai-worker"
[ "$OVERLAY" = local ] && IMAGES="$IMAGES migrate"
(cd "$RUN" && for i in $IMAGES; do "$KUSTOMIZE" edit set image "$R/eventflow-$i=$R/eventflow-$i:$TAG"; done)
"$KUSTOMIZE" build "$RUN" | grep 'image: ghcr' | sort | uniq -c     # every line must show $TAG
```

Optional additions to the same `$RUN/kustomization.yaml` (all verified to render with kustomize v5.7.1):

```yaml
components:                                   # local only, see section 3.4
  - ../../components/dev-infra/ephemeral-data
configMapGenerator:                           # staging: real hosts, see section 4.1
  - name: eventflow-app-config
    behavior: merge
    envs: [app-config.site.env]
  - name: eventflow-endpoints
    behavior: merge
    envs: [endpoints.site.env]
```

## 3. Local cluster (Docker Desktop Kubernetes or kind)

### 3.1 Preconditions

```bash
kubectl config current-context     # docker-desktop or kind-<name>
kubectl version                    # server >= v1.30
```

Docker Desktop has **no ingress controller, no metrics-server and no NetworkPolicy enforcement** (section 9).

### 3.2 Apply

```bash
# a finished migrate Job is immutable; delete it first (only on a local context!)
kubectl -n eventflow delete job eventflow-migrate --ignore-not-found
# server-side dry run first (creates the namespace for real so NamespaceLifecycle does not reject the rest)
kubectl get ns eventflow >/dev/null 2>&1 || kubectl create namespace eventflow
"$KUSTOMIZE" build "$RUN" | kubectl apply --dry-run=server -f -
# then for real
"$KUSTOMIZE" build "$RUN" | kubectl apply -f -
```

`bash .github/scripts/gates.sh` (C4.4) runs the same dry run against `overlays/local` when a cluster is reachable.
It deletes `job/eventflow-migrate` only on `docker-desktop` / `kind-*` and only if the Job succeeded; otherwise
it leaves the Job out of the dry run and says so.

### 3.3 Check readiness

```bash
kubectl -n eventflow wait --for=condition=complete job/eventflow-migrate --timeout=5m
kubectl -n eventflow rollout status statefulset/eventflow-postgres --timeout=5m
kubectl -n eventflow rollout status statefulset/eventflow-rabbitmq --timeout=5m
for d in $(kubectl -n eventflow get deploy -o name); do kubectl -n eventflow rollout status "$d" --timeout=5m; done
kubectl -n eventflow get pods -o wide        # every pod Running and READY n/n
kubectl -n eventflow get hpa                 # TARGETS <unknown> on Docker Desktop (no metrics-server)
```

If a pod is stuck in `CreateContainerConfigError`, a referenced Secret/ConfigMap key is missing (no ref is
optional on purpose). `kubectl -n eventflow describe pod <pod>` names the key.

### 3.4 Docker Desktop hostpath and fsGroup

Postgres (uid 70) and RabbitMQ (uid 100) write to PVCs and rely on `fsGroup`. The Docker Desktop `hostpath`
StorageClass does **not** apply `fsGroup`, so the data directory may not be writable: Postgres logs
`could not change permissions of directory` / RabbitMQ crash-loops on `/var/lib/rabbitmq`. Fix without root or extra
capabilities: add the `ephemeral-data` component to the temporary overlay (section 2). The data lives in `emptyDir`
and is **lost** when the pod is deleted. `volumeClaimTemplates` are immutable, so delete the StatefulSets first:

```bash
kubectl -n eventflow delete statefulset eventflow-postgres eventflow-rabbitmq
kubectl -n eventflow delete pvc -l app.kubernetes.io/part-of=eventflow    # optional, frees the old volumes
"$KUSTOMIZE" build "$RUN" | kubectl apply -f -
```

### 3.5 Access (port-forward; no ingress controller on Docker Desktop)

```bash
kubectl -n eventflow port-forward svc/eventflow-gateway 8080:8080 &
curl -fsS http://localhost:8080/healthz                     # expect 200
kubectl -n eventflow port-forward svc/eventflow-web 3000:80 &
curl -fsS http://localhost:3000/api/healthz                 # expect 200
kubectl -n eventflow port-forward svc/eventflow-mailpit 8025:8025 &   # dev mail UI
```

The web bundle calls `http://api.eventflow.localtest.me`; without an ingress controller that host does not answer.
With ingress-nginx installed, both `eventflow.localtest.me` and `api.eventflow.localtest.me` resolve to 127.0.0.1.

## 4. Staging

Staging has no dev-infra: Postgres, PgBouncer, Redis, RabbitMQ and SMTP are external. PSA `enforce=restricted`.

### 4.1 Placeholders you must replace

The committed staging values are placeholders. Replace them through `app-config.site.env` /
`endpoints.site.env` in the temporary overlay (section 2), never by editing the committed files:

| File / object | Key | Placeholder | Replace with |
|---|---|---|---|
| `app-config.env` | `CORS_ALLOWED_ORIGINS` | `https://staging.example.com` | the staging web origin |
| `app-config.env` | `VNPAY_RETURN_URL`, `MOMO_RETURN_URL` | `https://staging.example.com/` | staging web return page |
| `app-config.env` | `MOMO_NOTIFY_URL` | `https://api.staging.example.com/v1/payments/webhook/momo` | a **public** API host: MoMo IPN calls it from the internet |
| `app-config.env` | `SMTP_FROM` | `no-reply@staging.example.com` | a sender the SMTP server accepts |
| `app-config.env` | `GOOGLE_CLIENT_ID` | `REPLACE-ME-staging.apps.googleusercontent.com` | the staging Google OAuth client id |
| `endpoints.env` | `REDIS_ADDR`, `REDIS_URL` | `redis.example.internal:6379` | the private Redis (see 4.3) |
| `endpoints.env` | `SMTP_HOST`, `SMTP_PORT` | `smtp.example.internal`, `587` | the real SMTP server |
| `endpoints.env` | `OTEL_EXPORTER_OTLP_ENDPOINT` | empty (no traces) | an OTLP HTTP collector, if any |
| Ingress `eventflow-web` / `eventflow-gateway` | `host` | `eventflow.localtest.me`, `api.eventflow.localtest.me` | staging hosts, via a `patches:` entry in the temporary overlay (the policy pins the committed hosts, AC-23) |

Keep `GEMINI_MOCK=0`, `EMAIL_PROVIDER=smtp`, `PAYMENT_ENABLED_PROVIDERS=vnpay,momo` and
`PAYMENT_DEFAULT_PROVIDER=vnpay` (AC-32). Do not add `MIN_SALE_LEAD_SECONDS`, `ACTIVE_EVENT_IDS`, `ADMIT_*`,
`PAYMENT_SANDBOX_MODE` or any `STRIPE_*` key.

### 4.2 External Secrets (create before applying)

The staging overlay renders **no** Secret (AC-16). Create these in namespace `eventflow` (contract §8); names and
keys must match exactly. Values come from your secret store; the commands below read them from shell variables so
they do not land in shell history as literals.

| Secret | Keys |
|---|---|
| `eventflow-db` | `DATABASE_URL_DIRECT`, `DATABASE_URL_POOLED` |
| `eventflow-mq` | `AMQP_URL` |
| `eventflow-auth` | `JWT_SECRET`, `QUEUE_TOKEN_SECRET` |
| `eventflow-qr` | `HMAC_QR_SECRET` |
| `eventflow-identity` | `PII_ENC_KEY`, `PII_HASH_KEY` (each base64 of 32 bytes), `SMTP_USER`, `SMTP_PASS` |
| `eventflow-payment` | `VNPAY_TMN_CODE`, `VNPAY_HASH_SECRET`, `MOMO_PARTNER_CODE`, `MOMO_ACCESS_KEY`, `MOMO_SECRET_KEY` |
| `eventflow-ai` | `GEMINI_API_KEY` |
| `eventflow-notification` | `SMTP_USER`, `SMTP_PASSWORD` |

```bash
kubectl get ns eventflow >/dev/null 2>&1 || kubectl create namespace eventflow
kubectl -n eventflow create secret generic eventflow-db \
  --from-literal=DATABASE_URL_DIRECT="$DATABASE_URL_DIRECT" --from-literal=DATABASE_URL_POOLED="$DATABASE_URL_POOLED"
kubectl -n eventflow create secret generic eventflow-mq --from-literal=AMQP_URL="$AMQP_URL"
kubectl -n eventflow create secret generic eventflow-auth \
  --from-literal=JWT_SECRET="$JWT_SECRET" --from-literal=QUEUE_TOKEN_SECRET="$QUEUE_TOKEN_SECRET"
kubectl -n eventflow create secret generic eventflow-qr --from-literal=HMAC_QR_SECRET="$HMAC_QR_SECRET"
kubectl -n eventflow create secret generic eventflow-identity \
  --from-literal=PII_ENC_KEY="$PII_ENC_KEY" --from-literal=PII_HASH_KEY="$PII_HASH_KEY" \
  --from-literal=SMTP_USER="$SMTP_USER" --from-literal=SMTP_PASS="$SMTP_PASSWORD"
kubectl -n eventflow create secret generic eventflow-payment \
  --from-literal=VNPAY_TMN_CODE="$VNPAY_TMN_CODE" --from-literal=VNPAY_HASH_SECRET="$VNPAY_HASH_SECRET" \
  --from-literal=MOMO_PARTNER_CODE="$MOMO_PARTNER_CODE" --from-literal=MOMO_ACCESS_KEY="$MOMO_ACCESS_KEY" \
  --from-literal=MOMO_SECRET_KEY="$MOMO_SECRET_KEY"
kubectl -n eventflow create secret generic eventflow-ai --from-literal=GEMINI_API_KEY="$GEMINI_API_KEY"
kubectl -n eventflow create secret generic eventflow-notification \
  --from-literal=SMTP_USER="$SMTP_USER" --from-literal=SMTP_PASSWORD="$SMTP_PASSWORD"
```

**Every value must be non-empty.** A missing Secret or key stops the pod (`CreateContainerConfigError`, no ref is
optional), but an **empty** value is passed to the process as `""`, and the services then fall back to their
**public code defaults** (dev JWT/queue/QR secrets, sandbox payment credentials). Check for empty values before
applying:

```bash
for s in eventflow-db eventflow-mq eventflow-auth eventflow-qr eventflow-identity eventflow-payment eventflow-ai eventflow-notification; do
  kubectl -n eventflow get secret "$s" -o go-template='{{range $k, $v := .data}}{{if not $v}}{{printf "EMPTY %s\n" $k}}{{end}}{{end}}' \
    | sed "s/^/$s: /"
done     # expect no output
```

### 4.3 Staging infrastructure assumptions

- **Redis must be private and passwordless.** No TLS and no ACL/password are wired in this task (r2-m11): put Redis
  on a network only the cluster can reach.
- **SMTP must accept logins.** identity and notification always authenticate with `SMTP_USER` and
  `SMTP_PASS`/`SMTP_PASSWORD` on `SMTP_HOST:SMTP_PORT`; an open relay without AUTH is not supported.
- **MoMo IPN** needs `MOMO_NOTIFY_URL` on a host reachable from the internet (the gateway Ingress).
- **Stripe is disabled in staging** (human decision, spec E8.3). Security debt: the payment Stripe webhook accepts
  unsigned requests and the literal `v1=valid_sig` (`services/payment/internal/adapter/gateway/stripe.go:122-148`).
  Re-enable Stripe in staging only after that is fixed.
- `SMS_PROVIDER` stays `mock` (no SMS provider in scope).
- An ingress-nginx controller in namespace `ingress-nginx` (the NetworkPolicies allow web/gateway traffic only
  from there).

### 4.4 Apply

```bash
"$KUSTOMIZE" build "$RUN" | kubectl apply --dry-run=server -f -     # AC-31
"$KUSTOMIZE" build "$RUN" | kubectl apply -f -
for d in $(kubectl -n eventflow get deploy -o name); do kubectl -n eventflow rollout status "$d" --timeout=5m; done
```

## 5. Logs and day-2

```bash
kubectl -n eventflow logs deploy/eventflow-gateway --tail=100
kubectl -n eventflow logs job/eventflow-migrate                  # local
kubectl -n eventflow get events --sort-by=.lastTimestamp | tail -30
```

New tag: build (section 1), recreate the temporary overlay with the new `TAG` (section 2), apply again.
Local migrations run in the migrate Job; delete the finished Job before applying a new tag (section 3.2).

## 6. Rollback

### 6.1 One Deployment

```bash
kubectl -n eventflow rollout history deploy/eventflow-gateway
kubectl -n eventflow rollout undo deploy/eventflow-gateway            # previous ReplicaSet
kubectl -n eventflow rollout status deploy/eventflow-gateway
```

`ticketing-worker` uses `Recreate` (singleton, D5): a rollout or undo leaves a gap of a few seconds in sweeping.

### 6.2 Manifest change

```bash
git revert <commit>          # on a branch, through a PR
# then rebuild the temporary overlay from the reverted tree and apply it again (sections 2 and 3.2 / 4.4)
```

### 6.3 Remove everything

`kubectl delete -k deploy/k8s/overlays/local` (or a build of the temporary overlay piped to `kubectl delete -f -`)
**deletes Namespace `eventflow`**, together with the web slice and the dev PVC data. To remove the EventFlow objects
but keep the namespace, delete by label (every rendered object carries `app.kubernetes.io/part-of=eventflow`):

```bash
kubectl -n eventflow delete deploy,sts,job,svc,ingress,hpa,pdb,networkpolicy,cm,secret -l app.kubernetes.io/part-of=eventflow
kubectl -n eventflow delete pvc -l app.kubernetes.io/part-of=eventflow     # only if the dev data may go
```

On staging the second command does nothing (no PVCs) and the first one also deletes the external Secrets only if
they carry that label (the commands in 4.2 do not add it, so they survive).

NetworkPolicy lockout recovery: `kubectl -n eventflow delete networkpolicy --all`.

## 7. Validation and CI

| Where | What runs |
|---|---|
| `bash deploy/k8s/validate.sh` | render both overlays (pinned kustomize) → kubeconform `-strict` → policy on both renders → AC-13 on `deploy/docker` → policy self-test |
| `bash deploy/k8s/policy/test_policy.sh` | policy self-test plus checks on the real tree (section E) |
| CI `validate-deploy` | same steps: render once into `$RUNNER_TEMP/k8s-render`, kubeconform on those files, then policy + self-test; runs on changes to `deploy/k8s/**`, `deploy/docker/**`, `deploy/compose/**`, `deploy/observability/**` |
| `bash .github/scripts/gates.sh` C4.4 | `kubectl apply --dry-run=server` of `overlays/local` when a cluster is reachable; otherwise says nothing was checked |

## 8. Assumptions

- Kubelet probes are not blocked by NetworkPolicy on mainstream CNIs (Calico, Cilium); default-deny ingress stays.
- Monitoring namespaces are those labelled `eventflow.io/monitoring: "true"`; none exists yet.
- Egress is not restricted.

## 9. Not verified, known limits and debt

**Not verified in this task (Docker was off at integration, 2026-11-01):**

- **AC-30 NOT VERIFIED**: `overlays/local` has not been applied to a real cluster; the migrate Job, pod readiness,
  gateway `/healthz` via port-forward, PDB behaviour on pod deletion and NetworkPolicy isolation are unproven.
  On Docker Desktop, NetworkPolicy isolation cannot be verified at all (no enforcement).
- **AC-31 NOT VERIFIED**: `kubectl apply --dry-run=server` of `overlays/staging` has not been run.
- Image builds of all services at one commit (section 1) and the uid of each built image have not been run in this
  task; AC-13 is checked statically on the Dockerfiles.
- Everything else is checked offline by `validate.sh` (render, schema, policy) and by CI.

**Known limits and debt (spec §9 and E8):**

- identity, event, payment, notification: readiness = liveness (no `/readyz`); a pod that lost its DB stays in
  rotation.
- identity has no graceful shutdown: the preStop sleep drains endpoints, but in-flight requests at SIGTERM are cut.
- Worker probes (`waitingroom-controller`, `ticketing-worker`) hit `/metrics`: they prove the process serves
  HTTP, not that the work loop is alive.
- Web SSR in the cluster calls the public API host, which may not resolve inside a pod; client-side fetches work.
- Docker Desktop: no metrics-server (HPA `<unknown>`, minReplicas still applied), no ingress controller, no
  NetworkPolicy enforcement; `hostpath` ignores `fsGroup` (section 3.4).
- Only the web image is published to GHCR; other images are built locally.
- HPA is CPU-only (EV-191 custom metrics stays open, D3).
- `ticketing-worker` is a singleton with `Recreate` (D5).
- Staging Redis private and passwordless; SMTP must accept AUTH; `SMS_PROVIDER=mock`; MoMo needs a public notify
  host; Stripe disabled in staging with the webhook signature debt above.
- web keeps a writable root filesystem (`policy-exempt.AC-12.readOnlyRootFilesystem`, Next ISR cache).
