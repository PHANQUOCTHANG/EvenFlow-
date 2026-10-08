#!/usr/bin/env bash
#
# Test hanh vi cho hop dong K1 (docs/workflow/ci-cd-review/contracts.md) -- AC-4.
#
#   bash .github/scripts/tests/test-run-and-report-tests.sh               # chay tu GOC repo
#   bash .github/scripts/tests/test-run-and-report-tests.sh <repo-root>   # kiem ban sao khac
#
# Script duoc kiem: <repo-root>/.github/scripts/run-and-report-tests.sh
#   bash run-and-report-tests.sh <test-regex> <cmd> [args...]
# Goc repo: tham so 1, hoac bien REPO_ROOT, mac dinh thu muc hien tai.
#
# Khong chay `go test` that: <cmd> la mot script gia in ra cac dong dinh dang
# `go test -v` lay tu file fixture roi `exit N`.
#
# Bien moi truong (chi de tu kiem chinh bo test nay, KHONG dung trong CI):
#   SUT=<duong dan>   thay script duoc kiem (vd mot ban cai dat mau / ban dot bien)
#
# Moi ca in PASS/FAIL. Exit != 0 neu co it nhat mot ca FAIL.
# Chi dung bash + coreutils + grep + awk. Moi file tam nam trong mot thu muc mktemp
# va bi xoa bang trap.

set -u

ROOT="${1:-${REPO_ROOT:-.}}"
if [ ! -d "$ROOT/.github" ]; then
  echo "Khong thay $ROOT/.github. Hay chay tu goc repo: bash .github/scripts/tests/test-run-and-report-tests.sh [repo-root]" >&2
  exit 2
