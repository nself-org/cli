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
#          usage error or a missing tool; also 1 when the fixture server never answers.
# Constraints: bash 3.2 compatible; needs jq, python3 and curl; touches no network beyond
#          127.0.0.1 (the fixture catalogue server) and never a production key;
#          every process gets a clean HOME and project directory under $TMPDIR,
#          removed on exit. Each E404 case also asserts its cause text, so a dead
#          fixture server cannot pass as an unsigned or tampered refusal. The CLI
#          pins the real free release key, so a signed entry is judged after the
#          download: the fixture server serves a stand-in tarball, a wrong checksum
#          is refused with E124, and a right checksum with a bad signature by the
#          pinned key id is refused with E121.
set -euo pipefail

die() { echo "breakout-smoke: $*" >&2; exit 2; }
fail() { echo "FAIL: $*" >&2; FAILED=1; }
ok() { echo "ok   $*"; }
FAILED=0

HEAD_BIN="${NSELF_BIN:-}"
BASE_BIN="${NSELF_BASE_BIN:-}"
[ -x "$HEAD_BIN" ] || die "NSELF_BIN must name an executable"
[ -x "$BASE_BIN" ] || die "NSELF_BASE_BIN must name an executable"
command -v jq >/dev/null 2>&1 || die "needs jq"
command -v python3 >/dev/null 2>&1 || die "needs python3"
command -v curl >/dev/null 2>&1 || die "needs curl"

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
# A stand-in tarball at the fallback URL the CLI tries after the primary 404s, so a signed entry is judged on its checksum and signature, not on a failed download.
mkdir -p "$WORK/served/releases/download/v1.0.0"
printf 'breakout-smoke stand-in tarball, not a real release\n' >"$WORK/served/releases/download/v1.0.0/ci-1.0.0.tar.gz"
TARBALL_SHA="$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$WORK/served/releases/download/v1.0.0/ci-1.0.0.tar.gz")"
# badsig: the right checksum and a zero signature under the pinned free key id, so only the signature check can refuse it.
mkdir -p "$WORK/served/badsig"
jq --arg sha "$TARBALL_SHA" --arg sig "$(python3 -c 'import base64; print(base64.b64encode(bytes(64)).decode())')" \
  '.plugins[0].checksum = $sha | .plugins[0].release_signature = {key_id: "plugins-2ebca8dfad98e19e", alg: "ed25519", sig: $sig}' \
  "$WORK/served/tampered/registry.json" >"$WORK/served/badsig/registry.json"
(cd "$WORK/served" && exec python3 -m http.server "$PORT" --bind 127.0.0.1 >/dev/null 2>&1) &
SERVER_PID=$!
READY=0
for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
  if curl -fsS -o /dev/null "$BASE_URL/unsigned/registry.json" 2>/dev/null && curl -fsS -o /dev/null "$BASE_URL/tampered/registry.json" 2>/dev/null; then READY=1; break; fi
  kill -0 "$SERVER_PID" 2>/dev/null || break
  sleep 0.25
done
if [ "$READY" != 1 ]; then
  echo "FAIL: the fixture catalogue server on $BASE_URL never became ready; the unsigned and tampered cases cannot run" >&2
  exit 1
fi

expect_e404() { # <label> <cause regex for stderr> <env assignment or ""> <args...>
  local label="$1" cause="$2" assign="$3"; shift 3
  local home; home="$(fresh_home)"
  # shellcheck disable=SC2086 # $assign is one NAME=value pair or empty
  RC=0
  (cd "$WORK/proj" && env -u NSELF_PLUGIN_DIR -u NSELF_PLUGIN_REGISTRY -u NSELF_NO_AUTO_ADD $assign \
    HOME="$home" USERPROFILE="$home" NSELF_V15=1 NO_COLOR=1 "$HEAD_BIN" "$@") >"$WORK/out" 2>"$WORK/err" </dev/null || RC=$?
  OUT="$(cat "$WORK/out")"; ERR="$(cat "$WORK/err")"
  if [ "$RC" = 1 ] && printf '%s' "$ERR" | grep -q '\[E404\]' && printf '%s' "$ERR" | grep -Eq "$cause"; then
    ok "v1.5 $label: exit 1 with [E404] on stderr, cause matches '$cause'"
  else
    fail "v1.5 $label: exit $RC, want [E404] and cause '$cause', stderr '$ERR'"
  fi
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
expect_e404 "NSELF_NO_AUTO_ADD=1" 'automatic install is switched off' "NSELF_NO_AUTO_ADD=1" ci --check
expect_e404 "offline registry" 'registry fetch failed' "NSELF_PLUGIN_REGISTRY=http://127.0.0.1:9" ci --check
expect_e404 "unsigned release" '\[E107\].*has no release signature' "NSELF_PLUGIN_REGISTRY=$BASE_URL/unsigned/registry.json" ci --check
expect_e404 "tampered release (checksum mismatch, refused before extraction)" '\[E124\].*checksum mismatch' "NSELF_PLUGIN_REGISTRY=$BASE_URL/tampered/registry.json" runner verify x
expect_e404 "bad signature (right checksum, zero signature under the pinned free key)" '\[E121\]' "NSELF_PLUGIN_REGISTRY=$BASE_URL/badsig/registry.json" runner verify x

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
