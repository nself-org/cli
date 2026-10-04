#!/usr/bin/env bash
# check-guide-commands.sh — every documented `nself` command must be exercised.
#
# Usage: bash scripts/docs/check-guide-commands.sh <guide.md> [more.md ...]
#
# For each line inside a ```bash / ```sh / ```shell / ```zsh fence that starts
# with `nself ` (after an optional `$ `), and each `$ nself ` line inside a
# ```console fence, the signature is `nself` plus the leading tokens up to the
# first token that starts with `-`, `<`, `[`, `$`, a quote, `#`, or contains
# `=`; a token is also cut at the first `;`, `|`, `&`, `>`, `<` or `\` inside
# it (`start;` gives `start`). The signature must appear (fixed string) in a
# file under scripts/ (excluding scripts/docs/) or in any *_test.go file. A line
# ending in `# doc-check: skip <reason>` is skipped; an empty reason fails.
#
# Output: one `<file>:<line>: <signature>` per unmatched line on stdout; a
#         `checked N command lines (K skipped) in M files` summary on stderr.
# Exit:   0 all matched, 1 unmatched line, empty skip reason, or no command
#         lines found at all (a guide set that documents nothing is a failure),
#         2 usage error, missing file or missing root.
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
if [ ! -d "${ROOT}/scripts" ]; then
  printf 'error: missing root: no scripts/ directory under %s\n' "${ROOT}" >&2
  exit 2
fi
find "${ROOT}/scripts" -type f -not -path "${ROOT}/scripts/docs/*" -print0 \
  | xargs -0 cat >> "${CORPUS}" 2>/dev/null || true
find "${ROOT}" \( -name vendor -o -name node_modules -o -name .git -o -name .claude \) -prune \
  -o -type f -name '*_test.go' -print0 \
  | xargs -0 cat >> "${CORPUS}" 2>/dev/null || true

# Extract candidate lines as: <file>\t<line>\t<kind>\t<signature>
# kind = CMD (needs a corpus match) or EMPTYSKIP (skip marker without a reason).
for f in "$@"; do
  awk -v file="$f" '
    function signature(s,    n, t, i, j, sig, tok, term, cut, c) {
      term = "-<[$\"\047#"
      n = split(s, t, /[ \t]+/)
      sig = t[1]
      for (i = 2; i <= n; i++) {
        tok = t[i]
        if (tok == "") continue
        if (index(term, substr(tok, 1, 1)) > 0 || index(tok, "=") > 0) break
        cut = 0
        for (j = 1; j <= length(tok); j++) {
          c = substr(tok, j, 1)
          if (index(";|&><\\", c) > 0) { cut = j; break }
        }
        if (cut == 1) break
        if (cut > 1) { sig = sig " " substr(tok, 1, cut - 1); break }
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
        target = 0
        if (lang == "bash" || lang == "sh" || lang == "shell" || lang == "zsh") target = 1
        if (lang == "console" || lang == "shell-session") target = 2
        next
      }
      if (!in_fence || !target) next
      cmd = line
      sub(/^[ \t]+/, "", cmd)
      if (target == 2 && cmd !~ /^\$[ \t]+/) next   # console: only prompt lines are commands
      if (cmd ~ /^\$[ \t]+/) sub(/^\$[ \t]+/, "", cmd)
      if (cmd != "nself" && cmd !~ /^nself[ \t]/) next
      if (match(cmd, /#[ \t]*doc-check:[ \t]*skip([ \t].*)?$/)) {
        reason = substr(cmd, RSTART, RLENGTH)
        sub(/^#[ \t]*doc-check:[ \t]*skip/, "", reason)
        gsub(/^[ \t]+|[ \t]+$/, "", reason)
        if (reason == "") {
          printf "%s\t%d\tEMPTYSKIP\t%s\n", file, NR, "doc-check: skip needs a reason"
        } else {
          printf "%s\t%d\tSKIP\t%s\n", file, NR, reason
        }
        next
      }
      printf "%s\t%d\tCMD\t%s\n", file, NR, signature(cmd)
    }
  ' "$f" >> "${CANDIDATES}"
done

FAILED=0
TOTAL=0
SKIPPED=0
TAB="$(printf '\t')"
while IFS="${TAB}" read -r file lineno kind sig; do
  [ -n "${file}" ] || continue
  TOTAL=$((TOTAL + 1))
  if [ "${kind}" = "SKIP" ]; then
    SKIPPED=$((SKIPPED + 1))
  elif [ "${kind}" = "EMPTYSKIP" ]; then
    printf '%s:%s: %s\n' "${file}" "${lineno}" "${sig}"
    FAILED=1
  elif ! grep -aqF -- "${sig}" "${CORPUS}"; then
    printf '%s:%s: %s\n' "${file}" "${lineno}" "${sig}"
    FAILED=1
  fi
done < "${CANDIDATES}"

printf 'checked %d command lines (%d skipped) in %d files\n' "${TOTAL}" "${SKIPPED}" "$#" >&2
if [ "${TOTAL}" -eq 0 ]; then
  printf 'error: no documented nself command lines found in the given files\n' >&2
  FAILED=1
fi

exit "${FAILED}"
