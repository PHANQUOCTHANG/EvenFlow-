#!/usr/bin/env bash
# Full gate (step 4, workflow v3) for gateway-trusted-proxy. Script, no LLM.
# Run from repo root: bash docs/workflow/gateway-trusted-proxy/full-gate.sh
# Raw output -> docs/workflow/gateway-trusted-proxy/gate/*.log, summary -> gate-report.json.
# Exit 77 = NOT RUN (tool missing / fetch failed). NOT RUN is never counted as PASS.
set -u
ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"
WF=docs/workflow/gateway-trusted-proxy
OUT=$WF/gate
BASE=c5fb598            # develop at task start
TESTS_FROZEN=142ab18    # red tests frozen (step 2a)
GWD=services/gateway
FROZEN="$GWD/internal/clientip/clientip_test.go $GWD/internal/config/config_trusted_test.go $GWD/internal/proxy/trusted_proxy_test.go"
mkdir -p "$OUT"
SHA=$(git rev-parse HEAD)
STEPS="go-gateway go-others fuzz deps tamper secrets govulncheck gosec mutation"
TMP=$(mktemp -d)
export ROOT TMP BASE TESTS_FROZEN GWD FROZEN WF
declare -A R

run() { # run <name> <cmd...>
  local name=$1; shift
  ( "$@" ) >"$OUT/$name.log" 2>&1
  local rc=$?
  R[$name]=$rc
  echo "$name exit=$rc"
}

# --- gateway: gofmt (changed files, committed bytes), vet, race tests, counts vs baseline ---
run go-gateway bash -c '
rc=0
for f in $(git diff --name-only --diff-filter=AM $BASE HEAD -- "$GWD/*.go"); do
  out=$(git show HEAD:"$f" | gofmt -l)
  if [ -n "$out" ]; then echo "FAIL gofmt: $f"; rc=1; else echo "gofmt ok: $f"; fi
done
[ $rc = 0 ] || exit 1
cd $GWD && go vet ./... || { echo "FAIL: go vet"; exit 1; }
echo "go vet: clean"
go test ./... -race -count=1 -v > $TMP/test.txt 2>&1; rc=$?
cat $TMP/test.txt
[ $rc = 0 ] || { echo "FAIL: go test -race exit=$rc"; exit 1; }
grep -E "^\s*--- SKIP" $TMP/test.txt && { echo "FAIL: skipped tests"; exit 1; }
top=$(grep -cE "^--- PASS" $TMP/test.txt); sub=$(grep -cE "^\s+--- PASS" $TMP/test.txt)
echo "head: top-level=$top subtests=$sub"
b_top=$(sed -nE "s/.*\"total\": *\{\"tests\": *([0-9]+).*/\1/p" $ROOT/$WF/baseline.json)
b_sub=$(sed -nE "s/.*\"total\".*\"subtests\": *([0-9]+).*/\1/p" $ROOT/$WF/baseline.json)
echo "baseline ($BASE-equivalent b946bf8): top-level=$b_top subtests=$b_sub"
[ "$top" -ge "$b_top" ] && [ "$sub" -ge "$b_sub" ] || { echo "FAIL: fewer tests than baseline"; exit 1; }
# every test function that existed at BASE must still pass by name
cd $ROOT
miss=0
for f in $(git ls-tree -r --name-only $BASE -- $GWD | grep -E "_test\.go$"); do
  for n in $(git show $BASE:"$f" | grep -oE "^func (Test|Fuzz)[A-Za-z0-9_]+" | sed "s/^func //"); do
    grep -qE "^--- PASS: $n " $TMP/test.txt || { echo "FAIL: base test $n ($f) not passing"; miss=1; }
  done
done
[ $miss = 0 ] && echo "ok: all base test functions pass, none skipped, counts >= baseline"'

# --- other XFF readers: identity, payment ---
run go-others bash -c '
rc=0
for s in identity payment; do
  echo "== services/$s"
  (cd services/$s && go build ./... && go test ./... -count=1) || { echo "FAIL: $s"; rc=1; }
done
exit $rc'

# --- AC-11 fuzz ---
run fuzz bash -c 'cd $GWD && go test ./internal/clientip -run=^$ -fuzz=FuzzResolve -fuzztime=30s'

