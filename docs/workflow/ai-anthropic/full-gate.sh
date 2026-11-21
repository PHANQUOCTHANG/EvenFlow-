#!/usr/bin/env bash
# Full gate (step 4, workflow v3) for ai-anthropic. Script, no LLM.
# Run from repo root: bash docs/workflow/ai-anthropic/full-gate.sh
# Raw output -> docs/workflow/ai-anthropic/gate/*.log, summary -> gate-report.json.
# Exit 77 = NOT RUN (tool/engine missing). NOT RUN is never counted as PASS.
set -u
ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"
WF=docs/workflow/ai-anthropic
OUT=$WF/gate
BASE=c5fb598            # develop at task start
TESTS_FROZEN=0763f2a    # red tests frozen (step 2a)
BASELINE_PASSED=$(sed -nE 's/.*"pytest_passed": *([0-9]+).*/\1/p' $WF/baseline.json)
BASELINE_DESELECTED=$(sed -nE 's/.*"pytest_deselected": *([0-9]+).*/\1/p' $WF/baseline.json)
mkdir -p "$OUT"
SHA=$(git rev-parse HEAD)
STEPS="py-ai-worker lock-diff compose alerts tamper secrets sast audit"
TMP=$(mktemp -d)
export TMP
declare -A R

run() { # run <name> <cmd...>
  local name=$1; shift
  ( "$@" ) >"$OUT/$name.log" 2>&1
  local rc=$?
  R[$name]=$rc
  echo "$name exit=$rc"
}

