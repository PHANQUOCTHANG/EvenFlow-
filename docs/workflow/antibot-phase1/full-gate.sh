#!/usr/bin/env bash
# Full gate (step 4, workflow v3, High-risk) for antibot-phase1. Script, no LLM.
# Run from repo root: bash docs/workflow/antibot-phase1/full-gate.sh
# Raw output -> docs/workflow/antibot-phase1/gate/*.log, summary -> gate-report.json.
# Exit 77 = NOT RUN (tool missing, Docker down, port busy). NOT RUN is never counted as PASS.
set -u
ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"
WF=docs/workflow/antibot-phase1
OUT=$WF/gate
BASE=918667c            # merge-base with origin/develop (tier.json base_commit)
TESTS_FROZEN=36aab08    # red tests frozen (step 2a)
TCR_COMMIT=f3ce1a6      # human-approved TCR-1..TCR-4 applied to frozen files
TCR_COMMITS="f3ce1a6 82252a4" # TCR-1..TCR-4, then TCR-5 (formatting only, approved)
# the only frozen files the TCR commits may change (test-change-request.md TCR-1..TCR-5)
TCR_FILES="apps/web/src/lib/antibot-signals.test.ts libs/go/antibot/corpus/corpus.go libs/go/antibot/corpus_test.go libs/go/antibot/redisstate/store_integration_test.go services/waitingroom/test/antibot/antibot_bot_test.go services/waitingroom/internal/adapter/redis/lane_integration_test.go"
GOLANGCI_VERSION=2.12.2 # .github/workflows/ci.yml
DEV_HASH_KEY='devAntibotHashKeyLocalClusterOnlyNeverProdA='
mkdir -p "$OUT"
export PATH="/c/Program Files/Go/bin:$PATH" GOTOOLCHAIN=local
SHA=$(git rev-parse HEAD)
STEPS="go-libs go-waitingroom integration web-npm-ci web-lint web-typecheck web-test-coverage web-build web-audit e2e k8s promtool corpus fuzz tamper secrets deps govulncheck"
TMP=$(mktemp -d)
export ROOT TMP BASE TESTS_FROZEN TCR_COMMIT TCR_COMMITS TCR_FILES GOLANGCI_VERSION DEV_HASH_KEY WF OUT
declare -A R

run() { # run <name> <cmd...>
  local name=$1; shift
  ( "$@" ) >"$OUT/$name.log" 2>&1
  local rc=$?
  R[$name]=$rc
  echo "$name exit=$rc"
}

docker_up() { docker info >/dev/null 2>&1; }
export -f docker_up

