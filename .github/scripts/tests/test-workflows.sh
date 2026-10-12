#!/usr/bin/env bash
#
# Kiem TINH cac workflow GitHub Actions theo spec docs/workflow/ci-cd-review/spec.md
# (AC-1..AC-11) va contracts.md (K2..K5).
#
#   bash .github/scripts/tests/test-workflows.sh               # chay tu GOC repo
#   bash .github/scripts/tests/test-workflows.sh <repo-root>   # kiem mot ban sao khac
#                                                              # (vd ban export cua base)
#
# Chi dung bash + coreutils + grep + awk (POSIX awk: chay duoc voi gawk o Git Bash
# va mawk o ubuntu-latest). KHONG gia dinh co yq / PyYAML / node / make.
#
# Goc repo duoc kiem: tham so 1, hoac bien REPO_ROOT, mac dinh thu muc hien tai.
# Bien moi truong (chi de tu kiem bo test nay tren mot ban sao tam, KHONG dung trong CI):
#   WF_DIR=<thu muc>     thay <repo-root>/.github/workflows
#   MAKEFILE=<file>      thay <repo-root>/Makefile
#
# Cach doc YAML: moi file duoc "danh chi muc" mot lan bang awk (xem IDX_AWK):
#   - bo dong trong, bo dong ma ky tu dau tien khac khoang trang la '#', bo comment cuoi
#     dong dang ' #...'. Comment chua `version: latest` hay `permissions:` vi the KHONG
#     duoc tinh.
#   - moi dong con lai duoc gan: khoa top-level, job id, khoa cap job, so thu tu step,
#     khoa cap step, do thut le, noi dung.
# Gia dinh: workflow dung block style (khong viet ca job/step dang flow `{...}`).
#
# Moi assertion in "PASS|FAIL [AC-x] mo ta". Exit != 0 neu co it nhat mot FAIL.

set -u
export LC_ALL=C

ROOT="${1:-${REPO_ROOT:-.}}"
WF="${WF_DIR:-$ROOT/.github/workflows}"
MK="${MAKEFILE:-$ROOT/Makefile}"

if [ ! -d "$WF" ]; then
  echo "Khong thay $WF. Hay chay tu goc repo: bash .github/scripts/tests/test-workflows.sh [repo-root]" >&2
  exit 2
fi

T="$(mktemp -d "${TMPDIR:-/tmp}/wf-test.XXXXXX")" || exit 2
trap 'rm -rf "$T"' EXIT INT TERM

PASSED=0
FAILED=0
FAILED_IDS=()
REASON=""

check() { # check <AC> <mo ta> <ham> [args...]
  local ac="$1" desc="$2"; shift 2
  REASON=""
  if "$@"; then
    PASSED=$((PASSED + 1)); printf 'PASS  [%s] %s\n' "$ac" "$desc"
  else
    FAILED=$((FAILED + 1)); FAILED_IDS+=("$ac")
    printf 'FAIL  [%s] %s\n' "$ac" "$desc"
    [ -n "$REASON" ] && printf '%s\n' "$REASON" | sed 's/^/        /'
  fi
  return 0
}
why() { REASON="${REASON:+$REASON
}$*"; }

# ------------------------------------------------------------------ chi muc YAML
# Cot (tach bang TAB):
#   1 so dong  2 khoa top-level  3 job id  4 khoa cap job  5 so step (0 = khong phai step)
#   6 khoa cap step  7 thut le  8 noi dung (da bo khoang trang dau dong)
cat >"$T/idx.awk" <<'AWK'
function key(s,   k) { k = s; sub(/^- /, "", k); if (k !~ /^[A-Za-z0-9_.-]+:/) return ""; sub(/:.*/, "", k); return k }
BEGIN { OFS = "\t"; top = ""; job = ""; jk = ""; st = 0; sk = ""; jind = -1; pind = -1; insteps = 0; dind = -1 }
{
  raw = $0
  sub(/\r$/, "", raw)
  gsub(/\t/, "    ", raw)
  t = raw; sub(/^ +/, "", t)
  ind = length(raw) - length(t)
  if (t == "" || substr(t, 1, 1) == "#") next
  sub(/[ ]+#.*$/, "", t)
  if (t == "") next
  if (ind == 0) {
    top = key(t); job = ""; jk = ""; st = 0; sk = ""; insteps = 0; jind = -1; pind = -1; dind = -1
    print NR, top, "", "", 0, "", ind, t; next
  }
  if (top != "jobs") { print NR, top, "", "", 0, "", ind, t; next }
  if (jind < 0) jind = ind
  if (ind <= jind) {
    job = key(t); jk = ""; st = 0; sk = ""; insteps = 0; pind = -1; dind = -1
    print NR, top, job, "", 0, "", ind, t; next
  }
  if (pind < 0) pind = ind
  if (insteps) {
    if (dind < 0 && substr(t, 1, 2) == "- ") dind = ind
    if (dind >= 0 && ind == dind && substr(t, 1, 2) == "- ") {
      st++; sk = key(t); print NR, top, job, "steps", st, sk, ind, t; next
    }
    if (dind >= 0 && ind > dind) {
      if (ind == dind + 2 && key(t) != "") sk = key(t)
      print NR, top, job, "steps", st, sk, ind, t; next
    }
    insteps = 0; st = 0; sk = ""
  }
  if (ind == pind) { jk = key(t); if (jk == "steps") { insteps = 1; dind = -1 } }
  print NR, top, job, jk, 0, "", ind, t
}
AWK

idx() { # idx <file> -> duong dan file chi muc (cache)
  local f="$1" out
  out="$T/idx_$(printf '%s' "$f" | tr -c 'A-Za-z0-9' '_').tsv"
  [ -f "$out" ] || awk -f "$T/idx.awk" "$f" >"$out"
  printf '%s' "$out"
}

unq() { # bo dau nhay bao quanh mot gia tri YAML
  local v="$1"
  v="${v#"${v%%[![:space:]]*}"}"; v="${v%"${v##*[![:space:]]}"}"
  case "$v" in \"*\") v="${v#\"}"; v="${v%\"}" ;; \'*\') v="${v#\'}"; v="${v%\'}" ;; esac
  printf '%s' "$v"
}

wf_files() { local f; for f in "$WF"/*.yml "$WF"/*.yaml; do [ -f "$f" ] && printf '%s\n' "$f"; done; }

# Noi dung "sach" (khong comment) cua ca file
ftext() { awk -F'\t' '{print $8}' "$(idx "$1")"; }

# Danh sach job id
jobs_of() { awk -F'\t' '$2=="jobs" && $3!="" && $4=="" && $5==0 {print $3}' "$(idx "$1")"; }

job_name() { # job_name <file> <jobid>
  local v
  v="$(awk -F'\t' -v j="$2" '$3==j && $4=="name" && $5==0 {s=$8; sub(/^name:[ ]*/, "", s); print s; exit}' "$(idx "$1")")"
  unq "$v"
}

job_by_name() { # job_by_name <file> <ten> -> job id (mot lan awk)
  local j
  j="$(awk -F'\t' -v n="$2" '$2=="jobs" && $3!="" && $4=="name" && $5==0 {
        s=$8; sub(/^name:[ ]*/, "", s); sub(/[ ]+$/, "", s)
        if (s ~ /^".*"$/ || s ~ /^\047.*\047$/) s = substr(s, 2, length(s) - 2)
        if (s == n) {print $3; exit}}' "$(idx "$1")")"
  [ -n "$j" ] || return 1
  printf '%s' "$j"
}

job_text() { awk -F'\t' -v j="$2" '$3==j {print $8}' "$(idx "$1")"; }

job_if() { # noi cac dong cua `if:` cap job
  awk -F'\t' -v j="$2" '$3==j && $4=="if" && $5==0 {s=$8; sub(/^if:[ ]*/, "", s); if (s==">-"||s==">"||s=="|"||s=="|-") s=""; printf "%s ", s}' "$(idx "$1")" \
    | sed 's/[[:space:]]*$//'
}

steps_of() { awk -F'\t' -v j="$2" '$3==j && $5>0 {print $5}' "$(idx "$1")" | uniq; }

step_text() { awk -F'\t' -v j="$2" -v s="$3" '$3==j && $5==s {print $8}' "$(idx "$1")"; }

step_key_text() { # step_key_text <file> <job> <step> <khoa> -> cac dong thuoc khoa do
  awk -F'\t' -v j="$2" -v s="$3" -v k="$4" '$3==j && $5==s && $6==k {print $8}' "$(idx "$1")"
}

step_uses() {
  awk -F'\t' -v j="$2" -v s="$3" '$3==j && $5==s && $6=="uses" {u=$8; sub(/^- /, "", u); sub(/^uses:[ ]*/, "", u)
    if (u ~ /^".*"$/ || u ~ /^\047.*\047$/) u = substr(u, 2, length(u) - 2); print u; exit}' "$(idx "$1")"; }

step_with() { # step_with <file> <job> <step> <ten-input> -> gia tri (da bo nhay)
  local v
  v="$(step_key_text "$1" "$2" "$3" with | awk -v k="$4" '{s=$0; if (index(s, k ":")==1) {sub(/^[^:]*:[ ]*/, "", s); print s; exit}}')"
  unq "$v"
}

steps_using() { # steps_using <file> <job> <ERE cua uses> -> so step (mot lan awk)
  RE="$3" awk -F'\t' -v j="$2" '$3==j && $5>0 && $6=="uses" && !($5 in seen) {
      seen[$5]=1; u=$8; sub(/^- /, "", u); sub(/^uses:[ ]*/, "", u); gsub(/^["\047]|["\047]$/, "", u)
      if (u ~ ENVIRON["RE"]) print $5 }' "$(idx "$1")"
}

