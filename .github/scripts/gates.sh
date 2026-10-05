#!/usr/bin/env bash
#
# Cong chat luong cuc bo cho slice deploy -- WP-C, hop dong C4.
#
#   bash .github/scripts/gates.sh
#
# Script chay DUNG bon lenh cua C4.1, luon luon, dung thu tu do, roi them hai
# check ma CI khong co -- C4.2 (chan .skip/.only/it.todo moi them) va C4.3
# (chan probe bi prerender tinh) -- roi C4.4 (validate manifest k8s) va cuoi cung
# C4.5 (co readiness + nhanh 503 phai con trong .next/server/, tang ma 4 gate
# cong C4.2 cong C4.3 deu khong thay).
#
# KHONG co co dong lenh hay bien moi truong nao bo qua duoc bat ky buoc nao.
# Bien moi truong duy nhat script doc la BASE_REF (C4, C4.2).
#
# Ly do script ton tai (sua lai cho dung theo m1/P2): KHONG phai vi ci.yml thieu
# lenh -- ci.yml da co du bon (dong 104 lint, 105 typecheck, 170 test:coverage,
# 227 build). Loi that da xay ra la o vong chay CUC BO truoc khi commit. Script
# nay de chay dung bon lenh do cuc bo, cong hai check CI khong co.
#
# Phu thuoc: bash, git, node, npm. kubectl la TUY CHON (xem C4.4).
# KHONG phu thuoc make / go / golangci-lint -- may dev khong co ca ba.
# Vi vay `make gates` chi la tien ich; duong chay chinh thuc la chinh file nay.

set -euo pipefail

# Goc repo suy ra tu vi tri script => chay dung ca khi duoc goi tu thu muc khac.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

# Mac dinh la base commit ghi o dau contracts.md: commit chua chinh file do tren
# feature/deploy-k8s-web -- 8b869bb "docs(workflow): contract approved; close the
# remaining review items". Khong dung SHA cua develop (B5/m5).
BASE_REF="${BASE_REF:-8b869bb2981a41a3393180b31e2a14e5cb95f6ec}"

WEB_DIR="apps/web"
K8S_DIR="deploy/k8s"
PRERENDER_MANIFEST="$WEB_DIR/.next/prerender-manifest.json"
# Hai vung mu da duoc Challenge chung minh:
#
# 1. `apps/web/**/*.test.*` can it nhat mot cap thu muc, nen file test dat ngay tai
#    `apps/web/` se VO HINH. Dung pathspec `:(glob)` cho dung ngu nghia.
# 2. `vitest.config.ts` include ca `*.spec.*`, nhung pathspec cu chi co `*.test.*`:
#    doi ten file tu .test.ts sang .spec.ts la du vo hieu hoa TOAN BO AC-5b, ma test
#    van chay binh thuong nen khong ai thay gi la.
TEST_GLOBS=(
  ':(glob)apps/web/**/*.test.*'
  ':(glob)apps/web/*.test.*'
  ':(glob)apps/web/**/*.spec.*'
  ':(glob)apps/web/*.spec.*'
)
TEST_GLOB="${TEST_GLOBS[*]}"
PROBE_ROUTES=("/api/healthz" "/api/readyz")

TOTAL_STEPS=4
step_no=0

log()  { printf '[gates] %s\n' "$*"; }
fail() { printf '[gates] FAIL: %s\n' "$*" >&2; }

# run_step <nhan> <doan lenh>
# Doan lenh duoc in ra nguyen van roi chay nguyen van. Fail o bat ky buoc nao
# => in ro buoc nao fail va thoat voi dung exit code cua lenh do (AC-5).
run_step() {
  local label="$1" snippet="$2" rc=0
  step_no=$((step_no + 1))
  log "buoc ${step_no}/${TOTAL_STEPS} ${label}: ${snippet}"
  bash -c "$snippet" || rc=$?
  if [ "$rc" -ne 0 ]; then
    fail "buoc ${step_no}/${TOTAL_STEPS} ${label} that bai (exit ${rc}): ${snippet}"
    exit "$rc"
  fi
  log "  OK buoc ${step_no}/${TOTAL_STEPS} ${label}"
}

