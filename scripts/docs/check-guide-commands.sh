#!/usr/bin/env bash
# check-guide-commands.sh — every documented `nself` command must be exercised.
#
# Usage: bash scripts/docs/check-guide-commands.sh <guide.md> [more.md ...]
#
# For each line inside a ```bash / ```sh / ```shell fence that starts with
# `nself ` (after an optional `$ `), the signature is `nself` plus the leading
# tokens up to the first token that starts with `-`, `<`, `[`, `$`, a quote,
# `#`, `|`, `;`, `&`, `>` or `\`, or contains `=`. The signature must appear
# (fixed string) in a file under scripts/ (excluding scripts/docs/) or in any
# *_test.go file. A line ending in `# doc-check: skip <reason>` is skipped; an
# empty reason fails.
#
# Output: one `<file>:<line>: <signature>` per unmatched line on stdout.
# Exit:   0 all matched, 1 unmatched line or empty skip reason, 2 usage error
#         or missing file.
#
# Never executes a documented command and never touches the network.
# Test hook: GUIDE_CHECK_ROOT overrides the repository root.
# Bash 3.2 compatible (macOS system shell).

set -euo pipefail

if [ "$#" -eq 0 ]; then
  printf 'usage: %s <guide.md> [more.md ...]\n' "$0" >&2
  exit 2
fi

for f in "$@"; do
  if [ ! -f "$f" ]; then
    printf 'error: file not found: %s\n' "$f" >&2
    exit 2
  fi
done

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="${GUIDE_CHECK_ROOT:-$(cd "${SCRIPT_DIR}/../.." && pwd)}"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/guide-check.XXXXXX")"
trap 'rm -rf "${WORK}"' EXIT
CORPUS="${WORK}/corpus"
CANDIDATES="${WORK}/candidates"
: > "${CORPUS}"
: > "${CANDIDATES}"

# Corpus: scripts/ (minus scripts/docs/) plus every *_test.go outside vendor,
# node_modules, .git and .claude (which holds sibling worktrees).
if [ -d "${ROOT}/scripts" ]; then
  find "${ROOT}/scripts" -type f -not -path "${ROOT}/scripts/docs/*" -print0 \
    | xargs -0 cat >> "${CORPUS}" 2>/dev/null || true
fi
find "${ROOT}" \( -name vendor -o -name node_modules -o -name .git -o -name .claude \) -prune \
  -o -type f -name '*_test.go' -print0 \
  | xargs -0 cat >> "${CORPUS}" 2>/dev/null || true

# Extract candidate lines as: <file>\t<line>\t<kind>\t<signature>
# kind = CMD (needs a corpus match) or EMPTYSKIP (skip marker without a reason).
for f in "$@"; do
  awk -v file="$f" '
    function signature(s,    n, t, i, sig, tok, term) {
      term = "-<[$\"\047#|;&>\\"
      n = split(s, t, /[ \t]+/)
      sig = t[1]
      for (i = 2; i <= n; i++) {
        tok = t[i]
        if (tok == "") continue
        if (index(term, substr(tok, 1, 1)) > 0 || index(tok, "=") > 0) break
        sig = sig " " tok
      }
      return sig
    }
    {
      line = $0
      sub(/\r$/, "", line)
      if (line ~ /^[ \t]*```/) {
        if (in_fence) { in_fence = 0; target = 0; next }
        in_fence = 1
        lang = line
        sub(/^[ \t]*```[ \t]*/, "", lang)
        sub(/[^A-Za-z0-9_+-].*$/, "", lang)
        target = (lang == "bash" || lang == "sh" || lang == "shell") ? 1 : 0
        next
      }
      if (!in_fence || !target) next
      cmd = line
      sub(/^[ \t]+/, "", cmd)
      if (cmd ~ /^\$[ \t]+/) sub(/^\$[ \t]+/, "", cmd)
      if (cmd != "nself" && cmd !~ /^nself[ \t]/) next
      if (match(cmd, /#[ \t]*doc-check:[ \t]*skip([ \t].*)?$/)) {
        reason = substr(cmd, RSTART, RLENGTH)
        sub(/^#[ \t]*doc-check:[ \t]*skip/, "", reason)
        gsub(/^[ \t]+|[ \t]+$/, "", reason)
        if (reason == "") {
          printf "%s\t%d\tEMPTYSKIP\t%s\n", file, NR, "doc-check: skip needs a reason"
        }
        next
      }
      printf "%s\t%d\tCMD\t%s\n", file, NR, signature(cmd)
    }
  ' "$f" >> "${CANDIDATES}"
done

FAILED=0
TAB="$(printf '\t')"
while IFS="${TAB}" read -r file lineno kind sig; do
  [ -n "${file}" ] || continue
  if [ "${kind}" = "EMPTYSKIP" ]; then
    printf '%s:%s: %s\n' "${file}" "${lineno}" "${sig}"
    FAILED=1
  elif ! grep -aqF -- "${sig}" "${CORPUS}"; then
    printf '%s:%s: %s\n' "${file}" "${lineno}" "${sig}"
    FAILED=1
  fi
done < "${CANDIDATES}"

exit "${FAILED}"
