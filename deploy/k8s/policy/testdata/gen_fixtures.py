#!/usr/bin/env python3
"""Generate the policy fixtures: multi-document YAML (one compact JSON object per document).  stdlib only.

  python3 -I gen_fixtures.py          # (re)write good-local.yaml, good-staging.yaml, bad/*.yaml
  python3 -I gen_fixtures.py --check  # exit 1 if the committed files differ from generator output

Contract: docs/workflow/k8s-all/policy-contract.md (frozen).  First line of each fixture:
  # policy-args: --overlay <o> [--processes <csv>]
Bad fixtures are named bad/<RULE-ID>--<slug>.yaml and must trip exactly that one rule.
"""
import base64
import copy
import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
NS = "eventflow"
REG = "ghcr.io/phanquocthang/"
TAG = "sha-c5fb598"
EXEMPT = "eventflow.io/policy-exempt."
PART = {"app.kubernetes.io/part-of": "eventflow"}
MON = {"namespaceSelector": {"matchLabels": {"eventflow.io/monitoring": "true"}}}

# process, image, cmd, ports, liveness, readiness, startup, grace, hpa(min,max), replicas, requests, limits
M = [
 ("web", "eventflow-web", None, [("http", 3000)], "/api/healthz", "/api/readyz", "/api/healthz", 30, (2, 6), None, ("100m", "256Mi"), ("500m", "512Mi")),
 ("gateway", "eventflow-gateway", None, [("http", 8080), ("metrics", 9094)], "/healthz", "/readyz", "/healthz", 35, (2, 10), None, ("200m", "128Mi"), ("1000m", "256Mi")),
 ("identity", "eventflow-identity", None, [("http", 8083)], "/healthz", "/healthz", "/healthz", 30, (2, 6), None, ("100m", "128Mi"), ("1000m", "256Mi")),
 ("event", "eventflow-event", None, [("http", 8084)], "/healthz", "/healthz", "/healthz", 30, (2, 6), None, ("100m", "128Mi"), ("500m", "256Mi")),
 ("waitingroom", "eventflow-waitingroom", None, [("http", 8081)], "/healthz", "/readyz", "/healthz", 35, (2, 10), None, ("200m", "128Mi"), ("1000m", "256Mi")),
 ("waitingroom-controller", "eventflow-waitingroom", ["/controller"], [("metrics", 9091)], "/metrics", "/metrics", "/metrics", 30, None, 2, ("50m", "64Mi"), ("250m", "128Mi")),
 ("ticketing", "eventflow-ticketing", None, [("http", 8082)], "/healthz", "/readyz", "/healthz", 35, (2, 10), None, ("200m", "256Mi"), ("1000m", "512Mi")),
 ("ticketing-worker", "eventflow-ticketing", ["/worker"], [("metrics", 9092)], "/metrics", "/metrics", "/metrics", 30, None, 1, ("100m", "128Mi"), ("500m", "256Mi")),
 ("payment", "eventflow-payment", None, [("http", 8085)], "/healthz", "/healthz", "/healthz", 30, (2, 6), None, ("100m", "128Mi"), ("500m", "256Mi")),
 ("notification", "eventflow-notification", None, [("http", 8087)], "/healthz", "/healthz", "/healthz", 30, (2, 4), None, ("50m", "64Mi"), ("250m", "128Mi")),
 ("ai-worker", "eventflow-ai-worker", None, [("http", 8090)], "/healthz", "/readyz", "/healthz", 35, (2, 6), None, ("200m", "256Mi"), ("1000m", "512Mi")),
]
SERVICES = {"web", "gateway", "identity", "event", "waitingroom", "ticketing", "payment", "ai-worker", "notification"}
GO = {"gateway", "identity", "event", "waitingroom", "waitingroom-controller", "ticketing", "ticketing-worker", "payment", "notification"}
REQUEST_SERVING = ("web", "gateway", "identity", "event", "waitingroom", "ticketing", "payment", "ai-worker")


def SECRET(s, k, opt=False):
    ref = {"name": s, "key": k}
    if opt:
        ref["optional"] = True
    return {"secretKeyRef": ref}


def CMAP(k):
    return {"configMapKeyRef": {"name": "eventflow-endpoints", "key": k}}


def APPCFG(k, opt=False):
    ref = {"name": "eventflow-app-config", "key": k}
    if opt:
        ref["optional"] = True
    return {"configMapKeyRef": ref}


def env_for(p, overlay="local"):
    e = []

    def v(n, val):
        e.append({"name": n, "value": val})

    def f(n, src):
        e.append({"name": n, "valueFrom": src})

    if p in GO:
        f("APP_ENV", APPCFG("APP_ENV"))
    if p == "web":
        v("NEXT_PUBLIC_API_BASE", "http://api.eventflow.localtest.me")
    if p == "gateway":
        f("JWT_SECRET", SECRET("eventflow-auth", "JWT_SECRET"))
        f("CORS_ALLOWED_ORIGINS", APPCFG("CORS_ALLOWED_ORIGINS"))
        v("IDENTITY_URL", "http://eventflow-identity:8083")
    if p == "identity":
        f("DATABASE_URL", SECRET("eventflow-db", "DATABASE_URL_DIRECT"))
        f("JWT_SECRET", SECRET("eventflow-auth", "JWT_SECRET"))
        f("PII_ENC_KEY", SECRET("eventflow-identity", "PII_ENC_KEY"))
        f("PII_HASH_KEY", SECRET("eventflow-identity", "PII_HASH_KEY"))
        f("SMTP_HOST", CMAP("SMTP_HOST"))
        f("GOOGLE_CLIENT_ID", APPCFG("GOOGLE_CLIENT_ID"))
        if overlay == "staging":
            f("SMTP_USER", SECRET("eventflow-identity", "SMTP_USER"))
            f("SMTP_PASS", SECRET("eventflow-identity", "SMTP_PASS"))
    if p == "event":
        f("DATABASE_URL", SECRET("eventflow-db", "DATABASE_URL_DIRECT"))
        f("RABBITMQ_URL", SECRET("eventflow-mq", "AMQP_URL"))
        f("MIN_SALE_LEAD_SECONDS", APPCFG("MIN_SALE_LEAD_SECONDS", True))
    if p in ("waitingroom-controller", "ticketing-worker"):
        f("ACTIVE_EVENT_IDS", APPCFG("ACTIVE_EVENT_IDS", True))
    if p == "waitingroom-controller":
        f("ADMIT_MIN_RATE", APPCFG("ADMIT_MIN_RATE", True))
    if p in ("waitingroom", "waitingroom-controller"):
        f("REDIS_ADDR", CMAP("REDIS_ADDR"))
    if p == "waitingroom":
        f("QUEUE_TOKEN_SECRET", SECRET("eventflow-auth", "QUEUE_TOKEN_SECRET"))
    if p == "ticketing":
        f("DATABASE_URL", SECRET("eventflow-db", "DATABASE_URL_POOLED"))
        f("QUEUE_TOKEN_SECRET", SECRET("eventflow-auth", "QUEUE_TOKEN_SECRET"))
        f("HMAC_QR_SECRET", SECRET("eventflow-qr", "HMAC_QR_SECRET"))
    if p == "ticketing-worker":
        f("DATABASE_URL", SECRET("eventflow-db", "DATABASE_URL_POOLED"))
        f("RABBITMQ_URL", SECRET("eventflow-mq", "AMQP_URL"))
        f("HMAC_QR_SECRET", SECRET("eventflow-qr", "HMAC_QR_SECRET"))
    if p == "payment":
        f("DATABASE_URL", SECRET("eventflow-db", "DATABASE_URL_POOLED"))
        f("RABBITMQ_URL", SECRET("eventflow-mq", "AMQP_URL"))
        pkeys = ["VNPAY_TMN_CODE", "VNPAY_HASH_SECRET", "MOMO_PARTNER_CODE", "MOMO_ACCESS_KEY", "MOMO_SECRET_KEY"]
        if overlay == "local":
            pkeys += ["STRIPE_PUBLISHABLE_KEY", "STRIPE_SECRET_KEY", "STRIPE_WEBHOOK_SECRET"]
        for k in pkeys:
            f(k, SECRET("eventflow-payment", k))
        for k in ("VNPAY_RETURN_URL", "MOMO_RETURN_URL", "MOMO_NOTIFY_URL"):
            f(k, APPCFG(k))
        for k in ("STRIPE_SUCCESS_URL", "STRIPE_CANCEL_URL"):
            f(k, APPCFG(k, True))
        f("PAYMENT_DEFAULT_PROVIDER", APPCFG("PAYMENT_DEFAULT_PROVIDER"))
        f("PAYMENT_ENABLED_PROVIDERS", APPCFG("PAYMENT_ENABLED_PROVIDERS"))
        f("PAYMENT_SANDBOX_MODE", APPCFG("PAYMENT_SANDBOX_MODE", True))
    if p == "notification":
        f("DATABASE_URL", SECRET("eventflow-db", "DATABASE_URL_POOLED"))
        f("AMQP_URL", SECRET("eventflow-mq", "AMQP_URL"))
        f("EMAIL_PROVIDER", APPCFG("EMAIL_PROVIDER"))
        f("SMTP_FROM", APPCFG("SMTP_FROM"))
        if overlay == "staging":
            f("SMTP_USER", SECRET("eventflow-notification", "SMTP_USER"))
            f("SMTP_PASSWORD", SECRET("eventflow-notification", "SMTP_PASSWORD"))
    if p == "ai-worker":
        f("AMQP_URL", SECRET("eventflow-mq", "AMQP_URL"))
        f("REDIS_URL", CMAP("REDIS_URL"))
        f("GEMINI_API_KEY", SECRET("eventflow-ai", "GEMINI_API_KEY"))
        f("GEMINI_MOCK", APPCFG("GEMINI_MOCK"))
        v("INTERNAL_API_BASE", "http://eventflow-gateway:8080")
        v("MODERATION_API_BASE", "http://eventflow-event:8084")
    return e


