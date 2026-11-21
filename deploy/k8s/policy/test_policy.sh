#!/usr/bin/env bash
#
# Self-test of the k8s policy (spec docs/workflow/k8s-all/spec.md AC-25; interface frozen in
# docs/workflow/k8s-all/policy-contract.md).  bash + python3 stdlib; YAML->JSON with the pinned yq.
#
#   bash deploy/k8s/policy/test_policy.sh                  # everything
#   bash deploy/k8s/policy/test_policy.sh --fixtures-only  # sections A-D (what validate.sh runs)
#
# A  fixtures are in sync with testdata/gen_fixtures.py; every rule id has a bad fixture
# B  check_policy.py exists; good-local.yaml / good-staging.yaml -> exit 0 and "policy: OK" last line
# C  AC-13 dockerfile fixtures via --dockerfiles; every bad/<RULE-ID>--<slug>.yaml -> exit 1, a FAIL line for <RULE-ID>, no FAIL line for any other id,
#    FAIL lines match the contract regex, "policy: <k> violation(s)" last line; plus exit-2 usage errors
# D  AC-13: numeric USER in the four non-web Dockerfiles
# E  (not with --fixtures-only) the real tree: layout, render of both overlays with pinned kustomize,
#    kubeconform -strict, the checker on the real render, secrets.dev.env, compose guard
#
# Tools: versions from deploy/k8s/tools.env when it exists (WP5), else the pins of the contract section 6.
# Run mode go (default): `go install <module>@<version>` into a temp dir (no Docker).  A binary already
# on PATH is used when Go is missing.  Output "PASS|FAIL|SKIP [id] text"; last lines "selftest: p/t" and
# "RESULT ...".  Exit 1 on any FAIL (SKIP does not fail).
set -u
export LC_ALL=C

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
POL="$ROOT/deploy/k8s/policy"
TD="$POL/testdata"
CHECK="$POL/check_policy.py"
GEN="$TD/gen_fixtures.py"
FIXTURES_ONLY=0
[ "${1:-}" = "--fixtures-only" ] && FIXTURES_ONLY=1

KUSTOMIZE_VERSION=v5.7.1
KUBECONFORM_VERSION=v0.8.0
YQ_VERSION=v4.44.3
K8S_SCHEMA_VERSION=1.32.0
# shellcheck disable=SC1091
[ -f "$ROOT/deploy/k8s/tools.env" ] && . "$ROOT/deploy/k8s/tools.env"

export GOTOOLCHAIN=local
[ -d "/c/Program Files/Go/bin" ] && PATH="$PATH:/c/Program Files/Go/bin"
PY="$(command -v python3 || command -v python || true)"
winpath() { if command -v cygpath >/dev/null 2>&1; then cygpath -m "$1"; else printf '%s' "$1"; fi; }

PASS=0; FAILN=0; SKIPN=0; ST_P=0; ST_T=0
pass() { PASS=$((PASS + 1)); printf 'PASS [%s] %s\n' "$1" "$2"; }
fail() { FAILN=$((FAILN + 1)); printf 'FAIL [%s] %s\n' "$1" "$2"; }
skip() { SKIPN=$((SKIPN + 1)); printf 'SKIP [%s] %s\n' "$1" "$2"; }
# fixture-level results, counted for the "selftest: p/t" line
st_pass() { ST_P=$((ST_P + 1)); ST_T=$((ST_T + 1)); pass "$@"; }
st_fail() { ST_T=$((ST_T + 1)); fail "$@"; }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/bin"

if [ -z "$PY" ]; then fail ENV "python3 not found"; echo "RESULT pass=$PASS fail=$FAILN skip=$SKIPN"; exit 1; fi

# tool NAME MODULE@VERSION: echo path of a binary built into $TMP/bin, else one on PATH, else nothing
tool() {
  local name="$1" mod="$2" exe
  if command -v go >/dev/null 2>&1; then
    GOBIN="$(winpath "$TMP/bin")" go install "$mod" >"$TMP/go-$name.log" 2>&1
    for exe in "$TMP/bin/$name" "$TMP/bin/$name.exe"; do [ -x "$exe" ] && { echo "$exe"; return; }; done
  fi
  command -v "$name" 2>/dev/null || true
}

YQ="$(tool yq "github.com/mikefarah/yq/v4@$YQ_VERSION")"

# ---------------------------------------------------------------- A fixtures in sync
if "$PY" -I "$(winpath "$GEN")" --check >"$TMP/gen.out" 2>&1; then
  pass A-GEN-SYNC "committed fixtures match testdata/gen_fixtures.py"
