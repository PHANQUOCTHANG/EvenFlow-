#!/usr/bin/env python3
"""EventFlow Kubernetes policy checker (spec docs/workflow/k8s-all/spec.md, contract policy-contract.md rev 2).

    python3 -I check_policy.py --overlay {local|staging} --input <file.json|-> [--processes <csv>]
    python3 -I check_policy.py --dockerfiles <dir>

Input is a JSON array of Kubernetes objects produced by `yq ea -o=json '[.]' render.yaml` (null entries ignored).
One rule id = one check function (`check_ac<n>`), registered in RULES; see rules.md for the rule text.
Output: one `FAIL <rule> <Kind>/<name>: <reason>` line per violation and a summary line.
Exit 0 = no violation, 1 = violations, 2 = usage or input error.  Python stdlib only.
"""
import argparse
import base64
import binascii
import json
import os
import re
import sys

NS = "eventflow"
REG = "ghcr.io/phanquocthang/"
EXEMPT_PREFIX = "eventflow.io/policy-exempt"
EXEMPT_RE = re.compile(r"^eventflow\.io/policy-exempt\.(AC-[0-9]+)(\.([A-Za-z]+))?$")
TIER_LABEL = "eventflow.io/tier"
SUFFIX_RE = re.compile(r"-[a-z0-9]{10}$")
WORKLOAD_KINDS = ("Deployment", "StatefulSet", "Job")
OVERLAYS = ("local", "staging")

# ---------------------------------------------------------------- spec 4.1 (+ E4) contract tables
# process: image, port name used by probes, ports, liveness, readiness, startup, app grace (s), hpa (min,max) or None,
#          fixed replicas or None, requests (cpu, mem), limits (cpu, mem)
PROCESSES = {
    "web": ("eventflow-web", "http", {"http": 3000}, "/api/healthz", "/api/readyz", "/api/healthz", 5, (2, 6), None,
            ("100m", "256Mi"), ("500m", "512Mi")),
    "gateway": ("eventflow-gateway", "http", {"http": 8080, "metrics": 9094}, "/healthz", "/readyz", "/healthz", 20,
                (2, 10), None, ("200m", "128Mi"), ("1000m", "256Mi")),
    "identity": ("eventflow-identity", "http", {"http": 8083}, "/healthz", "/healthz", "/healthz", 0, (2, 6), None,
                 ("100m", "128Mi"), ("1000m", "256Mi")),
    "event": ("eventflow-event", "http", {"http": 8084}, "/healthz", "/healthz", "/healthz", 5, (2, 6), None,
              ("100m", "128Mi"), ("500m", "256Mi")),
    "waitingroom": ("eventflow-waitingroom", "http", {"http": 8081}, "/healthz", "/readyz", "/healthz", 20, (2, 10),
                    None, ("200m", "128Mi"), ("1000m", "256Mi")),
    "waitingroom-controller": ("eventflow-waitingroom", "metrics", {"metrics": 9091}, "/metrics", "/metrics",
                               "/metrics", 0, None, 2, ("50m", "64Mi"), ("250m", "128Mi")),
    "ticketing": ("eventflow-ticketing", "http", {"http": 8082}, "/healthz", "/readyz", "/healthz", 20, (2, 10), None,
                  ("200m", "256Mi"), ("1000m", "512Mi")),
    "ticketing-worker": ("eventflow-ticketing", "metrics", {"metrics": 9092}, "/metrics", "/metrics", "/metrics", 0,
                         None, 1, ("100m", "128Mi"), ("500m", "256Mi")),
    "payment": ("eventflow-payment", "http", {"http": 8085}, "/healthz", "/healthz", "/healthz", 10, (2, 6), None,
                ("100m", "128Mi"), ("500m", "256Mi")),
    "notification": ("eventflow-notification", "http", {"http": 8087}, "/healthz", "/healthz", "/healthz", 10, (2, 4),
                     None, ("50m", "64Mi"), ("250m", "128Mi")),
    "ai-worker": ("eventflow-ai-worker", "http", {"http": 8090}, "/healthz", "/readyz", "/healthz", 20, (2, 6), None,
                  ("200m", "256Mi"), ("1000m", "512Mi")),
}
ALL_PROCESSES = list(PROCESSES)
REQUEST_SERVING = {"web", "gateway", "identity", "event", "waitingroom", "ticketing", "payment", "ai-worker"}
GO_PROCESSES = {"gateway", "identity", "event", "waitingroom", "waitingroom-controller", "ticketing",
                "ticketing-worker", "payment", "notification"}
READINESS_PATHS = ("/readyz", "/api/readyz")

# contract section 3: allowed exemption pairs (rule, sub-check or None)
EXEMPT_ALLOWED_BY_NAME = {
    "eventflow-web": {("AC-12", "readOnlyRootFilesystem")},
    "eventflow-ticketing-worker": {("AC-11", None)},
}
EXEMPT_ALLOWED_DEV_INFRA = {("AC-6", None), ("AC-10", None), ("AC-11", None), ("AC-20", None),
                            ("AC-12", "runAsNonRoot")}

# contract section 8: secret env per process -> {env name: (secret base name, key)}
_DB_DIRECT = ("eventflow-db", "DATABASE_URL_DIRECT")
_DB_POOLED = ("eventflow-db", "DATABASE_URL_POOLED")
_AMQP = ("eventflow-mq", "AMQP_URL")
_VNPAY_MOMO = ("VNPAY_TMN_CODE", "VNPAY_HASH_SECRET", "MOMO_PARTNER_CODE", "MOMO_ACCESS_KEY", "MOMO_SECRET_KEY")
_STRIPE = ("STRIPE_PUBLISHABLE_KEY", "STRIPE_SECRET_KEY", "STRIPE_WEBHOOK_SECRET")
SECRET_ENV_BOTH = {
    "gateway": {"JWT_SECRET": ("eventflow-auth", "JWT_SECRET")},
    "identity": {"DATABASE_URL": _DB_DIRECT, "JWT_SECRET": ("eventflow-auth", "JWT_SECRET"),
                 "PII_ENC_KEY": ("eventflow-identity", "PII_ENC_KEY"),
                 "PII_HASH_KEY": ("eventflow-identity", "PII_HASH_KEY")},
    "event": {"DATABASE_URL": _DB_DIRECT, "RABBITMQ_URL": _AMQP},
    "waitingroom": {"QUEUE_TOKEN_SECRET": ("eventflow-auth", "QUEUE_TOKEN_SECRET")},
    "waitingroom-controller": {},
    "ticketing": {"DATABASE_URL": _DB_POOLED, "QUEUE_TOKEN_SECRET": ("eventflow-auth", "QUEUE_TOKEN_SECRET"),
                  "HMAC_QR_SECRET": ("eventflow-qr", "HMAC_QR_SECRET")},
    "ticketing-worker": {"DATABASE_URL": _DB_POOLED, "RABBITMQ_URL": _AMQP,
                         "HMAC_QR_SECRET": ("eventflow-qr", "HMAC_QR_SECRET")},
    "payment": {"DATABASE_URL": _DB_POOLED, "RABBITMQ_URL": _AMQP},
    "notification": {"DATABASE_URL": _DB_POOLED, "AMQP_URL": _AMQP},
    "ai-worker": {"AMQP_URL": _AMQP, "GEMINI_API_KEY": ("eventflow-ai", "GEMINI_API_KEY")},
    "web": {},
}
SECRET_ENV_LOCAL = {"payment": {k: ("eventflow-payment", k) for k in _VNPAY_MOMO + _STRIPE}}
SECRET_ENV_STAGING = {
    "identity": {"SMTP_USER": ("eventflow-identity", "SMTP_USER"), "SMTP_PASS": ("eventflow-identity", "SMTP_PASS")},
    "notification": {"SMTP_USER": ("eventflow-notification", "SMTP_USER"),
                     "SMTP_PASSWORD": ("eventflow-notification", "SMTP_PASSWORD")},
}
# staging payment provider keys: AC-18 checks only for extras; presence is AC-32
STAGING_PROVIDER_SECRETS = {k: ("eventflow-payment", k) for k in _VNPAY_MOMO}

