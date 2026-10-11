#!/usr/bin/env bash
# Purpose: prove Epic P7-LIVE acceptance 1-4 (live safety) on the real binary and
#          print one PASS or FAIL line per assertion:
#            1. a prod-class change is planned without writing, refused without
#               --yes (E403, exit 4) and applied with it (scripts/ci/live-safety-fixture.sh)
#            2. a hand-edited marked nginx conf survives build, config set and
#               doctor --fix without --force; the diff is printed; an unmarked
#               foreign conf is never touched
#            3. config set BASE_DOMAIN on dev regenerates nginx and compose with
#               no separate build; --preview writes nothing
#            4. doctor reports drift; --fix repairs it; a second --fix changes nothing
#          The cascade integration suite (internal/reconcile/cascade_integration_test.go)
#          covers the same ground in depth; this script is the fast, no-Go-test smoke.
# Usage:   bash scripts/ci/live-safety-smoke.sh [path-to-nself-binary]
# Inputs:  optional binary (built with go into a temp dir when absent); python3 on
#          PATH. Docker is used only by acceptance 1 (container impact, and the
#          plan-after-apply check of 3); without it those lines print SKIP.
# Outputs: one line per assertion, "PASS <name>", "FAIL <name>" or
#          "SKIP <name> (<reason>)"; the last line is "LIVE-SAFETY-SMOKE: PASS" or
#          "LIVE-SAFETY-SMOKE: FAIL". Exit 1 when any assertion FAILed (all lines
#          still print), when nothing passed, or when a tool is missing. A SKIP is
#          never a PASS.
# Constraints: bash 3.2 compatible. Fixture projects, HOME and plugin dirs live in
#          one temp dir that is removed on exit; no real project, host or remote
#          is touched. Docker is never stopped or started.
set -uo pipefail

cd "$(dirname "$0")/../.." || exit 1
ROOT="$PWD"
FX="$ROOT/internal/reconcile/testdata/fixtures"
REAL_HOME="$HOME"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/live-safety-smoke.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

PASSED=0
FAILED=0
SKIPPED=0
pass() { printf 'PASS %s\n' "$1"; PASSED=$((PASSED + 1)); }
fail() {
  printf 'FAIL %s\n' "$1"
  FAILED=$((FAILED + 1))
  if [ -n "${2:-}" ]; then printf '  %s\n' "$2" >&2; fi
}
skip() { printf 'SKIP %s (%s)\n' "$1" "$2"; SKIPPED=$((SKIPPED + 1)); }
# check <name> <command...>: PASS when the command exits 0.
check() {
  local name="$1"
  shift
  if "$@" >/dev/null 2>&1; then pass "$name"; else fail "$name" "$(printf 'check failed: %.200s' "$*")"; fi
}
finish() {
  printf '%s assertions: %s passed, %s failed, %s skipped\n' "$((PASSED + FAILED + SKIPPED))" "$PASSED" "$FAILED" "$SKIPPED" >&2
  if [ "$FAILED" -eq 0 ] && [ "$PASSED" -gt 0 ]; then
    echo "LIVE-SAFETY-SMOKE: PASS"
    exit 0
  fi
  echo "LIVE-SAFETY-SMOKE: FAIL"
  exit 1
}

command -v python3 >/dev/null 2>&1 || { fail "tool-python3-missing"; finish; }

# ---- the binary ----------------------------------------------------------------
BIN="${1:-}"
if [ -z "$BIN" ]; then
  command -v go >/dev/null 2>&1 || { fail "tool-go-missing (pass a binary path to skip the build)"; finish; }
  mkdir -p "$WORK/bin"
  BIN="$WORK/bin/nself"
  if CGO_ENABLED=0 go build -mod=vendor -buildvcs=false -o "$BIN" ./cmd/nself 2>"$WORK/build.err"; then
    pass "build the nself binary"
  else
    cat "$WORK/build.err" >&2
    fail "build the nself binary"
    finish
  fi