else
  fail A-GEN-SYNC "fixtures out of sync with generator: $(head -c 300 "$TMP/gen.out" | tr '\n' ' ')"
fi
nbad="$(ls "$TD"/bad/*.yaml 2>/dev/null | wc -l)"
[ "$nbad" -ge 100 ] && pass A-COUNT "$nbad bad fixtures" || fail A-COUNT "only $nbad bad fixtures (expected >= 100)"
for id in AC-1 AC-3 AC-4 AC-5 AC-6 AC-7 AC-8 AC-9 AC-10 AC-11 AC-12 AC-14 AC-15 AC-16 AC-17 AC-18 AC-19 AC-20 AC-21 AC-22 AC-23 AC-32 EXEMPT; do
  if ls "$TD"/bad/"$id"--*.yaml >/dev/null 2>&1; then pass "A-COVER-$id" "has bad fixture(s)"; else fail "A-COVER-$id" "no bad fixture"; fi
done
if ls -d "$TD"/dockerfiles/bad/AC-13--*/ >/dev/null 2>&1; then pass A-COVER-AC-13 "has bad dockerfile fixture(s)"; else fail A-COVER-AC-13 "no bad dockerfile fixture"; fi

# ---------------------------------------------------------------- B/C fixtures through the checker
if [ -f "$CHECK" ]; then pass B-CHECKER-EXISTS "check_policy.py exists"
else fail B-CHECKER-EXISTS "deploy/k8s/policy/check_policy.py does not exist"; fi

# run_fixture FILE: sets RC and writes $TMP/out.txt / $TMP/err.txt; returns 99 with ERRMSG when it cannot run
ERRMSG=""
run_fixture() {
  local f="$1" args
  args="$(head -n 1 "$f" | sed -n 's/^# policy-args:[[:space:]]*//p')"
  if [ -z "$args" ]; then ERRMSG="no '# policy-args:' first line"; return 99; fi
  if [ -z "$YQ" ]; then ERRMSG="no yq (needs Go for 'go install github.com/mikefarah/yq/v4@$YQ_VERSION', or yq on PATH)"; return 99; fi
  if ! "$YQ" ea -o=json '[.]' "$(winpath "$f")" >"$TMP/in.json" 2>"$TMP/yq.err"; then ERRMSG="yq failed: $(head -c 200 "$TMP/yq.err")"; return 99; fi
  # shellcheck disable=SC2086
  "$PY" -I "$(winpath "$CHECK")" $args --input "$(winpath "$TMP/in.json")" >"$TMP/out.txt" 2>"$TMP/err.txt"
  RC=$?
  return 0
}
FAIL_RE='^FAIL (AC-[0-9]+|EXEMPT) ([A-Za-z]+)/([a-z0-9.-]+): (.+)$'

for f in "$TD"/good-local.yaml "$TD"/good-staging.yaml; do
  id="B-GOOD-$(basename "$f" .yaml)"
  if [ ! -f "$CHECK" ]; then st_fail "$id" "cannot run: check_policy.py missing"; continue; fi
  run_fixture "$f"; if [ $? -eq 99 ]; then st_fail "$id" "$ERRMSG"; continue; fi
  if [ "$RC" -eq 0 ] && ! grep -q '^FAIL ' "$TMP/out.txt" && tail -n 1 "$TMP/out.txt" | grep -qE '^policy: OK \([0-9]+ objects, overlay=(local|staging)\)$'; then
    st_pass "$id" "accepted (exit 0, 'policy: OK' last line)"
  else
    st_fail "$id" "rc=$RC: $(head -c 300 "$TMP/out.txt" | tr '\n' '|') $(head -c 200 "$TMP/err.txt" | tr '\n' '|')"
  fi
done

