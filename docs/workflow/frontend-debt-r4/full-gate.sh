#!/usr/bin/env bash
# Full gate (buoc 4, quy trinh v3) cho frontend-debt-r4. Script, khong LLM.
# Chay tu goc repo: bash docs/workflow/frontend-debt-r4/full-gate.sh
# Ghi raw output vao docs/workflow/frontend-debt-r4/gate/ va gate-report.json.
set -u
ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"
OUT=docs/workflow/frontend-debt-r4/gate
BASE=43375eb            # develop luc bat dau task
TESTS_FROZEN=4cc8ed5    # commit dong bang red tests (buoc 2a)
mkdir -p "$OUT"
export PATH="/c/Program Files/Go/bin:$PATH" GOTOOLCHAIN=local
SHA=$(git rev-parse HEAD)
declare -A R

run() { # run <ten> <lenh...>
  local name=$1; shift
  ( "$@" ) >"$OUT/$name.log" 2>&1
  local rc=$?
  R[$name]=$rc
  echo "$name exit=$rc"
}

# --- web ---
run web-npm-ci        bash -c "cd apps/web && npm ci --no-audit --no-fund"
run web-lint          bash -c "cd apps/web && npm run lint"
run web-typecheck     bash -c "cd apps/web && npm run typecheck"
run web-build         bash -c "cd apps/web && env -u NEXT_PUBLIC_API_BASE npm run build"
run web-test-coverage bash -c "cd apps/web && npx vitest run --coverage"
# audit: so voi develop (BASE). Gate fail neu co lo hong high/critical MOI.
run web-audit         bash -c "cd apps/web && npm audit --omit=dev --json > ../../$OUT/audit-head.json; git show $BASE:apps/web/package-lock.json > /tmp/base-lock.json; node -e '
const h=require(\"../../$OUT/audit-head.json\").vulnerabilities||{};
const bad=Object.entries(h).filter(([,v])=>[\"high\",\"critical\"].includes(v.severity)).map(([k])=>k);
const known=[\"next\",\"postcss\",\"source-map-js\"]; // co san tren develop $BASE (npm audit cung ket qua)
const fresh=bad.filter(k=>!known.includes(k));
console.log(\"high/critical:\",bad.join(\",\")||\"none\",\"| new vs develop:\",fresh.join(\",\")||\"none\");
process.exit(fresh.length?1:0);'"

# --- go ---
run go-gateway bash -c "cd services/gateway && go vet ./... && go test ./... -count=1"
run go-event   bash -c "cd services/event && go vet ./... && go vet -tags integration ./... && go test ./... -count=1"

# --- tamper: contract test khong doi tu luc dong bang; khong skip/only moi; khong xoa file test ---
run tamper bash -c "
set -e
# Task nay: MOI file test da co o commit dong bang deu bi khoa (khong chi contract test)
# Chi xet commit cua nhanh nay (first-parent, bo merge tu develop): thay doi test do teammate merge vao develop khong tinh
d=\$(git log --first-parent --no-merges --format= --name-only --diff-filter=MD $TESTS_FROZEN..HEAD -- '*.test.ts' '*.test.tsx' '*_test.go' 'apps/web/src/lib/debt/harness.ts' 'apps/web/src/test-utils' | sort -u | sed '/^\$/d')
# Ngoai le DA DUYET (test-change-request.md, TCR-1): dung 1 dong them vao ac3-locks-errors.test.ts
ok='apps/web/src/lib/debt/ac3-locks-errors.test.ts'
if [ \"\$d\" = \"\$ok\" ]; then
  ch=\$(git diff $TESTS_FROZEN HEAD -- \$ok | grep -E '^[+-] ' | tr -d '\\r' | sed 's/^+ *//' | tr '\\n' '|')
  [ \"\$ch\" = 'await vi.advanceTimersByTimeAsync(0);|' ] || { echo \"unapproved change in \$ok: \$ch\"; exit 1; }
  echo 'approved TCR-1 only'
  d=''
fi
[ -z \"\$d\" ] || { echo \"contract tests changed: \$d\"; exit 1; }
del=\$(git log --first-parent --no-merges --format= --name-only --diff-filter=D $BASE..HEAD | grep -E '\\.test\\.(ts|tsx)\$|_test\\.go\$|\\.spec\\.ts\$' || true)
[ -z \"\$del\" ] || { echo \"deleted test files (must be listed in tcr-applied.md):\"; echo \"\$del\"; for f in \$del; do grep -q \"\$(basename \$f)\" docs/workflow/frontend-debt-r4/tcr-applied.md || { echo \"NOT LOGGED: \$f\"; exit 1; }; done; }
add=\$(git log --first-parent --no-merges -p --format= $BASE..HEAD -- 'apps/web' 'tests/e2e' 'services' | grep -E '^\\+.*\\b(it|test|describe)\\.(skip|only|todo)\\(|^\\+.*t\\.Skip\\(' || true)
[ -z \"\$add\" ] || { echo \"new skip/only: \$add\"; exit 1; }
echo ok"

# --- secret scan (regex tren diff, khong co gitleaks local) ---
run secrets bash -c "
hits=\$(git diff $BASE HEAD -- . ':(exclude)*.lock' ':(exclude)*package-lock.json' | grep -E '^\\+' | grep -nEi '(AKIA[0-9A-Z]{16}|-----BEGIN [A-Z ]*PRIVATE KEY|ghp_[A-Za-z0-9]{36}|xox[baprs]-|AIza[0-9A-Za-z_-]{35}|(secret|password|api[_-]?key)[\"'\\'' ]*[:=][\"'\\'' ]*[A-Za-z0-9/+]{16,})' || true)
[ -z \"\$hits\" ] && echo 'no secrets' || { echo \"\$hits\"; exit 1; }"

# --- SAST nhe: pattern nguy hiem moi trong diff web ---
run sast bash -c "
git diff $BASE HEAD -- apps/web/src ':(exclude)*.test.*' | grep -nE '^\\+.*(eval\\(|new Function\\(|dangerouslySetInnerHTML|innerHTML *=|document\\.write)' || true
echo '--- (review moi dong o tren; dangerouslySetInnerHTML chi chap nhan cho SVG do thu vien qrcode sinh)'"

# --- e2e: build voi API base, chay standalone server + playwright (stub qua webServer) ---
run e2e bash -c "
set -e
cd apps/web && NEXT_PUBLIC_API_BASE=http://localhost:8080 npm run build >/dev/null
s=\$(find .next/standalone -name server.js -not -path '*/node_modules/*' | awk -F/ '{print NF\"\\t\"\$0}' | sort -n | head -1 | cut -f2-)
d=\$(dirname \$s); cp -r .next/static \$d/.next/static; cp -r public \$d/public
(cd \$d && HOSTNAME=127.0.0.1 PORT=3000 node server.js > /tmp/web-e2e.log 2>&1 & echo \$! > /tmp/web-e2e.pid)
for i in \$(seq 1 30); do curl -sf http://127.0.0.1:3000/api/healthz >/dev/null && break; sleep 2; done
cd ../../tests/e2e && npm ci --no-audit --no-fund >/dev/null && CI=1 npx playwright test; rc=\$?
kill \$(cat /tmp/web-e2e.pid) 2>/dev/null
# Windows: kill cua MSYS khong luon toi duoc node.exe; giai phong cong 3000 de lan build sau khong bi khoa .next
for p in \$(netstat -ano 2>/dev/null | grep -E ':3000 .*LISTEN' | awk '{print \$5}' | sort -u); do taskkill //F //PID \$p >/dev/null 2>&1; done
exit \$rc"

# --- report ---
{
  echo "{"
  echo "  \"commit_sha\": \"$SHA\","
  echo "  \"base\": \"$BASE\","
  echo "  \"generated_at\": \"$(date -u +%FT%TZ)\","
  echo "  \"results\": {"
  first=1
  for k in web-npm-ci web-lint web-typecheck web-build web-test-coverage web-audit go-gateway go-event tamper secrets sast e2e; do
    [ $first -eq 1 ] || echo ","
    first=0
    printf '    "%s": {"exit": %s, "log": "gate/%s.log"}' "$k" "${R[$k]}" "$k"
  done
  echo ""
  echo "  }"
  echo "}"
} > docs/workflow/frontend-debt-r4/gate-report.json
fail=0; for k in "${!R[@]}"; do [ "${R[$k]}" = 0 ] || fail=1; done
echo "GATE $([ $fail = 0 ] && echo PASS || echo FAIL) @ $SHA"
exit $fail
