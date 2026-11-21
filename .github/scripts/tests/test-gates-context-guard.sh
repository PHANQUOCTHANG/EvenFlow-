#!/usr/bin/env bash
#
# Test hanh vi cho gates.sh C4.4 (k8s-all AC-26, TCR-3): context guard + kubectl pin + migrate Job.
#
#   bash .github/scripts/tests/test-gates-context-guard.sh               # chay tu GOC repo
#   bash .github/scripts/tests/test-gates-context-guard.sh <repo-root>   # kiem ban sao khac
#
# Khong can cluster: lay khoi C4.4 (tu dong `K8S_CTX=""` toi comment `# C4.5 -- kiem`) ra khoi gates.sh
# va chay no voi mot kubectl GIA ghi lai moi lenh goi. Kustomize da pin bi an (XDG_CACHE_HOME rong),
# nen nhanh `kc apply -k` duoc dung.
#
# Bien moi truong (chi de tu kiem bo test nay, KHONG dung trong CI):
#   SUT=<duong dan>   thay gates.sh duoc kiem (vd mot ban dot bien)
#
# Moi ca in PASS/FAIL. Exit != 0 neu co it nhat mot ca FAIL.

set -u

ROOT="${1:-${REPO_ROOT:-.}}"
if [ ! -d "$ROOT/.github" ]; then
  echo "Khong thay $ROOT/.github. Hay chay tu goc repo: bash .github/scripts/tests/test-gates-context-guard.sh [repo-root]" >&2
  exit 2