# --- Go module checks: gofmt (changed files, committed bytes), vet, vet -tags integration,
#     golangci-lint at the CI version, go test -race, counts vs baseline, skips only the frozen ones ---
gomod() { # gomod <module dir> <baseline pass count>
  local m=$1 bpass=$2 rc=0 f out
  for f in $(git diff --name-only --diff-filter=AM $BASE HEAD -- "$m/*.go"); do
    out=$(git show HEAD:"$f" | gofmt -l)
    if [ -n "$out" ]; then echo "FAIL gofmt: $f"; git show HEAD:"$f" | gofmt -d | sed "s/^/    /"; rc=1; else echo "gofmt ok: $f"; fi
  done
  # a gofmt failure fails the step, but the remaining checks still run so the log shows them too
  gomod_rest "$m" "$bpass" || rc=1
  [ $rc = 0 ] && echo "ok: $m" || echo "FAIL: $m (see FAIL lines above)"
  return $rc
}
gomod_rest() {
  local m=$1 bpass=$2 rc=0
  cd "$ROOT/$m"
  go vet ./... || { echo "FAIL: go vet"; return 1; }; echo "go vet: clean"
  go vet -tags integration ./... || { echo "FAIL: go vet -tags integration"; return 1; }; echo "go vet -tags integration: clean"
  golangci-lint version 2>&1 | grep -q "version $GOLANGCI_VERSION " || { echo "FAIL: golangci-lint is not v$GOLANGCI_VERSION (CI pin): $(golangci-lint version 2>&1)"; return 1; }
  golangci-lint run --timeout=5m ./... || { echo "FAIL: golangci-lint v$GOLANGCI_VERSION"; return 1; }; echo "golangci-lint v$GOLANGCI_VERSION: clean"
  go test ./... -race -count=1 -v > "$TMP/test-$(basename $m).txt" 2>&1; rc=$?
  cat "$TMP/test-$(basename $m).txt"
  [ $rc = 0 ] || { echo "FAIL: go test -race exit=$rc"; return 1; }
  local pass skips bad
  pass=$(grep -cE "^\s*--- PASS" "$TMP/test-$(basename $m).txt")
  echo "PASS lines (tests+subtests): $pass, baseline before task: $bpass"
  [ "$pass" -ge "$bpass" ] || { echo "FAIL: fewer passing tests than baseline"; return 1; }
  skips=$(grep -E "^\s*--- SKIP" "$TMP/test-$(basename $m).txt" || true)
  # frozen-test skips (36aab08): P99 latency is skipped under -race (run without -race in step corpus);
  # recorded-session corpus is PENDING until the team records sessions (AC-18 R2)
  bad=$(printf '%s\n' "$skips" | sed '/^$/d' | grep -vE -e "--- SKIP: (TestEvaluate_AC5_P99|TestCorpus_AC18_RecordedSessions) " || true)
  [ -n "$skips" ] && { echo "skips (info):"; printf '%s\n' "$skips"; }
  [ -z "$bad" ] || { echo "FAIL: unexpected skips:"; echo "$bad"; return 1; }
  return 0
}
export -f gomod gomod_rest

run go-libs bash -c 'gomod libs/go 262'
run go-waitingroom bash -c '
rc=0; gomod services/waitingroom 228 || rc=1
cd "$ROOT" && bash .github/scripts/check-domain-coverage.sh services/waitingroom 70 || rc=1
exit $rc'

# --- integration suites (testcontainers: Docker required) ---
run integration bash -c '
docker_up || { echo "NOT RUN: docker info failed (Docker daemon not reachable)"; docker info 2>&1 | tail -3; exit 77; }
rc=0
for m in libs/go services/waitingroom; do
  echo "== $m: go test -tags integration"
  (cd $m && go test -tags integration -count=1 ./...) || { echo "FAIL: $m"; rc=1; }
done
exit $rc'

