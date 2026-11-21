"""Secret scan for the k6-load-suite full gate (reads a unified diff on stdin).

A hit is a credential-looking value on an added line. Allowed: the compose DEV default JWT secret
(dev-jwt-secret-change-me-0123456789, already public in deploy/compose/docker-compose.yml) and JWTs inside
tests/load/lib/fixtures/access-token.json that verify with that DEV secret. Exit 0 clean, 1 hits.
"""
import base64
import hashlib
import hmac
import re
import sys

DEV_SECRET = "dev-jwt-secret-change-me-0123456789"
FIXTURE = "tests/load/lib/fixtures/access-token.json"
STRONG = re.compile(r"AKIA[0-9A-Z]{16}|-----BEGIN [A-Z ]*PRIVATE KEY|ghp_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9_]{20,}|"
                    r"xox[baprs]-|AIza[0-9A-Za-z_-]{35}|sk_live_[0-9A-Za-z]{10,}|sk-ant-[A-Za-z0-9_-]{10,}")
KV = re.compile(r"(?i)(secret|password|passwd|api[_-]?key|_key)[\"' ]*[:=][\"' ]*([A-Za-z0-9/+=_.:@-]{16,})")
JWT = re.compile(r"eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{16,}")


def dev_signed(tok):
    h, p, s = tok.split(".")
    mac = hmac.new(DEV_SECRET.encode(), (h + "." + p).encode(), hashlib.sha256).digest()
    return base64.urlsafe_b64encode(mac).decode().rstrip("=") == s


def main():
    hits, allowed = [], 0
    path = ""
    for n, line in enumerate(sys.stdin, 1):
        if line.startswith("+++ "):
            path = line[4:].strip()
            path = path[2:] if path.startswith("b/") else path
            continue
        if not line.startswith("+"):
            continue
        if STRONG.search(line):
            hits.append("%s:%d: %s" % (path, n, line.strip()[:160]))
            continue
        for tok in JWT.findall(line):
            if path == FIXTURE and dev_signed(tok):
                allowed += 1
            else:
                hits.append("%s:%d: JWT outside the dev fixture" % (path, n))
        for _, val in KV.findall(line):
            if DEV_SECRET in val or val.startswith("dev-") or val.startswith("${") or val.startswith("$("):
                allowed += 1
                continue
            hits.append("%s:%d: %s" % (path, n, line.strip()[:160]))
    for h in hits:
        print("HIT", h)
    print("secret scan: %d hits, %d allowed DEV values" % (len(hits), allowed))
    return 1 if hits else 0


if __name__ == "__main__":
    sys.exit(main())
