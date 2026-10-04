#!/usr/bin/env bash
# generate-f17_test.sh — tests for generate-f17.sh.
# Usage: bash scripts/sport/generate-f17_test.sh
# Bash 3.2 compatible.

set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
GEN="${HERE}/generate-f17.sh"
CLI_ROOT="$(cd "${HERE}/../.." && pwd)"
VERSION="$(tr -d '[:space:]' < "${CLI_ROOT}/.github/VERSION")"
T="$(mktemp -d "${TMPDIR:-/tmp}/f17-test.XXXXXX")"
trap 'rm -rf "${T}"' EXIT

PASS=0
FAIL=0
ok()  { PASS=$((PASS + 1)); printf '  ok   %s\n' "$1"; }
bad() { FAIL=$((FAIL + 1)); printf '  FAIL %s\n' "$1"; }

# run <nself-root> <out-file>: sets RC and OUT (env vars set per call).
run() {
  RC=0
  OUT="$(F17_NSELF_ROOT="$1" F17_OUT_FILE="$2" bash "${GEN}" 2>&1)" || RC=$?
}

# A root with a one-plugin free registry.
GOOD="${T}/good"
mkdir -p "${GOOD}/plugins" "${T}/out"
printf '{"plugins":{"demo":{"status":"stable","minNselfVersion":"1.0.0"}}}\n' > "${GOOD}/plugins/registry.json"

# 1. Happy path names the version and lists the plugin.
run "${GOOD}" "${T}/out/f17.md"
if [ "${RC}" -eq 0 ] && printf '%s' "${OUT}" | grep -qF "v${VERSION}" \
  && grep -qF "## v${VERSION}" "${T}/out/f17.md" && grep -qF 'demo' "${T}/out/f17.md"; then
  ok "generates with a registry and names v${VERSION}"
else bad "happy path (rc=${RC}): ${OUT}"; fi

# 2. No registries at all: non-zero, no output file.
EMPTY="${T}/empty"
mkdir -p "${EMPTY}"
run "${EMPTY}" "${T}/out/none.md"
if [ "${RC}" -ne 0 ] && [ ! -e "${T}/out/none.md" ] && printf '%s' "${OUT}" | grep -qF 'no plugin rows'; then
  ok "missing registries fail closed, no file written"
else bad "missing registries (rc=${RC}): ${OUT}"; fi

# 3. Registry present but empty plugin set: non-zero.
EMPTYREG="${T}/emptyreg"
mkdir -p "${EMPTYREG}/plugins"
printf '{"plugins":{}}\n' > "${EMPTYREG}/plugins/registry.json"
run "${EMPTYREG}" "${T}/out/emptyreg.md"
if [ "${RC}" -ne 0 ] && [ ! -e "${T}/out/emptyreg.md" ]; then ok "empty plugin set fails closed"
else bad "empty plugin set (rc=${RC}): ${OUT}"; fi

# 4. Output directory must exist; it is never created.
run "${GOOD}" "${T}/absent/dir/f17.md"
if [ "${RC}" -ne 0 ] && [ ! -e "${T}/absent" ]; then ok "absent output directory is refused, not created"
else bad "absent output dir (rc=${RC}): ${OUT}"; fi

# 5. The retired .claude/docs/sport path is refused and not created.
mkdir -p "${T}/proj/.claude/docs"
run "${GOOD}" "${T}/proj/.claude/docs/sport/F17-CLI-PLUGIN-COMPAT.md"
if [ "${RC}" -ne 0 ] && [ ! -e "${T}/proj/.claude/docs/sport" ] && printf '%s' "${OUT}" | grep -qF 'retired'; then
  ok "retired .claude/docs/sport path refused"
else bad "retired path (rc=${RC}): ${OUT}"; fi

# 6. Default output (no F17_OUT_FILE) goes under TMPDIR, never .claude/docs/sport.
RC=0
OUT="$(F17_NSELF_ROOT="${GOOD}" TMPDIR="${T}/out" bash "${GEN}" 2>&1)" || RC=$?
if [ "${RC}" -eq 0 ] && [ -f "${T}/out/F17-CLI-PLUGIN-COMPAT.md" ] \
  && ! printf '%s' "${OUT}" | grep -qF '.claude/docs/sport'; then
  ok "default output is under TMPDIR"
else bad "default output (rc=${RC}): ${OUT}"; fi

# 7. No integer-expression noise on a good run.
run "${GOOD}" "${T}/out/quiet.md"
if printf '%s' "${OUT}" | grep -qF 'integer'; then bad "integer noise: ${OUT}"; else ok "no integer-expression noise"; fi

printf '\n%s passed, %s failed\n' "${PASS}" "${FAIL}"
[ "${FAIL}" -eq 0 ]