steps_matching() { # steps_matching <file> <job> <ERE tren noi dung step> -> so step (mot lan awk)
  RE="$3" awk -F'\t' -v j="$2" '$3==j && $5>0 && $8 ~ ENVIRON["RE"] && !($5 in seen) {seen[$5]=1; print $5}' "$(idx "$1")"
}

# Noi dong tiep dien `\` trong script de lenh nhieu dong duoc xet nhu mot dong
joined() { awk '{ if (sub(/\\$/, "")) { buf = buf $0 " "; next } print buf $0; buf = "" } END { if (buf != "") print buf }'; }

# ------------------------------------------------------------------ du lieu dung chung
CI="$WF/ci.yml"; TRIVY="$WF/trivy.yml"; CD="$WF/cd-web.yml"
OVERSELL="$WF/oversell-gate.yml"; INTEG="$WF/integration.yml"; E2E="$WF/e2e-nightly.yml"
CDJOB="$(job_by_name "$CD" 'build & push ghcr web' 2>/dev/null || true)"

echo "== kiem tinh workflow trong $WF =="
echo

# ================================================================== AC-1
ac1_no_v6() {
  local f bad=""
  while read -r f; do
    ftext "$f" | grep -Eq 'golangci/golangci-lint-action@v[0-6]([^0-9]|$)' && bad="$bad $f"
  done < <(wf_files)
  [ -z "$bad" ] || { why "con golangci-lint-action@v6 (hoac cu hon) o:$bad"; return 1; }
}
ac1_no_latest() {
  local f bad=""
  while read -r f; do
    ftext "$f" | grep -Eq '^(- )?version:[ ]*["'"'"']?latest["'"'"']?$' && bad="$bad $f"
  done < <(wf_files)
  [ -z "$bad" ] || { why "con 'version: latest' (dong khong phai comment) o:$bad"; return 1; }
}
ac1_lint_go_runs_golangci() {
  local j; j="$(job_by_name "$CI" lint-go)" || { why "khong co job lint-go"; return 1; }
  [ -n "$(steps_using "$CI" "$j" '^golangci/golangci-lint-action@')$(steps_matching "$CI" "$j" 'golangci-lint (run|version)')" ] \
    || { why "job lint-go khong con goi golangci-lint"; return 1; }
}
ac1_action_major() {
  local j s u bad="" n=0; j="$(job_by_name "$CI" lint-go)" || { why "khong co job lint-go"; return 1; }
  for s in $(steps_using "$CI" "$j" '^golangci/golangci-lint-action@'); do
    n=$((n + 1)); u="$(step_uses "$CI" "$j" "$s")"
    printf '%s' "$u" | grep -Eq '@(v([7-9]|[1-9][0-9])([^0-9].*)?|[0-9a-f]{40})$' || bad="$bad $u"
  done
  [ -z "$bad" ] || { why "action khong ho tro golangci-lint v2 (can >= v7 hoac SHA):$bad"; return 1; }
  return 0
}
ac1_pinned_v2() {
  local j s v bad="" n=0; j="$(job_by_name "$CI" lint-go)" || { why "khong co job lint-go"; return 1; }
  for s in $(steps_using "$CI" "$j" '^golangci/golangci-lint-action@'); do
    n=$((n + 1)); v="$(step_with "$CI" "$j" "$s" version)"
    printf '%s' "$v" | grep -Eq '^v2\.[0-9]+\.[0-9]+$' || bad="$bad step#$s:version='$v'"
  done
  # Cai qua `run:` (go install / install.sh) thay vi action
  for s in $(steps_matching "$CI" "$j" 'golangci-lint'); do
    [ -n "$(step_uses "$CI" "$j" "$s")" ] && continue
    step_key_text "$CI" "$j" "$s" run | grep -q 'golangci-lint' || continue
    if step_key_text "$CI" "$j" "$s" run | grep -Eq '(install|curl|wget)'; then
      n=$((n + 1))
      step_key_text "$CI" "$j" "$s" run | grep -Eq '(@|[ =])v2\.[0-9]+\.[0-9]+([^0-9.]|$)' || bad="$bad step#$s:(run khong pin v2.X.Y)"
    fi
  done
  [ "$n" -gt 0 ] || { why "khong tim thay cho nao chon phien ban golangci-lint trong lint-go"; return 1; }
  [ -z "$bad" ] || { why "golangci-lint khong pin dang v2.X.Y (config .golangci.yml la version \"2\"):$bad"; return 1; }
}
check AC-1 "khong workflow nao con golangci/golangci-lint-action@v6 (hay cu hon)" ac1_no_v6
check AC-1 "khong workflow nao con 'version: latest' (bo qua comment)" ac1_no_latest
check AC-1 "job lint-go van goi golangci-lint" ac1_lint_go_runs_golangci
check AC-1 "golangci-lint-action trong lint-go la ban ho tro v2 (@v7+ hoac SHA)" ac1_action_major
check AC-1 "golangci-lint duoc pin dang vX.Y.Z voi X=2 (khop .golangci.yml version \"2\")" ac1_pinned_v2

# ================================================================== AC-2 / K4
TRIVY_STEP=""
if [ -n "$CDJOB" ]; then TRIVY_STEP="$(steps_using "$CD" "$CDJOB" '^aquasecurity/trivy-action@' | head -1)"; fi
BUILD_STEPS=""
if [ -n "$CDJOB" ]; then BUILD_STEPS="$(steps_using "$CD" "$CDJOB" '^docker/build-push-action@')"; fi

need_cdjob() { [ -n "$CDJOB" ] || { why "cd-web.yml khong co job 'build & push ghcr web'"; return 1; }; }
need_trivy() { need_cdjob || return 1; [ -n "$TRIVY_STEP" ] || { why "job CD khong co step aquasecurity/trivy-action"; return 1; }; }

ac2_trivy_exists() { need_trivy; }
ac2_severity() { need_trivy || return 1; local v; v="$(step_with "$CD" "$CDJOB" "$TRIVY_STEP" severity)"
  [ "$v" = CRITICAL ] || { why "severity='$v' (trivy.yml dung CRITICAL)"; return 1; }; }
ac2_unfixed() { need_trivy || return 1; local v; v="$(step_with "$CD" "$CDJOB" "$TRIVY_STEP" ignore-unfixed)"
  [ "$v" = true ] || { why "ignore-unfixed='$v'"; return 1; }; }
ac2_exitcode() { need_trivy || return 1; local v; v="$(step_with "$CD" "$CDJOB" "$TRIVY_STEP" exit-code)"
  [ "$v" = 1 ] || { why "exit-code='$v' (phai la '1' de quet fail thi step fail)"; return 1; }; }
ac2_image_scan() { need_trivy || return 1; local r t
  r="$(step_with "$CD" "$CDJOB" "$TRIVY_STEP" image-ref)"; t="$(step_with "$CD" "$CDJOB" "$TRIVY_STEP" scan-type)"
  [ -n "$r" ] || { why "step Trivy khong co image-ref (khong quet image)"; return 1; }
  [ -z "$t" ] || [ "$t" = image ] || { why "scan-type='$t' (phai quet image)"; return 1; }; }
ac2_no_continue() { need_trivy || return 1
  if step_text "$CD" "$CDJOB" "$TRIVY_STEP" | grep -Eq '^continue-on-error:[ ]*["'"'"']?true'; then why "step Trivy co continue-on-error: true"; return 1; fi
  if job_text "$CD" "$CDJOB" | grep -Eq '^continue-on-error:[ ]*["'"'"']?true'; then why "co continue-on-error: true trong job CD"; return 1; fi; }
ac2_no_push_true() {
  if ftext "$CD" | grep -Eq '^(- )?push:[ ]*["'"'"']?true'; then why "cd-web.yml con 'push: true'"; return 1; fi; }
ac2_load_true() { need_cdjob || return 1
  [ -n "$BUILD_STEPS" ] || { why "khong co step docker/build-push-action"; return 1; }
  local s v bad=""
  for s in $BUILD_STEPS; do v="$(step_with "$CD" "$CDJOB" "$s" load)"; [ "$v" = true ] || bad="$bad step#$s:load='$v'"; done
  [ -z "$bad" ] || { why "build-push-action phai build vao daemon local voi load: true:$bad"; return 1; }; }
ac2_build_before_scan() { need_trivy || return 1
  local first; first="$(printf '%s\n' $BUILD_STEPS | head -1)"
  [ -n "$first" ] && [ "$first" -lt "$TRIVY_STEP" ] || { why "build (step#${first:-?}) phai dung truoc Trivy (step#$TRIVY_STEP)"; return 1; }; }
PUSH_STEPS=""
if [ -n "$CDJOB" ]; then
  for s in $(steps_of "$CD" "$CDJOB"); do
    step_key_text "$CD" "$CDJOB" "$s" run | joined | grep -Eq '(^|[^A-Za-z0-9_-])docker[ ]+(image[ ]+)?push([ ]|$)' && PUSH_STEPS="$PUSH_STEPS $s"
  done
fi
ac2_push_after_scan() { need_trivy || return 1
  [ -n "${PUSH_STEPS// /}" ] || { why "khong co step 'docker push' nao trong job CD"; return 1; }
  local s bad=""
  for s in $PUSH_STEPS; do [ "$s" -gt "$TRIVY_STEP" ] || bad="$bad $s"; done
  [ -z "$bad" ] || { why "docker push o step#$bad dung TRUOC Trivy (step#$TRIVY_STEP)"; return 1; }; }
