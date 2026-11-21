#!/usr/bin/env bash
# gprobe.sh GATES_SH ROOT : runs the C4.4 block of GATES_SH against a stub kubectl; prints one verdict line per probe
G="$1"; ROOT="$2"; T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
mkdir -p "$T/bin"
cat > "$T/bin/kubectl" <<'K'
#!/usr/bin/env bash
echo "$*" >> "$CALLS"
case "$*" in
  "cluster-info") exit 0;;
  "config current-context") echo "$FAKE_CTX"; exit 0;;
esac
case "$*" in
  *"get namespace"*) exit 1;;
  *"get job eventflow-migrate -o jsonpath"*) printf '%s' "$FAKE_SUCC"; exit 0;;
  *"get job eventflow-migrate"*) [ -n "$FAKE_SUCC" ]; exit $?;;
  *"apply"*"-f -"*) cat >/dev/null 2>&1; exit 0;;
esac
exit 0
K
chmod +x "$T/bin/kubectl"
{ echo 'K8S_DIR="deploy/k8s"'; echo "log()  { printf '[gates] %s\n' \"\$*\"; }"; echo "fail() { printf '[gates] FAIL: %s\n' \"\$*\" >&2; }"
  awk '/^K8S_CTX=""/{p=1} /^# C4.5 -- kiem/{p=0} p' "$G"; } > "$T/c44.sh"
run() { : > "$T/calls"; (cd "$ROOT" && CALLS="$T/calls" FAKE_CTX="$1" FAKE_SUCC="$2" PATH="$T/bin:$PATH" XDG_CACHE_HOME="$T/nocache" bash "$T/c44.sh" </dev/null >"$T/out" 2>&1); }
ok=1
run prod-cluster ""; if grep -vqE '^(cluster-info|config current-context)$' "$T/calls"; then echo "P1 FAIL non-local ctx touched cluster: $(grep -vE '^(cluster-info|config current-context)$' "$T/calls" | head -1)"; ok=0; else echo "P1 ok non-local ctx skipped"; fi
run docker-desktop-prod ""; if grep -vqE '^(cluster-info|config current-context)$' "$T/calls"; then echo "P5 FAIL ctx docker-desktop-prod touched cluster"; ok=0; else echo "P5 ok lookalike ctx skipped"; fi
run docker-desktop 0; if grep -q 'delete job' "$T/calls"; then echo "P2 FAIL unfinished migrate Job deleted"; ok=0; elif ! grep -q 'app.kubernetes.io/name!=eventflow-migrate' "$T/calls"; then echo "P2 FAIL unfinished Job not excluded"; ok=0; else echo "P2 ok unfinished Job kept and excluded"; fi
run kind-dev 1; if grep -q -- '--context kind-dev -n eventflow delete job eventflow-migrate' "$T/calls"; then echo "P3 ok finished Job deleted on pinned ctx"; else echo "P3 FAIL finished Job not deleted with --context"; ok=0; fi
if grep -vE '^(cluster-info|config current-context|version --client.*)$' "$T/calls" | grep -vq -- '^--context kind-dev '; then echo "P4 FAIL unpinned kubectl call: $(grep -vE '^(cluster-info|config current-context)$' "$T/calls" | grep -v -- '^--context kind-dev ' | head -1)"; ok=0; else echo "P4 ok all calls pinned"; fi
[ $ok = 1 ] && { echo "PROBE PASS"; exit 0; } || { echo "PROBE FAIL"; exit 1; }
