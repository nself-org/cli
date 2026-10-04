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

# 7. Non-fenced, non-bash fences, non-nself lines and comments are ignored:
#    ignored.md has exactly one real command (nself start, line 15).
run "${EMPTY}" "${DATA}/ignored.md"
expect_rc "ignored.md reports its one command" 1
expect_out "ignored lines add nothing" "${DATA}/ignored.md:15: nself start"
if [ "$(printf '%s\n' "${OUT}" | grep -c ': nself')" -eq 1 ]; then ok "exactly one unmatched line"; else bad "extra lines: ${OUT}"; fi
ST="${T}/with-start"
mkdir -p "${ST}/scripts"
printf 'nself start\n' > "${ST}/scripts/leg.sh"
run "${ST}" "${DATA}/ignored.md"
expect_rc "ignored.md passes once matched" 0

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

# 10. Zero extracted commands fail (empty file, prose only, wrong fence).
run "${EMPTY}" "${DATA}/empty.md"
expect_rc "empty guide fails" 1
expect_out "empty guide message" "error: no documented nself command lines found in the given files"
run "${EMPTY}" "${DATA}/none.md"
expect_rc "guide without nself commands fails" 1
run "${SC}" "${DATA}/none.md" "${DATA}/unmatched.md"
expect_rc "one guide without commands is fine when the set has commands" 0
run "${EMPTY}" "${DATA}/skip-only.md"
expect_rc "skipped-only guide counts as documenting commands" 0
expect_out "summary on stderr" "checked 1 command lines (1 skipped) in 1 files"

# 11. console fences: only \$ prompt lines are commands.
run "${EMPTY}" "${DATA}/console.md"
expect_rc "console fence is scanned" 1
expect_out "console prompt line reported" "${DATA}/console.md:4: nself db import supabase"
expect_no_out "console output lines ignored" "nself 1.4.12"
expect_no_out "console output lines ignored (2)" "nself started"

# 12. A terminator glued to a token cuts the signature there.
GL="${T}/glued"
mkdir -p "${GL}/scripts"
printf 'nself start\nnself build\nnself status\nnself sync\nnself restart\n' > "${GL}/scripts/leg.sh"
run "${GL}" "${DATA}/glued.md"
expect_rc "glued terminators do not false-fail" 0
run "${EMPTY}" "${DATA}/glued.md"
expect_out "start; cut at ;"   "${DATA}/glued.md:4: nself start"
expect_out "build>out cut at >" "${DATA}/glued.md:5: nself build"
expect_out "status|jq cut at |" "${DATA}/glued.md:6: nself status"
expect_out "sync\\ cut at backslash" "${DATA}/glued.md:7: nself sync"
expect_out "restart&& cut at &" "${DATA}/glued.md:8: nself restart"

# 13. Missing root or a root without scripts/ is a configuration error, not a mismatch.
run "${T}/no-such-root" "${DATA}/unmatched.md"
expect_rc "missing root" 2
expect_out "missing root message" "error: missing root: no scripts/ directory under ${T}/no-such-root"
mkdir -p "${T}/noscripts"
run "${T}/noscripts" "${DATA}/unmatched.md"
expect_rc "root without scripts/" 2
expect_no_out "no mismatch line for a bad root" "unmatched.md:4"

# 14. node_modules, .git and .claude are not searched for *_test.go.
for d in node_modules .git .claude; do
  X="${T}/skip-${d}"
  mkdir -p "${X}/scripts" "${X}/${d}/pkg"
  printf '// nself db import supabase\n' > "${X}/${d}/pkg/y_test.go"
  run "${X}" "${DATA}/unmatched.md"
  expect_rc "${d} is not searched" 1
done

printf '\n%s passed, %s failed\n' "${PASS}" "${FAIL}"
[ "${FAIL}" -eq 0 ]