fi
ROOT="$(cd "$ROOT" && pwd)"
SUT="${SUT:-$ROOT/.github/scripts/gates.sh}"
case "$SUT" in /*|[A-Za-z]:*) ;; *) SUT="$ROOT/$SUT" ;; esac

T="$(mktemp -d "${TMPDIR:-/tmp}/gcg-test.XXXXXX")" || exit 2
trap 'rm -rf "$T"' EXIT INT TERM
mkdir -p "$T/bin" "$T/nocache"

PASSED=0
FAILED=0
FAILED_NAMES=()
ok()  { PASSED=$((PASSED + 1)); printf 'PASS  %s\n' "$1"; }
bad() { FAILED=$((FAILED + 1)); FAILED_NAMES+=("$1"); printf 'FAIL  %s\n      %s\n' "$1" "$2"; }

# kubectl gia: ghi "$*" vao $CALLS. FAKE_CTX = current-context; FAKE_SUCC = status.succeeded cua
# job/eventflow-migrate ("" = Job khong ton tai). Namespace luon "chua co" de nhanh create chay.
cat > "$T/bin/kubectl" <<'K'
#!/usr/bin/env bash
echo "$*" >> "$CALLS"
case "$*" in
  "cluster-info") exit 0;;
  "config current-context") echo "$FAKE_CTX"; exit 0;;
esac
case "$*" in
  *"get namespace"*) exit 1;;
  *"get job eventflow-migrate -o jsonpath"*) printf '%s' "$FAKE_SUCC"; exit 0;;
  *"get job eventflow-migrate"*) [ -n "$FAKE_SUCC" ]; exit $?;;
  *"apply"*"-f -"*) cat >/dev/null 2>&1; exit 0;;
esac
exit 0
K
chmod +x "$T/bin/kubectl"

if [ ! -f "$SUT" ]; then
  echo "SUT khong ton tai: $SUT" >&2
  exit 1
fi
{
  echo 'K8S_DIR="deploy/k8s"'
  echo "log()  { printf '[gates] %s\n' \"\$*\"; }"
  echo "fail() { printf '[gates] FAIL: %s\n' \"\$*\" >&2; }"
  awk '/^K8S_CTX=""/{p=1} /^# C4.5 -- kiem/{p=0} p' "$SUT"
} > "$T/c44.sh"
# Khong lay duoc khoi C4.4 (marker doi / bi xoa) thi moi ca ben duoi se vo nghia -> fail ngay.
if ! grep -q 'kubectl config current-context' "$T/c44.sh" || ! grep -q 'apply --dry-run=server' "$T/c44.sh"; then
  echo "FAIL  khong lay duoc khoi C4.4 tu $SUT (marker 'K8S_CTX=\"\"' .. '# C4.5 -- kiem')"
  exit 1
fi

RC=0
run_c44() { # run_c44 <ctx> <succeeded>
  : > "$T/calls"
  (cd "$ROOT" && CALLS="$T/calls" FAKE_CTX="$1" FAKE_SUCC="$2" PATH="$T/bin:$PATH" XDG_CACHE_HOME="$T/nocache" \
    bash "$T/c44.sh" </dev/null >"$T/out" 2>&1)
  RC=$?
}
cluster_calls() { grep -vE '^(cluster-info|config current-context)$' "$T/calls" || true; }
# lenh kubectl khong pin --context <ctx> (bo qua cluster-info, current-context, version --client trong dong log)
unpinned() { grep -vE '^(cluster-info|config current-context|version --client.*)$' "$T/calls" | grep -v -- "^--context $1 " || true; }

# ---------------------------------------------------------------- context khong phai local
for ctx in prod-cluster docker-desktop-prod staging-kind-dev xkind-dev; do
  name="G-$ctx: context khong phai docker-desktop/kind-* => khong dung vao cluster, exit 0"
  run_c44 "$ctx" ""
  c="$(cluster_calls)"
  if [ -n "$c" ]; then bad "$name" "kubectl da goi: $(echo "$c" | head -1)"
  elif [ "$RC" -ne 0 ]; then bad "$name" "exit=$RC"
  elif ! grep -q "BO QUA validate k8s: context '$ctx'" "$T/out"; then bad "$name" "khong in dong BO QUA cho '$ctx'"
  else ok "$name"; fi
done

# ---------------------------------------------------------------- docker-desktop, Job chua xong
name="G-unfinished: Job migrate chua xong => KHONG xoa, bi loai khoi dry-run"
run_c44 docker-desktop 0
if grep -q 'delete job' "$T/calls"; then bad "$name" "Job chua xong bi xoa"
elif ! grep -q -- '--context docker-desktop apply --dry-run=server -k deploy/k8s/overlays/local -l app.kubernetes.io/name!=eventflow-migrate' "$T/calls"; then
  bad "$name" "dry-run khong loai Job: $(grep apply "$T/calls" | head -1)"
elif [ "$RC" -ne 0 ]; then bad "$name" "exit=$RC"
else ok "$name"; fi
name="G-pin-docker-desktop: moi lenh kubectl sau guard deu --context docker-desktop"
u="$(unpinned docker-desktop)"
if [ -n "$u" ]; then bad "$name" "lenh khong pin: $(echo "$u" | head -1)"; else ok "$name"; fi
name="G-namespace: namespace chua co => tao tren context da pin"
if grep -qx -- '--context docker-desktop create namespace eventflow' "$T/calls"; then ok "$name"
else bad "$name" "khong thay 'create namespace' pin context"; fi

# ---------------------------------------------------------------- kind-*, Job da xong
name="G-finished: Job migrate da xong => xoa tren context da pin, dry-run khong loai Job"
run_c44 kind-dev 1
if ! grep -qx -- '--context kind-dev -n eventflow delete job eventflow-migrate --ignore-not-found' "$T/calls"; then
  bad "$name" "khong xoa Job voi --context kind-dev"
elif grep -q 'eventflow-migrate' <(grep apply "$T/calls"); then bad "$name" "dry-run van loai Job da xoa"
elif [ "$RC" -ne 0 ]; then bad "$name" "exit=$RC"
else ok "$name"; fi
name="G-pin-kind: moi lenh kubectl sau guard deu --context kind-dev"
u="$(unpinned kind-dev)"
if [ -n "$u" ]; then bad "$name" "lenh khong pin: $(echo "$u" | head -1)"; else ok "$name"; fi

# ---------------------------------------------------------------- kind-*, khong co Job
name="G-no-job: khong co Job migrate => khong xoa, khong loai, van dry-run"
run_c44 kind-ci ""
if grep -q 'delete job' "$T/calls"; then bad "$name" "xoa Job khong ton tai"
elif ! grep -qx -- '--context kind-ci apply --dry-run=server -k deploy/k8s/overlays/local' "$T/calls"; then
  bad "$name" "dry-run sai: $(grep apply "$T/calls" | head -1)"
elif [ "$RC" -ne 0 ]; then bad "$name" "exit=$RC"
else ok "$name"; fi

# ---------------------------------------------------------------- khong co context
name="G-no-ctx: cluster-info OK nhung current-context rong => khong dung vao cluster"
run_c44 "" ""
c="$(cluster_calls)"
if [ -n "$c" ]; then bad "$name" "kubectl da goi: $(echo "$c" | head -1)"
elif ! grep -q 'khong co current-context' "$T/out"; then bad "$name" "khong in dong BO QUA (khong co current-context)"
else ok "$name"; fi

# ---------------------------------------------------------------- ket qua
echo
echo "== Ket qua: $PASSED PASS, $FAILED FAIL =="
if [ "$FAILED" -gt 0 ]; then
  printf '   ca fail: %s\n' "$(printf '%s ' "${FAILED_NAMES[@]%%:*}")"
  exit 1
fi
exit 0
