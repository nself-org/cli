#!/usr/bin/env bash
# check-version-lockstep.sh
#
# Verifies all lockstep version files carry the same version string.
# Per nSelf PPI Hard Rule: CLI and Admin ship identical version numbers.
# Extended from 2-file check in P98 S98-02; retired Dart SDK entry removed
# 2026-06-30 (#159, ASI Policy 2).
#
# Lockstep files (10 in this repo or adjacent repos + 1 optional homebrew):
#   1.  cli/internal/version/version.go    (Go constant)
#   2.  cli/.github/VERSION                (plain text)
#   3.  cli/sdk/go/doc.go                  (Go const)
#   4.  cli/sdk/ts/package.json            (JSON version field)
#   5.  cli/sdk/py/pyproject.toml          (TOML version field)
#   6.  admin/package.json                 (JSON version field)
#   7.  admin/src/lib/cli-version.ts       (TS constant)
#   8.  admin/Dockerfile                   (ARG NSELF_VERSION)
#   9.  admin/Dockerfile                   (ENV ADMIN_VERSION)
#   10. admin/Dockerfile                   (LABEL image.version)
#   11. homebrew-nself/Formula/nself.rb    (optional — warn only; separate repo)
#
# Usage:
#   bash scripts/check-version-lockstep.sh [--skip-homebrew] [--quiet]
#
# Flags:
#   --skip-homebrew   Do not check homebrew formula (default: warn-only anyway)
#   --quiet           Suppress per-file OK lines; only print summary or errors
#
# Environment overrides:
#   NSELF_REPO_ROOT   Override auto-detection of cli/ root
#   NSELF_ADMIN_ROOT  Override auto-detection of admin/ root
#   NSELF_HB_ROOT     Override auto-detection of homebrew-nself/ root
#
# Exit codes:
#   0  — all required files match
#   1  — one or more required files differ or are missing
#
# Note: homebrew check is warn-only (exit 0 even if it mismatches).
# Use bump-version.sh --homebrew to update it after a GitHub release exists.

set -euo pipefail

# ── Argument parsing ──────────────────────────────────────────────────────────
SKIP_HOMEBREW=0
QUIET=0
for arg in "$@"; do
  case "${arg}" in
    --skip-homebrew) SKIP_HOMEBREW=1 ;;
    --quiet)         QUIET=1 ;;
    --help|-h)
      sed -n '2,30p' "$0" | grep '^#' | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      printf 'Unknown flag: %s\n' "${arg}" >&2
      exit 1
      ;;
  esac
done

# ── Path resolution ───────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLI_ROOT="${NSELF_REPO_ROOT:-$(cd "${SCRIPT_DIR}/.." && pwd)}"
ADMIN_ROOT="${NSELF_ADMIN_ROOT:-$(cd "${CLI_ROOT}/../admin" 2>/dev/null && pwd || true)}"
HB_ROOT="${NSELF_HB_ROOT:-$(cd "${CLI_ROOT}/../homebrew-nself" 2>/dev/null && pwd || true)}"

# ── Helpers ───────────────────────────────────────────────────────────────────
ERRORS=0
WARNINGS=0
REFERENCE_VERSION=""

log_ok() {
  if [ "${QUIET}" -eq 0 ]; then
    printf '  OK  %s == %s\n' "$1" "$2"
  fi
}

log_err() {
  printf '  FAIL  %s  (got %s, expected %s)\n' "$1" "$2" "$3" >&2
  ERRORS=$((ERRORS + 1))
}

log_warn() {
  printf '  WARN  %s  (%s)\n' "$1" "$2"
  WARNINGS=$((WARNINGS + 1))
}

# Set REFERENCE_VERSION on first call; subsequent calls compare against it.
check_version() {
  local label="$1"
  local version="$2"
  local required="${3:-1}"   # 0 = warn-only

  if [ -z "${version}" ]; then
    if [ "${required}" -eq 1 ]; then
      printf '  FAIL  %s  (could not parse version)\n' "${label}" >&2
      ERRORS=$((ERRORS + 1))
    else
      log_warn "${label}" "could not parse version"
    fi
    return
  fi

  if [ -z "${REFERENCE_VERSION}" ]; then
    REFERENCE_VERSION="${version}"
    log_ok "${label}" "${version}"
    return
  fi

  if [ "${version}" = "${REFERENCE_VERSION}" ]; then
    log_ok "${label}" "${version}"
  else
    if [ "${required}" -eq 1 ]; then
      log_err "${label}" "${version}" "${REFERENCE_VERSION}"
    else
      log_warn "${label}" "expected ${REFERENCE_VERSION}, got ${version}"
    fi
  fi
}