# ---------------------------------------------------------------------------
# C4.1 -- bon lenh, ghi literal, chay het moi lan
# ---------------------------------------------------------------------------

run_step lint          '(cd apps/web && npm run lint)'
run_step typecheck     '(cd apps/web && npm run typecheck)'
run_step test:coverage '(cd apps/web && npm run test:coverage)'
run_step build         '(cd apps/web && npm run build)'

# ---------------------------------------------------------------------------
# C4.2 -- chan .skip/.only/it.todo MOI THEM trong file test
# ---------------------------------------------------------------------------

log "check C4.2: .skip/.only/it.todo moi them trong ${TEST_GLOB} so voi BASE_REF"
log "  BASE_REF=${BASE_REF}"

if ! git rev-parse --verify --quiet "${BASE_REF}^{commit}" >/dev/null; then
  fail "BASE_REF='${BASE_REF}' khong giai duoc thanh commit trong repo nay."
  fail "Khong the so diff test => coi nhu KHONG DAT, khong im lang pass."
  exit 1
fi

# Chi xet dong THEM ('^\+'), nen test cu von da co .skip thi khong bi tinh.
# `|| true` bao ca pipeline: grep khong tim thay gi thi tra ve 1, va day la ket
# qua TOT, khong phai loi.
# `|| true` dat o cuoi CA pipeline se nuot luon exit code cua `git diff`, khong
# chi cua `grep` -- va `set -o pipefail` cung bi vo hieu vi `||` thay the status
# cua toan bo pipeline. Khi do mot BASE_REF khong co merge base voi HEAD (clone
# shallow bi cat to tien) lam `git diff` fail, bien rong, va gate in "OK" roi di
# tiep: dung che do am tham pass ma C4.2/AC-5 cam. Tach lam hai tang.
if ! committed_diff="$(git diff --unified=0 "$BASE_REF"...HEAD -- "${TEST_GLOBS[@]}" 2>&1)"; then
  fail "git diff voi BASE_REF='$BASE_REF' that bai:"
  printf '%s\n' "$committed_diff" >&2
  fail "KHONG khang dinh duoc la khong co .skip moi them => coi nhu KHONG DAT."
  exit 1
fi

# Tang thu hai: WORKING TREE. `$BASE_REF...HEAD` chi so hai COMMIT, no khong thay
# thay doi chua commit -- tuc mu dung voi vong chay ma script nay tu nhan la ly do
# no ton tai ("chay cuc bo TRUOC khi commit"). Kich ban dung tinh huong nhat: dev
# them .skip -> chay gate -> xanh -> commit -> push. Check chi bat duoc sau khi da
# commit, tuc sau thoi diem no can phat huy.
if ! worktree_diff="$(git diff --unified=0 HEAD -- "${TEST_GLOBS[@]}" 2>&1)"; then
  fail "git diff working tree that bai:"
  printf '%s\n' "$worktree_diff" >&2
  exit 1
fi

diff_out="$committed_diff
$worktree_diff"

added_skips="$(
  printf '%s\n' "$diff_out" \
    | grep -E '^\+' \
    | grep -E '\.(skip|only|todo|skipIf|runIf|concurrent\.skip)\(|\.(skip|only|todo)If\(' \
    || true
)"

if [ -n "$added_skips" ]; then
  fail "co .skip/.only/it.todo MOI THEM trong file test:"
  printf '%s\n' "$added_skips" >&2
  exit 1
fi
log "  OK: khong co dong them nao chua .skip/.only/it.todo"

# ---------------------------------------------------------------------------
# C4.3 -- hai probe KHONG duoc nam trong prerender manifest
# ---------------------------------------------------------------------------

log "check C4.3: ${PROBE_ROUTES[*]} khong duoc bi prerender tinh"

