#!/usr/bin/env bash
# Buoc 7 (quy trinh v3): quyet dinh cuoi bang script, khong LLM.
set -u
cd "$(git rev-parse --show-toplevel)"
D=docs/workflow/frontend-debt-r4
code_sha=$(node -p 'require("./'$D'/gate-report.json").commit_sha')
gate_ok=$(node -p 'Object.values(require("./'$D'/gate-report.json").results).every(r=>r.exit===0)')
only_docs=$( [ -z "$(git diff --name-only "$code_sha" HEAD -- . ':(exclude)docs/workflow')" ] && echo true || echo false )
verdict=$(grep -A3 '^## 7. Overall verdict' $D/evidence.md | grep -oE 'PASS|FAIL' | head -1)
notdone=$(grep -cE '\| *NOT DONE *\|' $D/evidence.md)
open=$(node -p 'const f=require("./'$D'/findings.json");[...f.items,...f.round2.items,...f.round3.items].filter(i=>i.status!=="fixed"&&i.status!=="deferred"||(i.status==="deferred"&&i.severity!=="minor")).length')
echo "gate commit=$code_sha gate_ok=$gate_ok code_unchanged_since_gate=$only_docs evidence=$verdict ac_not_done=$notdone open_or_nonminor_deferred=$open"
if [ "$gate_ok" = true ] && [ "$only_docs" = true ] && [ "$verdict" = PASS ] && [ "$notdone" = 0 ] && [ "$open" = 0 ]; then
  echo "DECISION: READY — High-risk: human sign-off required before merge"; exit 0
fi
echo "DECISION: BACK TO STEP 3"; exit 1