ac2_push_not_forced() { need_cdjob || return 1
  [ -n "${PUSH_STEPS// /}" ] || { why "khong co step 'docker push' nao de kiem"; return 1; }
  local s bad=""
  for s in $PUSH_STEPS; do
    step_key_text "$CD" "$CDJOB" "$s" if | grep -Eq '(always|failure|cancelled)\(\)' && bad="$bad $s"
  done
  [ -z "$bad" ] || { why "step push#$bad co if: always()/failure()/cancelled() -> van chay khi Trivy fail"; return 1; }; }
ac2_no_rebuild_after_scan() { need_trivy || return 1
  local s bad=""
  for s in $(steps_of "$CD" "$CDJOB"); do
    [ "$s" -gt "$TRIVY_STEP" ] || continue
    step_uses "$CD" "$CDJOB" "$s" | grep -q '^docker/build-push-action@' && bad="$bad $s"
    step_key_text "$CD" "$CDJOB" "$s" run | joined | grep -Eq 'docker[ ]+(buildx[ ]+)?build([ ]|$)' && bad="$bad $s"
  done
  [ -z "$bad" ] || { why "co build lai sau khi quet (step#$bad) -> image push khong phai image da quet"; return 1; }; }
ac2_thirdparty_sha() {
  local u bad=""
  while read -r u; do
    u="$(unq "$u")"; [ -z "$u" ] && continue
    case "$u" in actions/*|./*|docker://*) continue ;; esac
    printf '%s' "$u" | grep -Eq '^[A-Za-z0-9_.-]+/[A-Za-z0-9_./-]+@[0-9a-f]{40}$' || bad="$bad $u"
  done < <(ftext "$CD" | sed -n 's/^\(- \)\{0,1\}uses:[ ]*//p')
  [ -z "$bad" ] || { why "action ben thu ba chua pin SHA 40 hex:$bad"; return 1; }; }
check AC-2 "cd-web: job CD co step aquasecurity/trivy-action" ac2_trivy_exists
check AC-2 "cd-web: Trivy severity = CRITICAL (dung chinh sach trivy.yml)" ac2_severity
check AC-2 "cd-web: Trivy ignore-unfixed = true" ac2_unfixed
check AC-2 "cd-web: Trivy exit-code = '1'" ac2_exitcode
check AC-2 "cd-web: Trivy quet IMAGE (image-ref, khong phai fs)" ac2_image_scan
check AC-2 "cd-web: Trivy / job CD khong co continue-on-error: true" ac2_no_continue
check AC-2 "cd-web: khong con 'push: true' o bat ky dau (K4.1)" ac2_no_push_true
check AC-2 "cd-web: moi docker/build-push-action dung load: true (K4.1)" ac2_load_true
check AC-2 "cd-web: build dung TRUOC Trivy (K4)" ac2_build_before_scan
check AC-2 "cd-web: co 'docker push' va moi docker push dung SAU Trivy (K4.3)" ac2_push_after_scan
check AC-2 "cd-web: step docker push khong co if: always()/failure()/cancelled()" ac2_push_not_forced
check AC-2 "cd-web: khong build lai sau Trivy (push dung image da quet)" ac2_no_rebuild_after_scan
check AC-2 "cd-web: moi uses: ben thu ba (khong phai actions/*) pin SHA 40 hex (K4)" ac2_thirdparty_sha

ac2_trivy_no_if() { need_trivy || return 1
  local v; v="$(step_key_text "$CD" "$CDJOB" "$TRIVY_STEP" if)"
  [ -z "$v" ] || { why "step Trivy co if: ($v) => co the bi skip ma cac step sau van chay"; return 1; }; }

with_block() { # with_block <file> <job> <step> <input> -> moi gia tri cua input (block scalar hoac inline, tach dau phay)
  awk -F'\t' -v j="$2" -v s="$3" -v k="$4" '
    $3==j && $5==s && $6=="with" {
      if (inb && $7 > ki) { print $8; next }
      inb = 0
      if (index($8, k ":") == 1) { ki = $7; v = $8; sub(/^[^:]*:[ ]*/, "", v)
        if (v ~ /^[|>][-+]?$/ || v == "") inb = 1
        else { n = split(v, a, ","); for (i = 1; i <= n; i++) print a[i] } } }' "$(idx "$1")" \
    | while read -r x; do unq "$x"; echo; done | tr -d ' ' | grep -v '^$'
}
step_envmap() { # step_envmap <file> <job> <step> -> "TEN<TAB>gia-tri", env cua step roi env cua job
  { step_key_text "$1" "$2" "$3" env; awk -F'\t' -v j="$2" '$3==j && $4=="env" && $5==0 {print $8}' "$(idx "$1")"; } \
    | sed -n 's/^\([A-Za-z_][A-Za-z0-9_]*\):[ ]*\(.*\)$/\1\t\2/p'
}
pushed_refs() { # moi ref ma cac step docker push day len (da thay bien env, bo khoang trang va nhay)
  local s line name val refs
  for s in $PUSH_STEPS; do
    local map; map="$(step_envmap "$CD" "$CDJOB" "$s" | awk -F'\t' '{print length($1) "\t" $0}' | sort -rn | cut -f2-)"
    while read -r line; do
      while IFS="$(printf '\t')" read -r name val; do
        [ -z "$name" ] && continue; val="$(unq "$val")"
        line="${line//\$\{$name\}/$val}"; line="${line//\$$name/$val}"
      done <<<"$map"
      printf '%s\n' "$line"
    done < <(step_key_text "$CD" "$CDJOB" "$s" run | joined | grep -Eo 'docker[ ]+(image[ ]+)?push[ ]+[^;&|]*' \
               | sed -E 's/^docker[ ]+(image[ ]+)?push[ ]+//')
  done | tr -d "\"'" | sed 's/\${{[ ]*/${{/g; s/[ ]*}}/}}/g' | tr ' ' '\n' | grep -v '^-' | grep -v '^$' | sed 's/[[:space:]]//g'
}
ac2_scanned_is_pushed() { need_trivy || return 1
  [ -n "${PUSH_STEPS// /}" ] || { why "khong co step docker push"; return 1; }
  local r; r="$(step_with "$CD" "$CDJOB" "$TRIVY_STEP" image-ref | tr -d ' ')"
  pushed_refs | grep -qxF -- "$r" || { why "image-ref Trivy '$r' khong nam trong cac ref duoc docker push:" "$(pushed_refs | sed 's/^/  /')"; return 1; }; }
ac2_pushed_are_built() { need_cdjob || return 1
  [ -n "${PUSH_STEPS// /}" ] || { why "khong co step docker push"; return 1; }
  local s tags="" p bad=""
  for s in $BUILD_STEPS; do tags="$tags
$(with_block "$CD" "$CDJOB" "$s" tags)"; done
  while read -r p; do printf '%s\n' "$tags" | grep -qxF -- "$p" || bad="$bad $p"; done < <(pushed_refs)
  [ -z "$bad" ] || { why "ref duoc push khong phai tag cua image da build/quet:$bad"; return 1; }; }
ac2_login_order() { need_trivy || return 1
  local l bad=""; l="$(steps_using "$CD" "$CDJOB" '^docker/login-action@')"
  [ -n "$l" ] || { why "khong co docker/login-action"; return 1; }
  local first_push; first_push="$(printf '%s\n' $PUSH_STEPS | sort -n | head -1)"
  for s in $l; do
    [ "$s" -gt "$TRIVY_STEP" ] || bad="$bad login#$s<=trivy#$TRIVY_STEP"
    [ -n "$first_push" ] && [ "$s" -lt "$first_push" ] || bad="$bad login#$s>=push#${first_push:-?}"
  done
  [ -z "$bad" ] || { why "docker/login-action phai nam SAU Trivy va TRUOC docker push:$bad"; return 1; }; }
ac2_persist_creds() { need_cdjob || return 1
  local s bad="" n=0
  for s in $(steps_using "$CD" "$CDJOB" '^actions/checkout@'); do
    n=$((n + 1)); [ "$(step_with "$CD" "$CDJOB" "$s" persist-credentials)" = false ] || bad="$bad step#$s"
  done
  [ "$n" -gt 0 ] || { why "job CD khong co actions/checkout"; return 1; }
  [ -z "$bad" ] || { why "checkout khong co persist-credentials: false:$bad"; return 1; }; }
check AC-2 "cd-web: step Trivy KHONG co if:" ac2_trivy_no_if
check AC-2 "cd-web: image-ref cua Trivy la mot trong cac ref duoc docker push (K4.3)" ac2_scanned_is_pushed
check AC-2 "cd-web: moi ref duoc docker push la tag cua image da build (load) roi quet" ac2_pushed_are_built
check AC-2 "cd-web: docker/login-action nam SAU Trivy va TRUOC docker push" ac2_login_order
check AC-2 "cd-web: actions/checkout co persist-credentials: false" ac2_persist_creds

# ================================================================== AC-3 / K4
ac3_build_arg() { need_cdjob || return 1
  [ -n "$BUILD_STEPS" ] || { why "khong co step build-push-action"; return 1; }
  local s
  for s in $BUILD_STEPS; do
    step_key_text "$CD" "$CDJOB" "$s" with | grep -Eq 'NEXT_PUBLIC_API_BASE=\$\{\{[ ]*vars\.NEXT_PUBLIC_API_BASE[ ]*\}\}' && return 0
  done
  why "build-push-action khong co build-arg NEXT_PUBLIC_API_BASE=\${{ vars.NEXT_PUBLIC_API_BASE }}"; return 1; }
