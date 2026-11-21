# k8s-all: findings deferred from fix round 1 (2026-11-20)

Ids from `findings-review.json` (R-*) and `findings-challenge.json` (X-*). Each is out of the round-1 fix scope set
by the human; one line on why.

| Id | Sev | Deferred because |
|---|---|---|
| R-1 | major | Gateway code change (trusted-proxy `X-Forwarded-For`), human decision: separate gateway task; noted in runbook section 9 and spec section 9. |
| X-6 | minor | Secret/projected volumes, serviceAccountName and RBAC objects need new AC-12/AC-18 sub-rules and a contract change beyond this round; PSA + no RBAC in the overlays today. |
| X-9 | minor | externalIPs / ExternalName and probe host/scheme are new AC-23/AC-4 checks; no overlay uses them; next policy round. |
| X-12 | minor | Placeholder detection needs a strict mode tied to the runbook's temporary staging overlay; design work, not a bypass of an existing rule. |
| X-13 | minor | Web image change (cache dir ownership / emptyDir) is outside the k8s slice; needs a pod run to confirm Next behaviour. |
| R-4 | minor | Matches the approved NetworkPolicy matrix; residual risk (monitoring label is privileged) to document with the metrics-port debt. |
| R-5 | minor | No secret scanner exists; adding one (pinned tool + allowlist) is CI scope; AC-16/AC-17 remain the only guard. |
| R-8 | minor | Staging TLS on the Ingresses is a staging-apply precondition (cert/secret is environment-specific); runbook follow-up. |
| R-9 | minor | `TOOLS=docker` is optional; default go mode and CI are unaffected. |
| R-11 | minor | Dev-infra tags are local-only; pinning to digests needs the uids re-read per image. |

## Round 2 (findings-round2.json, 2026-11-21)

| Id | Sev | Status |
|---|---|---|
| R-13 | minor | Closed, no-op: not reproducible. `overlays/local/secrets.dev.env` has a single DEV ONLY header; lines 1-4 and 6-9 differ. Nothing to change. |
| R-10 | minor | Open, **merge precondition (human)**: see `merge-preconditions.md` item MP-2. |
| R-14 | minor | Open, **merge precondition (human)**: see `merge-preconditions.md` item MP-1. |
