#!/usr/bin/env bash
# Purpose: run (or, with --plan, print) the whole cli release chain S1..S10 from the
#          owner machine with the ambient `gh` session. No token is read or stored.
# Usage:   release.sh --version X.Y.Z [--plan] [--from S<n>] [--yes]
# Inputs:  --version  target release, X.Y.Z (no v)
#          --plan     print `S<n> <step-id> <repo> <command>` rows; only `gh api` GETs
#          --from     resume at step S<n> (steps recover their inputs from state or GitHub)
#          RELEASE_WAIT_MINUTES (env, default 45): PR-check and license-audit wait budget.
#          --yes      skip release.sh's own "Run S<n>? [y/N]" prompt only; admin-merge.sh
#                     always asks the owner at the terminal
# Outputs: progress on stdout; errors on stderr. Exit 0 ok, 1 a step failed (message
#          names the step and the recovery), 2 refused input (no/invalid --version, not a
#          TTY on a real run, version older than the latest release, or a fresh start
#          when the version is the latest or its tag exists).
# Constraints: bash 3.2; PRs on protected repos merge only through admin-merge.sh after
#          required checks are green; the tag is annotated and pushed by S4 after the denylist pre-checks.
#          Runbook: .github/wiki/Release-Process.md. State: $TMPDIR/nself-release-<v>.state
# YES, VERSION, TAG and friends are read by the sourced lib files.
# shellcheck disable=SC2034
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ADMIN_MERGE="$SCRIPT_DIR/../admin-merge.sh"
# shellcheck disable=SC1091
. "$SCRIPT_DIR/lib/common.sh"
# shellcheck disable=SC1091
. "$SCRIPT_DIR/lib/steps1.sh"
# shellcheck disable=SC1091
. "$SCRIPT_DIR/lib/steps2.sh"
# shellcheck disable=SC1091
. "$SCRIPT_DIR/lib/steps3.sh"
for _lib in checks hashes tag publish sdk; do
  # shellcheck disable=SC1090,SC1091
  . "$SCRIPT_DIR/lib/$_lib.sh"
done

VERSION=""; PLAN=0; YES=0; FROM_N=1

usage() {
  sed -n '2,/^set -euo/p' "$0" | sed -n 's/^# \{0,1\}//p' | grep -v '^set -euo' >&2
}

while [ $# -gt 0 ]; do
  case "$1" in
    --version) VERSION="${2:-}"; shift; [ $# -gt 0 ] && shift ;;
    --plan) PLAN=1; shift ;;
    --yes) YES=1; shift ;;
    --from)
      case "${2:-}" in
        S[1-9]|S10) FROM_N="${2#S}" ;;
        *) err "--from needs S1..S10"; exit 2 ;;
      esac
      shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) err "unknown argument: $1"; usage; exit 2 ;;
  esac
done

if ! printf '%s' "$VERSION" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  err "--version X.Y.Z is required (got '${VERSION}')"
  exit 2
fi
TAG="v$VERSION"
WORK="$(printf '%s' "${TMPDIR:-/tmp}" | sed 's:/*$::')/nself-release-$VERSION"
STATE="$WORK.state"

# A real run needs the owner at the terminal: every merge is answered there.
if [ "$PLAN" = 0 ] && [ ! -t 0 ]; then
  err "stdin is not a terminal; a real run needs the owner at the keyboard (use --plan to preview)"
  exit 2
fi
command -v gh >/dev/null 2>&1 || { err "gh is not installed"; exit 1; }
if [ "$PLAN" = 0 ]; then
  [ -f "$ADMIN_MERGE" ] || { err "admin-merge.sh missing next to scripts/release/"; exit 1; }
fi

# Latest guard (GET only). Older than the latest release is always refused.
# A fresh start (no --from, or S1..S3) also needs version > latest and no tag.
# S4 may run with the tag present (it checks the tag sits on the merged commit and
# never moves it). Equal to latest is allowed only from S5, when the release is in
# flight or published and S5..S10 are finishing it.
if gh_try api repos/nself-org/cli/releases/latest --jq .tag_name; then
  latest=$(printf '%s\n' "$GH_OUT" | head -1)
  latest="${latest#v}"
  if ! printf '%s' "$latest" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+'; then
    err "unexpected latest release tag '$latest'"; exit 1
  fi
elif is_404; then
  latest=""
else
  err "cannot read the latest cli release: $GH_ERR"; exit 1
fi
if [ -n "$latest" ] && ver_gt "$latest" "$VERSION"; then
  err "version $VERSION is older than the latest release v$latest"; exit 2
fi
if [ "$FROM_N" -le 4 ] && [ -n "$latest" ] && [ "$latest" = "$VERSION" ]; then
  err "version $VERSION is already the latest release; resume a finished release with --from S5 or later"; exit 2
fi
if [ "$FROM_N" -le 3 ] && tag_exists; then
  err "tag $TAG already exists; resume with --from S4 or later"; exit 2
fi

run_step() {
  CUR_S="S$1"
  case "$1" in
    1) CUR_ID=preflight; step_preflight ;;
    2) CUR_ID=admin-lockstep; step_admin_lockstep ;;
    3) CUR_ID=cli-version; step_cli_version ;;
    4) CUR_ID=tag; step_tag ;;
    5) CUR_ID=build-wait; step_build_wait ;;
    6) CUR_ID=homebrew-formula; step_homebrew_formula ;;
    7) CUR_ID=release-wait; step_release_wait ;;
    8) CUR_ID=docker-verify; step_docker_verify ;;
    9) CUR_ID=admin-image; step_admin_image ;;
    10) CUR_ID=sdk-publish; step_sdk_publish ;;
  esac
}

if [ "$PLAN" = 1 ]; then err "plan for $TAG from S$FROM_N (read-only: gh api GETs only)"; fi
n=$FROM_N
while [ "$n" -le 10 ]; do
  run_step "$n"
  n=$((n + 1))
done

if [ "$PLAN" = 0 ]; then
  say "release $TAG: S$FROM_N..S10 complete"
  case "$WORK" in */nself-release-"$VERSION") rm -rf "$WORK" "$STATE" ;; esac
fi