if [ ! -f "$PRERENDER_MANIFEST" ]; then
  fail "khong thay ${PRERENDER_MANIFEST} sau 'npm run build'."
  fail "Khong khang dinh duoc hai probe la dynamic => coi nhu KHONG DAT."
  fail "Im lang pass o day chinh la che do loi ma check nay sinh ra de chong."
  exit 1
fi

# Hai duong dan probe duoc truyen qua BIEN MOI TRUONG duoi dang JSON, KHONG qua
# argv. Ly do, da gap that khi tu kiem script nay: Git Bash (MSYS2) dich moi
# tham so trong giong duong dan POSIX thanh duong dan Windows truoc khi goi mot
# .exe native, nen `node -e ... /api/healthz` lam node nhan duoc
# "C:/Program Files/Git/api/healthz". So khop khi do KHONG BAO GIO dung, va check
# luon xanh KE CA khi probe thuc su bi prerender -- dung che do loi te nhat ma
# C4.3 sinh ra de chong. Gia tri JSON bat dau bang [ nen MSYS2 khong dich.
probe_routes_json="[$(printf '"%s",' "${PROBE_ROUTES[@]}" | sed 's/,$//')]"

GATES_PROBE_ROUTES="$probe_routes_json" node -e '
const fs = require("node:fs");
const manifestPath = process.argv[1];
const routes = JSON.parse(process.env.GATES_PROBE_ROUTES);

let manifest;
try {
  manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
} catch (err) {
  console.error("[gates] FAIL: khong doc/parse duoc " + manifestPath + ": " + err.message);
  process.exit(1);
}

// Duong dan route xuat hien o nhieu cho trong manifest: khoa cua "routes" va
// "dynamicRoutes", phan tu cua "notFoundRoutes". Quet ca cay va so khop ca khoa
// lan gia tri chuoi, de khong phu thuoc shape cua mot phien ban Next cu the.
const seen = new Set();
const walk = (node) => {
  if (Array.isArray(node)) { node.forEach(walk); return; }
  if (node && typeof node === "object") {
    for (const [key, value] of Object.entries(node)) { seen.add(key); walk(value); }
    return;
  }
  if (typeof node === "string") seen.add(node);
};
walk(manifest);

const hits = routes.filter((route) => seen.has(route));
if (hits.length > 0) {
  console.error("[gates] FAIL: probe bi prerender tinh: " + hits.join(", "));
  console.error("[gates] Probe bi prerender tra ve ket qua dong bang tu luc build,");
  console.error("[gates] tuc luon xanh ke ca khi tien trinh da hong -- che do loi te nhat.");
  console.error("[gates] Sua: dat dynamic = force-dynamic trong route handler (C1).");
  process.exit(1);
}

console.log("[gates]   OK: " + routes.join(", ") + " khong nam trong prerender manifest");
' "$PRERENDER_MANIFEST"

# ---------------------------------------------------------------------------
# C4.4 -- validate manifest k8s, hoac noi thang ra la khong kiem duoc gi
# ---------------------------------------------------------------------------

log "check C4.4: validate manifest trong ${K8S_DIR}/"

