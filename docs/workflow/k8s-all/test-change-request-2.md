# Test change request 2: fixtures for the review / challenge bypasses (fix round 1)

Status: **approved by TranDinhVo 2026-11-20 via coordinator** (add fixtures only; no existing fixture or test is
modified or deleted).

Scope: `deploy/k8s/policy/testdata/gen_fixtures.py` gains mutants and three Dockerfile cases; the generated files below
are new. `test_policy.sh` is unchanged. Regenerating changed no existing file (`gen_fixtures.py --check` clean,
`git status` shows only additions under `testdata/`). Checks they exercise: `policy-contract.md` revision r3.

Each bad fixture trips exactly one rule id (contract section 7).

| Finding | Fixture | Overlay | Mutation |
|---|---|---|---|
| X-1 / R-2 | `bad/AC-22--staging-duplicate-env-gemini-mock.yaml` | staging | ai-worker env gets a second `GEMINI_MOCK=1` after the ConfigMap ref |
| X-1 / R-2 | `bad/AC-22--staging-duplicate-env-providers.yaml` | staging | payment env gets a second `PAYMENT_ENABLED_PROVIDERS=sandbox` |
| X-2 | `bad/AC-32--envfrom-configmap-prefix.yaml` | staging | ConfigMap `tuning {HEALTH_MODE: static}` + controller `envFrom: [{prefix: ADMIT_, configMapRef: tuning}]` |
| X-3 | `bad/AC-20--cronjob-foreign-image.yaml` | local | CronJob `evil-cron` (gateway pod spec) with image `docker.io/evil/root:1.0` |
| X-3 | `bad/AC-17--pod-secret-literal.yaml` | staging | bare Pod `evil-pod` (gateway pod spec) with literal `JWT_SECRET` |
| X-3 | `bad/AC-12--daemonset-hostpath.yaml` | local | DaemonSet `evil-ds` (gateway pod spec) with a hostPath `/` volume |
| X-3 | `bad/AC-12--replicaset-privileged.yaml` | local | ReplicaSet `evil-rs` (gateway pod spec) with a privileged container |
| X-3 | `bad/AC-12--pod-ephemeral-privileged.yaml` | local | bare Pod `evil-debug` with a privileged ephemeral container |
| X-4 / R-6 | `bad/AC-21--staging-dev-infra-mailpit.yaml` | staging | the local `eventflow-mailpit` dev-infra Deployment added to staging |
| X-4 / R-6 | `bad/AC-21--staging-dev-infra-netpol.yaml` | staging | the local `allow-dev-infra` NetworkPolicy added to staging |
| X-5 | `bad/AC-17--empty-user-redis-url.yaml` | staging | endpoints ConfigMap `REDIS_URL=redis://:<pw>@host:6379/0` |
| X-5 / R-7 | `bad/AC-17--gateway-args-secret.yaml` | staging | gateway `args: [--jwt-secret=...]` |
| X-5 / R-7 | `bad/AC-17--gateway-args-dsn-password.yaml` | staging | gateway `args: [-db, "host=db user=app password=..."]` |
| X-5 | `bad/AC-17--configmap-binarydata-secret.yaml` | staging | endpoints ConfigMap `binaryData.JWT_SECRET` |
| X-5 | `bad/AC-17--literal-passphrase-name.yaml` | staging | gateway literal `SIGNING_PASSPHRASE` |
| X-5 | `bad/AC-17--literal-db-pass-name.yaml` | staging | gateway literal `DB_PASS` |
| X-5 | `bad/AC-17--literal-keys-name.yaml` | staging | gateway literal `PII_ENC_KEYS` |
| R-7 | `bad/AC-17--literal-dsn-password.yaml` | staging | gateway literal `PG_DSN="host=db user=app password=..."` |
| X-7 | `bad/AC-12--container-runasnonroot-false.yaml` | local | gateway container `runAsNonRoot: false` (pod level stays true) |
| X-7 | `bad/AC-12--container-seccomp-unconfined.yaml` | staging | gateway container `seccompProfile: Unconfined` |
| X-7 | `bad/AC-12--gateway-hostport.yaml` | staging | gateway port `hostPort: 80` |
| X-7 | `bad/AC-12--container-procmount-unmasked.yaml` | local | gateway container `procMount: Unmasked` |
| X-7 | `bad/AC-12--share-process-namespace.yaml` | local | gateway pod `shareProcessNamespace: true` |
| X-7 | `bad/AC-12--pod-fsgroup-0.yaml` | staging | gateway pod `fsGroup: 0` |
| X-8 | `bad/AC-15--identity-endport.yaml` | staging | `allow-identity` ports get `endPort: 65535` |
| X-10 | `dockerfiles/bad/AC-13--user-uid-0/go-service.Dockerfile` | dockerfiles | final stage `USER 0:0` |
| X-10 | `dockerfiles/bad/AC-13--lowercase-suffix/named.dockerfile` | dockerfiles | lowercase suffix, `USER root` |
| X-10 | `dockerfiles/bad/AC-13--plain-dockerfile-name/Dockerfile` | dockerfiles | file named `Dockerfile`, no `USER` |

Counts: bad YAML fixtures 153 -> 178 (+25), Dockerfile bad cases 3 -> 6 (+3).

## Additions, fix round 2 (findings-round2.json R2-1 / R2-2, 2026-11-21)

Within the approved TCR-2 scope (add fixtures only). `gen_fixtures.py` gains 2 mutants and 1 Dockerfile case;
regenerating changed no existing file (`gen_fixtures.py --check` clean). `test_policy.sh` unchanged. Checks: contract r3
section on AC-17 command/args and AC-13 (root group counts as root, as at pod level).

| Finding | Fixture | Overlay | Mutation |
|---|---|---|---|
| R2-1 | `bad/AC-17--gateway-args-split-password.yaml` | staging | gateway `args: [--password, hunter2-real]` (flag and value split) |
| R2-1 | `bad/AC-17--gateway-shell-pgpassword.yaml` | staging | gateway `command: [sh, -c, "PGPASSWORD=hunter2 psql -h db"]` |
| R2-2 | `dockerfiles/bad/AC-13--user-root-group/go-service.Dockerfile` | dockerfiles | final stage `USER 10001:0` |

Counts: bad YAML fixtures 178 -> 180 (+2), Dockerfile bad cases 6 -> 7 (+1).