ac3_build_args_key() { need_cdjob || return 1
  local s
  for s in $BUILD_STEPS; do
    step_key_text "$CD" "$CDJOB" "$s" with | awk '/^build-args:/{f=1} f' | grep -q 'NEXT_PUBLIC_API_BASE=' && return 0
  done
  why "NEXT_PUBLIC_API_BASE khong nam duoi build-args:"; return 1; }
WARN_STEP=""
if [ -n "$CDJOB" ]; then
  for s in $(steps_matching "$CD" "$CDJOB" '::warning'); do
    step_text "$CD" "$CDJOB" "$s" | grep -q 'NEXT_PUBLIC_API_BASE' && { WARN_STEP="$s"; break; }
  done
fi
ac3_warning() { need_cdjob || return 1
  [ -n "$WARN_STEP" ] || { why "khong co step nao vua in ::warning vua lien quan NEXT_PUBLIC_API_BASE"; return 1; }
  step_key_text "$CD" "$CDJOB" "$WARN_STEP" run | grep -Eq '(^|[^A-Za-z])-z[ ]|=[ ]*""|==[ ]*""|-n[ ]' \
    || { why "step#$WARN_STEP khong co kiem tra bien rong (-z / -n / == \"\")"; return 1; }; }
ac3_env_not_run() {
  local f bad=""
  while read -r f; do
    awk -F'\t' '$6=="run" && $8 ~ /vars\.NEXT_PUBLIC_API_BASE/' "$(idx "$f")" | grep -q . && bad="$bad $f"
  done < <(wf_files)
  [ -z "$bad" ] || { why "\${{ vars.NEXT_PUBLIC_API_BASE }} bi noi suy thang trong run: o:$bad"; return 1; }; }
ac3_warn_via_env() { need_cdjob || return 1
  [ -n "$WARN_STEP" ] || { why "khong co step canh bao"; return 1; }
  step_key_text "$CD" "$CDJOB" "$WARN_STEP" env | grep -Eq 'vars\.NEXT_PUBLIC_API_BASE' \
    || { why "step canh bao (#$WARN_STEP) khong nhan vars.NEXT_PUBLIC_API_BASE qua env:"; return 1; }; }
ac3_summary() { need_cdjob || return 1
  local s
  for s in $(steps_matching "$CD" "$CDJOB" 'GITHUB_STEP_SUMMARY'); do
    step_text "$CD" "$CDJOB" "$s" | grep -q 'API_BASE' && return 0
  done
  why "khong step nao vua ghi GITHUB_STEP_SUMMARY vua nhac API_BASE"; return 1; }
check AC-3 "cd-web: build-push-action truyen NEXT_PUBLIC_API_BASE=\${{ vars.NEXT_PUBLIC_API_BASE }}" ac3_build_arg
check AC-3 "cd-web: gia tri do nam duoi build-args:" ac3_build_args_key
check AC-3 "cd-web: co step in ::warning khi NEXT_PUBLIC_API_BASE rong (co kiem tra rong)" ac3_warning
check AC-3 "khong workflow nao noi suy vars.NEXT_PUBLIC_API_BASE thang vao run: (K4)" ac3_env_not_run
check AC-3 "cd-web: step canh bao doc bien qua env: cua step (K4)" ac3_warn_via_env
check AC-3 "cd-web: co dong job summary nhac API base (GITHUB_STEP_SUMMARY)" ac3_summary

# ================================================================== AC-4 / K1 / K2
ac4_oversell_wrapper() {
  local j s; j="$(job_by_name "$OVERSELL" oversell-gate)" || { why "khong co job oversell-gate"; return 1; }
  s="$(steps_matching "$OVERSELL" "$j" 'run-and-report-tests\.sh' | head -1)"
  [ -n "$s" ] || { why "oversell-gate khong goi run-and-report-tests.sh"; return 1; }
  local r; r="$(step_key_text "$OVERSELL" "$j" "$s" run | joined)"
  printf '%s\n' "$r" | grep -Eq 'run-and-report-tests\.sh["'"'"']?[ ]+["'"'"']?TestNoOversell' || why "tham so <test-regex> khong phai TestNoOversell"
  printf '%s\n' "$r" | grep -Eq 'run-and-report-tests\.sh.*make[ ]+test-oversell' || why "khong boc 'make test-oversell'"
  printf '%s\n' "$r" | grep -Eq 'TESTFLAGS=["'"'"']?([^"'"'"' ]* )*-v([ "'"'"']|$)' || why "khong truyen TESTFLAGS co -v (khong co -v thi go test khong in '--- PASS')"
  [ -z "$REASON" ]; }
ac4_integration_wrapper() {
  local j s; j="$(job_by_name "$INTEG" integration)" || { why "khong co job integration"; return 1; }
  s="$(steps_matching "$INTEG" "$j" 'run-and-report-tests\.sh' | head -1)"
  [ -n "$s" ] || { why "integration khong goi run-and-report-tests.sh"; return 1; }
  local r; r="$(step_key_text "$INTEG" "$j" "$s" run | joined)"
  printf '%s\n' "$r" | grep -Eq 'run-and-report-tests\.sh["'"'"']?[ ]+["'"'"']?TestNoOversell' || why "tham so <test-regex> khong phai TestNoOversell"
  printf '%s\n' "$r" | grep -Eq 'run-and-report-tests\.sh.*go[ ]+test' || why "khong boc 'go test'"
  printf '%s\n' "$r" | grep -Eq 'go[ ]+test.*-run[ =]["'"'"']?TestNoOversell' || why "go test khong -run TestNoOversell"
  printf '%s\n' "$r" | grep -Eq 'go[ ]+test.* -v([ ]|$)' || why "go test khong co -v"
  printf '%s\n' "$r" | grep -Eq -- '-count=20([^0-9]|$)' || why "smoke khong con -count=20 (AC-11: khong doi hanh vi)"
  [ -z "$REASON" ]; }

# Chuan hoa mot lenh shell nhieu dong: bo CR, noi dong tiep dien `\` (ke ca tab dau dong ke
# tiep ma `make -n` in ra), gom moi chuoi khoang trang thanh mot dau cach. CA HAI nhanh duoi
# (make -n va mo phong bang awk) deu di qua day, nen so cung mot dang; mot co bi bo hay doi
# van lam chuoi khac di.
norm_cmd() {
  tr -d '\r' \
    | awk '{ if (sub(/\\[ \t]*$/, "")) { buf = buf $0 " "; next } print buf $0; buf = "" } END { if (buf != "") print buf }' \
    | tr -s ' \t' '  ' | sed 's/^ //;s/ $//'
}
# Lay recipe cua target test-oversell, thay $(TESTFLAGS) bang gia tri cho truoc, chuan hoa.
# Dung `make -n` neu co make (ubuntu-latest), khong thi mo phong bang awk (Git Bash).
# Makefile co the la CRLF (core.autocrlf tren Windows) => bo CR truoc khi doc.
mk_recipe() { # mk_recipe <gia-tri-TESTFLAGS|__UNSET__>
  local val="$1"
  if command -v make >/dev/null 2>&1; then
    local mkf="$T/Makefile.lf"; tr -d '\r' <"$MK" >"$mkf"
    if [ "$val" = __UNSET__ ]; then make -s -n -f "$mkf" test-oversell 2>/dev/null
    else make -s -n -f "$mkf" test-oversell TESTFLAGS="$val" 2>/dev/null; fi | norm_cmd
    return
  fi
  local def
  def="$(tr -d '\r' <"$MK" | awk '/^TESTFLAGS[ \t]*[?:]?=/{s=$0; sub(/^TESTFLAGS[ \t]*[?:]?=[ \t]*/, "", s); print s; exit}')"
  [ "$val" = __UNSET__ ] && val="$def"
  tr -d '\r' <"$MK" | awk '
    /^test-oversell[ \t]*:/ {f = 1; next}
    f && /^\t/ { s = $0; sub(/^\t+/, "", s); sub(/^[@-]+/, "", s); print s; next }
    f && !/^\t/ { exit }' \
    | awk -v v="$val" '{ gsub(/\$\(TESTFLAGS\)|\$\{TESTFLAGS\}/, v); print }' \
    | norm_cmd
}
ORIG_OVERSELL='go test -tags=integration -run TestNoOversell ./services/ticketing/test/concurrency/... -count=200 -timeout=30m'
ac4_mk_uses_testflags() {
  tr -d '\r' <"$MK" | awk '/^test-oversell[ \t]*:/{f=1;next} f&&/^\t/{print} f&&!/^\t/{exit}' | grep -Eq '\$\(TESTFLAGS\)|\$\{TESTFLAGS\}' \
    || { why "recipe test-oversell khong dung \$(TESTFLAGS)"; return 1; }; }
ac4_mk_unchanged() {
  local got; got="$(mk_recipe __UNSET__)"
  [ "$got" = "$ORIG_OVERSELL" ] || { why "khong truyen TESTFLAGS: '$got'" "mong doi: '$ORIG_OVERSELL'"; return 1; }; }
ac4_mk_with_v() {
  local got; got="$(mk_recipe -v)"
  printf ' %s ' "$got" | grep -q ' -v ' || { why "TESTFLAGS=-v: '$got' khong co token -v"; return 1; }
  [ "$(printf '%s' "$got" | sed 's/ -v / /;s/ -v$//')" = "$ORIG_OVERSELL" ] || { why "ngoai -v lenh con doi khac: '$got'"; return 1; }; }