# Extract a semver string from a file using a grep pattern.
# $1 = file path, $2 = grep -E pattern to isolate the version line
extract_version_grep() {
  local file="$1"
  local pattern="$2"
  if [ ! -f "${file}" ]; then
    printf ''
    return
  fi
  grep -E "${pattern}" "${file}" \
    | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' \
    | head -1
}

# ── File checks ───────────────────────────────────────────────────────────────
printf 'Checking version lockstep across 12 files...\n'
printf '\n'

# 1. cli/internal/version/version.go
FILE1="${CLI_ROOT}/internal/version/version.go"
if [ -f "${FILE1}" ]; then
  V1=$(grep -E '^\s+Version\s+string\s*=' "${FILE1}" \
        | grep -oE '"[0-9]+\.[0-9]+\.[0-9]+"' | tr -d '"' | head -1)
  check_version "cli/internal/version/version.go" "${V1}" 1
else
  log_err "cli/internal/version/version.go" "(missing)" "${REFERENCE_VERSION:-?}"
fi

# 2. cli/.github/VERSION
FILE2="${CLI_ROOT}/.github/VERSION"
if [ -f "${FILE2}" ]; then
  V2=$(grep -oE '[0-9]+\.[0-9]+\.[0-9]+' "${FILE2}" | head -1)
  check_version "cli/.github/VERSION" "${V2}" 1
else
  log_err "cli/.github/VERSION" "(missing)" "${REFERENCE_VERSION:-?}"
fi

# 3-5. SDK files — WARN-ONLY (independent lifecycle from CLI per P102 W18 strategic decision)
#       SDKs under cli/sdk/{go,ts,py} ship at their own semver cadence
#       (currently v2.0.0). They are NOT lockstep-bound to CLI version. A mismatch
#       logs a WARN, never blocks release. See PPI § SDK lifecycle.

# 3. cli/sdk/go/doc.go (warn-only)
FILE3="${CLI_ROOT}/sdk/go/doc.go"
if [ -f "${FILE3}" ]; then
  V3=$(grep -E 'const Version\s*=' "${FILE3}" \
        | grep -oE '"[0-9]+\.[0-9]+\.[0-9]+"' | tr -d '"' | head -1)
  check_version "cli/sdk/go/doc.go" "${V3}" 0
else
  log_warn "cli/sdk/go/doc.go" "(missing)"
fi

# 4. cli/sdk/ts/package.json (warn-only)
FILE4="${CLI_ROOT}/sdk/ts/package.json"
if [ -f "${FILE4}" ]; then
  if command -v python3 >/dev/null 2>&1; then
    V4=$(python3 -c "import json,sys; d=json.load(open('${FILE4}')); print(d['version'])" 2>/dev/null || true)
  else
    V4=$(grep -E '"version"\s*:' "${FILE4}" \
          | grep -oE '"[0-9]+\.[0-9]+\.[0-9]+"' | tr -d '"' | head -1)
  fi
  check_version "cli/sdk/ts/package.json" "${V4}" 0
else
  log_warn "cli/sdk/ts/package.json" "(missing)"
fi

# 5. cli/sdk/py/pyproject.toml (warn-only)
FILE5="${CLI_ROOT}/sdk/py/pyproject.toml"
if [ -f "${FILE5}" ]; then
  V5=$(grep -E '^version\s*=' "${FILE5}" \
        | grep -oE '"[0-9]+\.[0-9]+\.[0-9]+"' | tr -d '"' | head -1)
  check_version "cli/sdk/py/pyproject.toml" "${V5}" 0
else
  log_warn "cli/sdk/py/pyproject.toml" "(missing)"
fi