def labels(p):
    return dict({"app.kubernetes.io/name": "eventflow-" + p}, **PART)


def sel(p):
    return {"matchLabels": labels(p)}


def deployment(row, overlay="local"):
    p, img, cmd, ports, live, ready, start, grace, hpa_, repl, req, lim = row
    pn = "http" if any(n == "http" for n, _ in ports) else "metrics"
    L = labels(p)

    def pr(path, extra):
        d = {"httpGet": {"path": path, "port": pn}, "timeoutSeconds": 2}
        d.update(extra)
        return d

    c = {
        "name": p,
        "image": "%s%s:%s" % (REG, img, TAG), "imagePullPolicy": "IfNotPresent",
        "ports": [{"name": n, "containerPort": num} for n, num in ports],
        "env": env_for(p, overlay),
        "resources": {"requests": {"cpu": req[0], "memory": req[1]}, "limits": {"cpu": lim[0], "memory": lim[1]}},
        "startupProbe": pr(start, {"periodSeconds": 2, "failureThreshold": 30}),
        "livenessProbe": pr(live, {"periodSeconds": 10, "failureThreshold": 3}),
        "readinessProbe": pr(ready, {"periodSeconds": 5, "failureThreshold": 3}),
        "securityContext": {"allowPrivilegeEscalation": False, "readOnlyRootFilesystem": p != "web",
                            "capabilities": {"drop": ["ALL"]}},
    }
    if cmd:
        c["command"] = cmd
    spec = {
        "automountServiceAccountToken": False,
        "terminationGracePeriodSeconds": grace,
        "securityContext": {"runAsNonRoot": True, "seccompProfile": {"type": "RuntimeDefault"}},
        "containers": [c],
    }
    if p == "ai-worker":
        c["volumeMounts"] = [{"name": "tmp", "mountPath": "/tmp"}]
        spec["volumes"] = [{"name": "tmp", "emptyDir": {}}]
    if p != "ticketing-worker":
        spec["affinity"] = {"podAntiAffinity": {"preferredDuringSchedulingIgnoredDuringExecution": [
            {"weight": 100, "podAffinityTerm": {"topologyKey": "kubernetes.io/hostname",
                                                "labelSelector": {"matchLabels": dict(L)}}}]}}
        spec["topologySpreadConstraints"] = [{"maxSkew": 1, "topologyKey": "topology.kubernetes.io/zone",
                                              "whenUnsatisfiable": "ScheduleAnyway",
                                              "labelSelector": {"matchLabels": dict(L)}}]
    if p in REQUEST_SERVING:
        c["lifecycle"] = {"preStop": ({"exec": {"command": ["node", "-e", "setTimeout(()=>{},5000)"]}} if p == "web"
                                      else {"sleep": {"seconds": 5}})}
    strat = {"type": "RollingUpdate", "rollingUpdate": {"maxUnavailable": 0, "maxSurge": 1}}
    if p == "ticketing-worker":
        strat = {"type": "Recreate"}
    d = {"apiVersion": "apps/v1", "kind": "Deployment",
         "metadata": {"name": "eventflow-" + p, "namespace": NS, "labels": dict(L)},
         "spec": {"selector": {"matchLabels": dict(L)}, "strategy": strat,
                  "template": {"metadata": {"labels": dict(L), "annotations": {
                      "prometheus.io/scrape": "true", "prometheus.io/path": "/metrics",
                      "prometheus.io/port": str(ports[-1][1])}}, "spec": spec}}}
    if repl is not None:
        d["spec"]["replicas"] = repl
    ex = {}
    if p == "web":
        ex["AC-12.readOnlyRootFilesystem"] = "Next ISR writes .next/cache"
    if p == "ticketing-worker":
        ex["AC-11"] = "singleton: no spread for 1 replica (spec D5)"
    if ex:
        d["metadata"]["annotations"] = {EXEMPT + k: v for k, v in ex.items()}
    return d


def service(row):
    p, ports = row[0], row[3]
    return {"apiVersion": "v1", "kind": "Service",
            "metadata": {"name": "eventflow-" + p, "namespace": NS, "labels": labels(p)},
            "spec": {"type": "ClusterIP", "selector": labels(p),
                     "ports": [{"name": n, "port": num, "targetPort": n} for n, num in ports]}}


def hpa(row):
    p = row[0]
    lo, hi = row[8]
    return {"apiVersion": "autoscaling/v2", "kind": "HorizontalPodAutoscaler",
            "metadata": {"name": "eventflow-" + p, "namespace": NS},
            "spec": {"scaleTargetRef": {"apiVersion": "apps/v1", "kind": "Deployment", "name": "eventflow-" + p},
                     "minReplicas": lo, "maxReplicas": hi,
                     "metrics": [{"type": "Resource", "resource": {"name": "cpu", "target": {"type": "Utilization", "averageUtilization": 70}}}],
                     "behavior": {"scaleDown": {"stabilizationWindowSeconds": 300}}}}


def pdb(row):
    p = row[0]
    return {"apiVersion": "policy/v1", "kind": "PodDisruptionBudget",
            "metadata": {"name": "eventflow-" + p, "namespace": NS},
            "spec": {"maxUnavailable": 1, "selector": sel(p)}}


def np_allow(dest, sources, ports, extra=None):
    """sources: ('pod', process) | ('ns', metadata-name) | ('mon',).  extra: [(from-list, ports)]"""
    frm = []
    for s in sources:
        if s[0] == "pod":
            frm.append({"podSelector": sel(s[1])})
        elif s[0] == "ns":
            frm.append({"namespaceSelector": {"matchLabels": {"kubernetes.io/metadata.name": s[1]}}})
        else:
            frm.append(copy.deepcopy(MON))
    rules = [{"from": frm, "ports": [{"protocol": "TCP", "port": x} for x in ports]}]
    for f2, p2 in (extra or []):
        rules.append({"from": f2, "ports": [{"protocol": "TCP", "port": x} for x in p2]})
    return {"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy",
            "metadata": {"name": "allow-" + dest, "namespace": NS},
            "spec": {"podSelector": sel(dest), "policyTypes": ["Ingress"], "ingress": rules}}