# Dieu kien phat hien la `kubectl cluster-info` THANH CONG, khong phai "co
# kubectl" (B1). Da kiem tren may that: --dry-run=client KHONG phai che do
# offline, no tai openapi tu API server va fail khi chua co cluster.
if kubectl cluster-info >/dev/null 2>&1; then
  if [ ! -d "$K8S_DIR" ]; then
    log "  ${K8S_DIR}/ chua ton tai (WP-B tao) -- khong co gi de validate."
  else
    # Duyet theo glob, khong gia dinh ten file, de WP-B tu do dat ten.
    shopt -s nullglob globstar
    manifests=( "$K8S_DIR"/**/*.yaml "$K8S_DIR"/**/*.yml )
    shopt -u nullglob globstar

    if [ "${#manifests[@]}" -eq 0 ]; then
      log "  khong co file .yaml/.yml nao trong ${K8S_DIR}/ -- khong co gi de validate."
    else
      files_args=()
      for manifest_file in "${manifests[@]}"; do
        files_args+=( -f "$manifest_file" )
      done
      # --dry-run=server KHONG persist object, nen Namespace trong chinh lan
      # dry-run nay chua ton tai that. Admission plugin NamespaceLifecycle se tu
      # choi moi resource namespaced bang `namespaces "eventflow" not found`,
      # lam gate do trong khi manifest hoan toan dung -- va nguoi doc log rat de
      # ket luan sai la manifest hong. Nen apply THAT Namespace truoc (idempotent)
      # roi moi dry-run phan con lai.
      ns_file="$K8S_DIR/00-namespace.yaml"
      if [ -f "$ns_file" ]; then
        log "  tao truoc namespace (that, idempotent) de --dry-run=server khong bi NamespaceLifecycle tu choi"
        if ! kubectl apply -f "$ns_file" >/dev/null; then
          fail "khong tao duoc namespace tu $ns_file"
          exit 1
        fi
      fi

      log "  co cluster -> kubectl apply --dry-run=server tren ${#manifests[@]} file"
      rc=0
      kubectl apply --dry-run=server "${files_args[@]}" || rc=$?
      if [ "$rc" -ne 0 ]; then
        fail "kubectl apply --dry-run=server that bai (exit ${rc})."
        exit "$rc"
      fi
      log "  OK: ${#manifests[@]} file qua duoc --dry-run=server"
    fi
  fi
else
  # Ba dong duoi day la nguyen van C4.4. KHONG thay bang parse YAML hay bat ky
  # check mot phan nao: parse chi bat loi cu phap, con loi that (sai apiVersion,
  # sai cap securityContext, selector khong khop, probe sai path) deu la YAML
  # hop le hoan hao. In "da parse cu phap" se bi doc thanh "da kiem o muc co
  # ban" -- an toan gia, dung thu C4.4 sinh ra de chong (D2).
  echo "[gates] BO QUA validate k8s: khong co cluster (kubectl cluster-info that bai)."
  echo "[gates] KHONG kiem duoc apiVersion, ten field, selector, hay duong dan probe."
  echo "[gates] Chay lai sau khi bat Kubernetes cua Docker Desktop."
fi


# C4.5 -- kiem o TANG BUNDLE: co readiness va nhanh 503 phai con trong ban build
#
# Vi sao can mot check rieng o day, khi da co 4 gate + C4.2 + C4.3: hoi quy that
# su da xay ra trong slice nay (integration-report.md muc 4.1) TAI HIEN DUOC voi
# CA SAU check kia xanh. Next dong goi `instrumentation.ts` va TUNG route handler
# thanh CAC BUNDLE WEBPACK RIENG, nen `lib/readiness.ts` bi nhan ban vao moi
# bundle. Khi co ready la mot `let` o tam module:
#
#   - bundle `/api/readyz` khong co ai GHI co  -> terser chung minh no luon true
#     -> body bi fold thanh {status:"ready"} va NHANH 503 BI XOA KHOI BAN BUILD
#   - bundle `instrumentation.js` khong co ai DOC co -> `setReady(false)` thanh
#     dead code va cung bi xoa
#
# Vitest KHONG bundle (no dung dung mot module instance) nen 1086 test van xanh;
# `next build` thanh cong; probe van la dynamic nen C4.3 van xanh. Tuc ca sau
# check deu khong the thay. Chi co doc chinh `.next/server/**` moi thay duoc.
#
# Check nay la grep tren marker, CO Y chon marker khong bi minify:
#   - `__eventflowWebReadiness__` la mot KHOA CHUOI tren globalThis, terser
#     khong rename duoc chuoi -> ton tai duoc trong ban production
#   - `not-ready` / `503` la literal cua nhanh loi
# `grep -c setReady` thi VO DUNG o day: ten ham bi minify, nen no ra 0 ca khi
# code hoan toan dung -- dung loai check sinh ra bao dong gia roi bi tat di.
log "check C4.5: co readiness + nhanh 503 phai con trong $WEB_DIR/.next/server/"