fi
ROOT="$(cd "$ROOT" && pwd)"
SUT="${SUT:-$ROOT/.github/scripts/run-and-report-tests.sh}"
case "$SUT" in /*|[A-Za-z]:*) ;; *) SUT="$ROOT/$SUT" ;; esac

T="$(mktemp -d "${TMPDIR:-/tmp}/rrt-test.XXXXXX")" || exit 2
trap 'rm -rf "$T"' EXIT INT TERM
mkdir -p "$T/fx" "$T/work" "$T/suttmp"

TIMEOUT=""
if command -v timeout >/dev/null 2>&1; then TIMEOUT="timeout 60"; fi

PASSED=0
FAILED=0
FAILED_NAMES=()

# Khong co SUT thi KHONG ca nao duoc PASS -- ke ca cac ca "khong co X" (khong canh
# bao, khong tao file) von se dung mot cach vo nghia khi khong co gi chay.
ok()  {
  if [ ! -f "$SUT" ]; then bad "$1" "SUT khong ton tai: $SUT"; return; fi
  PASSED=$((PASSED + 1)); printf 'PASS  %s\n' "$1"
}
bad() {
  FAILED=$((FAILED + 1)); FAILED_NAMES+=("$1")
  printf 'FAIL  %s\n' "$1"
  shift
  local m; for m in "$@"; do printf '        %s\n' "$m"; done
}

# ---------------------------------------------------------------- lenh gia
# fake.sh <file-stdout> <exit-code> [file-stderr]
cat >"$T/fake.sh" <<'EOF'
#!/usr/bin/env bash
cat "$1"
if [ -n "${3:-}" ]; then cat "$3" >&2; fi
exit "$2"
EOF

# args.sh: in tung tham so nhan duoc, de kiem tham so co khoang trang khong bi tach
cat >"$T/args.sh" <<'EOF'
#!/usr/bin/env bash
printf 'NARGS=%s\n' "$#"
i=0
for a in "$@"; do i=$((i + 1)); printf 'ARG%s=<%s>\n' "$i" "$a"; done
echo "--- PASS: TestNoOversell_Args (0.00s)"
exit 0
EOF

fx() { # fx <ten> <<'EOF' ... EOF   -- tao fixture tu stdin
  cat >"$T/fx/$1"
  printf '%s' "$T/fx/$1"
}

# ---------------------------------------------------------------- chay SUT
# run_sut <file-summary-hoac-rong> <args...>
#   Dat OUT (stdout), ERR (stderr), RC. Luon chay trong thu muc rong $T/work,
#   voi TMPDIR rieng, va GITHUB_STEP_SUMMARY chi duoc dat khi tham so 1 khac rong
#   (khi chinh bo test nay chay tren GitHub Actions, summary that KHONG bi dong vao).
run_sut() {
  local sf="$1"; shift
  (
    cd "$T/work" || exit 99
    unset GITHUB_STEP_SUMMARY
    if [ -n "$sf" ]; then export GITHUB_STEP_SUMMARY="$sf"; fi
    export TMPDIR="${SUT_TMPDIR:-$T/suttmp}"
    # shellcheck disable=SC2086
    $TIMEOUT bash "$SUT" "$@" </dev/null >"$T/out" 2>"$T/err"
  )
  RC=$?
  OUT="$(cat "$T/out")"
  ERR="$(cat "$T/err")"
}

count_warning_lines() { # so dong BAT DAU bang ::warning trong $1
  printf '%s\n' "$1" | grep -c '^::warning' || true
}

# summary_value <nhan> <file>
#   Lay so nguyen gan voi nhan (pass|fail|skip|exit) trong bang markdown cua
#   summary. Hop dong chi noi "bang markdown co ba so dem va exit code", khong chot
#   bo cuc, nen chap nhan ca hai bo cuc:
#     ngang:  | PASS | FAIL | SKIP | exit |      doc:  | PASS | 2 |
#             |------|------|------|------|            | FAIL | 1 |
#             |  2   |  1   |  1   |  1   |
#   In ra rong neu khong tim duoc.
summary_value() {
  LC_ALL=C awk -v lab="$1" '
    function trim(s) { sub(/^[ \t]+/, "", s); sub(/[ \t]+$/, "", s); return s }
    function firstint(s) { if (match(s, /[0-9]+/)) return substr(s, RSTART, RLENGTH); return "" }
    function nlabels(s,   n) { n = 0
      if (index(s, "pass")) n++; if (index(s, "fail")) n++
      if (index(s, "skip")) n++; if (index(s, "exit")) n++; return n }
    { line[NR] = tolower($0) }
    END {
      # 1) bo cuc ngang: mot dong tieu de co du pass, fail, skip o cac o khac nhau
      for (i = 1; i <= NR; i++) {
        l = line[i]
        if (index(l, "|") == 0) continue
        if (!(index(l, "pass") && index(l, "fail") && index(l, "skip"))) continue
        nc = split(l, hc, "|"); col = 0; multi = 0
        for (c = 1; c <= nc; c++) {
          if (nlabels(hc[c]) > 1) multi = 1
          if (index(hc[c], lab)) col = c
        }
        if (multi || col == 0) continue
        for (j = i + 1; j <= NR; j++) {
          d = line[j]
          if (index(d, "|") == 0) break
          if (d ~ /^[ \t|:-]+$/) continue
          split(d, dc, "|"); v = firstint(dc[col])
          if (v != "") { print v; exit }
          break
        }
      }
      # 2) bo cuc doc: dong chi mang MOT nhan, so nam sau nhan
      for (i = 1; i <= NR; i++) {
        l = line[i]
        if (nlabels(l) != 1 || index(l, lab) == 0) continue
        if (index(l, "|")) {
          nc = split(l, cc, "|")
          for (c = 1; c <= nc; c++) if (index(cc[c], lab)) {
            for (k = c + 1; k <= nc; k++) { v = firstint(cc[k]); if (v != "") { print v; exit } }
          }
        } else {
          rest = substr(l, index(l, lab) + length(lab)); v = firstint(rest)
          if (v != "") { print v; exit }
        }
      }
    }' "$2"
}

# expect_counts <ten-ca> <file-summary> <pass> <fail> <skip> [exit]
expect_counts() {
  local name="$1" f="$2" ep="$3" ef="$4" es="$5" ee="${6:-}"
  local gp gf gs ge msgs=()
  gp="$(summary_value pass "$f")"; gf="$(summary_value fail "$f")"; gs="$(summary_value skip "$f")"
  [ "$gp" = "$ep" ] || msgs+=("PASS: mong doi $ep, doc duoc '${gp}'")
  [ "$gf" = "$ef" ] || msgs+=("FAIL: mong doi $ef, doc duoc '${gf}'")
  [ "$gs" = "$es" ] || msgs+=("SKIP: mong doi $es, doc duoc '${gs}'")
  if [ -n "$ee" ]; then
    ge="$(summary_value exit "$f")"
    [ "$ge" = "$ee" ] || msgs+=("exit code: mong doi $ee, doc duoc '${ge}'")
  fi
  if [ "${#msgs[@]}" -eq 0 ]; then ok "$name"
  else bad "$name" "${msgs[@]}" "summary:" "$(sed 's/^/  | /' "$f" 2>/dev/null | head -20)"; fi
}

echo "== K1: run-and-report-tests.sh =="
echo "   SUT: $SUT"
if [ ! -f "$SUT" ]; then
  echo "   (SUT chua ton tai -- moi ca duoi day se FAIL)"
fi
echo

# ---------------------------------------------------------------- fixture
F_SKIP="$(fx all-skip <<'EOF'
=== RUN   TestNoOversell_UnderExtremeConcurrency
    helpers.go:33: TODO(EVF-39): can hien thuc testcontainers truoc khi bat test nay
--- SKIP: TestNoOversell_UnderExtremeConcurrency (0.00s)
=== RUN   TestNoOversell_Basic
    helpers.go:33: TODO(EVF-39): can hien thuc testcontainers truoc khi bat test nay
--- SKIP: TestNoOversell_Basic (0.00s)
=== RUN   TestNoOversell_Edge
--- SKIP: TestNoOversell_Edge (0.00s)
PASS
ok  	github.com/eventflow/ticketing/test/concurrency	0.012s
EOF
)"

F_NONE="$(fx no-match <<'EOF'
testing: warning: no tests to run
PASS
ok  	github.com/eventflow/ticketing/test/concurrency	0.005s [no tests to run]
EOF
)"

F_PASS="$(fx has-pass <<'EOF'
=== RUN   TestNoOversell_Basic
--- PASS: TestNoOversell_Basic (1.20s)
=== RUN   TestNoOversell_UnderExtremeConcurrency
--- SKIP: TestNoOversell_UnderExtremeConcurrency (0.00s)
PASS
ok  	github.com/eventflow/ticketing/test/concurrency	1.300s
EOF
)"

F_FAIL="$(fx has-fail <<'EOF'
=== RUN   TestNoOversell_Basic
--- PASS: TestNoOversell_Basic (1.20s)
=== RUN   TestNoOversell_Edge
--- FAIL: TestNoOversell_Edge (0.30s)
FAIL
EOF
)"

# Dung y nguyen vi du cua K1: SKIP = 1, PASS = 2, FAIL = 1
F_K1="$(fx k1-example <<'EOF'
=== RUN   TestNoOversell_UnderExtremeConcurrency
    helpers.go:33: TODO(EVF-39): can hien thuc testcontainers truoc khi bat test nay
--- SKIP: TestNoOversell_UnderExtremeConcurrency (0.00s)
--- PASS: TestNoOversell_Basic (1.20s)
    --- PASS: TestNoOversell_Basic/sub (0.10s)
--- FAIL: TestNoOversell_Edge (0.30s)
EOF
)"

# Chi co PASS nam o dong subtest thut le (space va tab) -> PASS phai > 0
F_SUB="$(fx sub-only <<EOF
=== RUN   TestNoOversell_Parent
    --- PASS: TestNoOversell_Parent/a (0.10s)
$(printf '\t')--- PASS: TestNoOversell_Parent/b (0.10s)
--- SKIP: TestNoOversell_Other (0.00s)
EOF
)"

F_OTHER_PASS="$(fx other-pass <<'EOF'
--- PASS: TestOther (0.01s)
--- PASS: TestOther/sub (0.01s)
--- SKIP: TestNoOversell_Basic (0.00s)
PASS
EOF
)"

F_OTHER_FAIL="$(fx other-fail <<'EOF'
--- FAIL: TestOther (0.01s)
--- SKIP: TestNoOversell_Basic (0.00s)
EOF
)"

F_PREFIX="$(fx prefix <<'EOF'
--- PASS: MyTestNoOversell (0.01s)
--- PASS: XTestNoOversell_Basic (0.01s)
--- SKIP: TestNoOversell_Basic (0.00s)
EOF
)"

F_NOTANCHORED="$(fx not-anchored <<'EOF'
log: --- PASS: TestNoOversell_Basic (0.01s)
helpers.go:10: expected "--- PASS: TestNoOversell_Basic"
--- SKIP: TestNoOversell_Basic (0.00s)
EOF
)"

# Bo dem phan biet duoc: PASS = 3, FAIL = 2, SKIP = 5
F_DISTINCT="$(fx distinct <<'EOF'
--- PASS: TestNoOversell_A (0.01s)
--- PASS: TestNoOversell_B (0.01s)
    --- PASS: TestNoOversell_B/x (0.01s)
--- FAIL: TestNoOversell_C (0.01s)
    --- FAIL: TestNoOversell_C/y (0.01s)
--- SKIP: TestNoOversell_D (0.00s)
--- SKIP: TestNoOversell_E (0.00s)
--- SKIP: TestNoOversell_F (0.00s)
--- SKIP: TestNoOversell_G (0.00s)
--- SKIP: TestNoOversell_H (0.00s)
--- PASS: TestOther (0.01s)
--- FAIL: TestOther2 (0.01s)
--- SKIP: TestOther3 (0.00s)
FAIL
EOF
)"

F_MARK_OUT="$(fx marker-out <<'EOF'
MARKER-STDOUT-7f3a9c
--- PASS: TestNoOversell_Basic (0.01s)
EOF
)"
F_MARK_ERR="$(fx marker-err <<'EOF'
MARKER-STDERR-b41e02
EOF
)"
F_EMPTY="$(fx empty </dev/null)"
F_PASS_ON_STDERR="$(fx pass-on-stderr <<'EOF'
--- PASS: TestNoOversell_Basic (0.01s)
EOF
)"

R=TestNoOversell

# ---------------------------------------------------------------- tham so
run_sut ""
if [ "$RC" -eq 2 ] && [ -n "$ERR" ]; then ok "T01 thieu ca <test-regex> va <cmd> => exit 2, in cach dung ra stderr"
else bad "T01 thieu ca <test-regex> va <cmd> => exit 2, in cach dung ra stderr" "exit=$RC" "stderr=$(printf '%s' "$ERR" | head -3)"; fi

run_sut "" "$R"
if [ "$RC" -eq 2 ] && [ -n "$ERR" ]; then ok "T02 co <test-regex> nhung thieu <cmd> => exit 2, in cach dung ra stderr"
else bad "T02 co <test-regex> nhung thieu <cmd> => exit 2, in cach dung ra stderr" "exit=$RC" "stderr=$(printf '%s' "$ERR" | head -3)"; fi

# ---------------------------------------------------------------- toan SKIP
run_sut "" "$R" bash "$T/fake.sh" "$F_SKIP" 0
if [ "$RC" -eq 0 ]; then ok "T03 toan SKIP + lenh exit 0 => script exit 0"
else bad "T03 toan SKIP + lenh exit 0 => script exit 0" "exit=$RC"; fi

n="$(count_warning_lines "$OUT")"
wl="$(printf '%s\n' "$OUT" | grep '^::warning' || true)"
if [ "$n" -eq 1 ] && printf '%s' "$wl" | grep -q 'KHONG kiem gi'; then
  ok "T04 toan SKIP + exit 0 => stdout co DUNG MOT dong '^::warning' chua 'KHONG kiem gi'"
else
  bad "T04 toan SKIP + exit 0 => stdout co DUNG MOT dong '^::warning' chua 'KHONG kiem gi'" "so dong ::warning=$n" "dong: $wl"
fi

# ---------------------------------------------------------------- 0/0/0
run_sut "" "$R" bash "$T/fake.sh" "$F_NONE" 0
n="$(count_warning_lines "$OUT")"
wl="$(printf '%s\n' "$OUT" | grep '^::warning' || true)"
if [ "$RC" -eq 0 ] && [ "$n" -eq 1 ] && printf '%s' "$wl" | grep -q 'KHONG kiem gi'; then
  ok "T05 regex khong khop test nao (0/0/0) + exit 0 => exit 0 va van DUNG MOT ::warning 'KHONG kiem gi'"
else
  bad "T05 regex khong khop test nao (0/0/0) + exit 0 => exit 0 va van DUNG MOT ::warning 'KHONG kiem gi'" "exit=$RC so dong ::warning=$n" "dong: $wl"
fi

run_sut "" "$R" bash "$T/fake.sh" "$F_EMPTY" 0
n="$(count_warning_lines "$OUT")"
if [ "$RC" -eq 0 ] && [ "$n" -eq 1 ]; then ok "T06 lenh khong in gi + exit 0 => exit 0 va DUNG MOT ::warning"
else bad "T06 lenh khong in gi + exit 0 => exit 0 va DUNG MOT ::warning" "exit=$RC so dong ::warning=$n"; fi

# ---------------------------------------------------------------- co PASS
run_sut "" "$R" bash "$T/fake.sh" "$F_PASS" 0
if [ "$RC" -eq 0 ]; then ok "T07 co PASS + exit 0 => script exit 0"
else bad "T07 co PASS + exit 0 => script exit 0" "exit=$RC"; fi
if ! printf '%s\n%s\n' "$OUT" "$ERR" | grep -q '::warning'; then ok "T08 co PASS => KHONG co dong ::warning nao (stdout lan stderr)"
else bad "T08 co PASS => KHONG co dong ::warning nao (stdout lan stderr)" "$(printf '%s\n%s\n' "$OUT" "$ERR" | grep '::warning')"; fi

# ---------------------------------------------------------------- FAIL / exit code
run_sut "" "$R" bash "$T/fake.sh" "$F_FAIL" 0
if [ "$RC" -eq 1 ]; then ok "T09 co FAIL nhung lenh exit 0 => script exit 1"
else bad "T09 co FAIL nhung lenh exit 0 => script exit 1" "exit=$RC"; fi

run_sut "" "$R" bash "$T/fake.sh" "$F_EMPTY" 3
if [ "$RC" -eq 3 ]; then ok "T10 lenh exit 3 => script exit DUNG 3"
else bad "T10 lenh exit 3 => script exit DUNG 3" "exit=$RC"; fi

run_sut "" "$R" bash "$T/fake.sh" "$F_PASS" 42
if [ "$RC" -eq 42 ]; then ok "T11 lenh in PASS roi exit 42 => script exit DUNG 42 (exit code khong bi pipe/tee nuot)"
else bad "T11 lenh in PASS roi exit 42 => script exit DUNG 42 (exit code khong bi pipe/tee nuot)" "exit=$RC"; fi

run_sut "" "$R" bash "$T/fake.sh" "$F_FAIL" 1
if [ "$RC" -eq 1 ]; then ok "T12 co FAIL + lenh exit 1 (go test that) => script exit 1"
else bad "T12 co FAIL + lenh exit 1 (go test that) => script exit 1" "exit=$RC"; fi

run_sut "" "$R" khong-co-lenh-nay-evf-xyz
if [ "$RC" -eq 127 ]; then ok "T13 <cmd> khong ton tai (shell tra 127) => script exit 127, khong bi doi"
else bad "T13 <cmd> khong ton tai (shell tra 127) => script exit 127, khong bi doi" "exit=$RC"; fi

# ---------------------------------------------------------------- dem
SF="$T/summary-k1.md"; : >"$SF"
run_sut "$SF" "$R" bash "$T/fake.sh" "$F_K1" 1
expect_counts "T14 vi du K1 (co dong subtest thut le) => SKIP=1 PASS=2 FAIL=1, exit code 1 trong summary" "$SF" 2 1 1 1

run_sut "" "$R" bash "$T/fake.sh" "$F_SUB" 0
if [ "$RC" -eq 0 ] && [ "$(count_warning_lines "$OUT")" -eq 0 ]; then
  ok "T15 PASS chi nam o dong subtest thut le (space va tab) van duoc dem => khong canh bao"
else
  bad "T15 PASS chi nam o dong subtest thut le (space va tab) van duoc dem => khong canh bao" "exit=$RC so dong ::warning=$(count_warning_lines "$OUT")"
fi

run_sut "" "$R" bash "$T/fake.sh" "$F_OTHER_PASS" 0
if [ "$RC" -eq 0 ] && [ "$(count_warning_lines "$OUT")" -eq 1 ]; then
  ok "T16 '--- PASS: TestOther' KHONG duoc tinh cho regex TestNoOversell => van canh bao"
else
  bad "T16 '--- PASS: TestOther' KHONG duoc tinh cho regex TestNoOversell => van canh bao" "exit=$RC so dong ::warning=$(count_warning_lines "$OUT")"
fi

run_sut "" "$R" bash "$T/fake.sh" "$F_OTHER_FAIL" 0
if [ "$RC" -eq 0 ]; then ok "T17 '--- FAIL: TestOther' + exit 0 => KHONG tinh la FAIL cua regex => exit 0"
else bad "T17 '--- FAIL: TestOther' + exit 0 => KHONG tinh la FAIL cua regex => exit 0" "exit=$RC"; fi

run_sut "" "$R" bash "$T/fake.sh" "$F_PREFIX" 0
if [ "$(count_warning_lines "$OUT")" -eq 1 ]; then ok "T18 '--- PASS: MyTestNoOversell' khong khop (regex dat ngay sau ': ') => van canh bao"
else bad "T18 '--- PASS: MyTestNoOversell' khong khop (regex dat ngay sau ': ') => van canh bao" "so dong ::warning=$(count_warning_lines "$OUT")"; fi

run_sut "" "$R" bash "$T/fake.sh" "$F_NOTANCHORED" 0
if [ "$(count_warning_lines "$OUT")" -eq 1 ]; then ok "T19 '--- PASS:' giua dong (truoc no khong chi la khoang trang) khong duoc dem => van canh bao"
else bad "T19 '--- PASS:' giua dong (truoc no khong chi la khoang trang) khong duoc dem => van canh bao" "so dong ::warning=$(count_warning_lines "$OUT")"; fi

: >"$T/stdout-pass.txt"
run_sut "" "$R" bash "$T/fake.sh" "$T/stdout-pass.txt" 0 "$F_PASS_ON_STDERR"
if [ "$RC" -eq 0 ] && [ "$(count_warning_lines "$OUT")" -eq 0 ]; then
  ok "T20 dong '--- PASS' do lenh in ra STDERR cung duoc ghi lai va dem => khong canh bao"
else
  bad "T20 dong '--- PASS' do lenh in ra STDERR cung duoc ghi lai va dem => khong canh bao" "exit=$RC so dong ::warning=$(count_warning_lines "$OUT")"
fi

# ---------------------------------------------------------------- summary
SF="$T/summary-distinct.md"; : >"$SF"
run_sut "$SF" "$R" bash "$T/fake.sh" "$F_DISTINCT" 6
if [ "$RC" -eq 6 ]; then ok "T21 lenh exit 6 co ca PASS/FAIL => script exit DUNG 6"
else bad "T21 lenh exit 6 co ca PASS/FAIL => script exit DUNG 6" "exit=$RC"; fi
expect_counts "T22 summary co bang voi PASS=3 FAIL=2 SKIP=5 va exit code 6 (khong dem TestOther*)" "$SF" 3 2 5 6
if grep -q '|' "$SF"; then ok "T23 summary la bang markdown (co ky tu '|')"
else bad "T23 summary la bang markdown (co ky tu '|')" "$(head -5 "$SF")"; fi

SF="$T/summary-append.md"
printf '%s\n' "### NOI DUNG CU CUA STEP TRUOC" "giu nguyen dong nay 5c1d" >"$SF"
cp "$SF" "$T/summary-before.md"
run_sut "$SF" "$R" bash "$T/fake.sh" "$F_PASS" 0
cp "$SF" "$T/summary-after1.md"
run_sut "$SF" "$R" bash "$T/fake.sh" "$F_SKIP" 0
sz0=$(wc -c <"$T/summary-before.md"); sz1=$(wc -c <"$T/summary-after1.md"); sz2=$(wc -c <"$SF")
pre1=$(head -c "$sz0" "$T/summary-after1.md" | cmp -s - "$T/summary-before.md" && echo y || echo n)
pre2=$(head -c "$sz1" "$SF" | cmp -s - "$T/summary-after1.md" && echo y || echo n)
if [ "$pre1" = y ] && [ "$pre2" = y ] && [ "$sz1" -gt "$sz0" ] && [ "$sz2" -gt "$sz1" ]; then
  ok "T24 GITHUB_STEP_SUMMARY duoc APPEND: noi dung cu con nguyen o dau file, moi lan chay them noi dung moi"
else
  bad "T24 GITHUB_STEP_SUMMARY duoc APPEND: noi dung cu con nguyen o dau file, moi lan chay them noi dung moi" \
      "kich thuoc: truoc=$sz0 sau-lan1=$sz1 sau-lan2=$sz2; giu-prefix lan1=$pre1 lan2=$pre2"
fi

SF="$T/summary-skip.md"; : >"$SF"
run_sut "$SF" "$R" bash "$T/fake.sh" "$F_SKIP" 0
gp="$(summary_value pass "$SF")"; gs="$(summary_value skip "$SF")"
if [ "$gp" = 0 ] && [ "$gs" = 3 ] && grep -Eq 'KHONG kiem gi|KHÔNG kiểm gì' "$SF"; then
  ok "T25 (AC-4) toan SKIP => summary ghi 0 PASS, 3 SKIP va 'KHONG kiem gi'"
else
  bad "T25 (AC-4) toan SKIP => summary ghi 0 PASS, 3 SKIP va 'KHONG kiem gi'" "PASS doc duoc='$gp' SKIP doc duoc='$gs'" "$(sed 's/^/  | /' "$SF" | head -10)"
fi

rm -rf "$T/work" "$T/suttmp"; mkdir -p "$T/work" "$T/suttmp"
run_sut "" "$R" bash "$T/fake.sh" "$F_PASS" 0
left_work="$(ls -A "$T/work")"; left_tmp="$(ls -A "$T/suttmp")"
if [ -z "$left_work" ] && [ -z "$left_tmp" ]; then
  ok "T26 GITHUB_STEP_SUMMARY KHONG dat => khong tao file nao (thu muc lam viec va TMPDIR van rong)"
else
  bad "T26 GITHUB_STEP_SUMMARY KHONG dat => khong tao file nao (thu muc lam viec va TMPDIR van rong)" "cwd: $left_work" "TMPDIR: $left_tmp"
fi

rm -rf "$T/suttmp"; mkdir -p "$T/suttmp"
SF="$T/summary-tmp.md"; : >"$SF"
run_sut "$SF" "$R" bash "$T/fake.sh" "$F_FAIL" 5
left_tmp="$(ls -A "$T/suttmp")"; left_work="$(ls -A "$T/work")"
if [ -z "$left_tmp" ] && [ -z "$left_work" ]; then ok "T27 ca khi lenh fail, script khong de lai file tam trong TMPDIR / cwd"
else bad "T27 ca khi lenh fail, script khong de lai file tam trong TMPDIR / cwd" "cwd: $left_work" "TMPDIR: $left_tmp"; fi

# ---------------------------------------------------------------- output khong bi nuot
run_sut "" "$R" bash "$T/fake.sh" "$F_MARK_OUT" 0 "$F_MARK_ERR"
if printf '%s\n' "$OUT" | grep -q 'MARKER-STDOUT-7f3a9c' && printf '%s\n' "$OUT" | grep -q -- '--- PASS: TestNoOversell_Basic'; then
  ok "T28 stdout cua lenh van hien ra stdout cua script (khong bi nuot)"
else
  bad "T28 stdout cua lenh van hien ra stdout cua script (khong bi nuot)" "stdout: $(printf '%s' "$OUT" | head -5)"
fi
if printf '%s\n%s\n' "$OUT" "$ERR" | grep -q 'MARKER-STDERR-b41e02'; then ok "T29 stderr cua lenh van hien ra console"
else bad "T29 stderr cua lenh van hien ra console" "stdout+stderr khong chua marker"; fi

run_sut "" "$R" bash "$T/args.sh" "a b" "" 'c"d'
if printf '%s\n' "$OUT" | grep -qx 'NARGS=3' && printf '%s\n' "$OUT" | grep -qx 'ARG1=<a b>' \
   && printf '%s\n' "$OUT" | grep -qx 'ARG2=<>' && printf '%s\n' "$OUT" | grep -qx 'ARG3=<c"d>'; then
  ok "T30 [args...] chuyen nguyen ven cho <cmd> (khoang trang, chuoi rong, dau nhay)"
else
  bad "T30 [args...] chuyen nguyen ven cho <cmd> (khoang trang, chuoi rong, dau nhay)" "$(printf '%s' "$OUT" | head -5)"
fi

# ---------------------------------------------------------------- regex (K1, bo sung sau Challenge vong 1)
F_AB="$(fx a-or-b <<'EOF'
=== RUN   TestA
--- PASS: TestA (0.01s)
=== RUN   TestB
--- PASS: TestB (0.01s)
PASS
EOF
)"
run_sut "" 'TestA|TestB' bash "$T/fake.sh" "$F_AB" 0
if [ "$RC" -eq 0 ] && [ "$(count_warning_lines "$OUT")" -eq 0 ]; then
  ok "T31 regex 'TestA|TestB', ca hai PASS => exit 0, khong canh bao"
else
  bad "T31 regex 'TestA|TestB', ca hai PASS => exit 0, khong canh bao" "exit=$RC so dong ::warning=$(count_warning_lines "$OUT")"
fi

# Phan biet "nhom" voi "khong nhom": khong nhom thi `^...--- PASS: TestA|TestB` khop MOI dong
# chua TestB o bat ky dau (ca dong SKIP) => PASS > 0 sai => mat canh bao.
F_B_SKIP="$(fx b-skip <<'EOF'
=== RUN   TestB
    helpers.go:33: TODO(EVF-39): TestB chua hien thuc
--- SKIP: TestB (0.00s)
PASS
EOF
)"
SF="$T/summary-group.md"; : >"$SF"
run_sut "$SF" 'TestA|TestB' bash "$T/fake.sh" "$F_B_SKIP" 0
gp="$(summary_value pass "$SF")"; gs="$(summary_value skip "$SF")"
if [ "$RC" -eq 0 ] && [ "$(count_warning_lines "$OUT")" -eq 1 ] && [ "$gp" = 0 ] && [ "$gs" = 1 ]; then
  ok "T32 regex 'TestA|TestB' duoc NHOM: chi '--- SKIP: TestB' => PASS=0 SKIP=1 va van canh bao"
else
  bad "T32 regex 'TestA|TestB' duoc NHOM: chi '--- SKIP: TestB' => PASS=0 SKIP=1 va van canh bao" \
      "exit=$RC so dong ::warning=$(count_warning_lines "$OUT") PASS='$gp' SKIP='$gs'"
fi

F_X="$(fx x-pass <<'EOF'
=== RUN   TestX
--- PASS: TestX (0.01s)
PASS
EOF
)"
run_sut "" '^TestX$' bash "$T/fake.sh" "$F_X" 0
if [ "$RC" -eq 0 ] && [ "$(count_warning_lines "$OUT")" -eq 0 ]; then
  ok "T33 regex '^TestX\$' (cu phap go test -run) voi '--- PASS: TestX' => bo ^/\$, dem duoc PASS, khong canh bao"
else
  bad "T33 regex '^TestX\$' (cu phap go test -run) voi '--- PASS: TestX' => bo ^/\$, dem duoc PASS, khong canh bao" \
      "exit=$RC so dong ::warning=$(count_warning_lines "$OUT")"
fi

run_sut "" 'Test[' bash "$T/fake.sh" "$F_PASS" 0
if [ "$RC" -eq 2 ] && [ -n "$ERR" ]; then ok "T34 regex khong hop le 'Test[' => exit 2 + loi ra stderr (fail-closed, khong roi ve exit 0)"
else bad "T34 regex khong hop le 'Test[' => exit 2 + loi ra stderr (fail-closed, khong roi ve exit 0)" "exit=$RC" "stderr=$(printf '%s' "$ERR" | head -3)"; fi

SUT_TMPDIR="/nonexistent-evf-$$/x"
run_sut "" "$R" bash "$T/fake.sh" "$F_PASS" 0
unset SUT_TMPDIR
if [ "$RC" -eq 2 ] && [ -n "$ERR" ]; then ok "T35 TMPDIR=/nonexistent/x (khong tao duoc file tam) => exit 2 + loi ra stderr"
else bad "T35 TMPDIR=/nonexistent/x (khong tao duoc file tam) => exit 2 + loi ra stderr" "exit=$RC" "stderr=$(printf '%s' "$ERR" | head -3)"; fi

# ---------------------------------------------------------------- ket qua
echo
echo "== Ket qua: $PASSED PASS, $FAILED FAIL =="
if [ "$FAILED" -gt 0 ]; then
  printf '   ca fail: %s\n' "$(printf '%s ' "${FAILED_NAMES[@]%% *}")"
  exit 1
fi
exit 0
