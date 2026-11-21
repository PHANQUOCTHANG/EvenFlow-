#!/bin/bash
# Re-verification 2026-11-21: full src/lib + checkout + hooks scope.
cd D:/Documents/Nam4-1/Project1/EvenFlow-frontend-debt-r5/apps/web
L=../../docs/workflow/frontend-debt-r5/verify-logs/mutation/r2; mkdir -p $L
AP=../../docs/workflow/frontend-debt-r5/verify-logs/mutation/apply.js
DIRS="src/lib src/components/checkout src/hooks"
S=$L/${SUM:-summary}.txt; : > $S
for id in "$@"; do
  git checkout -q -- src
  if [ "$id" != BASELINE ]; then node $AP . $id > /dev/null || { echo "$id APPLY_FAIL" >> $S; continue; }; fi
  git diff --stat -- src | tail -1 > $L/$id.log
  timeout 900 env -u VITEST_MAX_THREADS -u VITEST_MAX_FORKS npx vitest run $DIRS --reporter=dot >> $L/$id.log 2>&1; rc=$?
  git checkout -q -- src
  line=$(sed 's/\x1b\[[0-9;]*m//g' $L/$id.log | grep -E '^ +Tests ' | tail -1 | sed 's/^ *//')
  failed=$(echo "$line" | grep -oE '[0-9]+ failed' | grep -oE '[0-9]+'); failed=${failed:-0}
  if [ $rc -eq 124 ]; then st=TIMEOUT; elif [ "$failed" -gt 0 ]; then st=KILLED; elif [ $rc -ne 0 ]; then st="KILLED(rc=$rc)"; else st=SURVIVED; fi
  echo "$id $st failed=$failed rc=$rc | $line" >> $S
done
git status --short -- src >> $S
echo DONE >> $S
