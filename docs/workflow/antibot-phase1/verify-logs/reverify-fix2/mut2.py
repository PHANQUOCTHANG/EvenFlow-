import json, os, subprocess, sys, time

ROOT = "D:/Documents/Nam4-1/Project1/EvenFlow-antibot"
LOGDIR = ROOT + "/docs/workflow/antibot-phase1/verify-logs/reverify-fix2"
os.makedirs(LOGDIR, exist_ok=True)
ENV = dict(os.environ)
ENV.pop("VITEST_MAX_THREADS", None)
ENV.pop("VITEST_MAX_FORKS", None)
SUITES = {
    "G": (ROOT + "/libs/go", ["go", "test", "-count=1", "./antibot/..."]),
    "W": (ROOT + "/services/waitingroom", ["go", "test", "-count=1", "./internal/adapter/http/", "./cmd/server/", "./internal/adapter/postgres/"]),
    "WEB": (ROOT + "/apps/web", ["npx.cmd", "vitest", "run", "src/lib/antibot-signals.test.ts", "src/lib/antibot-signals.fixround1.test.ts",
             "src/components/queue/waiting-room.antibot.test.tsx", "src/components/event/event-action-panel.antibot.test.tsx",
             "src/lib/session.contract.test.ts", "src/lib/antibot-signals.fixround2.test.ts", "src/components/queue/waiting-room.fixround2.test.tsx"]),
}

muts = json.load(open(sys.argv[1], encoding="utf-8"))
only = set(sys.argv[2:])
results = []
for m in muts:
    if only and m["id"] not in only:
        continue
    path = ROOT + "/" + m["file"]
    orig = open(path, "rb").read()
    text = orig.decode("utf-8")
    old = m["old"].replace("\n", "\r\n") if "\r\n" in text and m["old"] not in text else m["old"]
    new = m["new"].replace("\n", "\r\n") if old != m["old"] else m["new"]
    n = text.count(old)
    if n != 1:
        results.append({"id": m["id"], "status": "APPLY_FAIL(%d)" % n})
        print(m["id"], "APPLY_FAIL", n, flush=True)
        continue
    open(path, "wb").write(text.replace(old, new).encode("utf-8"))
    cwd, cmd = SUITES[m["suite"]]
    t0 = time.time()
    try:
        p = subprocess.run(cmd, cwd=cwd, env=ENV, capture_output=True, text=True, encoding="utf-8", errors="replace", timeout=600)
        out, rc = p.stdout + p.stderr, p.returncode
    except subprocess.TimeoutExpired:
        out, rc = "TIMEOUT", 124
    finally:
        open(path, "wb").write(orig)
    fails = [l.strip() for l in out.splitlines() if l.strip().startswith("--- FAIL") or " FAIL " in l or "✗" in l or "×" in l][:8]
    st = "KILLED" if rc != 0 else "SURVIVED"
    open(LOGDIR + "/" + m["id"] + ".log", "w", encoding="utf-8").write(out[-20000:])
    results.append({"id": m["id"], "status": st, "rc": rc, "secs": round(time.time() - t0), "fails": fails})
    print(m["id"], st, rc, fails[:3], flush=True)
json.dump(results, open(LOGDIR + "/results-" + (sys.argv[1].split("/")[-1].split("\\")[-1]), "w"), indent=1)