for f in "$TD"/bad/*.yaml; do
  b="$(basename "$f" .yaml)"; rule="${b%%--*}"; id="C-$b"
  if [ ! -f "$CHECK" ]; then st_fail "$id" "cannot run: check_policy.py missing"; continue; fi
  run_fixture "$f"; if [ $? -eq 99 ]; then st_fail "$id" "$ERRMSG"; continue; fi
  got="$(grep -E '^FAIL ' "$TMP/out.txt" | awk '{print $2}' | sort -u | tr '\n' ' ')"
  malformed="$(grep -E '^FAIL ' "$TMP/out.txt" | grep -vE "$FAIL_RE" | head -n 1)"
  last="$(tail -n 1 "$TMP/out.txt")"
  if [ "$RC" -ne 1 ]; then
    st_fail "$id" "expected exit 1, got $RC: $(head -c 200 "$TMP/out.txt" | tr '\n' '|') $(head -c 200 "$TMP/err.txt" | tr '\n' '|')"
  elif ! printf ' %s' "$got" | grep -q " $rule "; then
    st_fail "$id" "no FAIL $rule line; got: ${got:-none}"
  elif [ "$got" != "$rule " ]; then
    st_fail "$id" "must trip only $rule, got: $got"
  elif [ -n "$malformed" ]; then
    st_fail "$id" "FAIL line does not match the contract regex: $malformed"
  elif ! printf '%s\n' "$last" | grep -qE '^policy: [0-9]+ violation\(s\) \(overlay=(local|staging)\)$'; then
    st_fail "$id" "last line is not the 'policy: <k> violation(s)' summary: $last"
  else
    st_pass "$id" "rejected with $rule only"
  fi
done

# AC-13 through the checker's --dockerfiles mode (contract sections 4 and 7)
if [ ! -f "$CHECK" ]; then
  st_fail C-DOCKERFILES-good "cannot run: check_policy.py missing"
elif "$PY" -I "$(winpath "$CHECK")" --dockerfiles "$(winpath "$TD/dockerfiles/good")" >"$TMP/out.txt" 2>"$TMP/err.txt" \
     && ! grep -q '^FAIL ' "$TMP/out.txt" && tail -n 1 "$TMP/out.txt" | grep -qE '^policy: OK \([0-9]+ objects, overlay=dockerfiles\)$'; then
  st_pass C-DOCKERFILES-good "dockerfiles/good accepted"
else
  st_fail C-DOCKERFILES-good "dockerfiles/good rejected: $(head -c 300 "$TMP/out.txt" | tr '\n' '|') $(head -c 200 "$TMP/err.txt" | tr '\n' '|')"
fi
for d in "$TD"/dockerfiles/bad/AC-13--*/; do
  b="$(basename "$d")"; id="C-DOCKERFILES-$b"
  if [ ! -f "$CHECK" ]; then st_fail "$id" "cannot run: check_policy.py missing"; continue; fi
  "$PY" -I "$(winpath "$CHECK")" --dockerfiles "$(winpath "$d")" >"$TMP/out.txt" 2>"$TMP/err.txt"; rc=$?
  got="$(grep -E '^FAIL ' "$TMP/out.txt" | awk '{print $2}' | sort -u | tr '\n' ' ')"
  malformed="$(grep -E '^FAIL ' "$TMP/out.txt" | grep -vE "$FAIL_RE" | head -n 1)"
  if [ "$rc" -ne 1 ]; then st_fail "$id" "expected exit 1, got $rc: $(head -c 200 "$TMP/out.txt" | tr '\n' '|')"
  elif [ "$got" != "AC-13 " ]; then st_fail "$id" "must trip only AC-13, got: ${got:-none}"
  elif [ -n "$malformed" ]; then st_fail "$id" "FAIL line does not match the contract regex: $malformed"
  elif ! tail -n 1 "$TMP/out.txt" | grep -qE '^policy: [0-9]+ violation\(s\) \(overlay=dockerfiles\)$'; then st_fail "$id" "bad summary line: $(tail -n 1 "$TMP/out.txt")"
  else st_pass "$id" "rejected with AC-13 only"; fi
done

# exit code 2: usage / input errors (contract section 4)
usage_case() { # id description args...
  local id="$1" d="$2" rc; shift 2
  if [ ! -f "$CHECK" ]; then st_fail "$id" "cannot run: check_policy.py missing"; return; fi
  "$PY" -I "$(winpath "$CHECK")" "$@" >"$TMP/out.txt" 2>"$TMP/err.txt"; rc=$?
  if [ "$rc" -eq 2 ] && grep -q '^ERROR ' "$TMP/err.txt"; then st_pass "$id" "$d -> exit 2 with ERROR on stderr"
  else st_fail "$id" "$d: expected exit 2 + 'ERROR ...' on stderr, got rc=$rc err=$(head -c 120 "$TMP/err.txt")"; fi
}
printf '{"kind":"Namespace"}\n' >"$TMP/obj.json"
printf '[]\n' >"$TMP/empty.json"
EMPTY="$(winpath "$TMP/empty.json")"
usage_case C-EXIT2-NO-INPUT "missing --input" --overlay local
usage_case C-EXIT2-NO-OVERLAY "missing --overlay" --input "$EMPTY"
usage_case C-EXIT2-BAD-OVERLAY "unknown overlay" --overlay prod --input "$EMPTY"
usage_case C-EXIT2-UNREADABLE "unreadable file" --overlay local --input "$(winpath "$TMP/does-not-exist.json")"
usage_case C-EXIT2-NOT-ARRAY "JSON object instead of array" --overlay local --input "$(winpath "$TMP/obj.json")"
usage_case C-EXIT2-DOCKERFILES-DIR "missing Dockerfile dir" --dockerfiles "$(winpath "$TMP/no-such-dir")"
usage_case C-EXIT2-BAD-PROCESS "unknown process name" --overlay local --input "$EMPTY" --processes web,nosuch