def netpols():
    n = [{"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy",
          "metadata": {"name": "default-deny-ingress", "namespace": NS},
          "spec": {"podSelector": {}, "policyTypes": ["Ingress"]}}]
    n.append(np_allow("web", [("ns", "ingress-nginx")], [3000]))
    n.append(np_allow("gateway", [("ns", "ingress-nginx"), ("pod", "web"), ("pod", "ai-worker")], [8080],
                      extra=[([copy.deepcopy(MON)], [9094])]))
    for p, port in (("identity", 8083), ("event", 8084), ("waitingroom", 8081), ("ticketing", 8082), ("payment", 8085)):
        src = [("pod", "gateway")] + ([("pod", "ai-worker")] if p == "event" else [])
        n.append(np_allow(p, src, [port], extra=[([copy.deepcopy(MON)], [port])]))
    n.append(np_allow("ai-worker", [("pod", "gateway"), ("mon",)], [8090]))
    n.append(np_allow("notification", [("mon",)], [8087]))
    n.append(np_allow("waitingroom-controller", [("mon",)], [9091]))
    n.append(np_allow("ticketing-worker", [("mon",)], [9092]))
    return n


def ingress(name, host, svc, anns=None):
    return {"apiVersion": "networking.k8s.io/v1", "kind": "Ingress",
            "metadata": {"name": name, "namespace": NS, "annotations": anns or {}},
            "spec": {"ingressClassName": "nginx", "rules": [{"host": host, "http": {"paths": [
                {"path": "/", "pathType": "Prefix",
                 "backend": {"service": {"name": svc, "port": {"name": "http"}}}}]}}]}}


def b64(s):
    return base64.b64encode(s.encode()).decode()


SECRETS = {
    "eventflow-db": {"DATABASE_URL_DIRECT": "postgres://eventflow:dev-pw@eventflow-postgres:5432/eventflow",
                     "DATABASE_URL_POOLED": "postgres://eventflow:dev-pw@eventflow-pgbouncer:5432/eventflow"},
    "eventflow-mq": {"AMQP_URL": "amqp://eventflow:dev-pw@eventflow-rabbitmq:5672/"},
    "eventflow-auth": {"JWT_SECRET": "dev-jwt-secret-change-me-0123456789", "QUEUE_TOKEN_SECRET": "dev-queue-token-secret-0123456789"},
    "eventflow-qr": {"HMAC_QR_SECRET": "dev-hmac-qr-secret-0123456789"},
    "eventflow-identity": {"PII_ENC_KEY": "dev-pii-enc-key-0123456789", "PII_HASH_KEY": "dev-pii-hash-key-0123456789"},
    "eventflow-payment": {"VNPAY_TMN_CODE": "dev-vnpay-tmn", "VNPAY_HASH_SECRET": "dev-vnpay-hash", "MOMO_PARTNER_CODE": "dev-momo-partner",
                          "MOMO_ACCESS_KEY": "dev-momo-access", "MOMO_SECRET_KEY": "dev-momo-secret", "STRIPE_PUBLISHABLE_KEY": "dev-stripe-pub",
                          "STRIPE_SECRET_KEY": "dev-stripe-sk", "STRIPE_WEBHOOK_SECRET": "dev-stripe-whsec"},
    "eventflow-ai": {"GEMINI_API_KEY": "dev-gemini-key"},
    "eventflow-dev-infra": {"POSTGRES_PASSWORD": "dev-pw", "RABBITMQ_DEFAULT_PASS": "dev-pw"},
}


def secret_obj(name, data):
    return {"apiVersion": "v1", "kind": "Secret", "type": "Opaque",
            "metadata": {"name": name, "namespace": NS}, "data": {k: b64(v) for k, v in data.items()}}


PG_LABELS = {"app.kubernetes.io/name": "eventflow-postgres", "app.kubernetes.io/part-of": "eventflow", "eventflow.io/tier": "dev-infra"}
PG_EXEMPT = {"AC-6": "statefulset, single replica dev db", "AC-10": "single replica dev db",
             "AC-11": "single replica", "AC-20": "upstream image, version pinned (AC-21)"}


def devinfra_pg():
    def pr():
        return {"exec": {"command": ["pg_isready", "-h", "127.0.0.1"]}, "periodSeconds": 10}
    return {"apiVersion": "apps/v1", "kind": "StatefulSet",
            "metadata": {"name": "eventflow-postgres", "namespace": NS, "labels": dict(PG_LABELS),
                         "annotations": {EXEMPT + k: v for k, v in PG_EXEMPT.items()}},
            "spec": {"serviceName": "eventflow-postgres", "replicas": 1, "selector": {"matchLabels": dict(PG_LABELS)},
                     "template": {"metadata": {"labels": dict(PG_LABELS)}, "spec": {
                         "automountServiceAccountToken": False,
                         "securityContext": {"runAsNonRoot": True, "runAsUser": 70, "runAsGroup": 70, "fsGroup": 70,
                                             "seccompProfile": {"type": "RuntimeDefault"}},
                         "containers": [{"name": "postgres", "securityContext": {"allowPrivilegeEscalation": False, "capabilities": {"drop": ["ALL"]}}, "image": "postgres:16-alpine",
                                         "ports": [{"name": "pg", "containerPort": 5432}],
                                         "env": [{"name": "POSTGRES_PASSWORD", "valueFrom": SECRET("eventflow-dev-infra", "POSTGRES_PASSWORD")}],
                                         "resources": {"requests": {"cpu": "100m", "memory": "256Mi"}, "limits": {"cpu": "500m", "memory": "512Mi"}},
                                         "startupProbe": pr(), "livenessProbe": pr(), "readinessProbe": pr()}]}}}}


def devinfra_mailpit():
    L = {"app.kubernetes.io/name": "eventflow-mailpit", "app.kubernetes.io/part-of": "eventflow", "eventflow.io/tier": "dev-infra"}
    pr = {"tcpSocket": {"port": "smtp"}, "periodSeconds": 10}
    ex = {"AC-6": "dev mail catcher, single replica", "AC-10": "single replica dev tool",
          "AC-11": "single replica", "AC-20": "upstream image, version pinned (AC-21)"}
    return {"apiVersion": "apps/v1", "kind": "Deployment",
            "metadata": {"name": "eventflow-mailpit", "namespace": NS, "labels": dict(L),
                         "annotations": {EXEMPT + k: v for k, v in ex.items()}},
            "spec": {"replicas": 1, "selector": {"matchLabels": dict(L)},
                     "template": {"metadata": {"labels": dict(L)}, "spec": {
                         "automountServiceAccountToken": False,
                         "securityContext": {"runAsNonRoot": True, "runAsUser": 1000, "runAsGroup": 1000, "seccompProfile": {"type": "RuntimeDefault"}},
                         "containers": [{"name": "mailpit", "image": "axllent/mailpit:v1.21",
                                         "ports": [{"name": "smtp", "containerPort": 1025}, {"name": "ui", "containerPort": 8025}],
                                         "securityContext": {"allowPrivilegeEscalation": False, "capabilities": {"drop": ["ALL"]}},
                                         "resources": {"requests": {"cpu": "20m", "memory": "32Mi"}, "limits": {"cpu": "200m", "memory": "128Mi"}},
                                         "startupProbe": dict(pr), "livenessProbe": dict(pr), "readinessProbe": dict(pr)}]}}}}


def devinfra_np():
    return {"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy",
            "metadata": {"name": "allow-dev-infra", "namespace": NS},
            "spec": {"podSelector": {"matchLabels": {"eventflow.io/tier": "dev-infra"}}, "policyTypes": ["Ingress"],
                     "ingress": [{"from": [{"namespaceSelector": {"matchLabels": {"kubernetes.io/metadata.name": NS}}}]}]}}


def devinfra_svc():
    return {"apiVersion": "v1", "kind": "Service", "metadata": {"name": "eventflow-postgres", "namespace": NS},
            "spec": {"type": "ClusterIP", "selector": dict(PG_LABELS),
                     "ports": [{"name": "pg", "port": 5432, "targetPort": "pg"}]}}


SUF = "5g8h2k9m4t"


