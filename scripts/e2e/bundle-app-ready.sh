#!/usr/bin/env bash
# Purpose: prove `nself bundle install <bundle>` reaches APP-READY on a clean
#          project (P7-PLUG-19): files installed, ledger written, reconciled,
#          started, boot migrations applied, tables tracked, services healthy.
# Usage:   bundle-app-ready.sh --bundle NAME [--local-licence]
#            --bundle NAME     bundle to install (task is free and needs no licence)
#            --local-licence   paid bundles: build the CLI with the nself_devkeys tag
#                              and a throwaway Ed25519 key (never the owner key).
#                              Needs $NSELF/web (ping_api); exits 3 when it is absent.
# Env:     E2E_WORK  work directory (default: a fresh mktemp dir). When this runs
#                    inside a container that talks to the host Docker socket, the
#                    directory must exist at the same path on the host, because
#                    compose bind-mounts it: mount it identically and set E2E_WORK.
#          NSELF     checkout root holding web/ (only for --local-licence)
# Outputs: the install output, then "PASS" or "FAIL: <reason>" on stdout.
# Exit:    0 PASS, 1 FAIL, 2 usage, 3 precondition not met (no Docker, no web/).
# Constraints: bash 3.2 compatible. Runs sequentially, one bundle at a time. The
#          stack is stopped and the work directory removed on exit. No owner
#          licence key is read, printed or written.
set -euo pipefail

usage() {
  echo "usage: bundle-app-ready.sh --bundle NAME [--local-licence]" >&2
}

BUNDLE=""
LOCAL_LICENCE=0
while [ $# -gt 0 ]; do
  case "$1" in
    --bundle)
      [ $# -ge 2 ] || { usage; exit 2; }
      BUNDLE="$2"
      shift 2
      ;;
    --local-licence)
      LOCAL_LICENCE=1
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done
[ -n "$BUNDLE" ] || { usage; exit 2; }

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

if ! command -v docker >/dev/null 2>&1; then
  # The golang image has no Docker CLI; install it when we can (root + apt).
  if [ "$(id -u)" = "0" ] && command -v apt-get >/dev/null 2>&1; then
    apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq docker.io >/dev/null 2>&1 || true
  fi
fi
if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
  echo "bundle-app-ready: Docker is not available in this environment" >&2
  exit 3
fi

if [ "$LOCAL_LICENCE" = "1" ]; then
  if [ -z "${NSELF:-}" ] || [ ! -f "$NSELF/web/backend/services/ping_api/package.json" ]; then
    echo "bundle-app-ready: --local-licence is not available in this environment (no \$NSELF/web ping_api)" >&2
    exit 3
  fi
fi

WORK="${E2E_WORK:-$(mktemp -d "${TMPDIR:-/tmp}/bundle-app-ready.XXXXXX")}"
mkdir -p "$WORK/plugins" "$WORK/project"
CLEAN_WORK=1
[ -n "${E2E_WORK:-}" ] && CLEAN_WORK=0
LOG="$WORK/install.log"

teardown() {
  if [ -f "$WORK/project/.env" ] && [ -x "$WORK/nself" ]; then
    (cd "$WORK/project" && NSELF_PLUGIN_DIR="$WORK/plugins" "$WORK/nself" stop >/dev/null 2>&1) || true
  fi
  if [ "$CLEAN_WORK" = "1" ]; then
    rm -rf "$WORK"
  fi
}
trap teardown EXIT

fail() {
  echo "FAIL: $1"
  [ -f "$LOG" ] && tail -n 40 "$LOG"
  exit 1
}

TAGS=""
if [ "$LOCAL_LICENCE" = "1" ]; then
  TAGS="-tags nself_devkeys"
fi
# shellcheck disable=SC2086 # TAGS is empty or one flag pair
(cd "$REPO_ROOT" && CGO_ENABLED=0 go build -mod=vendor $TAGS -o "$WORK/nself" ./cmd/nself) || fail "go build"

if [ "$LOCAL_LICENCE" = "1" ]; then
  # Minting a licence needs ping_api running against a database with the
  # throwaway key; that harness is proven in P7-PLUG-30, not here.
  echo "bundle-app-ready: --local-licence build done, but starting ping_api and minting a licence is not available in this environment" >&2
  exit 3
fi

cd "$WORK/project"
export NSELF_PLUGIN_DIR="$WORK/plugins"
export HOME="$WORK/home"
mkdir -p "$HOME"

"$WORK/nself" init --non-interactive >"$WORK/init.log" 2>&1 || {
  cat "$WORK/init.log"
  fail "nself init"
}

set +e
NSELF_V15=1 "$WORK/nself" bundle install "$BUNDLE" --yes >"$LOG" 2>&1
code=$?
set -e
cat "$LOG"

[ "$code" = "0" ] || fail "bundle install exited $code"
grep -q "APP-READY" "$LOG" || fail "output lacks APP-READY"
if grep -q "NOT APP-READY" "$LOG"; then
  fail "bundle is not app-ready"
fi
echo "PASS"