LOCAL_SECRETS = {"eventflow-db", "eventflow-mq", "eventflow-auth", "eventflow-qr", "eventflow-identity",
                 "eventflow-payment", "eventflow-ai", "eventflow-dev-infra"}
# staging external Secrets (AC-22): keys from any column of section 8
EXTERNAL_SECRET_KEYS = {
    "eventflow-db": {"DATABASE_URL_DIRECT", "DATABASE_URL_POOLED"},
    "eventflow-mq": {"AMQP_URL"},
    "eventflow-auth": {"JWT_SECRET", "QUEUE_TOKEN_SECRET"},
    "eventflow-qr": {"HMAC_QR_SECRET"},
    "eventflow-identity": {"PII_ENC_KEY", "PII_HASH_KEY", "SMTP_USER", "SMTP_PASS"},
    "eventflow-payment": set(_VNPAY_MOMO + _STRIPE),
    "eventflow-ai": {"GEMINI_API_KEY"},
    "eventflow-notification": {"SMTP_USER", "SMTP_PASSWORD"},
}

SECRET_NAME_RE = re.compile(r"(?i)(PASSWORD|SECRET|TOKEN|_KEY$|API_KEY)")
CRED_URL_RE = re.compile(r"://[^/@\s]+:[^/@\s]+@")
ALWAYS_SECRET_ENV = {"DATABASE_URL", "AMQP_URL", "RABBITMQ_URL"}

PSA = {
    "staging": {"pod-security.kubernetes.io/enforce": "restricted"},
    "local": {"pod-security.kubernetes.io/enforce": "baseline", "pod-security.kubernetes.io/warn": "restricted",
              "pod-security.kubernetes.io/audit": "restricted"},
}

# spec 6.2: destination -> list of (peer, allowed ports); peer = ("pod", process) | ("ns", name) | ("mon",)
MON = ("mon",)
INGRESS_NS = ("ns", "ingress-nginx")


def _netpol_matrix():
    m = {"web": [(INGRESS_NS, {3000})],
         "gateway": [(INGRESS_NS, {8080}), (("pod", "web"), {8080}), (("pod", "ai-worker"), {8080}), (MON, {9094})],
         "ai-worker": [(("pod", "gateway"), {8090}), (MON, {8090})],
         "notification": [(MON, {8087})],
         "waitingroom-controller": [(MON, {9091})],
         "ticketing-worker": [(MON, {9092})]}
    for p in ("identity", "event", "waitingroom", "ticketing", "payment"):
        port = PROCESSES[p][2]["http"]
        m[p] = [(("pod", "gateway"), {port}), (MON, {port})]
    m["event"].append((("pod", "ai-worker"), {8084}))
    return m


NETPOL_MATRIX = _netpol_matrix()
NO_OPEN_FROM = {"identity", "event", "waitingroom", "ticketing", "payment", "ai-worker"}

# AC-32
FORBIDDEN_EXACT = {"MIN_SALE_LEAD_SECONDS", "ACTIVE_EVENT_IDS", "PAYMENT_SANDBOX_MODE"}
FORBIDDEN_PREFIX = ("ADMIT_", "STRIPE_")
PROVIDER_URLS = {"vnpay": ("VNPAY_RETURN_URL",), "momo": ("MOMO_RETURN_URL", "MOMO_NOTIFY_URL")}
PROVIDER_SECRETS = {"vnpay": ("VNPAY_TMN_CODE", "VNPAY_HASH_SECRET"),
                    "momo": ("MOMO_PARTNER_CODE", "MOMO_ACCESS_KEY", "MOMO_SECRET_KEY")}
STAGING_PROVIDERS = {"vnpay", "momo"}

AC12_SUBCHECKS = ("runAsNonRoot", "seccomp", "automount", "allowPrivilegeEscalation", "dropAll", "privileged",
                  "hostNamespaces", "hostPath", "rootUid", "noRunAsUser", "readOnlyRootFilesystem", "tmpEmptyDir")

CLUSTER_SCOPED = {"Namespace", "ClusterRole", "ClusterRoleBinding", "PersistentVolume", "StorageClass",
                  "CustomResourceDefinition", "PriorityClass", "IngressClass", "ValidatingWebhookConfiguration",
                  "MutatingWebhookConfiguration", "APIService", "RuntimeClass", "CSIDriver", "VolumeSnapshotClass"}


class InputError(Exception):
    pass


# ---------------------------------------------------------------- helpers
def base_name(name):
    return SUFFIX_RE.sub("", name or "")


def meta(o):
    m = o.get("metadata")
    return m if isinstance(m, dict) else {}


def name_of(o):
    return str(meta(o).get("name") or "unnamed")


def labels_of(o):
    return meta(o).get("labels") or {}


def d(x):
    return x if isinstance(x, dict) else {}


def lst(x):
    return x if isinstance(x, list) else []


def selector_matches(selector, labels):
    """Kubernetes LabelSelector semantics. An empty selector matches everything; None matches nothing."""
    if not isinstance(selector, dict):
        return False
    for k, v in d(selector.get("matchLabels")).items():
        if labels.get(k) != v:
            return False
    for e in lst(selector.get("matchExpressions")):
        e = d(e)
        k, op, vals = e.get("key"), e.get("operator"), lst(e.get("values"))
        if op == "In" and labels.get(k) not in vals:
            return False
        if op == "NotIn" and k in labels and labels[k] in vals:
            return False
        if op == "Exists" and k not in labels:
            return False
        if op == "DoesNotExist" and k in labels:
            return False
    return True


def parse_quantity(q):
    if q is None:
        return None
    s = str(q).strip()
    units = {"m": 1e-3, "k": 1e3, "M": 1e6, "G": 1e9, "T": 1e12, "Ki": 1024.0, "Mi": 1024.0 ** 2,
             "Gi": 1024.0 ** 3, "Ti": 1024.0 ** 4}
    for u in sorted(units, key=len, reverse=True):
        if s.endswith(u):
            try:
                return float(s[:-len(u)]) * units[u]
            except ValueError:
                return None
    try:
        return float(s)
    except ValueError:
        return None


class Workload:
    def __init__(self, obj):
        self.obj = obj
        self.kind = obj.get("kind")
        self.name = name_of(obj)
        self.spec = d(obj.get("spec"))
        tmpl = d(self.spec.get("template"))
        if self.kind == "Job":
            pass
        self.pod_meta = d(tmpl.get("metadata"))
        self.pod = d(tmpl.get("spec"))
        self.pod_labels = d(self.pod_meta.get("labels"))
        self.dev_infra = labels_of(obj).get(TIER_LABEL) == "dev-infra"
        self.process = None
        if not self.dev_infra and self.name.startswith("eventflow-") and self.name[len("eventflow-"):] in PROCESSES:
            self.process = self.name[len("eventflow-"):]
        self.containers = [d(c) for c in lst(self.pod.get("containers"))]
        self.init_containers = [d(c) for c in lst(self.pod.get("initContainers"))]
        self.all_containers = self.init_containers + self.containers
        self.exempt = set()  # valid (rule, sub) pairs

    @property
    def app(self):
        return not self.dev_infra

    def exempt_rule(self, rule):
        return (rule, None) in self.exempt

    def exempt_sub(self, rule, sub):
        return (rule, sub) in self.exempt


