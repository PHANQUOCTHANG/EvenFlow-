#!/usr/bin/env bash
# Full gate (buoc 4, quy trinh v3) cho k8s-all. Script, khong LLM.
# Chay tu goc repo: bash docs/workflow/k8s-all/full-gate.sh
# Ghi raw output vao docs/workflow/k8s-all/gate/ va gate-report.json.
# Exit code cua tung buoc: 0 = pass, 77 = NOT RUN (khong chay duoc o day, co ly do), khac = fail.
set -u
ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"
DIR=docs/workflow/k8s-all
OUT=$DIR/gate
BASE=c5fb598            # develop luc bat dau task
TESTS_FROZEN=e408ea3    # commit dong bang red tests k8s (buoc 2a)
TCR_E8=8bdb540          # TCR-E8: fixtures theo contract rev 2 (scope test_policy.sh + testdata/**)
mkdir -p "$OUT"
rm -f "$OUT"/*.log
export PATH="/c/Program Files/Go/bin:$PATH" GOTOOLCHAIN=local
SHA=$(git rev-parse HEAD)
declare -A R
ORDER=()

run() { # run <ten> <lenh...>
  local name=$1; shift
  ( "$@" ) >"$OUT/$name.log" 2>&1
  local rc=$?
  R[$name]=$rc
  ORDER+=("$name")
  echo "$name exit=$rc"
}
notrun() { # notrun <ten> <ly do>
  printf 'NOT RUN: %s\n' "$2" >"$OUT/$1.log"
  R[$1]=77
  ORDER+=("$1")
  echo "$1 exit=77 (NOT RUN)"
}

# --- k8s: render + kubeconform + policy tren render that, va bo test policy ---
run k8s-validate    bash deploy/k8s/validate.sh
run k8s-test-policy bash deploy/k8s/policy/test_policy.sh
run k8s-fixtures-sync bash -c "cd deploy/k8s/policy/testdata && python gen_fixtures.py --check && echo 'fixtures in sync'"

# --- CI scripts / workflows ---
if [ -f .github/scripts/tests/test-workflows.sh ]; then
  run ci-test-workflows bash .github/scripts/tests/test-workflows.sh
else
  notrun ci-test-workflows ".github/scripts/tests/test-workflows.sh khong ton tai"
fi
run gates-syntax bash -c "bash -n .github/scripts/gates.sh && echo 'bash -n gates.sh ok'"
# TCR-3: behaviour test of the C4.4 context guard (stub kubectl); CI runs it in lint-ci (.github/scripts/tests/*.sh)
run gates-context-guard bash .github/scripts/tests/test-gates-context-guard.sh
run actionlint bash -c "
out=\$(go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7 .github/workflows/*.yml 2>&1); rc=\$?
echo \"\$out\"
if [ \$rc -ne 0 ] && echo \"\$out\" | grep -qiE 'dial tcp|no such host|proxyconnect|i/o timeout|cannot find module|toolchain not available|go: downloading.*: .*refused'; then
  echo 'NOT RUN: khong tai duoc actionlint v1.7.7'; exit 77
fi
exit \$rc"

# --- Go services: go-service/ticketing/waitingroom Dockerfile doi -> moi Go module duoc build bang cac Dockerfile do ---
GO_TOUCHED=$(git diff --name-only $BASE HEAD -- deploy/docker/go-service.Dockerfile deploy/docker/ticketing.Dockerfile deploy/docker/waitingroom.Dockerfile)
if [ -n "$GO_TOUCHED" ]; then
  run go-build bash -c "
rc=0
for m in services/*/go.mod; do d=\$(dirname \$m); echo \"== go build ./... in \$d\"; (cd \$d && go build ./...) || { echo \"FAIL \$d\"; rc=1; }; done
exit \$rc"
else
  notrun go-build "khong Dockerfile Go nao doi"
fi
if [ -n "$(git diff --name-only $BASE HEAD -- deploy/docker/ai-worker.Dockerfile)" ]; then
  run py-ai-worker bash -c "cd services/ai-worker && uv sync --all-extras \$([ -f uv.lock ] && echo --locked) && uv run ruff check . && uv run pytest -q"
else
  notrun py-ai-worker "ai-worker.Dockerfile khong doi"
fi