check AC-4 "oversell-gate: goi run-and-report-tests.sh TestNoOversell boc 'make test-oversell TESTFLAGS=-v'" ac4_oversell_wrapper
check AC-4 "integration: smoke goi run-and-report-tests.sh TestNoOversell boc 'go test -v -run TestNoOversell -count=20'" ac4_integration_wrapper
ac4_wrapper_not_soft() { # ac4_wrapper_not_soft <file> <ten-job>: exit code cua wrapper khong bi nuot
  local j s r; j="$(job_by_name "$1" "$2")" || { why "khong co job $2"; return 1; }
  s="$(steps_matching "$1" "$j" 'run-and-report-tests\.sh' | head -1)"
  [ -n "$s" ] || { why "khong co step goi run-and-report-tests.sh"; return 1; }
  step_key_text "$1" "$j" "$s" continue-on-error | grep -Eqv 'continue-on-error:[ ]*["'"'"']?false' && why "step co continue-on-error"
  awk -F'\t' -v j="$j" '$3==j && $4=="continue-on-error" && $5==0 && $8 !~ /false/ {f=1} END{exit !f}' "$(idx "$1")" && why "job $2 co continue-on-error"
  r="$(step_key_text "$1" "$j" "$s" run | joined)"
  printf '%s\n' "$r" | grep -Eq '\|\|[ ]*(true|:|exit[ ]+0)([ ;)]|$)|;[ ]*(true|:)([ ;)]|$)|^[ ]*(true|:)[ ]*$' \
    && why "exit code cua wrapper bi nuot (|| true / ; true):$(printf '%s' "$r" | tr '\n' ' ' | cut -c1-160)"
  [ -z "$REASON" ]; }
check AC-4 "oversell-gate: exit code cua run-and-report-tests.sh khong bi nuot (|| true, ; true, continue-on-error)" ac4_wrapper_not_soft "$OVERSELL" oversell-gate
check AC-4 "integration: exit code cua run-and-report-tests.sh khong bi nuot (|| true, ; true, continue-on-error)" ac4_wrapper_not_soft "$INTEG" integration
check AC-4 "Makefile: recipe test-oversell dung \$(TESTFLAGS) (K2)" ac4_mk_uses_testflags
check AC-4 "Makefile: khong truyen TESTFLAGS => lenh y het hien tai (K2, khong hoi quy)" ac4_mk_unchanged
check AC-4 "Makefile: TESTFLAGS=-v => them dung token -v, phan con lai giu nguyen (K2)" ac4_mk_with_v

# ================================================================== AC-5
# Bo loc dorny/paths-filter cua job detect-changes: in "nhom<TAB>pattern"
filters_of() { # filters_of <file> (co cache)
  local c; c="$T/flt_$(printf '%s' "$1" | tr -c 'A-Za-z0-9' '_')"
  [ -f "$c" ] || _filters_of "$1" >"$c"
  cat "$c"
}
_filters_of() {
  local j s; j="$(job_by_name "$1" detect-changes)" || return 0
  for s in $(steps_using "$1" "$j" '^dorny/paths-filter@'); do
    step_key_text "$1" "$j" "$s" with | awk '
      /^filters:/ {f = 1; next}
      f && /^[A-Za-z0-9_-]+:[ ]*(&[A-Za-z0-9_-]+)?$/ {g = $0; sub(/:.*/, "", g); next}
      f && /^- / {p = $0; sub(/^- [ ]*/, "", p); gsub(/^["\047]|["\047]$/, "", p); if (g != "" && p !~ /^!/) print g "\t" p; next}
      f && /^[A-Za-z0-9_-]+:/ {f = 0}'
  done
}
# Ten output cua job detect-changes -> ten nhom filter
out_to_group() { # out_to_group <file> <output>
  local j g; j="$(job_by_name "$1" detect-changes)" || { printf '%s' "$2"; return; }
  g="$(awk -F'\t' -v j="$j" -v o="$2" '$3==j && $4=="outputs" && index($8, o ":")==1 {s=$8; if (match(s, /steps\.[A-Za-z0-9_-]+\.outputs\.[A-Za-z0-9_-]+/)) {s=substr(s, RSTART, RLENGTH); sub(/.*\./, "", s); print s; exit}}' "$(idx "$1")")"
  printf '%s' "${g:-$2}"
}
groups_gating() { # groups_gating <file> <jobid> -> nhom filter ma if: cua job tham chieu
  local o
  for o in $(job_if "$1" "$2" | grep -Eo 'outputs\.[A-Za-z0-9_-]+' | sed 's/^outputs\.//' | sort -u); do
    out_to_group "$1" "$o"; echo
  done
}
path_in_groups() { # path_in_groups <file> <path> <nhom...>
  local f="$1" p="$2"; shift 2
  local g pat
  while IFS="$(printf '\t')" read -r g pat; do
    local hit=0 x; for x in "$@"; do [ "$x" = "$g" ] && hit=1; done
    [ "$hit" = 1 ] || continue
    pat="${pat//\*\*/*}"
    # shellcheck disable=SC2053
    [[ "$p" == $pat ]] && return 0
  done < <(filters_of "$f")
  return 1
}
groups_matching() { # groups_matching <file> <path> -> moi nhom co pattern khop
  local g pat
  while IFS="$(printf '\t')" read -r g pat; do
    pat="${pat//\*\*/*}"
    # shellcheck disable=SC2053
    [[ "$2" == $pat ]] && echo "$g"
  done < <(filters_of "$1") | sort -u
}
ac5_job_runs_on() { # ac5_job_runs_on <file> <ten-job> <path>
  local j; j="$(job_by_name "$1" "$2")" || { why "khong co job $2"; return 1; }
  local gs; gs="$(groups_gating "$1" "$j")"
  [ -n "$gs" ] || { why "if: cua $2 khong tham chieu output filter nao: '$(job_if "$1" "$j")'"; return 1; }
  # shellcheck disable=SC2086
  path_in_groups "$1" "$3" $gs || { why "$3 khong khop pattern nao cua nhom: $(echo $gs)"; return 1; }; }
manifest_job_for() { # manifest_job_for <path> <ERE lenh 1> <ERE lenh 2> -> ok neu co job chay dung khi path doi
  local gs j g s; gs="$(groups_matching "$CI" "$1")"
  [ -n "$gs" ] || { why "$1 khong khop filter nao trong ci.yml"; return 1; }
  for j in $(jobs_of "$CI"); do
    local jg; jg="$(groups_gating "$CI" "$j")"
    local hit=0; for g in $gs; do printf '%s\n' "$jg" | grep -qx "$g" && hit=1; done
    [ "$hit" = 1 ] || continue
    for s in $(steps_matching "$CI" "$j" "$2"); do
      local r; r="$(step_key_text "$CI" "$j" "$s" run | joined)"
      printf '%s\n' "$r" | grep -Eq -- "$2" && printf '%s\n' "$r" | grep -Eq -- "$3" && return 0
    done
  done
  why "khong job nao (gated boi nhom: $(echo $gs)) chay lenh khop '$2' + '$3'"; return 1; }
ac5_k8s() { manifest_job_for deploy/k8s/10-deployment.yaml 'kubeconform' '-strict'; }
ac5_compose() { manifest_job_for deploy/compose/docker-compose.yml 'docker[ ]+compose' '(^|[ ])config([ ]|$)'; }
for p in deploy/docker/web.Dockerfile .github/scripts/gates.sh .github/workflows/ci.yml; do
  for jn in lint-web test-web; do
    check AC-5 "ci.yml: doi $p => job $jn chay" ac5_job_runs_on "$CI" "$jn" "$p"
  done
done
for jn in lint-go test-go lint-python test-python; do
  check AC-5 "ci.yml: doi .github/workflows/ci.yml => job $jn chay" ac5_job_runs_on "$CI" "$jn" .github/workflows/ci.yml
done
check AC-5 "ci.yml: doi deploy/k8s/** => co job chay 'kubeconform -strict'" ac5_k8s
check AC-5 "ci.yml: doi deploy/compose/** => co job chay 'docker compose ... config'" ac5_compose
check AC-5 "trivy.yml: doi .github/workflows/trivy.yml => job trivy (scan) chay" ac5_job_runs_on "$TRIVY" trivy .github/workflows/trivy.yml
# go.work, moi services/** (Dockerfile Go COPY go.mod cua MOI module, AC-12) va .dockerignore
# deu doi duoc noi dung image => Trivy phai quet lai.
for p in go.work services/gateway/go.mod services/notification/cmd/main.go services/ai-worker/app/main.py .dockerignore; do
  check AC-5 "trivy.yml: doi $p => job trivy (scan) chay" ac5_job_runs_on "$TRIVY" trivy "$p"
done

# ================================================================== AC-6
GATES_JOB="$(RE='(^|[ ;&|("/])\.github/scripts/gates\.sh(["'"'"' ]|$)' \
  awk -F'\t' '$3!="" && $6=="run" && $8 ~ ENVIRON["RE"] {print $3; exit}' "$(idx "$CI")")"
need_gates() { [ -n "$GATES_JOB" ] || { why "ci.yml khong co job nao chay .github/scripts/gates.sh"; return 1; }; }
ac6_job() { need_gates; }
ac6_fetch_depth() { need_gates || return 1
  local s
  for s in $(steps_using "$CI" "$GATES_JOB" '^actions/checkout@'); do
    [ "$(step_with "$CI" "$GATES_JOB" "$s" fetch-depth)" = 0 ] && return 0
  done
  why "checkout cua job $GATES_JOB khong co fetch-depth: 0"; return 1; }
