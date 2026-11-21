import subprocess, sys, os, re
ROOT = sys.argv[1]; OUT = sys.argv[2]; only = sys.argv[3:] 
CP = os.path.join(ROOT, "deploy/k8s/policy/check_policy.py")
M = [
 ("M01","AC-12 pod runAsNonRoot", 'if psc.get("runAsNonRoot") is not True:', 'if False:'),
 ("M02","AC-13 USER root uid 0", 'elif int(user.split(":", 1)[0]) == 0:', 'elif False:'),
 ("M03","AC-13 USER root gid 0", 'elif ":" in user and int(user.split(":", 1)[1]) == 0:', 'elif False:'),
 ("M04","AC-13 USER non-numeric", 'elif not re.match(r"^[0-9]+(:[0-9]+)?$", user):', 'elif False:'),
 ("M05","AC-15 NetworkPolicy endPort", 'if "endPort" in rp:', 'if False:'),
 ("M06","AC-15 empty from", '                if not frm:\n                    ctx.fail("AC-15"', '                if False:\n                    ctx.fail("AC-15"'),
 ("M07","AC-17 DSN password regex", 'DSN_PASSWORD_RE = re.compile(r"(?i)(password|passwd|pwd)=[^\s&]")', 'DSN_PASSWORD_RE = re.compile(r"(?!x)x")'),
 ("M08","AC-17 split-flag args", 'split_flag = (ARG_SECRET_FLAG_RE.search(a)', 'split_flag = False and (ARG_SECRET_FLAG_RE.search(a)'),
 ("M09","AC-17 credential URL regex", 'CRED_URL_RE = re.compile(r"://[^/@\s]*:[^/@\s]+@")', 'CRED_URL_RE = re.compile(r"(?!x)x")'),
 ("M10","AC-18 envFrom secretRef", '                if "secretRef" in ef:\n                    ctx.wfail("AC-18"', '                if False:\n                    ctx.wfail("AC-18"'),
 ("M11","AC-18 optional secretKeyRef", 'if ref.get("optional") is True:', 'if False:'),
 ("M12","AC-20 pod kinds list (drop Pod/DaemonSet/ReplicaSet/RC/CronJob)", 'WORKLOAD_KINDS = ("Deployment", "StatefulSet", "Job", "Pod", "DaemonSet", "ReplicaSet", "ReplicationController",\n                  "CronJob")', 'WORKLOAD_KINDS = ("Deployment", "StatefulSet", "Job")'),
 ("M13","AC-20 pod kinds list (drop CronJob only)", '"ReplicationController",\n                  "CronJob")', '"ReplicationController")'),
 ("M14","AC-21 dev-infra in staging", 'if ctx.overlay == "staging":  # rev 3 (X-4/R-6)', 'if False:  # rev 3 (X-4/R-6)'),
 ("M15","AC-22 duplicate env", 'if n in seen:', 'if False:'),
 ("M16","AC-32 GEMINI_MOCK must be 0", 'lambda v: v == "0", \'"0"\'', 'lambda v: True, \'"0"\''),
 ("M17","AC-32 EMAIL_PROVIDER smtp", 'lambda v: v == "smtp"', 'lambda v: True'),
 ("M18","AC-32 GOOGLE_CLIENT_ID non-empty", '"GOOGLE_CLIENT_ID", lambda v: v.strip() != ""', '"GOOGLE_CLIENT_ID", lambda v: True'),
 ("M28","AC-32 provider URL/secret presence", 'require("payment", k, lambda v: v.strip() != ""', 'require("payment", k, lambda v: True'),
 ("M19","AC-32 envFrom configMapRef in staging", '                if "configMapRef" in ef:\n                    ctx.wfail("AC-32"', '                if False:\n                    ctx.wfail("AC-32"'),
 ("M20","AC-32 forbidden ConfigMap key", '            if _forbidden(k):\n                ctx.fail("AC-32"', '            if False:\n                ctx.fail("AC-32"'),
 ("M21","EXEMPT pair not allowed", 'if pair not in allowed:', 'if False:'),
 ("M22","EXEMPT empty reason", 'if not isinstance(v, str) or not v.strip():', 'if False:'),
 ("M23","EXEMPT malformed key accepted as whole-rule", '                if not m:\n                    self.invalid_exemptions.append((w, "malformed exemption key %r" % k))\n                    continue', '                if not m:\n                    continue'),
 ("M24","AC-9 non-exemptable (guard dropped + AC-9 put in worker allow-list)", 'if pair[0] == "AC-9":', 'if False:'),
 ("M25","AC-9 HPA on fixed-replica workload", 'if not w.process or PROCESSES[w.process][7] is None:', 'if False:'),
 ("M26","AC-16 Secret in staging", '        if ctx.overlay == "staging":\n            ctx.fail("AC-16"', '        if False:\n            ctx.fail("AC-16"'),
 ("M27","AC-12 web exemption hard-coded (sub-check ignored)", 'return (rule, sub) in self.exempt', 'return (rule, sub) in self.exempt or self.name == "eventflow-web"'),
]
EXTRA = {"M24": ('"eventflow-ticketing-worker": {("AC-11", None)},', '"eventflow-ticketing-worker": {("AC-11", None), ("AC-9", None)},')}
orig = open(CP, "rb").read()
src = orig.decode("utf-8")
nl = "\r\n" if "\r\n" in src else "\n"
rows = []
for mid, desc, old, new in M:
    if only and mid not in only: continue
    o, n = old.replace("\n", nl), new.replace("\n", nl)
    if src.count(o) != 1:
        rows.append((mid, desc, "BAD-MUTANT count=%d" % src.count(o), "")); continue
    s = src.replace(o, n)
    if mid in EXTRA:
        eo, en = (x.replace("\n", nl) for x in EXTRA[mid]); assert s.count(eo) == 1; s = s.replace(eo, en)
    try:
        open(CP, "wb").write(s.encode("utf-8"))
        r = subprocess.run(["bash", "deploy/k8s/policy/test_policy.sh", "--fixtures-only"], cwd=ROOT, capture_output=True, text=True, errors="replace")
        fails = [l for l in r.stdout.splitlines() if l.startswith("FAIL")]
        open(os.path.join(OUT, mid + ".log"), "w").write(r.stdout + r.stderr)
        rows.append((mid, desc, "KILLED" if r.returncode != 0 else "SURVIVED", "exit=%d; %d FAIL; first: %s" % (r.returncode, len(fails), fails[0][:140] if fails else "-")))
    finally:
        open(CP, "wb").write(orig)
    print(rows[-1], flush=True)
open(os.path.join(OUT, "mutation-summary.txt"), "a").write("\n".join("|".join(r) for r in rows) + "\n")