# ---------------------------------------------------------------- D Dockerfile USER (AC-13)
dockuser() { # file expected
  local f="$ROOT/deploy/docker/$1" want="$2" last
  last="$(grep -E '^USER[[:space:]]' "$f" | tail -n 1 | sed -E 's/^USER[[:space:]]+//; s/[[:space:]]+$//')"
  if [ "$last" = "$want" ]; then pass "D-USER-$1" "last USER is $want"
  else fail "D-USER-$1" "FAIL AC-13 Dockerfile/$1: last USER is '${last:-none}', expected '$want' (numeric)"; fi
}
dockuser go-service.Dockerfile "65532:65532"
dockuser ticketing.Dockerfile "65532:65532"
dockuser waitingroom.Dockerfile "65532:65532"
dockuser ai-worker.Dockerfile "10001"

finish() {
  echo "selftest: $ST_P/$ST_T"
  echo "RESULT pass=$PASS fail=$FAILN skip=$SKIPN"
  [ "$FAILN" -eq 0 ]
  exit
}
[ "$FIXTURES_ONLY" -eq 1 ] && finish

# ---------------------------------------------------------------- E real tree
K8S="$ROOT/deploy/k8s"
PROCS="web gateway identity event waitingroom waitingroom-controller ticketing ticketing-worker payment notification ai-worker"

left="$(ls "$K8S"/*.yaml 2>/dev/null | xargs -n1 basename 2>/dev/null | tr '\n' ' ')"
if [ -z "$left" ]; then pass E-AC3-FLAT "no deploy/k8s/*.yaml left at top level"; else fail E-AC3-FLAT "top-level manifests still present: $left"; fi

KUSTOMIZE="$(tool kustomize "sigs.k8s.io/kustomize/kustomize/v5@$KUSTOMIZE_VERSION")"
KUBECONFORM="$(tool kubeconform "github.com/yannh/kubeconform/cmd/kubeconform@$KUBECONFORM_VERSION")"
KUBECTL="$(command -v kubectl || true)"
render() { # overlay -> $TMP/<overlay>.yaml
  if [ -n "$KUSTOMIZE" ]; then (cd "$ROOT" && "$KUSTOMIZE" build "deploy/k8s/overlays/$1") >"$TMP/$1.yaml" 2>"$TMP/$1.err"
  else (cd "$ROOT" && "$KUBECTL" kustomize "deploy/k8s/overlays/$1") >"$TMP/$1.yaml" 2>"$TMP/$1.err"; fi
}
RENDERED=""
for ov in local staging; do
  if [ -z "$KUSTOMIZE$KUBECTL" ]; then skip "E-AC1-RENDER-$ov" "no kustomize (go install failed) and no kubectl"; continue; fi
  if [ ! -d "$K8S/overlays/$ov" ]; then fail "E-AC1-RENDER-$ov" "deploy/k8s/overlays/$ov does not exist"; continue; fi
  if render "$ov"; then
    pass "E-AC1-RENDER-$ov" "kustomize build overlays/$ov exits 0"
    RENDERED="$RENDERED $ov"
    if [ -z "$YQ" ]; then skip "E-AC1-DEPLOYMENTS-$ov" "yq unavailable"
    else
      n="$("$YQ" ea '[select(.kind == "Deployment" and .metadata.labels["eventflow.io/tier"] != "dev-infra")] | length' "$(winpath "$TMP/$ov.yaml")" 2>/dev/null | tail -n 1)"
      if [ "$n" = "11" ]; then pass "E-AC1-DEPLOYMENTS-$ov" "11 app Deployments (dev-infra excluded)"; else fail "E-AC1-DEPLOYMENTS-$ov" "${n:-?} app Deployments (expected 11, dev-infra label excluded)"; fi
    fi
    for p in $PROCS; do
      grep -q "^  name: eventflow-$p\$" "$TMP/$ov.yaml" || fail "E-AC1-PROC-$ov" "no object named eventflow-$p"
    done
    ns="$(grep -E '^  namespace: ' "$TMP/$ov.yaml" | sort -u | tr '\n' ' ')"
    if [ "$ns" = "  namespace: eventflow " ]; then pass "E-AC1-NAMESPACE-$ov" "all namespaced objects in eventflow"; else fail "E-AC1-NAMESPACE-$ov" "namespaces found: $ns"; fi
    if awk '/^kind: Deployment$/{d=1} d&&/^  name: eventflow-web$/{w=1} w&&/^  selector:/{s=1;next} s&&/^    matchLabels:/{m=1;next} m{ if ($0 ~ /^      [a-z]/) c++; else {exit} } END{exit !(c==2)}' "$TMP/$ov.yaml"; then
      pass "E-AC3-SELECTOR-$ov" "web selector has exactly 2 labels"
    else fail "E-AC3-SELECTOR-$ov" "web selector is not the original two labels"; fi
    if [ "$ov" = "staging" ]; then
      if grep -q '^kind: Secret$' "$TMP/$ov.yaml"; then fail E-AC16-STAGING-SECRET "staging render contains a Secret"; else pass E-AC16-STAGING-SECRET "staging render has no Secret"; fi
    fi
  else
    fail "E-AC1-RENDER-$ov" "render failed: $(head -c 300 "$TMP/$ov.err" | tr '\n' ' ')"
  fi
