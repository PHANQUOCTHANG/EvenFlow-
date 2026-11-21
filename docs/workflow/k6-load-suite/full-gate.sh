#!/usr/bin/env bash
# Full gate (step 4, workflow v3) for k6-load-suite. Script, no LLM.
# Run from anywhere in the repo: bash docs/workflow/k6-load-suite/full-gate.sh
# Raw output -> docs/workflow/k6-load-suite/gate/*.log, summary -> gate-report.json.
# Exit code per step: 0 = pass, 77 = NOT RUN (cannot run here, reason in the log), other = fail.
set -u
ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"
DIR=docs/workflow/k6-load-suite
OUT=$DIR/gate
BASE=918667c            # develop when the task started (tier.json base_commit)
TESTS_FROZEN=9f69937    # red-check commit (step 2a): tests + fixture frozen there
K6_IMAGE=grafana/k6:0.57.0
SHELLCHECK_IMAGE=koalaman/shellcheck:v0.10.0
mkdir -p "$OUT"
rm -f "$OUT"/*.log
SHA=$(git rev-parse HEAD)
declare -A R
ORDER=()

run() { # run <name> <command...>
  local name=$1; shift
  ( "$@" ) >"$OUT/$name.log" 2>&1
  local rc=$?
  R[$name]=$rc
  ORDER+=("$name")
  echo "$name exit=$rc"
}
notrun() { # notrun <name> <reason>
  printf 'NOT RUN: %s\n' "$2" >"$OUT/$1.log"
  R[$1]=77
  ORDER+=("$1")
  echo "$1 exit=77 (NOT RUN)"
}
docker_up() { command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; }

# --- offline unit tests (AC-12) ---
run node-unit bash -c "node --version && node --test tests/load/lib/*.test.mjs"

# --- shell scripts ---
run bash-syntax bash -c "bash -n tests/load/seed-load.sh && bash -n $DIR/full-gate.sh && echo 'bash -n ok'"
if command -v shellcheck >/dev/null 2>&1; then
  run shellcheck shellcheck tests/load/seed-load.sh
elif docker_up; then
  run shellcheck docker run --rm -v "$ROOT:/mnt" -w /mnt $SHELLCHECK_IMAGE tests/load/seed-load.sh
else
  notrun shellcheck "no shellcheck binary and Docker is not running"
fi

# --- k6: parse options of every profile without sending traffic ---
k6_inspect() { # k6_inspect <k6 command...>
  local rc=0 p
  for p in smoke spike wait checkout full; do
    echo "== k6 inspect PROFILE=$p"
    "$@" inspect -e PROFILE=$p -e JOIN_TARGET=1000 tests/load/main.js >/dev/null || { echo "FAIL $p"; rc=1; }
  done
  echo "== unknown profile must fail at init"
  if "$@" inspect -e PROFILE=soak tests/load/main.js >/dev/null 2>&1; then echo "FAIL: PROFILE=soak accepted"; rc=1; fi
  return $rc
}
if command -v k6 >/dev/null 2>&1; then
  run k6-inspect k6_inspect k6
elif docker_up; then
  run k6-inspect k6_inspect docker run --rm -i -v "$ROOT:/w" -w /w $K6_IMAGE
else
  notrun k6-inspect "k6 is not installed and Docker is not running (main.js is covered offline by main-wiring.test.mjs with stubbed k6 modules)"
fi

# --- Makefile targets resolve (dry run) ---
MAKE_BIN=$(command -v make || { [ -x /c/msys64/usr/bin/make.exe ] && echo /c/msys64/usr/bin/make.exe; } || true)
if [ -n "$MAKE_BIN" ]; then
  run make-dry-run bash -c "
set -e
for t in load load-seed load-verify load-smoke test-load-lib; do echo \"== make -n \$t\"; '$MAKE_BIN' -n \$t; done
echo '== make -n load PROFILE=spike JOIN_TARGET=5000'
'$MAKE_BIN' -n load PROFILE=spike JOIN_TARGET=5000 | grep -q -- '-e PROFILE=spike .*-e JOIN_TARGET=5000 .*tests/load/main.js'
'$MAKE_BIN' -n load | grep -q 'tests/load/main.js'
echo 'make targets ok'"
else
  notrun make-dry-run "no make binary"
fi

# --- compose-only checks (AC-1, AC-6 replay, AC-10 verify) ---
if docker_up && docker compose -f deploy/compose/docker-compose.yml ps --status running 2>/dev/null | grep -q gateway; then
  notrun load-smoke "compose is up but the gate does not start load on its own: run make load-smoke + make load-verify and record evidence.md"
else
  notrun load-smoke "needs Docker + compose up + migrate + k6 (AC-1, AC-6 replay, AC-10 verify); Docker is not running"
fi

# --- scope: only test tooling changed (tier.json rules_not_hit) ---
run scope bash -c "
bad=\$(git diff --name-only $BASE HEAD | grep -vE '^(tests/load/|docs/workflow/k6-load-suite/|Makefile\$|\\.gitignore\$)' || true)
git diff --name-only $BASE HEAD
[ -z \"\$bad\" ] && echo 'scope ok' || { echo \"out of scope: \$bad\"; exit 1; }"

# --- tamper: tests + fixture frozen at the red-check commit are unchanged ---
run tamper bash -c "
set -e
fail=0
frozen=\$(git diff --name-only --diff-filter=A $TESTS_FROZEN~1 $TESTS_FROZEN -- tests/load)
[ -n \"\$frozen\" ] || { echo 'no frozen files found'; exit 1; }
for f in \$frozen; do
  if git diff --quiet $TESTS_FROZEN HEAD -- \"\$f\"; then echo \"unchanged: \$f\"; else echo \"MODIFIED after red check: \$f\"; fail=1; fi
done
add=\$(git diff $TESTS_FROZEN HEAD -- tests/load | grep -E '^\\+.*\\b(test|it|describe)\\.(skip|only|todo)\\(|^\\+.*\\{ *(skip|only|todo): *true' || true)
[ -z \"\$add\" ] || { echo \"new skip/only: \$add\"; fail=1; }
[ \$fail = 0 ] && echo 'tamper ok'
exit \$fail"

# --- secret scan of this branch's diff ---
SCAN_BASE=$(git merge-base HEAD origin/develop 2>/dev/null || echo $BASE)
run secrets bash -c "set -o pipefail; git diff $SCAN_BASE HEAD -- . | python $DIR/gate_secret_scan.py"

# --- report ---
{
  echo "{"
  echo "  \"task\": \"k6-load-suite\","
  echo "  \"commit_sha\": \"$SHA\","
  echo "  \"base\": \"$BASE\","
  echo "  \"tests_frozen\": \"$TESTS_FROZEN\","
  echo "  \"generated_at\": \"$(date -u +%FT%TZ)\","
  echo "  \"exit_codes\": \"0 pass, 77 NOT RUN, other fail\","
  echo "  \"results\": {"
  first=1
  for k in "${ORDER[@]}"; do
    [ $first -eq 1 ] || echo ","
    first=0
    printf '    "%s": {"exit": %s, "log": "gate/%s.log"}' "$k" "${R[$k]}" "$k"
  done
  echo ""
  echo "  },"
  nr=""; for k in "${ORDER[@]}"; do [ "${R[$k]}" = 77 ] && nr="$nr\"$k\","; done
  echo "  \"not_run\": [${nr%,}],"
  fail=0; for k in "${ORDER[@]}"; do case "${R[$k]}" in 0|77) ;; *) fail=1;; esac; done
  echo "  \"verdict\": \"$([ $fail = 0 ] && echo PASS || echo FAIL)\""
  echo "}"
} > $DIR/gate-report.json
fail=0; for k in "${ORDER[@]}"; do case "${R[$k]}" in 0|77) ;; *) fail=1;; esac; done
echo "GATE $([ $fail = 0 ] && echo PASS || echo FAIL) @ $SHA (NOT RUN = 77 listed in gate-report.json)"
exit $fail
