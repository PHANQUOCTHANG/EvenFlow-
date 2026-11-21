#!/usr/bin/env bash
# Independent verifier mutants V1..V10 (not the M1..M8 set). Run from repo root.
# Each: apply perl substitution -> confirm diff -> go test -> revert.
set -u
D=docs/workflow/gateway-trusted-proxy/verify-logs/mutation-verify
CIP=services/gateway/internal/clientip/clientip.go
GW=services/gateway/internal/proxy/gateway.go
declare -a IDS=(V1 V2 V3 V4 V5 V6 V7 V8 V9 V10)
declare -A FILE=( [V1]=$GW [V2]=$GW [V3]=$CIP [V4]=$CIP [V5]=$CIP [V6]=$CIP [V7]=$GW [V8]=$CIP [V9]=$CIP [V10]=$CIP )
declare -A SUB=(
 [V1]='s/"Forwarded", "X-Real-IP", /"Forwarded", /'
 [V2]='s/^(\s*)if fromXFF \{/$1if false \&\& fromXFF {/'
 [V3]='s/const maxTrustedHops = 16/const maxTrustedHops = 17/'
 [V4]='s/return n <= 65535/return n <= 65536/'
 [V5]='s/minBitsIPv6 = 7/minBitsIPv6 = 6/'
 [V6]='s/^(\s*)e = strings\.TrimSpace\(e\)/$1_ = 0/'
 [V7]='s/^(\s*)if fromXFF \{/$1if true || fromXFF {/'
 [V8]='s/^func trimSpaceTab\(s string\) string \{/func trimSpaceTab(s string) string {\n\treturn s/'
 [V9]='s/err != nil \|\| a\.Zone\(\) != "" \|\| \(bracketed \&\& !a\.Is6\(\)\)/err != nil || a.Zone() != "" || (bracketed && false)/'
 [V10]='s/^(\s*)if a\.Is4In6\(\) \{/$1if false \&\& a.Is4In6() {/'
)
declare -A DESC=(
 [V1]="drop X-Real-IP from clientIPHeaders (header no longer stripped)"
 [V2]="fromXFF always false (upstream XFF override never applied)"
 [V3]="hop cap off-by-one 16 -> 17"
 [V4]="port range upper bound 65535 -> 65536"
 [V5]="IPv6 prefix floor /7 -> /6"
 [V6]="per-element TrimSpace removed in ParseTrusted"
 [V7]="XFF override applied even when not fromXFF"
 [V8]="trimSpaceTab is identity (XFF element whitespace not trimmed)"
 [V9]="accept bracketed IPv4 ([1.2.3.4]) in XFF"
 [V10]="accept IPv4-mapped bare IP in TRUSTED_PROXY_CIDRS"
)
: > $D/summary.txt
for id in "${IDS[@]}"; do
  f=${FILE[$id]}; log=$D/$id.log
  perl -pi -e "${SUB[$id]}" "$f"
  { echo "# $id: ${DESC[$id]}"; git -c core.autocrlf=false diff -- "$f"; } > $log
  if git -c core.autocrlf=false diff --quiet -- "$f"; then
    echo "$id NOT_APPLIED ${DESC[$id]}" | tee -a $D/summary.txt; continue
  fi
  (cd services/gateway && go test ./... -count=1 2>&1) >> $log; rc=$?
  git -c core.autocrlf=false checkout -- services/gateway
  if grep -q "build failed" $log; then st=INVALID_BUILD; elif [ $rc -ne 0 ]; then st=KILLED; else st=SURVIVED; fi
  echo "$id $st ${DESC[$id]}" | tee -a $D/summary.txt
done
git -c core.autocrlf=false diff --quiet -- services/gateway && echo "tree clean" | tee -a $D/summary.txt
