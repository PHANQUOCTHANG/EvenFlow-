#!/usr/bin/env bash
#
# Offline validation of the EventFlow Kubernetes manifests (spec docs/workflow/k8s-all/spec.md AC-2, AC-24;
# interface docs/workflow/k8s-all/policy-contract.md).  No cluster and no Docker needed: tools are pinned in
# deploy/k8s/tools.env and built with Go (network only on the first run; modules and schemas are cached).
#
#   bash deploy/k8s/validate.sh [--render-dir DIR] [--skip-render] [--skip-kubeconform] [--skip-policy]
#                               [--render-only] [--fixtures-only]
#
#   1. read tools.env, print tool versions
#   2. render overlays/local and overlays/staging with the pinned kustomize  -> DIR/<overlay>.yaml
#   3. kubeconform -strict -summary -kubernetes-version $K8S_SCHEMA_VERSION -cache <dir>   (AC-2)
#   4. yq ea -o=json '[.]' DIR/<o>.yaml | check_policy.py --overlay <o>                      (AC-4..AC-23, AC-32)
#   5. check_policy.py --dockerfiles deploy/docker                                            (AC-13)
#   6. bash policy/test_policy.sh --fixtures-only                                             (AC-25 self-test)
#
#   --render-dir DIR    where rendered YAML/JSON go (default: a temp dir, removed on exit)
#   --skip-render       reuse DIR/<o>.yaml from an earlier --render-only run (needs --render-dir)
#   --skip-kubeconform  skip step 3
#   --skip-policy       skip steps 4-6
#   --render-only       steps 1-2 only
#   --fixtures-only     steps 1 and 6 only (policy self-test on the fixtures, no render)
#
# Env: TOOLS=go (default; `go install <module>@<version>` into a version-keyed cache dir) or TOOLS=docker
#      (images at the same versions).  KUBECONFORM_CACHE overrides the schema cache dir, K8S_TOOLS_DIR the
#      binary cache dir.
# Exit 0 = every step that ran passed; 1 = a violation or a failed step (each prints a FAIL line); 2 = usage.
set -u
export LC_ALL=C

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
K8S="$ROOT/deploy/k8s"
POL="$K8S/policy"
OVERLAYS="local staging"

RENDER_DIR=""; SKIP_RENDER=0; SKIP_KC=0; SKIP_POLICY=0; RENDER_ONLY=0; FIXTURES_ONLY=0
usage() { sed -n '7,8p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//' >&2; }
while [ $# -gt 0 ]; do
  case "$1" in
    --render-dir) [ $# -ge 2 ] || { echo "ERROR --render-dir needs a value" >&2; usage; exit 2; }; RENDER_DIR="$2"; shift ;;
    --render-dir=*) RENDER_DIR="${1#*=}" ;;
    --skip-render) SKIP_RENDER=1 ;;
    --skip-kubeconform) SKIP_KC=1 ;;
    --skip-policy) SKIP_POLICY=1 ;;
    --render-only) RENDER_ONLY=1 ;;
    --fixtures-only) FIXTURES_ONLY=1 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "ERROR unknown argument: $1" >&2; usage; exit 2 ;;
  esac
  shift
done
if [ "$SKIP_RENDER" -eq 1 ] && [ -z "$RENDER_DIR" ]; then echo "ERROR --skip-render needs --render-dir" >&2; exit 2; fi
if [ "$RENDER_ONLY" -eq 1 ] && [ "$FIXTURES_ONLY" -eq 1 ]; then echo "ERROR --render-only and --fixtures-only exclude each other" >&2; exit 2; fi

# ---------------------------------------------------------------- 1. tools
# tools.env is read line by line (CR stripped) so a CRLF checkout cannot break the pins.
if [ ! -f "$K8S/tools.env" ]; then echo "FAIL validate: deploy/k8s/tools.env missing" >&2; exit 1; fi
while IFS='=' read -r k v; do
  k="$(printf '%s' "$k" | tr -d '\r[:space:]')"; v="$(printf '%s' "${v%%#*}" | tr -d '\r[:space:]')"
  case "$k" in KUSTOMIZE_VERSION|KUBECONFORM_VERSION|YQ_VERSION|K8S_SCHEMA_VERSION) printf -v "$k" '%s' "$v" ;; esac
done < "$K8S/tools.env"
for k in KUSTOMIZE_VERSION KUBECONFORM_VERSION YQ_VERSION K8S_SCHEMA_VERSION; do
  [ -n "${!k:-}" ] || { echo "FAIL validate: $k not set in deploy/k8s/tools.env" >&2; exit 1; }
done

TOOLS="${TOOLS:-go}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"
[ -d "/c/Program Files/Go/bin" ] && PATH="$PATH:/c/Program Files/Go/bin"
PY="$(command -v python3 || command -v python || true)"
winpath() { if command -v cygpath >/dev/null 2>&1; then cygpath -m "$1"; else printf '%s' "$1"; fi; }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
if [ -z "$RENDER_DIR" ]; then RENDER_DIR="$TMP/render"; fi
mkdir -p "$RENDER_DIR"
RENDER_DIR="$(cd "$RENDER_DIR" && pwd)"
CACHE_BASE="${XDG_CACHE_HOME:-$HOME/.cache}/eventflow-k8s"
TOOLS_DIR="${K8S_TOOLS_DIR:-$CACHE_BASE/bin}"
KC_CACHE="${KUBECONFORM_CACHE:-$CACHE_BASE/kubeconform}"

