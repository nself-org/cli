#!/usr/bin/env bash
# scripts/mutation_test.sh
#
# Purpose: test scripts/mutation.sh --only without running real mutation
#          testing: a fake gremlins (GREMLINS_BIN) prints canned reports and
#          records its working directory and arguments.
# Usage:   bash scripts/mutation_test.sh
# Output:  PASS/FAIL per case; exit 0 only when every case passes.
set -uo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d "${TMPDIR:-/tmp}/mutation-test.XXXXXX")"
trap 'rm -rf "$work"' EXIT

cat > "$work/fake-gremlins" <<'FAKE'
#!/usr/bin/env bash
# Fake gremlins: logs "pwd" and the arguments, then prints $FAKE_REPORT.
{ pwd; printf '%s\n' "$@"; } >> "$FAKE_LOG"
[ -n "${FAKE_REPORT:-}" ] && cat "$FAKE_REPORT"
echo "Mutation testing completed in 1 second"
exit 0
FAKE
chmod +x "$work/fake-gremlins"

fail=0
check() { # name expected_exit actual_exit
  if [ "$2" = "$3" ]; then echo "PASS: $1"; else echo "FAIL: $1 (exit $3, want $2)"; fail=1; fi
}
# run_case LOG REPORT-CONTENT ARGS... -> sets rc, text in $work/out
run_case() {
  local report="$1"; shift
  : > "$work/log"; printf '%s' "$report" > "$work/report"
  FAKE_LOG="$work/log" FAKE_REPORT="$work/report" GREMLINS_BIN="$work/fake-gremlins" \
    MUTATION_OUT="$work/mut.out" bash "$root/scripts/mutation.sh" "$@" > "$work/out" 2>&1
  rc=$?
}
has() { grep -qF -- "$1" "$2"; }

killed=$'      KILLED CONDITIONALS_NEGATION at hostspec.go:58:13\n      KILLED ARITHMETIC_BASE at hostspec.go:59:21\n      KILLED CONDITIONALS_BOUNDARY at copy.go:62:10\n'

# 1. sdk/go path: the --only file resolves under sdk/go, gremlins runs in the
#    module dir on ./remote and every other file is excluded.
run_case "$killed" sdk/go/remote --only hostspec.go
check "sdk path resolves, killed mutants judged, exit 0" 0 "$rc"
has "judged 2 mutants in hostspec.go (killed 2)" "$work/out" || { echo "FAIL: judged line missing"; fail=1; }
[ "$(sed -n 1p "$work/log")" = "$root/sdk/go" ] || { echo "FAIL: gremlins ran in $(sed -n 1p "$work/log")"; fail=1; }
has "./remote" "$work/log" || { echo "FAIL: package arg not ./remote"; fail=1; }
has '(^|/)copy\.go$' "$work/log" || { echo "FAIL: copy.go not excluded"; fail=1; }
! has 'hostspec\.go' "$work/log" || { echo "FAIL: hostspec.go excluded"; fail=1; }

# 2. A missing --only file is an error, not a skip.
run_case "$killed" sdk/go/remote --only hostspec.go,nosuchfile.go
check "missing --only file exits non-zero" 1 "$rc"
has "not found: remote/nosuchfile.go" "$work/out" || { echo "FAIL: missing-file message"; fail=1; }
run_case "$killed" internal/license --only nosuchfile.go
check "missing --only file exits non-zero (internal)" 1 "$rc"

# 3. Zero mutants judged is an error (empty report, or mutants only in other files).
run_case "" sdk/go/remote --only hostspec.go
check "empty report exits non-zero" 1 "$rc"
has "zero mutants were judged" "$work/out" || { echo "FAIL: zero-judged message"; fail=1; }
run_case $'      KILLED CONDITIONALS_BOUNDARY at copy.go:62:10\n' sdk/go/remote --only hostspec.go
check "mutants only in other files exits non-zero" 1 "$rc"

# 4. A surviving mutant in a listed file still fails; one in another file does not.
run_case "$killed"$'       LIVED CONDITIONALS_NEGATION at hostspec.go:70:9\n' sdk/go/remote --only hostspec.go
check "lived mutant in a listed file exits non-zero" 1 "$rc"
run_case "$killed"$'       LIVED CONDITIONALS_NEGATION at copy.go:70:9\n' sdk/go/remote --only hostspec.go
check "lived mutant in an unlisted file is ignored" 0 "$rc"

# 5. internal/ packages: same behaviour, run from the repo root with ./internal/license.
run_case $'      KILLED CONDITIONALS_NEGATION at validate.go:40:9\n' internal/license --only validate.go
check "internal path judged, exit 0" 0 "$rc"
[ "$(sed -n 1p "$work/log")" = "$root" ] || { echo "FAIL: internal run cwd $(sed -n 1p "$work/log")"; fail=1; }
has "./internal/license" "$work/log" || { echo "FAIL: internal package arg"; fail=1; }
run_case $'       LIVED CONDITIONALS_NEGATION at validate.go:40:9\n' internal/license --only validate.go
check "internal lived mutant exits non-zero" 1 "$rc"
run_case $'       LIVED ARITHMETIC_BASE at validate.go:23:32\n      KILLED CONDITIONALS_NEGATION at validate.go:40:9\n' internal/license --only validate.go
check "internal allowlisted equivalent still passes" 0 "$rc"

[ "$fail" = 0 ] && echo "ALL PASS" || echo "SOME FAILED"
exit "$fail"