ac6_base_ref_set() { need_gates || return 1
  # BASE_REF phai TOI DUOC gates.sh (tien trinh con): khai trong env: (job/step), hoac
  # `export BASE_REF` / `declare -x`, hoac tien to `BASE_REF=... bash ...gates.sh`.
  # Gan tran `BASE_REF="$x"` tren mot dong rieng KHONG du: bien shell khong duoc ke thua.
  local idxf; idxf="$(idx "$CI")"
  awk -F'\t' -v j="$GATES_JOB" '$3==j && (($4=="env" && $5==0) || $6=="env") && $8 ~ /^BASE_REF:/ {f=1} END{exit !f}' "$idxf" && return 0
  awk -F'\t' -v j="$GATES_JOB" '$3==j && $6=="run" {print $8}' "$idxf" | joined \
    | grep -Eq '(^|[ ;&(])(export|declare[ ]+-x)[ ]+BASE_REF([= ]|$)|(^|[ ;&(])BASE_REF=[^ ;]*[ ]+(bash[ ]+|sh[ ]+)?[^ ]*gates\.sh' && return 0
  why "job $GATES_JOB khong EXPORT BASE_REF cho gates.sh (env:, export, hoac tien to BASE_REF=... bash gates.sh)"; return 1; }
ac6_base_ref_pr() { need_gates || return 1
  job_text "$CI" "$GATES_JOB" | grep -Eq 'github\.event\.pull_request\.base\.(sha|ref)|github\.base_ref' \
    || { why "BASE_REF khong lay tu base cua PR (pull_request.base.sha / github.base_ref)"; return 1; }; }
ac6_base_ref_push() { need_gates || return 1
  job_text "$CI" "$GATES_JOB" | grep -Eq 'github\.event\.before' \
    || { why "BASE_REF khong lay commit truoc cua push (github.event.before)"; return 1; }; }
ac6_not_soft() { need_gates || return 1
  job_text "$CI" "$GATES_JOB" | grep -Eq '^(- )?continue-on-error:[ ]*["'"'"']?true' && { why "job $GATES_JOB co continue-on-error: true"; return 1; }
  job_text "$CI" "$GATES_JOB" | joined | grep -Eq 'gates\.sh[^;&|]*(\|\|[ ]*true|;[ ]*true)' && { why "loi cua gates.sh bi nuot (|| true)"; return 1; }
  return 0; }
check AC-6 "ci.yml: co job chay bash .github/scripts/gates.sh" ac6_job
check AC-6 "ci.yml: job gates checkout voi fetch-depth: 0" ac6_fetch_depth
check AC-6 "ci.yml: job gates dat BASE_REF" ac6_base_ref_set
check AC-6 "ci.yml: BASE_REF = base cua PR khi pull_request" ac6_base_ref_pr
check AC-6 "ci.yml: BASE_REF = commit truoc (github.event.before) khi push" ac6_base_ref_push
check AC-6 "ci.yml: loi cua gates.sh khong bi nuot (khong continue-on-error / || true)" ac6_not_soft

# ================================================================== AC-7
perm_entries() { # perm_entries <file> -> "pham-vi<TAB>scope<TAB>value"; pham-vi = top | job:<id>
  awk -F'\t' '
    function emit(where, s,   n, i, kv, a) {
      gsub(/[{} ]/, "", s); n = split(s, a, ",")
      for (i = 1; i <= n; i++) if (a[i] != "") { split(a[i], kv, ":"); print where "\t" kv[1] "\t" kv[2] }
    }
    $2=="permissions" && $3=="" && $7==0 { s=$8; sub(/^permissions:[ ]*/, "", s)
      if (s ~ /^\{/) emit("top", s); else if (s != "") print "top\t*\t" s; next }
    $2=="permissions" && $3=="" { s=$8; gsub(/ /, "", s); split(s, kv, ":"); print "top\t" kv[1] "\t" kv[2]; next }
    $4=="permissions" && $5==0 { s=$8
      if (s ~ /^permissions:/) { sub(/^permissions:[ ]*/, "", s)
        if (s ~ /^\{/) emit("job:" $3, s); else if (s != "") print "job:" $3 "\t*\t" s; next }
      gsub(/ /, "", s); split(s, kv, ":"); print "job:" $3 "\t" kv[1] "\t" kv[2] }' "$(idx "$1")"
}
has_top_perm() { awk -F'\t' '$2=="permissions" && $3=="" && $7==0 {f=1} END{exit !f}' "$(idx "$1")"; }
has_job_perm() { awk -F'\t' -v j="$2" '$3==j && $4=="permissions" && $5==0 {f=1} END{exit !f}' "$(idx "$1")"; }
ac7_declared() {
  has_top_perm "$1" && return 0
  local j miss=""
  for j in $(jobs_of "$1"); do has_job_perm "$1" "$j" || miss="$miss $j"; done
  [ -z "$miss" ] && [ -n "$(jobs_of "$1")" ] && return 0
  why "khong co permissions: top-level, va job thieu permissions:${miss:- (khong co job)}"; return 1; }
ac7_contents_read() {
  perm_entries "$1" | awk -F'\t' '$2=="contents" && $3=="read" {f=1} END{exit !f}' || { why "khong co 'contents: read'"; return 1; }; }
ac7_no_write() {
  local w; w="$(perm_entries "$1" | awk -F'\t' '$3 ~ /write/ {print $1 ":" $2 "=" $3}' | tr '\n' ' ')"
  [ -z "$w" ] || { why "co quyen ghi: $w"; return 1; }; }
ac7_cd_only_packages() {
  local all w; all="$(perm_entries "$CD")"
  printf '%s\n' "$all" | awk -F'\t' '$2=="packages" && $3=="write" {f=1} END{exit !f}' || why "cd-web.yml khong co packages: write (CD khong push duoc)"
  w="$(printf '%s\n' "$all" | awk -F'\t' '$3 ~ /write/ && $2!="packages" {print $1 ":" $2 "=" $3}' | tr '\n' ' ')"
  [ -z "$w" ] || why "cd-web.yml co quyen ghi khac ngoai packages: $w"
  [ -z "$REASON" ]; }
ac7_pr_read() {
  ftext "$1" | grep -Eq '^(- )?uses:[ ]*["'"'"']?dorny/paths-filter@' || return 0
  perm_entries "$1" | awk -F'\t' '$2=="pull-requests" && $3=="read" {f=1} END{exit !f}' \
    || { why "dung dorny/paths-filter nhung khong co pull-requests: read"; return 1; }; }
while read -r f; do
  b="$(basename "$f")"
  check AC-7 "$b: khai permissions: (top-level, hoac o moi job)" ac7_declared "$f"
  check AC-7 "$b: co contents: read" ac7_contents_read "$f"
  if [ "$b" = cd-web.yml ]; then
    check AC-7 "$b: co packages: write va KHONG co quyen ghi nao khac" ac7_cd_only_packages
  else
    check AC-7 "$b: khong co quyen ghi nao (chi cd-web.yml duoc packages: write)" ac7_no_write "$f"
  fi
  if ftext "$f" | grep -Eq '^(- )?uses:[ ]*["'"'"']?dorny/paths-filter@'; then
    check AC-7 "$b: dung dorny/paths-filter => co pull-requests: read" ac7_pr_read "$f"
  fi
done < <(wf_files)

# ================================================================== AC-8 / K5
ac8_if_always() { # ac8_if_always <file> <ten-job>
  local j v; j="$(job_by_name "$1" "$2")" || { why "khong co job $2"; return 1; }
  v="$(job_if "$1" "$j")"; v="${v#\$\{\{ }"; v="${v% \}\}}"; v="$(unq "$v")"
  [ "$v" = 'always()' ] || { why "if: cua job $2 = '$v' (phai DUNG BANG always(); dieu kien trong if: lam job bi SKIP => GitHub bao Success)"; return 1; }; }
ac8_if_no_cond() {
  local j v; j="$(job_by_name "$1" "$2")" || { why "khong co job $2"; return 1; }
  v="$(job_if "$1" "$j")"
  printf '%s' "$v" | grep -Eq 'failure|cancelled|contains' && { why "if: cua job $2 chua failure/cancelled/contains: '$v'"; return 1; }
  return 0; }
gate_step() { # gate_step <file> <jobid> -> step co exit 1 + ::error + needs.*.result
  local s t
  for s in $(steps_of "$1" "$2"); do
    t="$(step_text "$1" "$2" "$s")"
    printf '%s\n' "$t" | grep -q 'needs\.\*\.result' || continue
    printf '%s\n' "$t" | grep -q '::error' || continue
    printf '%s\n' "$t" | grep -Eq '(^|[^0-9A-Za-z])exit[ ]+1([^0-9]|$)' || continue
    printf '%s' "$s"; return 0
  done
  return 1; }
ac8_step() {
  local j s; j="$(job_by_name "$1" "$2")" || { why "khong co job $2"; return 1; }
  s="$(gate_step "$1" "$j")" || { why "job $2 khong co step nao vua doc needs.*.result, vua in ::error, vua exit 1"; return 1; }; }
ac8_step_both() {
  local j s t; j="$(job_by_name "$1" "$2")" || { why "khong co job $2"; return 1; }
  s="$(gate_step "$1" "$j")" || { why "khong co step gate"; return 1; }
  t="$(step_text "$1" "$j" "$s")"
  printf '%s\n' "$t" | grep -Eq "contains\([ ]*needs\.\*\.result[ ]*,[ ]*['\"]failure['\"][ ]*\)" || why "step gate khong xet contains(needs.*.result, 'failure')"
  printf '%s\n' "$t" | grep -Eq "contains\([ ]*needs\.\*\.result[ ]*,[ ]*['\"]cancelled['\"][ ]*\)" || why "step gate khong xet contains(needs.*.result, 'cancelled')"
  [ -z "$REASON" ]; }
ac8_step_not_skipped() {
  local j s t; j="$(job_by_name "$1" "$2")" || { why "khong co job $2"; return 1; }
  s="$(gate_step "$1" "$j")" || { why "khong co step gate"; return 1; }
  t="$(step_text "$1" "$j" "$s")"
  printf '%s\n' "$t" | grep -Eq "needs\.\*\.result[ ]*,[ ]*['\"](skipped|success)['\"]" \
    && { why "step gate xet 'skipped'/'success' => se fail ca khi moi needs la success/skipped"; return 1; }
  return 0; }
ac8_build_steps_keep_filter() {
  local j s t u bad=""; j="$(job_by_name "$CI" build)" || { why "khong co job build"; return 1; }
  for s in $(steps_of "$CI" "$j"); do
    t="$(step_text "$CI" "$j" "$s")"; u="$(step_uses "$CI" "$j" "$s")"
    local want=""
    case "$u" in actions/setup-go@*) want=go ;; actions/setup-node@*) want=web ;; astral-sh/setup-uv@*) want=python ;; esac
    printf '%s\n' "$t" | grep -Eq 'go[ ]+build' && want=go
    printf '%s\n' "$t" | grep -Eq 'npm[ ]+run[ ]+build' && want=web
    printf '%s\n' "$t" | grep -Eq 'import app\.main' && want=python
    [ -z "$want" ] && continue
    step_key_text "$CI" "$j" "$s" if | grep -Eq "outputs\.$want[ ]*==[ ]*'true'" || bad="$bad step#$s(can outputs.$want)"
  done
  [ -z "$bad" ] || { why "step cua build mat dieu kien filter:$bad"; return 1; }; }

