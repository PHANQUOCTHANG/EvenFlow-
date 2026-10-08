#!/usr/bin/env bash
# Chay mot lenh test Go roi BAO RO no da thuc su kiem bao nhieu lan -- contract K1
# (docs/workflow/ci-cd-review/contracts.md).
#
# Vi sao can: `go test` coi SKIP la thanh cong. Oversell gate (EVF-39) dang chay
# TestNoOversell x200 va xanh, trong khi `setupEnv` goi `t.Skip` -- tuc 0 lan kiem
# that. Mot gate xanh ma khong kiem gi la thu nguy hiem nhat mot CI co the in ra,
# vi no trong y het mot gate dang bao ve that.
#
# Script nay KHONG doi ket luan cua lenh test: exit code duoc giu nguyen (fail van
# la fail). No chi them hai thu:
#   - summary dem PASS / FAIL / SKIP vao $GITHUB_STEP_SUMMARY
#   - mot dong `::warning` khi khong co lan PASS nao, de "xanh" khong con cam
#
# Dung:
#   bash run-and-report-tests.sh <test-regex> <cmd> [args...]
# Lenh can in theo dinh dang `go test -v` thi moi dem duoc.
set -uo pipefail

if [ "$#" -lt 2 ]; then
  echo "dung: $0 <test-regex> <cmd> [args...]" >&2
  exit 2
fi

pattern="$1"
shift

# Cu phap quen cua `go test -run` la `^TestX$`, nhung o day ten test dung GIUA dong
# (`--- PASS: TestX (0.01s)`), nen `^`/`$` khong bao gio khop va script se canh bao
# "khong kiem gi" mot cach sai. Bo dung mot `^` dau va mot `$` cuoi.
pattern="${pattern#^}"
pattern="${pattern%\$}"

# Fail-CLOSED: khong tao duoc file tam thi khong dem duoc gi -- khong duoc coi la "0 FAIL".
if ! log="$(mktemp)"; then
  echo "run-and-report-tests: khong tao duoc file tam" >&2
  exit 2
fi
trap 'rm -f "$log"' EXIT

# `tee` de output van hien ra console ngay khi chay. PIPESTATUS[0] la exit code cua
# CHINH lenh test, khong phai cua tee.
"$@" 2>&1 | tee "$log"
cmd_rc=${PIPESTATUS[0]}

# Dem theo dinh dang `go test -v`. Subtest thut le ("    --- PASS: X/sub") cung tinh,
# nen cho phep khoang trang dau dong. Regex duoc NHOM: khong co ngoac thi `TestA|TestB`
# thanh "dong bat dau bang TestA, HOAC chuoi TestB o bat ky dau".
#
# grep: exit 1 = khong khop (dem = 0, hop le); exit >= 2 = regex hong. Truoc day ca hai
# bi `|| true` nuot, so dem thanh rong, phep so sanh so hoc loi va bi coi la false ->
# script khong canh bao va khong ep FAIL: fail-OPEN. Gio regex hong la exit 2.
count() {
  local n rc
  n=$(grep -cE "^[[:space:]]*--- $1: (${pattern})" "$log")
  rc=$?
  if [ "$rc" -ge 2 ]; then
    echo "run-and-report-tests: regex khong hop le: ${pattern}" >&2
    exit 2
  fi
  echo "$n"
}
# Goi trong $(...) nen `exit 2` ben trong chi thoat subshell: phai kiem lai o day.
n_pass=$(count PASS) || exit 2
n_fail=$(count FAIL) || exit 2
n_skip=$(count SKIP) || exit 2

rc=$cmd_rc
if [ "$rc" -eq 0 ] && [ "$n_fail" -gt 0 ]; then
  # Lenh exit 0 ma van co dong FAIL: khong tin exit code, tin output.
  rc=1
fi

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  {
    echo "### Ket qua test \`${pattern}\`"
    echo
    echo "| PASS | FAIL | SKIP | exit |"
    echo "|---|---|---|---|"
    echo "| ${n_pass} | ${n_fail} | ${n_skip} | ${rc} |"
    echo
    if [ "$cmd_rc" -eq 0 ] && [ "$n_pass" -eq 0 ]; then
      echo "**Gate nay KHONG kiem gi:** 0 lan PASS (${n_skip} lan SKIP). Mau xanh o day khong chung minh dieu gi."
    fi
  } >> "$GITHUB_STEP_SUMMARY"
fi

if [ "$cmd_rc" -eq 0 ] && [ "$n_pass" -eq 0 ]; then
  echo "::warning title=${pattern} KHONG kiem gi::0 lan PASS, ${n_fail} lan FAIL, ${n_skip} lan SKIP. Lenh exit 0 nhung khong co test nao thuc su chay qua -- KHONG kiem gi."
fi

exit "$rc"
