# gmutate.py ROOT OUTDIR : writes 6 gates.sh C4.4 mutants (G1-G6) to OUTDIR and runs, for each, the repo's own
# test (.github/scripts/tests/test-gates-context-guard.sh with SUT=<mutant>) and the verifier probe (gprobe.sh).
# The worktree gates.sh is never written. Prints one line per mutant: id|desc|repo test|probe.
import os, subprocess, sys
ROOT, OUT = sys.argv[1], sys.argv[2]
G = os.path.join(ROOT, ".github/scripts/gates.sh")
TEST = os.path.join(ROOT, ".github/scripts/tests/test-gates-context-guard.sh")
PROBE = os.path.join(ROOT, "docs/workflow/k8s-all/verify-logs/tools/gprobe.sh")
GUARD = '[[ "$K8S_CTX" =~ ^(docker-desktop|kind-.+)$ ]]'
M = [
    ("G0", "control (unmutated)", None, None),
    ("G1", "context guard accepts any context", GUARD, '[[ "$K8S_CTX" =~ .* ]]'),
    ("G2", "context guard regex unanchored at end", GUARD, '[[ "$K8S_CTX" =~ ^(docker-desktop|kind-.+) ]]'),
    ("G3", "kubectl calls not pinned to approved context", 'kc() { kubectl --context "$K8S_CTX" "$@"; }', 'kc() { kubectl "$@"; }'),
    ("G4", "migrate Job deleted even when not finished",
     'if [[ "${succeeded:-0}" =~ ^[0-9]+$ ]] && [ "${succeeded:-0}" -ge 1 ]; then', 'if true; then'),
    ("G5", "unfinished Job not excluded from dry-run", "sel_args=( -l 'app.kubernetes.io/name!=eventflow-migrate' )", "sel_args=()"),
    ("G6", "guard skipped: non-local branch removed", 'if [ -n "$K8S_CTX" ] && ! ' + GUARD + '; then', 'if false; then'),
]
src = open(G, "rb").read().decode("utf-8")
os.makedirs(OUT, exist_ok=True)
for mid, desc, old, new in M:
    s = src
    if old is not None:
        if src.count(old) != 1:
            print("%s|%s|BAD-MUTANT count=%d|" % (mid, desc, src.count(old)), flush=True)
            continue
        s = src.replace(old, new)
    p = os.path.join(OUT, mid + ".gates.sh")
    open(p, "wb").write(s.encode("utf-8"))
    t = subprocess.run(["bash", TEST, ROOT], env=dict(os.environ, SUT=p), capture_output=True, text=True, errors="replace")
    g = subprocess.run(["bash", PROBE, p, ROOT], capture_output=True, text=True, errors="replace")
    open(os.path.join(OUT, mid + ".log"), "w").write(t.stdout + t.stderr + "\n--- gprobe\n" + g.stdout + g.stderr)
    first = next((l for l in t.stdout.splitlines() if l.startswith("FAIL")), "-")
    want_pass = old is None
    rt = ("PASS" if t.returncode == 0 else "FAIL") if want_pass else ("KILLED" if t.returncode != 0 else "SURVIVED")
    pr = ("PASS" if g.returncode == 0 else "FAIL") if want_pass else ("KILLED" if g.returncode != 0 else "SURVIVED")
    print("%s|%s|repo test %s (exit %d; first: %s)|probe %s" % (mid, desc, rt, t.returncode, first[:110], pr), flush=True)
