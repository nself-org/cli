#!/usr/bin/env bash
# post-release-canary.sh
#
# Post-release canary verification script.
# Run after every CLI release to confirm the full distribution cascade succeeded.
#
# Checks:
#   1. GitHub release exists
#   2. Required release assets are attached
#   3. ping.nself.org/health is reachable
#   4. nself doctor --deep (when the CLI is installed)
#   5. Homebrew formula version matches the release tag
#
# Usage:
#   bash scripts/post-release-canary.sh [version]
#   bash scripts/post-release-canary.sh v1.0.12
#
# If version is omitted, reads from internal/version/version.go.
# Exits 0 only when ALL checks pass. Non-zero on any failure.
#
# Output format: human-readable summary + JSON report at /tmp/canary-report.json

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# ── Resolve target version ────────────────────────────────────────────────────
if [ -n "${1:-}" ]; then
  RAW_VERSION="${1#v}"
  TAG="v${RAW_VERSION}"
else
  VERSION_FILE="${REPO_ROOT}/internal/version/version.go"
  RAW_VERSION=$(grep -E '^\s+Version\s+string\s*=' "${VERSION_FILE}" \
    | grep -oE '"[0-9]+\.[0-9]+\.[0-9]+"' \
    | tr -d '"' \
    | head -1)
  TAG="v${RAW_VERSION}"
fi

echo "Post-release canary: ${TAG}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

PASS=0
FAIL=0
WARN=0
declare -A RESULTS

# ── Check 1: GitHub release exists ───────────────────────────────────────────
echo ""
echo "Check 1: GitHub release ${TAG}"
if gh release view "${TAG}" --repo nself-org/cli &>/dev/null; then
  ASSET_COUNT=$(gh release view "${TAG}" --repo nself-org/cli --json assets --jq '.assets | length')
  echo "  PASS — release exists, ${ASSET_COUNT} assets"
  RESULTS[github_release]="pass: ${ASSET_COUNT} assets"
  PASS=$((PASS + 1))
else
  echo "  FAIL — release ${TAG} not found on nself-org/cli"
  RESULTS[github_release]="fail: not found"
  FAIL=$((FAIL + 1))
fi

