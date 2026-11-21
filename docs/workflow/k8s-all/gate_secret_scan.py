"""Secret scan for the k8s-all full gate (reads a unified diff on stdin).

A hit is a credential-looking token on an added line. The only secrets allowed are DEV ONLY values:
deploy/k8s/overlays/local/secrets.dev.env itself (excluded from the diff by the caller), exact copies of its values,
and values that follow its DEV ONLY convention (spec AC-16: the value, plain or base64-decoded, starts with "dev-"),
which is what the generated policy fixtures carry. The bad policy fixtures also carry two deliberate fake
non-dev values (they exist to prove AC-16 / AC-17 reject them); those are allowed only in the listed test/review
paths and are listed by name below. Anything else fails. Exit 0 clean, 1 hits.
"""
import base64
import binascii
import re
import sys

DEV_ENV = "deploy/k8s/overlays/local/secrets.dev.env"
STRONG = re.compile(r"AKIA[0-9A-Z]{16}|-----BEGIN [A-Z ]*PRIVATE KEY|ghp_[A-Za-z0-9]{36}|xox[baprs]-|"
                    r"AIza[0-9A-Za-z_-]{35}|sk_live_[0-9A-Za-z]{10,}")
KV = re.compile(r"(?i)(secret|password|passwd|token|api[_-]?key|_key)[\"' ]*[:=][\"' ]*([A-Za-z0-9/+=_.:@-]{16,})")


# deliberate fakes that bad fixtures use to trip AC-16 / AC-17, allowed only under FAKE_PATHS
FAKES = {"prod-real-secret", "Zq9x7Lw3Pn5Rt8Vb2KcMd4Hs6Fj1GyUe"}
FAKE_PATHS = ("deploy/k8s/policy/testdata/bad/", "deploy/k8s/policy/testdata/gen_fixtures.py",
              "docs/workflow/k8s-all/findings-")


def dev_values():
    vals = set()
    with open(DEV_ENV, encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            v = line.split("=", 1)[1].strip()
            if v:
                vals.add(v)
    return vals


def decode(v):
    try:
        return base64.b64decode(v, validate=True).decode("utf-8")
    except (binascii.Error, ValueError, UnicodeDecodeError):
        return None


def main():
    allowed = dev_values()
    hits, ok, fakes = [], 0, 0
    path = ""
    for n, line in enumerate(sys.stdin, 1):
        if line.startswith("+++ "):
            path = line[4:].strip()
            path = path[2:] if path.startswith("b/") else path
            continue
        if not line.startswith("+"):
            continue
        if STRONG.search(line):
            hits.append("%d: %s" % (n, line.strip()[:160]))
            continue
        for m in KV.finditer(line):
            v = m.group(2).rstrip("\"',")
            dv = decode(v)
            if v in allowed or dv in allowed or v.startswith("dev-") or (dv or "").startswith("dev-"):
                ok += 1
                continue
            if (v in FAKES or dv in FAKES) and path.startswith(FAKE_PATHS):
                fakes += 1
                continue
            hits.append("%d %s: %s=%s" % (n, path, m.group(1), v[:60]))
    print("dev-only values matched (DEV ONLY, %s convention): %d" % (DEV_ENV, ok))
    print("deliberate fake values in bad fixtures / review notes: %d" % fakes)
    if hits:
        print("HITS:")
        print("\n".join(hits))
        return 1
    print("no secrets in diff")
    return 0


if __name__ == "__main__":
    sys.exit(main())