job_needs() { # job_needs <file> <jobid> -> moi job id trong needs:, mot dong mot id
  awk -F'\t' -v j="$2" '$3==j && $4=="needs" && $5==0 {
      s=$8; sub(/^needs:[ ]*/, "", s); sub(/^- /, "", s); gsub(/[][ "\047]/, "", s)
      n=split(s, a, ","); for (i=1; i<=n; i++) if (a[i] != "") print a[i] }' "$(idx "$1")"
}
ac8_needs_all() { # needs: cua gate phai chua MOI job khac trong file
  local j o miss="" nd; j="$(job_by_name "$1" "$2")" || { why "khong co job $2"; return 1; }
  nd="$(job_needs "$1" "$j")"
  for o in $(jobs_of "$1"); do
    [ "$o" = "$j" ] && continue
    printf '%s\n' "$nd" | grep -qx -- "$o" || miss="$miss $o"
  done
  [ -z "$miss" ] || { why "needs: cua $2 thieu:$miss (job do fail thi gate van xanh)"; return 1; }; }
ac8_gate_not_soft() {
  local j s; j="$(job_by_name "$1" "$2")" || { why "khong co job $2"; return 1; }
  s="$(gate_step "$1" "$j")" || { why "khong co step gate"; return 1; }
  step_key_text "$1" "$j" "$s" continue-on-error | grep -Eqv 'continue-on-error:[ ]*["'"'"']?false' \
    && { why "step gate co continue-on-error (khong phai false) => exit 1 khong lam job fail"; return 1; }
  awk -F'\t' -v j="$j" '$3==j && $4=="continue-on-error" && $5==0 && $8 !~ /false/ {f=1} END{exit !f}' "$(idx "$1")" \
    && { why "job $2 co continue-on-error"; return 1; }
  return 0; }
ac8_gate_step_if() { # if: cua step gate: khong co, hoac DUNG BANG always()
  local j s v; j="$(job_by_name "$1" "$2")" || { why "khong co job $2"; return 1; }
  s="$(gate_step "$1" "$j")" || { why "khong co step gate"; return 1; }
  v="$(step_key_text "$1" "$j" "$s" if | sed 's/^- //;s/^if:[ ]*//' | tr '\n' ' ' | sed 's/[[:space:]]*$//')"
  [ -z "$v" ] && return 0
  v="$(unq "$v")"; v="${v#\$\{\{}"; v="${v%\}\}}"; v="$(unq "$v")"
  [ "$v" = 'always()' ] || { why "if: cua step gate = '$v' (chi duoc bo trong hoac always())"; return 1; }; }
ac8_gate_literal() { # so sanh ket qua bieu thuc needs.*.result voi dung literal 'true'
  local j s; j="$(job_by_name "$1" "$2")" || { why "khong co job $2"; return 1; }
  s="$(gate_step "$1" "$j")" || { why "khong co step gate"; return 1; }
  local vars v run cmp bad="" n=0
  vars="$( { step_key_text "$1" "$j" "$s" env; awk -F'\t' -v j="$j" '$3==j && $4=="env" && $5==0 {print $8}' "$(idx "$1")"; } \
           | grep 'needs\.\*\.result' | sed -n 's/^\([A-Za-z_][A-Za-z0-9_]*\):.*/\1/p')"
  run="$(step_key_text "$1" "$j" "$s" run | joined | sed 's/\${{[^}]*needs\.\*\.result[^}]*}}/$__GATE__/g')"
  for v in $vars __GATE__; do
    while read -r cmp; do
      [ -z "$cmp" ] && continue
      n=$((n + 1))
      local op lit
      op="$(printf '%s' "$cmp" | grep -Eo '(==|!=|=)' | head -1)"
      lit="$(printf '%s' "$cmp" | sed -E 's/.*(==|!=|=)[ ]*//; s/["'"'"']//g')"
      case "$op:$lit" in "=:true"|"==:true"|"!=:false") ;; *) bad="$bad [$cmp]" ;; esac
    done < <(printf '%s\n' "$run" | grep -Eo '["'"'"']?\$\{?'"$v"'\}?["'"'"']?[ ]*(==|!=|=)[ ]*["'"'"']?[A-Za-z0-9_]*["'"'"']?')
  done
  [ "$n" -gt 0 ] || { why "step gate khong so sanh ket qua bieu thuc needs.*.result voi literal nao"; return 1; }
  [ -z "$bad" ] || { why "so sanh sai literal (bieu thuc GitHub tra 'true'/'false' chu thuong):$bad"; return 1; }; }
for pair in "$TRIVY|trivy-gate" "$CI|build"; do
  f="${pair%%|*}"; jn="${pair#*|}"; b="$(basename "$f")"
  check AC-8 "$b/$jn: if: cua job DUNG BANG always() (K5)" ac8_if_always "$f" "$jn"
  check AC-8 "$b/$jn: if: cua job KHONG chua failure/cancelled/contains (K5)" ac8_if_no_cond "$f" "$jn"
  check AC-8 "$b/$jn: co step doc needs.*.result, in ::error va exit 1 (K5)" ac8_step "$f" "$jn"
  check AC-8 "$b/$jn: step gate xet ca 'failure' lan 'cancelled'" ac8_step_both "$f" "$jn"
  check AC-8 "$b/$jn: step gate khong fail khi needs chi la success/skipped" ac8_step_not_skipped "$f" "$jn"
  check AC-8 "$b/$jn: needs: chua MOI job khac trong file" ac8_needs_all "$f" "$jn"
  check AC-8 "$b/$jn: step gate / job khong co continue-on-error" ac8_gate_not_soft "$f" "$jn"
  check AC-8 "$b/$jn: if: cua step gate bo trong hoac DUNG BANG always()" ac8_gate_step_if "$f" "$jn"
  check AC-8 "$b/$jn: step gate so ket qua bieu thuc voi dung literal 'true' (hoac != 'false')" ac8_gate_literal "$f" "$jn"
done
check AC-8 "ci.yml/build: cac step build giu dieu kien theo filter go/web/python (K5)" ac8_build_steps_keep_filter

# ================================================================== AC-9
node_env_val() { # gia tri NODE_VERSION khai trong file (dong dau tien)
  local v; v="$(ftext "$1" | sed -n 's/^NODE_VERSION:[ ]*//p' | head -1)"; unq "$v"; }
is22() { printf '%s' "$1" | grep -Eq '^22(\.x|\.[0-9]+(\.[0-9]+)?)?$'; }
ac9_env() {
  local f v bad=""
  while read -r f; do
    while read -r v; do [ -z "$v" ] && continue; v="$(unq "$v")"; is22 "$v" || bad="$bad $(basename "$f"):NODE_VERSION=$v"; done \
      < <(ftext "$f" | sed -n 's/^NODE_VERSION:[ ]*//p')
  done < <(wf_files)
  [ -z "$bad" ] || { why "$bad"; return 1; }; }
ac9_setup_node() { # moi actions/setup-node trong file co node-version ra 22
  local f="$1" j s v bad="" n=0
  for j in $(jobs_of "$f"); do
    for s in $(steps_using "$f" "$j" '^actions/setup-node@'); do
      n=$((n + 1)); v="$(step_with "$f" "$j" "$s" node-version)"
      case "$v" in
        *env.NODE_VERSION*) v="$(node_env_val "$f")" ;;
        '') local nf; nf="$(step_with "$f" "$j" "$s" node-version-file)"
            [ -n "$nf" ] && [ -f "$nf" ] && v="$(tr -d ' \r\nv' <"$nf")" ;;
      esac
      is22 "$v" || bad="$bad $j/step#$s:'$v'"
    done
  done
  [ -z "$bad" ] || { why "node-version khong phai 22:$bad"; return 1; }; }
ac9_no_20() {
  local f bad=""
  while read -r f; do
    ftext "$f" | grep -Eq '^(- )?node-version:[ ]*["'"'"']?20([.x0-9]*)["'"'"']?$|node:20([^0-9]|$)' && bad="$bad $(basename "$f")"
  done < <(wf_files)
  [ -z "$bad" ] || { why "con Node 20 o:$bad"; return 1; }; }