fi
case "$BIN" in /*) ;; *) BIN="$ROOT/$BIN" ;; esac
[ -x "$BIN" ] || { fail "binary $BIN is not executable"; finish; }

HAVE_DOCKER=0
if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then HAVE_DOCKER=1; fi

# ---- helpers ----------------------------------------------------------------
# mkproj <name> <fixture>: a project with HOME and the plugin dir beside it
# (the same layout as live-safety-fixture.sh).
mkproj() {
  local d="$WORK/$1" f="$FX/$2" c
  mkdir -p "$d/project/.nself" "$d/home" "$d/plugins"
  cat "$f/project/dotenv" "$FX/secrets.env" > "$d/project/.env"
  chmod 600 "$d/project/.env"
  if [ -d "$f/plugins" ]; then cp -R "$f/plugins/." "$d/plugins/"; fi
  if [ -f "$f/certs.txt" ]; then
    # shellcheck disable=SC2013
    for c in $(cat "$f/certs.txt"); do
      mkdir -p "$d/project/ssl/certificates/$c"
      echo CERT > "$d/project/ssl/certificates/$c/fullchain.pem"
      echo KEY > "$d/project/ssl/certificates/$c/privkey.pem"
    done
  fi
}

# tree <name> [subdir...]: one hash of every directory and file (content) under
# the given subdirs of the project root (default: all), minus the lock file.
tree() {
  local name="$1" subs p
  shift
  subs="$*"
  [ -n "$subs" ] || subs="."
  (cd "$WORK/$name" && {
    for p in $subs; do
      find "$p" -type d
      find "$p" -type f ! -name op.lock | LC_ALL=C sort | while IFS= read -r f; do
        printf '%s %s\n' "$(python3 -c 'import hashlib,sys;print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$f")" "$f"
      done
    done
  } | LC_ALL=C sort | python3 -c 'import hashlib,sys;print(hashlib.sha256(sys.stdin.buffer.read()).hexdigest())')
}

# run_nself <name> <v15:0|1> <args...>: run in the project; stdout in $WORK/out,
# stderr in $WORK/err, exit status in $RC. The mode is set on the child only.
RC=0
run_nself() {
  local name="$1" v15="$2" envs="NSELF_CMD_LOG_ENABLED=false"
  shift 2
  if [ "$v15" = 1 ]; then envs="$envs NSELF_V15=1"; fi
  RC=0
  # shellcheck disable=SC2086
  (cd "$WORK/$name/project" && env -u NSELF_V15 $envs HOME="$WORK/$name/home" NSELF_PLUGIN_DIR="$WORK/$name/plugins" \
    DOCKER_CONFIG="${DOCKER_CONFIG:-$REAL_HOME/.docker}" AI_AUTO_INSTALL=false "$BIN" "$@" > "$WORK/out" 2> "$WORK/err" < /dev/null) || RC=$?
}
allout() { cat "$WORK/out" "$WORK/err"; }
has() { grep -qF -- "$1" "$WORK/out" "$WORK/err"; }
# jget <python-expr over d>: evaluate against the JSON on $WORK/out (data field).
jget() {
  python3 -c 'import json,sys;d=json.load(open(sys.argv[1]));d=d.get("data",d);print(eval(sys.argv[2]))' "$WORK/out" "$1" 2>/dev/null
}

# ---- acceptance 1: prod class (live-safety-fixture.sh on this binary) --------------
if bash "$ROOT/scripts/ci/live-safety-fixture.sh" "$BIN" > "$WORK/fx.out" 2> "$WORK/fx.err"; then FXRC=0; else FXRC=$?; fi
while IFS= read -r line; do
  case "$line" in "ok "*) pass "acc1 ${line#ok }" ;; esac
done < "$WORK/fx.out"
if [ "$FXRC" -ne 0 ]; then
  fail "acc1 live-safety-fixture.sh exited $FXRC" "$(grep -m1 '^FAIL' "$WORK/fx.err" || tail -n 2 "$WORK/fx.err" | tr '\n' ' ')"
elif grep -qx 'LIVE-SAFETY-FIXTURE: PASS' "$WORK/fx.out"; then
  pass "acc1 live-safety-fixture.sh completed"
else
  fail "acc1 live-safety-fixture.sh printed no PASS line"
fi
if [ "$HAVE_DOCKER" = 0 ]; then
  skip "acc1 plan starts, stops and creates no container" "docker not on PATH"
  skip "acc1 container impact: stale config hash plans recreate" "docker not on PATH"
else
  grep -qx 'ok plan starts, stops and creates no container' "$WORK/fx.out" \
    || fail "acc1 plan starts, stops and creates no container: the fixture did not assert it"
  grep -qx 'ok container impact: stale config hash plans recreate' "$WORK/fx.out" \
    || skip "acc1 container impact: stale config hash plans recreate" "no cached docker image to create the scratch container from"
fi

# The plan is one v1 envelope that matches change-plan v1 (contract:cli.change-plan).
mkproj p1 prod-plugins
H0="$(tree p1)"
run_nself p1 1 build --plan --json
check "acc1 build --plan --json exits 0" test "$RC" -eq 0
check "acc1 plan envelope: schema_version 1 and command build" \
  python3 -c 'import json,sys;d=json.load(open(sys.argv[1]));assert d["schema_version"]=="1" and d["command"]=="build"' "$WORK/out"
check "acc1 plan data matches change-plan v1 (keys and types)" python3 - "$WORK/out" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))["data"]
want = {"schema_version": str, "plan_id": str, "command": str, "trigger": dict, "env": str, "env_class": str,
        "artifacts": list, "effects": list, "containers": dict, "destructive": bool,
        "destructive_reasons": list, "requires_confirmation": bool, "empty": bool}
for k, t in want.items():
    assert isinstance(d[k], t), k
assert d["schema_version"] == "1" and d["env_class"] == "prod" and d["requires_confirmation"] is True
assert len(d["artifacts"]) > 5 and set(d["containers"]) >= {"known", "items"}
for a in d["artifacts"]:
    assert set(a) >= {"kind", "path", "action", "generated", "hand_edited", "diff_lines", "redacted"}
PY
check "acc1 build --plan leaves the project tree, HOME and plugin dir unchanged" test "$(tree p1)" = "$H0"
run_nself p1 1 build --json
check "acc1 non-interactive prod build without --yes: exit 4 and E403" \
  python3 -c 'import json,sys;assert json.load(open(sys.argv[1]))["error"]["code"]=="E403"; assert int(sys.argv[2])==4' "$WORK/out" "$RC"
check "acc1 the refused prod build wrote nothing" test "$(tree p1)" = "$H0"
run_nself p1 1 build --yes
check "acc1 build --yes applies (exit 0)" test "$RC" -eq 0
if [ "$HAVE_DOCKER" = 1 ]; then
  run_nself p1 1 build --plan --json
  check "acc1 the plan after --yes is empty" test "$(jget 'd["empty"]')" = True
else
  skip "acc1 the plan after --yes is empty" "docker not on PATH: container state unknown, so a plan is never empty"
fi

# ---- acceptance 2: hand-edited marked conf, v1.5 ----------------------------------
CONF="nginx/sites/api-docs.conf"
FOREIGN_BODY='server { listen 8089; server_name foreign.invalid; return 204; }'
mkproj h dev-minimal
run_nself h 1 build
check "acc2 setup build exits 0" test "$RC" -eq 0
printf '# hand edit\n' >> "$WORK/h/project/$CONF"
printf '%s\n' "$FOREIGN_BODY" > "$WORK/h/project/nginx/sites/foreign.conf"
HE="$(tree h project plugins)"
for cmd in "build" "config set BASE_DOMAIN moved.example.org" "doctor --fix"; do
  # shellcheck disable=SC2086
  run_nself h 1 $cmd
  check "acc2 $cmd refuses without --force: exit 4 and E403" test "$RC" -eq 4 -a "$(allout | grep -c E403)" -gt 0
  check "acc2 $cmd prints the diff of the hand edit" test "$(allout | grep -cF -- '--- a/'"$CONF")" -gt 0 -a "$(allout | grep -cF -- '-# hand edit')" -gt 0
  check "acc2 $cmd writes nothing" test "$(tree h project plugins)" = "$HE"
done
check "acc2 the hand edit and the foreign conf survived" test "$(grep -c '# hand edit' "$WORK/h/project/$CONF")" -eq 1 -a "$(cat "$WORK/h/project/nginx/sites/foreign.conf")" = "$FOREIGN_BODY"
run_nself h 1 build --force
check "acc2 build --force overwrites the hand edit" test "$RC" -eq 0 -a "$(grep -c '# hand edit' "$WORK/h/project/$CONF")" -eq 0
check "acc2 the unmarked foreign conf is never touched, even by --force" test "$(cat "$WORK/h/project/nginx/sites/foreign.conf")" = "$FOREIGN_BODY"

# ---- acceptance 3: config set cascades on dev, v1.5 ----------------------------------
NEWDOM="moved.example.org"
mkproj c dev-minimal
run_nself c 1 build
C0="$(tree c)"
run_nself c 1 config set BASE_DOMAIN "$NEWDOM" --preview
check "acc3 --preview exits 0" test "$RC" -eq 0
check "acc3 --preview writes nothing (tree hash including .env)" test "$(tree c)" = "$C0"
run_nself c 1 config set BASE_DOMAIN "$NEWDOM"
check "acc3 config set exits 0 with no separate build" test "$RC" -eq 0
SITES_OK=1
SITE_COUNT=0
for f in "$WORK"/c/project/nginx/sites/*.conf; do
  [ -f "$f" ] || continue
  SITE_COUNT=$((SITE_COUNT + 1))
  if ! grep -qF "$NEWDOM" "$f" || grep -qF "example.test" "$f"; then SITES_OK=0; fi
done
check "acc3 every nginx site conf names the new domain and not the old" test "$SITES_OK" -eq 1 -a "$SITE_COUNT" -gt 0
check "acc3 docker-compose.yml names the new domain and not the old" \
  test "$(grep -cF "$NEWDOM" "$WORK/c/project/docker-compose.yml")" -gt 0 -a "$(grep -cF 'example.test' "$WORK/c/project/docker-compose.yml")" -eq 0
if [ "$HAVE_DOCKER" = 1 ]; then
  run_nself c 1 build --plan --json
  check "acc3 the plan afterwards is empty" test "$(jget 'd["empty"]')" = True
else
  skip "acc3 the plan afterwards is empty" "docker not on PATH: container state unknown, so a plan is never empty"
fi

# ---- acceptance 4: doctor drift and --fix ----------------------------------------
mkproj d dev-minimal
run_nself d 1 build
cp "$WORK/d/project/docker-compose.yml" "$WORK/compose.good"
cp "$WORK/d/project/nginx/sites/auth.conf" "$WORK/auth.good"
printf '# corrupted\n' >> "$WORK/d/project/docker-compose.yml"
rm -f "$WORK/d/project/nginx/sites/auth.conf"
run_nself d 1 doctor
check "acc4 doctor reports drift of both files" test "$(allout | grep 'Generated files' | grep -c 'docker-compose.yml')" -gt 0 -a "$(allout | grep 'Generated files' | grep -c 'nginx/sites/auth.conf')" -gt 0
run_nself d 1 doctor --fix --force
check "acc4 doctor --fix --force reports the repair" has "Generated files: repaired"
check "acc4 the repaired files equal the original build" test "$(cmp -s "$WORK/d/project/docker-compose.yml" "$WORK/compose.good" && cmp -s "$WORK/d/project/nginx/sites/auth.conf" "$WORK/auth.good" && echo same)" = same
D1="$(tree d project plugins)"
run_nself d 1 doctor --fix --force
check "acc4 a second --fix changes nothing" test "$(tree d project plugins)" = "$D1"
run_nself d 1 doctor
check "acc4 doctor then reports no drift" has "Generated files: no generated-file drift"

finish