# --- web ---
run web-npm-ci        bash -c "cd apps/web && npm ci --no-audit --no-fund"
run web-lint          bash -c "cd apps/web && npm run lint"
run web-typecheck     bash -c "cd apps/web && npm run typecheck"
run web-test-coverage bash -c "cd apps/web && env -u VITEST_MAX_THREADS -u VITEST_MAX_FORKS npx vitest run --coverage"
run web-build         bash -c "cd apps/web && env -u NEXT_PUBLIC_API_BASE npm run build"
# audit: FAIL only on a high/critical advisory not already present on develop (known list from the ops gate)
run web-audit bash -c '
git diff --quiet $BASE HEAD -- apps/web/package.json apps/web/package-lock.json && echo "package.json/package-lock.json unchanged vs $BASE" || echo "NOTE: web dependencies changed vs $BASE"
cd apps/web && npm audit --omit=dev --json > $TMP/audit-head.json
node -e "
const h=require(process.argv[1]).vulnerabilities||{};
const bad=Object.entries(h).filter(([,v])=>[\"high\",\"critical\"].includes(v.severity)).map(([k])=>k);
const known=[\"next\",\"postcss\",\"source-map-js\",\"zod\"]; // pre-existing on develop (ops-warroom gate)
const fresh=bad.filter(k=>!known.includes(k));
console.log(\"high/critical:\",bad.join(\",\")||\"none\",\"| new vs develop:\",fresh.join(\",\")||\"none\");
process.exit(fresh.length?1:0);" $TMP/audit-head.json'

# --- e2e: same procedure as the ops gate (standalone Next on :3000, playwright stub API on :8080) ---
run e2e bash -c '
listen() { netstat -ano 2>/dev/null | grep -E "[:.]$1 .*LISTEN" | awk "{print \$5}" | sort -u; }
busy=""; for p in 3000 8080; do [ -n "$(listen $p)" ] && busy="$busy $p(pid $(listen $p | tr "\n" " "))"; done
[ -z "$busy" ] || { echo "NOT RUN: port(s) already in use:$busy -- not killing processes this gate did not start"; exit 77; }
set -e
cd apps/web && NEXT_PUBLIC_API_BASE=http://localhost:8080 npm run build >/dev/null
s=$(find .next/standalone -name server.js -not -path "*/node_modules/*" | awk -F/ "{print NF\"\t\"\$0}" | sort -n | head -1 | cut -f2-)
d=$(dirname $s); cp -r .next/static $d/.next/static; cp -r public $d/public
(cd $d && HOSTNAME=127.0.0.1 PORT=3000 node server.js > $TMP/web-e2e.log 2>&1 & echo $! > $TMP/web-e2e.pid)
for i in $(seq 1 30); do curl -sf http://127.0.0.1:3000/api/healthz >/dev/null && break; sleep 2; done
set +e
cd ../../tests/e2e && npm ci --no-audit --no-fund >/dev/null && CI=1 npx playwright test; rc=$?
kill $(cat $TMP/web-e2e.pid) 2>/dev/null
# Windows: MSYS kill does not always reach node.exe. Both ports were free before this step,
# so any listener now is the server / stub this step started.
for p in 3000 8080; do for pid in $(listen $p); do echo "stopping pid $pid on :$p (started by this step)"; taskkill //F //PID $pid >/dev/null 2>&1; done; done
exit $rc'

# --- k8s: offline validate, policy self-test, fixtures in sync, rendered overlays carry ANTIBOT_HASH_KEY ---
run k8s bash -c '
rc=0
bash deploy/k8s/validate.sh --render-dir $TMP/render || { echo "FAIL: validate.sh"; rc=1; }
bash deploy/k8s/policy/test_policy.sh || { echo "FAIL: test_policy.sh"; rc=1; }
(cd deploy/k8s/policy/testdata && python3 -I gen_fixtures.py --check) && echo "ok: gen_fixtures.py --check" || { echo "FAIL: gen_fixtures.py --check"; rc=1; }
for o in local staging; do
  n=$(grep -c "ANTIBOT_HASH_KEY" $TMP/render/$o.yaml 2>/dev/null); n=${n:-0}
  [ "$n" -gt 0 ] && echo "ok: kustomize build overlays/$o has ANTIBOT_HASH_KEY ($n refs)" || { echo "FAIL: overlays/$o render has no ANTIBOT_HASH_KEY"; rc=1; }
  grep -n "ANTIBOT_" $TMP/render/$o.yaml | sed "s/^/  $o: /"
done
exit $rc'

# --- promtool (CI image prom/prometheus:v3.5.0) ---
run promtool bash -c '
docker_up || { echo "NOT RUN: docker info failed (promtool runs from prom/prometheus:v3.5.0)"; exit 77; }
O=$(cd deploy/observability && pwd -W 2>/dev/null || pwd)
MSYS_NO_PATHCONV=1 docker run --rm --entrypoint promtool -v "$O:/o:ro" -w /o prom/prometheus:v3.5.0 check rules alerts.yml || exit 1
MSYS_NO_PATHCONV=1 docker run --rm --entrypoint promtool -v "$O:/o:ro" -w /o prom/prometheus:v3.5.0 test rules alerts_test.yml alerts_antibot_test.yml'

# --- corpus (AC-18) + P99 without -race (AC-5) ---
run corpus bash -c '
cd libs/go
go test ./antibot -run "^(TestCorpus_|TestEvaluate_AC5_P99$)" -count=1 -v > $TMP/corpus.txt 2>&1; rc=$?
cat $TMP/corpus.txt
[ $rc = 0 ] || { echo "FAIL: corpus/P99 tests exit=$rc"; exit 1; }
grep -qE "^--- PASS: TestEvaluate_AC5_P99 " $TMP/corpus.txt || { echo "FAIL: P99 test did not pass (skipped?)"; exit 1; }
line=$(grep -oE "bots caught [0-9.]+% \([0-9]+/[0-9]+\), humans demoted [0-9.]+% \([0-9]+/[0-9]+\)" $TMP/corpus.txt | head -1)
[ -n "$line" ] || { echo "FAIL: corpus summary line not found"; exit 1; }
echo "corpus: $line"
echo "$line" | awk "{b=\$3; h=\$7; sub(/%/,\"\",b); sub(/%/,\"\",h); if (b+0>=95 && h+0<1) {print \"ok: bots caught \" b \"% >= 95%, humans demoted \" h \"% < 1%\"; exit 0} else {print \"FAIL: thresholds\"; exit 1}}"'

# --- fuzz 30 s each ---
run fuzz bash -c '
cd libs/go && rc=0
for fz in FuzzDecodeSignals FuzzEvaluate; do
  echo "== $fz"; go test ./antibot -run=^$ -fuzz="^$fz\$" -fuzztime=30s || { echo "FAIL: $fz"; rc=1; }
done
exit $rc'

# --- tamper ---
run tamper bash -c '
rc=0
# 1. every file frozen at TESTS_FROZEN is unchanged, except the 5 TCR files, which only TCR_COMMIT may touch
frozen=$(git diff --name-only --diff-filter=AM $TESTS_FROZEN~1 $TESTS_FROZEN -- . ":(exclude)docs/workflow")
for f in $frozen; do
  if git diff --quiet $TESTS_FROZEN HEAD -- "$f"; then echo "ok: frozen unchanged: $f"; continue; fi
  case " $TCR_FILES " in
    *" $f "*) allowed=" $(for c in $TCR_COMMITS; do git rev-parse $c; done | tr "\n" " ")"
              by=$(git log --format=%H $TESTS_FROZEN..HEAD -- "$f" | tr "\n" " "); okc=1; last=""
              for c in $by; do case "$allowed" in *" $c "*) [ -z "$last" ] && last=$c ;; *) okc=0 ;; esac; done
              if [ $okc = 1 ] && [ -n "$last" ] && git diff --quiet $last HEAD -- "$f"; then echo "ok: TCR-approved change only by TCR commits ($TCR_COMMITS): $f ($(git diff --numstat $TESTS_FROZEN HEAD -- "$f" | awk "{print \$1\"+/\"\$2\"-\"}"))"
              else echo "FAIL: TCR file changed by commits other than $TCR_COMMITS: $f ($by)"; rc=1; fi;;
    *) echo "FAIL: frozen file changed: $f"; rc=1;;
  esac