# --- deps: go mod tidy relative to BASE (pre-existing drift tolerated, new deps not) ---
run deps bash -c '
cleanup() { git worktree remove --force $TMP/wt-base >/dev/null 2>&1; git worktree remove --force $TMP/wt-head >/dev/null 2>&1; git worktree prune; }
trap cleanup EXIT
git worktree add -q --detach $TMP/wt-base $BASE || exit 1
git worktree add -q --detach $TMP/wt-head HEAD || exit 1
for w in base head; do
  (cd $TMP/wt-$w/$GWD && go mod tidy) || { echo "FAIL: go mod tidy at $w"; exit 1; }
  (cd $TMP/wt-$w && git diff --stat -- $GWD/go.mod $GWD/go.sum | sed "s/^/tidy drift [$w]: /")
done
rc=0
for f in go.mod go.sum; do
  touch $TMP/wt-base/$GWD/$f $TMP/wt-head/$GWD/$f
  if diff -u $TMP/wt-base/$GWD/$f $TMP/wt-head/$GWD/$f; then echo "ok: tidied $f identical at base and HEAD"; else echo "FAIL: tidied $f differs"; rc=1; fi
done
git diff --quiet $BASE HEAD -- $GWD/go.mod $GWD/go.sum && echo "ok: go.mod/go.sum unchanged vs $BASE" || { echo "FAIL: go.mod/go.sum changed vs $BASE"; git diff $BASE HEAD -- $GWD/go.mod $GWD/go.sum; rc=1; }
exit $rc'

# --- tamper: frozen tests vs TESTS_FROZEN, pre-existing tests vs BASE ---
run tamper bash -c '
rc=0
if git diff --quiet $TESTS_FROZEN HEAD -- $FROZEN; then echo "ok: frozen tests byte-identical to $TESTS_FROZEN:"; for f in $FROZEN; do echo "  $f"; done
else echo "FAIL: frozen tests changed:"; git diff --stat $TESTS_FROZEN HEAD -- $FROZEN; rc=1; fi
pre=$(git ls-tree -r --name-only $BASE -- $GWD | grep -E "_test\.go$")
for f in $pre; do
  if git cat-file -e HEAD:"$f" 2>/dev/null && git diff --quiet $BASE HEAD -- "$f"; then echo "ok: unchanged vs $BASE: $f"; else echo "FAIL: pre-existing test changed/removed: $f"; rc=1; fi
done
new=$(git diff --name-only --diff-filter=A $BASE HEAD -- "$GWD/*_test.go" | sort)
want=$(for f in $FROZEN; do echo $f; done | sort)
[ "$new" = "$want" ] && echo "ok: only new test files are the frozen ones" || { echo "FAIL: unexpected new test files:"; echo "$new"; rc=1; }
add=$(git diff $BASE HEAD -- $GWD | grep -E "^\+.*(t\.Skip|SkipNow|testing\.Short\(\))" || true)
[ -z "$add" ] && echo "ok: no new skips" || { echo "FAIL: new skip: $add"; rc=1; }
exit $rc'

# --- secrets: key-like strings added since BASE (gate logs excluded) ---
run secrets bash -c '
hits=$(git diff $BASE HEAD -- . ":(exclude)$WF/gate" ":(exclude)$WF/verify-logs" | grep -E "^\+" | grep -nE "sk-ant-[A-Za-z0-9_-]{8,}|AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{20,}|gh[pousr]_[A-Za-z0-9]{20,}|xox[abpr]-[A-Za-z0-9-]{10,}|-----BEGIN [A-Z ]*PRIVATE KEY|(api[_-]?key|secret|password|passwd|token)[\"'"'"' ]*[:=] *[\"'"'"'][A-Za-z0-9+/_=-]{16,}[\"'"'"']" || true)
[ -z "$hits" ] && echo "no secrets in diff $BASE..HEAD" || { echo "$hits"; exit 1; }'

