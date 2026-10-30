#!/usr/bin/env bash
# run.sh — the MCP-only agent E2E harness (P7-SHIP-02, SHIP Epic D7).
#
# Modes (run from the cli repo root):
#   run.sh --replay [--transcript FILE]
#       Build nself, start two loopback sim hosts (qa tier local-servers,
#       prod tier prod) and replay test/e2e/agent/testdata/replay.jsonl over
#       `nself mcp` in the sandbox. Writes the transcript (default
#       $AGENT_E2E_OUT/replay.jsonl, else $TMPDIR/agent-e2e-out) and verifies it.
#       Needs Docker.
#   run.sh --live --transcript FILE [--prod-probe DIR]
#       The same fleet and sandbox, driven by a headless `claude -p` session
#       whose only tools are the sandboxed nself server (P7-SHIP-13).
#       --prod-probe has the harness (never the agent) run a read-only
#       `docker inspect` of every container on $NSELF_PROD_PROBE_HOST before
#       and after, into DIR, and fails on any difference. Needs Docker.
#   run.sh --verify-transcript FILE
#       Exit 0 when FILE satisfies the D7 transcript rules, else 1.
#   run.sh --self-test
#       Unit tests of the verifier and replay file, then proves mcp-sandbox.sh
#       refuses a foreign host, SSH_AUTH_SOCK and a reachable ~/.ssh.
#   run.sh --tools-list BIN [--env K=V]...
#       Start BIN's `nself mcp` in the sandbox and print tools/list as a JSON
#       array sorted by name ({name, inputSchema}).
#
# Exit codes: 0 ok, 1 failed or refused, 2 usage.
# Bash 3.2 compatible.

set -u

ROOT=$(cd "$(dirname "$0")/../../.." && pwd)
PKG=./test/e2e/agent/
SANDBOX="$ROOT/scripts/e2e/agent/mcp-sandbox.sh"
GOTEST="go test -mod=vendor -tags e2eagent -count=1"

usage() { sed -n '2,28p' "$0" >&2; exit 2; }
fail() { printf 'agent-e2e: %s\n' "$1" >&2; exit 1; }

scratch() { mktemp -d "${TMPDIR:-/tmp}/agent-e2e.XXXXXX"; }

need_docker() {
  command -v docker >/dev/null 2>&1 || fail "Docker is required (no docker CLI)"
  v=$(docker info --format '{{.ServerVersion}}' 2>/dev/null)
  [ -n "$v" ] || fail "Docker is required (the daemon is not reachable)"
}

build_bin() { # out
  (cd "$ROOT" && CGO_ENABLED=0 go build -mod=vendor -o "$1" ./cmd/nself) || fail "go build failed"
}

abspath() { (cd "$(dirname "$1")" && printf '%s/%s' "$(pwd)" "$(basename "$1")"); }

mode_replay() {
  out="${AGENT_E2E_OUT:-${TMPDIR:-/tmp}/agent-e2e-out}"
  transcript="$out/replay.jsonl"
  while [ $# -gt 0 ]; do
    case "$1" in
      --transcript) transcript="${2:-}"; shift 2 ;;
      *) usage ;;
    esac
  done
  need_docker
  mkdir -p "$(dirname "$transcript")" || fail "cannot create $(dirname "$transcript")"
  transcript=$(abspath "$transcript")
  tmp=$(scratch); bin="$tmp/nself"
  build_bin "$bin"
  # GOTMPDIR becomes the test's TMPDIR: keep the fixture project under TMPDIR
  # (a Colima VM shares only $HOME, so bind mounts elsewhere fail).
  (cd "$ROOT" && GOTMPDIR="$tmp" INTEGRATION=1 NSELF_V15=1 AGENT_E2E_BIN="$bin" AGENT_E2E_TRANSCRIPT_OUT="$transcript" \
    $GOTEST -timeout 60m -v -run '^TestAgentReplay$' "$PKG")
  rc=$?
  rm -rf "$tmp"
  [ $rc -eq 0 ] || fail "replay failed; transcript: $transcript"
  printf 'agent-e2e: replay passed; transcript: %s\n' "$transcript"
}

prod_probe() { # dir label
  [ -n "${NSELF_PROD_PROBE_HOST:-}" ] || fail "--prod-probe needs NSELF_PROD_PROBE_HOST (never committed)"
  ssh -o BatchMode=yes "$NSELF_PROD_PROBE_HOST" \
    'docker ps -q | xargs -r docker inspect --format "{{.Name}} {{.Id}} {{.State.StartedAt}} {{.RestartCount}}" | sort' \
    > "$1/$2.txt" || fail "prod probe ($2) failed"
}

