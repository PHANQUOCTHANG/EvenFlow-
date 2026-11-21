#!/bin/bash
cd D:/Documents/Nam4-1/Project1/EvenFlow-r5-verify/apps/web
export VITEST_MIN_FORKS=1 VITEST_MIN_THREADS=1
L=../../docs/workflow/frontend-debt-r5/verify-logs/mutation; mkdir -p $L
AP=C:/Users/trand/AppData/Local/Temp/claude/f0677a4c773c2732/4fa31633-a9c8-4fc3-b1b5-55941dccbd84/scratchpad/mut/apply.js
DIRS="src/lib/debt5 src/lib/debt5-fix src/lib/debt src/lib/fix2 src/components/checkout src/hooks"
S=$L/summary.txt; : > $S
for id in "$@"; do
  git checkout -q -- src
  node $AP . $id > /dev/null || { echo "$id APPLY_FAIL" >> $S; continue; }
  git diff --stat -- src | tail -1 > $L/$id.log
  timeout 600 npx vitest run $DIRS --reporter=dot >> $L/$id.log 2>&1; rc=$?
  git checkout -q -- src
  line=$(sed 's/\x1b\[[0-9;]*m//g' $L/$id.log | grep -E '^ +Tests ' | tail -1 | sed 's/^ *//')
  failed=$(echo "$line" | grep -oE '[0-9]+ failed' | grep -oE '[0-9]+'); failed=${failed:-0}
  if [ $rc -eq 124 ]; then st=TIMEOUT; elif [ "$failed" -gt 0 ]; then st=KILLED; elif [ $rc -ne 0 ]; then st="KILLED(rc=$rc)"; else st=SURVIVED; fi
  echo "$id $st failed=$failed rc=$rc | $line" >> $S
done
git status --short -- src >> $S
echo DONE >> $S