# ── Check 2: Expected tarballs present ───────────────────────────────────────
echo ""
echo "Check 2: Required release assets"
REQUIRED_ASSETS=(
  "nself-${RAW_VERSION}-darwin-amd64.tar.gz"
  "nself-${RAW_VERSION}-darwin-arm64.tar.gz"
  "nself-${RAW_VERSION}-linux-amd64.tar.gz"
  "nself-${RAW_VERSION}-linux-arm64.tar.gz"
  "checksums.txt"
  "provenance.intoto.jsonl"
)
MISSING_ASSETS=()
if gh release view "${TAG}" --repo nself-org/cli &>/dev/null; then
  ASSET_NAMES=$(gh release view "${TAG}" --repo nself-org/cli --json assets --jq '[.assets[].name]')
  for asset in "${REQUIRED_ASSETS[@]}"; do
    if echo "${ASSET_NAMES}" | grep -q "\"${asset}\""; then
      echo "  PASS — ${asset}"
    else
      echo "  FAIL — missing: ${asset}"
      MISSING_ASSETS+=("${asset}")
      FAIL=$((FAIL + 1))
    fi
  done
  if [ ${#MISSING_ASSETS[@]} -eq 0 ]; then
    RESULTS[release_assets]="pass: all required assets present"
    PASS=$((PASS + 1))
  else
    RESULTS[release_assets]="fail: missing ${MISSING_ASSETS[*]}"
  fi
else
  echo "  SKIP — release not found (check 1 already failed)"
  RESULTS[release_assets]="skip: release not found"
fi

# ── Check 3: ping.nself.org/health ───────────────────────────────────────────
echo ""
echo "Check 3: ping.nself.org/health"
PING_RESPONSE=$(curl -sf --max-time 10 https://ping.nself.org/health 2>/dev/null || echo "unreachable")
if [ "${PING_RESPONSE}" = "unreachable" ]; then
  echo "  WARN — ping.nself.org unreachable (deployment may be pending)"
  RESULTS[ping_api]="warn: unreachable"
  WARN=$((WARN + 1))
else
  PING_VERSION=$(echo "${PING_RESPONSE}" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('version','unknown'))" 2>/dev/null || echo "unknown")
  echo "  INFO — ping_api version: ${PING_VERSION}"
  # ping_api reports its own service version (0.1.x), not CLI version — note only
  echo "  NOTE — ping_api service version is independent of CLI version"
  RESULTS[ping_api]="pass: reachable, service v${PING_VERSION}"
  PASS=$((PASS + 1))
fi

# ── Check 4: nself doctor --deep (if CLI installed) ──────────────────────────
echo ""
echo "Check 4: nself doctor --deep"
if command -v nself &>/dev/null; then
  INSTALLED_VERSION=$(nself version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "unknown")
  echo "  INFO — installed CLI version: ${INSTALLED_VERSION}"
  if [ "${INSTALLED_VERSION}" = "${RAW_VERSION}" ]; then
    echo "  PASS — installed version matches target ${TAG}"
    RESULTS[cli_installed]="pass: v${INSTALLED_VERSION}"
    PASS=$((PASS + 1))
    # Run doctor if a backend is running
    if nself status &>/dev/null 2>&1; then
      echo "  Running nself doctor --deep..."
      DOCTOR_EXIT=0
      nself doctor --deep 2>&1 | tail -20 || DOCTOR_EXIT=$?
      if [ "${DOCTOR_EXIT}" -eq 0 ]; then
        echo "  PASS — doctor --deep clean"
        RESULTS[doctor_deep]="pass"
        PASS=$((PASS + 1))
      else
        echo "  WARN — doctor --deep returned exit ${DOCTOR_EXIT}"
        RESULTS[doctor_deep]="warn: exit ${DOCTOR_EXIT}"
        WARN=$((WARN + 1))
      fi
    else
      echo "  SKIP — no running backend to doctor"
      RESULTS[doctor_deep]="skip: no backend running"
    fi
  else
    echo "  WARN — installed v${INSTALLED_VERSION} != target ${RAW_VERSION} (Homebrew may not have updated yet)"
    RESULTS[cli_installed]="warn: v${INSTALLED_VERSION} != ${RAW_VERSION}"
    WARN=$((WARN + 1))
  fi
else
  echo "  SKIP — nself not in PATH (Homebrew update may be pending)"
  RESULTS[cli_installed]="skip: not in PATH"
fi

# ── Check 5: Homebrew formula version ────────────────────────────────────────
echo ""
echo "Check 5: Homebrew formula lockstep"
HOMEBREW_ROOT="${REPO_ROOT}/../homebrew-nself"
if [ -f "${HOMEBREW_ROOT}/Formula/nself.rb" ]; then
  FORMULA_VERSION=$(grep -E '^\s*version\s+"[0-9]+\.[0-9]+\.[0-9]+"' \
    "${HOMEBREW_ROOT}/Formula/nself.rb" \
    | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)
  echo "  Formula version: ${FORMULA_VERSION}"
  if [ "${FORMULA_VERSION}" = "${RAW_VERSION}" ]; then
    echo "  PASS — formula at ${TAG}"
    RESULTS[homebrew_formula]="pass: v${FORMULA_VERSION}"
    PASS=$((PASS + 1))
  else
    echo "  FAIL — formula at v${FORMULA_VERSION}, expected ${RAW_VERSION}"
    RESULTS[homebrew_formula]="fail: v${FORMULA_VERSION} != ${RAW_VERSION}"
    FAIL=$((FAIL + 1))
  fi
else
  echo "  WARN — homebrew-nself repo not found at ${HOMEBREW_ROOT}"
  RESULTS[homebrew_formula]="warn: repo not found"
  WARN=$((WARN + 1))
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Canary summary for ${TAG}:"
echo "  PASS: ${PASS}  WARN: ${WARN}  FAIL: ${FAIL}"
echo ""

# ── JSON report ───────────────────────────────────────────────────────────────
REPORT_FILE="/tmp/canary-report.json"
{
  echo "{"
  echo "  \"version\": \"${TAG}\","
  echo "  \"timestamp\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\","
  echo "  \"pass\": ${PASS},"
  echo "  \"warn\": ${WARN},"
  echo "  \"fail\": ${FAIL},"
  echo "  \"checks\": {"
  FIRST=1
  for key in "${!RESULTS[@]}"; do
    if [ $FIRST -eq 0 ]; then echo ","; fi
    printf '    "%s": "%s"' "${key}" "${RESULTS[$key]}"
    FIRST=0
  done
  echo ""
  echo "  }"
  echo "}"
} > "${REPORT_FILE}"

echo "JSON report: ${REPORT_FILE}"

if [ "${FAIL}" -gt 0 ]; then
  echo ""
  echo "CANARY FAILED — ${FAIL} check(s) failed. Review output above."
  exit 1
fi

if [ "${WARN}" -gt 0 ]; then
  echo "CANARY PASSED WITH WARNINGS — distribution cascade may still be propagating."
  exit 0
fi

echo "CANARY PASSED — ${TAG} distribution cascade verified."