C45_SERVER_DIR="$WEB_DIR/.next/server"
C45_READINESS_KEY='__eventflowWebReadiness__'
c45_failed=0

# Tra ve 0 neu $1 chua literal $2. Dung -F: marker la chuoi thuan, de regex
# dien giai `__...__` hay `503` la moi cua cho duong tinh gia.
c45_has() { grep -q -F -- "$2" "$1"; }

# $1=file  $2=marker  $3=hau qua neu marker bien mat
c45_need() {
  if [ ! -f "$1" ]; then
    fail "C4.5: khong thay bundle $1"
    echo "[gates] Buoc build 4/4 o tren phai sinh ra file nay. Neu Next doi cau truc" >&2
    echo "[gates] .next/server/ thi phai CAP NHAT check nay, khong duoc bo di --" >&2
    echo "[gates] mot check tu im lang chinh la che do loi C4.5 sinh ra de chong." >&2
    c45_failed=1
    return
  fi
  if ! c45_has "$1" "$2"; then
    fail "C4.5: '$2' khong con trong $1"
    echo "[gates] $3" >&2
    c45_failed=1
  fi
}

# $1=file  $2=marker  $3=hau qua neu marker XUAT HIEN
c45_forbid() {
  if [ -f "$1" ] && c45_has "$1" "$2"; then
    fail "C4.5: '$2' KHONG duoc co trong $1"
    echo "[gates] $3" >&2
    c45_failed=1
  fi
}

c45_readyz="$C45_SERVER_DIR/app/api/readyz/route.js"
c45_healthz="$C45_SERVER_DIR/app/api/healthz/route.js"
c45_instr="$C45_SERVER_DIR/instrumentation.js"

c45_need "$c45_readyz" "$C45_READINESS_KEY" \
  "Co ready khong con doc tu globalThis trong bundle readyz => no da thanh bien cuc bo cua bundle => setReady(false) tu instrumentation KHONG con tac dung o day."
c45_need "$c45_readyz" "not-ready" \
  "Nhanh 503 da bi optimizer xoa. /api/readyz se LUON tra 200, ke ca luc drain: kubelet khong bao gio rut endpoint va graceful shutdown im lang mat tac dung."
c45_need "$c45_readyz" "503" \
  "Khong con ma 503 nao trong bundle readyz. Cung nguyen nhan voi dong tren."
c45_need "$c45_instr" "$C45_READINESS_KEY" \
  "setReady(false) da bi xoa khoi bundle instrumentation (dead code elimination). Handler SIGTERM se chi cho 5s roi exit, khong bao gio ha readiness."
c45_need "$c45_instr" "SIGTERM" \
  "Khong con dang ky SIGTERM trong ban build => k8s gui SIGTERM, khong ai nghe, pod chet bang default disposition (exit 143) thay vi drain roi exit 0."
c45_need "$c45_instr" "SIGINT" \
  "Khong con dang ky SIGINT. NEXT_MANUAL_SIG_HANDLE=1 da tat handler cua Next cho CA SIGINT, nen Ctrl+C se mat graceful shutdown (exit 130)."

# Bat bien nguoc: liveness KHONG BAO GIO duoc tra 503. Doi cho hai duong dan
# probe la loi khong parser nao bat duoc, va day la mot trong vai dau hieu may
# doc duoc tu ban build.
c45_forbid "$c45_healthz" "503" \
  "Liveness khong bao gio duoc tra 503 (AC-1). Co 503 trong bundle healthz nghia la logic readiness da lot vao liveness -- k8s se RESTART pod dang drain thay vi chi rut endpoint."

if [ "$c45_failed" -ne 0 ]; then
  echo "[gates] C4.5 that bai. Day la tang ma 4 gate + C4.2 + C4.3 deu KHONG thay." >&2
  exit 1
fi
log "  OK: co readiness co trong ca hai bundle, nhanh 503 con nguyen, healthz khong co 503"

log "TAT CA GATE DAT."
