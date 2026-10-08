#!/usr/bin/env bash
# Purpose: prove the P7-REG machine contract on the real binary, on macOS and on
#          Linux (golang:1.26.9, GitHub-hosted ubuntu). The go tests prove the
#          pieces in-process; this proves the built `nself` end to end in both
#          compat modes (ADR 0021: v1.4 = NSELF_V15 unset, v1.5 = NSELF_V15=1).
#          Epic P7-REG acceptance 1, 3, 4 and 5.
# Usage:   bash scripts/ci/contract-smoke.sh
# Inputs:  go and jq on PATH (a missing tool is a FAIL, never a skip).
# Outputs: one line per assertion on stdout, "PASS <name>" or "FAIL <name>",
#          nothing else on stdout. Diagnostics for a FAIL go to stderr. Exit 0
#          only when every assertion passed and at least one ran.
# Constraints: bash 3.2 compatible. Builds the binary into a temp dir; uses a
#          fixture project, a temp HOME and a stub `docker` that always fails,
#          so no real Docker, network or user state is touched.
set -euo pipefail

cd "$(dirname "$0")/../.."
ROOT="$(pwd)"
PASSED=0
FAILED=0

pass() { printf 'PASS %s\n' "$1"; PASSED=$((PASSED + 1)); }
fail() {
  printf 'FAIL %s\n' "$1"
  FAILED=$((FAILED + 1))
  if [ -n "${2:-}" ]; then printf '  %s\n' "$2" >&2; fi
}
# check <name> <command...>: PASS when the command exits 0.
check() {
  local name="$1"
  shift
  if "$@" >/dev/null 2>&1; then pass "$name"; else fail "$name" "$(printf 'check failed: %.160s' "$*")"; fi
}

for tool in go jq; do
  command -v "$tool" >/dev/null 2>&1 || { printf 'FAIL tool-%s-missing\n' "$tool"; exit 1; }
done

TMP="$(mktemp -d "${TMPDIR:-/tmp}/contract-smoke.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT
BIN="$TMP/nself"
PROJ="$TMP/proj"
STUB="$TMP/stub"
HOME_DIR="$TMP/home"
mkdir -p "$PROJ" "$STUB" "$HOME_DIR"
printf 'PROJECT_NAME=smoke\nENV=dev\nPOSTGRES_PASSWORD=smoke-fixture-value\n' > "$PROJ/.env"
printf '#!/bin/sh\necho "Cannot connect to the Docker daemon" >&2\nexit 1\n' > "$STUB/docker"
chmod +x "$STUB/docker"

# The caller's environment must not steer the binary.
for v in $(env | sed -n 's/^\(NSELF_[A-Za-z0-9_]*\)=.*/\1/p'); do unset "$v"; done

# -buildvcs=false: a bind-mounted worktree (.git file pointing outside the
# container) must still build.
if ! CGO_ENABLED=0 go build -mod=vendor -buildvcs=false -o "$BIN" ./cmd/nself 2>"$TMP/build.err"; then
  cat "$TMP/build.err" >&2
  printf 'FAIL build\n'
  exit 1
fi
pass build

RC=0
OUT="$TMP/out"
ERR="$TMP/err"
# run <v14|v15> [KEY=VALUE ...] -- <nself args...>: stdout in $OUT, stderr in $ERR,
# exit status in $RC. The stub docker leads PATH; HOME is the temp dir.
run() {
  local mode="$1" extra=()
  shift
  while [ "$1" != "--" ]; do extra+=("$1"); shift; done
  shift
  [ "$mode" = v15 ] && extra+=("NSELF_V15=1")
  RC=0
  (cd "$PROJ" && env HOME="$HOME_DIR" AI_AUTO_INSTALL=false PATH="$STUB:$PATH" \
    ${extra[@]+"${extra[@]}"} "$BIN" "$@") >"$OUT" 2>"$ERR" || RC=$?
}
# one_doc: stdout is exactly one JSON document.
one_doc() { [ "$(jq -s 'length' "$OUT")" = 1 ]; }
# envelope <command>: stdout is one v1 envelope for <command>.
envelope() {
  one_doc && jq -e --arg c "$1" '.schema_version == "1" and .command == $c' "$OUT" >/dev/null
}
has_err() { grep -qF "$1" "$ERR"; }
# rc_err <status> <text>: the last run exited <status> and stderr holds <text>.
rc_err() { [ "$RC" -eq "$1" ] && has_err "$2"; }
# rc_out <status> <text>: the last run exited <status> and stdout holds <text>.
rc_out() { [ "$RC" -eq "$1" ] && grep -qF -- "$2" "$OUT"; }
# rc_in <lo> <hi>: the last run's exit status is within [lo, hi].
rc_in() { [ "$RC" -ge "$1" ] && [ "$RC" -le "$2" ]; }

# ---- acceptance 1: help --json (v1.5), from a directory with no project ----
EMPTY="$TMP/empty"
mkdir -p "$EMPTY"
RC=0
(cd "$EMPTY" && env HOME="$HOME_DIR" NSELF_V15=1 "$BIN" help --json) >"$OUT" 2>"$ERR" || RC=$?
check "help-json: exit 0 and one v1 envelope outside a project" test "$RC" -eq 0 -a "$(jq -s length "$OUT")" = 1
check "help-json: command is help" envelope help
REG=".github/command-registry.json"
check "help-json: data equals $REG minus _generated" \
  test "$(jq -S '.data' "$OUT")" = "$(jq -S 'del(._generated)' "$ROOT/$REG")"