FAILS=0
step_fail() { FAILS=$((FAILS + 1)); echo "FAIL $*"; }

# gobin NAME MODULE VERSION: echo a binary built from MODULE@VERSION, cached per version
gobin() {
  local name="$1" mod="$2" ver="$3" dir exe
  dir="$TOOLS_DIR/$name-$ver"
  for exe in "$dir/$name" "$dir/$name.exe"; do [ -x "$exe" ] && { echo "$exe"; return 0; }; done
  command -v go >/dev/null 2>&1 || { echo "go not found (needed for TOOLS=go)" >&2; return 1; }
  mkdir -p "$dir"
  if ! GOBIN="$(winpath "$dir")" go install "$mod@$ver" >"$TMP/go-$name.log" 2>&1; then
    echo "go install $mod@$ver failed: $(tail -n 3 "$TMP/go-$name.log" | tr '\n' ' ')" >&2; return 1
  fi
  for exe in "$dir/$name" "$dir/$name.exe"; do [ -x "$exe" ] && { echo "$exe"; return 0; }; done
  echo "go install $mod@$ver produced no binary" >&2; return 1
}

MOUNT="$(pwd -W 2>/dev/null || pwd)"
if [ "$TOOLS" = "docker" ]; then
  export MSYS_NO_PATHCONV=1
  command -v docker >/dev/null 2>&1 || { echo "FAIL validate: TOOLS=docker but docker CLI missing"; exit 1; }
fi
# run_tool NAME ARGS... (paths in ARGS must be relative to the repo root in docker mode)
run_tool() {
  local name="$1"; shift
  if [ "$TOOLS" = "docker" ]; then
    case "$name" in
      kustomize) docker run --rm -v "$(winpath "$ROOT"):/w" -w /w "registry.k8s.io/kustomize/kustomize:$KUSTOMIZE_VERSION" "$@" ;;
      kubeconform) docker run --rm -i -v "$(winpath "$ROOT"):/w" -v "$(winpath "$KC_CACHE"):/cache" -w /w "ghcr.io/yannh/kubeconform:$KUBECONFORM_VERSION" "$@" ;;
      yq) docker run --rm -i -v "$(winpath "$ROOT"):/w" -w /w "mikefarah/yq:${YQ_VERSION#v}" "$@" ;;
    esac
  else
    case "$name" in
      kustomize) "$KUSTOMIZE" "$@" ;;
      kubeconform) "$KUBECONFORM" "$@" ;;
      yq) "$YQ" "$@" ;;
    esac
  fi
}

NEED_RENDER=1; NEED_KC=1; NEED_POLICY=1; NEED_SELFTEST=1
[ "$SKIP_RENDER" -eq 1 ] && NEED_RENDER=0
[ "$SKIP_KC" -eq 1 ] && NEED_KC=0
[ "$SKIP_POLICY" -eq 1 ] && { NEED_POLICY=0; NEED_SELFTEST=0; }
[ "$RENDER_ONLY" -eq 1 ] && { NEED_KC=0; NEED_POLICY=0; NEED_SELFTEST=0; }
[ "$FIXTURES_ONLY" -eq 1 ] && { NEED_RENDER=0; NEED_KC=0; NEED_POLICY=0; }

if [ "$TOOLS" = "go" ]; then
  KUSTOMIZE=""; KUBECONFORM=""; YQ=""
  if [ "$NEED_RENDER" -eq 1 ]; then
    KUSTOMIZE="$(gobin kustomize sigs.k8s.io/kustomize/kustomize/v5 "$KUSTOMIZE_VERSION")" || { echo "FAIL validate: no kustomize"; exit 1; }
  fi
  if [ "$NEED_KC" -eq 1 ]; then
    KUBECONFORM="$(gobin kubeconform github.com/yannh/kubeconform/cmd/kubeconform "$KUBECONFORM_VERSION")" || { echo "FAIL validate: no kubeconform"; exit 1; }
  fi
  if [ "$NEED_POLICY" -eq 1 ]; then
    YQ="$(gobin yq github.com/mikefarah/yq/v4 "$YQ_VERSION")" || { echo "FAIL validate: no yq"; exit 1; }
  fi
elif [ "$TOOLS" != "docker" ]; then
  echo "ERROR TOOLS must be go or docker, got $TOOLS" >&2; exit 2
fi

