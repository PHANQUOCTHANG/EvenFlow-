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

PKGS=$(go list ./... 2>/dev/null | grep -E '/internal/(domain|app)(/|$)' || true)
if [ -z "$PKGS" ]; then
  echo "[$MODULE_NAME] chua co goi internal/domain hoac internal/app, bo qua coverage gate"
  exit 0
fi

COVER_FILE=$(mktemp)
go test -covermode=atomic -coverprofile="$COVER_FILE" $PKGS >/dev/null

PCT=$(go tool cover -func="$COVER_FILE" | tail -1 | grep -oE '[0-9]+\.[0-9]+')
echo "[$MODULE_NAME] coverage domain/app: ${PCT}% (nguong ${THRESHOLD}%)"

awk -v pct="$PCT" -v th="$THRESHOLD" 'BEGIN { exit !(pct + 0 >= th + 0) }' || {
  echo "[$MODULE_NAME] FAIL: coverage domain/app ${PCT}% < ${THRESHOLD}%"
  exit 1
}
