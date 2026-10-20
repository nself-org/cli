#!/usr/bin/env bash
# bump-version.sh — Atomic version bump across all lockstep files.
#
# Usage:
#   ./scripts/bump-version.sh <new-version> [--dry-run] [--homebrew]
#
# <new-version>   Required. Target version like "1.0.14" or "v1.0.14" (v stripped).
# --dry-run       Print what would change; write nothing.
# --homebrew      Phase 2: update homebrew formula after GitHub release exists.
#                 Without this flag, the formula is skipped and a reminder printed.
# --no-goldens    Skip the golden regeneration below (the release tool sets it for the
#                 admin-only lockstep bump, which commits nothing from cli).
#
# Goldens (same run, after the files above): the four version-hash goldens hash
# `.nself/build-version`, so every bump changes them. They are regenerated through
# the tests' own UPDATE_GOLDEN=1 path; the script then checks that only
# `.nself/build-version` lines changed and that the plain tests pass:
#   cli/internal/build/testdata/plan-mode/golden-{dev-minimal,fronted,prod-plugins}.txt
#   cli/internal/reconcile/testdata/fixtures/dev-minimal/golden-tree.txt
# Skipped with a note when the golden tests are absent (fixture trees).
#
# Files updated (11 + 1 optional homebrew) — Flutter SDK removed 2026-06-30 (#159);
# sdk/ts-sdk added 2026-07-06 (was missing, causing @nself/sdk version lag):
#   1.  cli/.github/VERSION
#   2.  cli/internal/version/version.go       Version string = "x.y.z" (type-annotated)
#   3.  cli/sdk/go/doc.go                     const Version = "x.y.z"
#   4.  cli/sdk/ts/package.json               "version": "x.y.z"   (via jq)
#   5.  cli/sdk/ts-sdk/package.json           "version": "x.y.z"   (via jq)
#   6.  cli/sdk/py/pyproject.toml             version = "x.y.z" under [project]
#   7.  admin/package.json                    "version": "x.y.z"   (via jq)
#   8.  admin/src/lib/cli-version.ts          CLI_VERSION = 'x.y.z'
#   9.  admin/Dockerfile                      ARG NSELF_VERSION=x.y.z
#   10. admin/Dockerfile                      ENV ADMIN_VERSION=x.y.z
#   11. admin/Dockerfile                      LABEL org.opencontainers.image.version="x.y.z"
# +12. homebrew-nself/Formula/nself.rb       version + darwin arm64/amd64 sha256
#                                             from release checksums.txt (--homebrew only)
#
# Exit codes:
#   0 — success (all changes applied, or all already at target in idempotent run)
#   1 — usage error, bad semver, missing file, or homebrew release not found
#
# Hard rules:
#   - Atomic writes: all file writes use tmp+rename, cleaned on EXIT
#   - Portable patterns: POSIX BRE only (no GNU \s); jq for JSON files
#   - Idempotent: skip files already at the target version
#   - shellcheck-clean (no echo -e, no ${var,,}, no declare -A)
#
# L-P98-06 fix (2026-05-13):
#   - Replaced GNU \s with POSIX [[:space:]] in all regex
#   - Routed JSON edits through jq to handle nested/contextual "version" fields
#   - Added Go type-annotated form: Version<spaces>string<spaces>=<spaces>"..."
#   - Added scripts/bump-version_test.sh with fixtures for all 11 line shapes

set -euo pipefail

# ── Argument parsing ───────────────────────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
NSELF_ROOT="$(cd "${REPO_ROOT}/.." && pwd)"

DRY_RUN=false
HOMEBREW=false
GOLDENS=true
RAW_VERSION=""

for arg in "$@"; do
  case "${arg}" in
    --dry-run)   DRY_RUN=true ;;
    --homebrew)  HOMEBREW=true ;;
    --no-goldens) GOLDENS=false ;;
    -*)
      printf 'Unknown flag: %s\n' "${arg}" >&2
      printf 'Usage: %s <version> [--dry-run] [--homebrew] [--no-goldens]\n' "$0" >&2
      exit 1
      ;;
    *)
      if [ -n "${RAW_VERSION}" ]; then
        printf 'Unexpected extra argument: %s\n' "${arg}" >&2
        exit 1
      fi
      RAW_VERSION="${arg}"
      ;;
  esac
done

# ── Semver validation ─────────────────────────────────────────────────────────

if [ -z "${RAW_VERSION}" ]; then
  printf 'Usage: %s <version> [--dry-run] [--homebrew] [--no-goldens]\n' "$0" >&2
  exit 1
fi

NEW_VERSION="${RAW_VERSION#v}"

