# Policy interface contract: k8s-all (FROZEN at round 2)

Frozen interface between WP0 (test designer: fixtures + `test_policy.sh`) and WP5 (implementer:
`check_policy.py`, `validate.sh`). Hand this file to WP0 together with `spec.md` (incl. its Erratum section).
Changes after freeze need a change request approved by the human, like a test change request.

## 1. Files

| Path | Owner | Notes |
|---|---|---|
| `deploy/k8s/policy/check_policy.py` | WP5 | python3 stdlib only, run as `python3 -I` |
| `deploy/k8s/policy/test_policy.sh` | WP0 | self-test; protected after freeze |
| `deploy/k8s/policy/testdata/**` | WP0 | fixtures; protected after freeze |
| `deploy/k8s/tools.env` | WP5 | pinned tool versions (section 6); sourced by both scripts |
| `deploy/k8s/validate.sh` | WP5 | entry point (section 7) |

## 2. Rule ids

A rule id is the spec AC number it enforces. One id = one check function, so the mutation test can disable exactly
one id at a time.

| Id | Checks (spec AC) | Scope | Overlay-specific |
|---|---|---|---|
| AC-1 | one Deployment per process in `--processes`, exactly one Namespace `eventflow`, no namespaced object elsewhere | whole render | no |
| AC-3 | web names, selector, container `web`, port `http`, probe paths, strategy unchanged | web objects | no |
| AC-4 | liveness + readiness on every container; startup on app containers; paths/ports per spec 4.1 | workloads | no |
| AC-5 | readiness ≠ liveness where `/readyz` exists; no liveness on a readiness path | workloads | no |
| AC-6 | request-serving: maxUnavailable 0, maxSurge 1, preStop, grace ≥ 5 + app grace + 5 | workloads | no |
| AC-7 | worker replicas 1 + Recreate; controller replicas 2; HPA-managed have no `spec.replicas` | workloads | no |
| AC-8 | requests + limits cpu/mem on every container incl. init; app values = 4.1 | workloads | no |
| AC-9 | one autoscaling/v2 HPA per HPA-managed Deployment, min ≥ 2, max per 4.1, cpu 60-75, scaleDown window ≥ 300 | HPA + workloads | no |
| AC-10 | exactly one policy/v1 PDB per app Deployment, `maxUnavailable: 1`, selector = pod labels | PDB + workloads | no |
| AC-11 | preferred hostname anti-affinity + zone spread `ScheduleAnyway` on own labels | workloads | no |
| AC-12 | pod/container security context, automount false, no host* / hostPath / uid 0 / runAsUser on app pods | workloads | no |
| AC-13 | numeric `USER` in the 4 Dockerfiles (run by `validate.sh`, not by the checker; same output format) | Dockerfiles | no |
| AC-14 | Namespace PSA labels | Namespace | yes: staging=restricted, local=baseline+warn/audit restricted |
| AC-15 | default-deny ingress; every app pod has an allow policy; matrix spec 6.2; no open `from` for backends | NetworkPolicy | no |
| AC-16 | Secret objects: none in staging; local only the allowed names, every value contains `dev` | Secret | yes |
| AC-17 | no secret-like env/ConfigMap literal (name or `://user:pass@` value) | workloads + ConfigMap | no |
| AC-18 | each process gets exactly its secret keys (spec Erratum E3 table); DIRECT vs POOLED DB URL | workloads | no |
| AC-19 | `APP_ENV` = production (staging) / development (local) on every Go process | ConfigMap/env | yes |
| AC-20 | app image name/tag/digest rule, pull policy IfNotPresent | workloads | no |
| AC-21 | dev-infra images pinned (no `latest`, no missing tag) | dev-infra workloads | local only |
| AC-22 | cross refs: Service→pods, targetPort names, Ingress→Service/port, HPA/PDB targets, secretKeyRef/configMapKeyRef | whole render | no |
| AC-23 | only web + gateway Ingress, hosts and SSE annotations; no NodePort/LoadBalancer | Ingress/Service | no |
| AC-32 | staging carries no demo settings (spec Erratum E2): no `MIN_SALE_LEAD_SECONDS`, no `ACTIVE_EVENT_IDS`, no `ADMIT_*`, no `GEMINI_MOCK` other than `"0"`, no `PAYMENT_SANDBOX_MODE`, `PAYMENT_DEFAULT_PROVIDER` ≠ `sandbox`, `sandbox` not in `PAYMENT_ENABLED_PROVIDERS` — whether set as env literal or through any rendered ConfigMap | workloads + ConfigMap | staging only |
| EXEMPT | an exemption annotation names a rule/workload pair not in spec section 7, or has an empty reason | annotations | no |

