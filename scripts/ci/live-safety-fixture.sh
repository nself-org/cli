#!/usr/bin/env bash
# Purpose: prove the P7-LIVE-03 live-safety guarantees on the real binary:
#          `nself build --plan` writes nothing and starts nothing, a prod-class
#          change needs --yes (E403, exit 4, in v1.5 mode), --plan-id refuses a
#          stale plan (E450), and a plan after an apply is empty. Prod and dev
#          fixture projects are built in temp dirs; no real project, host or
#          remote is touched.
# Usage:   bash scripts/ci/live-safety-fixture.sh <path-to-nself-binary>
# Inputs:  the binary; python3 on PATH. Local Docker is used only to list
#          containers (and to create, never start, one labelled scratch container
#          when an image is cached); without Docker the container checks are
#          skipped with a note.
# Outputs: one line per assertion ("ok <name>"); the last line is
#          "LIVE-SAFETY-FIXTURE: PASS" on success. Any failure prints
#          "FAIL <name>" to stderr and exits 1.
# Constraints: bash 3.2 compatible. Colima/Docker is never stopped. The scratch
#          container is removed on exit.
set -euo pipefail

BIN="${1:?usage: live-safety-fixture.sh <nself-binary>}"
case "$BIN" in /*) ;; *) BIN="$PWD/$BIN" ;; esac
[ -x "$BIN" ] || { echo "FAIL: $BIN is not executable" >&2; exit 1; }
command -v python3 >/dev/null || { echo "FAIL: python3 is required" >&2; exit 1; }
cd "$(dirname "$0")/../.."
FX="$PWD/internal/reconcile/testdata/fixtures"
REAL_HOME="$HOME"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/live-safety.XXXXXX")"
SCRATCH=""

cleanup() {
  if [ -n "$SCRATCH" ]; then docker rm -f "$SCRATCH" >/dev/null 2>&1 || true; fi
  rm -rf "$WORK"
}
trap cleanup EXIT

ok() { printf 'ok %s\n' "$1"; }
fail() { printf 'FAIL %s\n' "$1" >&2; exit 1; }

# mkproj <name> <fixture>: a project with HOME and the plugin dir beside it.
mkproj() {
  local d="$WORK/$1" f="$FX/$2"
  mkdir -p "$d/project/.nself" "$d/home" "$d/plugins"
  cat "$f/project/dotenv" "$FX/secrets.env" > "$d/project/.env"
  chmod 600 "$d/project/.env"
  if [ -d "$f/plugins" ]; then cp -R "$f/plugins/." "$d/plugins/"; fi
  if [ -f "$f/certs.txt" ]; then
    for c in $(cat "$f/certs.txt"); do
      mkdir -p "$d/project/ssl/certificates/$c"
      echo CERT > "$d/project/ssl/certificates/$c/fullchain.pem"
      echo KEY > "$d/project/ssl/certificates/$c/privkey.pem"
    done
  fi
}

# tree <name>: a hash of every file (content) and directory under the root,
# except the command guard's lock file.
tree() {
  (cd "$WORK/$1" && {
    find . -type d | LC_ALL=C sort
    find . -type f ! -name op.lock | LC_ALL=C sort | while IFS= read -r p; do
      printf '%s %s\n' "$(python3 -c 'import hashlib,sys;print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$p")" "$p"
    done
  } | python3 -c 'import hashlib,sys;print(hashlib.sha256(sys.stdin.buffer.read()).hexdigest())')
}

# nself <name> <v15:0|1> <args...>: run in the project; stdout to $WORK/out,
# stderr to $WORK/err; exit status in $RC.
RC=0
nself() {
  local name="$1" v15="$2"
  shift 2
  local envs="NSELF_CMD_LOG_ENABLED=false"
  if [ "$v15" = 1 ]; then envs="$envs NSELF_V15=1"; fi
  set +e
  (cd "$WORK/$name/project" && env -u NSELF_V15 $envs HOME="$WORK/$name/home" NSELF_PLUGIN_DIR="$WORK/$name/plugins" \
    DOCKER_CONFIG="${DOCKER_CONFIG:-$REAL_HOME/.docker}" AI_AUTO_INSTALL=false "$BIN" "$@" > "$WORK/out" 2> "$WORK/err" < /dev/null)
  RC=$?
  set -e
}

# jget <python-expr over d>: evaluate against the JSON on $WORK/out (data field).
jget() {
  python3 -c 'import json,sys;d=json.load(open(sys.argv[1]));d=d.get("data",d);print(eval(sys.argv[2]))' "$WORK/out" "$1"
}

containers() { docker ps -a --format '{{.ID}} {{.Names}} {{.State}}' 2>/dev/null | LC_ALL=C sort; }
HAVE_DOCKER=0
if docker info >/dev/null 2>&1; then HAVE_DOCKER=1; fi

# ---- prod fixture -----------------------------------------------------------
mkproj prod prod-plugins
if [ "$HAVE_DOCKER" = 1 ]; then
  IMG="$(docker image ls --format '{{.Repository}}:{{.Tag}}' | grep -v '<none>' | head -1 || true)"
  if [ -n "$IMG" ]; then
    SCRATCH="l03-fx-$$"
    docker create --name "$SCRATCH" --label com.docker.compose.project=fx --label com.docker.compose.service=hasura \
      --label com.docker.compose.config-hash=stale "$IMG" >/dev/null
  fi
fi
CBEFORE=""
if [ "$HAVE_DOCKER" = 1 ]; then CBEFORE="$(containers)"; fi
T0="$(tree prod)"

nself prod 1 build --plan --json
[ "$RC" = 0 ] || fail "plan exit $RC: $(cat "$WORK/err")"
[ "$(tree prod)" = "$T0" ] || fail "build --plan changed the project tree, HOME or plugin dir"
ok "plan writes nothing (tree hash unchanged)"
if [ "$HAVE_DOCKER" = 1 ]; then
  [ "$(containers)" = "$CBEFORE" ] || fail "build --plan changed the container list"
  ok "plan starts, stops and creates no container"
fi
[ "$(jget 'd["env_class"]')" = prod ] || fail "plan env_class is not prod"
[ "$(jget 'len(d["artifacts"])>5')" = True ] || fail "plan lists no artifacts"
[ "$(jget 'd["requires_confirmation"]')" = True ] || fail "plan does not require confirmation"
ok "prod plan lists artifacts and requires confirmation"
if [ -n "$SCRATCH" ]; then
  if [ "$(jget 'd["containers"]["known"]')" = True ]; then
    [ "$(jget '[i["action"] for i in d["containers"]["items"] if i["service"]=="hasura"]')" = "['recreate']" ] \
      || fail "the stale-labelled hasura container is not planned as recreate"
    ok "container impact: stale config hash plans recreate"
  else
    echo "note: docker compose could not hash the plan here; container impact reported unknown"
  fi
fi
SHOWN="$(jget 'd["plan_id"]')"

nself prod 1 build --json
[ "$RC" = 4 ] || fail "non-interactive prod build without --yes exited $RC, want 4"
[ "$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["error"]["code"])' "$WORK/out")" = E403 ] || fail "refusal is not E403"
[ "$(tree prod)" = "$T0" ] || fail "the refused build wrote"
ok "v1.5 prod build without --yes: E403, exit 4, nothing written"

nself prod 0 build
[ "$RC" = 0 ] || fail "v1.4 prod build exited $RC"
grep -q "v1.5 will require --yes" "$WORK/err" || fail "v1.4 build printed no notice"
ok "v1.4 prod build proceeds with a notice"

# back to the fresh state for the --yes path
rm -rf "$WORK/prod"
mkproj prod prod-plugins
T0="$(tree prod)"
nself prod 1 build --plan --json
SHOWN="$(jget 'd["plan_id"]')"
echo "MONITORING_ENABLED=true" >> "$WORK/prod/project/.env"
T1="$(tree prod)"
nself prod 1 build --yes --plan-id "$SHOWN" --json
[ "$RC" = 1 ] || fail "stale --plan-id exited $RC, want 1"
[ "$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["error"]["code"])' "$WORK/out")" = E450 ] || fail "stale plan is not E450"
[ "$(tree prod)" = "$T1" ] || fail "the E450 build wrote"
ok "stale --plan-id: E450, exit 1, nothing written"

# The stale-labelled scratch container would (correctly) keep the plan non-empty.
if [ -n "$SCRATCH" ]; then docker rm -f "$SCRATCH" >/dev/null 2>&1 || true; SCRATCH=""; fi
nself prod 1 build --plan --json
FRESH="$(jget 'd["plan_id"]')"
nself prod 1 build --yes --plan-id "$FRESH"
[ "$RC" = 0 ] || fail "build --yes --plan-id (fresh) exited $RC: $(cat "$WORK/err")"
[ -f "$WORK/prod/project/docker-compose.yml" ] || fail "--yes did not build"
nself prod 1 build --plan --json
[ "$(jget 'd["empty"]')" = True ] || fail "plan after apply is not empty"
ok "--yes --plan-id applies; the plan afterwards is empty"

# ---- dev fixture --------------------------------------------------------------
mkproj dev dev-minimal
nself dev 1 build --plan --json
[ "$(jget 'd["requires_confirmation"]')" = False ] || fail "a dev plan requires confirmation"
nself dev 1 build
[ "$RC" = 0 ] || fail "dev build in v1.5 mode exited $RC without --yes"
nself dev 1 build --plan --json
[ "$(jget 'd["empty"]')" = True ] || fail "dev plan after build is not empty"
ok "dev build needs no confirmation; the plan afterwards is empty"

echo "LIVE-SAFETY-FIXTURE: PASS"