done

SEC="$K8S/overlays/local/secrets.dev.env"
if [ ! -f "$SEC" ]; then
  fail E-AC16-DEVENV "overlays/local/secrets.dev.env does not exist"
else
  if head -n 3 "$SEC" | grep -q 'DEV ONLY'; then pass E-AC16-HEADER "DEV ONLY header present"; else fail E-AC16-HEADER "no DEV ONLY header in first lines"; fi
  bad="$(grep -vE '^[[:space:]]*(#|$)' "$SEC" | grep -v 'dev' | cut -d= -f1 | tr '\n' ' ')"
  if [ -z "$bad" ]; then pass E-AC16-DEVVALUES "every value contains 'dev'"; else fail E-AC16-DEVVALUES "keys whose value lacks 'dev': $bad"; fi
fi

for ov in local staging; do
  case " $RENDERED " in *" $ov "*) ;; *) fail "E-AC2-KUBECONFORM-$ov" "no render to validate"; fail "E-AC24-POLICY-$ov" "no render to check"; continue ;; esac
  if [ -z "$KUBECONFORM" ]; then skip "E-AC2-KUBECONFORM-$ov" "kubeconform unavailable (needs Go + network): CI-only"
  elif "$KUBECONFORM" -strict -summary -kubernetes-version "$K8S_SCHEMA_VERSION" - <"$TMP/$ov.yaml" >"$TMP/kc.out" 2>&1; then
    pass "E-AC2-KUBECONFORM-$ov" "$(tail -n 1 "$TMP/kc.out")"
  else fail "E-AC2-KUBECONFORM-$ov" "kubeconform: $(tail -n 3 "$TMP/kc.out" | tr '\n' '|')"; fi

  if [ -z "$YQ" ]; then skip "E-AC24-POLICY-$ov" "yq unavailable"; continue; fi
  if [ ! -f "$CHECK" ]; then fail "E-AC24-POLICY-$ov" "check_policy.py missing"; continue; fi
  if ! "$YQ" ea -o=json '[.]' "$(winpath "$TMP/$ov.yaml")" >"$TMP/$ov.json" 2>"$TMP/yq.err"; then fail "E-AC24-POLICY-$ov" "yq failed"; continue; fi
  if "$PY" -I "$(winpath "$CHECK")" --overlay "$ov" --input "$(winpath "$TMP/$ov.json")" >"$TMP/out.txt" 2>"$TMP/err.txt"; then
    pass "E-AC24-POLICY-$ov" "$(tail -n 1 "$TMP/out.txt")"
  else fail "E-AC24-POLICY-$ov" "$(grep -c '^FAIL ' "$TMP/out.txt") violation(s): $(grep '^FAIL ' "$TMP/out.txt" | head -n 3 | tr '\n' '|')"; fi
done

if command -v docker >/dev/null 2>&1; then
  if (cd "$ROOT" && docker compose -f deploy/compose/docker-compose.yml config -q) >/dev/null 2>&1; then
    pass E-AC27-COMPOSE "docker compose config -q"
  else fail E-AC27-COMPOSE "docker compose config -q failed"; fi
else skip E-AC27-COMPOSE "docker CLI missing"; fi

finish
