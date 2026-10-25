#!/usr/bin/env bash
# Purpose: ratchet on raw panic() call sites in first-party library Go code
#          (CONSTITUTION §4 rule 2: no panics outside main). Fails when a file
#          gains a site or a new file has one, so the count only goes down
#          (D-0117). Existing sites are converted to typed errors when touched.
# Inputs:  optional --print (write the current "<count> <path>" lines, for
#          regenerating the baseline); optional $1 = root to scan (default: the repo root two levels above
#          this script); optional PANIC_BASELINE = baseline file (default
#          scripts/ci/panic-baseline.txt, lines "<count> <path>").
# Outputs: exit 0 within the baseline; exit 1 listing each file over its
#          baseline; exit 2 on usage or a missing baseline. Prints a note when a
#          file is below its baseline so the baseline can be lowered.
# Constraints: bash 3.2 compatible; read-only. Skips vendor/, testdata/,
#          .claude/, node_modules/, *_test.go and files in package main.
#          Comment-only lines are ignored. Tested by panic-ratchet_test.sh.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
PRINT=0
if [ "${1:-}" = "--print" ]; then PRINT=1; shift; fi
ROOT="${1:-$(cd "${HERE}/../.." && pwd)}"
BASELINE="${PANIC_BASELINE:-${HERE}/panic-baseline.txt}"

[ -d "${ROOT}" ] || { echo "panic-ratchet: no such directory: ${ROOT}" >&2; exit 2; }
[ "${PRINT}" -eq 1 ] || [ -f "${BASELINE}" ] || { echo "panic-ratchet: baseline not found: ${BASELINE}" >&2; exit 2; }

# current prints "<count> <path>" per first-party library file with a raw panic(.
current() {
  (cd "${ROOT}" && find . \( -name vendor -o -name testdata -o -name .claude -o -name node_modules -o -name .git \) -prune -o \
      -type f -name '*.go' ! -name '*_test.go' -print0 \
    | xargs -0 grep -c -E '(^|[^A-Za-z0-9_."])panic\(' 2>/dev/null || true) \
  | while IFS=: read -r path n; do
      [ "${n:-0}" -gt 0 ] || continue
      path="${path#./}"
      grep -q '^package main$' "${ROOT}/${path}" && continue
      # Recount without comment-only lines.
      n="$(grep -E '(^|[^A-Za-z0-9_."])panic\(' "${ROOT}/${path}" | grep -c -v -E '^[[:space:]]*//' || true)"
      [ "${n:-0}" -gt 0 ] && printf '%s %s\n' "${n}" "${path}"
    done | LC_ALL=C sort -k2
}

if [ "${PRINT}" -eq 1 ]; then current; exit 0; fi

rc=0
while read -r n path; do
  [ -n "${path:-}" ] || continue
  base="$(awk -v p="${path}" '$2 == p {print $1}' "${BASELINE}")"
  base="${base:-0}"
  if [ "${n}" -gt "${base}" ]; then
    echo "panic-ratchet: FAIL: ${path} has ${n} raw panic() site(s), baseline ${base}. Return a typed error instead (CONSTITUTION §4 rule 2)." >&2
    rc=1
  elif [ "${n}" -lt "${base}" ]; then
    echo "panic-ratchet: note: ${path} is down to ${n} (baseline ${base}); lower scripts/ci/panic-baseline.txt." >&2
  fi
done < <(current)

# A baseline entry whose file no longer has a site is stale.
while read -r base path; do
  [ -n "${path:-}" ] || continue
  if ! current | awk -v p="${path}" '$2 == p {found=1} END {exit !found}'; then
    echo "panic-ratchet: note: ${path} has no raw panic() left; remove it from the baseline." >&2
  fi
done < "${BASELINE}"

[ "${rc}" -eq 0 ] && echo "panic-ratchet: ok ($(current | awk '{s+=$1} END {print s+0}') site(s))"
exit "${rc}"