class Ctx:
    def __init__(self, objs, overlay, processes):
        self.objs = objs
        self.overlay = overlay
        self.processes = processes
        self.violations = []
        self.workloads = [Workload(o) for o in objs if o.get("kind") in WORKLOAD_KINDS]
        self.by_kind = {}
        for o in objs:
            self.by_kind.setdefault(o.get("kind"), []).append(o)
        self.app_deploy = {}
        for w in self.workloads:
            if w.kind == "Deployment" and w.process and w.process not in self.app_deploy:
                self.app_deploy[w.process] = w
        self.configmaps = {name_of(o): o for o in self.kind("ConfigMap")}
        self.secrets = {name_of(o): o for o in self.kind("Secret")}
        self._load_exemptions()

    def kind(self, k):
        return self.by_kind.get(k, [])

    def fail(self, rule, kind, name, reason):
        self.violations.append((rule, kind or "Unknown", name, reason))

    def render_fail(self, rule, reason):
        self.fail(rule, "Render", self.overlay, reason)

    def wfail(self, rule, w, reason):
        self.fail(rule, w.kind, w.name, reason)

    def wanted(self, p):
        return p in self.processes

    def _load_exemptions(self):
        """Parse exemption annotations into Workload.exempt; check_exempt reports the invalid ones."""
        self.invalid_exemptions = []
        for w in self.workloads:
            anns = d(meta(w.obj).get("annotations"))
            allowed = set(EXEMPT_ALLOWED_BY_NAME.get(w.name, set()))
            if w.dev_infra:
                allowed |= EXEMPT_ALLOWED_DEV_INFRA
            for k, v in anns.items():
                if not k.startswith(EXEMPT_PREFIX):
                    continue
                m = EXEMPT_RE.match(k)
                if not m:
                    self.invalid_exemptions.append((w, "malformed exemption key %r" % k))
                    continue
                pair = (m.group(1), m.group(3))
                if pair[0] == "AC-9":
                    self.invalid_exemptions.append((w, "AC-9 is not exemptable (%s)" % k))
                    continue
                if pair not in allowed:
                    self.invalid_exemptions.append((w, "exemption %s is not allowed for this workload" % k))
                    continue
                if not isinstance(v, str) or not v.strip():
                    self.invalid_exemptions.append((w, "exemption %s has an empty reason" % k))
                    continue
                w.exempt.add(pair)

    # value resolution ------------------------------------------------
    def cm_by_base(self, name):
        if name in self.configmaps:
            return self.configmaps[name]
        for n, o in self.configmaps.items():
            if base_name(n) == base_name(name):
                return o
        return None

    def secret_by_base(self, name):
        if name in self.secrets:
            return self.secrets[name]
        for n, o in self.secrets.items():
            if base_name(n) == base_name(name):
                return o
        return None


def secret_keys(o):
    return set(d(o.get("data"))) | set(d(o.get("stringData")))


def env_items(c):
    return [d(e) for e in lst(c.get("env"))]


def container_ports(w):
    out = {}
    for c in w.containers:
        for p in lst(c.get("ports")):
            p = d(p)
            if p.get("name"):
                out[p["name"]] = p.get("containerPort")
    return out


def probe_http(probe):
    h = d(d(probe).get("httpGet"))
    return h if h else None


def is_workload_of(w, labels):
    return selector_matches({"matchLabels": labels}, w.pod_labels) if labels else False


# ---------------------------------------------------------------- rules
def check_ac1(ctx):
    app = [w for w in ctx.workloads if w.kind == "Deployment" and w.app]
    names = sorted(w.name for w in app)
    want = sorted("eventflow-" + p for p in ctx.processes)
    if names != want:
        missing = sorted(set(want) - set(names))
        extra = sorted(set(names) - set(want))
        detail = []
        if missing:
            detail.append("missing " + ",".join(missing))
        if extra:
            detail.append("unexpected " + ",".join(extra))
        if len(names) != len(set(names)):
            detail.append("duplicate names")
        ctx.render_fail("AC-1", "%d app Deployments, want %d (%s)" % (len(names), len(want), "; ".join(detail)))
    nss = ctx.kind("Namespace")
    if len(nss) != 1 or name_of(nss[0]) != NS:
        ctx.render_fail("AC-1", "want exactly one Namespace %s, got %s" % (NS, ",".join(name_of(n) for n in nss) or "none"))
    for o in ctx.objs:
        if o.get("kind") in CLUSTER_SCOPED:
            continue
        ns = meta(o).get("namespace")
        if ns != NS:
            ctx.fail("AC-1", o.get("kind"), name_of(o), "namespace %r, want %s" % (ns, NS))


def check_ac3(ctx):
    if not ctx.wanted("web"):
        return
    w = ctx.app_deploy.get("web")
    if w is None:
        return  # AC-1 reports the missing Deployment
    want = {"app.kubernetes.io/name": "eventflow-web", "app.kubernetes.io/part-of": "eventflow"}
    if d(w.spec.get("selector")) != {"matchLabels": want}:
        ctx.wfail("AC-3", w, "selector changed (immutable): %s" % json.dumps(w.spec.get("selector"), sort_keys=True))
    c = [x for x in w.containers if x.get("name") == "web"]
    if not c:
        ctx.wfail("AC-3", w, "no container named web")
    else:
        c = c[0]
        if "http" not in {d(p).get("name") for p in lst(c.get("ports"))}:
            ctx.wfail("AC-3", w, "container web has no port named http")
        for probe, path in (("livenessProbe", "/api/healthz"), ("readinessProbe", "/api/readyz"),
                            ("startupProbe", "/api/healthz")):
            h = probe_http(c.get(probe))
            if h is not None and h.get("path") != path:
                ctx.wfail("AC-3", w, "%s path %r, want %s" % (probe, h.get("path"), path))
    st = d(w.spec.get("strategy"))
    ru = d(st.get("rollingUpdate"))
    if st.get("type", "RollingUpdate") != "RollingUpdate" or ru.get("maxUnavailable") != 0 or ru.get("maxSurge") != 1:
        ctx.wfail("AC-3", w, "strategy must stay RollingUpdate maxUnavailable 0 / maxSurge 1")
    for kind in ("Service", "Ingress", "HorizontalPodAutoscaler"):
        if not any(name_of(o) == "eventflow-web" for o in ctx.kind(kind)):
            ctx.fail("AC-3", kind, "eventflow-web", "web %s missing or renamed" % kind)


def check_ac4(ctx):
    for w in ctx.workloads:
        if w.kind == "Job":
            continue
        for c in w.containers:
            cn = c.get("name", "?")
            for probe in ("livenessProbe", "readinessProbe") + (("startupProbe",) if w.app else ()):
                if not isinstance(c.get(probe), dict):
                    ctx.wfail("AC-4", w, "container %s has no %s" % (cn, probe))
            if not w.process:
                continue
            row = PROCESSES[w.process]
            for probe, path in (("livenessProbe", row[3]), ("readinessProbe", row[4]), ("startupProbe", row[5])):
                pr = c.get(probe)
                if not isinstance(pr, dict):
                    continue
                h = probe_http(pr)
                if h is None:
                    ctx.wfail("AC-4", w, "container %s %s is not httpGet %s" % (cn, probe, path))
                    continue
                if h.get("path") != path:
                    ctx.wfail("AC-4", w, "container %s %s path %r, want %s" % (cn, probe, h.get("path"), path))
                if h.get("port") != row[1]:
                    ctx.wfail("AC-4", w, "container %s %s port %r, want port name %s" % (cn, probe, h.get("port"), row[1]))


def check_ac5(ctx):
    for w in ctx.workloads:
        for c in w.containers:
            cn = c.get("name", "?")
            live = probe_http(c.get("livenessProbe"))
            ready = probe_http(c.get("readinessProbe"))
            if live and live.get("path") in READINESS_PATHS:
                ctx.wfail("AC-5", w, "container %s liveness probe points at readiness path %s" % (cn, live.get("path")))
            if w.process and PROCESSES[w.process][4] in READINESS_PATHS and live and ready \
                    and live.get("path") == ready.get("path"):
                ctx.wfail("AC-5", w, "container %s readiness path equals liveness path %s" % (cn, live.get("path")))


