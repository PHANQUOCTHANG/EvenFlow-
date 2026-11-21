#!/usr/bin/env bash
# Mutation testing for gateway-trusted-proxy (traceability.yaml mutations M1..M8, threshold 8/8).
# Each mutant: apply a perl substitution to production code, require it to change the file and
# to build (go vet), run `go test ./... -count=1` in services/gateway, expect FAIL, then revert
# with `git checkout -- services/gateway` (autocrlf off so files stay LF). A mutant that does not apply or does not build is
# INVALID (never counted as killed).
# Round 1: M5 disables the prefix-width floor (Amendment 1), which subsumes the old /0 check.
# Run from repo root: bash docs/workflow/gateway-trusted-proxy/verify-logs/mutation/mutate.sh
# Logs -> verify-logs/mutation/M*.log, summary -> verify-logs/mutation/summary.txt. Exit 0 iff 8/8 killed.
set -u
ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"
D=docs/workflow/gateway-trusted-proxy/verify-logs/mutation
CIP=services/gateway/internal/clientip/clientip.go
GW=services/gateway/internal/proxy/gateway.go

git diff --quiet -- services/gateway || { echo "ABORT: services/gateway has uncommitted changes"; exit 2; }

# id | file | perl substitution (applied with -0pi, whole file)
MUTANTS=(
'M1|'$CIP'|s/(\tvar leftmost netip.Addr\r?\n)/$1\tif len(xff) > 0 {\n\t\tif f := trimSpaceTab(strings.Split(xff[0], ",")[0]); f != "" {\n\t\t\tif a, ok := parseAddr(f); ok {\n\t\t\t\treturn a.String(), true\n\t\t\t}\n\t\t}\n\t}\n/'
'M2|'$CIP'|s/ \|\| !r\.isTrusted\(peer\.Unmap\(\)\) \{/ {/'
'M3|'$CIP'|s/(if !ok \{\r?\n\t+)return key, false/${1}continue/'
'M4|'$CIP'|s/return a\.Unmap\(\), true/return a, true/; s/!r\.isTrusted\(peer\.Unmap\(\)\)/!r.isTrusted(peer)/'
'M5|'$CIP'|s/if p\.Bits\(\) < floor \{/if false \&\& p.Bits() < floor {/'
'M6|'$GW'|s/pr\.Out\.Header\.Set\("X-Forwarded-For", ip\)/_ = ip/'
'M7|'$CIP'|s/if hops > maxTrustedHops \{/if false \&\& hops > maxTrustedHops {/'
'M8|'$CIP'|s/if p != p\.Masked\(\) \{/if false \&\& p != p.Masked() {/'
)

killed=0; total=0; : > "$D/summary.txt"
for m in "${MUTANTS[@]}"; do
  id=${m%%|*}; rest=${m#*|}; file=${rest%%|*}; expr=${rest#*|}
  total=$((total+1)); log="$D/$id.log"
  {
    echo "== $id: $file"
    echo "perl -0pi -e '$expr'"
    perl -0pi -e "$expr" "$file"
    if git diff --quiet -- "$file"; then
      echo "RESULT $id INVALID (substitution did not apply)"
    else
      git diff -- services/gateway
      if ! (cd services/gateway && go vet ./...); then
        echo "RESULT $id INVALID (mutant does not build/vet)"
      elif (cd services/gateway && go test ./... -count=1); then
        echo "RESULT $id SURVIVED (tests passed)"
      else
        echo "--- failing tests:"; (cd services/gateway && go test ./... -count=1 2>&1 | grep -E '^\s*--- FAIL' | sort -u)
        echo "RESULT $id KILLED"
      fi
    fi
  } >"$log" 2>&1
  git -c core.autocrlf=false checkout -- services/gateway  # keep LF, as committed
  r=$(grep -E "^RESULT $id " "$log" | tail -1)
  echo "$r" | tee -a "$D/summary.txt"
  case "$r" in *KILLED) killed=$((killed+1));; esac
done
git diff --quiet -- services/gateway || { echo "ABORT: revert failed"; exit 2; }
echo "killed $killed/$total (threshold 8/8)" | tee -a "$D/summary.txt"
[ "$killed" = 8 ] && [ "$total" = 8 ]
