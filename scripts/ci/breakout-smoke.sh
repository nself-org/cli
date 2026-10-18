#!/usr/bin/env bash
# Purpose: prove, on two real binaries, the v1.5 break-out relocation of P7-CANON-13:
#          v1.4 is byte-identical to the base binary, installed plugins run the old
#          spellings with the right argv, a missing plugin fails with E404 (switched
#          off, offline, unsigned, tampered), and removed commands fail with E410.
# Usage:   NSELF_BIN=<head nself> NSELF_BASE_BIN=<base nself> bash scripts/ci/breakout-smoke.sh
# Inputs:  NSELF_BIN       the binary under test (executable).
#          NSELF_BASE_BIN  the binary of the merge base (executable).
#          scripts/ci/testdata/breakout/{plugins,registry}: fixture plugin manifests
#          with stub binaries, and an unsigned and a tampered release catalogue.
# Outputs: one "ok" line per assertion on stdout; failures on stderr.
#          Exit 0 only when every assertion held; 1 on a failed assertion; 2 on a
#          usage error or a missing tool.
# Constraints: bash 3.2 compatible; needs curl, jq and python3; touches no network beyond
#          127.0.0.1 (the fixture catalogue server) and never a production key;
#          every process gets a clean HOME and project directory under $TMPDIR,
#          removed on exit. The CLI pins no release key in this build, so the
#          unsigned and tampered catalogues are both refused before any download.
set -euo pipefail

die() { echo "breakout-smoke: $*" >&2; exit 2; }
fail() { echo "FAIL: $*" >&2; FAILED=1; }
ok() { echo "ok   $*"; }
FAILED=0

HEAD_BIN="${NSELF_BIN:-}"
BASE_BIN="${NSELF_BASE_BIN:-}"
[ -x "$HEAD_BIN" ] || die "NSELF_BIN must name an executable"
[ -x "$BASE_BIN" ] || die "NSELF_BASE_BIN must name an executable"
command -v curl >/dev/null 2>&1 || die "needs curl"
command -v jq >/dev/null 2>&1 || die "needs jq"
command -v python3 >/dev/null 2>&1 || die "needs python3"

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
FIX="$ROOT/scripts/ci/testdata/breakout"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/breakout-smoke.XXXXXX")"
SERVER_PID=""
cleanup() {
  if [ -n "$SERVER_PID" ]; then kill "$SERVER_PID" 2>/dev/null || true; wait "$SERVER_PID" 2>/dev/null || true; fi
  rm -rf "$WORK"
}
trap cleanup EXIT
mkdir -p "$WORK/proj"

# run BIN HOME MODE ARGS...: runs the binary in a clean environment; sets RC, OUT, ERR.
run() {
  local bin="$1" home="$2" mode="$3"
  shift 3
  RC=0
  (cd "$WORK/proj" && env -u NSELF_PLUGIN_DIR -u NSELF_PLUGIN_REGISTRY -u NSELF_NO_AUTO_ADD \
    HOME="$home" USERPROFILE="$home" NSELF_V15="$mode" NO_COLOR=1 "$bin" "$@") >"$WORK/out" 2>"$WORK/err" </dev/null || RC=$?
  OUT="$(cat "$WORK/out")"
  ERR="$(cat "$WORK/err")"
}

fresh_home() { local h; h="$(mktemp -d "$WORK/home.XXXXXX")"; echo "$h"; }

installed_home() {
  local h slug; h="$(fresh_home)"
  mkdir -p "$h/.nself/plugins/bin"
  for slug in ci functions infra plugin-dev; do
    cp -R "$FIX/plugins/$slug" "$h/.nself/plugins/$slug"
    cp "$FIX/plugins/bin/nself-$slug" "$h/.nself/plugins/bin/nself-$slug"
    chmod 755 "$h/.nself/plugins/bin/nself-$slug"
  done
  echo "$h"
}