# 7. admin/package.json
if [ -n "${ADMIN_ROOT}" ] && [ -f "${ADMIN_ROOT}/package.json" ]; then
  if command -v python3 >/dev/null 2>&1; then
    V7=$(python3 -c "import json,sys; d=json.load(open('${ADMIN_ROOT}/package.json')); print(d['version'])" 2>/dev/null || true)
  else
    V7=$(grep -E '"version"\s*:' "${ADMIN_ROOT}/package.json" \
          | grep -oE '"[0-9]+\.[0-9]+\.[0-9]+"' | tr -d '"' | head -1)
  fi
  check_version "admin/package.json" "${V7}" 1
else
  log_warn "admin/package.json" "not found at ${ADMIN_ROOT:-<undetected>}; set NSELF_ADMIN_ROOT to check"
fi

# 8. admin/src/lib/cli-version.ts
if [ -n "${ADMIN_ROOT}" ] && [ -f "${ADMIN_ROOT}/src/lib/cli-version.ts" ]; then
  V8=$(grep -E "CLI_VERSION\s*=\s*'" "${ADMIN_ROOT}/src/lib/cli-version.ts" \
        | grep -oE "'[0-9]+\.[0-9]+\.[0-9]+'" | tr -d "'" | head -1)
  check_version "admin/src/lib/cli-version.ts" "${V8}" 1
else
  log_warn "admin/src/lib/cli-version.ts" "not found; set NSELF_ADMIN_ROOT"
fi

# 9-11. admin/Dockerfile (three lines in one file)
if [ -n "${ADMIN_ROOT}" ] && [ -f "${ADMIN_ROOT}/Dockerfile" ]; then
  V9=$(grep -E '^ARG NSELF_VERSION=' "${ADMIN_ROOT}/Dockerfile" \
        | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)
  check_version "admin/Dockerfile (ARG NSELF_VERSION)" "${V9}" 1

  V10=$(grep -E '^ENV ADMIN_VERSION=' "${ADMIN_ROOT}/Dockerfile" \
         | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)
  check_version "admin/Dockerfile (ENV ADMIN_VERSION)" "${V10}" 1

  V11=$(grep -E 'org.opencontainers.image.version=' "${ADMIN_ROOT}/Dockerfile" \
         | grep -oE '"[0-9]+\.[0-9]+\.[0-9]+"' | tr -d '"' | head -1)
  check_version "admin/Dockerfile (LABEL image.version)" "${V11}" 1
else
  log_warn "admin/Dockerfile" "not found; set NSELF_ADMIN_ROOT"
fi

# 12. homebrew-nself/Formula/nself.rb  (warn-only — separate repo)
if [ "${SKIP_HOMEBREW}" -eq 0 ]; then
  if [ -n "${HB_ROOT}" ] && [ -f "${HB_ROOT}/Formula/nself.rb" ]; then
    V12=$(grep -E '^\s+version\s+' "${HB_ROOT}/Formula/nself.rb" \
           | grep -oE '"[0-9]+\.[0-9]+\.[0-9]+"' | tr -d '"' | head -1)
    if [ -z "${V12}" ]; then
      # Fall back to url line
      V12=$(grep -E 'url.*v[0-9]+\.[0-9]+\.[0-9]+' "${HB_ROOT}/Formula/nself.rb" \
             | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | tr -d 'v' | head -1)
    fi
    check_version "homebrew-nself/Formula/nself.rb (warn-only)" "${V12}" 0
  else
    printf '  SKIP  homebrew-nself/Formula/nself.rb (repo not found; update via bump-version.sh --homebrew)\n'
  fi
fi

# ── Summary ───────────────────────────────────────────────────────────────────
printf '\n'
if [ "${ERRORS}" -gt 0 ]; then
  printf 'VERSION LOCKSTEP FAILED: %d error(s), %d warning(s)\n' "${ERRORS}" "${WARNINGS}" >&2
  printf '\n' >&2
  printf '  Reference version: %s\n' "${REFERENCE_VERSION:-unknown}" >&2
  printf '  Fix: run  scripts/bump-version.sh <target-version>  to synchronise all files.\n' >&2
  printf '  Then re-run this script to verify.\n' >&2
  exit 1
fi

if [ "${WARNINGS}" -gt 0 ]; then
  printf 'Version lockstep OK (required files): v%s  [%d warning(s)]\n' "${REFERENCE_VERSION:-unknown}" "${WARNINGS}"
else
  printf 'Version lockstep OK: all files == v%s\n' "${REFERENCE_VERSION:-unknown}"
fi