def add_hash_suffixes(objs, overlay):
    """kustomize generator names: local Secrets and ConfigMaps, staging ConfigMaps get a hash suffix; refs follow."""
    renamed = {"ConfigMap": {"eventflow-endpoints", "eventflow-app-config"}}
    if overlay == "local":
        renamed["Secret"] = {x["metadata"]["name"] for x in objs if x["kind"] == "Secret"}
    for x in objs:
        if x["metadata"]["name"] in renamed.get(x["kind"], ()):
            x["metadata"]["name"] += "-" + SUF

    def walk(v):
        if isinstance(v, dict):
            for rk, kind in (("secretKeyRef", "Secret"), ("configMapKeyRef", "ConfigMap"), ("secretRef", "Secret")):
                r = v.get(rk)
                if isinstance(r, dict) and r.get("name") in renamed.get(kind, ()):
                    r["name"] += "-" + SUF
            for c in v.values():
                walk(c)
        elif isinstance(v, list):
            for c in v:
                walk(c)
    walk(objs)
    return objs


def good(overlay):
    objs = []
    psa = ({"pod-security.kubernetes.io/enforce": "restricted"} if overlay == "staging" else
           {"pod-security.kubernetes.io/enforce": "baseline", "pod-security.kubernetes.io/warn": "restricted",
            "pod-security.kubernetes.io/audit": "restricted"})
    objs.append({"apiVersion": "v1", "kind": "Namespace", "metadata": {"name": NS, "labels": psa}})
    objs.append({"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "eventflow-endpoints", "namespace": NS},
                 "data": {"REDIS_ADDR": "eventflow-redis:6379" if overlay == "local" else "redis.example.internal:6379",
                          "REDIS_URL": "redis://eventflow-redis:6379/0" if overlay == "local" else "redis://redis.example.internal:6379/0",
                          "SMTP_HOST": "eventflow-mailpit", "SMTP_PORT": "1025", "OTEL_EXPORTER_OTLP_ENDPOINT": ""}})
    common = {"EMAIL_PROVIDER": "smtp", "SMTP_FROM": "noreply@eventflow.example.com", "GOOGLE_CLIENT_ID": "dev-google-client-id.apps.example.com"}
    if overlay == "local":
        cfg = dict(common, APP_ENV="development", MIN_SALE_LEAD_SECONDS="60",
                   ACTIVE_EVENT_IDS="00000000-0000-0000-0000-000000000001,00000000-0000-0000-0000-000000000002",
                   ADMIT_MIN_RATE="5", GEMINI_MOCK="1", PAYMENT_DEFAULT_PROVIDER="sandbox",
                   PAYMENT_ENABLED_PROVIDERS="sandbox,vnpay,momo,stripe", PAYMENT_SANDBOX_MODE="success",
                   CORS_ALLOWED_ORIGINS="http://eventflow.localtest.me",
                   VNPAY_RETURN_URL="http://eventflow.localtest.me/pay/vnpay", MOMO_RETURN_URL="http://eventflow.localtest.me/pay/momo",
                   MOMO_NOTIFY_URL="http://api.eventflow.localtest.me/payments/momo/ipn",
                   STRIPE_SUCCESS_URL="http://eventflow.localtest.me/pay/ok", STRIPE_CANCEL_URL="http://eventflow.localtest.me/pay/cancel")
    else:
        cfg = dict(common, APP_ENV="production", GEMINI_MOCK="0", PAYMENT_DEFAULT_PROVIDER="vnpay",
                   PAYMENT_ENABLED_PROVIDERS="vnpay,momo", CORS_ALLOWED_ORIGINS="https://staging.example.com",
                   VNPAY_RETURN_URL="https://staging.example.com/pay/vnpay", MOMO_RETURN_URL="https://staging.example.com/pay/momo",
                   MOMO_NOTIFY_URL="https://staging.example.com/payments/momo/ipn")
    objs.append({"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "eventflow-app-config", "namespace": NS}, "data": cfg})
    for row in M:
        objs.append(deployment(row, overlay))
        if row[0] in SERVICES:
            objs.append(service(row))
        if row[8]:
            objs.append(hpa(row))
        objs.append(pdb(row))
    objs += netpols()
    objs.append(ingress("eventflow-web", "eventflow.localtest.me", "eventflow-web"))
    objs.append(ingress("eventflow-gateway", "api.eventflow.localtest.me", "eventflow-gateway",
                        {"nginx.ingress.kubernetes.io/proxy-buffering": "off",
                         "nginx.ingress.kubernetes.io/proxy-read-timeout": "3600"}))
    if overlay == "local":
        for n, d in SECRETS.items():
            objs.append(secret_obj(n, d))
        objs += [devinfra_pg(), devinfra_mailpit(), devinfra_svc(), devinfra_np()]
    return add_hash_suffixes(objs, overlay)




# ---- helpers for mutations ---------------------------------------------------
def find(objs, kind, name):
    for o in objs:
        if o["kind"] == kind and o["metadata"]["name"] in (name, name + "-" + SUF):
            return o
    raise KeyError((kind, name))


def dep(o, p):
    return find(o, "Deployment", "eventflow-" + p)


def ctr(o, p):
    return dep(o, p)["spec"]["template"]["spec"]["containers"][0]


def pod(o, p):
    return dep(o, p)["spec"]["template"]["spec"]


def drop(objs, kind, name):
    objs[:] = [x for x in objs if not (x["kind"] == kind and x["metadata"]["name"] in (name, name + "-" + SUF))]


def envitem(o, p, name):
    for e in ctr(o, p)["env"]:
        if e["name"] == name:
            return e
    raise KeyError(name)


def setenv(o, p, name, **new):
    e = envitem(o, p, name)
    e.clear()
    e["name"] = name
    e.update(new)


def rname(o, kind, base):
    for x in o:
        if x["kind"] == kind and x["metadata"]["name"] in (base, base + "-" + SUF):
            return x["metadata"]["name"]
    return base


def SX(o, s_, k):
    return SECRET(rname(o, "Secret", s_), k)


def CX(o, n, k, opt=False):
    ref = {"name": rname(o, "ConfigMap", n), "key": k}
    if opt:
        ref["optional"] = True
    return {"configMapKeyRef": ref}


def pgctr(o):
    return find(o, "StatefulSet", "eventflow-postgres")["spec"]["template"]["spec"]["containers"][0]


def cfg(o, name="eventflow-app-config"):
    return find(o, "ConfigMap", name)["data"]


PROC_NAMES = sorted([r[0] for r in M], key=len, reverse=True)


def proc_of(obj):
    """Process a rendered object belongs to (by metadata.name), or None for shared objects."""
    name = obj["metadata"].get("name", "")
    if obj["kind"] == "NetworkPolicy":
        if not name.startswith("allow-"):
            return None
        name = "eventflow-" + name[6:]
    if obj["kind"] in ("Deployment", "Service", "HorizontalPodAutoscaler", "PodDisruptionBudget", "NetworkPolicy", "Ingress"):
        for p in PROC_NAMES:
            if name == "eventflow-" + p:
                return p
    return None


def key(o):
    return (o["kind"], o["metadata"].get("namespace"), o["metadata"]["name"])


MUT = []


def mut(rule, slug, fn, overlay="local", extra=()):
    MUT.append((rule, slug, overlay, fn, tuple(extra)))


def hpa_on(p):
    def fn(o):
        h = copy.deepcopy(find(o, "HorizontalPodAutoscaler", "eventflow-gateway"))
        h["metadata"]["name"] = "eventflow-" + p
        h["spec"]["scaleTargetRef"]["name"] = "eventflow-" + p
        o.append(h)
    return fn


def pdb_min_available(o):
    s = find(o, "PodDisruptionBudget", "eventflow-ticketing-worker")["spec"]
    s.pop("maxUnavailable")
    s["minAvailable"] = 1


def anti_required(o):
    a = pod(o, "payment")["affinity"]["podAntiAffinity"]
    a.pop("preferredDuringSchedulingIgnoredDuringExecution")
    a["requiredDuringSchedulingIgnoredDuringExecution"] = [{"topologyKey": "kubernetes.io/hostname", "labelSelector": {"matchLabels": labels("payment")}}]


def ai_no_tmp(o):
    ctr(o, "ai-worker").pop("volumeMounts")
    pod(o, "ai-worker").pop("volumes")


def web_extra_label(o):
    L = {"app.kubernetes.io/instance": "eventflow"}
    dep(o, "web")["spec"]["selector"]["matchLabels"].update(L)
    dep(o, "web")["spec"]["template"]["metadata"]["labels"].update(L)
    find(o, "PodDisruptionBudget", "eventflow-web")["spec"]["selector"]["matchLabels"].update(L)
    pod(o, "web")["affinity"]["podAntiAffinity"]["preferredDuringSchedulingIgnoredDuringExecution"][0]["podAffinityTerm"]["labelSelector"]["matchLabels"].update(L)
    pod(o, "web")["topologySpreadConstraints"][0]["labelSelector"]["matchLabels"].update(L)


def pg_no_pdb_exempt(o):
    find(o, "StatefulSet", "eventflow-postgres")["metadata"]["annotations"].pop(EXEMPT + "AC-10")


def ghost_hpa(o):
    h = copy.deepcopy(find(o, "HorizontalPodAutoscaler", "eventflow-gateway"))
    h["metadata"]["name"] = "eventflow-ghost"
    h["spec"]["scaleTargetRef"]["name"] = "eventflow-ghost"
    o.append(h)


def ghost_pdb(o):
    p = copy.deepcopy(find(o, "PodDisruptionBudget", "eventflow-gateway"))
    p["metadata"]["name"] = "eventflow-ghost"
    p["spec"]["selector"] = {"matchLabels": {"app.kubernetes.io/name": "eventflow-ghost"}}
    o.append(p)


def pg_liveness_readyz(o):
    pgctr(o)["livenessProbe"] = {"httpGet": {"path": "/readyz", "port": "pg"}, "periodSeconds": 10}


def exempt(o, p, rule, reason):
    d = dep(o, p)
    d["metadata"].setdefault("annotations", {})[EXEMPT + rule] = reason


# AC-1 (only the cases that do not also break per-process rules)
mut("AC-1", "object-outside-namespace", lambda o: o.append({"apiVersion": "v1", "kind": "ServiceAccount", "metadata": {"name": "stray", "namespace": "kube-system"}}))
mut("AC-1", "second-namespace", lambda o: o.append({"apiVersion": "v1", "kind": "Namespace", "metadata": {"name": "other"}}))
# AC-3
mut("AC-3", "extra-selector-label", web_extra_label)
mut("AC-3", "container-renamed", lambda o: ctr(o, "web").update({"name": "frontend"}))
# AC-4
mut("AC-4", "gateway-liveness-missing", lambda o: ctr(o, "gateway").pop("livenessProbe"))
mut("AC-4", "ticketing-readiness-missing", lambda o: ctr(o, "ticketing").pop("readinessProbe"))
mut("AC-4", "payment-startup-missing", lambda o: ctr(o, "payment").pop("startupProbe"))
mut("AC-4", "dev-infra-liveness-missing", lambda o: pgctr(o).pop("livenessProbe"))
mut("AC-4", "dev-infra-readiness-missing", lambda o: pgctr(o).pop("readinessProbe"))
mut("AC-4", "identity-wrong-path", lambda o: ctr(o, "identity")["readinessProbe"]["httpGet"].update({"path": "/ping"}))
mut("AC-4", "worker-http-path", lambda o: ctr(o, "ticketing-worker")["livenessProbe"]["httpGet"].update({"path": "/healthz"}))
mut("AC-4", "numeric-probe-port", lambda o: ctr(o, "event")["livenessProbe"]["httpGet"].update({"port": 8084}))
# AC-5 (only dev-infra can isolate it: for spec 4.1 processes AC-4 pins the paths too)
mut("AC-5", "dev-infra-liveness-on-readyz", pg_liveness_readyz)
# AC-6
mut("AC-6", "gateway-unavailable-1", lambda o: dep(o, "gateway")["spec"]["strategy"]["rollingUpdate"].update({"maxUnavailable": 1}))
mut("AC-6", "payment-recreate", lambda o: dep(o, "payment")["spec"].update({"strategy": {"type": "Recreate"}}))
mut("AC-6", "ticketing-no-prestop", lambda o: ctr(o, "ticketing").pop("lifecycle"))
mut("AC-6", "gateway-grace-20", lambda o: pod(o, "gateway").update({"terminationGracePeriodSeconds": 20}))
# AC-7
mut("AC-7", "worker-rolling", lambda o: dep(o, "ticketing-worker")["spec"].update({"strategy": {"type": "RollingUpdate"}}))
mut("AC-7", "worker-two-replicas", lambda o: dep(o, "ticketing-worker")["spec"].update({"replicas": 2}))
mut("AC-7", "controller-one-replica", lambda o: dep(o, "waitingroom-controller")["spec"].update({"replicas": 1}))
mut("AC-7", "gateway-has-replicas", lambda o: dep(o, "gateway")["spec"].update({"replicas": 3}))
# AC-8
mut("AC-8", "event-requests-missing", lambda o: ctr(o, "event")["resources"].pop("requests"))
mut("AC-8", "identity-limits-missing", lambda o: ctr(o, "identity")["resources"].pop("limits"))
mut("AC-8", "notification-no-cpu-request", lambda o: ctr(o, "notification")["resources"]["requests"].pop("cpu"))
mut("AC-8", "ai-worker-no-memory-limit", lambda o: ctr(o, "ai-worker")["resources"]["limits"].pop("memory"))
mut("AC-8", "dev-infra-no-resources", lambda o: pgctr(o).pop("resources"))
mut("AC-8", "ticketing-cpu-changed", lambda o: ctr(o, "ticketing")["resources"]["requests"].update({"cpu": "500m"}))
# AC-9
mut("AC-9", "gateway-hpa-missing", lambda o: drop(o, "HorizontalPodAutoscaler", "eventflow-gateway"))
mut("AC-9", "identity-min-1", lambda o: find(o, "HorizontalPodAutoscaler", "eventflow-identity")["spec"].update({"minReplicas": 1}))
mut("AC-9", "event-max-20", lambda o: find(o, "HorizontalPodAutoscaler", "eventflow-event")["spec"].update({"maxReplicas": 20}))
mut("AC-9", "payment-cpu-90", lambda o: find(o, "HorizontalPodAutoscaler", "eventflow-payment")["spec"]["metrics"][0]["resource"]["target"].update({"averageUtilization": 90}))
mut("AC-9", "web-stabilization-60", lambda o: find(o, "HorizontalPodAutoscaler", "eventflow-web")["spec"]["behavior"]["scaleDown"].update({"stabilizationWindowSeconds": 60}))
mut("AC-9", "ticketing-no-behavior", lambda o: find(o, "HorizontalPodAutoscaler", "eventflow-ticketing")["spec"].pop("behavior"))
mut("AC-9", "hpa-on-ticketing-worker", hpa_on("ticketing-worker"))
mut("AC-9", "hpa-on-controller", hpa_on("waitingroom-controller"))
# AC-10
mut("AC-10", "gateway-pdb-missing", lambda o: drop(o, "PodDisruptionBudget", "eventflow-gateway"))
mut("AC-10", "dev-infra-exemption-removed", pg_no_pdb_exempt)
mut("AC-10", "worker-min-available", pdb_min_available)
mut("AC-10", "event-max-unavailable-2", lambda o: find(o, "PodDisruptionBudget", "eventflow-event")["spec"].update({"maxUnavailable": 2}))
mut("AC-10", "identity-selector-subset", lambda o: find(o, "PodDisruptionBudget", "eventflow-identity")["spec"]["selector"].update({"matchLabels": {"app.kubernetes.io/name": "eventflow-identity"}}))
# AC-11
mut("AC-11", "event-affinity-missing", lambda o: pod(o, "event").pop("affinity"))
mut("AC-11", "payment-required-not-preferred", anti_required)
mut("AC-11", "gateway-wrong-topology-key", lambda o: pod(o, "gateway")["affinity"]["podAntiAffinity"]["preferredDuringSchedulingIgnoredDuringExecution"][0]["podAffinityTerm"].update({"topologyKey": "topology.kubernetes.io/zone"}))
mut("AC-11", "controller-not-exempt", lambda o: pod(o, "waitingroom-controller").pop("affinity"))
mut("AC-11", "ai-worker-spread-missing", lambda o: pod(o, "ai-worker").pop("topologySpreadConstraints"))
mut("AC-11", "identity-do-not-schedule", lambda o: pod(o, "identity")["topologySpreadConstraints"][0].update({"whenUnsatisfiable": "DoNotSchedule"}))
mut("AC-11", "web-foreign-spread-selector", lambda o: pod(o, "web")["topologySpreadConstraints"][0]["labelSelector"]["matchLabels"].update({"app.kubernetes.io/name": "eventflow-gateway"}))
# AC-12
mut("AC-12", "payment-runasnonroot-missing", lambda o: pod(o, "payment")["securityContext"].pop("runAsNonRoot"))
mut("AC-12", "gateway-runasnonroot-false", lambda o: pod(o, "gateway")["securityContext"].update({"runAsNonRoot": False}))
mut("AC-12", "event-pod-runasuser", lambda o: pod(o, "event")["securityContext"].update({"runAsUser": 65532}))
mut("AC-12", "identity-container-uid-0", lambda o: ctr(o, "identity")["securityContext"].update({"runAsUser": 0}))
mut("AC-12", "ticketing-seccomp-missing", lambda o: pod(o, "ticketing")["securityContext"].pop("seccompProfile"))
mut("AC-12", "waitingroom-automount-true", lambda o: pod(o, "waitingroom").update({"automountServiceAccountToken": True}))
mut("AC-12", "notification-privesc-allowed", lambda o: ctr(o, "notification")["securityContext"].update({"allowPrivilegeEscalation": True}))
mut("AC-12", "ai-worker-no-drop-all", lambda o: ctr(o, "ai-worker")["securityContext"].pop("capabilities"))
mut("AC-12", "worker-privileged", lambda o: ctr(o, "ticketing-worker")["securityContext"].update({"privileged": True}))
mut("AC-12", "gateway-writable-rootfs", lambda o: ctr(o, "gateway")["securityContext"].update({"readOnlyRootFilesystem": False}))
mut("AC-12", "identity-hostnetwork", lambda o: pod(o, "identity").update({"hostNetwork": True}))
mut("AC-12", "ai-worker-hostpath", lambda o: pod(o, "ai-worker")["volumes"].append({"name": "h", "hostPath": {"path": "/var/run"}}))
mut("AC-12", "ai-worker-no-tmp-emptydir", ai_no_tmp)
# AC-14
mut("AC-14", "staging-enforce-baseline", lambda o: find(o, "Namespace", NS)["metadata"]["labels"].update({"pod-security.kubernetes.io/enforce": "baseline"}), overlay="staging")
mut("AC-14", "staging-enforce-missing", lambda o: find(o, "Namespace", NS)["metadata"]["labels"].pop("pod-security.kubernetes.io/enforce"), overlay="staging")
mut("AC-14", "local-warn-missing", lambda o: find(o, "Namespace", NS)["metadata"]["labels"].pop("pod-security.kubernetes.io/warn"))
mut("AC-14", "local-enforce-privileged", lambda o: find(o, "Namespace", NS)["metadata"]["labels"].update({"pod-security.kubernetes.io/enforce": "privileged"}))
# AC-15
mut("AC-15", "default-deny-missing", lambda o: drop(o, "NetworkPolicy", "default-deny-ingress"))
mut("AC-15", "default-deny-selects-subset", lambda o: find(o, "NetworkPolicy", "default-deny-ingress")["spec"].update({"podSelector": sel("web")}))
mut("AC-15", "worker-allow-missing", lambda o: drop(o, "NetworkPolicy", "allow-ticketing-worker"))
mut("AC-15", "identity-empty-from", lambda o: find(o, "NetworkPolicy", "allow-identity")["spec"]["ingress"].__setitem__(0, {"ports": [{"protocol": "TCP", "port": 8083}]}))
mut("AC-15", "event-all-namespaces", lambda o: find(o, "NetworkPolicy", "allow-event")["spec"]["ingress"][0]["from"].append({"namespaceSelector": {}}))
mut("AC-15", "identity-from-web", lambda o: find(o, "NetworkPolicy", "allow-identity")["spec"]["ingress"][0]["from"].append({"podSelector": sel("web")}))
mut("AC-15", "ticketing-from-ai-worker", lambda o: find(o, "NetworkPolicy", "allow-ticketing")["spec"]["ingress"][0]["from"].append({"podSelector": sel("ai-worker")}))
# AC-16
mut("AC-16", "staging-plain-secret", lambda o: o.append(secret_obj("eventflow-db", SECRETS["eventflow-db"])), overlay="staging")
mut("AC-16", "local-unknown-secret", lambda o: o.append(secret_obj("eventflow-extra", {"X": "dev-x"})))
mut("AC-16", "local-value-without-dev", lambda o: find(o, "Secret", "eventflow-auth")["data"].update({"JWT_SECRET": b64("Zq9x7Lw3Pn5Rt8Vb2KcMd4Hs6Fj1GyUe")}))
# AC-17
mut("AC-17", "web-token-literal", lambda o: ctr(o, "web")["env"].append({"name": "API_TOKEN", "value": "abc"}))
mut("AC-17", "web-database-url-literal", lambda o: ctr(o, "web")["env"].append({"name": "DATABASE_URL", "value": "postgres://postgres/db"}))
mut("AC-17", "web-url-with-credentials", lambda o: ctr(o, "web")["env"].append({"name": "UPSTREAM_URL", "value": "http://user:pass@host:8080/x"}))
mut("AC-17", "configmap-url-with-credentials", lambda o: cfg(o, "eventflow-endpoints").update({"REDIS_URL": "redis://default:hunter2@host:6379/0"}))
mut("AC-17", "configmap-password-key", lambda o: cfg(o, "eventflow-endpoints").update({"REDIS_PASSWORD": "dev-pw"}))
# AC-18
mut("AC-18", "notification-gets-jwt", lambda o: ctr(o, "notification")["env"].append({"name": "JWT_SECRET", "valueFrom": SX(o, "eventflow-auth", "JWT_SECRET")}))
mut("AC-18", "web-envfrom-secret", lambda o: ctr(o, "web").update({"envFrom": [{"secretRef": {"name": rname(o, "Secret", "eventflow-auth")}}]}))
mut("AC-18", "ticketing-gets-amqp", lambda o: ctr(o, "ticketing")["env"].append({"name": "AMQP_URL", "valueFrom": SX(o, "eventflow-mq", "AMQP_URL")}))
mut("AC-18", "identity-gets-queue-token", lambda o: ctr(o, "identity")["env"].append({"name": "QUEUE_TOKEN_SECRET", "valueFrom": SX(o, "eventflow-auth", "QUEUE_TOKEN_SECRET")}))
mut("AC-18", "identity-uses-pooled-db", lambda o: setenv(o, "identity", "DATABASE_URL", valueFrom=SX(o, "eventflow-db", "DATABASE_URL_POOLED")))
mut("AC-18", "ticketing-uses-direct-db", lambda o: setenv(o, "ticketing", "DATABASE_URL", valueFrom=SX(o, "eventflow-db", "DATABASE_URL_DIRECT")))
# AC-19
mut("AC-19", "staging-app-env-development", lambda o: cfg(o).update({"APP_ENV": "development"}), overlay="staging")
mut("AC-19", "local-app-env-production", lambda o: cfg(o).update({"APP_ENV": "production"}))
mut("AC-19", "staging-gateway-literal-development", lambda o: setenv(o, "gateway", "APP_ENV", value="development"), overlay="staging")
# AC-20
mut("AC-20", "identity-latest", lambda o: ctr(o, "identity").update({"image": REG + "eventflow-identity:latest"}))
mut("AC-20", "event-no-tag", lambda o: ctr(o, "event").update({"image": REG + "eventflow-event"}))
mut("AC-20", "payment-semver", lambda o: ctr(o, "payment").update({"image": REG + "eventflow-payment:v1.2.3"}))
mut("AC-20", "ticketing-sha-too-long", lambda o: ctr(o, "ticketing").update({"image": REG + "eventflow-ticketing:sha-c5fb5980"}))
mut("AC-20", "gateway-docker-hub", lambda o: ctr(o, "gateway").update({"image": "docker.io/library/nginx:" + TAG}))
mut("AC-20", "notification-wrong-name", lambda o: ctr(o, "notification").update({"image": REG + "something-else:" + TAG}))
mut("AC-20", "web-pull-always", lambda o: ctr(o, "web").update({"imagePullPolicy": "Always"}))
# AC-21
mut("AC-21", "dev-infra-latest", lambda o: pgctr(o).update({"image": "postgres:latest"}))
mut("AC-21", "dev-infra-no-tag", lambda o: pgctr(o).update({"image": "postgres"}))
# AC-22
mut("AC-22", "service-matches-nothing", lambda o: find(o, "Service", "eventflow-identity")["spec"]["selector"].update({"app.kubernetes.io/name": "eventflow-idnetity"}))
mut("AC-22", "service-matches-two-workloads", lambda o: find(o, "Service", "eventflow-waitingroom")["spec"].update({"selector": {"app.kubernetes.io/part-of": "eventflow"}}))
mut("AC-22", "targetport-name-missing", lambda o: find(o, "Service", "eventflow-gateway")["spec"]["ports"][1].update({"targetPort": "prom"}))
mut("AC-22", "targetport-numeric-for-named", lambda o: find(o, "Service", "eventflow-event")["spec"]["ports"][0].update({"targetPort": 8084}))
mut("AC-22", "ingress-service-missing", lambda o: find(o, "Ingress", "eventflow-web")["spec"]["rules"][0]["http"]["paths"][0]["backend"]["service"].update({"name": "eventflow-frontend"}))
mut("AC-22", "ingress-port-missing", lambda o: find(o, "Ingress", "eventflow-gateway")["spec"]["rules"][0]["http"]["paths"][0]["backend"]["service"]["port"].update({"name": "https"}))
mut("AC-22", "hpa-target-missing", ghost_hpa)
mut("AC-22", "pdb-selects-nothing", ghost_pdb)
mut("AC-22", "secret-unknown-local", lambda o: setenv(o, "event", "RABBITMQ_URL", valueFrom=SECRET("eventflow-rabbit", "AMQP_URL")))
mut("AC-22", "secret-unknown-staging", lambda o: setenv(o, "event", "RABBITMQ_URL", valueFrom=SECRET("eventflow-rabbit", "AMQP_URL")), overlay="staging")
mut("AC-22", "secret-key-missing-local", lambda o: setenv(o, "ticketing", "HMAC_QR_SECRET", valueFrom=SX(o, "eventflow-qr", "HMAC_SECRET")))
mut("AC-22", "configmap-key-missing", lambda o: setenv(o, "waitingroom", "REDIS_ADDR", valueFrom=CX(o, "eventflow-endpoints", "REDIS_ADDRESS")))
mut("AC-22", "configmap-missing", lambda o: setenv(o, "identity", "SMTP_HOST", valueFrom={"configMapKeyRef": {"name": "eventflow-config", "key": "SMTP_HOST"}}))
# AC-23
mut("AC-23", "web-host-changed", lambda o: find(o, "Ingress", "eventflow-web")["spec"]["rules"][0].update({"host": "web.example.com"}))
mut("AC-23", "gateway-ingress-missing", lambda o: drop(o, "Ingress", "eventflow-gateway"))
mut("AC-23", "gateway-wrong-host", lambda o: find(o, "Ingress", "eventflow-gateway")["spec"]["rules"][0].update({"host": "gw.example.com"}))
mut("AC-23", "gateway-buffering-on", lambda o: find(o, "Ingress", "eventflow-gateway")["metadata"]["annotations"].update({"nginx.ingress.kubernetes.io/proxy-buffering": "on"}))
mut("AC-23", "gateway-read-timeout-60", lambda o: find(o, "Ingress", "eventflow-gateway")["metadata"]["annotations"].update({"nginx.ingress.kubernetes.io/proxy-read-timeout": "60"}))
mut("AC-23", "identity-nodeport", lambda o: find(o, "Service", "eventflow-identity")["spec"].update({"type": "NodePort"}))
mut("AC-23", "ticketing-loadbalancer", lambda o: find(o, "Service", "eventflow-ticketing")["spec"].update({"type": "LoadBalancer"}))
mut("AC-23", "identity-ingress", lambda o: o.append(ingress("eventflow-identity", "id.eventflow.localtest.me", "eventflow-identity")))
# AC-32 (staging only)
mut("AC-32", "min-sale-lead-seconds-env", lambda o: setenv(o, "event", "MIN_SALE_LEAD_SECONDS", value="0"), overlay="staging")
mut("AC-32", "min-sale-lead-seconds-configmap", lambda o: cfg(o).update({"MIN_SALE_LEAD_SECONDS": "0"}), overlay="staging", extra=("event",))
mut("AC-32", "gemini-mock-1-configmap", lambda o: cfg(o).update({"GEMINI_MOCK": "1"}), overlay="staging", extra=("ai-worker",))
mut("AC-32", "payment-sandbox-mode-env", lambda o: setenv(o, "payment", "PAYMENT_SANDBOX_MODE", value="true"), overlay="staging")
mut("AC-32", "payment-sandbox-mode-configmap", lambda o: cfg(o).update({"PAYMENT_SANDBOX_MODE": "success"}), overlay="staging", extra=("payment",))
mut("AC-32", "active-event-ids-configmap", lambda o: cfg(o).update({"ACTIVE_EVENT_IDS": "demo-event-1"}), overlay="staging", extra=("waitingroom-controller",))
mut("AC-32", "admit-min-rate-configmap", lambda o: cfg(o).update({"ADMIT_MIN_RATE": "5"}), overlay="staging", extra=("waitingroom-controller",))
mut("AC-32", "default-provider-sandbox", lambda o: cfg(o).update({"PAYMENT_DEFAULT_PROVIDER": "sandbox"}), overlay="staging", extra=("payment",))
mut("AC-32", "enabled-providers-include-sandbox", lambda o: cfg(o).update({"PAYMENT_ENABLED_PROVIDERS": "sandbox,vnpay"}), overlay="staging", extra=("payment",))
# EXEMPT
mut("EXEMPT", "unknown-pair-gateway-ac12", lambda o: exempt(o, "gateway", "AC-12", "lazy"))
mut("EXEMPT", "empty-reason-gateway-ac11", lambda o: exempt(o, "gateway", "AC-11", "   "))
mut("EXEMPT", "unknown-pair-worker-ac4", lambda o: exempt(o, "ticketing-worker", "AC-4", "no probes please"))


# ---- contract revision 2 additions (TCR-E8) ---------------------------------
def opt_ref(o, p, name):
    e = envitem(o, p, name)
    e["valueFrom"]["configMapKeyRef"]["optional"] = True


def optional_secret(o, p, name):
    envitem(o, p, name)["valueFrom"]["secretKeyRef"]["optional"] = True


def rm_env(o, p, name):
    c = ctr(o, p)
    c["env"] = [e for e in c["env"] if e["name"] != name]


def gemini_absent(o):
    opt_ref(o, "ai-worker", "GEMINI_MOCK")
    cfg(o).pop("GEMINI_MOCK")


def enabled_absent(o):
    opt_ref(o, "payment", "PAYMENT_ENABLED_PROVIDERS")
    cfg(o).pop("PAYMENT_ENABLED_PROVIDERS")


def momo_notify_absent(o):
    opt_ref(o, "payment", "MOMO_NOTIFY_URL")
    cfg(o).pop("MOMO_NOTIFY_URL")


def worker_ac9(o):
    exempt(o, "ticketing-worker", "AC-9", "x")


def web_whole_rule(o):
    exempt(o, "web", "AC-12", "whole rule is not allowed")


def web_unknown_sub(o):
    exempt(o, "web", "AC-12.runAsNonRoot", "runAsNonRoot stays true")


def web_exemption_missing(o):
    dep(o, "web")["metadata"].pop("annotations")


mut("AC-12", "web-exempt-runasnonroot-missing", lambda o: pod(o, "web")["securityContext"].pop("runAsNonRoot"))
mut("AC-12", "web-exemption-missing", web_exemption_missing)
mut("AC-12", "dev-infra-drop-all-missing", lambda o: pgctr(o)["securityContext"].pop("capabilities"))
mut("AC-12", "dev-infra-runasnonroot-false", lambda o: find(o, "StatefulSet", "eventflow-postgres")["spec"]["template"]["spec"]["securityContext"].update({"runAsNonRoot": False}))
# WP5 TCR (optional, added by WP6): dev-infra may set runAsUser, so only the rootUid sub-check can catch uid 0.
mut("AC-12", "dev-infra-run-as-uid-0", lambda o: find(o, "StatefulSet", "eventflow-postgres")["spec"]["template"]["spec"]["securityContext"].update({"runAsUser": 0}))
mut("AC-18", "local-provider-key-optional", lambda o: optional_secret(o, "payment", "VNPAY_HASH_SECRET"))
mut("AC-18", "staging-optional-secret-ref", lambda o: optional_secret(o, "ai-worker", "GEMINI_API_KEY"), overlay="staging")
mut("AC-18", "staging-stripe-key-extra", lambda o: ctr(o, "payment")["env"].append({"name": "STRIPE_SECRET_KEY", "valueFrom": SECRET("eventflow-payment", "STRIPE_SECRET_KEY")}), overlay="staging")
mut("AC-18", "staging-identity-smtp-missing", lambda o: rm_env(o, "identity", "SMTP_PASS"), overlay="staging")
mut("AC-22", "ref-uses-base-name-local", lambda o: setenv(o, "event", "RABBITMQ_URL", valueFrom=SECRET("eventflow-mq", "AMQP_URL")))
mut("AC-32", "gemini-mock-absent", gemini_absent, overlay="staging", extra=("ai-worker",))
mut("AC-32", "gemini-mock-optional-ref", lambda o: opt_ref(o, "ai-worker", "GEMINI_MOCK"), overlay="staging")
mut("AC-32", "enabled-providers-absent", enabled_absent, overlay="staging")
mut("AC-32", "stripe-enabled", lambda o: cfg(o).update({"PAYMENT_ENABLED_PROVIDERS": "vnpay,momo,stripe"}), overlay="staging", extra=("payment",))
mut("AC-32", "default-provider-not-enabled", lambda o: cfg(o).update({"PAYMENT_DEFAULT_PROVIDER": "stripe"}), overlay="staging", extra=("payment",))
mut("AC-32", "momo-secret-missing", lambda o: rm_env(o, "payment", "MOMO_SECRET_KEY"), overlay="staging")
mut("AC-32", "momo-notify-url-absent", momo_notify_absent, overlay="staging")
mut("AC-32", "email-provider-mock", lambda o: cfg(o).update({"EMAIL_PROVIDER": "mock"}), overlay="staging", extra=("notification",))
mut("AC-32", "stripe-url-in-configmap", lambda o: cfg(o).update({"STRIPE_SUCCESS_URL": "https://staging.example.com/pay/ok"}), overlay="staging", extra=("payment",))
mut("AC-32", "google-client-id-empty", lambda o: cfg(o).update({"GOOGLE_CLIENT_ID": ""}), overlay="staging", extra=("identity",))
mut("EXEMPT", "worker-ac9", worker_ac9)
mut("EXEMPT", "web-ac12-whole-rule", web_whole_rule)
mut("EXEMPT", "web-unknown-subcheck", web_unknown_sub)


# ---- Dockerfile fixtures (AC-13, --dockerfiles mode) -------------------------
DF_GOOD = {
    "go-service.Dockerfile": "FROM golang:1.23 AS build\nRUN go build -o /out/server ./cmd/server\n\nFROM gcr.io/distroless/static-debian12\nCOPY --from=build /out/server /server\nUSER 65532:65532\nENTRYPOINT [\"/server\"]\n",
    "ticketing.Dockerfile": "FROM golang:1.23 AS build\nRUN go build -o /out/server ./cmd/server\n\nFROM gcr.io/distroless/static-debian12\nCOPY --from=build /out/server /server\nCOPY --from=build /out/worker /worker\nUSER 65532:65532\nENTRYPOINT [\"/server\"]\n",
    "waitingroom.Dockerfile": "FROM golang:1.23 AS build\nRUN go build -o /out/server ./cmd/server\n\nFROM gcr.io/distroless/static-debian12\nCOPY --from=build /out/server /server\nCOPY --from=build /out/controller /controller\nUSER 65532:65532\nENTRYPOINT [\"/server\"]\n",
    "ai-worker.Dockerfile": "FROM python:3.12-slim\nRUN useradd --uid 10001 worker\nCOPY app /app\nUSER 10001\nCMD [\"uvicorn\", \"app.main:app\"]\n",
    "web.Dockerfile": "FROM node:22-alpine AS build\nRUN npm run build\n\nFROM node:22-alpine\nCOPY --from=build /app /app\nUSER 1000\nCMD [\"node\", \"server.js\"]\n",
    "migrate.Dockerfile": "FROM ghcr.io/kukymbr/goose-docker:3.22.0\nCOPY migrations /migrations\nUSER 65532:65532\n",
}
DF_BAD = {
    "named-user": {"go-service.Dockerfile": "FROM golang:1.23 AS build\nRUN go build -o /out/server ./cmd/server\n\nFROM gcr.io/distroless/static-debian12\nCOPY --from=build /out/server /server\nUSER nonroot:nonroot\nENTRYPOINT [\"/server\"]\n"},
    "no-user": {"go-service.Dockerfile": "FROM golang:1.23 AS build\nRUN go build -o /out/server ./cmd/server\n\nFROM gcr.io/distroless/static-debian12\nCOPY --from=build /out/server /server\nENTRYPOINT [\"/server\"]\n"},
    "user-only-in-build-stage": {"go-service.Dockerfile": "FROM golang:1.23 AS build\nUSER 65532:65532\nRUN go build -o /out/server ./cmd/server\n\nFROM gcr.io/distroless/static-debian12\nCOPY --from=build /out/server /server\nENTRYPOINT [\"/server\"]\n"},
}



def prune(objs, keep):
    return [x for x in objs if proc_of(x) is None or proc_of(x) in keep]


def dump(objs, overlay, procs, trailing_null=False):
    head = "# policy-args: --overlay %s%s\n" % (overlay, " --processes " + ",".join(procs) if procs else "")
    body = "".join("---\n" + json.dumps(x, separators=(",", ":"), sort_keys=True) + "\n" for x in objs)
    return head + body + ("---\n" if trailing_null else "")


def build():
    out = {}
    base = {"local": good("local"), "staging": good("staging")}
    out["good-local.yaml"] = dump(base["local"], "local", None, trailing_null=True)
    out["good-staging.yaml"] = dump(base["staging"], "staging", None)
    allp = [r[0] for r in M]
    for n, t in DF_GOOD.items():
        out["dockerfiles/good/" + n] = t
    for slug, files in DF_BAD.items():
        for n, t in files.items():
            out["dockerfiles/bad/AC-13--%s/%s" % (slug, n)] = t
    for rule, slug, overlay, fn, extra in MUT:
        b = copy.deepcopy(base[overlay])
        o = copy.deepcopy(b)
        fn(o)
        old = {key(x): x for x in b}
        new = {key(x): x for x in o}
        assert old != new, "no-op mutation %s %s" % (rule, slug)
        procs = {"web", "gateway"} | set(extra)
        for k in set(old) | set(new):
            if old.get(k) != new.get(k):
                p = proc_of(new.get(k) or old[k])
                if p:
                    procs.add(p)
        o = prune(o, procs)
        path = "bad/%s--%s.yaml" % (rule, slug)
        assert path not in out, path
        out[path] = dump(o, overlay, [p for p in allp if p in procs])
    return out


def main():
    files = build()
    if "--check" in sys.argv:
        bad = 0
        for k, v in files.items():
            p = os.path.join(HERE, k)
            if not os.path.exists(p) or open(p, newline="").read().replace(chr(13) + chr(10), chr(10)) != v:
                print("OUT-OF-SYNC", k)
                bad += 1
        for d in ("bad", "dockerfiles"):
            for dp, _, fns in os.walk(os.path.join(HERE, d)):
                for f in fns:
                    rel = os.path.relpath(os.path.join(dp, f), HERE).replace(os.sep, "/")
                    if rel not in files:
                        print("STRAY", rel)
                        bad += 1
        sys.exit(1 if bad else 0)
    import shutil
    for d in ("bad", "dockerfiles"):
        shutil.rmtree(os.path.join(HERE, d), ignore_errors=True)
    for k, v in files.items():
        os.makedirs(os.path.dirname(os.path.join(HERE, k)), exist_ok=True)
        with open(os.path.join(HERE, k), "w", newline="\n") as fh:
            fh.write(v)
    print("wrote %d fixtures (%d bad)" % (len(files), len(MUT)))


if __name__ == "__main__":
    main()