# 1. v1.4, clean HOME: the break-out commands answer exactly like the base binary.
for c in "ci --help" "runner --help" "functions --help" "server --help" "plugin init --help" "plugin marketplace --help"; do
  # shellcheck disable=SC2086 # the command words are split on purpose
  home="$(fresh_home)"; run "$BASE_BIN" "$home" 0 $c; b_rc=$RC; b_out="$OUT"; b_err="$ERR"
  home="$(fresh_home)"; run "$HEAD_BIN" "$home" 0 $c
  if [ "$RC" = "$b_rc" ] && [ "$OUT" = "$b_out" ] && [ "$ERR" = "$b_err" ]; then
    ok "v1.4 'nself $c' is byte-identical to the base binary (exit $RC)"
  else
    fail "v1.4 'nself $c' differs from the base binary (exit $b_rc -> $RC)"
  fi
done

# 2. v1.5 with the plugins installed: the old spelling runs the plugin with the right argv.
HOME_P="$(installed_home)"
expect_stub() { # <want stdout> <args...>
  local want="$1"; shift
  run "$HEAD_BIN" "$HOME_P" 1 "$@"
  if [ "$RC" = 0 ] && [ "$OUT" = "$want" ] && ! printf '%s' "$ERR" | grep -q 'now comes from'; then
    ok "v1.5 'nself $*' runs the installed plugin: $want"
  else
    fail "v1.5 'nself $*': exit $RC, stdout '$OUT', stderr '$ERR', want '$want'"
  fi
}
expect_stub "ci: --help" ci --help
expect_stub "ci: nodes verify x" runner verify x
expect_stub "infra: server list" server list
expect_stub "functions: list" functions list
expect_stub "plugin-dev: init demo" plugin init demo

# help --json lists each installed break-out command as a plugin command (a second
# application of the canon, which the registry build does, must not detach it).
run "$HEAD_BIN" "$HOME_P" 1 help --json
for slug in ci functions infra plugin-dev; do
  if printf '%s' "$OUT" | jq -e --arg p "nself $slug" --arg s "$slug" '[.data.commands[] | select(.path == $p and .canon == "plugin" and .plugin == $s)] | length == 1' >/dev/null 2>&1; then
    ok "v1.5 help --json lists 'nself $slug' with canon plugin from $slug"
  else
    fail "v1.5 help --json does not list 'nself $slug' as a plugin command"
  fi
done

# 3. v1.5, plugin not installed: E404 whatever the reason, never a silent success.
PORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')"
BASE_URL="http://127.0.0.1:$PORT"
for kind in unsigned tampered; do
  mkdir -p "$WORK/served/$kind"
  sed "s#@BASE@#$BASE_URL#g" "$FIX/registry/$kind/registry.json" >"$WORK/served/$kind/registry.json"
done
(cd "$WORK/served" && exec python3 -m http.server "$PORT" --bind 127.0.0.1 >/dev/null 2>&1) &
SERVER_PID=$!
# D-0909: if the fixture server never answers, the unsigned and tampered cases would
# pass on the offline E404 alone. Stop here instead of letting them lie.
READY=0
for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 24 25 26 27 28 29 30 31 32 33 34 35 36 37 38 39 40; do
  if curl -fsS -o /dev/null "$BASE_URL/unsigned/registry.json" 2>/dev/null && curl -fsS -o /dev/null "$BASE_URL/tampered/registry.json" 2>/dev/null; then READY=1; break; fi
  kill -0 "$SERVER_PID" 2>/dev/null || break
  sleep 0.25
done
[ "$READY" = 1 ] || die "the fixture catalogue server on $BASE_URL did not become ready"

