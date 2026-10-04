#!/usr/bin/env bash
# check-guide-commands_test.sh — tests for check-guide-commands.sh.
# Usage: bash scripts/docs/check-guide-commands_test.sh
# Builds throwaway repository roots (GUIDE_CHECK_ROOT) so fixtures never
# depend on the real scripts/ tree. Bash 3.2 compatible.

set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
CHECK="${HERE}/check-guide-commands.sh"
DATA="${HERE}/testdata"
T="$(mktemp -d "${TMPDIR:-/tmp}/guide-check-test.XXXXXX")"
trap 'rm -rf "${T}"' EXIT

PASS=0
FAIL=0
ok()   { PASS=$((PASS + 1)); printf '  ok   %s\n' "$1"; }
bad()  { FAIL=$((FAIL + 1)); printf '  FAIL %s\n' "$1"; }

# run <root> <files...>: sets RC and OUT.
run() {
  local root="$1"; shift
  RC=0
  OUT="$(GUIDE_CHECK_ROOT="${root}" bash "${CHECK}" "$@" 2>&1)" || RC=$?
}

expect_rc() { # name want
  if [ "${RC}" -eq "$2" ]; then ok "$1 (exit $2)"; else bad "$1 (exit ${RC}, want $2): ${OUT}"; fi
}
expect_out() { # name want-line
  if printf '%s\n' "${OUT}" | grep -qxF -- "$2"; then ok "$1"; else bad "$1: want '$2', got: ${OUT}"; fi
}
expect_no_out() { # name text
  if printf '%s\n' "${OUT}" | grep -qF -- "$2"; then bad "$1: unexpected '$2' in: ${OUT}"; else ok "$1"; fi
}

EMPTY="${T}/empty"
mkdir -p "${EMPTY}/scripts"

# 1. Unmatched line fails with <file>:<line>: <signature>.
run "${EMPTY}" "${DATA}/unmatched.md"
expect_rc "unmatched fails" 1
expect_out "unmatched names file:line: signature" "${DATA}/unmatched.md:4: nself db import supabase"

# 2. A scripts/ file that mentions the signature makes it pass.
SC="${T}/with-script"
mkdir -p "${SC}/scripts"
printf '# exercises: nself db import supabase\n' > "${SC}/scripts/leg.sh"
run "${SC}" "${DATA}/unmatched.md"
expect_rc "matched by a scripts/ file" 0

# 3. A *_test.go file that mentions the signature makes it pass.
GO="${T}/with-gotest"
mkdir -p "${GO}/scripts" "${GO}/internal/dbimport"
printf '// "nself db import supabase --file x"\n' > "${GO}/internal/dbimport/x_test.go"
run "${GO}" "${DATA}/unmatched.md"
expect_rc "matched by a *_test.go file" 0

# 4. scripts/docs/ and vendor/ do not count.
EX="${T}/excluded"
mkdir -p "${EX}/scripts/docs" "${EX}/vendor/x"
printf 'nself db import supabase\n' > "${EX}/scripts/docs/leg.sh"
printf '// nself db import supabase\n' > "${EX}/vendor/x/y_test.go"
run "${EX}" "${DATA}/unmatched.md"
expect_rc "scripts/docs and vendor are excluded" 1

# 5. The `$ ` prefix is stripped.
run "${EMPTY}" "${DATA}/dollar.md"
expect_rc "dollar prefix still checked" 1
expect_out "dollar prefix signature" "${DATA}/dollar.md:4: nself db import supabase"

# 6. Skip marker with a reason is ignored; without a reason it fails.
run "${EMPTY}" "${DATA}/skip-reason.md"
expect_rc "skip with reason passes" 0
run "${EMPTY}" "${DATA}/skip-empty.md"
expect_rc "skip without reason fails" 1
expect_out "skip without reason message" "${DATA}/skip-empty.md:4: doc-check: skip needs a reason"

# 7. Non-fenced, non-bash fences, non-nself lines and comments are ignored.
run "${EMPTY}" "${DATA}/ignored.md"
expect_rc "ignored lines pass" 0
expect_out "ignored lines print nothing" ""

# 8. Signature rule.
run "${EMPTY}" "${DATA}/signatures.md"
expect_rc "signature fixture fails (empty root)" 1
expect_out "plain command"            "${DATA}/signatures.md:4: nself start"
expect_out "stops at <"               "${DATA}/signatures.md:5: nself plugin install"
expect_out "stops at KEY=value"       "${DATA}/signatures.md:6: nself env set"
expect_out "stops at a quote"         "${DATA}/signatures.md:7: nself exec"
expect_out "stops at ["               "${DATA}/signatures.md:8: nself logs"
expect_out "stops at \$"              "${DATA}/signatures.md:9: nself deploy staging"
expect_out "stops at |"               "${DATA}/signatures.md:10: nself status"
expect_out "stops at >"               "${DATA}/signatures.md:11: nself backup create"
expect_out "stops at #"               "${DATA}/signatures.md:12: nself doctor"
expect_out "stops at -"               "${DATA}/signatures.md:13: nself init"
expect_out "bare nself"               "${DATA}/signatures.md:14: nself"

# 9. Missing file exits 2; several files are all checked.
run "${EMPTY}" "${DATA}/does-not-exist.md"
expect_rc "missing file" 2
run "${EMPTY}" "${DATA}/unmatched.md" "${DATA}/does-not-exist.md"
expect_rc "missing file among several" 2
run "${SC}" "${DATA}/unmatched.md" "${DATA}/skip-reason.md"
expect_rc "several files, all match" 0
run "${EMPTY}"
expect_rc "no arguments" 2

printf '\n%s passed, %s failed\n' "${PASS}" "${FAIL}"
[ "${FAIL}" -eq 0 ]