# --- tamper ---
run tamper bash -c "
set -e
fail=0
# (1) test co san o develop $BASE: khong duoc sua / xoa tren cac commit cua nhanh (first-parent, bo merge)
pre=\$(git log --first-parent --no-merges --format= --name-only --diff-filter=MDR $BASE..HEAD -- '*_test.go' '*.test.ts' '*.test.tsx' '*.spec.ts' 'services/ai-worker/tests' 'tests/e2e' '.github/scripts/tests' | sort -u | sed '/^\$/d')
for f in \$pre; do
  if git cat-file -e $BASE:\"\$f\" 2>/dev/null; then
    grep -q \"\$(basename \"\$f\")\" $DIR/tcr-applied.md || { echo \"existing test changed without TCR: \$f\"; fail=1; }
  fi
done
# (2) red tests k8s dong bang o $TESTS_FROZEN: moi commit sau do sua/xoa chung phai la TCR da ghi
#   - 03d1ff5: sua 1 dong --check (CRLF-tolerant) truoc TCR-E8; nam trong scope testdata/** cua TCR-E8 duyet ngay sau
#   - $TCR_E8 (TCR-E8), a0d5219 (TCR-E8c, test_policy.sh): ghi trong tcr-applied.md
#   - con lai: chi gen_fixtures.py, chi THEM dong (TCR-E8b, TCR-WP5, TCR-2 va phan bo sung round 2)
frozen=\$(git diff --name-only --diff-filter=A $TESTS_FROZEN~1 $TESTS_FROZEN)
for c in \$(git rev-list --first-parent --no-merges $TESTS_FROZEN..HEAD); do
  ch=\$(git show --format= --name-status --diff-filter=MDR \$c | awk '{print \$2}')
  for f in \$ch; do
    echo \"\$frozen\" | grep -qxF \"\$f\" || continue
    s=\$(git rev-parse --short=7 \$c)
    case \$s in
      03d1ff5|${TCR_E8:0:7}|a0d5219) echo \"approved TCR commit \$s: \$f\"; continue;;
    esac
    if [ \"\$f\" = deploy/k8s/policy/testdata/gen_fixtures.py ]; then
      del=\$(git show --numstat --format= \$c -- \"\$f\" | awk '{print \$2}')
      if [ \"\$del\" = 0 ]; then echo \"additions-only gen_fixtures.py in \$s\"; continue; fi
    fi
    echo \"frozen test/fixture modified or removed without TCR: \$f in \$s\"; fail=1
  done
done
# (3) khong xoa file nao duoi testdata tu sau TCR-E8 (TCR-E8 xoa 1 fixture, da ghi)
d=\$(git log --first-parent --no-merges --format= --name-only --diff-filter=D $TCR_E8..HEAD -- deploy/k8s/policy/testdata | sed '/^\$/d')
[ -z \"\$d\" ] || { echo \"deleted fixtures after TCR-E8: \$d\"; fail=1; }
# (4) khong them skip/only moi
add=\$(git diff $BASE HEAD -- services apps tests .github/scripts | grep -E '^\\+.*\\b(it|test|describe)\\.(skip|only)\\(|^\\+.*t\\.Skip\\(|^\\+.*pytest\\.mark\\.skip' || true)
[ -z \"\$add\" ] || { echo \"new skip/only: \$add\"; fail=1; }
[ \$fail = 0 ] && echo 'tamper ok'
exit \$fail"

# --- secret scan (regex tren diff; chi secrets.dev.env DEV ONLY duoc phep) ---
run secrets bash -c "
set -o pipefail
echo '--- secrets.dev.env: moi gia tri phai chua \"dev\" (DEV ONLY, spec AC-16)'
bad=\$(grep -vE '^\\s*(#|\$)' deploy/k8s/overlays/local/secrets.dev.env | grep -viE '=.*dev' || true)
[ -z \"\$bad\" ] && echo 'secrets.dev.env ok' || { echo \"non-dev value: \$bad\"; exit 1; }
echo '--- diff $BASE..HEAD (tru secrets.dev.env, lockfiles)'
git diff $BASE HEAD -- . ':(exclude)*.lock' ':(exclude)*package-lock.json' ':(exclude)deploy/k8s/overlays/local/secrets.dev.env' \
  | python $DIR/gate_secret_scan.py"

# --- can cluster: khong chay duoc o day ---
notrun ac30-local-apply "AC-30: apply overlays/local len Docker Desktop/kind (migrate Job, readiness, PDB, NetworkPolicy) can cluster; xem merge-preconditions.md MP-1"
notrun ac31-staging-dry-run "AC-31: staging --dry-run=server + build image/uid can cluster va registry; xem merge-preconditions.md MP-1"

# --- report ---
{
  echo "{"
  echo "  \"task\": \"k8s-all\","
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