mode_live() {
  transcript="" probe=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --transcript) transcript="${2:-}"; shift 2 ;;
      --prod-probe) probe="${2:-}"; shift 2 ;;
      *) usage ;;
    esac
  done
  [ -n "$transcript" ] || usage
  need_docker
  command -v claude >/dev/null 2>&1 || fail "claude is not on PATH"
  mkdir -p "$(dirname "$transcript")" || fail "cannot create $(dirname "$transcript")"
  transcript=$(abspath "$transcript")
  if [ -n "$probe" ]; then mkdir -p "$probe" && prod_probe "$probe" before; fi
  tmp=$(scratch); bin="$tmp/nself"
  build_bin "$bin"
  (cd "$ROOT" && GOTMPDIR="$tmp" INTEGRATION=1 NSELF_V15=1 AGENT_E2E_LIVE=1 AGENT_E2E_BIN="$bin" AGENT_E2E_TRANSCRIPT_OUT="$transcript" \
    $GOTEST -timeout 90m -v -run '^TestAgentLive$' "$PKG")
  rc=$?
  rm -rf "$tmp"
  if [ -n "$probe" ]; then
    prod_probe "$probe" after
    diff "$probe/before.txt" "$probe/after.txt" > "$probe/diff.txt" || fail "production changed during the live run: $probe/diff.txt"
  fi
  [ $rc -eq 0 ] || fail "live run failed; transcript: $transcript"
}

mode_verify() {
  [ $# -eq 1 ] || usage
  [ -f "$1" ] || fail "no transcript at $1"
  f=$(abspath "$1")
  (cd "$ROOT" && AGENT_E2E_TRANSCRIPT="$f" $GOTEST -run '^TestVerifyTranscriptFile$' "$PKG") || fail "transcript $1 does not verify"
}

expect_refusal() { # name want-substring sandbox-args...
  name="$1"; want="$2"; shift 2
  err=$("$@" 2>&1 >/dev/null); rc=$?
  [ $rc -eq 1 ] || fail "self-test $name: want exit 1, got $rc ($err)"
  case "$err" in *"$want"*) ;; *) fail "self-test $name: refusal does not name $want: $err" ;; esac
  printf 'agent-e2e: self-test %s: refused as expected\n' "$name"
}

mode_self_test() {
  (cd "$ROOT" && $GOTEST -run '^(TestVerifyFixtures|TestReplayFile)$' "$PKG") || fail "unit tests failed"
  tmp=$(scratch)
  op="$tmp/operator"; mkdir -p "$op/.ssh" "$tmp/project"
  mk_home() { d="$tmp/$1"; mkdir -p "$d/.ssh"; printf 'Host 127.0.0.1\n  IdentitiesOnly yes\n' > "$d/.ssh/config"; printf '%s' "$d"; }
  base="--project $tmp/project --operator-home $op"
  h=$(mk_home clean)
  # shellcheck disable=SC2086
  env -u SSH_AUTH_SOCK bash "$SANDBOX" --home "$h" $base -- true || fail "self-test clean: a clean sandbox was refused"
  printf 'agent-e2e: self-test clean: accepted\n'
  h=$(mk_home foreign); printf 'Host sim\n  HostName 192.0.2.10\n' >> "$h/.ssh/config"
  # shellcheck disable=SC2086
  expect_refusal foreign-host 192.0.2.10 env -u SSH_AUTH_SOCK bash "$SANDBOX" --home "$h" $base -- true
  h=$(mk_home inventory); printf 'environments:\n  prod:\n    servers:\n      - host: deploy@db.example.com:22\n' > "$tmp/inv.yaml"
  # shellcheck disable=SC2086
  expect_refusal foreign-inventory db.example.com env -u SSH_AUTH_SOCK bash "$SANDBOX" --home "$h" $base --inventory "$tmp/inv.yaml" -- true
  h=$(mk_home agent)
  # shellcheck disable=SC2086
  expect_refusal ssh-auth-sock SSH_AUTH_SOCK env SSH_AUTH_SOCK="$tmp/agent.sock" bash "$SANDBOX" --home "$h" $base -- true
  h=$(mk_home link); ln -s "$op/.ssh" "$h/operator-ssh"
  # shellcheck disable=SC2086
  expect_refusal operator-ssh "$h/operator-ssh" env -u SSH_AUTH_SOCK bash "$SANDBOX" --home "$h" $base -- true
  rm -rf "$tmp"
  printf 'agent-e2e: self-test passed\n'
}

mode_tools_list() {
  [ $# -ge 1 ] || usage
  bin=$(abspath "$1"); shift
  envs=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --env) envs="${envs:+$envs,}${2:-}"; shift 2 ;;
      *) usage ;;
    esac
  done
  [ -x "$bin" ] || fail "$bin is not an executable"
  tmp=$(scratch)
  (cd "$ROOT" && AGENT_E2E_BIN="$bin" AGENT_E2E_TOOLS_OUT="$tmp/tools.json" AGENT_E2E_ENV="$envs" \
    $GOTEST -run '^TestToolsList$' "$PKG" >&2) || { rm -rf "$tmp"; fail "tools/list failed"; }
  cat "$tmp/tools.json"
  rm -rf "$tmp"
}

[ $# -ge 1 ] || usage
mode="$1"; shift
case "$mode" in
  --replay) mode_replay "$@" ;;
  --live) mode_live "$@" ;;
  --verify-transcript) mode_verify "$@" ;;
  --self-test) mode_self_test "$@" ;;
  --tools-list) mode_tools_list "$@" ;;
  -h|--help) usage ;;
  *) usage ;;
esac
