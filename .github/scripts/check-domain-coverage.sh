#!/usr/bin/env bash
# Chan PR khi coverage cua internal/domain va internal/app duoi nguong.
# Dung: check-domain-coverage.sh <thu-muc-module-go> [nguong=70]
#
# Chi ap dung cho module da co goi domain/app (nhieu service con dang scaffold
# rong nen se duoc bo qua thay vi lam CI do do khi chua co gi de test).
set -euo pipefail

MODULE_DIR="$1"
THRESHOLD="${2:-70}"

cd "$MODULE_DIR"
MODULE_NAME=$(basename "$MODULE_DIR")

# Tach loi cua `go list` khoi truong hop "khong co goi nao". Ban cu
# `go list ./... 2>/dev/null | grep ... || true` nuot ca hai: module khong nap duoc (go.mod
# hong, go.work lech version) cung ra danh sach rong -> in "bo qua" -> exit 0. Tuc mot loi
# build lam cong coverage XANH. Da xay ra that khi go.work ghi `go 1.25` con go.mod ghi
# `go 1.25.0`.
if ! ALL_PKGS=$(go list ./... 2>&1); then
  echo "[$MODULE_NAME] FAIL: go list loi -- khong xac dinh duoc goi nao can do coverage:" >&2
  printf '%s\n' "$ALL_PKGS" >&2
  exit 1
fi
PKGS=$(printf '%s\n' "$ALL_PKGS" | grep -E '/internal/(domain|app)(/|$)' || true)
if [ -z "$PKGS" ]; then
  echo "[$MODULE_NAME] chua co goi internal/domain hoac internal/app, bo qua coverage gate"
  exit 0
fi

COVER_FILE=$(mktemp)
trap 'rm -f "$COVER_FILE"' EXIT
# Giu output khi test FAIL: ban cu `>/dev/null` lam job do ma khong noi test nao do.
if ! TEST_OUT=$(go test -covermode=atomic -coverprofile="$COVER_FILE" $PKGS 2>&1); then
  echo "[$MODULE_NAME] FAIL: test domain/app khong qua:" >&2
  printf '%s\n' "$TEST_OUT" >&2
  exit 1
fi

PCT=$(go tool cover -func="$COVER_FILE" | tail -1 | grep -oE '[0-9]+\.[0-9]+')
echo "[$MODULE_NAME] coverage domain/app: ${PCT}% (nguong ${THRESHOLD}%)"

awk -v pct="$PCT" -v th="$THRESHOLD" 'BEGIN { exit !(pct + 0 >= th + 0) }' || {
  echo "[$MODULE_NAME] FAIL: coverage domain/app ${PCT}% < ${THRESHOLD}%"
  exit 1
}