check AC-9 "moi NODE_VERSION khai trong workflow = 22" ac9_env
check AC-9 "khong con node-version 20 / node:20 o workflow nao" ac9_no_20
while read -r f; do
  ftext "$f" | grep -Eq '^(- )?uses:[ ]*["'"'"']?actions/setup-node@' || continue
  check AC-9 "$(basename "$f"): moi actions/setup-node co node-version = 22" ac9_setup_node "$f"
done < <(wf_files)

# ================================================================== AC-12
# Dockerfile Go copy go.work (workspace mode) => phai copy go.mod (hoac ca thu muc) cua MOI
# module trong `use (...)` cua go.work, neu khong: "cannot load module ../x listed in go.work".
# Danh sach module LAY TU go.work, khong viet cung.
GOWORK_FILE="$ROOT/go.work"
gowork_modules() {
  awk '
    { sub(/\r$/, ""); sub(/\/\/.*/, "") }
    /^[ \t]*use[ \t]*\(/ { inb = 1; next }
    inb && /^[ \t]*\)/ { inb = 0; next }
    inb { s = $0; gsub(/[ \t"]/, "", s); if (s != "") print s; next }
    /^[ \t]*use[ \t]+[^( \t]/ { s = $0; sub(/^[ \t]*use[ \t]+/, "", s); gsub(/[ \t"]/, "", s); print s }' "$GOWORK_FILE" \
    | sed 's#^\./##; s#/$##'
}
# In "nguon<TAB>dich" cho moi nguon cua moi lenh COPY (bo COPY --from=..., bo co --xxx),
# chi o stage dau tien (stage build). Dich da chuan hoa: bo ./ dau, '.' => rong.
docker_copies() {
  awk '
    { sub(/\r$/, "") }
    /^[ \t]*#/ { next }
    { if (cont) { line = line " " $0 } else { line = $0 }
      if (sub(/\\[ \t]*$/, "", line)) { cont = 1; next } cont = 0 }
    toupper(line) ~ /^[ \t]*FROM[ \t]/ { nfrom++ }
    nfrom == 1 && toupper(line) ~ /^[ \t]*COPY[ \t]/ {
      n = split(line, t, /[ \t]+/); k = 0
      for (i = 1; i <= n; i++) { if (t[i] == "" || toupper(t[i]) == "COPY") continue
        if (t[i] ~ /^--from/) { k = -1; break }
        if (t[i] ~ /^--/) continue
        a[++k] = t[i] }
      if (k >= 2) { d = a[k]; multi = (k > 2)
        for (i = 1; i < k; i++) print a[i] "\t" d "\t" multi }
    }' "$1"
}
norm_path() { local p="$1"; p="${p#./}"; p="${p%/}"; [ "$p" = . ] && p=""; printf '%s' "$p"; }
copied_to() { # copied_to <nguon> <dich> <multi> <duong-dan-can> -> 0 neu duong dan can nam dung cho trong image
  local src dst want="$4"; src="$(norm_path "$1")"; dst="$(norm_path "$2")"
  case "$2" in */) local dir_dest=1 ;; *) local dir_dest=$3 ;; esac
  # nguon la chinh file can
  if [ "$src" = "$want" ]; then
    if [ "${dir_dest:-0}" = 1 ] || [ -z "$dst" ]; then [ "${dst:+$dst/}$(basename "$want")" = "$want" ]; return; fi
    [ "$dst" = "$want" ]; return
  fi
  # nguon la thu muc to tien (hoac '.'): noi dung thu muc duoc dat vao dich
  if [ -z "$src" ] || [ "${want#"$src"/}" != "$want" ]; then
    local rel="${want#"$src"/}"; [ -z "$src" ] && rel="$want"
    [ "${dst:+$dst/}$rel" = "$want" ]; return
  fi
  return 1
}
dockerfile_has() { # dockerfile_has <Dockerfile> <duong-dan>
  local s d m
  while IFS="$(printf '\t')" read -r s d m; do
    copied_to "$s" "$d" "$m" "$2" && return 0
  done < <(docker_copies "$1")
  return 1
}
ac12_gowork_parsed() {
  [ -f "$GOWORK_FILE" ] || { why "khong co $GOWORK_FILE"; return 1; }
  [ -n "$(gowork_modules)" ] || { why "khong doc duoc module nao trong use (...) cua go.work"; return 1; }; }
ac12_dockerfile() { # ac12_dockerfile <Dockerfile>
  [ -f "$1" ] || { why "khong co $1"; return 1; }
  [ -f "$GOWORK_FILE" ] || { why "khong co go.work"; return 1; }
  local m miss=""
  dockerfile_has "$1" go.work || miss="$miss go.work"
  for m in $(gowork_modules); do dockerfile_has "$1" "$m/go.mod" || miss="$miss $m/go.mod"; done
  [ -z "$miss" ] || { why "stage build khong COPY dung cho:$miss"; return 1; }; }
check AC-12 "go.work co khoi use (...) doc duoc" ac12_gowork_parsed
for df in ticketing waitingroom; do
  check AC-12 "deploy/docker/$df.Dockerfile: COPY go.work + go.mod (hoac ca thu muc) cua MOI module trong go.work" ac12_dockerfile "$ROOT/deploy/docker/$df.Dockerfile"
done

# ================================================================== AC-11
ac11_job_name() { job_by_name "$1" "$2" >/dev/null || { why "khong con job co name: '$2' trong $(basename "$1")"; return 1; }; }
for spec in \
  "ci.yml|detect-changes" "ci.yml|lint-go" "ci.yml|lint-web" "ci.yml|lint-python" \
  "ci.yml|test-go" "ci.yml|test-web" "ci.yml|test-python" "ci.yml|build" \
  "trivy.yml|detect-changes" "trivy.yml|trivy" "trivy.yml|trivy-gate" \
  "cd-web.yml|build & push ghcr web" "integration.yml|integration" \
  "oversell-gate.yml|oversell-gate" "e2e-nightly.yml|e2e"; do
  check AC-11 "K3: ${spec%%|*} con job '${spec#*|}'" ac11_job_name "$WF/${spec%%|*}" "${spec#*|}"
done
ac11_cd_cond() { need_cdjob || return 1
  job_if "$CD" "$CDJOB" | grep -Eq -- "$1" || { why "if: cua job CD thieu: $1"; return 1; }; }
check AC-11 "cd-web: if: con github.event.workflow_run.event == 'push'" ac11_cd_cond "github\.event\.workflow_run\.event[ ]*==[ ]*'push'"
check AC-11 "cd-web: if: con workflow_run.head_repository.full_name == github.repository" ac11_cd_cond "github\.event\.workflow_run\.head_repository\.full_name[ ]*==[ ]*github\.repository"
check AC-11 "cd-web: if: con github.ref == 'refs/heads/main' (workflow_dispatch)" ac11_cd_cond "github\.ref[ ]*==[ ]*'refs/heads/main'"
check AC-11 "cd-web: if: con workflow_run.conclusion == 'success' (chi chay sau CI xanh)" ac11_cd_cond "github\.event\.workflow_run\.conclusion[ ]*==[ ]*'success'"
ac11_cd_trigger() {
  local t; t="$(awk -F'\t' '$2=="on" {print $8}' "$(idx "$CD")")"
  printf '%s\n' "$t" | grep -Eq '^workflows:[ ]*\[[ ]*["'"'"']?CI["'"'"']?[ ]*\]' || why "workflow_run khong con workflows: [\"CI\"]"
  printf '%s\n' "$t" | grep -Eq '^branches:[ ]*\[[ ]*["'"'"']?main["'"'"']?[ ]*\]' || why "workflow_run khong con branches: [main]"
  [ "$(ftext "$CI" | sed -n 's/^name:[ ]*//p' | head -1 | { read -r v; unq "${v:-}"; })" = CI ] || why "ci.yml khong con name: CI (cd-web.yml phu thuoc ten nay)"
  [ -z "$REASON" ]; }
check AC-11 "cd-web: van chi kich hoat sau workflow 'CI' tren main" ac11_cd_trigger
ac11_no_deploy() {
  local f bad=""
  while read -r f; do
    while read -r l; do
      printf '%s' "$l" | grep -q -- '--dry-run' && continue
      bad="$bad
$(basename "$f"): $l"
    done < <(ftext "$f" | joined | grep -E '(^|[^A-Za-z0-9_-])(kubectl[ ]+([^|;&]*[ ])?(apply|create|replace|patch|set|rollout)|helm[ ]+(install|upgrade))([ ]|$)')
  done < <(wf_files)
  [ -z "$bad" ] || { why "lenh deploy that (khong --dry-run):$bad"; return 1; }; }
ac11_push_only_cd() {
  local f bad=""
  while read -r f; do
    [ "$(basename "$f")" = cd-web.yml ] && continue
    ftext "$f" | joined | grep -Eq '(^|[^A-Za-z0-9_-])docker[ ]+(image[ ]+)?push([ ]|$)|^(- )?push:[ ]*["'"'"']?true' && bad="$bad $(basename "$f")"
  done < <(wf_files)
  [ -z "$bad" ] || { why "publish image ngoai cd-web.yml:$bad"; return 1; }; }
check AC-11 "khong workflow nao chay kubectl apply/create/... hay helm install/upgrade that (khong --dry-run)" ac11_no_deploy
check AC-11 "chi cd-web.yml duoc push image" ac11_push_only_cd

# ------------------------------------------------------------------ ket qua
echo
echo "== Ket qua: $PASSED PASS, $FAILED FAIL =="
if [ "$FAILED" -gt 0 ]; then
  printf '   AC co ca fail: %s\n' "$(printf '%s\n' "${FAILED_IDS[@]}" | sort -u -t- -k2n | tr '\n' ' ')"
  exit 1
fi
exit 0