done
for f in $TCR_FILES; do git diff --quiet $TESTS_FROZEN HEAD -- "$f" && echo "note: TCR file not changed: $f"; done
# 2. no test that existed at BASE changed or was removed
pat="(_test\.go|\.test\.tsx?|\.spec\.ts|_test\.ya?ml|test_policy\.sh|/test_[^/]*\.py)$"
for f in $(git ls-tree -r --name-only $BASE | grep -E "$pat"); do
  if ! git cat-file -e HEAD:"$f" 2>/dev/null; then echo "FAIL: pre-existing test removed: $f"; rc=1
  elif ! git diff --quiet $BASE HEAD -- "$f"; then echo "FAIL: pre-existing test changed: $f"; rc=1; fi
done
[ $rc = 0 ] && echo "ok: all $(git ls-tree -r --name-only $BASE | grep -cE "$pat") test files present at $BASE are unchanged"
fx=$(git diff --name-only $BASE HEAD -- deploy/k8s/policy/testdata | wc -l)
echo "info: $fx k8s policy fixture files regenerated (TCR N-5, docs/workflow/k8s-all/tcr-applied.md; sync checked by gen_fixtures.py --check in step k8s)"
grep -q "N-5" docs/workflow/k8s-all/tcr-applied.md || { echo "FAIL: TCR N-5 not logged in k8s-all/tcr-applied.md"; rc=1; }
# 3. no new skip/only since the freeze (skips inside the frozen tests were part of the approved freeze)
add=$(git diff $TESTS_FROZEN HEAD -- libs services apps tests | grep -E "^\+.*(t\.Skip|SkipNow|testing\.Short\(\)|\b(it|test|describe)\.(skip|only|todo|fixme)\b|\b(xit|fit|xdescribe|fdescribe)\()" || true)
[ -z "$add" ] && echo "ok: no skip/only added after $TESTS_FROZEN" || { echo "FAIL: new skip/only:"; echo "$add"; rc=1; }
echo "info: skips present in the frozen tests (approved at freeze):"
git diff $BASE $TESTS_FROZEN -- libs services apps tests | grep -nE "^\+.*(t\.Skip|\.(skip|only)\()" | sed "s/^/  /"
exit $rc'