# D-0909: every refusal also names its cause. Offline gives the same E404 as a
# refused signature, so a bare exit 1 + [E404] would pass with the server dead.
# <cause> is an extended regex the stderr must match; a leading "!" means it must not.
expect_e404() { # <label> <cause> <env assignment or ""> <args...>
  local label="$1" cause="$2" assign="$3"; shift 3
  local home; home="$(fresh_home)"
  # shellcheck disable=SC2086 # $assign is one NAME=value pair or empty
  RC=0
  (cd "$WORK/proj" && env -u NSELF_PLUGIN_DIR -u NSELF_PLUGIN_REGISTRY -u NSELF_NO_AUTO_ADD $assign \
    HOME="$home" USERPROFILE="$home" NSELF_V15=1 NO_COLOR=1 "$HEAD_BIN" "$@") >"$WORK/out" 2>"$WORK/err" </dev/null || RC=$?
  OUT="$(cat "$WORK/out")"; ERR="$(cat "$WORK/err")"
  if [ "$RC" = 1 ] && printf '%s' "$ERR" | grep -q '\[E404\]'; then
    ok "v1.5 $label: exit 1 with [E404] on stderr"
  else
    fail "v1.5 $label: exit $RC, stderr '$ERR'"
  fi
  case "$cause" in
    '!'*) if printf '%s' "$ERR" | grep -Eqi "${cause#!}"; then fail "v1.5 $label: stderr names a cause it must not (${cause#!}): '$ERR'"; else ok "v1.5 $label: stderr names no signature cause"; fi ;;
    *) if printf '%s' "$ERR" | grep -Eqi "$cause"; then ok "v1.5 $label: stderr names the cause ($cause)"; else fail "v1.5 $label: stderr lacks the cause ($cause): '$ERR'"; fi ;;
  esac
  if [ -e "$home/.nself/plugins/bin/nself-ci" ]; then fail "v1.5 $label: a binary was published"; fi
  # The same refusal as a machine document.
  RC=0
  (cd "$WORK/proj" && env -u NSELF_PLUGIN_DIR -u NSELF_PLUGIN_REGISTRY -u NSELF_NO_AUTO_ADD $assign \
    HOME="$home" USERPROFILE="$home" NSELF_V15=1 NO_COLOR=1 "$HEAD_BIN" "$@" --json) >"$WORK/out" 2>"$WORK/err" </dev/null || RC=$?
  OUT="$(cat "$WORK/out")"
  if [ "$RC" = 1 ] && printf '%s' "$OUT" | jq -e '.error.code == "E404" and (.error.remediation | contains("nself add ci"))' >/dev/null 2>&1; then
    ok "v1.5 $label --json: .error.code E404 with remediation naming 'nself add ci'"
  else
    fail "v1.5 $label --json: exit $RC, stdout '$OUT'"
  fi
}
expect_e404 "NSELF_NO_AUTO_ADD=1" "switched off" "NSELF_NO_AUTO_ADD=1" ci --check
expect_e404 "offline registry" "!release signature|cannot be verified|switched off" "NSELF_PLUGIN_REGISTRY=http://127.0.0.1:9" ci --check
expect_e404 "unsigned release" "no release signature" "NSELF_PLUGIN_REGISTRY=$BASE_URL/unsigned" ci --check
expect_e404 "tampered release (refused before download: no key pinned; the in-process test proves the checksum and signature)" "cannot be verified" "NSELF_PLUGIN_REGISTRY=$BASE_URL/tampered" runner verify x

# 4. v1.5: the marketplace is gone.
for c in "plugin marketplace" "plugin submit"; do
  # shellcheck disable=SC2086
  run "$HEAD_BIN" "$(fresh_home)" 1 $c
  if [ "$RC" = 1 ] && printf '%s' "$ERR" | grep -q '\[E410\]' && printf '%s' "$ERR" | grep -q 'no plugin marketplace'; then
    ok "v1.5 'nself $c' exits 1 with [E410] and the relocation text"
  else
    fail "v1.5 'nself $c': exit $RC, stderr '$ERR'"
  fi
done

[ "$FAILED" = 0 ] || exit 1
echo "breakout-smoke: all assertions held"