# --- python: lint + unit tests, pass count vs baseline, nothing skipped ---
# dev tools (pytest, ruff) are a [project.optional-dependencies] extra, hence --all-extras (same as baseline.json)
run py-ai-worker bash -c "
cd services/ai-worker && uv sync --all-extras --locked && uv run ruff check . || exit 1
uv run pytest -q > \$TMP/pytest.txt 2>&1; rc=\$?
cat \$TMP/pytest.txt
[ \$rc = 0 ] || exit \$rc
sum=\$(grep -E '[0-9]+ passed' \$TMP/pytest.txt | tail -1)
echo \"summary: \$sum\"
p=\$(echo \"\$sum\" | grep -oE '[0-9]+ passed' | grep -oE '[0-9]+'); d=\$(echo \"\$sum\" | grep -oE '[0-9]+ deselected' | grep -oE '[0-9]+')
echo \"\$sum\" | grep -qE 'skipped|failed|error|xfail|xpass' && { echo 'FAIL: skipped/failed/error/xfail in summary'; exit 1; }
[ \"\${d:-0}\" = \"$BASELINE_DESELECTED\" ] || { echo \"FAIL: deselected=\${d:-0}, expected $BASELINE_DESELECTED\"; exit 1; }
[ \"\${p:-0}\" -ge \"$BASELINE_PASSED\" ] || { echo \"FAIL: passed=\${p:-0} < baseline $BASELINE_PASSED\"; exit 1; }
echo \"ok: passed=\$p (baseline $BASELINE_PASSED), deselected=\$d (integration), none skipped\""

# --- AC-16: uv.lock only gains lines ---
run lock-diff bash -c "
git diff $BASE HEAD -- services/ai-worker/uv.lock > \$TMP/lock.diff
echo \"added lines: \$(grep -cE '^\+[^+]' \$TMP/lock.diff)\"
rem=\$(grep -E '^-' \$TMP/lock.diff | grep -vE '^--- ' || true)
grep -E '^\+name = ' \$TMP/lock.diff | sed 's/^+/added package: /'
[ -z \"\$rem\" ] && echo 'ok: no removed lines' || { echo 'FAIL: removed lines:'; echo \"\$rem\"; exit 1; }"

# --- AC-17: compose renders (docker CLI only, no engine needed) ---
run compose bash -c "
command -v docker >/dev/null || { echo 'NOT RUN: docker CLI missing'; exit 77; }
F=deploy/compose/docker-compose.yml; rc=0
for v in default GEMINI_MOCK=0 AI_MOCK=0; do
  e=''; [ \$v = default ] || e=\$v
  env -u AI_MOCK -u GEMINI_MOCK \$e docker compose -f \$F config -q; r=\$?
  echo \"config -q [\$v] exit=\$r\"; [ \$r = 0 ] || rc=1
  env -u AI_MOCK -u GEMINI_MOCK \$e docker compose -f \$F config 2>/dev/null | grep -E '^ +(AI_MOCK|AI_RPM|AI_TPM):' | sed 's/^ */  rendered /'
done
exit \$rc"

# --- AC-18: alert rules + rule unit tests via promtool in docker ---
run alerts bash -c "
command -v docker >/dev/null || { echo 'NOT RUN: docker CLI missing'; exit 77; }
docker info >/dev/null 2>&1 || { echo 'NOT RUN: Docker engine unavailable'; exit 77; }
D=\$(cd deploy/observability && (pwd -W 2>/dev/null || pwd))
MSYS_NO_PATHCONV=1 docker run --rm -v \"\$D:/w\" -w /w --entrypoint promtool prom/prometheus:v3.5.0 check rules alerts.yml || exit 1
MSYS_NO_PATHCONV=1 docker run --rm -v \"\$D:/w\" -w /w --entrypoint promtool prom/prometheus:v3.5.0 test rules alerts_test.yml"

# --- tamper ---
run tamper bash -c "
set -e
T=services/ai-worker/tests
# Lock only the files that existed at the freeze commit; new test files added later are allowed.
frozen=\$(git ls-tree -r --name-only $TESTS_FROZEN -- \$T/conftest.py \$T/llm \$T/test_llm_sdk_contract.py \$T/test_wiring.py deploy/observability/alerts_test.yml | tr '\n' ' ')
echo \"frozen files: \$(echo \$frozen | wc -w)\"
if git diff --quiet $TESTS_FROZEN HEAD -- \$frozen; then echo 'ok: frozen tests byte-identical to $TESTS_FROZEN'; else echo 'FAIL: frozen tests changed:'; git diff --stat $TESTS_FROZEN HEAD -- \$frozen; exit 1; fi
allowed=\" \$T/test_gemini_client.py \$T/test_llm_client.py \$T/test_guarded.py \$T/test_chat_handler.py \$T/test_moderation_consumer.py \$T/test_classify_real_errors.py \$T/test_config.py \$T/integration/test_assistant_integration.py \$T/integration/test_consumers_integration.py \$T/test_alias_scan.py \"
bad=0
for f in \$(git diff --name-only --no-renames --diff-filter=MD $TESTS_FROZEN HEAD -- \$T); do
  case \"\$allowed\" in *\" \$f \"*) echo \"allowed change (plan 2.2 / TCR-1 / new): \$f\";; *) echo \"FAIL: unexpected test change: \$f\"; bad=1;; esac
done
[ \$bad = 0 ]
names() { # every test function as file::name; test_gemini_client.py is mapped to its git mv target
  git ls-tree -r --name-only \$1 -- \$T | grep -E '\.py\$' | while read -r f; do
    g=\${f/test_gemini_client.py/test_llm_client.py}
    git show \$1:\"\$f\" | grep -oE '^[[:space:]]*(async[[:space:]]+)?def[[:space:]]+test_[A-Za-z0-9_]+' | sed -E 's/.*def[[:space:]]+//' | sed \"s|^|\$g::|\"
  done | sort -u
}
names $BASE > \$TMP/names-base.txt; names HEAD > \$TMP/names-head.txt
gone=\$(comm -23 \$TMP/names-base.txt \$TMP/names-head.txt)
echo \"test functions: base=\$(wc -l < \$TMP/names-base.txt) head=\$(wc -l < \$TMP/names-head.txt)\"
[ -z \"\$gone\" ] || { echo 'FAIL: test functions removed vs $BASE:'; echo \"\$gone\"; exit 1; }
git diff --name-status -M $BASE HEAD -- \$T | grep -E '^R' || true
add=\$(git diff $BASE HEAD -- services/ai-worker | grep -E '^\+.*(pytest\.mark\.skip|pytest\.skip\(|mark\.xfail|importorskip)' || true)
[ -z \"\$add\" ] || { echo \"FAIL: new skip/xfail: \$add\"; exit 1; }
echo 'tamper ok'"

# --- secrets: key-like strings added since BASE ---
# Allowlisted: two obviously fake fixture values in frozen tests (they assert the key never leaks into logs).
run secrets bash -c "
echo 'allowlisted fake fixtures (tests/findings only): sk-ant-TEST-SECRET-0001, sk-ant-WIRING-SECRET-1111, sk-ant-CHALLENGE-SECRET-777, sk-ant-x'
hits=\$(git diff $BASE HEAD -- . ':(exclude)*.lock' | grep -E '^\+' | grep -nE 'sk-ant-[A-Za-z0-9_-]+|AIza[0-9A-Za-z_-]{20,}|-----BEGIN [A-Z ]*PRIVATE KEY' | grep -vE 'sk-ant-(TEST-SECRET-0001|WIRING-SECRET-1111|CHALLENGE-SECRET-777|x)([^A-Za-z0-9_-]|\$)' || true)
[ -z \"\$hits\" ] && echo 'no secrets' || { echo \"\$hits\"; exit 1; }"

# --- SAST: bandit medium+ on app code ---
run sast bash -c "
command -v uvx >/dev/null || { echo 'NOT RUN: uvx missing'; exit 77; }
uvx bandit -q -r services/ai-worker/app -ll; rc=\$?
[ \$rc = 0 ] && echo 'bandit: no medium/high findings'; exit \$rc"

# --- dependency audit: advisories in locked deps, HEAD vs BASE ---
run audit bash -c "
command -v uvx >/dev/null || { echo 'NOT RUN: uvx missing'; exit 77; }
mkdir -p \$TMP/base
git show $BASE:services/ai-worker/pyproject.toml > \$TMP/base/pyproject.toml
git show $BASE:services/ai-worker/uv.lock > \$TMP/base/uv.lock
(cd services/ai-worker && uv export --locked --no-hashes --all-extras --no-emit-project) > \$TMP/head.txt || exit 1
(cd \$TMP/base && uv export --locked --no-hashes --all-extras --no-emit-project) > \$TMP/base.txt || exit 1
uvx pip-audit --no-deps --disable-pip -r \$TMP/head.txt -f json -o \$TMP/head.json >/dev/null 2>\$TMP/err-h
uvx pip-audit --no-deps --disable-pip -r \$TMP/base.txt -f json -o \$TMP/base.json >/dev/null 2>\$TMP/err-b
[ -s \$TMP/head.json ] && [ -s \$TMP/base.json ] || { echo 'NOT RUN: pip-audit produced no report'; cat \$TMP/err-h \$TMP/err-b; exit 77; }
uv run --no-project python -I $WF/gate-audit-diff.py \$TMP/head.json \$TMP/base.json"

# --- report ---
{
  echo "{"
  echo "  \"commit_sha\": \"$SHA\","
  echo "  \"base\": \"$BASE\","
  echo "  \"tests_frozen\": \"$TESTS_FROZEN\","
  echo "  \"generated_at\": \"$(date -u +%FT%TZ)\","
  echo "  \"note\": \"exit 77 = NOT RUN (tool or engine missing), never a pass\","
  echo "  \"results\": {"
  first=1
  for k in $STEPS; do
    [ $first -eq 1 ] || echo ","
    first=0
    case "${R[$k]}" in 0) st=PASS;; 77) st=NOT_RUN;; *) st=FAIL;; esac
    printf '    "%s": {"exit": %s, "status": "%s", "log": "gate/%s.log"}' "$k" "${R[$k]}" "$st" "$k"
  done
  echo ""
  echo "  }"
  echo "}"
} > $WF/gate-report.json
rm -rf "$TMP"
fail=0; notrun=""
for k in $STEPS; do case "${R[$k]}" in 0) ;; 77) notrun="$notrun $k";; *) fail=1;; esac; done
if [ $fail = 1 ]; then echo "GATE FAIL @ $SHA"
elif [ -n "$notrun" ]; then echo "GATE PASS (NOT RUN:$notrun) @ $SHA"
else echo "GATE PASS @ $SHA"; fi
exit $fail