echo "== tools (TOOLS=$TOOLS)"
echo "pins: kustomize=$KUSTOMIZE_VERSION kubeconform=$KUBECONFORM_VERSION yq=$YQ_VERSION k8s-schema=$K8S_SCHEMA_VERSION"
[ "$TOOLS" = "go" ] && command -v go >/dev/null 2>&1 && echo "go: $(go version)"
[ -n "$PY" ] && echo "python: $("$PY" --version 2>&1)"
[ "$NEED_RENDER" -eq 1 ] && echo "kustomize: $(run_tool kustomize version 2>&1 | head -n 1)"
[ "$NEED_KC" -eq 1 ] && echo "kubeconform: $(run_tool kubeconform -v 2>&1 | head -n 1)"
[ "$NEED_POLICY" -eq 1 ] && echo "yq: $(run_tool yq --version 2>&1 | head -n 1)"
if [ -z "$PY" ] && { [ "$NEED_POLICY" -eq 1 ] || [ "$NEED_SELFTEST" -eq 1 ]; }; then echo "FAIL validate: python3 not found"; exit 1; fi

# ---------------------------------------------------------------- 2. render
relrender() { # path of DIR/<file> as seen by the tool (relative to ROOT in docker mode)
  if [ "$TOOLS" = "docker" ]; then
    case "$RENDER_DIR/" in "$ROOT"/*) printf '%s' "${RENDER_DIR#"$ROOT"/}/$1" ;; *) printf '%s' "$RENDER_DIR/$1" ;; esac
  else winpath "$RENDER_DIR/$1"; fi
}
if [ "$NEED_RENDER" -eq 1 ]; then
  echo "== render"
  for o in $OVERLAYS; do
    if [ ! -d "$K8S/overlays/$o" ]; then step_fail "render: deploy/k8s/overlays/$o does not exist"; continue; fi
    if (cd "$ROOT" && run_tool kustomize build "deploy/k8s/overlays/$o") >"$RENDER_DIR/$o.yaml" 2>"$TMP/render-$o.err"; then
      echo "rendered overlays/$o -> $RENDER_DIR/$o.yaml ($(grep -c '^kind: ' "$RENDER_DIR/$o.yaml") objects)"
    else
      rm -f "$RENDER_DIR/$o.yaml"
      step_fail "render: kustomize build overlays/$o: $(head -c 600 "$TMP/render-$o.err" | tr '\n' ' ')"
    fi
  done
fi

if [ "$RENDER_ONLY" -eq 0 ]; then
  # ---------------------------------------------------------------- 3. kubeconform
  if [ "$NEED_KC" -eq 1 ]; then
    echo "== kubeconform -strict (k8s $K8S_SCHEMA_VERSION)"
    mkdir -p "$KC_CACHE"
    kc_cache="$(winpath "$KC_CACHE")"; [ "$TOOLS" = "docker" ] && kc_cache=/cache
    for o in $OVERLAYS; do
      if [ ! -f "$RENDER_DIR/$o.yaml" ]; then step_fail "kubeconform: no render of $o"; continue; fi
      if (cd "$ROOT" && run_tool kubeconform -strict -summary -kubernetes-version "$K8S_SCHEMA_VERSION" \
            -cache "$kc_cache" "$(relrender "$o.yaml")") >"$TMP/kc-$o.out" 2>&1; then
        echo "kubeconform $o: $(tail -n 1 "$TMP/kc-$o.out")"
      else
        cat "$TMP/kc-$o.out"
        step_fail "kubeconform: overlays/$o is not schema-valid"
      fi
    done
  fi

  # ---------------------------------------------------------------- 4. policy on the renders
  if [ "$NEED_POLICY" -eq 1 ]; then
    echo "== policy (check_policy.py)"
    for o in $OVERLAYS; do
      if [ ! -f "$RENDER_DIR/$o.yaml" ]; then step_fail "policy: no render of $o"; continue; fi
      if ! (cd "$ROOT" && run_tool yq ea -o=json '[.]' "$(relrender "$o.yaml")") >"$RENDER_DIR/$o.json" 2>"$TMP/yq-$o.err"; then
        step_fail "policy: yq failed on $o: $(head -c 300 "$TMP/yq-$o.err" | tr '\n' ' ')"; continue
      fi
      "$PY" -I "$(winpath "$POL/check_policy.py")" --overlay "$o" --input "$(winpath "$RENDER_DIR/$o.json")"
      rc=$?
      [ "$rc" -eq 0 ] || step_fail "policy: overlays/$o (exit $rc)"
    done

    # ---------------------------------------------------------------- 5. AC-13 numeric USER
    echo "== AC-13 Dockerfiles"
    "$PY" -I "$(winpath "$POL/check_policy.py")" --dockerfiles "$(winpath "$ROOT/deploy/docker")"
    rc=$?
    [ "$rc" -eq 0 ] || step_fail "policy: deploy/docker (exit $rc)"
  fi
fi

# ---------------------------------------------------------------- 6. self-test
if [ "$NEED_SELFTEST" -eq 1 ]; then
  echo "== policy self-test (test_policy.sh --fixtures-only)"
  bash "$POL/test_policy.sh" --fixtures-only >"$TMP/selftest.out" 2>&1
  rc=$?
  grep -v '^PASS ' "$TMP/selftest.out"
  [ "$rc" -eq 0 ] || step_fail "self-test: test_policy.sh --fixtures-only (exit $rc)"
fi

if [ "$FAILS" -eq 0 ]; then echo "validate: OK"; exit 0; fi
echo "validate: $FAILS step(s) failed"
exit 1