TOP="$(CGO_ENABLED=0 go run -mod=vendor ./tools/cmdinventory -format names | wc -l | tr -d ' ')"
check "help-json: counts.top_level equals cmdinventory ($TOP)" \
  test "$(jq '.data.counts.top_level' "$OUT")" = "$TOP"
check "help-json: counts.commands equals the commands listed" \
  test "$(jq '.data.counts.commands' "$OUT")" = "$(jq '.data.commands | length' "$OUT")"

# ---- acceptance 3: pilots, both modes ----
# v1.5: one v1 envelope per pilot. v1.4: the pre-contract output, no envelope.
is_envelope_free() { ! grep -q '"schema_version"' "$OUT"; }
for cmd in status doctor; do
  run v15 -- "$cmd" --json
  check "v15 $cmd --json: one v1 envelope" envelope "$cmd"
  check "v15 $cmd --json: data.state present" jq -e '.data.state | type == "string"' "$OUT"
  check "v15 $cmd --json: exit code is a state code (10-12) with the docker stub" rc_in 10 12
done
run v14 -- status --json
check "v14 status --json: bare legacy JSON, no envelope" \
  jq -e 'type == "object" and has("services") and (has("schema_version") | not)' "$OUT"
check "v14 status --json: exit 0" test "$RC" -eq 0
run v14 -- doctor --json
check "v14 doctor --json: legacy report (checks), no envelope, exit 0" \
  rc_out 0 '"checks"'
check "v14 doctor --json: no v1 envelope" is_envelope_free
for sub in "show" "get PROJECT_NAME" "list"; do
  name="config ${sub%% *}"
  # shellcheck disable=SC2086
  run v15 -- config $sub --json
  check "v15 $name --json: one v1 envelope" envelope "$name"
  # shellcheck disable=SC2086
  run v14 -- config $sub --json
  check "v14 $name --json: exit 0, no envelope" rc_out 0 ""
  check "v14 $name --json: output is not a v1 envelope" is_envelope_free
done
# The v1.5 escape hatch reproduces the v1.4 status JSON and the doctor report
# (the free-disk figure of the "Disk space" check differs between two runs).
STABLE='del(.timestamp) | if has("checks") then .checks |= map(if (.name | startswith("Disk space")) then del(.message, .detail) else . end) else . end'
run v14 -- status --json
OLD="$(jq -S "$STABLE" "$OUT" 2>/dev/null || true)"
run v15 NSELF_JSON_LEGACY=1 -- status --json
NEW="$(jq -S "$STABLE" "$OUT" 2>/dev/null || true)"
check "NSELF_JSON_LEGACY=1 reproduces v1.4 status --json" test -n "$OLD" -a "$OLD" = "$NEW"
run v15 -- doctor --json
ENV_DATA="$(jq -S ".data | del(.state) | $STABLE" "$OUT" 2>/dev/null || true)"
run v15 NSELF_JSON_LEGACY=1 -- doctor --json
LEGACY="$(jq -S "$STABLE" "$OUT" 2>/dev/null || true)"
check "NSELF_JSON_LEGACY=1 doctor --json equals the envelope data minus state" \
  test -n "$LEGACY" -a "$LEGACY" = "$ENV_DATA"

# ---- acceptance 4: --json refusal (E402) ----
for mode in v14 v15; do
  run "$mode" -- restart --json
  check "$mode restart --json: exit 1 with [E402] on stderr" rc_err 1 "[E402]"
  if [ "$mode" = v15 ]; then
    check "$mode restart --json: error envelope on stdout" jq -e '.error.code == "E402"' "$OUT"
  else
    check "$mode restart --json: nothing on stdout" test ! -s "$OUT"
  fi
done
run v14 -- config validate --json
if has_err "[E402]"; then fail "v14 config validate --json: behaves as before (no E402)" "stderr mentions E402"
else pass "v14 config validate --json: behaves as before (no E402)"; fi
run v15 -- config validate --json
check "v15 config validate --json: refused with E402" rc_err 1 "[E402]"

# ---- acceptance 5: exit-code contract with a failing docker ----
run v14 -- start
check "v14 start: exit 1 (base behaviour) with the stub docker" test "$RC" -eq 1
if has_err "[E002]"; then fail "v14 start: no [E002] code on stderr" "stderr has E002"; else pass "v14 start: no [E002] code on stderr"; fi
run v15 -- start
check "v15 start: exit 2 with [E002] on stderr" rc_err 2 "[E002]"
# `start` is json: none, so --json is refused by the guard (E402) before docker
# is touched; the E002 error envelope itself is proven in-process by
# cmd/nself's report() test. Here: one error envelope on stdout whose exit_code
# is the process status.
run v15 -- start --json
check "v15 start --json: one error envelope on stdout" \
  one_doc
check "v15 start --json: error.exit_code equals the process status" \
  jq -e --argjson rc "$RC" '.error.exit_code == $rc and (.error.code | startswith("E"))' "$OUT"

printf '%s assertions: %s passed, %s failed\n' "$((PASSED + FAILED))" "$PASSED" "$FAILED" >&2
[ "$FAILED" -eq 0 ] && [ "$PASSED" -gt 0 ]