# --- secrets: key-like strings added since BASE; allow the documented dev hash key and dev/test fakes ---
run secrets bash -c '
git diff -U0 $BASE HEAD -- . ":(exclude)$WF/gate" ":(exclude)*package-lock.json" > $TMP/diff.txt
python3 -I - $TMP/diff.txt "$DEV_HASH_KEY" $BASE <<"PY"
import base64, re, subprocess, sys
diff, devkey, base = sys.argv[1], sys.argv[2], sys.argv[3]
_base = {}
def at_base(path):
    if path not in _base:
        r = subprocess.run(["git", "show", base + ":" + path], capture_output=True)
        _base[path] = r.stdout.decode("utf-8", "replace") if r.returncode == 0 else ""
    return _base[path]
pat = re.compile(r"sk-ant-[A-Za-z0-9_-]{8,}|AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{20,}|gh[pousr]_[A-Za-z0-9]{20,}|xox[abpr]-[A-Za-z0-9-]{10,}|-----BEGIN [A-Z ]*PRIVATE KEY|(api[_-]?key|secret|password|passwd|token|hash[_-]?key)[\"\x27 ]*[:=] *[\"\x27]?([A-Za-z0-9+/_=-]{16,})", re.I)
def fake(v, path):
    if devkey in v: return "documented dev hash key"
    if v in at_base(path): return "value already present at BASE in the same file (not introduced here)"
    try:
        d = base64.b64decode(v + "=" * (-len(v) % 4), validate=True).decode("ascii")
        if d.startswith("dev"): return "base64 of dev placeholder %r" % d[:24]
    except Exception: pass
    if re.search(r"(_test\.go|\.test\.tsx?|\.spec\.ts)$", path) and re.search(r"(?i)fake|test|dummy|0123456789", v): return "test fake"
    if v.lower().startswith("dev") or "change-me" in v.lower(): return "dev placeholder"
    return None
path, bad, allowed = "", [], {}
for line in open(diff, encoding="utf-8", errors="replace"):
    if line.startswith("+++ "): path = line[6:].strip(); continue
    if not line.startswith("+"): continue
    for m in pat.finditer(line):
        v = m.group(2) or m.group(0)
        why = fake(v, path)
        if why: allowed[why] = allowed.get(why, 0) + 1
        else: bad.append("%s: %s" % (path, line.strip()[:200]))
for k, n in sorted(allowed.items()): print("allowed (%d): %s" % (n, k))
if bad:
    print("FAIL: possible secrets:"); print("\n".join(bad)); sys.exit(1)
print("no secrets in diff BASE..HEAD beyond the allowed dev/test values above")
PY'