if ! printf '%s' "${NEW_VERSION}" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  printf 'ERROR: Invalid semver: "%s"\n' "${RAW_VERSION}" >&2
  printf '  Expected X.Y.Z (e.g. 1.0.14) or vX.Y.Z (e.g. v1.0.14)\n' >&2
  exit 1
fi

# ── Tool availability checks ──────────────────────────────────────────────────

if ! command -v jq >/dev/null 2>&1; then
  printf 'ERROR: jq is required for JSON file bumps (sdk/ts/package.json, sdk/ts-sdk/package.json, admin/package.json)\n' >&2
  printf '  Install: brew install jq  (macOS)  |  apt-get install jq  (Linux)\n' >&2
  exit 1
fi

# ── File paths ────────────────────────────────────────────────────────────────

VERSION_FILE="${REPO_ROOT}/.github/VERSION"
# The version before this run edits anything: the golden step needs it (see update_goldens).
OLD_VERSION="$(tr -d '[:space:]' < "${VERSION_FILE}" 2>/dev/null || true)"
VERSION_GO="${REPO_ROOT}/internal/version/version.go"
SDK_GO="${REPO_ROOT}/sdk/go/doc.go"
SDK_TS="${REPO_ROOT}/sdk/ts/package.json"
SDK_PY="${REPO_ROOT}/sdk/py/pyproject.toml"
SDK_TS_SDK="${REPO_ROOT}/sdk/ts-sdk/package.json"
# SDK_FLUTTER removed 2026-06-30 (#159, ASI Policy 2)
ADMIN_PKG="${NSELF_ROOT}/admin/package.json"
ADMIN_CLIVER="${NSELF_ROOT}/admin/src/lib/cli-version.ts"
ADMIN_DOCKERFILE="${NSELF_ROOT}/admin/Dockerfile"
HOMEBREW_FORMULA="${NSELF_ROOT}/homebrew-nself/Formula/nself.rb"

# ── Tmp file cleanup ───────────────────────────────────────────────────────────

TMPFILES=()
GOLD_BAK=""
cleanup_tmpfiles() {
  [ -z "${GOLD_BAK}" ] || rm -rf "${GOLD_BAK}"
  for f in "${TMPFILES[@]+"${TMPFILES[@]}"}"; do
    rm -f "${f}"
  done
}
trap cleanup_tmpfiles EXIT

# shellcheck source=bump-version/files.sh
. "${SCRIPT_DIR}/bump-version/files.sh"
# shellcheck source=bump-version/homebrew.sh
. "${SCRIPT_DIR}/bump-version/homebrew.sh"
# shellcheck source=bump-version/goldens.sh
. "${SCRIPT_DIR}/bump-version/goldens.sh"

# ── Main ───────────────────────────────────────────────────────────────────────

printf '\n'
if [ "${DRY_RUN}" = "true" ]; then
  printf 'DRY RUN — no files will be modified\n'
fi
printf 'Target version: %s\n\n' "${NEW_VERSION}"

if [ "${HOMEBREW}" = "true" ]; then
  printf 'Phase 2 — updating Homebrew formula only\n\n'
  update_homebrew_formula
  printf '\nDone. Homebrew formula updated to %s.\n' "${NEW_VERSION}"
  exit 0
fi

printf 'Phase 1 — updating lockstep files and version-hash goldens (Homebrew deferred to phase 2)\n\n'

update_version_file
update_version_go
update_sdk_go_doc
update_sdk_ts_pkg
update_sdk_ts_sdk_pkg
update_json_version "${REPO_ROOT}/sdk/ts-cli-api/package.json"
update_sdk_py_pyproject
update_admin_pkg
update_admin_cliver
update_admin_dockerfile
if [ "${GOLDENS}" = "true" ]; then
  update_goldens
fi

printf '\n'
if [ "${DRY_RUN}" = "true" ]; then
  printf 'Dry run complete — no files modified.\n'
else
  printf 'Version bump complete: all files now at %s\n' "${NEW_VERSION}"
  printf '\nNext steps:\n'
  printf '  1. Review the diff: git diff\n'
  printf '  2. Commit: git add -A && git commit -m "chore: bump version to v%s"\n' "${NEW_VERSION}"
  printf '  3. Push + tag: git tag v%s && git push origin main v%s\n' "${NEW_VERSION}" "${NEW_VERSION}"
  printf '  4. After GitHub release is published, run:\n'
  printf '     ./scripts/bump-version.sh %s --homebrew\n' "${NEW_VERSION}"
  printf '\nReminder: never hand-bump individual files — always use this script.\n'
fi