def check_ac6(ctx):
    for w in ctx.workloads:
        if w.kind == "Job":
            continue
        if w.app and w.process not in REQUEST_SERVING:
            continue
        if w.dev_infra and w.exempt_rule("AC-6"):
            continue
        if w.kind == "Deployment":
            st = d(w.spec.get("strategy"))
            ru = d(st.get("rollingUpdate"))
            if st.get("type", "RollingUpdate") != "RollingUpdate":
                ctx.wfail("AC-6", w, "strategy %s, want RollingUpdate" % st.get("type"))
            else:
                if ru.get("maxUnavailable") not in (0, "0", "0%"):
                    ctx.wfail("AC-6", w, "maxUnavailable %r, want 0" % ru.get("maxUnavailable"))
                if ru.get("maxSurge") not in (1, "1"):
                    ctx.wfail("AC-6", w, "maxSurge %r, want 1" % ru.get("maxSurge"))
        for c in w.containers:
            if not d(d(c.get("lifecycle")).get("preStop")):
                ctx.wfail("AC-6", w, "container %s has no preStop hook" % c.get("name", "?"))
        app_grace = PROCESSES[w.process][6] if w.process else 0
        grace = w.pod.get("terminationGracePeriodSeconds", 30)
        want = 5 + app_grace + 5
        if not isinstance(grace, int) or grace < want:
            ctx.wfail("AC-6", w, "terminationGracePeriodSeconds %r, want >= %d" % (grace, want))


def check_ac7(ctx):
    for p, w in ctx.app_deploy.items():
        if not ctx.wanted(p):
            continue
        row = PROCESSES[p]
        repl = w.spec.get("replicas")
        if row[8] is not None:
            if repl != row[8]:
                ctx.wfail("AC-7", w, "replicas %r, want %d" % (repl, row[8]))
            if p == "ticketing-worker" and d(w.spec.get("strategy")).get("type") != "Recreate":
                ctx.wfail("AC-7", w, "strategy %r, want Recreate (singleton)" % d(w.spec.get("strategy")).get("type"))
        elif "replicas" in w.spec:
            ctx.wfail("AC-7", w, "HPA-managed Deployment must not set spec.replicas")


def check_ac8(ctx):
    for w in ctx.workloads:
        for c in w.all_containers:
            cn = c.get("name", "?")
            res = d(c.get("resources"))
            ok = True
            for part in ("requests", "limits"):
                for r in ("cpu", "memory"):
                    if d(res.get(part)).get(r) in (None, ""):
                        ctx.wfail("AC-8", w, "container %s has no %s.%s" % (cn, part, r))
                        ok = False
            if ok and w.process and c in w.containers:
                row = PROCESSES[w.process]
                for part, want in (("requests", row[9]), ("limits", row[10])):
                    for r, v in zip(("cpu", "memory"), want):
                        got = d(res.get(part)).get(r)
                        if parse_quantity(got) != parse_quantity(v):
                            ctx.wfail("AC-8", w, "container %s %s.%s %r, want %s" % (cn, part, r, got, v))


def check_ac9(ctx):
    hpas = ctx.kind("HorizontalPodAutoscaler")
    targets = {}
    for h in hpas:
        ref = d(d(h.get("spec")).get("scaleTargetRef"))
        targets.setdefault((ref.get("kind"), ref.get("name")), []).append(h)
        tw = [w for w in ctx.workloads if w.kind == ref.get("kind") and w.name == ref.get("name")]
        if tw:  # a missing target is AC-22
            w = tw[0]
            if not w.process or PROCESSES[w.process][7] is None:
                ctx.fail("AC-9", "HorizontalPodAutoscaler", name_of(h),
                         "targets %s/%s, which must not be autoscaled" % (w.kind, w.name))
    for p, w in ctx.app_deploy.items():
        row = PROCESSES[p]
        if row[7] is None or not ctx.wanted(p):
            continue
        hs = targets.get(("Deployment", w.name), [])
        if len(hs) != 1:
            ctx.wfail("AC-9", w, "%d HPAs target it, want exactly 1" % len(hs))
            continue
        h = hs[0]
        hn = name_of(h)
        spec = d(h.get("spec"))
        if h.get("apiVersion") != "autoscaling/v2":
            ctx.fail("AC-9", "HorizontalPodAutoscaler", hn, "apiVersion %r, want autoscaling/v2" % h.get("apiVersion"))
        mn = spec.get("minReplicas", 1)
        if not isinstance(mn, int) or mn < 2:
            ctx.fail("AC-9", "HorizontalPodAutoscaler", hn, "minReplicas %r, want >= 2" % mn)
        if spec.get("maxReplicas") != row[7][1]:
            ctx.fail("AC-9", "HorizontalPodAutoscaler", hn, "maxReplicas %r, want %d" % (spec.get("maxReplicas"), row[7][1]))
        cpu = [d(d(m.get("resource")).get("target")).get("averageUtilization") for m in map(d, lst(spec.get("metrics")))
               if m.get("type") == "Resource" and d(m.get("resource")).get("name") == "cpu"]
        if len(cpu) != 1 or not isinstance(cpu[0], int) or not 60 <= cpu[0] <= 75:
            ctx.fail("AC-9", "HorizontalPodAutoscaler", hn, "cpu averageUtilization %r, want one value in 60-75" % cpu)
        win = d(d(spec.get("behavior")).get("scaleDown")).get("stabilizationWindowSeconds")
        if not isinstance(win, int) or win < 300:
            ctx.fail("AC-9", "HorizontalPodAutoscaler", hn, "scaleDown.stabilizationWindowSeconds %r, want >= 300" % win)


def check_ac10(ctx):
    pdbs = ctx.kind("PodDisruptionBudget")
    for w in ctx.workloads:
        if w.kind not in ("Deployment", "StatefulSet"):
            continue
        if w.app and not (w.process and ctx.wanted(w.process)):
            continue
        if w.dev_infra and w.exempt_rule("AC-10"):
            continue
        sel = [p for p in pdbs if w.pod_labels and selector_matches(d(p.get("spec")).get("selector"), w.pod_labels)]
        if len(sel) != 1:
            ctx.wfail("AC-10", w, "selected by %d PDBs, want exactly 1" % len(sel))
            continue
        p = sel[0]
        spec = d(p.get("spec"))
        pn = name_of(p)
        if p.get("apiVersion") != "policy/v1":
            ctx.fail("AC-10", "PodDisruptionBudget", pn, "apiVersion %r, want policy/v1" % p.get("apiVersion"))
        if "minAvailable" in spec or spec.get("maxUnavailable") not in (1, "1"):
            ctx.fail("AC-10", "PodDisruptionBudget", pn, "want maxUnavailable: 1 and no minAvailable")
        if d(spec.get("selector")) != {"matchLabels": w.pod_labels}:
            ctx.fail("AC-10", "PodDisruptionBudget", pn, "selector must equal the pod labels of %s" % w.name)