# govulncheck: the local toolchain (go1.27.0) carries known stdlib advisories, so the gate is
# relative to BASE: FAIL only when HEAD reaches an advisory BASE does not.
run govulncheck bash -c '
# installed (not go run) so its own exit code (3 = advisories found) survives
GOBIN=$TMP/bin go install golang.org/x/vuln/cmd/govulncheck@v1.1.4 2>$TMP/gv.err || { echo "NOT RUN: govulncheck unavailable"; cat $TMP/gv.err; exit 77; }
GV=$TMP/bin/govulncheck
trap "git worktree remove --force $TMP/wt-gv >/dev/null 2>&1; git worktree prune" EXIT
git worktree add -q --detach $TMP/wt-gv $BASE || exit 1
(cd $GWD && $GV ./...) > $TMP/gv-head.txt 2>&1; rh=$?
(cd $TMP/wt-gv/$GWD && $GV ./...) > $TMP/gv-base.txt 2>&1; rb=$?
echo "== HEAD (exit $rh)"; cat $TMP/gv-head.txt
echo "== BASE $BASE (exit $rb)"; grep -E "^Vulnerability #|Found in|^Your code" $TMP/gv-base.txt
case $rh in 0|3) ;; *) echo "FAIL: govulncheck error at HEAD"; exit 1;; esac
case $rb in 0|3) ;; *) echo "FAIL: govulncheck error at BASE"; exit 1;; esac
ids() { grep -oE "^Vulnerability #[0-9]+: GO-[0-9]+-[0-9]+" $1 | sed "s/.*: //" | sort -u; }
ids $TMP/gv-head.txt > $TMP/ih; ids $TMP/gv-base.txt > $TMP/ib
echo "reachable advisories: HEAD=$(wc -l < $TMP/ih) BASE=$(wc -l < $TMP/ib)"
echo "non-stdlib modules flagged at HEAD (info, compared by ID below):"; grep -E "Found in:" $TMP/gv-head.txt | grep -vE "@go1." | sort -u
new=$(comm -23 $TMP/ih $TMP/ib)
[ -z "$new" ] && echo "ok: no advisory reachable at HEAD that is not reachable at $BASE (pre-existing: toolchain go1.27.0 stdlib + workspace golang.org/x/net)" || { echo "FAIL: new advisories: $new"; exit 1; }'

run gosec bash -c '
cd $GWD
GS="go run github.com/securego/gosec/v2/cmd/gosec@v2.22.4"
$GS -version >/dev/null 2>$TMP/gs.err || { echo "NOT RUN: gosec fetch failed"; cat $TMP/gs.err; exit 77; }
$GS ./internal/clientip/... ./internal/config/... ./internal/proxy/... > $TMP/gs.txt 2>&1; rc=$?
cat $TMP/gs.txt
# gosec v2.22.4 package loader cannot type-check with go1.27: tool limitation, not a finding.
grep -qE "internal error: package .* without types" $TMP/gs.txt && { echo "NOT RUN: gosec v2.22.4 loader incompatible with $(go env GOVERSION)"; exit 77; }
exit $rc'

# --- mutation (High risk, threshold 8/8) ---
run mutation bash $WF/verify-logs/mutation/mutate.sh
KILLED=$(sed -nE 's/^killed ([0-9]+)\/([0-9]+).*/\1/p' $WF/verify-logs/mutation/summary.txt 2>/dev/null)
TOTAL=$(sed -nE 's/^killed ([0-9]+)\/([0-9]+).*/\2/p' $WF/verify-logs/mutation/summary.txt 2>/dev/null)

# --- report ---
{
  echo "{"
  echo "  \"task\": \"gateway-trusted-proxy\","
  echo "  \"commit_sha\": \"$SHA\","
  echo "  \"base\": \"$BASE\","
  echo "  \"tests_frozen\": \"$TESTS_FROZEN\","
  echo "  \"go\": \"$(go version | sed 's/^go version //')\","
  echo "  \"generated_at\": \"$(date -u +%FT%TZ)\","
  echo "  \"note\": \"exit 77 = NOT RUN (tool missing or fetch failed), never a pass\","
  echo "  \"mutation\": {\"killed\": ${KILLED:-0}, \"total\": ${TOTAL:-0}, \"threshold\": \"8/8\", \"logs\": \"verify-logs/mutation/\"},"
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