AC-2 (kubeconform), AC-24..AC-31 are not checker rules.

## 3. Exemption annotation (replaces the single-value form in spec section 4)

One annotation **per exempted rule**, on the workload's top-level `metadata.annotations`:

```yaml
metadata:
  annotations:
    eventflow.io/policy-exempt.AC-9: "singleton: reconciler not proven safe with 2 replicas (spec D5)"
    eventflow.io/policy-exempt.AC-11: "singleton: no spread for 1 replica (spec D5)"
```

- Key regex: `^eventflow\.io/policy-exempt\.(AC-[0-9]+)$`; value = non-empty reason (after trim).
- The checker skips that rule for that workload only (and for its HPA/PDB when the rule is about them).
- Allowed pairs = spec section 7, encoded as a constant table in `check_policy.py`. Dev-infra workloads are
  recognised by label `eventflow.io/tier: dev-infra` and may carry exemptions for AC-6, AC-9, AC-10, AC-11, AC-20 and,
  per workload, AC-12 (`runAsNonRoot` only). Any other pair → `FAIL EXEMPT …`.

## 4. CLI

```
python3 -I deploy/k8s/policy/check_policy.py --overlay {local|staging} --input <file.json|-> [--processes <csv>]
```

- `--overlay` (required) selects overlay-specific rules (AC-14, AC-16, AC-19, AC-21, AC-32).
- `--input`: a JSON **array** of Kubernetes objects; `-` = stdin. Produced only by
  `yq ea -o=json '[.]' <render.yaml>` with the pinned yq (section 6). The checker never reads YAML.
- `--processes`: comma-separated process names from spec 4.1 that the input is expected to contain; default = all 11.
  Rules AC-1/AC-9/AC-10/AC-15/AC-22 only demand objects for these processes. Fixtures use it to stay small.
- Exit codes: `0` no violation; `1` one or more violations; `2` usage or input error (bad args, unreadable file,
  JSON not an array, unknown process name). Exit 2 prints `ERROR <message>` on stderr.

## 5. Output

- One line per violation on stdout, regex:
  `^FAIL (AC-[0-9]+|EXEMPT) ([A-Za-z]+)/([a-z0-9.-]+): (.+)$` — kind, `metadata.name`, human reason.
  Whole-render violations use kind `Render` and name = overlay (e.g. `FAIL AC-1 Render/staging: 10 Deployments, want 11`).
- Last line always: `policy: OK (<n> objects, overlay=<o>)` on exit 0, or `policy: <k> violation(s) (overlay=<o>)` on exit 1.
- Order of FAIL lines is not part of the contract.

## 6. Pinned tools (`deploy/k8s/tools.env`)

```
KUSTOMIZE_VERSION=v5.7.1     # sigs.k8s.io/kustomize/kustomize/v5
KUBECONFORM_VERSION=v0.8.0   # github.com/yannh/kubeconform/cmd/kubeconform (same as the current CI image)
YQ_VERSION=v4.44.3           # github.com/mikefarah/yq/v4
K8S_SCHEMA_VERSION=1.32.0
```

Run mode `TOOLS=go` (default, CI and Windows): `go run <module>@<version>`. All three were run successfully with
`go run` on the dev machine with Go 1.27, no Docker needed (2026-11-01). `TOOLS=docker` is optional and uses the
images at the same versions with `MSYS_NO_PATHCONV=1` for `-v` mounts on Git Bash.

## 7. Fixtures and self-test

- Layout: `testdata/good-local.yaml`, `testdata/good-staging.yaml` (must pass), and
  `testdata/bad/<RULE-ID>--<slug>.yaml` (must fail).
- First line of every fixture: `# policy-args: --overlay <o> [--processes <csv>]`. `test_policy.sh` reads that line
  from the raw file, converts the file with the pinned yq, and runs the checker with those args.
- A bad fixture passes the self-test only if the checker exits 1 **and** prints at least one `FAIL <RULE-ID>` line
  for the id in its file name **and** no FAIL line for any other id (one fixture = one rule).
- A good fixture passes only on exit 0 with the `policy: OK` line.
- Every id of section 2 except AC-13 has ≥ 1 bad fixture; AC-32 has at least: `MIN_SALE_LEAD_SECONDS` in staging,
  `GEMINI_MOCK: "1"` via ConfigMap in staging, `PAYMENT_SANDBOX_MODE` in staging. EXEMPT has: unknown pair, empty reason.
- `test_policy.sh` exit 0 only if every fixture behaves as above; prints `selftest: <pass>/<total>`.
- RED check (WP0): before `check_policy.py` exists, `test_policy.sh` must exit non-zero for the right reason
  (checker missing → every fixture reported failed).