def check_ac11(ctx):
    for w in ctx.workloads:
        if w.kind not in ("Deployment", "StatefulSet"):
            continue
        if w.app and not (w.process and ctx.wanted(w.process)):
            continue
        if w.exempt_rule("AC-11"):
            continue
        own = d(d(w.spec.get("selector")).get("matchLabels"))
        terms = lst(d(d(w.pod.get("affinity")).get("podAntiAffinity")).get("preferredDuringSchedulingIgnoredDuringExecution"))
        if not any(d(d(t).get("podAffinityTerm")).get("topologyKey") == "kubernetes.io/hostname"
                   and d(d(d(t).get("podAffinityTerm")).get("labelSelector")).get("matchLabels") == own and own
                   for t in terms):
            ctx.wfail("AC-11", w, "no preferred podAntiAffinity on kubernetes.io/hostname selecting its own labels")
        spread = lst(w.pod.get("topologySpreadConstraints"))
        if not any(d(s).get("topologyKey") == "topology.kubernetes.io/zone"
                   and d(s).get("whenUnsatisfiable") == "ScheduleAnyway"
                   and d(d(s).get("labelSelector")).get("matchLabels") == own and own for s in spread):
            ctx.wfail("AC-11", w, "no zone topologySpreadConstraint (ScheduleAnyway) selecting its own labels")


def _ac12_violations(w):
    """Yield (sub-check, reason) for one workload; exemptions are applied by the caller."""
    psc = d(w.pod.get("securityContext"))
    if psc.get("runAsNonRoot") is not True:
        yield "runAsNonRoot", "pod securityContext.runAsNonRoot is not true"
    if d(psc.get("seccompProfile")).get("type") != "RuntimeDefault":
        yield "seccomp", "pod seccompProfile.type is not RuntimeDefault"
    if w.pod.get("automountServiceAccountToken") is not False:
        yield "automount", "automountServiceAccountToken is not false"
    for k in ("hostNetwork", "hostPID", "hostIPC"):
        if w.pod.get(k):
            yield "hostNamespaces", "pod sets %s" % k
    for v in lst(w.pod.get("volumes")):
        if "hostPath" in d(v):
            yield "hostPath", "volume %s is a hostPath" % d(v).get("name")
    if psc.get("runAsUser") == 0:
        yield "rootUid", "pod runAsUser 0"
    if w.app and "runAsUser" in psc:
        yield "noRunAsUser", "app pod sets runAsUser (image uid must be used)"
    for c in w.all_containers:
        cn = c.get("name", "?")
        sc = d(c.get("securityContext"))
        if sc.get("allowPrivilegeEscalation") is not False:
            yield "allowPrivilegeEscalation", "container %s allowPrivilegeEscalation is not false" % cn
        caps = d(sc.get("capabilities"))
        if "ALL" not in lst(caps.get("drop")) or lst(caps.get("add")):
            yield "dropAll", "container %s does not drop ALL capabilities (or adds some)" % cn
        if sc.get("privileged"):
            yield "privileged", "container %s is privileged" % cn
        if sc.get("runAsUser") == 0:
            yield "rootUid", "container %s runAsUser 0" % cn
        if w.app and "runAsUser" in sc:
            yield "noRunAsUser", "app container %s sets runAsUser" % cn
        if w.app and sc.get("readOnlyRootFilesystem") is not True:
            yield "readOnlyRootFilesystem", "container %s readOnlyRootFilesystem is not true" % cn
    if w.process == "ai-worker":
        empty = {d(v).get("name") for v in lst(w.pod.get("volumes")) if "emptyDir" in d(v)}
        if not any(d(m).get("mountPath") == "/tmp" and d(m).get("name") in empty
                   for c in w.containers for m in lst(c.get("volumeMounts"))):
            yield "tmpEmptyDir", "ai-worker has no emptyDir mounted at /tmp"


def check_ac12(ctx):
    for w in ctx.workloads:
        if w.app and w.kind == "Deployment" and not (w.process and ctx.wanted(w.process)) and w.process:
            continue
        for sub, reason in _ac12_violations(w):
            if w.exempt_sub("AC-12", sub):
                continue
            ctx.wfail("AC-12", w, "%s: %s" % (sub, reason))


def _final_stage_user(text):
    lines, buf = [], ""
    for raw in text.splitlines():
        s = raw.rstrip()
        if s.endswith("\\"):
            buf += s[:-1] + " "
            continue
        lines.append(buf + s)
        buf = ""
    if buf:
        lines.append(buf)
    user, seen_from = None, False
    for ln in lines:
        s = ln.strip()
        if not s or s.startswith("#"):
            continue
        word = s.split(None, 1)
        instr = word[0].upper()
        if instr == "FROM":
            user, seen_from = None, True
        elif instr == "USER":
            user = word[1].strip() if len(word) > 1 else ""
    return seen_from, user


def check_ac13(dirpath, ctx):
    files = sorted(f for f in os.listdir(dirpath) if f.endswith(".Dockerfile") and os.path.isfile(os.path.join(dirpath, f)))
    for f in files:
        name = f[:-len(".Dockerfile")].lower()
        try:
            with open(os.path.join(dirpath, f), encoding="utf-8") as fh:
                text = fh.read()
        except OSError as e:
            raise InputError("cannot read %s: %s" % (f, e))
        seen_from, user = _final_stage_user(text)
        if not seen_from:
            ctx.fail("AC-13", "Dockerfile", name, "no FROM instruction")
        elif user is None:
            ctx.fail("AC-13", "Dockerfile", name, "final stage has no USER instruction")
        elif not re.match(r"^[0-9]+(:[0-9]+)?$", user):
            ctx.fail("AC-13", "Dockerfile", name, "final stage USER %r is not numeric uid[:gid]" % user)
    return len(files)


def check_ac14(ctx):
    for n in ctx.kind("Namespace"):
        if name_of(n) != NS:
            continue
        labels = labels_of(n)
        for k, v in PSA[ctx.overlay].items():
            if labels.get(k) != v:
                ctx.fail("AC-14", "Namespace", NS, "label %s=%r, want %s" % (k, labels.get(k), v))


def _peer(peer):
    """Normalise a NetworkPolicy peer to a matrix peer tuple, or None if it is not one of the allowed shapes."""
    peer = d(peer)
    if set(peer) - {"podSelector", "namespaceSelector"} or not peer:
        return None
    ps, nss = peer.get("podSelector"), peer.get("namespaceSelector")
    if ps is not None and nss is None:
        ps = d(ps)
        name = d(ps.get("matchLabels")).get("app.kubernetes.io/name", "")
        if ps.get("matchExpressions") or not name.startswith("eventflow-"):
            return None
        return ("pod", name[len("eventflow-"):])
    if nss is not None and ps is None:
        nss = d(nss)
        ml = d(nss.get("matchLabels"))
        if nss.get("matchExpressions"):
            return None
        if ml == {"eventflow.io/monitoring": "true"}:
            return MON
        if list(ml) == ["kubernetes.io/metadata.name"]:
            return ("ns", ml["kubernetes.io/metadata.name"])
    return None


def check_ac15(ctx):
    nps = ctx.kind("NetworkPolicy")
    deny = [n for n in nps if d(n.get("spec")).get("podSelector") == {} and "Ingress" in lst(d(n.get("spec")).get("policyTypes"))
            and not lst(d(n.get("spec")).get("ingress"))]
    if not deny:
        ctx.render_fail("AC-15", "no default-deny ingress NetworkPolicy selecting all pods")
    for p, w in ctx.app_deploy.items():
        if not ctx.wanted(p):
            continue
        sel = [n for n in nps if lst(d(n.get("spec")).get("ingress"))
               and selector_matches(d(n.get("spec")).get("podSelector"), w.pod_labels)]
        if not sel:
            ctx.wfail("AC-15", w, "no allow NetworkPolicy selects its pods")
        ports = container_ports(w)
        allowed = NETPOL_MATRIX.get(p, [])
        for n in sel:
            nn = name_of(n)
            for rule in map(d, lst(d(n.get("spec")).get("ingress"))):
                frm = lst(rule.get("from"))
                if not frm:
                    ctx.fail("AC-15", "NetworkPolicy", nn, "ingress rule with empty from reaches %s" % w.name)
                    continue
                rports = []
                for rp in map(d, lst(rule.get("ports"))):
                    v = rp.get("port")
                    rports.append(ports.get(v) if isinstance(v, str) else v)
                for peer in frm:
                    np_ = _peer(peer)
                    ok_ports = set()
                    for ap, aports in allowed:
                        if ap == np_:
                            ok_ports |= aports
                    if np_ is None or not ok_ports:
                        ctx.fail("AC-15", "NetworkPolicy", nn, "peer %s is not allowed to reach %s (spec 6.2)"
                                 % (json.dumps(peer, sort_keys=True), w.name))
                    elif not rports or any(x not in ok_ports for x in rports):
                        ctx.fail("AC-15", "NetworkPolicy", nn, "peer %s may reach %s only on %s, rule allows %s"
                                 % (json.dumps(peer, sort_keys=True), w.name, sorted(ok_ports), rports or "all ports"))


