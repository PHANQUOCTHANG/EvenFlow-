"""Compare two pip-audit JSON reports (HEAD, BASE); exit 1 if HEAD has an advisory BASE lacks."""
import json
import sys


def load(path):
    data = json.load(open(path, encoding="utf-8"))
    deps = data["dependencies"] if isinstance(data, dict) else data
    return {(d["name"], d.get("version"), v["id"]) for d in deps for v in d.get("vulns", [])}


head, base = load(sys.argv[1]), load(sys.argv[2])
base_keys = {(n, i) for n, _, i in base}
new = sorted(x for x in head if (x[0], x[2]) not in base_keys)
print("advisories HEAD:", sorted(head) or "none")
print("advisories BASE:", sorted(base) or "none")
print("new vs base:", new or "none")
sys.exit(1 if new else 0)