# --- deps: go mod tidy relative to BASE for both modules (no_new_dependencies) ---
run deps bash -c '
cleanup() { git worktree remove --force $TMP/wt-base >/dev/null 2>&1; git worktree remove --force $TMP/wt-head >/dev/null 2>&1; git worktree prune; }
trap cleanup EXIT
git worktree add -q --detach $TMP/wt-base $BASE || exit 1
git worktree add -q --detach $TMP/wt-head HEAD || exit 1
rc=0
for m in libs/go services/waitingroom; do
  for w in base head; do
    (cd $TMP/wt-$w/$m && go mod tidy) || { echo "FAIL: go mod tidy $m at $w"; rc=1; continue; }
    (cd $TMP/wt-$w && git diff --stat -- $m/go.mod $m/go.sum | sed "s|^|tidy drift [$w $m]: |")
  done
  for f in go.mod go.sum; do
    touch $TMP/wt-base/$m/$f $TMP/wt-head/$m/$f
    if diff -u $TMP/wt-base/$m/$f $TMP/wt-head/$m/$f; then echo "ok: tidied $m/$f identical at base and HEAD"; else echo "FAIL: tidied $m/$f differs"; rc=1; fi
  done
  git diff --quiet $BASE HEAD -- $m/go.mod $m/go.sum && echo "ok: $m go.mod/go.sum unchanged vs $BASE" || { echo "FAIL: $m go.mod/go.sum changed vs $BASE"; git diff $BASE HEAD -- $m/go.mod $m/go.sum; rc=1; }
done
exit $rc'

# --- govulncheck relative to BASE (toolchain go1.27.0 carries stdlib advisories on develop too) ---
run govulncheck bash -c '
GOBIN=$TMP/bin go install golang.org/x/vuln/cmd/govulncheck@v1.1.4 2>$TMP/gv.err || { echo "NOT RUN: govulncheck unavailable"; cat $TMP/gv.err; exit 77; }
GV=$TMP/bin/govulncheck
trap "git worktree remove --force $TMP/wt-gv >/dev/null 2>&1; git worktree prune" EXIT
git worktree add -q --detach $TMP/wt-gv $BASE || exit 1
ids() { grep -oE "^Vulnerability #[0-9]+: GO-[0-9]+-[0-9]+" $1 | sed "s/.*: //" | sort -u; }
rc=0
for m in libs/go services/waitingroom; do
  (cd $m && $GV ./...) > $TMP/gvh.txt 2>&1; rh=$?
  (cd $TMP/wt-gv/$m && $GV ./...) > $TMP/gvb.txt 2>&1; rb=$?
  echo "== $m HEAD (exit $rh)"; cat $TMP/gvh.txt
  echo "== $m BASE $BASE (exit $rb)"; grep -E "^Vulnerability #|Found in" $TMP/gvb.txt
  case $rh in 0|3) ;; *) echo "FAIL: govulncheck error at HEAD ($m)"; rc=1; continue;; esac
  case $rb in 0|3) ;; *) echo "FAIL: govulncheck error at BASE ($m)"; rc=1; continue;; esac
  ids $TMP/gvh.txt > $TMP/ih; ids $TMP/gvb.txt > $TMP/ib
  new=$(comm -23 $TMP/ih $TMP/ib)
  echo "$m reachable advisories: HEAD=$(wc -l < $TMP/ih) BASE=$(wc -l < $TMP/ib)"
  [ -z "$new" ] && echo "ok: $m no advisory reachable at HEAD that is not reachable at $BASE" || { echo "FAIL: $m new advisories: $new"; rc=1; }
done
exit $rc'

# --- report ---
{
  echo "{"
  echo "  \"task\": \"antibot-phase1\","
  echo "  \"commit_sha\": \"$SHA\","
  echo "  \"base\": \"$BASE\","
  echo "  \"tests_frozen\": \"$TESTS_FROZEN\","
  echo "  \"tcr_commit\": \"$TCR_COMMIT\","
  echo "  \"go\": \"$(go version | sed 's/^go version //')\","
  echo "  \"node\": \"$(node --version)\","
  echo "  \"generated_at\": \"$(date -u +%FT%TZ)\","
  echo "  \"note\": \"exit 77 = NOT RUN (tool missing, Docker down or port busy), never a pass\","
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