def check_ac16(ctx):
    for s in ctx.kind("Secret"):
        n = name_of(s)
        if ctx.overlay == "staging":
            ctx.fail("AC-16", "Secret", n, "staging must not render Secret objects (external)")
            continue
        if base_name(n) not in LOCAL_SECRETS:
            ctx.fail("AC-16", "Secret", n, "Secret %s is not in the allowed local set" % base_name(n))
        for k, v in d(s.get("data")).items():
            try:
                val = base64.b64decode(str(v), validate=False).decode("utf-8", "replace")
            except (binascii.Error, ValueError):
                val = ""
            if "dev" not in val:
                ctx.fail("AC-16", "Secret", n, "value of %s does not contain 'dev'" % k)
        for k, v in d(s.get("stringData")).items():
            if "dev" not in str(v):
                ctx.fail("AC-16", "Secret", n, "value of %s does not contain 'dev'" % k)


def check_ac17(ctx):
    for w in ctx.workloads:
        for c in w.all_containers:
            for e in env_items(c):
                if "value" not in e:
                    continue
                n, v = str(e.get("name", "")), str(e.get("value") or "")
                if SECRET_NAME_RE.search(n):
                    ctx.wfail("AC-17", w, "env %s is a secret-like literal; use secretKeyRef" % n)
                elif n in ALWAYS_SECRET_ENV:
                    ctx.wfail("AC-17", w, "env %s must come from a Secret" % n)
                if CRED_URL_RE.search(v):
                    ctx.wfail("AC-17", w, "env %s literal contains URL credentials" % n)
    for cm in ctx.kind("ConfigMap"):
        for k, v in d(cm.get("data")).items():
            if SECRET_NAME_RE.search(str(k)):
                ctx.fail("AC-17", "ConfigMap", name_of(cm), "key %s looks like a secret" % k)
            if CRED_URL_RE.search(str(v)):
                ctx.fail("AC-17", "ConfigMap", name_of(cm), "value of %s contains URL credentials" % k)


def _secret_resolves_by_base(ctx, name, key):
    if ctx.overlay == "staging" and not ctx.secrets:
        return key in EXTERNAL_SECRET_KEYS.get(base_name(name), set())
    s = ctx.secret_by_base(name)
    if s is not None:
        return key in secret_keys(s)
    return ctx.overlay == "staging" and key in EXTERNAL_SECRET_KEYS.get(base_name(name), set())


def check_ac18(ctx):
    for w in ctx.workloads:
        for c in w.all_containers:
            for e in env_items(c):
                ref = d(d(e.get("valueFrom")).get("secretKeyRef"))
                if ref.get("optional") is True:
                    ctx.wfail("AC-18", w, "env %s secretKeyRef is optional" % e.get("name"))
            for ef in map(d, lst(c.get("envFrom"))):
                if "secretRef" in ef:
                    ctx.wfail("AC-18", w, "container %s uses envFrom.secretRef" % c.get("name", "?"))
    for p, w in ctx.app_deploy.items():
        if not ctx.wanted(p):
            continue
        want = dict(SECRET_ENV_BOTH[p])
        extra_ok = {}
        if ctx.overlay == "local":
            want.update(SECRET_ENV_LOCAL.get(p, {}))
        else:
            want.update(SECRET_ENV_STAGING.get(p, {}))
            if p == "payment":
                extra_ok = STAGING_PROVIDER_SECRETS
        got, unresolved = {}, set()
        for c in w.all_containers:
            for e in env_items(c):
                ref = d(d(e.get("valueFrom")).get("secretKeyRef"))
                if not ref:
                    continue
                n = e.get("name")
                if not _secret_resolves_by_base(ctx, str(ref.get("name")), ref.get("key")):
                    unresolved.add(n)  # AC-22 reports it
                    continue
                got[n] = (base_name(ref.get("name")), ref.get("key"))
        for n, src in sorted(want.items()):
            if n in unresolved:
                continue
            if n not in got:
                ctx.wfail("AC-18", w, "missing secret env %s <- %s/%s" % (n, src[0], src[1]))
            elif got[n] != src:
                ctx.wfail("AC-18", w, "secret env %s <- %s/%s, want %s/%s" % (n, got[n][0], got[n][1], src[0], src[1]))
        for n, src in sorted(got.items()):
            if n in want:
                continue
            if extra_ok.get(n) == src:
                continue
            ctx.wfail("AC-18", w, "unexpected secret env %s <- %s/%s" % (n, src[0], src[1]))


def _env_value(ctx, e):
    """(kind, value, optional) of an env item: kind in literal/configmap/secret/missing/other."""
    if "value" in e:
        return "literal", str(e.get("value") or ""), False
    vf = d(e.get("valueFrom"))
    cref = d(vf.get("configMapKeyRef"))
    if cref:
        cm = ctx.cm_by_base(str(cref.get("name")))
        data = d(cm.get("data")) if cm else {}
        if cref.get("key") in data:
            return "configmap", str(data[cref.get("key")]), cref.get("optional") is True
        return "missing", None, cref.get("optional") is True
    sref = d(vf.get("secretKeyRef"))
    if sref:
        return "secret", None, sref.get("optional") is True
    return "other", None, False


def check_ac19(ctx):
    want = "production" if ctx.overlay == "staging" else "development"
    for p, w in ctx.app_deploy.items():
        if p not in GO_PROCESSES or not ctx.wanted(p):
            continue
        items = [e for c in w.containers for e in env_items(c) if e.get("name") == "APP_ENV"]
        if not items:
            ctx.wfail("AC-19", w, "APP_ENV not set, want %s" % want)
            continue
        for e in items:
            cref = d(d(e.get("valueFrom")).get("configMapKeyRef"))
            if cref and base_name(str(cref.get("name"))) != "eventflow-app-config":
                ctx.wfail("AC-19", w, "APP_ENV must come from eventflow-app-config, not %s" % cref.get("name"))
                continue
            kind, val, _ = _env_value(ctx, e)
            if val != want:
                ctx.wfail("AC-19", w, "APP_ENV=%r, want %s" % (val, want))


def check_ac20(ctx):
    for w in ctx.workloads:
        if w.dev_infra and w.exempt_rule("AC-20"):
            continue
        if w.app and w.kind == "Deployment" and w.process and not ctx.wanted(w.process):
            continue
        img_name = PROCESSES[w.process][0] if w.process else None
        for c in w.all_containers:
            img = str(c.get("image") or "")
            name_re = re.escape(img_name) if img_name else r"eventflow-[a-z0-9-]+"
            if not re.match(r"^%s%s(:sha-[0-9a-f]{7}|@sha256:[0-9a-f]{64})$" % (re.escape(REG), name_re), img):
                ctx.wfail("AC-20", w, "container %s image %r, want %s%s:sha-<7 hex> or @sha256 digest"
                          % (c.get("name", "?"), img, REG, img_name or "eventflow-<name>"))
            if c.get("imagePullPolicy") != "IfNotPresent":
                ctx.wfail("AC-20", w, "container %s imagePullPolicy %r, want IfNotPresent"
                          % (c.get("name", "?"), c.get("imagePullPolicy")))


