#!/usr/bin/env bash
# Helpers for scripts/drill/two-command.sh (sourced, never run).
#
# - run_leg_a / run_leg_b: run one command under the two prompt guards of EPIC
#   P7-CANON D14. Leg A: stdin /dev/null, stdout to a file (no TTY). Leg B: a
#   pseudo-TTY whose input never produces data, so any prompt blocks until the
#   timeout (exit 124).
# - write_results: drill.json (two-command-drill/v1) and perfbench.json
#   (perfbench/v1, metric time_to_healthy, unit s, informational only).
# - graphql_probe: one authenticated `{ __typename }` query against Hasura.
#
# Bash 3.2 compatible. Needs timeout (or gtimeout) and jq; leg B also needs python3 where util-linux script is absent (macOS).

# Strip the variables that would switch the CLI into non-interactive mode, so a
# real prompt can appear and be caught.
drill_env() { env -u CI -u GITHUB_ACTIONS -u NSELF_NONINTERACTIVE "$@"; }

TIMEOUT_BIN=$(command -v timeout || command -v gtimeout || true)

# run_leg_a <seconds> <logfile> <cmd...>: exit status of the command, 124 on timeout.
run_leg_a() {
  local secs=$1 log=$2
  shift 2
  drill_env "$TIMEOUT_BIN" -k 5 "$secs" "$@" </dev/null >"$log" 2>&1
}

# run_leg_b <seconds> <logfile> <cmd...>: same, under a pseudo-TTY that gets no input.
# The stdin is a FIFO held open read-write by this shell, so it never reaches EOF.
run_leg_b() {
  local secs=$1 log=$2 fifo rc
  shift 2
  fifo=$(mktemp -u "${TMPDIR:-/tmp}/drill-in.XXXXXX")
  mkfifo "$fifo"
  exec 9<>"$fifo"
  if script --version >/dev/null 2>&1; then
    # util-linux script: -q quiet, -e return the child's status, -c command.
    drill_env "$TIMEOUT_BIN" -k 5 "$secs" script -qec "$(drill_quote "$@")" /dev/null <&9 >"$log" 2>&1
  else
    # BSD/macOS script insists on a real TTY for stdin, so the local run uses
    # python3's pty module instead: same pseudo-TTY, same unanswered input.
    drill_env "$TIMEOUT_BIN" -k 5 "$secs" python3 -c \
      'import os,pty,sys; sys.exit(os.waitstatus_to_exitcode(pty.spawn(sys.argv[1:])))' \
      "$@" <&9 >"$log" 2>&1
  fi
  rc=$?
  exec 9<&-
  rm -f "$fifo"
  return $rc
}

# drill_quote <args...>: one shell-quoted command line for `script -c`.
drill_quote() {
  local out="" a
  for a in "$@"; do out="$out '$(printf '%s' "$a" | sed "s/'/'\\\\''/g")'"; done
  printf '%s' "${out# }"
}

# detect_prompt <seconds> <cmd...>: succeeds when the command is stuck on input
# under leg B (timeout) or fails under leg A (EOF on stdin). Prints the verdict.
detect_prompt() {
  local secs=$1 log rc
  shift
  log=$(mktemp "${TMPDIR:-/tmp}/drill-detect.XXXXXX")
  rc=0
  run_leg_b "$secs" "$log" "$@" || rc=$?
  rm -f "$log"
  [ "$rc" -eq 124 ] || [ "$rc" -eq 137 ]
}

# graphql_probe <env-file> <url>: 0 when Hasura answers `{ __typename }`.
graphql_probe() {
  local envf=$1 url=$2 secret hdr body
  secret=$(sed -n 's/^HASURA_GRAPHQL_ADMIN_SECRET=//p' "$envf" 2>/dev/null | head -n 1)
  hdr=$(mktemp "${TMPDIR:-/tmp}/drill-hdr.XXXXXX")
  chmod 600 "$hdr"
  printf 'x-hasura-admin-secret: %s\n' "$secret" >"$hdr"
  body=$(curl -sS -m 10 -H @"$hdr" -H 'content-type: application/json' \
    -d '{"query":"{ __typename }"}' "$url" 2>/dev/null || true)
  rm -f "$hdr"
  printf '%s' "$body" | jq -e '.data.__typename == "query_root"' >/dev/null 2>&1
}

# write_results <outdir> <mode> <seconds> <graphql_ok> <prompt_detected> <leg_a> <leg_b>
write_results() {
  local out=$1 mode=$2 secs=$3 gql=$4 prompt=$5 la=$6 lb=$7 sha goos goarch
  mkdir -p "$out"
  sha=${GITHUB_SHA:-$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse HEAD 2>/dev/null || true)}
  goos=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$(uname -m)" in x86_64) goarch=amd64 ;; aarch64 | arm64) goarch=arm64 ;; *) goarch=$(uname -m) ;; esac
  jq -n --arg mode "$mode" --argjson s "$secs" --argjson g "$gql" --argjson p "$prompt" \
    --arg la "$la" --arg lb "$lb" \
    '{schema:"two-command-drill/v1", mode:$mode, time_to_healthy_s:$s, graphql_ok:$g,
      prompt_detected:$p, leg_a:$la, leg_b:$lb}' >"$out/drill.json"
  # No measurement (the stack never got healthy): no perfbench result.
  [ "$secs" = null ] && return 0
  jq -n --arg sha "$sha" --arg goos "$goos" --arg goarch "$goarch" --argjson s "$secs" \
    '{schema:"perfbench/v1", scenario:"two-command-drill", sha:$sha, goos:$goos, goarch:$goarch,
      runs:1, injected:false,
      metrics:[{name:"time_to_healthy", unit:"s", p50:$s, p95:$s, max:$s, n:1}]}' >"$out/perfbench.json"
}
