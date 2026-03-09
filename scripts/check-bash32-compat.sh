#!/bin/bash
# check-bash32-compat.sh
# Scan CLI source for Bash 4+ incompatible patterns.
# Exit 0 if clean. Exit 1 if any violations found.
# Usage: bash scripts/check-bash32-compat.sh [dir]
#   dir: directory to scan (default: src/)

set -eu

SCAN_DIR="${1:-src}"
ERRORS=0

# Colors (safe for all terminals — use ANSI only when stdout is a tty)
if [ -t 1 ]; then
  RED='\033[0;31m'
  GREEN='\033[0;32m'
  YELLOW='\033[1;33m'
  RESET='\033[0m'
else
  RED=''
  GREEN=''
  YELLOW=''
  RESET=''
fi

fail() {
  printf "${RED}FAIL${RESET}: %s\n" "$1" >&2
  ERRORS=$((ERRORS + 1))
}

warn() {
  printf "${YELLOW}WARN${RESET}: %s\n" "$1"
}

pass() {
  printf "${GREEN}PASS${RESET}: %s\n" "$1"
}

check_pattern() {
  local pattern="$1"
  local desc="$2"
  local exclude="${3:-}"

  local grep_cmd="grep -rn --include=*.sh"
  if [ -n "$exclude" ]; then
    grep_cmd="$grep_cmd --exclude=$exclude"
  fi

  # Use eval-free approach: write to temp file
  local tmpfile
  tmpfile=$(mktemp)
  # shellcheck disable=SC2086
  grep -rn --include="*.sh" "$pattern" "$SCAN_DIR" > "$tmpfile" 2>/dev/null || true

  if [ -n "$exclude" ]; then
    grep -v "$exclude" "$tmpfile" > "${tmpfile}.filtered" 2>/dev/null || true
    mv "${tmpfile}.filtered" "$tmpfile"
  fi

  if [ -s "$tmpfile" ]; then
    fail "$desc"
    cat "$tmpfile" >&2
    printf "\n" >&2
  fi
  rm -f "$tmpfile"
}

printf "Checking %s for Bash 3.2 incompatibilities...\n\n" "$SCAN_DIR"

# --- Forbidden patterns ---

check_pattern 'echo -e' \
  'echo -e usage (portability: use printf instead)'

check_pattern '\${[^}]*,,[^}]*}' \
  'Lowercase expansion \${var,,} (Bash 4+ only)'

check_pattern '\${[^}]*\^\^[^}]*}' \
  'Uppercase expansion \${var^^} (Bash 4+ only)'

check_pattern '\bmapfile\b' \
  'mapfile builtin (Bash 4+ only — use while read loop)'

check_pattern '\breadarray\b' \
  'readarray builtin (Bash 4+ only — use while read loop)'

# declare -A: exclude files that explicitly check Bash version before using it
# (ports.sh, base.sh, provision.sh are known to have version-guarded declare -A)
check_pattern 'declare -A' \
  'Associative arrays declare -A (Bash 4+ only — use parallel arrays or case)' \
  'ports.sh\|base.sh\|provision.sh'

# stat -c is GNU coreutils only (Linux). stat -f is BSD (macOS).
# Both are forbidden in raw form — use safe_stat_perms() from platform-compat.sh instead.
check_pattern 'stat -c ' \
  'stat -c (GNU only — use safe_stat_perms() from platform-compat.sh)'

check_pattern 'stat -f ' \
  'stat -f (BSD only — use safe_stat_mtime() from platform-compat.sh)'

# sed -i '' is BSD; sed -i is GNU. Both fail on the other.
# Use safe_sed_inline() from platform-compat.sh instead.
check_pattern "sed -i ''" \
  "sed -i '' (BSD only — use safe_sed_inline() from platform-compat.sh)"

# readlink -f is GNU. macOS readlink does not support -f.
# Use safe_readlink() from platform-compat.sh instead.
check_pattern 'readlink -f' \
  'readlink -f (GNU only — use safe_readlink() from platform-compat.sh)'

# {1..N} brace expansion works in Bash 3.2+ for literals but NOT with variables.
# Flag any variable-based ranges: {$var..10} or {1..$var}
check_pattern '{\$[a-zA-Z_][a-zA-Z0-9_]*\.\.' \
  'Variable brace range {\$var..N} (not supported in Bash 3.2 — use seq or while loop)'

# --- Summary ---
printf "\n"
if [ "$ERRORS" -eq 0 ]; then
  pass "All checks passed. No Bash 3.2 incompatibilities found."
  exit 0
else
  printf "${RED}Found %d incompatibility issue(s).${RESET}\n" "$ERRORS" >&2
  printf "Fix the patterns listed above before committing.\n" >&2
  exit 1
fi