def check_ac21(ctx):
    for w in ctx.workloads:
        if not w.dev_infra:
            continue
        for c in w.all_containers:
            img = str(c.get("image") or "")
            if "@sha256:" in img:
                continue
            last = img.rsplit("/", 1)[-1]
            tag = last.split(":", 1)[1] if ":" in last else ""
            if not tag or tag == "latest":
                ctx.wfail("AC-21", w, "container %s image %r has no pinned tag" % (c.get("name", "?"), img))


def check_ac22(ctx):
    pod_workloads = [w for w in ctx.workloads if w.kind in ("Deployment", "StatefulSet")]
    services = {name_of(s): s for s in ctx.kind("Service")}
    for s in ctx.kind("Service"):
        spec = d(s.get("spec"))
        sel = spec.get("selector")
        if not sel:
            continue
        hits = [w for w in pod_workloads if selector_matches({"matchLabels": sel}, w.pod_labels)]
        if len(hits) != 1:
            ctx.fail("AC-22", "Service", name_of(s), "selector matches %d workloads, want 1" % len(hits))
            continue
        w = hits[0]
        names = {}
        for c in w.containers:
            for p in map(d, lst(c.get("ports"))):
                names.setdefault(p.get("containerPort"), set()).add(p.get("name"))
        pnames = container_ports(w)
        for sp in map(d, lst(spec.get("ports"))):
            tp = sp.get("targetPort", sp.get("port"))
            if isinstance(tp, str):
                if tp not in pnames:
                    ctx.fail("AC-22", "Service", name_of(s), "targetPort %s is not a port name of %s" % (tp, w.name))
            else:
                named = {n for n in names.get(tp, set()) if n}
                if named:
                    ctx.fail("AC-22", "Service", name_of(s), "targetPort %r must use the port name %s" % (tp, sorted(named)[0]))
                elif tp not in names:
                    ctx.fail("AC-22", "Service", name_of(s), "targetPort %r is not a container port of %s" % (tp, w.name))
    for ing in ctx.kind("Ingress"):
        backends = []
        spec = d(ing.get("spec"))
        if d(spec.get("defaultBackend")).get("service"):
            backends.append(d(d(spec.get("defaultBackend")).get("service")))
        for r in map(d, lst(spec.get("rules"))):
            for path in map(d, lst(d(r.get("http")).get("paths"))):
                backends.append(d(d(path.get("backend")).get("service")))
        for b in backends:
            svc = services.get(b.get("name"))
            if svc is None:
                ctx.fail("AC-22", "Ingress", name_of(ing), "backend Service %s does not exist" % b.get("name"))
                continue
            port = d(b.get("port"))
            sports = [d(p) for p in lst(d(svc.get("spec")).get("ports"))]
            if "name" in port and port["name"] not in {p.get("name") for p in sports}:
                ctx.fail("AC-22", "Ingress", name_of(ing), "Service %s has no port %s" % (b.get("name"), port["name"]))
            if "number" in port and port["number"] not in {p.get("port") for p in sports}:
                ctx.fail("AC-22", "Ingress", name_of(ing), "Service %s has no port %s" % (b.get("name"), port["number"]))
    for h in ctx.kind("HorizontalPodAutoscaler"):
        ref = d(d(h.get("spec")).get("scaleTargetRef"))
        if not any(w.kind == ref.get("kind") and w.name == ref.get("name") for w in ctx.workloads):
            ctx.fail("AC-22", "HorizontalPodAutoscaler", name_of(h),
                     "scaleTargetRef %s/%s does not exist" % (ref.get("kind"), ref.get("name")))
    for p in ctx.kind("PodDisruptionBudget"):
        sel = d(p.get("spec")).get("selector")
        if not any(w.pod_labels and selector_matches(sel, w.pod_labels) for w in pod_workloads):
            ctx.fail("AC-22", "PodDisruptionBudget", name_of(p), "selector matches no workload")
    for w in ctx.workloads:
        for c in w.all_containers:
            for e in env_items(c):
                vf = d(e.get("valueFrom"))
                sref, cref = d(vf.get("secretKeyRef")), d(vf.get("configMapKeyRef"))
                if sref:
                    _check_secret_ref(ctx, w, e.get("name"), sref.get("name"), sref.get("key"))
                if cref:
                    cm = ctx.configmaps.get(cref.get("name"))
                    if cm is None:
                        ctx.wfail("AC-22", w, "env %s: ConfigMap %s is not rendered" % (e.get("name"), cref.get("name")))
                    elif cref.get("key") not in d(cm.get("data")) and cref.get("optional") is not True:
                        ctx.wfail("AC-22", w, "env %s: ConfigMap %s has no key %s" % (e.get("name"), cref.get("name"), cref.get("key")))
            for ef in map(d, lst(c.get("envFrom"))):
                if "secretRef" in ef:
                    _check_secret_ref(ctx, w, "envFrom", d(ef["secretRef"]).get("name"), None)
                if "configMapRef" in ef and d(ef["configMapRef"]).get("name") not in ctx.configmaps \
                        and d(ef["configMapRef"]).get("optional") is not True:
                    ctx.wfail("AC-22", w, "envFrom: ConfigMap %s is not rendered" % d(ef["configMapRef"]).get("name"))
        for v in map(d, lst(w.pod.get("volumes"))):
            if "configMap" in v and d(v["configMap"]).get("name") not in ctx.configmaps \
                    and d(v["configMap"]).get("optional") is not True:
                ctx.wfail("AC-22", w, "volume %s: ConfigMap %s is not rendered" % (v.get("name"), d(v["configMap"]).get("name")))
            if "secret" in v and d(v["secret"]).get("optional") is not True:
                _check_secret_ref(ctx, w, "volume " + str(v.get("name")), d(v["secret"]).get("secretName"), None)


def _check_secret_ref(ctx, w, what, name, key):
    s = ctx.secrets.get(name)
    if s is not None:
        if key is not None and key not in secret_keys(s):
            ctx.wfail("AC-22", w, "%s: Secret %s has no key %s" % (what, name, key))
        return
    if ctx.overlay == "staging" and name in EXTERNAL_SECRET_KEYS:
        if key is not None and key not in EXTERNAL_SECRET_KEYS[name]:
            ctx.wfail("AC-22", w, "%s: external Secret %s has no key %s in the contract" % (what, name, key))
        return
    ctx.wfail("AC-22", w, "%s: Secret %s is not %s" % (what, name, "rendered" if ctx.overlay == "local"
                                                         else "rendered or a contract external Secret"))


def check_ac23(ctx):
    allowed_svc = {"eventflow-web", "eventflow-gateway"}
    services = {name_of(s) for s in ctx.kind("Service")}
    ings = {}
    for ing in ctx.kind("Ingress"):
        n = name_of(ing)
        ings[n] = ing
        if n not in allowed_svc:
            ctx.fail("AC-23", "Ingress", n, "only eventflow-web and eventflow-gateway may have an Ingress")
            continue
        spec = d(ing.get("spec"))
        for r in map(d, lst(spec.get("rules"))):
            for path in map(d, lst(d(r.get("http")).get("paths"))):
                b = d(d(path.get("backend")).get("service")).get("name")
                if b in services and b not in allowed_svc:
                    ctx.fail("AC-23", "Ingress", n, "exposes Service %s" % b)
    for svc, host, anns in (("eventflow-web", "eventflow.localtest.me", False),
                            ("eventflow-gateway", "api.eventflow.localtest.me", True)):
        if not ctx.wanted(svc[len("eventflow-"):]):
            continue
        ing = ings.get(svc)
        if ing is None:
            ctx.fail("AC-23", "Ingress", svc, "Ingress missing")
            continue
        hosts = [d(r).get("host") for r in lst(d(ing.get("spec")).get("rules"))]
        if hosts != [host]:
            ctx.fail("AC-23", "Ingress", svc, "hosts %s, want [%s]" % (hosts, host))
        if anns:
            a = d(meta(ing).get("annotations"))
            if a.get("nginx.ingress.kubernetes.io/proxy-buffering") != "off":
                ctx.fail("AC-23", "Ingress", svc, "proxy-buffering must be \"off\" (SSE)")
            try:
                t = int(str(a.get("nginx.ingress.kubernetes.io/proxy-read-timeout")))
            except ValueError:
                t = 0
            if t < 3600:
                ctx.fail("AC-23", "Ingress", svc, "proxy-read-timeout must be >= 3600 (SSE)")
    for s in ctx.kind("Service"):
        t = d(s.get("spec")).get("type", "ClusterIP")
        if t in ("NodePort", "LoadBalancer"):
            ctx.fail("AC-23", "Service", name_of(s), "type %s exposes the Service outside the cluster" % t)


def _forbidden(key):
    return key in FORBIDDEN_EXACT or key.startswith(FORBIDDEN_PREFIX)


def check_ac32(ctx):
    if ctx.overlay != "staging":
        return
    for cm in ctx.kind("ConfigMap"):
        for k in d(cm.get("data")):
            if _forbidden(k):
                ctx.fail("AC-32", "ConfigMap", name_of(cm), "forbidden staging key %s" % k)
    for w in ctx.workloads:
        for c in w.all_containers:
            for e in env_items(c):
                n = str(e.get("name", ""))
                if not _forbidden(n):
                    continue
                kind, _, _ = _env_value(ctx, e)
                if kind in ("literal", "configmap"):
                    ctx.wfail("AC-32", w, "forbidden staging setting %s is set (%s)" % (n, kind))

    def present(w, key):
        """value if present (literal or non-optional existing ConfigMap key), True for a non-optional secret ref."""
        for c in w.containers:
            for e in env_items(c):
                if e.get("name") != key:
                    continue
                kind, val, opt = _env_value(ctx, e)
                if kind == "literal" or (kind == "configmap" and not opt):
                    return val
                if kind == "secret" and not opt:
                    return True
        return None

    def require(p, key, check, what):
        w = ctx.app_deploy.get(p)
        if w is None or not ctx.wanted(p):
            return None
        v = present(w, key)
        if v is None or v is True or not check(v):
            ctx.wfail("AC-32", w, "%s must be present with %s (got %s)" % (key, what, "absent" if v is None else repr(v)))
            return None
        return v

    require("ai-worker", "GEMINI_MOCK", lambda v: v == "0", '"0"')
    require("notification", "EMAIL_PROVIDER", lambda v: v == "smtp", '"smtp"')
    require("identity", "GOOGLE_CLIENT_ID", lambda v: v.strip() != "", "a non-empty value")
    w = ctx.app_deploy.get("payment")
    if w is not None and ctx.wanted("payment"):
        enabled = require("payment", "PAYMENT_ENABLED_PROVIDERS",
                          lambda v: {x.strip() for x in v.split(",") if x.strip()} == STAGING_PROVIDERS, '"vnpay,momo"')
        provs = sorted(STAGING_PROVIDERS if enabled is None else {x.strip() for x in enabled.split(",") if x.strip()})
        require("payment", "PAYMENT_DEFAULT_PROVIDER", lambda v: v in provs, "one of the enabled providers %s" % provs)
        for prov in provs:
            for k in PROVIDER_URLS.get(prov, ()):
                require("payment", k, lambda v: v.strip() != "", "a non-empty value")
            for k in PROVIDER_SECRETS.get(prov, ()):
                if present(w, k) is not True:
                    ctx.wfail("AC-32", w, "%s must come from a non-optional secretKeyRef" % k)


def check_exempt(ctx):
    for w, reason in ctx.invalid_exemptions:
        ctx.wfail("EXEMPT", w, reason)


RULES = [
    ("AC-1", check_ac1), ("AC-3", check_ac3), ("AC-4", check_ac4), ("AC-5", check_ac5), ("AC-6", check_ac6),
    ("AC-7", check_ac7), ("AC-8", check_ac8), ("AC-9", check_ac9), ("AC-10", check_ac10), ("AC-11", check_ac11),
    ("AC-12", check_ac12), ("AC-14", check_ac14), ("AC-15", check_ac15), ("AC-16", check_ac16),
    ("AC-17", check_ac17), ("AC-18", check_ac18), ("AC-19", check_ac19), ("AC-20", check_ac20),
    ("AC-21", check_ac21), ("AC-22", check_ac22), ("AC-23", check_ac23), ("AC-32", check_ac32),
    ("EXEMPT", check_exempt),
]


# ---------------------------------------------------------------- CLI
class Parser(argparse.ArgumentParser):
    def error(self, message):
        sys.stderr.write("ERROR %s\n" % message)
        sys.exit(2)


def load_input(path):
    try:
        if path == "-":
            raw = sys.stdin.read()
        else:
            with open(path, encoding="utf-8") as fh:
                raw = fh.read()
    except OSError as e:
        raise InputError("cannot read input %s: %s" % (path, e.strerror or e))
    try:
        data = json.loads(raw)
    except ValueError as e:
        raise InputError("input is not valid JSON: %s" % e)
    if not isinstance(data, list):
        raise InputError("input must be a JSON array of Kubernetes objects")
    objs = []
    for i, o in enumerate(data):
        if o is None:
            continue
        if not isinstance(o, dict) or not isinstance(o.get("kind"), str):
            raise InputError("entry %d is not a Kubernetes object" % i)
        objs.append(o)
    return objs


def emit(violations, n, label):
    for rule, kind, name, reason in violations:
        safe = re.sub(r"[^a-z0-9.-]", "-", name.lower()) or "unnamed"
        kind = re.sub(r"[^A-Za-z]", "", kind) or "Unknown"
        print("FAIL %s %s/%s: %s" % (rule, kind, safe, reason.replace("\n", " ")))
    if violations:
        print("policy: %d violation(s) (overlay=%s)" % (len(violations), label))
        return 1
    print("policy: OK (%d objects, overlay=%s)" % (n, label))
    return 0


def main(argv=None):
    ap = Parser(prog="check_policy.py", description="EventFlow k8s policy checker")
    ap.add_argument("--overlay", choices=OVERLAYS)
    ap.add_argument("--input")
    ap.add_argument("--processes")
    ap.add_argument("--dockerfiles")
    a = ap.parse_args(argv)
    try:
        if a.dockerfiles is not None:
            if a.overlay or a.input or a.processes:
                ap.error("--dockerfiles cannot be combined with --overlay/--input/--processes")
            if not os.path.isdir(a.dockerfiles):
                raise InputError("Dockerfile directory not found: %s" % a.dockerfiles)
            ctx = Ctx([], "dockerfiles", [])
            n = check_ac13(a.dockerfiles, ctx)
            return emit(ctx.violations, n, "dockerfiles")
        if not a.overlay:
            ap.error("--overlay is required")
        if not a.input:
            ap.error("--input is required")
        procs = ALL_PROCESSES
        if a.processes is not None:
            procs = [p.strip() for p in a.processes.split(",") if p.strip()]
            unknown = [p for p in procs if p not in PROCESSES]
            if unknown or not procs:
                raise InputError("unknown process name(s): %s" % (",".join(unknown) or "(empty)"))
        objs = load_input(a.input)
    except InputError as e:
        sys.stderr.write("ERROR %s\n" % e)
        return 2
    ctx = Ctx(objs, a.overlay, procs)
    for _, fn in RULES:
        fn(ctx)
    return emit(ctx.violations, len(objs), a.overlay)


if __name__ == "__main__":
    sys.exit(main())
